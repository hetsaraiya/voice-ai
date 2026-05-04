import { useAssistantWebhookPageStore } from '@/app/pages/assistant/actions/store/use-webhook-page-store';

jest.mock('@rapidaai/react', () => {
  class ConnectionConfig {
    constructor(_: unknown) {}
    static WithDebugger(config: unknown) {
      return config;
    }
  }

  return {
    ConnectionConfig,
    GetAllAssistantWebhook: jest.fn(),
    DeleteAssistantWebhook: jest.fn(),
    GetAllAssistantWebhookRequest: class {},
    DeleteAssistantWebhookRequest: class {},
    Paginate: class {},
    Criteria: class {},
  };
});

describe('useAssistantWebhookPageStore', () => {
  beforeEach(() => {
    useAssistantWebhookPageStore.setState({
      criteria: [],
      page: 1,
      pageSize: 20,
      totalCount: 0,
      webhooks: [],
    });
  });

  it('replaces criteria only for the same key and logic', () => {
    const state = useAssistantWebhookPageStore.getState();

    state.addCriteria('status', 'active', 'and');
    state.addCriteria('status', 'draft', 'or');
    state.addCriteria('status', 'paused', 'and');

    expect(useAssistantWebhookPageStore.getState().criteria).toEqual([
      { key: 'status', value: 'draft', logic: 'or' },
      { key: 'status', value: 'paused', logic: 'and' },
    ]);
  });
});

