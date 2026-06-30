// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

import "errors"

const (
	CertificateInsecure   = "insecure"
	CertificateSkipVerify = "skip-verify"

	TransportSecurityTLS       = "TLS"
	TransportSecurityPlaintext = "PLAINTEXT"

	TLSVerificationVerify     = "VERIFY"
	TLSVerificationSkipVerify = "SKIP_VERIFY"

	DefaultTransportSecurity   = TransportSecurityTLS
	DefaultTLSVerification     = TLSVerificationVerify
	DefaultConnectTimeoutMs    = uint32(10000)
	DefaultKeepaliveTimeMs     = uint32(30000)
	DefaultKeepaliveTimeoutMs  = uint32(10000)
	DefaultMaxRecvMessageBytes = uint32(16777216)
	DefaultMaxSendMessageBytes = uint32(16777216)

	MinConnectTimeoutMs   = 1
	MaxConnectTimeoutMs   = 300000
	MinKeepaliveTimeMs    = 10000
	MaxKeepaliveTimeMs    = 3600000
	MinKeepaliveTimeoutMs = 1000
	MaxKeepaliveTimeoutMs = 300000
	MinMessageBytes       = 1024
	MaxMessageBytes       = 104857600

	AgentkitObservabilityPrefix = "agentkit."
	AgentkitToolIDPrefix        = "agentkit-tool-"
)

var (
	ErrAgentkitCommunicationRequired              = errors.New("agentkit communication is required")
	ErrAgentkitConfigurationRequired              = errors.New("agentkit configuration is required")
	ErrAgentkitAssistantRequired                  = errors.New("agentkit assistant is required")
	ErrAgentkitProviderConfigurationRequired      = errors.New("agentkit provider configuration is required")
	ErrAgentkitInitializationConnect              = errors.New("agentkit connect failed")
	ErrAgentkitInitializationOpenTalkStream       = errors.New("agentkit stream start failed")
	ErrAgentkitInitializationSend                 = errors.New("agentkit failed to send initialization")
	ErrAgentkitExecuteUnsupportedPacket           = errors.New("agentkit unsupported packet type")
	ErrAgentkitExecutorNotConnected               = errors.New("agentkit executor not connected")
	ErrAgentkitResponse                           = errors.New("agentkit error")
	ErrAgentkitConnectionDialOptions              = errors.New("agentkit connection dial options failed")
	ErrAgentkitConnectionConnect                  = errors.New("agentkit connection connect failed")
	ErrAgentkitConnectionOpenTalkStream           = errors.New("agentkit connection open talk stream failed")
	ErrAgentkitConnectionSend                     = errors.New("agentkit connection send failed")
	ErrAgentkitConnectionRecv                     = errors.New("agentkit connection recv failed")
	ErrAgentkitConnectionCloseStream              = errors.New("agentkit connection close stream failed")
	ErrAgentkitConnectionCloseConn                = errors.New("agentkit connection close connection failed")
	ErrAgentkitConnectionNotConnected             = errors.New("agentkit connection not connected")
	ErrAgentkitConnectionStreamAlreadyOpen        = errors.New("agentkit connection stream already open")
	ErrAgentkitConnectionChanged                  = errors.New("agentkit connection changed")
	ErrAgentkitConnectionInvalidCertificate       = errors.New("agentkit connection invalid certificate")
	ErrAgentkitConnectionInvalidTransportSecurity = errors.New("agentkit connection invalid transport security")
)
