export type WebhookEventGroup = 'Call' | 'Conversation';

export type WebhookEventOption = {
  id: string;
  name: string;
  description: string;
  group: WebhookEventGroup;
};

export const webhookEvents: WebhookEventOption[] = [
  {
    id: 'call.received',
    name: 'call.received',
    description: 'Triggered when an inbound call reaches the assistant.',
    group: 'Call',
  },
  {
    id: 'call.ringing',
    name: 'call.ringing',
    description: 'Triggered when an outbound call is ringing.',
    group: 'Call',
  },
  {
    id: 'call.provider_answered',
    name: 'call.provider_answered',
    description: 'Triggered when the telephony provider answers the call.',
    group: 'Call',
  },
  {
    id: 'call.outbound_requested',
    name: 'call.outbound_requested',
    description: 'Triggered when an outbound call is requested.',
    group: 'Call',
  },
  {
    id: 'call.outbound_dispatched',
    name: 'call.outbound_dispatched',
    description:
      'Triggered when an outbound call request is sent to the provider.',
    group: 'Call',
  },
  {
    id: 'call.outbound_dispatch_failed',
    name: 'call.outbound_dispatch_failed',
    description:
      'Triggered when an outbound call request cannot be sent to the provider.',
    group: 'Call',
  },
  {
    id: 'call.started',
    name: 'call.started',
    description: 'Triggered when the call session starts.',
    group: 'Call',
  },
  {
    id: 'call.hangup',
    name: 'call.hangup',
    description: 'Triggered when a hangup signal is received.',
    group: 'Call',
  },
  {
    id: 'call.ended',
    name: 'call.ended',
    description: 'Triggered when the call session finishes.',
    group: 'Call',
  },
  {
    id: 'call.failed',
    name: 'call.failed',
    description: 'Triggered when the call fails.',
    group: 'Call',
  },
  {
    id: 'call.cancelled',
    name: 'call.cancelled',
    description: 'Triggered when an outbound call is cancelled before connect.',
    group: 'Call',
  },
  {
    id: 'conversation.begin',
    name: 'conversation.begin',
    description: 'Triggered when a new conversation begins.',
    group: 'Conversation',
  },
  {
    id: 'conversation.resume',
    name: 'conversation.resume',
    description: 'Triggered when an existing conversation resumes.',
    group: 'Conversation',
  },
  {
    id: 'conversation.completed',
    name: 'conversation.completed',
    description: 'Triggered when a conversation completes.',
    group: 'Conversation',
  },
  {
    id: 'conversation.error',
    name: 'conversation.error',
    description: 'Triggered when a conversation fails with an error.',
    group: 'Conversation',
  },
];
