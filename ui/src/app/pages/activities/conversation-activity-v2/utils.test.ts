import { telemetryRecordToTimelineDocument } from './utils';

const mapFromEntries = (entries: Array<[string, string]> = []) => ({
  toArray: () => entries,
});

const timestamp = (value = '2026-06-04T03:10:00.000Z') => ({
  toDate: () => new Date(value),
});

const metricRecord = ({ name, value }: { name: string; value: string }) =>
  ({
    getAttributesMap: () => mapFromEntries([['component', 'stt']]),
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
      observabilityRecord(metricRecord({ name: 'input_tokens', value: '200' })),
      0,
    );

    expect(document?.durationMs).toBeUndefined();
  });
});
