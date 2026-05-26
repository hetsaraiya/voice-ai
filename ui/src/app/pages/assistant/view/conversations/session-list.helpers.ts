import { AssistantConversation } from '@rapidaai/react';
import { getMetadataValueOrDefault, getMetricValue } from '@/utils/metadata';

const DISCONNECT_REASON_KEY = 'disconnect_reason';
const UNKNOWN_DISCONNECT_REASON = 'unknown';
const CHANNEL_KEY = 'client.channel';
const UNKNOWN_CHANNEL = 'unknown';

export const UNKNOWN_DURATION_VALUE = 'unknown';

const DURATION_BREAKDOWN_METRICS = [
  {
    key: 'duration',
    sourceUnit: 'nanoseconds',
  },
  {
    key: 'telephony_duration',
    sourceUnit: 'seconds',
  },
  {
    key: 'tts_duration',
    sourceUnit: 'nanoseconds',
  },
  {
    key: 'stt_duration',
    sourceUnit: 'nanoseconds',
  },
] as const;

export type DurationBreakdownRow = {
  key: string;
  value: string;
};

const getSessionMetadataValue = (
  conversation: AssistantConversation,
  key: string,
  fallback: string,
): string => {
  const value = getMetadataValueOrDefault(
    conversation.getMetadataList(),
    key,
    fallback,
  );
  return value?.trim() ? value : fallback;
};

export const getDisconnectReasonValue = (
  conversation: AssistantConversation,
): string => {
  return getSessionMetadataValue(
    conversation,
    DISCONNECT_REASON_KEY,
    UNKNOWN_DISCONNECT_REASON,
  );
};

export const getChannelValue = (conversation: AssistantConversation): string =>
  getSessionMetadataValue(conversation, CHANNEL_KEY, UNKNOWN_CHANNEL);

const formatDurationMetricSeconds = (
  rawValue: string,
  sourceUnit: (typeof DURATION_BREAKDOWN_METRICS)[number]['sourceUnit'],
): string => {
  if (!rawValue?.trim()) return UNKNOWN_DURATION_VALUE;

  const numericValue = Number(rawValue);
  if (!Number.isFinite(numericValue)) return UNKNOWN_DURATION_VALUE;

  const seconds =
    sourceUnit === 'nanoseconds' ? numericValue / 1_000_000_000 : numericValue;
  return seconds.toFixed(2);
};

export const getDurationBreakdownRows = (
  conversation: AssistantConversation,
): DurationBreakdownRow[] => {
  const metrics = conversation.getMetricsList();
  return DURATION_BREAKDOWN_METRICS.map(metric => ({
    key: metric.key,
    value: formatDurationMetricSeconds(
      getMetricValue(metrics, metric.key),
      metric.sourceUnit,
    ),
  }));
};
