export type TimelineDocument = {
  id: string;
  kind: string;
  name: string;
  category: string;
  level: string;
  outcome: string;
  title: string;
  projectId: number;
  organizationId: number;
  scope: 'assistant' | 'conversation' | 'message';
  assistantId: number;
  assistantConversationId: number;
  messageId?: string;
  messageRole?: string;
  contextId: string;
  occurredAt: string;
  receivedAt: string;
  durationMs?: number;
  attributes?: Record<string, string>;
  data?: Record<string, unknown>;
};

export type TimelineItem = TimelineDocument & {
  endMs: number;
  offsetPct: number;
  startMs: number;
  widthPct: number;
};

export type TimelineGroup = {
  contextId: string;
  durationMs: number;
  endMs: number;
  items: TimelineItem[];
  startMs: number;
  title: string;
};

export type TraceSummary = {
  assistantConversationId: number;
  assistantId: number;
  components: string[];
  contextId: string;
  durationMs: number;
  endMs: number;
  failureCount: number;
  level: string;
  outcome: string;
  spanCount: number;
  startMs: number;
  title: string;
};

export type MetricSummary = {
  averageDurationMs: number;
  component: string;
  count: number;
  failureCount: number;
  p95DurationMs: number;
  slowestContextId: string;
  slowestDurationMs: number;
};

export type SpanCountBucket = {
  endMs: number;
  failureCount: number;
  label: string;
  spanCount: number;
  startMs: number;
};

export type LatencyBucket = {
  endMs: number;
  eos: number;
  label: string;
  llm: number;
  startMs: number;
  stt: number;
  total: number;
  tts: number;
};
