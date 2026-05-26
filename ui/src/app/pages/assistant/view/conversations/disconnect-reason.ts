export type DisconnectReasonDisplay = {
  label: string;
  tooltip: string;
};

const UNKNOWN_REASON: DisconnectReasonDisplay = {
  label: 'unknown',
  tooltip: 'No disconnect reason metadata was recorded for this session.',
};

const DISCONNECT_REASON_BY_TYPE: Record<string, DisconnectReasonDisplay> = {
  tool: {
    label: 'Tool ended',
    tooltip:
      'A configured tool or assistant flow intentionally ended the session.',
  },
  user: {
    label: 'User disconnected',
    tooltip: 'The user disconnected, hung up, or cut the call.',
  },
  idle_timeout: {
    label: 'Idle timeout',
    tooltip:
      'The session ended because the user stopped responding for the configured idle timeout.',
  },
  max_duration: {
    label: 'Max duration',
    tooltip: 'The session reached the configured maximum session duration.',
  },
  error: {
    label: 'Error',
    tooltip:
      'The session ended because an error occurred during the conversation.',
  },
};

const DISCONNECT_REASON_ALIASES: Record<
  string,
  keyof typeof DISCONNECT_REASON_BY_TYPE
> = {
  '1': 'tool',
  '2': 'user',
  '3': 'idle_timeout',
  '4': 'max_duration',
  '5': 'error',
  TOOL: 'tool',
  USER: 'user',
  IDLE_TIMEOUT: 'idle_timeout',
  MAX_DURATION: 'max_duration',
  ERROR: 'error',
  DISCONNECTION_TYPE_TOOL: 'tool',
  DISCONNECTION_TYPE_USER: 'user',
  DISCONNECTION_TYPE_IDLE_TIMEOUT: 'idle_timeout',
  DISCONNECTION_TYPE_MAX_DURATION: 'max_duration',
  DISCONNECTION_TYPE_ERROR: 'error',
};

const extractReasonToken = (reason: string): string => {
  const trimmed = reason.trim();
  const enumToken = trimmed.match(/DISCONNECTION_TYPE_[A-Z_]+/)?.[0];
  if (enumToken) return enumToken;
  return trimmed.toUpperCase().replace(/[\s-]+/g, '_');
};

const humanizeReason = (reason: string): string =>
  reason
    .replace(/^DISCONNECTION_TYPE_/, '')
    .replace(/^CONVERSATIONDISCONNECTION_/, '')
    .replace(/_/g, ' ')
    .toLowerCase()
    .replace(/\b\w/g, char => char.toUpperCase());

export const normalizeDisconnectReason = (
  reason?: string | null,
): DisconnectReasonDisplay => {
  if (!reason?.trim()) return UNKNOWN_REASON;

  const token = extractReasonToken(reason);
  const reasonType = DISCONNECT_REASON_ALIASES[token];
  if (reasonType) return DISCONNECT_REASON_BY_TYPE[reasonType];

  return {
    label: humanizeReason(token),
    tooltip: 'The backend reported this disconnect reason for the session.',
  };
};
