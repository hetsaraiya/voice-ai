import { AssistantConversation, Metadata, Metric } from '@rapidaai/react';
import {
  getChannelValue,
  getDisconnectReasonValue,
  getDurationBreakdownRows,
} from './session-list.helpers';

const metadata = (key: string, value: string): Metadata => {
  const m = new Metadata();
  m.setKey(key);
  m.setValue(value);
  return m;
};

const metric = (name: string, value: string): Metric => {
  const m = new Metric();
  m.setName(name);
  m.setValue(value);
  return m;
};

const conversation = (): AssistantConversation => new AssistantConversation();

describe('session list helpers', () => {
  it('reads channel metadata with unknown fallback', () => {
    const conv = conversation();
    conv.addMetadata(metadata('client.channel', 'phone'));

    expect(getChannelValue(conv)).toBe('phone');
    expect(getChannelValue(conversation())).toBe('unknown');
  });

  it('reads disconnect reason metadata with unknown fallback for blank values', () => {
    const conv = conversation();
    conv.addMetadata(metadata('disconnect_reason', '  '));

    expect(getDisconnectReasonValue(conv)).toBe('unknown');
  });

  it('converts duration breakdown metrics to seconds', () => {
    const conv = conversation();
    conv.addMetrics(metric('duration', '6575255208'));
    conv.addMetrics(metric('telephony_duration', '7'));
    conv.addMetrics(metric('tts_duration', '5456317791'));
    conv.addMetrics(metric('stt_duration', '5481100708'));

    expect(getDurationBreakdownRows(conv)).toEqual([
      { key: 'duration', value: '6.58' },
      { key: 'telephony_duration', value: '7.00' },
      { key: 'tts_duration', value: '5.46' },
      { key: 'stt_duration', value: '5.48' },
    ]);
  });
});
