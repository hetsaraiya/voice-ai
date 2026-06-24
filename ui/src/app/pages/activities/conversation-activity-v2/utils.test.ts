import {
  COMPONENT_OPTIONS,
  EVENTS_BY_COMPONENT,
  LEVEL_OPTIONS,
  ROLE_OPTIONS,
  SCOPE_OPTIONS,
  getEventOptionsForComponent,
} from './constants';
import {
  getDocumentComponent,
  telemetryRecordToTimelineDocument,
} from './utils';

const mapFromEntries = (entries: Array<[string, string]> = []) => ({
  toArray: () => entries,
});

const timestamp = (value = '2026-06-04T03:10:00.000Z') => ({
  toDate: () => new Date(value),
});

const metricRecord = ({
  component = 'stt',
  name,
  value,
}: {
  component?: string;
  name: string;
  value: string;
}) =>
  ({
    getAttributesMap: () =>
      mapFromEntries(component ? [['component', component]] : []),
    getContextMap: () => mapFromEntries([['traceId', 'trace-1']]),
    getDescription: () => '',
    getId: () => 'metric-1',
    getName: () => name,
    getOccurredat: () => timestamp(),
    getOrganizationid: () => '1',
    getProjectid: () => '2',
    getScope: () => 'message',
    getScopeattributesMap: () =>
      mapFromEntries([
        ['assistantId', '10'],
        ['assistantConversationId', '20'],
        ['messageId', 'message-1'],
        ['messageRole', 'user'],
      ]),
    getValue: () => value,
  }) as any;

const observabilityRecord = (metric: any) =>
  ({
    getEvent: () => undefined,
    getLog: () => undefined,
    getMetric: () => metric,
  }) as any;

describe('conversation activity v2 telemetry utilities', () => {
  it('uses component latency metrics as waterfall durations', () => {
    const document = telemetryRecordToTimelineDocument(
      observabilityRecord(
        metricRecord({ name: 'stt_latency_ms', value: '47.4' }),
      ),
      0,
    );

    expect(document?.durationMs).toBe(47);
  });

  it('does not treat non-latency counters as durations', () => {
    const document = telemetryRecordToTimelineDocument(
      observabilityRecord(metricRecord({ name: 'input_token', value: '200' })),
      0,
    );

    expect(document?.durationMs).toBeUndefined();
  });

  it('maps backend metric names to canonical timeline components', () => {
    const document = telemetryRecordToTimelineDocument(
      observabilityRecord(
        metricRecord({
          component: '',
          name: 'webrtc.ice_latency_ms',
          value: '22',
        }),
      ),
      0,
    );

    expect(document?.category).toBe('webrtc');
    expect(document ? getDocumentComponent(document) : '').toBe('webrtc');
  });

  it('uses current backend log levels, scopes, and message roles', () => {
    expect(LEVEL_OPTIONS.map(option => option.id)).toEqual([
      'all',
      'debug',
      'info',
      'error',
      'critical',
    ]);
    expect(SCOPE_OPTIONS.map(option => option.id)).toEqual([
      'all',
      'project',
      'assistant',
      'conversation',
      'message',
    ]);
    expect(ROLE_OPTIONS.map(option => option.id)).toEqual([
      'all',
      'user',
      'assistant',
    ]);
  });

  it('uses current backend observability components and events', () => {
    const componentIds = COMPONENT_OPTIONS.map(option => option.id);
    expect(componentIds).toContain('sip');
    expect(componentIds).toContain('webrtc');
    expect(componentIds).not.toContain('session');
    expect(componentIds).not.toContain('audio');

    expect(EVENTS_BY_COMPONENT.call).toContain('call.provider_answered');
    expect(EVENTS_BY_COMPONENT.call).not.toContain('call.answered');
    expect(EVENTS_BY_COMPONENT.stt).toContain('stt.interim');
    expect(EVENTS_BY_COMPONENT.stt).not.toContain('stt.final');
    expect(EVENTS_BY_COMPONENT.tts).toContain('tts.discarded');
    expect(EVENTS_BY_COMPONENT.tts).not.toContain('tts.first_audio');
    expect(EVENTS_BY_COMPONENT.conversation).toContain(
      'conversation.authentication_started',
    );
    expect(EVENTS_BY_COMPONENT.conversation).toContain(
      'conversation.mode_switch_failed',
    );

    expect(
      getEventOptionsForComponent('webrtc').map(option => option.id),
    ).toEqual([
      'all',
      'webrtc.connecting',
      'webrtc.connected',
      'webrtc.reconnecting',
      'webrtc.disconnected',
      'webrtc.failed',
      'webrtc.ice_connection_state',
      'webrtc.ice_connected',
      'webrtc.ice_failed',
      'webrtc.audio_track_received',
      'webrtc.peer_quality',
      'webrtc.selected_ice_candidate_pair',
      'webrtc.negotiation_offer_sent',
      'webrtc.negotiation_answer_received',
      'webrtc.negotiation_retry_queued',
      'webrtc.negotiation_retry_sent',
      'webrtc.ice_restart_deferred',
    ]);
  });
});
