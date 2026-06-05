package adapter_internal

// func requestorTelemetryTestLogger(t *testing.T) commons.Logger {
// 	t.Helper()
// 	logger, err := commons.NewApplicationLogger(
// 		commons.Name("requestor-telemetry-test"),
// 		commons.Level("error"),
// 		commons.EnableFile(false),
// 	)
// 	require.NoError(t, err)
// 	return logger
// }

// func requestorForTelemetryTest(t *testing.T, providers []*internal_telemetry_entity.AssistantTelemetryProvider) *genericRequestor {
// 	t.Helper()
// 	projectID := uint64(11)
// 	orgID := uint64(22)

// 	return &genericRequestor{
// 		logger:           requestorTelemetryTestLogger(t),
// 		config:           &assistant_config.AssistantConfig{},
// 		messageLifecycle: adapter_lifecycle.NewMessageLifecycle(),
// 		sessionLifecycle: adapter_lifecycle.NewSessionLifecycle(),
// 		auth: &types.ServiceScope{
// 			ProjectId:      &projectID,
// 			OrganizationId: &orgID,
// 		},
// 		assistant: &internal_assistant_entity.Assistant{
// 			Audited:                     gorm_model.Audited{Id: 101},
// 			AssistantTelemetryProviders: providers,
// 		},
// 		assistantConversation: &internal_conversation_entity.AssistantConversation{
// 			Audited: gorm_model.Audited{Id: 202},
// 		},
// 	}
// }

// func TestInitializeCollectors_NoProvidersConfigured_UsesNoopCollectors(t *testing.T) {
// 	r := requestorForTelemetryTest(t, nil)

// 	r.initializeCollectors(context.Background())

// 	assert.NotNil(t, r.observer)
// 	evtCollector := r.observer.EventCollectors()
// 	metCollector := r.observer.MetricCollectors()
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", evtCollector), "noopEventCollector"))
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", metCollector), "noopMetricCollector"))
// 	assert.NotPanics(t, func() {
// 		evtCollector.Collect(context.Background(), sessionEventRecord("connected"))
// 		metCollector.Collect(context.Background(), conversationMetricRecord("202"))
// 		r.observer.Shutdown(context.Background())
// 	})
// }

// func TestInitializeCollectors_LoggingProvider_UsesFanoutCollectors(t *testing.T) {
// 	r := requestorForTelemetryTest(t, []*internal_telemetry_entity.AssistantTelemetryProvider{
// 		{
// 			ProviderType: "logging",
// 			Enabled:      true,
// 		},
// 	})
// 	r.initializeCollectors(context.Background())
// 	assert.NotNil(t, r.observer)
// 	evtCollector := r.observer.EventCollectors()
// 	metCollector := r.observer.MetricCollectors()
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", evtCollector), "fanoutEventCollector"))
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", metCollector), "fanoutMetricCollector"))
// 	assert.NotPanics(t, func() {
// 		evtCollector.Collect(context.Background(), sessionEventRecord("connected"))
// 		metCollector.Collect(context.Background(), conversationMetricRecord("202"))
// 		r.observer.Shutdown(context.Background())
// 	})
// }

// func TestInitializeCollectors_OTLPMissingEndpoint_SkipsToNoopCollectors(t *testing.T) {
// 	r := requestorForTelemetryTest(t, []*internal_telemetry_entity.AssistantTelemetryProvider{
// 		{
// 			ProviderType: "otlp_http",
// 			Enabled:      true,
// 		},
// 	})

// 	r.initializeCollectors(context.Background())

// 	assert.NotNil(t, r.observer)
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", r.observer.EventCollectors()), "noopEventCollector"))
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", r.observer.MetricCollectors()), "noopMetricCollector"))
// }

// func sessionEventRecord(eventType string) observe.EventRecord {
// 	return observe.EventRecord{Name: "session", Data: map[string]string{"type": eventType}}
// }

// func conversationMetricRecord(conversationID string) observe.ConversationMetricRecord {
// 	return observe.ConversationMetricRecord{
// 		ConversationID: conversationID,
// 		Metrics: []*protos.Metric{
// 			{
// 				Name:  type_enums.CONVERSATION_STATUS.String(),
// 				Value: type_enums.CONVERSATION_IN_PROGRESS.String(),
// 			},
// 		},
// 	}
// }

// func TestInitializeCollectors_UnknownProvider_SkipsToNoopCollectors(t *testing.T) {
// 	r := requestorForTelemetryTest(t, []*internal_telemetry_entity.AssistantTelemetryProvider{
// 		{
// 			ProviderType: "unknown_provider",
// 			Enabled:      true,
// 		},
// 	})
// 	r.initializeCollectors(context.Background())
// 	assert.NotNil(t, r.observer)
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", r.observer.EventCollectors()), "noopEventCollector"))
// 	assert.True(t, strings.Contains(fmt.Sprintf("%T", r.observer.MetricCollectors()), "noopMetricCollector"))
// }

// // TestInitializeTelemetry_ChannelHandoffIsRaceFree asserts the synchronization
// // pattern used in production: telemetry runs in a goroutine, then sends a
// // packet (channel send) that another goroutine receives before reading
// // r.observer. Channel send/receive establishes happens-before, so reads from
// // the receiver's goroutine observe the fully-published observer.
// //
// // Run with -race; must pass.
// func TestInitializeTelemetry_ChannelHandoffIsRaceFree(t *testing.T) {
// 	r := requestorForTelemetryTest(t, nil)
// 	handoff := make(chan struct{}, 1)

// 	go func() {
// 		r.initializeCollectors(context.Background())
// 		handoff <- struct{}{}
// 	}()

// 	<-handoff
// 	require.NotNil(t, r.observer)
// 	_ = r.observer.EventCollectors()
// }
