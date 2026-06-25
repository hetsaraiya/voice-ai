// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_llm_agentkit

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
)
