import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { CreateAssistantWebhook } from '@/app/pages/assistant/actions/configure-assistant-webhook/create-assistant-webhook';
import { UpdateAssistantWebhook } from '@/app/pages/assistant/actions/configure-assistant-webhook/update-assistant-webhook';
import {
  CreateWebhook,
  GetAssistantWebhook,
  UpdateWebhook,
} from '@rapidaai/react';

let mockParams: Record<string, string | undefined> = {
  assistantId: 'assistant-1',
  webhookId: 'webhook-1',
};

const mockShowLoader = jest.fn();
const mockHideLoader = jest.fn();
const mockNavigate = {
  goBack: jest.fn(),
  goToAssistantWebhook: jest.fn(),
};

jest.mock('@rapidaai/react', () => ({
  ConnectionConfig: class {
    constructor(_: unknown) {}
    static WithDebugger(config: unknown) {
      return config;
    }
  },
  Metadata: class {
    key = '';
    value = '';
    setKey(value: string) {
      this.key = value;
    }
    getKey() {
      return this.key;
    }
    setValue(value: string) {
      this.value = value;
    }
    getValue() {
      return this.value;
    }
  },
  CreateAssistantWebhookRequest: class {
    assistantId = '';
    provider = '';
    assistantEventsList: string[] = [];
    description = '';
    executionPriority = 0;
    optionsList: any[] = [];
    setAssistantid(value: string) {
      this.assistantId = value;
    }
    getAssistantid() {
      return this.assistantId;
    }
    setAssistanteventsList(value: string[]) {
      this.assistantEventsList = value;
    }
    setProvider(value: string) {
      this.provider = value;
    }
    getProvider() {
      return this.provider;
    }
    getAssistanteventsList() {
      return this.assistantEventsList;
    }
    setDescription(value: string) {
      this.description = value;
    }
    getDescription() {
      return this.description;
    }
    setExecutionpriority(value: number) {
      this.executionPriority = value;
    }
    getExecutionpriority() {
      return this.executionPriority;
    }
    setOptionsList(value: any[]) {
      this.optionsList = value;
    }
    getOptionsList() {
      return this.optionsList;
    }
  },
  UpdateAssistantWebhookRequest: class {
    id = '';
    assistantId = '';
    provider = '';
    assistantEventsList: string[] = [];
    description = '';
    executionPriority = 0;
    optionsList: any[] = [];
    setId(value: string) {
      this.id = value;
    }
    getId() {
      return this.id;
    }
    setAssistantid(value: string) {
      this.assistantId = value;
    }
    getAssistantid() {
      return this.assistantId;
    }
    setAssistanteventsList(value: string[]) {
      this.assistantEventsList = value;
    }
    setProvider(value: string) {
      this.provider = value;
    }
    getProvider() {
      return this.provider;
    }
    getAssistanteventsList() {
      return this.assistantEventsList;
    }
    setDescription(value: string) {
      this.description = value;
    }
    getDescription() {
      return this.description;
    }
    setExecutionpriority(value: number) {
      this.executionPriority = value;
    }
    getExecutionpriority() {
      return this.executionPriority;
    }
    setOptionsList(value: any[]) {
      this.optionsList = value;
    }
    getOptionsList() {
      return this.optionsList;
    }
  },
  GetAssistantWebhookRequest: class {
    id = '';
    assistantId = '';
    setId(value: string) {
      this.id = value;
    }
    getId() {
      return this.id;
    }
    setAssistantid(value: string) {
      this.assistantId = value;
    }
    getAssistantid() {
      return this.assistantId;
    }
  },
  CreateWebhook: jest.fn(),
  GetAssistantWebhook: jest.fn(),
  UpdateWebhook: jest.fn(),
}));

jest.mock('react-router-dom', () => ({
  ...jest.requireActual('react-router-dom'),
  useParams: () => mockParams,
}));

jest.mock('@/hooks', () => ({
  useRapidaStore: () => ({
    loading: false,
    showLoader: mockShowLoader,
    hideLoader: mockHideLoader,
  }),
}));

jest.mock('@/hooks/use-global-navigator', () => ({
  useGlobalNavigation: () => mockNavigate,
}));

jest.mock('@/hooks/use-credential', () => ({
  useCurrentCredential: () => ({ authId: 'u1', token: 't1', projectId: 'p1' }),
}));

jest.mock('@/app/pages/assistant/actions/hooks/use-confirmation', () => ({
  useConfirmDialog: () => ({
    showDialog: (cb: () => void) => cb(),
    ConfirmDialogComponent: () => null,
  }),
}));

jest.mock('@/app/components/form/tab-form', () => ({
  TabForm: ({ form, activeTab, errorMessage, formHeading }: any) => {
    const React = require('react');
    const active = form.find((f: any) => f.code === activeTab) || form[0];
    return (
      <div>
        <h1>{formHeading}</h1>
        {errorMessage ? <div>{errorMessage}</div> : null}
        <div>{active.body}</div>
        <div>
          {Array.isArray(active.actions)
            ? active.actions.map((action: React.ReactElement, idx: number) => (
                <div key={idx}>{action}</div>
              ))
            : active.actions}
        </div>
      </div>
    );
  },
}));

jest.mock('@/app/components/carbon/form', () => {
  const React = require('react');
  return {
    Stack: ({ children }: any) => React.createElement('div', null, children),
    FormGroup: ({ children, legendText, messageText }: any) =>
      React.createElement(
        'fieldset',
        null,
        legendText ? React.createElement('legend', null, legendText) : null,
        messageText ? React.createElement('div', null, messageText) : null,
        children,
      ),
    TextInput: ({
      id,
      labelText,
      value,
      onChange,
      placeholder,
      hideLabel,
    }: any) =>
      React.createElement(
        'div',
        null,
        !hideLabel && labelText
          ? React.createElement('label', { htmlFor: id }, labelText)
          : null,
        React.createElement('input', {
          id,
          value: value ?? '',
          onChange,
          placeholder,
          'data-testid': id,
        }),
      ),
    TextArea: ({ id, labelText, value, onChange, placeholder }: any) =>
      React.createElement(
        'div',
        null,
        labelText
          ? React.createElement('label', { htmlFor: id }, labelText)
          : null,
        React.createElement('textarea', {
          id,
          value: value ?? '',
          onChange,
          placeholder,
          'data-testid': id,
        }),
      ),
  };
});

jest.mock('@/app/components/input-group', () => {
  const React = require('react');
  return {
    InputGroup: ({ title, children }: any) =>
      React.createElement(
        'section',
        null,
        title ? React.createElement('div', null, title) : null,
        children,
      ),
  };
});

jest.mock('@/app/components/form/slider', () => {
  const React = require('react');
  return {
    Slider: ({ value, onSlide }: any) =>
      React.createElement('input', {
        type: 'range',
        value,
        onChange: (e: any) => onSlide(Number(e.target.value)),
        'data-testid': 'webhook-timeout-slider',
      }),
  };
});

jest.mock('@/app/components/carbon/button', () => {
  const React = require('react');
  return {
    PrimaryButton: ({
      children,
      isLoading: _,
      renderIcon: _r,
      hasIconOnly: _h,
      iconDescription: _d,
      ...props
    }: any) => React.createElement('button', props, children),
    SecondaryButton: ({
      children,
      isLoading: _,
      renderIcon: _r,
      hasIconOnly: _h,
      iconDescription: _d,
      ...props
    }: any) => React.createElement('button', props, children),
    TertiaryButton: ({
      children,
      isLoading: _,
      renderIcon: _r,
      hasIconOnly: _h,
      iconDescription: _d,
      ...props
    }: any) => React.createElement('button', props, children),
  };
});

jest.mock('@carbon/react', () => {
  const React = require('react');
  return {
    ButtonSet: ({ children }: any) =>
      React.createElement('div', null, children),
    Tooltip: ({ children }: any) => React.createElement('span', null, children),
    Select: ({ id, labelText, value, onChange, children, hideLabel }: any) =>
      React.createElement(
        'div',
        null,
        !hideLabel && labelText
          ? React.createElement('label', { htmlFor: id }, labelText)
          : null,
        React.createElement(
          'select',
          { id, value, onChange, 'data-testid': id },
          children,
        ),
      ),
    SelectItem: ({ value, text }: any) =>
      React.createElement('option', { value }, text),
    NumberInput: ({ id, value, onChange, label, hideLabel }: any) =>
      React.createElement(
        'div',
        null,
        !hideLabel && label
          ? React.createElement('label', { htmlFor: id }, label)
          : null,
        React.createElement('input', {
          id,
          type: 'number',
          value,
          onChange: (e: any) => onChange?.(e, { value: e.target.value }),
          'data-testid': id,
        }),
      ),
    Checkbox: ({ id, labelText, checked, onChange }: any) =>
      React.createElement(
        'label',
        { htmlFor: id },
        React.createElement('input', {
          id,
          type: 'checkbox',
          checked: !!checked,
          onChange: (e: any) => onChange?.(e, { checked: e.target.checked }),
        }),
        labelText,
      ),
    StructuredListWrapper: ({
      children,
      isCondensed: _isCondensed,
      isFlush: _isFlush,
      selection: _selection,
      selectedInitialRow: _selectedInitialRow,
      ...props
    }: any) => React.createElement('div', props, children),
    StructuredListHead: ({ children }: any) =>
      React.createElement('div', null, children),
    StructuredListBody: ({ children }: any) =>
      React.createElement('div', null, children),
    StructuredListRow: ({
      children,
      head: _head,
      selection: _selection,
    }: any) => React.createElement('div', null, children),
    StructuredListCell: ({ children, head: _head, noWrap: _noWrap }: any) =>
      React.createElement('div', null, children),
    Button: ({
      iconDescription,
      children,
      hasIconOnly: _,
      renderIcon: _r,
      ...props
    }: any) =>
      React.createElement(
        'button',
        { ...props, 'aria-label': iconDescription || children || 'button' },
        children || 'button',
      ),
  };
});

jest.mock('react-hot-toast/headless', () => ({
  success: jest.fn(),
  error: jest.fn(),
}));

describe('Assistant webhook flows', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockParams = { assistantId: 'assistant-1', webhookId: 'webhook-1' };

    (CreateWebhook as jest.Mock).mockResolvedValue({
      getSuccess: () => true,
    });

    (GetAssistantWebhook as jest.Mock).mockResolvedValue({
      getData: () => ({
        getDescription: () => 'existing webhook',
        getExecutionpriority: () => 1,
        getAssistanteventsList: () => ['conversation.begin'],
        getOptionsList: () => [
          {
            getKey: () => 'http_method',
            getValue: () => 'POST',
          },
          {
            getKey: () => 'http_url',
            getValue: () => 'https://hooks.example.com/incoming',
          },
          {
            getKey: () => 'retry_status_codes',
            getValue: () => '["50X"]',
          },
          {
            getKey: () => 'max_retry_count',
            getValue: () => '2',
          },
          {
            getKey: () => 'timeout_seconds',
            getValue: () => '200',
          },
          {
            getKey: () => 'http_headers',
            getValue: () => '{"Authorization":"Bearer token"}',
          },
          {
            getKey: () => 'webhook.condition',
            getValue: () =>
              '[{"key":"source","condition":"=","value":"phone"}]',
          },
          {
            getKey: () => 'http_body',
            getValue: () =>
              '{"event.type":"event","assistant.id":"assistant_id"}',
          },
        ],
      }),
    });

    (UpdateWebhook as jest.Mock).mockResolvedValue({
      getSuccess: () => true,
    });
  });

  const selectFirstEventAndContinue = () => {
    const eventCheckbox = document.getElementById(
      'webhook-event-call.received',
    ) as HTMLInputElement;
    fireEvent.click(eventCheckbox);
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  };

  const selectWebRTCEventAndContinue = () => {
    const eventCheckbox = document.getElementById(
      'webhook-event-webrtc.connected',
    ) as HTMLInputElement;
    fireEvent.click(eventCheckbox);
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  };

  const continueWithLoadedEvent = async () => {
    await waitFor(() => {
      expect(
        document.getElementById(
          'webhook-event-conversation.begin',
        ) as HTMLInputElement,
      ).toBeChecked();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  };

  const continueWithLoadedAndWebRTCEvent = async () => {
    await waitFor(() => {
      expect(
        document.getElementById(
          'webhook-event-conversation.begin',
        ) as HTMLInputElement,
      ).toBeChecked();
    });
    const eventCheckbox = document.getElementById(
      'webhook-event-webrtc.reconnecting',
    ) as HTMLInputElement;
    fireEvent.click(eventCheckbox);
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
  };

  it('create webhook supports header add-delete and continues without payload mapping', () => {
    render(<CreateAssistantWebhook assistantId="assistant-1" />);

    selectFirstEventAndContinue();

    fireEvent.change(screen.getByTestId('webhook-endpoint'), {
      target: { value: 'https://api.example.com/webhook' },
    });

    expect(screen.getByText('Headers (0)')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Add header' }));
    expect(screen.getByText('Headers (1)')).toBeInTheDocument();
    const headerKeyInput = screen.getByTestId('api-header-key-0');
    const headerRow = headerKeyInput.closest('tr');
    const headerRemoveButton = headerRow?.querySelector('button');
    expect(headerRemoveButton).toBeTruthy();
    fireEvent.click(headerRemoveButton!);
    expect(screen.getByText('Headers (0)')).toBeInTheDocument();
    expect(screen.getByText('Destination')).toBeInTheDocument();
  });

  it('create webhook validates headers when key is present without value', () => {
    render(<CreateAssistantWebhook assistantId="assistant-1" />);

    selectFirstEventAndContinue();

    fireEvent.change(screen.getByTestId('webhook-endpoint'), {
      target: { value: 'https://api.example.com/webhook' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Add header' }));
    fireEvent.change(screen.getByTestId('api-header-key-0'), {
      target: { value: 'Authorization' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Configure webhook' }));

    expect(
      screen.getByText('Headers with a key must also include a value.'),
    ).toBeInTheDocument();
  });

  it('create webhook submits with selected event and configuration', async () => {
    render(<CreateAssistantWebhook assistantId="assistant-1" />);

    selectWebRTCEventAndContinue();

    fireEvent.change(screen.getByTestId('webhook-endpoint'), {
      target: { value: 'https://api.example.com/webhook' },
    });
    fireEvent.change(screen.getByTestId('webhook-max-retries'), {
      target: { value: '2' },
    });
    fireEvent.change(screen.getByTestId('webhook-timeout'), {
      target: { value: '220' },
    });
    fireEvent.change(screen.getByTestId('webhook-priority'), {
      target: { value: '4' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Configure webhook' }));

    await waitFor(() => expect(CreateWebhook).toHaveBeenCalledTimes(1));
    const req = (CreateWebhook as jest.Mock).mock.calls[0][1];
    expect(req.getAssistantid()).toBe('assistant-1');
    expect(req.getAssistanteventsList()).toEqual(['webrtc.connected']);
    expect(req.getExecutionpriority()).toBe(4);
    expect(req.getDescription()).toBe('');

    const optionMap = new Map(
      req
        .getOptionsList()
        .map((option: any) => [option.getKey(), option.getValue()]),
    );
    expect(optionMap.get('http_method')).toBe('POST');
    expect(optionMap.get('http_url')).toBe('https://api.example.com/webhook');
    expect(optionMap.get('max_retry_count')).toBe('2');
    expect(optionMap.get('timeout_seconds')).toBe('220');
    expect(optionMap.has('http_body')).toBe(false);
    expect(optionMap.has('webhook.condition')).toBe(false);
  });

  it('update webhook validates invalid destination url', async () => {
    render(<UpdateAssistantWebhook assistantId="assistant-1" />);

    await continueWithLoadedEvent();

    await waitFor(() => {
      expect(screen.getByTestId('webhook-endpoint')).toHaveValue(
        'https://hooks.example.com/incoming',
      );
    });

    fireEvent.change(screen.getByTestId('webhook-endpoint'), {
      target: { value: 'bad-url' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Update webhook' }));

    expect(
      screen.getByText('Please provide a valid server URL for the webhook.'),
    ).toBeInTheDocument();
  });

  it('update webhook validates headers when key is present without value', async () => {
    render(<UpdateAssistantWebhook assistantId="assistant-1" />);

    await continueWithLoadedEvent();

    await waitFor(() => {
      expect(screen.getByTestId('webhook-endpoint')).toHaveValue(
        'https://hooks.example.com/incoming',
      );
    });

    fireEvent.change(screen.getByTestId('api-header-val-0'), {
      target: { value: '' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Update webhook' }));

    expect(
      screen.getByText('Headers with a key must also include a value.'),
    ).toBeInTheDocument();
  });

  it('update webhook submits with loaded values', async () => {
    render(<UpdateAssistantWebhook assistantId="assistant-1" />);

    await continueWithLoadedAndWebRTCEvent();

    await waitFor(() => {
      expect(screen.getByTestId('webhook-endpoint')).toHaveValue(
        'https://hooks.example.com/incoming',
      );
    });

    fireEvent.click(screen.getByRole('button', { name: 'Update webhook' }));

    await waitFor(() => expect(UpdateWebhook).toHaveBeenCalledTimes(1));
    const req = (UpdateWebhook as jest.Mock).mock.calls[0][1];
    expect(req.getAssistantid()).toBe('assistant-1');
    expect(req.getId()).toBe('webhook-1');
    expect(req.getAssistanteventsList()).toEqual([
      'conversation.begin',
      'webrtc.reconnecting',
    ]);
    expect(req.getExecutionpriority()).toBe(1);
    expect(req.getDescription()).toBe('existing webhook');
    const optionMap = new Map(
      req
        .getOptionsList()
        .map((option: any) => [option.getKey(), option.getValue()]),
    );
    expect(optionMap.has('http_body')).toBe(false);
    expect(optionMap.has('webhook.condition')).toBe(false);
  });
});
