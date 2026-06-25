// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package errors

import "net/http"

const (
	CreateAssistantInvalidRequestCode                     ErrorCode = 1001001
	CreateAssistantUnauthenticatedCode                    ErrorCode = 1001002
	CreateAssistantMissingAuthScopeCode                   ErrorCode = 1001003
	CreateAssistantMissingNameCode                        ErrorCode = 1001004
	CreateAssistantMissingProviderCode                    ErrorCode = 1001005
	CreateAssistantInvalidProviderCode                    ErrorCode = 1001006
	CreateAssistantMissingModelProviderNameCode           ErrorCode = 1001007
	CreateAssistantMissingAgentKitURLCode                 ErrorCode = 1001008
	CreateAssistantMissingWebsocketURLCode                ErrorCode = 1001009
	CreateAssistantInvalidSourceIdentifierCode            ErrorCode = 1001010
	CreateAssistantCreateAssistantCode                    ErrorCode = 1001011
	CreateAssistantInvalidProviderTemplateCode            ErrorCode = 1001012
	CreateAssistantCreateProviderModelCode                ErrorCode = 1001013
	CreateAssistantAttachProviderModelCode                ErrorCode = 1001014
	CreateAssistantCreateProviderAgentkitCode             ErrorCode = 1001015
	CreateAssistantAttachProviderAgentkitCode             ErrorCode = 1001016
	CreateAssistantCreateProviderWebsocketCode            ErrorCode = 1001017
	CreateAssistantAttachProviderWebsocketCode            ErrorCode = 1001018
	CreateAssistantInvalidKnowledgeIDCode                 ErrorCode = 1001019
	CreateAssistantInvalidRerankerModelProviderIDCode     ErrorCode = 1001020
	CreateAssistantCreateToolsCode                        ErrorCode = 1001021
	CreateAssistantCreateKnowledgeCode                    ErrorCode = 1001022
	CreateAssistantCreateTagsCode                         ErrorCode = 1001023
	CreateAssistantInvalidAgentKitCertificateCode         ErrorCode = 1001024
	CreateAssistantInvalidAgentKitTransportCode           ErrorCode = 1001025
	CreateAssistantInvalidAgentKitTLSVerificationCode     ErrorCode = 1001026
	CreateAssistantInvalidAgentKitConnectTimeoutCode      ErrorCode = 1001027
	CreateAssistantInvalidAgentKitKeepaliveTimeCode       ErrorCode = 1001028
	CreateAssistantInvalidAgentKitKeepaliveTimeoutCode    ErrorCode = 1001029
	CreateAssistantInvalidAgentKitMaxRecvMessageBytesCode ErrorCode = 1001030
	CreateAssistantInvalidAgentKitMaxSendMessageBytesCode ErrorCode = 1001031
)

var (
	CreateAssistantInvalidRequest = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidRequestCode,
		Error:          "invalid request",
		ErrorMessage:   "Invalid request.",
	}
	CreateAssistantUnauthenticated = PlatformError{
		HTTPStatusCode: http.StatusUnauthorized,
		Code:           CreateAssistantUnauthenticatedCode,
		Error:          "unauthenticated request",
		ErrorMessage:   "Unauthenticated request, please try again with valid authentication.",
	}
	CreateAssistantMissingAuthScope = PlatformError{
		HTTPStatusCode: http.StatusForbidden,
		Code:           CreateAssistantMissingAuthScopeCode,
		Error:          "missing authentication scope",
		ErrorMessage:   "Unauthenticated request, please try again with valid authentication.",
	}
	CreateAssistantMissingName = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantMissingNameCode,
		Error:          "missing name parameter",
		ErrorMessage:   "Please provide the required name parameter.",
	}
	CreateAssistantMissingProvider = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantMissingProviderCode,
		Error:          "missing assistant_provider parameter",
		ErrorMessage:   "Please provide the required assistantProvider parameter.",
	}
	CreateAssistantInvalidProvider = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidProviderCode,
		Error:          "invalid assistant_provider parameter",
		ErrorMessage:   "Please provide exactly one assistant provider.",
	}
	CreateAssistantMissingModelProviderName = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantMissingModelProviderNameCode,
		Error:          "missing model_provider_name parameter",
		ErrorMessage:   "Please provide the required modelProviderName parameter.",
	}
	CreateAssistantMissingAgentKitURL = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantMissingAgentKitURLCode,
		Error:          "missing agent_kit_url parameter",
		ErrorMessage:   "Please provide the required agentKitUrl parameter.",
	}
	CreateAssistantMissingWebsocketURL = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantMissingWebsocketURLCode,
		Error:          "missing websocket_url parameter",
		ErrorMessage:   "Please provide the required websocketUrl parameter.",
	}
	CreateAssistantInvalidSourceIdentifier = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidSourceIdentifierCode,
		Error:          "invalid source_identifier parameter",
		ErrorMessage:   "Please provide a valid sourceIdentifier parameter.",
	}
	CreateAssistantCreateAssistant = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateAssistantCode,
		Error:          "unable to create assistant",
		ErrorMessage:   "Unable to create assistant, please try again later.",
	}
	CreateAssistantInvalidProviderTemplate = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidProviderTemplateCode,
		Error:          "invalid assistant provider template",
		ErrorMessage:   "Invalid assistant provider template.",
	}
	CreateAssistantCreateProviderModel = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateProviderModelCode,
		Error:          "unable to create assistant provider model",
		ErrorMessage:   "Unable to create assistant provider model, please try again later.",
	}
	CreateAssistantAttachProviderModel = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantAttachProviderModelCode,
		Error:          "unable to attach assistant provider model",
		ErrorMessage:   "Unable to attach assistant provider model, please try again later.",
	}
	CreateAssistantCreateProviderAgentkit = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateProviderAgentkitCode,
		Error:          "unable to create assistant provider agentkit",
		ErrorMessage:   "Unable to create assistant provider agentkit, please try again later.",
	}
	CreateAssistantAttachProviderAgentkit = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantAttachProviderAgentkitCode,
		Error:          "unable to attach assistant provider agentkit",
		ErrorMessage:   "Unable to attach assistant provider agentkit, please try again later.",
	}
	CreateAssistantCreateProviderWebsocket = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateProviderWebsocketCode,
		Error:          "unable to create assistant provider websocket",
		ErrorMessage:   "Unable to create assistant provider websocket, please try again later.",
	}
	CreateAssistantAttachProviderWebsocket = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantAttachProviderWebsocketCode,
		Error:          "unable to attach assistant provider websocket",
		ErrorMessage:   "Unable to attach assistant provider websocket, please try again later.",
	}
	CreateAssistantInvalidKnowledgeID = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidKnowledgeIDCode,
		Error:          "invalid knowledge_id parameter",
		ErrorMessage:   "Please provide a valid knowledgeId parameter.",
	}
	CreateAssistantInvalidRerankerModelProviderID = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidRerankerModelProviderIDCode,
		Error:          "invalid reranker_model_provider_id parameter",
		ErrorMessage:   "Please provide a valid rerankerModelProviderId parameter.",
	}
	CreateAssistantCreateTools = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateToolsCode,
		Error:          "unable to create assistant tools",
		ErrorMessage:   "Unable to create assistant tools, please try again later.",
	}
	CreateAssistantCreateKnowledge = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateKnowledgeCode,
		Error:          "unable to create assistant knowledge",
		ErrorMessage:   "Unable to create assistant knowledge, please try again later.",
	}
	CreateAssistantCreateTags = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantCreateTagsCode,
		Error:          "unable to create assistant tags",
		ErrorMessage:   "Unable to create assistant tags, please try again.",
	}
	CreateAssistantInvalidAgentKitCertificate = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitCertificateCode,
		Error:          "invalid agentkit certificate parameter",
		ErrorMessage:   "certificate must be a CA PEM. Use transportSecurity and tlsVerification for transport options.",
	}
	CreateAssistantInvalidAgentKitTransport = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitTransportCode,
		Error:          "invalid agentkit transport_security parameter",
		ErrorMessage:   "transportSecurity must be TLS or PLAINTEXT.",
	}
	CreateAssistantInvalidAgentKitTLSVerification = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitTLSVerificationCode,
		Error:          "invalid agentkit tls_verification parameter",
		ErrorMessage:   "tlsVerification must be VERIFY or SKIP_VERIFY.",
	}
	CreateAssistantInvalidAgentKitConnectTimeout = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitConnectTimeoutCode,
		Error:          "invalid agentkit connect_timeout_ms parameter",
		ErrorMessage:   "connectTimeoutMs must be between 1 and 300000.",
	}
	CreateAssistantInvalidAgentKitKeepaliveTime = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitKeepaliveTimeCode,
		Error:          "invalid agentkit keepalive_time_ms parameter",
		ErrorMessage:   "keepaliveTimeMs must be between 10000 and 3600000.",
	}
	CreateAssistantInvalidAgentKitKeepaliveTimeout = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitKeepaliveTimeoutCode,
		Error:          "invalid agentkit keepalive_timeout_ms parameter",
		ErrorMessage:   "keepaliveTimeoutMs must be between 1000 and 300000.",
	}
	CreateAssistantInvalidAgentKitMaxRecvMessageBytes = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitMaxRecvMessageBytesCode,
		Error:          "invalid agentkit max_recv_message_bytes parameter",
		ErrorMessage:   "maxRecvMessageBytes must be between 1024 and 104857600.",
	}
	CreateAssistantInvalidAgentKitMaxSendMessageBytes = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantInvalidAgentKitMaxSendMessageBytesCode,
		Error:          "invalid agentkit max_send_message_bytes parameter",
		ErrorMessage:   "maxSendMessageBytes must be between 1024 and 104857600.",
	}
)
