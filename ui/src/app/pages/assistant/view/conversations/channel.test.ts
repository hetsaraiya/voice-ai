import { normalizeChannel } from './channel';

describe('normalizeChannel', () => {
  it.each([
    ['telephony', 'Phone'],
    ['phone-call', 'Phone'],
    ['twilio', 'Twilio'],
    ['twilio-call', 'Twilio'],
    ['exotel', 'Exotel'],
    ['telnyx', 'Telnyx'],
    ['vonage', 'Vonage'],
    ['sip', 'SIP'],
    ['webrtc', 'WebRTC'],
    ['web', 'Web'],
    ['web_plugin', 'Web'],
    ['debugger', 'Debugger'],
    ['node-sdk', 'SDK'],
    ['api', 'API'],
    ['whatsapp', 'WhatsApp'],
    ['mobile_app', 'App'],
  ])('normalizes %s to %s', (raw, label) => {
    expect(normalizeChannel(raw).label).toBe(label);
  });

  it('falls back to unknown when the metadata is missing', () => {
    expect(normalizeChannel(undefined).label).toBe('unknown');
    expect(normalizeChannel('').label).toBe('unknown');
    expect(normalizeChannel('   ').label).toBe('unknown');
  });

  it('humanizes unmapped channel values', () => {
    const display = normalizeChannel('custom_sip_channel');

    expect(display.label).toBe('Custom SIP Channel');
    expect(display.tooltip).toBe('client.channel: custom_sip_channel');
  });
});
