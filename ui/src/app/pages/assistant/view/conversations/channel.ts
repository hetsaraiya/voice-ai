export type ChannelIconName =
  | 'api'
  | 'app'
  | 'chat'
  | 'debugger'
  | 'phone'
  | 'sdk'
  | 'unknown'
  | 'web';

export type ChannelDisplay = {
  label: string;
  tooltip: string;
  icon: ChannelIconName;
};

const UNKNOWN_CHANNEL_LABEL = 'unknown';

const CHANNEL_ALIASES: Record<
  string,
  Pick<ChannelDisplay, 'label' | 'icon'>
> = {
  api: { label: 'API', icon: 'api' },
  app: { label: 'App', icon: 'app' },
  call: { label: 'Phone', icon: 'phone' },
  chat: { label: 'Chat', icon: 'chat' },
  debug: { label: 'Debugger', icon: 'debugger' },
  debugger: { label: 'Debugger', icon: 'debugger' },
  asterisk: { label: 'Asterisk', icon: 'phone' },
  exotel: { label: 'Exotel', icon: 'phone' },
  'exotel-call': { label: 'Exotel', icon: 'phone' },
  'go-sdk': { label: 'SDK', icon: 'sdk' },
  'java-sdk': { label: 'SDK', icon: 'sdk' },
  krsip: { label: 'Krsip', icon: 'phone' },
  'mobile-app': { label: 'App', icon: 'app' },
  'node-sdk': { label: 'SDK', icon: 'sdk' },
  'phone-call': { label: 'Phone', icon: 'phone' },
  'python-sdk': { label: 'SDK', icon: 'sdk' },
  'rapida-app': { label: 'App', icon: 'app' },
  sdk: { label: 'SDK', icon: 'sdk' },
  sip: { label: 'SIP', icon: 'phone' },
  sms: { label: 'SMS', icon: 'chat' },
  telephony: { label: 'Phone', icon: 'phone' },
  telnyx: { label: 'Telnyx', icon: 'phone' },
  'telnyx-call': { label: 'Telnyx', icon: 'phone' },
  twilio: { label: 'Twilio', icon: 'phone' },
  'twilio-call': { label: 'Twilio', icon: 'phone' },
  'twilio-whatsapp': { label: 'Twilio WhatsApp', icon: 'chat' },
  voice: { label: 'Phone', icon: 'phone' },
  vonage: { label: 'Vonage', icon: 'phone' },
  'vonage-call': { label: 'Vonage', icon: 'phone' },
  web: { label: 'Web', icon: 'web' },
  'web-plugin': { label: 'Web', icon: 'web' },
  'web-widget': { label: 'Web', icon: 'web' },
  webrtc: { label: 'WebRTC', icon: 'web' },
  whatsapp: { label: 'WhatsApp', icon: 'chat' },
  widget: { label: 'Web', icon: 'web' },
};

const ACRONYMS = new Set(['api', 'ivr', 'sdk', 'sip', 'sms']);

const normalizeChannelKey = (channel: string): string =>
  channel
    .trim()
    .toLowerCase()
    .replace(/[_.\s]+/g, '-');

const formatChannelWord = (word: string): string => {
  const lowerWord = word.toLowerCase();
  if (ACRONYMS.has(lowerWord)) return lowerWord.toUpperCase();
  return `${lowerWord.charAt(0).toUpperCase()}${lowerWord.slice(1)}`;
};

const humanizeChannel = (channel: string): string =>
  channel
    .trim()
    .replace(/[_.-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .split(' ')
    .map(formatChannelWord)
    .join(' ');

export const normalizeChannel = (channel?: string | null): ChannelDisplay => {
  const rawChannel = channel?.trim();

  if (!rawChannel || rawChannel.toLowerCase() === UNKNOWN_CHANNEL_LABEL) {
    return {
      label: UNKNOWN_CHANNEL_LABEL,
      tooltip: 'No client.channel metadata was recorded for this session.',
      icon: 'unknown',
    };
  }

  const normalizedKey = normalizeChannelKey(rawChannel);
  const knownChannel = CHANNEL_ALIASES[normalizedKey];

  if (knownChannel) {
    return {
      ...knownChannel,
      tooltip: `client.channel: ${rawChannel}`,
    };
  }

  return {
    label: humanizeChannel(rawChannel),
    tooltip: `client.channel: ${rawChannel}`,
    icon: 'unknown',
  };
};
