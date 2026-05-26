import { normalizeDisconnectReason } from './disconnect-reason';

describe('normalizeDisconnectReason', () => {
  it.each([
    ['ConversationDisconnection_DISCONNECTION_TYPE_TOOL', 'Tool ended'],
    ['DISCONNECTION_TYPE_USER', 'User disconnected'],
    ['DISCONNECTION_TYPE_IDLE_TIMEOUT', 'Idle timeout'],
    ['DISCONNECTION_TYPE_MAX_DURATION', 'Max duration'],
    ['DISCONNECTION_TYPE_ERROR', 'Error'],
  ])('normalizes %s to %s', (raw, label) => {
    expect(normalizeDisconnectReason(raw).label).toBe(label);
  });

  it('normalizes numeric enum values', () => {
    expect(normalizeDisconnectReason('1').label).toBe('Tool ended');
    expect(normalizeDisconnectReason('2').label).toBe('User disconnected');
    expect(normalizeDisconnectReason('3').label).toBe('Idle timeout');
    expect(normalizeDisconnectReason('4').label).toBe('Max duration');
    expect(normalizeDisconnectReason('5').label).toBe('Error');
  });

  it('returns unknown when the value is missing or blank', () => {
    expect(normalizeDisconnectReason(undefined).label).toBe('unknown');
    expect(normalizeDisconnectReason('').label).toBe('unknown');
    expect(normalizeDisconnectReason('   ').label).toBe('unknown');
  });

  it('humanizes unexpected non-empty values', () => {
    expect(normalizeDisconnectReason('network-timeout').label).toBe(
      'Network Timeout',
    );
  });
});
