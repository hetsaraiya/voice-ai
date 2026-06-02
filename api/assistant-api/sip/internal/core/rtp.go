// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rapidaai/pkg/commons"
	"golang.org/x/sys/unix"
)

// RTP constants
const (
	rtpVersion         = 2
	rtpHeaderSize      = 12
	rtpReadBufferSize  = 65536
	rtpWriteBufferSize = 65536
	rtpPacketMaxSize   = 1500
	rtpReadTimeout     = 100 * time.Millisecond
	rtpPacketInterval  = 20 * time.Millisecond

	// Audio channel buffer sizes
	rtpAudioInBufferSize  = 100
	rtpAudioOutBufferSize = 100
)

// RTPPacket represents an RTP packet
type RTPPacket struct {
	Version        uint8
	Padding        bool
	Extension      bool
	CSRCCount      uint8
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
	Payload        []byte
}

type RTPHandlerFactory func(context.Context, *RTPConfig) (*RTPHandler, error)

// RTPHandler manages RTP streams for SIP calls
// No WebSocket needed - audio goes directly over RTP/UDP
type RTPHandler struct {
	mu      sync.RWMutex
	logger  commons.Logger
	running atomic.Bool

	conn      *net.UDPConn // Receiving socket (ListenUDP, bound to 0.0.0.0:port)
	sendConn  *net.UDPConn // Connected sending socket (DialUDP) — ensures correct UDP checksums
	localIP   string
	localPort int

	remoteAddr *net.UDPAddr

	// RTP state
	ssrc           uint32
	sequenceNumber uint16
	timestamp      uint32
	codec          *Codec

	// Audio channels
	audioInChan  chan []byte
	audioOutChan chan []byte

	// flushAudioCh signals the sendLoop to discard all pending audio
	// (used on user interruption to silence stale frames immediately).
	flushAudioCh chan struct{}

	// codecVersion is bumped by SetCodec so the sendLoop can detect mid-call
	// codec changes and regenerate its pre-computed silence chunk.
	codecVersion uint32

	ctx    context.Context
	cancel context.CancelFunc

	// Statistics
	packetsSent     atomic.Uint64
	packetsReceived atomic.Uint64
	bytesReceived   atomic.Uint64
	bytesSent       atomic.Uint64
	firstPacketSeen atomic.Bool
	onFirstPacket   func()
}

// RTPConfig holds configuration for RTP handler
type RTPConfig struct {
	LocalIP     string
	LocalPort   int
	PayloadType uint8  // 0 = PCMU, 8 = PCMA
	ClockRate   uint32 // 8000 for G.711
	Logger      commons.Logger
}

// Validate validates the RTP configuration
func (c *RTPConfig) Validate() error {
	if c.LocalIP == "" {
		return fmt.Errorf("local_ip is required")
	}
	if c.LocalPort < 0 || c.LocalPort > 65535 {
		return fmt.Errorf("invalid local_port: %d", c.LocalPort)
	}
	if c.ClockRate == 0 {
		c.ClockRate = 8000 // Default to 8kHz
	}
	return nil
}

// NewRTPHandler creates a new RTP handler for direct audio transport
func NewRTPHandler(ctx context.Context, config *RTPConfig) (*RTPHandler, error) {
	if err := config.Validate(); err != nil {
		return nil, NewSIPError("NewRTPHandler", "", "invalid configuration", err)
	}

	handlerCtx, cancel := context.WithCancel(ctx)

	ip := net.ParseIP(config.LocalIP)
	addr := &net.UDPAddr{
		IP:   ip,
		Port: config.LocalPort,
	}

	// Use "udp4" explicitly for IPv4 addresses to prevent Go from creating an
	// IPv6 socket (::) that won't receive IPv4 RTP packets on macOS/BSD.
	// On Linux, dual-stack sockets receive both, but macOS disables IPV6_V6ONLY
	// by default, so an IPv6 socket never sees IPv4 traffic.
	network := "udp4"
	if ip != nil && ip.To4() == nil {
		network = "udp6"
	}

	// Use net.ListenConfig with SO_REUSEADDR + SO_REUSEPORT so that a connected
	// send socket (created later in SetRemoteAddr) can bind to the same local port.
	// The connected send socket ensures correct UDP checksums for outbound RTP,
	// which prevents "bad udp cksum" errors that cause call disconnects.
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			if err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if opErr != nil {
					return
				}
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}); err != nil {
				return err
			}
			return opErr
		},
	}

	packetConn, err := lc.ListenPacket(handlerCtx, network, addr.String())
	if err != nil {
		cancel()
		return nil, NewSIPError("NewRTPHandler", "", "failed to create RTP socket", err)
	}
	conn := packetConn.(*net.UDPConn)

	// Set buffer sizes
	if err := conn.SetReadBuffer(rtpReadBufferSize); err != nil && config.Logger != nil {
		config.Logger.Warnw("Failed to set RTP read buffer size", "error", err)
	}
	if err := conn.SetWriteBuffer(rtpWriteBufferSize); err != nil && config.Logger != nil {
		config.Logger.Warnw("Failed to set RTP write buffer size", "error", err)
	}

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Get codec from payload type or use default
	codec := GetCodecByPayloadType(config.PayloadType)
	if codec == nil {
		codec = &CodecPCMU
	}

	handler := &RTPHandler{
		logger:       config.Logger,
		conn:         conn,
		localIP:      localAddr.IP.String(),
		localPort:    localAddr.Port,
		ssrc:         rand.Uint32(),
		codec:        codec,
		audioInChan:  make(chan []byte, rtpAudioInBufferSize),
		audioOutChan: make(chan []byte, rtpAudioOutBufferSize),
		flushAudioCh: make(chan struct{}, 1),
		ctx:          handlerCtx,
		cancel:       cancel,
	}

	return handler, nil
}

// Start begins RTP processing
func (h *RTPHandler) Start() {
	if !h.running.CompareAndSwap(false, true) {
		return // Already running
	}

	// Log the actual socket address the OS assigned (important: 0.0.0.0 vs specific IP)
	h.logger.Infow("RTP Start() called",
		"conn_local_addr", h.conn.LocalAddr().String(),
		"handler_local_ip", h.localIP,
		"handler_local_port", h.localPort,
		"remote_addr", fmt.Sprintf("%v", h.remoteAddr),
		"codec", h.codec.Name,
		"ssrc", h.ssrc)

	// Send an initial silence packet immediately to "punch" the RTP path.
	// Some PBXes (Asterisk with direct_media) expect to see RTP traffic very
	// quickly after the call is bridged. Waiting for the 20ms send cycle
	// may be too slow.
	h.sendInitialSilence()

	go h.receiveLoop()
	go h.sendLoop()

	h.logger.Infow("RTP handler started — sendLoop and receiveLoop launched",
		"local_addr", fmt.Sprintf("%s:%d", h.localIP, h.localPort),
		"codec", h.codec.Name)
}

// sendInitialSilence sends the first silence RTP packet synchronously to
// "punch" the RTP path immediately, then returns. The sendLoop goroutine
// will take over and keep sending silence every 20ms until real audio arrives.
func (h *RTPHandler) sendInitialSilence() {
	h.mu.RLock()
	remoteAddr := h.remoteAddr
	h.mu.RUnlock()

	if remoteAddr == nil {
		if h.logger != nil {
			h.logger.Warnw("sendInitialSilence: remoteAddr is nil — no RTP will be sent until remote address is set")
		}
		return
	}

	samplesPerPacket := int(h.codec.ClockRate * 20 / 1000)
	chunk := h.createSilenceChunk(samplesPerPacket)
	packet := h.createRTPPacket(chunk)
	data := h.serializeRTPPacket(packet)

	// LOG BEFORE WRITE: exact socket + destination + packet size
	h.logger.Debugw("sendInitialSilence: sending RTP",
		"conn_local_addr", h.conn.LocalAddr().String(),
		"dest_addr", remoteAddr.String(),
		"dest_ip", remoteAddr.IP.String(),
		"dest_port", remoteAddr.Port,
		"packet_bytes", len(data),
		"payload_bytes", len(chunk),
		"seq", packet.SequenceNumber,
		"ssrc", packet.SSRC,
		"has_send_conn", h.sendConn != nil)

	n, err := h.sendPacket(data, remoteAddr)
	if err != nil {
		if h.logger != nil {
			h.logger.Warnw("sendInitialSilence: send FAILED",
				"error", err,
				"dest", remoteAddr.String(),
				"conn_local", h.conn.LocalAddr().String())
		}
		return
	}

	h.packetsSent.Add(1)
	h.bytesSent.Add(uint64(len(chunk)))
	if h.logger != nil {
		h.logger.Debugw("sendInitialSilence: sent",
			"bytes_written", n,
			"dest", remoteAddr.String(),
			"conn_local", h.conn.LocalAddr().String(),
			"seq", packet.SequenceNumber,
			"payload_size", len(chunk))
	}
}

// Stop stops RTP processing gracefully
func (h *RTPHandler) Stop() error {
	if !h.running.CompareAndSwap(true, false) {
		return nil // Already stopped
	}

	h.cancel()

	// Close channels safely
	h.closeChannels()

	// Close the connected send socket first (if any)
	h.mu.Lock()
	if h.sendConn != nil {
		h.sendConn.Close()
		h.sendConn = nil
	}
	h.mu.Unlock()

	var err error
	if h.conn != nil {
		err = h.conn.Close()
	}

	if h.logger != nil {
		sent, received := h.GetStats()
		h.logger.Debugw("RTP handler stopped",
			"packets_sent", sent,
			"packets_received", received)
	}

	return err
}

// closeChannels safely closes audio channels
func (h *RTPHandler) closeChannels() {
	defer func() {
		recover() // Recover if channels are already closed
	}()
	close(h.audioInChan)
	close(h.audioOutChan)
}

// IsRunning returns whether the RTP handler is running
func (h *RTPHandler) IsRunning() bool {
	return h.running.Load()
}

// SetRemoteAddr sets the remote RTP endpoint and creates a connected UDP
// socket for sending. A connected socket (via net.DialUDP) ensures the kernel
// fills the correct source IP in the UDP pseudo-header, which fixes "bad udp
// cksum" errors seen when the receiving socket is bound to 0.0.0.0.
// This is critical for outbound calls — without a connected send socket,
// some firewalls/NAT/SBCs drop packets with bad checksums, causing the call
// to disconnect immediately when the user picks up.
func (h *RTPHandler) SetRemoteAddr(ip string, port int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	parsedIP := net.ParseIP(ip)
	h.remoteAddr = &net.UDPAddr{
		IP:   parsedIP,
		Port: port,
	}

	// Create a connected UDP socket for sending.
	// net.DialUDP "connects" the UDP socket to the remote address, which:
	// 1. Locks in the source IP (kernel picks the correct interface route)
	// 2. Guarantees correct UDP checksum computation in the pseudo-header
	// 3. Allows using Write() instead of WriteToUDP() — slightly faster
	//
	// We bind the local side to the same port as the receiving socket so
	// that the remote side sees RTP coming from the port we advertised in SDP.
	// This requires SO_REUSEADDR + SO_REUSEPORT since the port is already
	// bound by the receive socket (h.conn).

	// Close any previous send socket (e.g., re-INVITE changed the remote address)
	if h.sendConn != nil {
		h.sendConn.Close()
		h.sendConn = nil
	}

	network := "udp4"
	if parsedIP != nil && parsedIP.To4() == nil {
		network = "udp6"
	}

	// Use net.Dialer with Control to set SO_REUSEADDR + SO_REUSEPORT before bind.
	// This allows the send socket to share the same local port as the receive socket.
	dialer := net.Dialer{
		LocalAddr: &net.UDPAddr{
			IP:   net.ParseIP(h.localIP),
			Port: h.localPort,
		},
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			if err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if opErr != nil {
					return
				}
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}); err != nil {
				return err
			}
			return opErr
		},
	}

	rawConn, err := dialer.DialContext(h.ctx, network, h.remoteAddr.String())
	if err != nil {
		if h.logger != nil {
			h.logger.Warnw("RTP: failed to create connected send socket, falling back to unconnected writes",
				"error", err,
				"local_port", h.localPort,
				"remote", h.remoteAddr.String())
		}
		// sendConn stays nil — sendPacket() will fall back to WriteToUDP on h.conn
	} else {
		sendConn, ok := rawConn.(*net.UDPConn)
		if !ok {
			rawConn.Close()
			if h.logger != nil {
				h.logger.Warnw("RTP: DialContext returned non-UDP connection, falling back to unconnected writes")
			}
		} else {
			// Apply same buffer sizes as the receive socket
			_ = sendConn.SetWriteBuffer(rtpWriteBufferSize)
			h.sendConn = sendConn
			if h.logger != nil {
				h.logger.Infow("RTP: connected send socket created (fixes UDP checksum)",
					"local", sendConn.LocalAddr().String(),
					"remote", sendConn.RemoteAddr().String())
			}
		}
	}

	if h.logger != nil {
		h.logger.Infow("RTP remote address set",
			"input_ip", ip,
			"parsed_ip", fmt.Sprintf("%v", parsedIP),
			"port", port,
			"resolved_addr", h.remoteAddr.String(),
			"has_send_conn", h.sendConn != nil,
			"is_ipv4", parsedIP != nil && parsedIP.To4() != nil)
	}
}

// GetRemoteAddr returns the remote RTP address
func (h *RTPHandler) GetRemoteAddr() *net.UDPAddr {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.remoteAddr
}

// LocalAddr returns the local RTP address
func (h *RTPHandler) LocalAddr() (string, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.localIP, h.localPort
}

// AudioIn returns the channel for received audio
func (h *RTPHandler) AudioIn() <-chan []byte {
	return h.audioInChan
}

// AudioOut returns the channel for sending audio
func (h *RTPHandler) AudioOut() chan<- []byte {
	return h.audioOutChan
}

// FlushAudioOut signals the sendLoop to discard all pending audio.
// Used on user interruption to silence stale frames immediately.
func (h *RTPHandler) FlushAudioOut() {
	select {
	case h.flushAudioCh <- struct{}{}:
	default:
	}
}

// GetCodec returns the codec used by this handler
func (h *RTPHandler) GetCodec() *Codec {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.codec
}

// SetCodec updates the codec used by this RTP handler.
// This is needed when the remote side answers with a different codec than
// what was initially offered (e.g., PCMA instead of PCMU). The payload type
// and clock rate of outgoing packets are updated immediately; the silence
// pattern is also adjusted (0xFF for PCMU, 0xD5 for PCMA).
// The codecVersion counter is bumped so the sendLoop regenerates its
// pre-computed silence chunk on the next iteration.
func (h *RTPHandler) SetCodec(codec *Codec) {
	if codec == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	old := h.codec
	h.codec = codec
	h.codecVersion++
	if h.logger != nil {
		h.logger.Infow("RTP codec updated",
			"old_codec", old.Name,
			"new_codec", codec.Name,
			"payload_type", codec.PayloadType,
			"clock_rate", codec.ClockRate,
			"codec_version", h.codecVersion)
	}
}

func (h *RTPHandler) receiveLoop() {
	// Safety net: recover from "send on closed channel" panic that can occur
	// if Stop() closes audioInChan while this goroutine is mid-send.
	defer func() {
		if r := recover(); r != nil {
			if h.logger != nil {
				h.logger.Warnw("RTP receiveLoop recovered from panic", "panic", r)
			}
		}
	}()

	if h.logger != nil {
		h.logger.Infow("RTP receiveLoop started",
			"local_addr", h.conn.LocalAddr().String(),
			"codec", h.codec.Name)
	}

	buf := make([]byte, rtpPacketMaxSize)

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		// Exit early if handler is stopped — avoids sending to closed channels
		if !h.running.Load() {
			return
		}

		// Determine which socket to read from.
		//
		// When a connected send socket (sendConn) exists, the kernel routes
		// inbound UDP packets from the remote peer to it instead of the
		// unconnected receive socket (h.conn), because SO_REUSEPORT with a
		// connected socket creates a more-specific 4-tuple match. If we only
		// read from h.conn, we will never see any packets.
		//
		// Strategy:
		//   - If sendConn is available, read from it (connected → Read).
		//   - Otherwise, fall back to h.conn (unconnected → ReadFromUDP).
		//
		// We re-check sendConn each iteration so that a mid-call re-INVITE
		// (which calls SetRemoteAddr → creates a new sendConn) is picked up.
		h.mu.RLock()
		sendConn := h.sendConn
		h.mu.RUnlock()

		var n int
		var remoteAddr *net.UDPAddr
		var err error

		if sendConn != nil {
			// Connected socket: the kernel delivers packets from the connected
			// remote peer here. Use Read() — the source address is the
			// connected remote, which we already know.
			if deadlineErr := sendConn.SetReadDeadline(time.Now().Add(rtpReadTimeout)); deadlineErr != nil {
				continue
			}
			n, err = sendConn.Read(buf)
			if err == nil {
				// Source is the connected remote address
				h.mu.RLock()
				remoteAddr = h.remoteAddr
				h.mu.RUnlock()
			}
		} else {
			// Unconnected socket: no sendConn yet (remote address not set,
			// or DialUDP failed). Read from the listener socket.
			if deadlineErr := h.conn.SetReadDeadline(time.Now().Add(rtpReadTimeout)); deadlineErr != nil {
				continue
			}
			n, remoteAddr, err = h.conn.ReadFromUDP(buf)
		}

		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// When SetRemoteAddr replaces sendConn during a re-INVITE, the
			// old socket is closed and Read() returns "use of closed network
			// connection". This is expected — the next iteration will pick up
			// the new sendConn. Don't log it as a warning.
			if sendConn != nil && errors.Is(err, net.ErrClosed) {
				continue
			}
			// Connection closed or other error
			if h.running.Load() && h.logger != nil {
				// h.logger.Warnw("RTP receive error", "error", err, "from_send_conn", sendConn != nil)
			}
			continue
		}

		if n < rtpHeaderSize {
			if h.logger != nil {
				h.logger.Warnw("RTP packet too small", "size", n)
			}
			continue
		}

		packet, err := h.parseRTPPacket(buf[:n])
		if err != nil {
			if h.logger != nil {
				h.logger.Warnw("Failed to parse RTP packet", "error", err)
			}
			continue
		}

		// Auto-detect remote address if not set
		if remoteAddr != nil {
			h.mu.Lock()
			if h.remoteAddr == nil {
				h.remoteAddr = remoteAddr
				if h.logger != nil {
					h.logger.Info("RTP: Auto-detected remote address", "addr", remoteAddr.String())
				}
			}
			h.mu.Unlock()
		}

		// Update statistics
		h.packetsReceived.Add(1)
		h.bytesReceived.Add(uint64(len(packet.Payload)))
		h.notifyFirstPacketReceived()
		// running state and context together with the send.
		if !h.running.Load() {
			return
		}
		select {
		case <-h.ctx.Done():
			return
		case h.audioInChan <- packet.Payload:
			// Successfully sent to channel
		default:
			if h.logger != nil {
				h.logger.Warnw("RTP: Audio input channel full, dropping packet", "seq", packet.SequenceNumber)
			}
		}
	}
}

func (h *RTPHandler) SetOnFirstPacket(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onFirstPacket = fn
}

func (h *RTPHandler) notifyFirstPacketReceived() {
	if !h.firstPacketSeen.CompareAndSwap(false, true) {
		return
	}
	h.mu.RLock()
	fn := h.onFirstPacket
	h.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func (h *RTPHandler) sendLoop() {
	// Calculate samples per packet based on codec (20ms packets)
	h.mu.RLock()
	samplesPerPacket := int(h.codec.ClockRate * 20 / 1000) // e.g., 160 bytes for PCMU at 8kHz
	lastCodecVersion := h.codecVersion
	h.mu.RUnlock()

	// Pre-create silence chunk (μ-law silence is 0xFF, PCMA silence is 0xD5)
	silenceChunk := h.createSilenceChunk(samplesPerPacket)

	var pendingAudio []byte
	// First sendLoop packet should go out immediately (sendInitialSilence
	// already sent packet #1, this will send packet #2 without delay).
	nextSendTime := time.Now()

	for {
		// Check for context cancellation
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		// If the codec changed (e.g., via re-INVITE), regenerate the
		// silence chunk so it uses the correct silence byte pattern.
		h.mu.RLock()
		cv := h.codecVersion
		h.mu.RUnlock()
		if cv != lastCodecVersion {
			lastCodecVersion = cv
			h.mu.RLock()
			samplesPerPacket = int(h.codec.ClockRate * 20 / 1000)
			h.mu.RUnlock()
			silenceChunk = h.createSilenceChunk(samplesPerPacket)
		}

		// Collect pending audio (non-blocking) or handle flush signal
		select {
		case <-h.flushAudioCh:
			// Interruption: discard all queued audio immediately
			pendingAudio = nil
			// Drain any remaining audio in the channel
			for {
				select {
				case <-h.audioOutChan:
				default:
					goto collectDone
				}
			}
		case audio, ok := <-h.audioOutChan:
			if ok {
				pendingAudio = append(pendingAudio, audio...)
			}
		default:
		}
	collectDone:

		// Wait until next send time with precision
		now := time.Now()
		if sleepDuration := nextSendTime.Sub(now); sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}

		// Schedule next send immediately to minimize drift
		nextSendTime = nextSendTime.Add(rtpPacketInterval)

		// If we've fallen behind, reset timing (don't try to catch up)
		if time.Now().After(nextSendTime) {
			nextSendTime = time.Now().Add(rtpPacketInterval)
		}

		h.mu.RLock()
		remoteAddr := h.remoteAddr
		h.mu.RUnlock()

		if remoteAddr == nil {
			continue
		}

		// Get exactly ONE chunk: audio if available, otherwise silence
		chunk := h.getAudioChunk(&pendingAudio, samplesPerPacket, silenceChunk)

		packet := h.createRTPPacket(chunk)
		data := h.serializeRTPPacket(packet)

		_, err := h.sendPacket(data, remoteAddr)
		if err != nil {
			if h.running.Load() && h.logger != nil {
				h.logger.Warnw("RTP sendLoop: send FAILED",
					"error", err,
					"dest", remoteAddr.String(),
					"conn_local", h.conn.LocalAddr().String())
			}
			continue
		}

		// Update statistics
		h.packetsSent.Add(1)
		h.bytesSent.Add(uint64(len(chunk)))

	}
}

// createSilenceChunk creates a silence chunk for the codec
func (h *RTPHandler) createSilenceChunk(size int) []byte {
	chunk := make([]byte, size)
	silenceValue := byte(0xFF) // μ-law silence
	if h.codec.Name == "PCMA" {
		silenceValue = 0xD5 // A-law silence
	}
	for i := range chunk {
		chunk[i] = silenceValue
	}
	return chunk
}

// getAudioChunk extracts or creates an audio chunk for sending
func (h *RTPHandler) getAudioChunk(pendingAudio *[]byte, size int, silenceChunk []byte) []byte {
	if len(*pendingAudio) >= size {
		chunk := (*pendingAudio)[:size]
		*pendingAudio = (*pendingAudio)[size:]
		return chunk
	}

	if len(*pendingAudio) > 0 {
		// Partial audio - pad with silence
		silenceValue := silenceChunk[0]
		chunk := make([]byte, size)
		copy(chunk, *pendingAudio)
		for i := len(*pendingAudio); i < size; i++ {
			chunk[i] = silenceValue
		}
		*pendingAudio = nil
		return chunk
	}

	// No audio - return silence
	return silenceChunk
}

func (h *RTPHandler) parseRTPPacket(data []byte) (*RTPPacket, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("packet too small")
	}

	packet := &RTPPacket{
		Version:        (data[0] >> 6) & 0x03,
		Padding:        (data[0] & 0x20) != 0,
		Extension:      (data[0] & 0x10) != 0,
		CSRCCount:      data[0] & 0x0F,
		Marker:         (data[1] & 0x80) != 0,
		PayloadType:    data[1] & 0x7F,
		SequenceNumber: binary.BigEndian.Uint16(data[2:4]),
		Timestamp:      binary.BigEndian.Uint32(data[4:8]),
		SSRC:           binary.BigEndian.Uint32(data[8:12]),
	}

	if packet.Version != 2 {
		return nil, fmt.Errorf("unsupported RTP version: %d", packet.Version)
	}

	headerLen := 12 + int(packet.CSRCCount)*4

	if packet.Extension && len(data) >= headerLen+4 {
		extLen := binary.BigEndian.Uint16(data[headerLen+2 : headerLen+4])
		headerLen += 4 + int(extLen)*4
	}

	payloadLen := len(data) - headerLen
	if packet.Padding && payloadLen > 0 {
		paddingLen := int(data[len(data)-1])
		payloadLen -= paddingLen
	}

	if payloadLen < 0 || headerLen+payloadLen > len(data) {
		return nil, fmt.Errorf("invalid packet length")
	}

	packet.Payload = make([]byte, payloadLen)
	copy(packet.Payload, data[headerLen:headerLen+payloadLen])

	return packet, nil
}

// sendPacket sends serialized RTP data to the remote address.
// Prefers the connected send socket (h.sendConn) which produces correct UDP
// checksums. Falls back to WriteToUDP on the receive socket if no connected
// socket is available (e.g., DialUDP failed or remote not yet set).
func (h *RTPHandler) sendPacket(data []byte, remoteAddr *net.UDPAddr) (int, error) {
	h.mu.RLock()
	sendConn := h.sendConn
	h.mu.RUnlock()

	if sendConn != nil {
		// Connected socket — Write() uses the pre-connected remote address.
		// The kernel computes the correct UDP checksum because the source IP
		// is locked to the interface selected during DialUDP.
		return sendConn.Write(data)
	}

	// Fallback: unconnected socket — may produce bad checksums when bound to 0.0.0.0
	return h.conn.WriteToUDP(data, remoteAddr)
}

func (h *RTPHandler) createRTPPacket(payload []byte) *RTPPacket {
	h.mu.Lock()
	defer h.mu.Unlock()

	packet := &RTPPacket{
		Version:        rtpVersion,
		PayloadType:    h.codec.PayloadType,
		SequenceNumber: h.sequenceNumber,
		Timestamp:      h.timestamp,
		SSRC:           h.ssrc,
		Payload:        payload,
	}

	h.sequenceNumber++
	h.timestamp += uint32(len(payload))

	return packet
}

func (h *RTPHandler) serializeRTPPacket(packet *RTPPacket) []byte {
	headerLen := 12 + len(packet.CSRC)*4
	data := make([]byte, headerLen+len(packet.Payload))

	data[0] = (packet.Version << 6)
	if packet.Padding {
		data[0] |= 0x20
	}
	if packet.Extension {
		data[0] |= 0x10
	}
	data[0] |= packet.CSRCCount & 0x0F

	data[1] = packet.PayloadType & 0x7F
	if packet.Marker {
		data[1] |= 0x80
	}

	binary.BigEndian.PutUint16(data[2:4], packet.SequenceNumber)
	binary.BigEndian.PutUint32(data[4:8], packet.Timestamp)
	binary.BigEndian.PutUint32(data[8:12], packet.SSRC)

	for i, csrc := range packet.CSRC {
		binary.BigEndian.PutUint32(data[12+i*4:16+i*4], csrc)
	}

	copy(data[headerLen:], packet.Payload)

	return data
}

// GetStats returns RTP statistics
func (h *RTPHandler) GetStats() (sent, received uint64) {
	return h.packetsSent.Load(), h.packetsReceived.Load()
}

// GetDetailedStats returns detailed RTP statistics
func (h *RTPHandler) GetDetailedStats() RTPStats {
	return RTPStats{
		PacketsSent:     h.packetsSent.Load(),
		PacketsReceived: h.packetsReceived.Load(),
		BytesSent:       h.bytesSent.Load(),
		BytesReceived:   h.bytesReceived.Load(),
	}
}
