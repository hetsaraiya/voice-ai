// Copyright (c) 2023-2025 RapidaAI
// Author: RapidaAI Team <team@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package internal_telnyx

import (
	"fmt"
	"strings"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
)

type AudioChunk struct {
	Data     []byte
	Duration time.Duration
}

type StatusCallback struct {
	EventType    string
	ChannelUUID  string
	Duration     *time.Duration
	Price        string
	Reason       string
	ErrorCode    string
	ErrorMessage string
	Payload      map[string]interface{}
}

func NewStatusCallback(payload map[string]interface{}) (*StatusCallback, error) {
	rawData, ok := payload["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("data field not found in payload")
	}
	data := utils.Option(rawData)

	eventType, _ := data.GetString("event_type")
	if !validator.NotBlank(eventType) {
		return nil, fmt.Errorf("event_type not found in payload")
	}

	payloadData := utils.Option{}
	if rawPayloadData, ok := data["payload"].(map[string]interface{}); ok {
		payloadData = utils.Option(rawPayloadData)
	}

	channelUUID, _ := payloadData.GetString("call_control_id")
	if !validator.NotBlank(channelUUID) {
		channelUUID, _ = payloadData.GetString("call_session_id")
	}
	if !validator.NotBlank(channelUUID) {
		channelUUID, _ = data.GetString("call_control_id")
	}
	if !validator.NotBlank(channelUUID) {
		channelUUID, _ = data.GetString("id")
	}

	duration, err := payloadData.GetDuration("duration")
	if err != nil {
		duration, err = payloadData.GetDuration("duration_secs")
	}
	if err != nil {
		duration, err = payloadData.GetDuration("call_duration")
	}
	var durationPtr *time.Duration
	if err == nil {
		durationPtr = utils.Ptr(duration)
	}

	price, _ := payloadData.GetString("price")
	if !validator.NotBlank(price) {
		price, _ = payloadData.GetString("cost")
	}

	reason, _ := payloadData.GetString("hangup_cause")
	if !validator.NotBlank(reason) {
		reason, _ = payloadData.GetString("cause")
	}
	if !validator.NotBlank(reason) {
		reason, _ = payloadData.GetString("sip_hangup_cause")
	}
	errorCode, _ := payloadData.GetString("error_code")
	errorMessage, _ := payloadData.GetString("error_message")

	return &StatusCallback{
		EventType:    eventType,
		ChannelUUID:  channelUUID,
		Duration:     durationPtr,
		Price:        price,
		Reason:       reason,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Payload:      payload,
	}, nil
}

func (s *StatusCallback) StatusInfo() *internal_type.StatusInfo {
	statusInfo := &internal_type.StatusInfo{
		Event:       s.EventType,
		ChannelUUID: s.ChannelUUID,
		Duration:    s.Duration,
		Price:       s.Price,
		Payload:     s.Payload,
	}
	if s.Failed() {
		statusInfo.Error = &internal_type.StatusError{Error: "failed", Reason: s.FailureReason()}
	}
	return statusInfo
}

func (s *StatusCallback) Failed() bool {
	eventLower := strings.ToLower(s.EventType)
	failed := eventLower == "call.failed" ||
		eventLower == "call.rejected" ||
		eventLower == "call.bridging.failed" ||
		validator.NotBlank(s.ErrorCode) ||
		validator.NotBlank(s.ErrorMessage)
	if eventLower == "call.hangup" {
		lowerReason := strings.ToLower(s.Reason)
		failed = failed ||
			lowerReason == "busy" ||
			lowerReason == "no_answer" ||
			lowerReason == "no-answer" ||
			lowerReason == "rejected" ||
			lowerReason == "failed" ||
			lowerReason == "timeout"
	}
	return failed
}

func (s *StatusCallback) FailureReason() string {
	if validator.NotBlank(s.Reason) {
		return s.Reason
	}
	if validator.NotBlank(s.ErrorMessage) {
		return s.ErrorMessage
	}
	if validator.NotBlank(s.ErrorCode) {
		return s.ErrorCode
	}
	return s.EventType
}
