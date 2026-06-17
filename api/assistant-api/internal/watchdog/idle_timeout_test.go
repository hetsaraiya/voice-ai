// Copyright (c) 2023-2025 RapidaAI
// Author: Prashant Srivastav <prashant@rapida.ai>
//
// Licensed under GPL-2.0 with Rapida Additional Terms.
// See LICENSE.md or contact sales@rapida.ai for commercial usage.
package watchdog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rapidaai/api/assistant-api/internal/observability"
	internal_type "github.com/rapidaai/api/assistant-api/internal/type"
)

func TestIdleTimeoutWatchdog_StartExpiresWhenDeadlinePasses(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	require.True(t, idleTimeoutWatchdog.Start("ctx-idle", 25*time.Millisecond))

	select {
	case packet := <-pushedPackets:
		observabilityLogPacket, ok := packet.(internal_type.ObservabilityLogRecordPacket)
		require.True(t, ok)
		assert.Equal(t, "ctx-idle", observabilityLogPacket.ContextID)
		assert.Equal(t, "idle-timeout-watchdog: deadline expired", observabilityLogPacket.Record.Message)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("idle timeout watchdog did not expire")
	}

	select {
	case packet := <-pushedPackets:
		idleTimeoutExpiredPacket, ok := packet.(internal_type.IdleTimeoutExpiredPacket)
		require.True(t, ok)
		assert.Equal(t, "ctx-idle", idleTimeoutExpiredPacket.ContextID)
		assert.Equal(t, uint64(0), idleTimeoutExpiredPacket.Count)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("idle timeout watchdog did not push expired packet")
	}

	assert.False(t, idleTimeoutWatchdog.Stop(false))
}

func TestIdleTimeoutWatchdog_StartRejectsInvalidTimeout(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	require.False(t, idleTimeoutWatchdog.Start("ctx-invalid", 0))
	require.False(t, idleTimeoutWatchdog.Start("ctx-invalid", -time.Millisecond))

	select {
	case packet := <-pushedPackets:
		t.Fatalf("idle timeout watchdog pushed packet for invalid timeout: %+v", packet)
	case <-time.After(60 * time.Millisecond):
	}
}

func TestIdleTimeoutWatchdog_StopCancelsExpirationAndCanResetCount(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	assert.Equal(t, uint64(1), idleTimeoutWatchdog.IncrementCount())
	require.True(t, idleTimeoutWatchdog.Start("ctx-stop", 40*time.Millisecond))
	require.True(t, idleTimeoutWatchdog.Stop(true))
	assert.Equal(t, uint64(0), idleTimeoutWatchdog.Count())

	select {
	case packet := <-pushedPackets:
		t.Fatalf("idle timeout watchdog pushed packet after stop: %+v", packet)
	case <-time.After(90 * time.Millisecond):
	}
}

func TestIdleTimeoutWatchdog_StopCanKeepCount(t *testing.T) {
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog()

	assert.Equal(t, uint64(1), idleTimeoutWatchdog.IncrementCount())
	require.True(t, idleTimeoutWatchdog.Start("ctx-stop", 40*time.Millisecond))
	require.True(t, idleTimeoutWatchdog.Stop(false))

	assert.Equal(t, uint64(1), idleTimeoutWatchdog.Count())
}

func TestIdleTimeoutWatchdog_ExtendDelaysExpiration(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	require.True(t, idleTimeoutWatchdog.Start("ctx-extend", 35*time.Millisecond))
	time.Sleep(15 * time.Millisecond)
	require.True(t, idleTimeoutWatchdog.Extend("ctx-extend", 80*time.Millisecond))

	select {
	case packet := <-pushedPackets:
		t.Fatalf("idle timeout watchdog pushed packet before extended deadline: %+v", packet)
	case <-time.After(55 * time.Millisecond):
	}

	select {
	case packet := <-pushedPackets:
		observabilityLogPacket, ok := packet.(internal_type.ObservabilityLogRecordPacket)
		require.True(t, ok)
		assert.Equal(t, "ctx-extend", observabilityLogPacket.ContextID)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("idle timeout watchdog did not expire after extended deadline")
	}
}

func TestIdleTimeoutWatchdog_ExtendIgnoresInactiveAndDifferentContext(t *testing.T) {
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog()

	require.False(t, idleTimeoutWatchdog.Extend("ctx-inactive", time.Second))

	require.True(t, idleTimeoutWatchdog.Start("ctx-active", 40*time.Millisecond))
	require.False(t, idleTimeoutWatchdog.Extend("ctx-other", time.Second))
	require.True(t, idleTimeoutWatchdog.Stop(false))
}

func TestIdleTimeoutWatchdog_StartReplacesPreviousContext(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	require.True(t, idleTimeoutWatchdog.Start("ctx-old", 25*time.Millisecond))
	require.True(t, idleTimeoutWatchdog.Start("ctx-new", 120*time.Millisecond))
	defer idleTimeoutWatchdog.Cancel()

	select {
	case packet := <-pushedPackets:
		t.Fatalf("previous context pushed packet after replacement: %+v", packet)
	case <-time.After(70 * time.Millisecond):
	}

	require.True(t, idleTimeoutWatchdog.Stop(false))
}

func TestIdleTimeoutWatchdog_ExpirationKeepsCountForBehaviorBackoff(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 4)
	idleTimeoutWatchdog := NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))
	<-pushedPackets

	assert.Equal(t, uint64(1), idleTimeoutWatchdog.IncrementCount())
	require.True(t, idleTimeoutWatchdog.Start("ctx-count", 25*time.Millisecond))

	<-pushedPackets
	select {
	case packet := <-pushedPackets:
		idleTimeoutExpiredPacket, ok := packet.(internal_type.IdleTimeoutExpiredPacket)
		require.True(t, ok)
		assert.Equal(t, uint64(1), idleTimeoutExpiredPacket.Count)
	case <-time.After(250 * time.Millisecond):
		t.Fatal("idle timeout watchdog did not expire")
	}

	assert.Equal(t, uint64(1), idleTimeoutWatchdog.Count())
}

func TestIdleTimeoutWatchdog_ConstructorPushesInitializationInfo(t *testing.T) {
	pushedPackets := make(chan internal_type.Packet, 1)

	NewIdleTimeoutWatchdog(WithOnPacket(func(_ context.Context, packets ...internal_type.Packet) error {
		for _, packet := range packets {
			pushedPackets <- packet
		}
		return nil
	}))

	packet := <-pushedPackets
	observabilityLogPacket, ok := packet.(internal_type.ObservabilityLogRecordPacket)
	require.True(t, ok)
	assert.Equal(t, internal_type.ObservabilityRecordScopeConversation, observabilityLogPacket.Scope)
	assert.Equal(t, observability.LevelInfo, observabilityLogPacket.Record.Level)
	assert.Equal(t, "idle-timeout-watchdog: initialization completed", observabilityLogPacket.Record.Message)
	assert.Equal(t, "idle_timeout", observabilityLogPacket.Record.Attributes["watchdog"])
}
