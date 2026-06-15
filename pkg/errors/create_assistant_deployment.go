// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package errors

import "net/http"

const (
	CreateAssistantDebuggerDeploymentInvalidRequestCode         ErrorCode = 1004001
	CreateAssistantDebuggerDeploymentUnauthenticatedCode        ErrorCode = 1004002
	CreateAssistantDebuggerDeploymentMissingAuthScopeCode       ErrorCode = 1004003
	CreateAssistantDebuggerDeploymentInvalidAssistantIDCode     ErrorCode = 1004004
	CreateAssistantDebuggerDeploymentCreateDeploymentCode       ErrorCode = 1004005
	CreateAssistantDebuggerDeploymentInvalidAudioProviderCode   ErrorCode = 1004006
	CreateAssistantDebuggerDeploymentInvalidIdealTimeoutCode    ErrorCode = 1004007
	CreateAssistantDebuggerDeploymentInvalidTimeoutBackoffCode  ErrorCode = 1004008
	CreateAssistantDebuggerDeploymentInvalidSessionDurationCode ErrorCode = 1004009
)

var (
	CreateAssistantDebuggerDeploymentInvalidRequest = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidRequestCode,
		Error:          "invalid request",
		ErrorMessage:   "Invalid request.",
	}
	CreateAssistantDebuggerDeploymentUnauthenticated = PlatformError{
		HTTPStatusCode: http.StatusUnauthorized,
		Code:           CreateAssistantDebuggerDeploymentUnauthenticatedCode,
		Error:          "unauthenticated request",
		ErrorMessage:   "Unauthenticated request, please try again with valid authentication.",
	}
	CreateAssistantDebuggerDeploymentMissingAuthScope = PlatformError{
		HTTPStatusCode: http.StatusForbidden,
		Code:           CreateAssistantDebuggerDeploymentMissingAuthScopeCode,
		Error:          "missing authentication scope",
		ErrorMessage:   "Unauthenticated request, please try again with valid authentication.",
	}
	CreateAssistantDebuggerDeploymentInvalidAssistantID = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidAssistantIDCode,
		Error:          "invalid assistant_id parameter",
		ErrorMessage:   "Please provide a valid assistantId parameter.",
	}
	CreateAssistantDebuggerDeploymentCreateDeployment = PlatformError{
		HTTPStatusCode: http.StatusInternalServerError,
		Code:           CreateAssistantDebuggerDeploymentCreateDeploymentCode,
		Error:          "unable to create assistant debugger deployment",
		ErrorMessage:   "Unable to create assistant debugger deployment, please try again later.",
	}
	CreateAssistantDebuggerDeploymentInvalidAudioProvider = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidAudioProviderCode,
		Error:          "invalid audio provider parameter",
		ErrorMessage:   "Please provide a valid audioProvider parameter.",
	}
	CreateAssistantDebuggerDeploymentInvalidIdealTimeout = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidIdealTimeoutCode,
		Error:          "invalid ideal_timeout parameter",
		ErrorMessage:   "Please provide idealTimeout between 15 and 120 seconds.",
	}
	CreateAssistantDebuggerDeploymentInvalidTimeoutBackoff = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidTimeoutBackoffCode,
		Error:          "invalid ideal_timeout_backoff parameter",
		ErrorMessage:   "Please provide idealTimeoutBackoff between 0 and 5 times.",
	}
	CreateAssistantDebuggerDeploymentInvalidSessionDuration = PlatformError{
		HTTPStatusCode: http.StatusBadRequest,
		Code:           CreateAssistantDebuggerDeploymentInvalidSessionDurationCode,
		Error:          "invalid max_session_duration parameter",
		ErrorMessage:   "Please provide maxSessionDuration between 180 and 600 seconds.",
	}
)
