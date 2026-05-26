// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package internal_exotel

import (
	"fmt"
	"strings"
	"time"

	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
	"github.com/rapidaai/pkg/utils"
	"github.com/rapidaai/pkg/validator"
)

type ExotelMediaEvent struct {
	Event     string       `json:"event"`
	StreamSid string       `json:"stream_sid"`
	Media     *ExotelMedia `json:"media,omitempty"`
}

type ExotelMedia struct {
	Payload string `json:"payload"`
}

type MakeCallResponse struct {
	Call struct {
		Sid              string  `json:"Sid"`
		Status           string  `json:"Status"`
		RecordingUrl     string  `json:"RecordingUrl"`
		ConversationUuid *string `json:"ParentCallSid"` // Use pointers for nullable fields
	} `json:"Call"`
}

type StatusCallback struct {
	Event        string
	ChannelUUID  string
	Duration     *time.Duration
	Price        string
	Cause        string
	ErrorCode    string
	ErrorMessage string
	Payload      utils.Option
}

func NewStatusCallback(eventDetails utils.Option) (*StatusCallback, error) {
	event, _ := eventDetails.GetString("Status")
	if !validator.NotBlank(event) {
		return nil, fmt.Errorf("status not found in payload")
	}

	channelUUID, _ := eventDetails.GetString("CallSid")
	duration, err := eventDetails.GetDuration("ConversationDuration")
	if err != nil {
		duration, err = eventDetails.GetDuration("Duration")
	}
	if err != nil {
		duration, err = eventDetails.GetDuration("CallDuration")
	}
	var durationPtr *time.Duration
	if err == nil {
		durationPtr = utils.Ptr(duration)
	}

	price, _ := eventDetails.GetString("Price")
	cause, _ := eventDetails.GetString("Cause")
	errorMessage, _ := eventDetails.GetString("ErrorMessage")
	errorCode, _ := eventDetails.GetString("ErrorCode")

	return &StatusCallback{
		Event:        event,
		ChannelUUID:  channelUUID,
		Duration:     durationPtr,
		Price:        price,
		Cause:        cause,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		Payload:      eventDetails,
	}, nil
}

func (s *StatusCallback) StatusInfo() *internal_type.StatusInfo {
	statusInfo := &internal_type.StatusInfo{
		Event:       s.Event,
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
	statusLower := strings.ToLower(s.Event)
	return statusLower == "failed" ||
		statusLower == "busy" ||
		statusLower == "no-answer" ||
		statusLower == "no_answer" ||
		statusLower == "canceled" ||
		statusLower == "cancelled" ||
		validator.NotBlank(s.Cause) ||
		validator.NotBlank(s.ErrorMessage) ||
		validator.NotBlank(s.ErrorCode)
}

func (s *StatusCallback) FailureReason() string {
	if validator.NotBlank(s.Cause) {
		return s.Cause
	}
	if validator.NotBlank(s.ErrorMessage) {
		return s.ErrorMessage
	}
	if validator.NotBlank(s.ErrorCode) {
		return s.ErrorCode
	}
	return s.Event
}
