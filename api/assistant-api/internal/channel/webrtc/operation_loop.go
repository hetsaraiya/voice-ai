// Copyright (c) 2023-2026 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_webrtc

import (
	"fmt"
	"strings"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
	webrtc_internal "github.com/rapidaai/api/assistant-api/internal/channel/webrtc/internal"
	"github.com/rapidaai/api/assistant-api/internal/observability"
	"github.com/rapidaai/protos"
)

func (s *webrtcStreamer) runWebRTCOperationLoop() {
	for {
		select {
		case <-s.Ctx.Done():
			return
		case operation := <-s.webrtcOperationCh:
			s.handleWebRTCOperation(operation)
		}
	}
}

func (s *webrtcStreamer) enqueueWebRTCOperation(operation webrtc_internal.WebRTCOperation) {
	if s.webrtcOperationCh == nil {
		s.handleWebRTCOperation(operation)
		return
	}

	switch operation.Kind {
	case webrtc_internal.WebRTCOperationSendOffer,
		webrtc_internal.WebRTCOperationApplyRemoteAnswer,
		webrtc_internal.WebRTCOperationRestartICE,
		webrtc_internal.WebRTCOperationICEGatheringComplete:
		select {
		case s.webrtcOperationCh <- operation:
		case <-s.Ctx.Done():
		}
		return
	}

	select {
	case s.webrtcOperationCh <- operation:
	case <-s.Ctx.Done():
	default:
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelDebug,
			Message: "WebRTC operation queue full, dropping operation",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
				webrtc_internal.DataOperation:      operation.Kind.String(),
				webrtc_internal.DataReason:         operation.Reason,
			},
		})
	}
}

func (s *webrtcStreamer) handleWebRTCOperation(operation webrtc_internal.WebRTCOperation) {
	if operation.MediaSessionID != 0 && !s.sessionState.IsActiveMediaSession(operation.MediaSessionID) {
		return
	}

	switch operation.Kind {
	case webrtc_internal.WebRTCOperationSendOffer:
		if _, err := s.sendWebRTCOffer(operation); err != nil {
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Failed to send WebRTC offer",
				Attributes: observability.Attributes{
					"component":                        observability.ComponentWebRTC.String(),
					webrtc_internal.DataSessionID:      s.sessionID,
					webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
					webrtc_internal.DataReason:         operation.Reason,
					"error":                            err.Error(),
				},
			})
			s.stopMediaSessionAndFallbackToText()
		}

	case webrtc_internal.WebRTCOperationRestartICE:
		if _, err := s.handleICERestartOperation(operation); err != nil {
			requestedAt := operation.RequestedAt
			if requestedAt.IsZero() {
				requestedAt = time.Now()
			}
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "Failed to restart WebRTC ICE, restarting media session",
				Attributes: observability.Attributes{
					"component":                        observability.ComponentWebRTC.String(),
					webrtc_internal.DataSessionID:      s.sessionID,
					webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
					webrtc_internal.DataReason:         operation.Reason,
					"error":                            err.Error(),
				},
			})
			s.restartMediaSessionOrFallbackToText(operation.MediaSessionID, operation.Reason, requestedAt)
		}

	case webrtc_internal.WebRTCOperationApplyRemoteAnswer:
		s.applyRemoteAnswer(operation)

	case webrtc_internal.WebRTCOperationAddRemoteICECandidate:
		s.addRemoteICECandidateFromOperation(operation)

	case webrtc_internal.WebRTCOperationSendLocalICECandidate:
		s.sendLocalICECandidateFromOperation(operation)

	case webrtc_internal.WebRTCOperationICEGatheringComplete:
		s.handleICEGatheringCompleteOperation(operation)
	}
}

func (s *webrtcStreamer) applyRemoteAnswer(operation webrtc_internal.WebRTCOperation) {
	s.Mu.Lock()
	peerConnection := s.peerConnection
	s.Mu.Unlock()
	if peerConnection == nil {
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelDebug,
			Message: "Received SDP answer but peer connection is nil, ignoring",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
			},
		})
		return
	}

	if err := peerConnection.SetRemoteDescription(pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  operation.RemoteAnswerSDP,
	}); err != nil {
		s.clearNegotiationState(peerConnection)
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Failed to set WebRTC remote description",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
				"error":                            err.Error(),
			},
		})
		return
	}

	audioMediaSectionFound := false
	insideAudioMediaSection := false
	negotiatedAudioDirection := "missing"
	audioMediaHasClientStream := false
	for _, answerSDPLine := range strings.Split(operation.RemoteAnswerSDP, "\n") {
		answerSDPLine = strings.TrimSpace(answerSDPLine)
		if strings.HasPrefix(answerSDPLine, "m=") {
			insideAudioMediaSection = strings.HasPrefix(answerSDPLine, "m=audio ")
			if insideAudioMediaSection {
				audioMediaSectionFound = true
				negotiatedAudioDirection = "sendrecv"
			}
			continue
		}
		if !insideAudioMediaSection {
			continue
		}
		switch answerSDPLine {
		case "a=sendrecv", "a=sendonly", "a=recvonly", "a=inactive":
			negotiatedAudioDirection = strings.TrimPrefix(answerSDPLine, "a=")
		default:
			if strings.HasPrefix(answerSDPLine, "a=msid:") {
				audioMediaHasClientStream = true
			}
		}
	}
	if !audioMediaSectionFound {
		negotiatedAudioDirection = "missing"
	}
	_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
		Level:   observability.LevelDebug,
		Message: "WebRTC remote answer audio negotiation",
		Attributes: observability.Attributes{
			"component":                        observability.ComponentWebRTC.String(),
			webrtc_internal.DataSessionID:      s.sessionID,
			webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
			"audio_media_section":              fmt.Sprintf("%t", audioMediaSectionFound),
			"direction":                        negotiatedAudioDirection,
			"client_stream":                    fmt.Sprintf("%t", audioMediaHasClientStream),
		},
	})

	remoteDescriptionSetAt := time.Now()
	s.Mu.Lock()
	if s.peerConnection != peerConnection || !s.sessionState.IsActiveMediaSession(operation.MediaSessionID) {
		s.Mu.Unlock()
		return
	}
	s.mediaHealthState.RecordRemoteDescriptionSet(remoteDescriptionSetAt)
	pendingRemoteICECandidates := append([]pionwebrtc.ICECandidateInit(nil), s.signalPendingRemoteICECandidates...)
	s.signalPendingRemoteICECandidates = nil
	retryNegotiation, retryICE := s.sessionState.CompleteNegotiation()
	mediaSessionID := s.sessionState.ActiveMediaSessionID()
	s.Mu.Unlock()
	s.emitWebRTCNegotiationEvent(webrtc_internal.EventNegotiationAnswerReceived, operation, false, retryNegotiation, remoteDescriptionSetAt)

	for _, candidate := range pendingRemoteICECandidates {
		s.addRemoteICECandidate(peerConnection, candidate)
	}
	if !retryNegotiation {
		return
	}

	var offerOptions *pionwebrtc.OfferOptions
	if retryICE {
		offerOptions = &pionwebrtc.OfferOptions{ICERestart: true}
	}
	retryOperation := webrtc_internal.WebRTCOperation{
		Kind:           webrtc_internal.WebRTCOperationSendOffer,
		MediaSessionID: mediaSessionID,
		Reason:         operation.Reason,
		OfferOptions:   offerOptions,
	}
	var offerSent bool
	var err error
	if retryICE {
		retryOperation.Kind = webrtc_internal.WebRTCOperationRestartICE
		offerSent, err = s.handleICERestartOperation(retryOperation)
	} else {
		offerSent, err = s.sendWebRTCOffer(retryOperation)
	}
	if err != nil {
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Failed to send queued WebRTC negotiation offer",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", mediaSessionID),
				webrtc_internal.DataReason:         operation.Reason,
				"error":                            err.Error(),
			},
		})
		s.queueMediaSessionRestart(mediaSessionID, webrtc_internal.ReasonRemoteAnswerDeadline, time.Now())
		return
	}
	if offerSent {
		s.emitWebRTCNegotiationEvent(webrtc_internal.EventNegotiationRetrySent, retryOperation, retryICE, false, time.Now())
	}
}

func (s *webrtcStreamer) addRemoteICECandidateFromOperation(operation webrtc_internal.WebRTCOperation) {
	s.Mu.Lock()
	peerConnection := s.peerConnection
	s.Mu.Unlock()
	if peerConnection == nil {
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelDebug,
			Message: "Received ICE candidate but peer connection is nil, ignoring",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
			},
		})
		return
	}

	if peerConnection.RemoteDescription() == nil {
		s.Mu.Lock()
		if s.peerConnection == peerConnection && peerConnection.RemoteDescription() == nil {
			if len(s.signalPendingRemoteICECandidates) >= webrtc_internal.PendingRemoteICECandidateLimit {
				s.Mu.Unlock()
				_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
					Level:   observability.LevelDebug,
					Message: "WebRTC pending remote ICE candidate queue full, dropping candidate",
					Attributes: observability.Attributes{
						"component":                        observability.ComponentWebRTC.String(),
						webrtc_internal.DataSessionID:      s.sessionID,
						webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
						"limit":                            fmt.Sprintf("%d", webrtc_internal.PendingRemoteICECandidateLimit),
					},
				})
				return
			}
			s.signalPendingRemoteICECandidates = append(s.signalPendingRemoteICECandidates, operation.RemoteICECandidate)
			s.Mu.Unlock()
			return
		}
		s.Mu.Unlock()
	}

	s.addRemoteICECandidate(peerConnection, operation.RemoteICECandidate)
}

func (s *webrtcStreamer) sendLocalICECandidateFromOperation(operation webrtc_internal.WebRTCOperation) {
	if operation.LocalICECandidate.Candidate == "" {
		return
	}

	iceCandidate := &protos.ICECandidate{
		Candidate: operation.LocalICECandidate.Candidate,
	}
	if operation.LocalICECandidate.SDPMid != nil {
		iceCandidate.SdpMid = *operation.LocalICECandidate.SDPMid
	}
	if operation.LocalICECandidate.SDPMLineIndex != nil {
		iceCandidate.SdpMLineIndex = int32(*operation.LocalICECandidate.SDPMLineIndex)
	}
	if operation.LocalICECandidate.UsernameFragment != nil {
		iceCandidate.UsernameFragment = *operation.LocalICECandidate.UsernameFragment
	}

	s.Mu.Lock()
	if !s.sessionState.IsActiveMediaSession(operation.MediaSessionID) {
		s.Mu.Unlock()
		return
	}
	signalingSessionID := s.signalingSessionID
	if !s.signalOfferSent {
		s.signalPendingLocalICECandidates = append(s.signalPendingLocalICECandidates, iceCandidate)
		s.Mu.Unlock()
		return
	}
	s.Mu.Unlock()
	if signalingSessionID == "" {
		signalingSessionID = s.sessionID
	}

	s.Output(&protos.ServerSignaling{
		SessionId: signalingSessionID,
		Message: &protos.ServerSignaling_IceCandidate{
			IceCandidate: iceCandidate,
		},
	})
}

func (s *webrtcStreamer) handleICERestartOperation(operation webrtc_internal.WebRTCOperation) (bool, error) {
	if s.sessionState.ICEGatheringActive() {
		s.sessionState.DeferICERestart(webrtc_internal.WebRTCDeferredICERestart{
			MediaSessionID: operation.MediaSessionID,
			Reason:         operation.Reason,
			RequestedAt:    operation.RequestedAt,
		})
		s.emitWebRTCNegotiationEvent(webrtc_internal.EventICERestartDeferred, operation, true, false, time.Now())
		return false, nil
	}
	return s.sendWebRTCOffer(operation)
}

func (s *webrtcStreamer) handleICEGatheringCompleteOperation(operation webrtc_internal.WebRTCOperation) {
	s.sessionState.SetICEGatheringActive(false)
	deferredICERestart, ok := s.sessionState.TakeDeferredICERestart(operation.MediaSessionID)
	if !ok {
		return
	}

	restartOperation := webrtc_internal.WebRTCOperation{
		Kind:           webrtc_internal.WebRTCOperationRestartICE,
		MediaSessionID: deferredICERestart.MediaSessionID,
		Reason:         deferredICERestart.Reason,
		RequestedAt:    deferredICERestart.RequestedAt,
		OfferOptions:   &pionwebrtc.OfferOptions{ICERestart: true},
	}
	if _, err := s.handleICERestartOperation(restartOperation); err != nil {
		requestedAt := deferredICERestart.RequestedAt
		if requestedAt.IsZero() {
			requestedAt = time.Now()
		}
		_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
			Level:   observability.LevelError,
			Message: "Failed to restart deferred WebRTC ICE, restarting media session",
			Attributes: observability.Attributes{
				"component":                        observability.ComponentWebRTC.String(),
				webrtc_internal.DataSessionID:      s.sessionID,
				webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", deferredICERestart.MediaSessionID),
				webrtc_internal.DataReason:         deferredICERestart.Reason,
				"error":                            err.Error(),
			},
		})
		s.restartMediaSessionOrFallbackToText(deferredICERestart.MediaSessionID, deferredICERestart.Reason, requestedAt)
	}
}

func (s *webrtcStreamer) emitWebRTCNegotiationEvent(eventType observability.EventName, operation webrtc_internal.WebRTCOperation, iceRestart bool, retryPending bool, occurredAt time.Time) {
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	eventData := observability.Attributes{
		"component":                        observability.ComponentWebRTC.String(),
		webrtc_internal.DataType:           eventType.String(),
		webrtc_internal.DataSessionID:      s.sessionID,
		webrtc_internal.DataMediaSessionID: fmt.Sprintf("%d", operation.MediaSessionID),
		webrtc_internal.DataOperation:      operation.Kind.String(),
		webrtc_internal.DataICERestart:     fmt.Sprintf("%t", iceRestart),
		webrtc_internal.DataRetryPending:   fmt.Sprintf("%t", retryPending),
	}
	if operation.Reason != "" {
		eventData[webrtc_internal.DataReason] = operation.Reason
	}
	_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordEvent{
		Component:  observability.ComponentWebRTC,
		Event:      eventType,
		Attributes: eventData,
	})
}
