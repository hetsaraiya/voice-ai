// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.

package channel_webrtc

import (
	"fmt"
	"time"

	internal_output "github.com/rapidaai/api/assistant-api/internal/channel/output"
	webrtc_internal "github.com/rapidaai/api/assistant-api/internal/channel/webrtc/internal"
	"github.com/rapidaai/api/assistant-api/internal/observability"
)

func (s *webrtcStreamer) runOutputHealthReporter() {
	ticker := time.NewTicker(webrtc_internal.OutputHealthReportInterval)
	defer ticker.Stop()

	var prev internal_output.HealthSnapshot
	var previousReceiverReports uint64
	for {
		select {
		case <-s.Ctx.Done():
			return
		case <-ticker.C:
		}

		if s.outputHealth == nil {
			continue
		}
		snap := s.outputHealth.Snapshot()

		healthReportedAt := time.Now()
		outputAudioFrames := 0
		outputAudioOldestAgeMs := int64(0)
		s.outputAudioQueueMu.Lock()
		outputAudioFrames = len(s.outputAudioQueue)
		if outputAudioFrames > 0 {
			outputAudioOldestAgeMs = healthReportedAt.Sub(s.outputAudioQueue[0].QueuedAt).Milliseconds()
			if outputAudioOldestAgeMs < 0 {
				outputAudioOldestAgeMs = 0
			}
		}
		s.outputAudioQueueMu.Unlock()

		s.Mu.Lock()
		mediaHealthState := s.mediaHealthState
		peerConnection := s.peerConnection
		s.Mu.Unlock()
		if s.sessionState.PeerConnected() {
			s.reportSelectedICECandidatePair(peerConnection, healthReportedAt)
		}
		lastAssistantFrameSentMsAgo := int64(0)
		if !mediaHealthState.LastAssistantFrameSentAt.IsZero() {
			lastAssistantFrameSentMsAgo = healthReportedAt.Sub(mediaHealthState.LastAssistantFrameSentAt).Milliseconds()
			if lastAssistantFrameSentMsAgo < 0 {
				lastAssistantFrameSentMsAgo = 0
			}
		}

		if snap.Ticks != prev.Ticks {
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelInfo,
				Message: "WebRTC output pacer health snapshot recorded for transport diagnostics; high late ticks, send errors, or queued audio can explain assistant audio delay or dropouts.",
				Attributes: observability.Attributes{
					"component":                                     observability.ComponentWebRTC.String(),
					webrtc_internal.DataType:                        webrtc_internal.EventOutputPacerHealth,
					webrtc_internal.DataSessionID:                   s.sessionID,
					webrtc_internal.DataTicks:                       fmt.Sprintf("%d", snap.Ticks),
					webrtc_internal.DataLateTicks:                   fmt.Sprintf("%d", snap.LateTicks),
					webrtc_internal.DataActiveTicks:                 fmt.Sprintf("%d", snap.ActiveTicks),
					webrtc_internal.DataIdleTicks:                   fmt.Sprintf("%d", snap.IdleTicks),
					webrtc_internal.DataSendErrors:                  fmt.Sprintf("%d", snap.SendErrors),
					webrtc_internal.DataIdleRatio:                   fmt.Sprintf("%.4f", snap.IdleRatio),
					webrtc_internal.DataPendingAudioFrames:          fmt.Sprintf("%d", outputAudioFrames),
					webrtc_internal.DataPendingAudioOldestAgeMs:     fmt.Sprintf("%d", outputAudioOldestAgeMs),
					webrtc_internal.DataLastAssistantFrameSentMsAgo: fmt.Sprintf("%d", lastAssistantFrameSentMsAgo),
					webrtc_internal.DataTotalDroppedFrames:          fmt.Sprintf("%d", s.sessionState.OutputAudioDroppedFrames()),
				},
			})

			if snap.SendErrors > prev.SendErrors {
				_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
					Level:   observability.LevelError,
					Message: "WebRTC output send error",
					Attributes: observability.Attributes{
						"component":                         observability.ComponentWebRTC.String(),
						webrtc_internal.DataType:            webrtc_internal.EventOutputSendError,
						webrtc_internal.DataSessionID:       s.sessionID,
						webrtc_internal.DataSendErrorsDelta: fmt.Sprintf("%d", snap.SendErrors-prev.SendErrors),
						webrtc_internal.DataTotalSendErrors: fmt.Sprintf("%d", snap.SendErrors),
						webrtc_internal.DataTicks:           fmt.Sprintf("%d", snap.Ticks),
						webrtc_internal.DataLateTicks:       fmt.Sprintf("%d", snap.LateTicks),
						webrtc_internal.DataActiveTicks:     fmt.Sprintf("%d", snap.ActiveTicks),
						webrtc_internal.DataIdleTicks:       fmt.Sprintf("%d", snap.IdleTicks),
						webrtc_internal.DataIdleRatio:       fmt.Sprintf("%.4f", snap.IdleRatio),
					},
				})
			}
			prev = snap
		}

		if mediaHealthState.ReceiverReports > previousReceiverReports && !mediaHealthState.LastReceiverReportAt.IsZero() {
			lastFeedbackMsAgo := healthReportedAt.Sub(mediaHealthState.LastReceiverReportAt).Milliseconds()
			if lastFeedbackMsAgo < 0 {
				lastFeedbackMsAgo = 0
			}
			qualityData := map[string]string{
				webrtc_internal.DataType:                 webrtc_internal.EventPeerQuality,
				webrtc_internal.DataSessionID:            s.sessionID,
				webrtc_internal.DataReceiverReports:      fmt.Sprintf("%d", mediaHealthState.ReceiverReports),
				webrtc_internal.DataPacketLossFraction:   fmt.Sprintf("%d", mediaHealthState.LastReceiverReportFractionLost),
				webrtc_internal.DataPacketLossPercent:    fmt.Sprintf("%.4f", mediaHealthState.LastReceiverReportPacketLossPercent),
				webrtc_internal.DataPacketLossTotal:      fmt.Sprintf("%d", mediaHealthState.LastReceiverReportTotalLost),
				webrtc_internal.DataJitterMs:             fmt.Sprintf("%.4f", mediaHealthState.LastReceiverReportJitterMs),
				webrtc_internal.DataLastFeedbackMsAgo:    fmt.Sprintf("%d", lastFeedbackMsAgo),
				webrtc_internal.DataLastFeedbackUnixMs:   fmt.Sprintf("%d", mediaHealthState.LastReceiverReportAt.UnixMilli()),
				webrtc_internal.DataRoundTripTimePresent: fmt.Sprintf("%t", mediaHealthState.LastReceiverReportRoundTripTimeUsable),
				webrtc_internal.DataQualityState:         mediaHealthState.QualityState(healthReportedAt),
			}
			if mediaHealthState.LastReceiverReportRoundTripTimeUsable {
				qualityData[webrtc_internal.DataRoundTripTimeMs] = fmt.Sprintf("%d", mediaHealthState.LastReceiverReportRoundTripTimeMs)
			}
			qualityData["component"] = observability.ComponentWebRTC.String()
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:      observability.LevelInfo,
				Message:    "WebRTC peer quality snapshot recorded from RTCP receiver feedback; packet loss, jitter, and RTT help explain degraded audio quality.",
				Attributes: observability.Attributes(qualityData),
			})
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordEvent{
				Component:  observability.ComponentWebRTC,
				Event:      observability.WebRTCPeerQuality,
				Attributes: observability.Attributes(qualityData),
			})
			previousReceiverReports = mediaHealthState.ReceiverReports
		}
	}
}

func (s *webrtcStreamer) runHealthWatchdog() {
	ticker := time.NewTicker(webrtc_internal.HealthWatchdogInterval)
	defer ticker.Stop()

	var lastConnectedNoUserAudioEventAt time.Time
	var lastAssistantAudioQueuedNotSentEventAt time.Time
	var lastRTCPFeedbackMissingEventAt time.Time
	var lastRepeatedWriteFailuresEventAt time.Time

	for {
		select {
		case <-s.Ctx.Done():
			return
		case <-ticker.C:
		}

		if !s.sessionState.PeerConnected() {
			continue
		}

		watchdogCheckedAt := time.Now()
		outputAudioFrames := 0
		outputAudioOldestQueuedAt := time.Time{}
		s.outputAudioQueueMu.Lock()
		outputAudioFrames = len(s.outputAudioQueue)
		if outputAudioFrames > 0 {
			outputAudioOldestQueuedAt = s.outputAudioQueue[0].QueuedAt
		}
		s.outputAudioQueueMu.Unlock()

		s.Mu.Lock()
		mediaHealthState := s.mediaHealthState
		s.Mu.Unlock()
		qualityState := mediaHealthState.QualityState(watchdogCheckedAt)

		missingRemoteAudioTrackMediaSessionID, shouldRestartMissingRemoteAudioTrack := s.shouldRestartConnectedNoRemoteAudioTrack(mediaHealthState, watchdogCheckedAt)
		if shouldRestartMissingRemoteAudioTrack &&
			(lastConnectedNoUserAudioEventAt.IsZero() || watchdogCheckedAt.Sub(lastConnectedNoUserAudioEventAt) >= webrtc_internal.HealthWatchdogEventCooldown) {
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "WebRTC connected without user audio",
				Attributes: observability.Attributes{
					"component":                            observability.ComponentWebRTC.String(),
					webrtc_internal.DataType:               webrtc_internal.EventConnectedNoUserAudio,
					webrtc_internal.DataSessionID:          s.sessionID,
					webrtc_internal.DataConnectedMs:        fmt.Sprintf("%d", watchdogCheckedAt.Sub(mediaHealthState.PeerConnectedAt).Milliseconds()),
					webrtc_internal.DataThresholdMs:        fmt.Sprintf("%d", webrtc_internal.ConnectedNoUserAudioThreshold.Milliseconds()),
					webrtc_internal.DataReadErrors:         fmt.Sprintf("%d", mediaHealthState.UserAudioReadErrors),
					webrtc_internal.DataRTPEmptyPayloads:   fmt.Sprintf("%d", mediaHealthState.UserAudioEmptyRTPPayloads),
					webrtc_internal.DataRTPParseFailures:   fmt.Sprintf("%d", mediaHealthState.UserAudioRTPUnmarshalFailures),
					webrtc_internal.DataOpusDecodeFailures: fmt.Sprintf("%d", mediaHealthState.UserAudioOpusDecodeFailures),
					webrtc_internal.DataQualityState:       qualityState,
				},
			})
			lastConnectedNoUserAudioEventAt = watchdogCheckedAt
			s.queueMediaSessionRestart(missingRemoteAudioTrackMediaSessionID, webrtc_internal.ReasonConnectedNoUserAudio, watchdogCheckedAt)
		}

		if outputAudioFrames > 0 &&
			!outputAudioOldestQueuedAt.IsZero() &&
			(mediaHealthState.LastAssistantFrameSentAt.IsZero() || mediaHealthState.LastAssistantFrameSentAt.Before(outputAudioOldestQueuedAt)) &&
			watchdogCheckedAt.Sub(outputAudioOldestQueuedAt) >= webrtc_internal.AssistantQueuedNoSendThreshold &&
			(lastAssistantAudioQueuedNotSentEventAt.IsZero() || watchdogCheckedAt.Sub(lastAssistantAudioQueuedNotSentEventAt) >= webrtc_internal.HealthWatchdogEventCooldown) {
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "WebRTC assistant audio queued but not sent",
				Attributes: observability.Attributes{
					"component":                                 observability.ComponentWebRTC.String(),
					webrtc_internal.DataType:                    webrtc_internal.EventAssistantAudioQueuedNotSent,
					webrtc_internal.DataSessionID:               s.sessionID,
					webrtc_internal.DataPendingAudioFrames:      fmt.Sprintf("%d", outputAudioFrames),
					webrtc_internal.DataPendingAudioOldestAgeMs: fmt.Sprintf("%d", watchdogCheckedAt.Sub(outputAudioOldestQueuedAt).Milliseconds()),
					webrtc_internal.DataThresholdMs:             fmt.Sprintf("%d", webrtc_internal.AssistantQueuedNoSendThreshold.Milliseconds()),
					webrtc_internal.DataQualityState:            qualityState,
				},
			})
			lastAssistantAudioQueuedNotSentEventAt = watchdogCheckedAt
		}

		if !mediaHealthState.LastAssistantFrameSentAt.IsZero() &&
			(mediaHealthState.LastReceiverReportAt.IsZero() || watchdogCheckedAt.Sub(mediaHealthState.LastReceiverReportAt) >= webrtc_internal.RTCPFeedbackMissingThreshold) &&
			watchdogCheckedAt.Sub(mediaHealthState.LastAssistantFrameSentAt) >= webrtc_internal.RTCPFeedbackMissingThreshold &&
			(lastRTCPFeedbackMissingEventAt.IsZero() || watchdogCheckedAt.Sub(lastRTCPFeedbackMissingEventAt) >= webrtc_internal.HealthWatchdogEventCooldown) {
			lastFeedbackAvailable := false
			lastFeedbackMsAgo := int64(0)
			if !mediaHealthState.LastReceiverReportAt.IsZero() {
				lastFeedbackAvailable = true
				lastFeedbackMsAgo = watchdogCheckedAt.Sub(mediaHealthState.LastReceiverReportAt).Milliseconds()
			}
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "WebRTC RTCP feedback missing",
				Attributes: observability.Attributes{
					"component":                                     observability.ComponentWebRTC.String(),
					webrtc_internal.DataType:                        webrtc_internal.EventRTCPFeedbackMissing,
					webrtc_internal.DataSessionID:                   s.sessionID,
					webrtc_internal.DataLastAssistantFrameSentMsAgo: fmt.Sprintf("%d", watchdogCheckedAt.Sub(mediaHealthState.LastAssistantFrameSentAt).Milliseconds()),
					webrtc_internal.DataLastFeedbackAvailable:       fmt.Sprintf("%t", lastFeedbackAvailable),
					webrtc_internal.DataLastFeedbackMsAgo:           fmt.Sprintf("%d", lastFeedbackMsAgo),
					webrtc_internal.DataThresholdMs:                 fmt.Sprintf("%d", webrtc_internal.RTCPFeedbackMissingThreshold.Milliseconds()),
					webrtc_internal.DataReceiverReports:             fmt.Sprintf("%d", mediaHealthState.ReceiverReports),
					webrtc_internal.DataQualityState:                qualityState,
				},
			})
			lastRTCPFeedbackMissingEventAt = watchdogCheckedAt
		}

		if mediaHealthState.ConsecutiveAssistantFrameWriteFailures >= webrtc_internal.RepeatedWriteFailuresThreshold &&
			!mediaHealthState.LastAssistantFrameWriteFailureAt.IsZero() &&
			(lastRepeatedWriteFailuresEventAt.IsZero() || watchdogCheckedAt.Sub(lastRepeatedWriteFailuresEventAt) >= webrtc_internal.HealthWatchdogEventCooldown) {
			_ = s.observer.Record(s.Ctx, s.sessionState.Scope, observability.RecordLog{
				Level:   observability.LevelError,
				Message: "WebRTC repeated assistant frame write failures",
				Attributes: observability.Attributes{
					"component":                                  observability.ComponentWebRTC.String(),
					webrtc_internal.DataType:                     webrtc_internal.EventRepeatedWriteFailures,
					webrtc_internal.DataSessionID:                s.sessionID,
					webrtc_internal.DataConsecutiveWriteFailures: fmt.Sprintf("%d", mediaHealthState.ConsecutiveAssistantFrameWriteFailures),
					webrtc_internal.DataTotalWriteFailures:       fmt.Sprintf("%d", mediaHealthState.AssistantFrameWriteFailures),
					webrtc_internal.DataLastFailureMsAgo:         fmt.Sprintf("%d", watchdogCheckedAt.Sub(mediaHealthState.LastAssistantFrameWriteFailureAt).Milliseconds()),
					webrtc_internal.DataFailureThreshold:         fmt.Sprintf("%d", webrtc_internal.RepeatedWriteFailuresThreshold),
					webrtc_internal.DataQualityState:             qualityState,
				},
			})
			lastRepeatedWriteFailuresEventAt = watchdogCheckedAt
		}
	}
}

func (s *webrtcStreamer) shouldRestartConnectedNoRemoteAudioTrack(mediaHealthState webrtc_internal.MediaHealthState, watchdogCheckedAt time.Time) (uint64, bool) {
	if mediaHealthState.PeerConnectedAt.IsZero() || watchdogCheckedAt.Sub(mediaHealthState.PeerConnectedAt) < webrtc_internal.ConnectedNoUserAudioThreshold {
		return 0, false
	}
	activeMediaSessionID := s.sessionState.ActiveMediaSessionID()
	if activeMediaSessionID == 0 || s.sessionState.RemoteAudioReaderMediaSessionID() == activeMediaSessionID {
		return 0, false
	}
	return activeMediaSessionID, true
}
