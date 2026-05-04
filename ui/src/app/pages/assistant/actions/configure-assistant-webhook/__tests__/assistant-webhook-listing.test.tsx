import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ConfigureAssistantWebhookPage } from '@/app/pages/assistant/actions/configure-assistant-webhook';
import { useAssistantWebhookPageStore } from '@/app/pages/assistant/actions/store/use-webhook-page-store';

const mockGetAssistantWebhook = jest.fn();
const mockShowLoader = jest.fn();
const mockHideLoader = jest.fn();

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useParams: () => ({ assistantId: 'assistant-1' }),
}));

jest.mock('@/hooks/use-credential', () => ({
  useCurrentCredential: () => ({ authId: 'u1', token: 't1', projectId: 'p1' }),
}));

jest.mock('@/hooks/use-global-navigator', () => ({
  useGlobalNavigation: () => ({
    goToCreateAssistantWebhook: jest.fn(),
    goToEditAssistantWebhook: jest.fn(),
  }),
}));

jest.mock('@/hooks', () => ({
  useRapidaStore: () => ({
    loading: false,
    showLoader: mockShowLoader,
    hideLoader: mockHideLoader,
  }),
}));

jest.mock('@/app/components/loader/section-loader', () => ({
  SectionLoader: () => <div>loading</div>,
}));

jest.mock('@/app/components/sections/table-section', () => ({
  TableSection: ({ children }: any) => <div>{children}</div>,
}));

jest.mock('@/app/components/carbon/empty-state', () => ({
  EmptyState: ({ title }: any) => <div>{title}</div>,
}));

jest.mock('@/app/components/carbon/button', () => ({
  IconOnlyButton: ({
    children,
    renderIcon: _renderIcon,
    iconDescription: _iconDescription,
    ...props
  }: any) => (
    <button {...props}>{children || 'icon'}</button>
  ),
  PrimaryButton: ({ children, renderIcon: _renderIcon, ...props }: any) => (
    <button {...props}>{children || 'primary'}</button>
  ),
}));

jest.mock('@/app/components/carbon/pagination', () => ({
  Pagination: ({ onChange, pageSize }: any) => (
    <div>
      <button
        data-testid="change-page"
        onClick={() => onChange({ page: 2, pageSize })}
      >
        next
      </button>
      <button
        data-testid="change-page-size"
        onClick={() => onChange({ page: 3, pageSize: 50 })}
      >
        resize
      </button>
    </div>
  ),
}));

jest.mock('@carbon/react', () => {
  const React = require('react');
  const Div = ({ children }: any) => React.createElement('div', null, children);
  return {
    Table: Div,
    TableHead: Div,
    TableRow: Div,
    TableHeader: Div,
    TableBody: Div,
    TableCell: Div,
    TableToolbar: Div,
    TableToolbarContent: Div,
    TableToolbarSearch: ({ onChange }: any) =>
      React.createElement('input', { onChange }),
    Button: ({
      children,
      hasIconOnly: _hasIconOnly,
      renderIcon: _renderIcon,
      iconDescription: _iconDescription,
      ...props
    }: any) =>
      React.createElement('button', props, children || 'button'),
    TableBatchActions: Div,
    TableBatchAction: ({ children, ...props }: any) =>
      React.createElement('button', props, children || 'action'),
    RadioButton: ({
      onChange,
      checked,
      labelText: _labelText,
      hideLabel: _hideLabel,
      ...props
    }: any) =>
      React.createElement('input', { ...props, type: 'radio', checked, onChange }),
    Breadcrumb: Div,
    BreadcrumbItem: ({ children, ...props }: any) =>
      React.createElement('a', props, children),
    Tag: ({ children }: any) => React.createElement('span', null, children),
  };
});

jest.mock('react-hot-toast/headless', () => ({
  error: jest.fn(),
}));

const makeWebhook = (id: string) =>
  ({
    getId: () => id,
    getOptionsList: () => [],
    getAssistanteventsList: () => ['conversation.begin'],
    getExecutionpriority: () => 1,
    getStatus: () => 'ACTIVE',
    getCreateddate: () => undefined,
  }) as any;

describe('ConfigureAssistantWebhookPage listing', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useAssistantWebhookPageStore.setState({
      webhooks: [makeWebhook('w-1')],
      page: 1,
      pageSize: 20,
      totalCount: 1,
      criteria: [],
      getAssistantWebhook: mockGetAssistantWebhook as any,
    });
  });

  it('refetches when page changes from pagination', async () => {
    render(<ConfigureAssistantWebhookPage />);

    await waitFor(() => expect(mockGetAssistantWebhook).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByTestId('change-page'));

    await waitFor(() => expect(mockGetAssistantWebhook).toHaveBeenCalledTimes(2));
    expect(useAssistantWebhookPageStore.getState().page).toBe(2);
  });

  it('resets to first page and refetches when page size changes', async () => {
    useAssistantWebhookPageStore.setState({ page: 4 });
    render(<ConfigureAssistantWebhookPage />);

    await waitFor(() => expect(mockGetAssistantWebhook).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByTestId('change-page-size'));

    await waitFor(() => expect(mockGetAssistantWebhook).toHaveBeenCalledTimes(2));
    expect(useAssistantWebhookPageStore.getState().page).toBe(1);
    expect(useAssistantWebhookPageStore.getState().pageSize).toBe(50);
  });
});
