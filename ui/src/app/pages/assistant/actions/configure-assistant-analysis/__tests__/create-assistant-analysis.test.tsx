import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';

import { CreateAssistantAnalysis } from '@/app/pages/assistant/actions/configure-assistant-analysis/create-assistant-analysis';
import { CreateAnalysis } from '@rapidaai/react';
import toast from 'react-hot-toast/headless';

const mockGoBack = jest.fn();
const mockGoToConfigureAssistantAnalysis = jest.fn();

let mockEndpointIdToPick = 'endpoint-1';

jest.mock('@rapidaai/react', () => ({
  ConnectionConfig: class ConnectionConfig {
    constructor(_: unknown) {}
  },
  Metadata: class Metadata {
    key = '';
    value = '';
    setKey(v: string) {
      this.key = v;
    }
    setValue(v: string) {
      this.value = v;
    }
    getKey() {
      return this.key;
    }
    getValue() {
      return this.value;
    }
  },
  CreateAssistantAnalysisRequest: class CreateAssistantAnalysisRequest {
    assistantId = '';
    provider = '';
    name = '';
    description = '';
    executionPriority = 0;
    optionsList: any[] = [];
    setAssistantid(v: string) {
      this.assistantId = v;
    }
    setName(v: string) {
      this.name = v;
    }
    setProvider(v: string) {
      this.provider = v;
    }
    setDescription(v: string) {
      this.description = v;
    }
    setExecutionpriority(v: number) {
      this.executionPriority = v;
    }
    setOptionsList(v: any[]) {
      this.optionsList = v;
    }
    getOptionsList() {
      return this.optionsList;
    }
  },
  CreateAnalysis: jest.fn(),
}));

jest.mock('react-hot-toast/headless', () => ({
  __esModule: true,
  default: {
    success: jest.fn(),
  },
}));

jest.mock('@/hooks/use-global-navigator', () => ({
  useGlobalNavigation: () => ({
    goBack: mockGoBack,
    goToConfigureAssistantAnalysis: mockGoToConfigureAssistantAnalysis,
  }),
}));

jest.mock('@/hooks/use-credential', () => ({
  useCurrentCredential: () => ({
    authId: 'auth-1',
    token: 'token-1',
    projectId: 'project-1',
  }),
}));

jest.mock('@/utils', () => ({
  cn: (...inputs: any[]) => inputs.filter(Boolean).join(' '),
  randomMeaningfullName: () => 'analysis-default',
}));

jest.mock('@/app/pages/assistant/actions/hooks/use-confirmation', () => ({
  useConfirmDialog: () => ({
    showDialog: (cb: () => void) => cb(),
    ConfirmDialogComponent: () => null,
  }),
}));

jest.mock('@/app/components/dropdown/endpoint-dropdown', () => ({
  EndpointDropdown: ({ onChangeEndpoint }: any) => (
    <button
      type="button"
      onClick={() =>
        onChangeEndpoint({
          getId: () => mockEndpointIdToPick,
        })
      }
    >
      Pick endpoint
    </button>
  ),
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

jest.mock('@/app/components/carbon/form', () => ({
  Stack: ({ children }: any) => <div>{children}</div>,
  TextInput: ({
    labelText: _l,
    helperText: _h,
    hideLabel: _hl,
    warn: _w,
    warnText: _wt,
    invalid: _inv,
    invalidText: _it,
    ...props
  }: any) => <input {...props} />,
  TextArea: ({
    labelText: _l,
    helperText: _h,
    hideLabel: _hl,
    warn: _w,
    warnText: _wt,
    invalid: _inv,
    invalidText: _it,
    ...props
  }: any) => <textarea {...props} />,
}));

jest.mock('@/app/components/carbon/button', () => ({
  PrimaryButton: ({
    children,
    isLoading: _,
    renderIcon: _r,
    hasIconOnly: _h,
    iconDescription: _d,
    ...props
  }: any) => <button {...props}>{children}</button>,
  SecondaryButton: ({
    children,
    isLoading: _,
    renderIcon: _r,
    hasIconOnly: _h,
    iconDescription: _d,
    ...props
  }: any) => <button {...props}>{children}</button>,
  TertiaryButton: ({
    children,
    isLoading: _,
    renderIcon: _r,
    hasIconOnly: _h,
    iconDescription: _d,
    ...props
  }: any) => <button {...props}>{children}</button>,
}));

jest.mock('@carbon/react', () => ({
  ButtonSet: ({ children }: any) => <div>{children}</div>,
  Button: ({
    children,
    hasIconOnly: _,
    renderIcon: _r,
    iconDescription: _d,
    ...props
  }: any) => <button {...props}>{children}</button>,
  Select: ({ children, labelText: _, hideLabel: _h, ...props }: any) => (
    <select {...props}>{children}</select>
  ),
  SelectItem: ({ value, text }: any) => <option value={value}>{text}</option>,
  Tooltip: ({ children }: any) => <span>{children}</span>,
  NumberInput: ({
    label,
    helperText: _ht,
    onChange,
    value,
    hideLabel: _hl,
    ...rest
  }: any) => (
    <input
      aria-label={label}
      value={value}
      onChange={e => onChange(e, { value: Number(e.target.value) })}
    />
  ),
}));

describe('CreateAssistantAnalysis', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockEndpointIdToPick = 'endpoint-1';
  });

  it('shows endpoint validation error when continuing without endpoint', () => {
    render(<CreateAssistantAnalysis assistantId="assistant-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(
      screen.getByText(
        'Please select a valid endpoint to be executed for analysis.',
      ),
    ).toBeInTheDocument();
    expect(CreateAnalysis).not.toHaveBeenCalled();
  });

  it('creates analysis successfully and navigates back to analysis listing', async () => {
    (CreateAnalysis as jest.Mock).mockResolvedValue({ getSuccess: () => true });

    render(<CreateAssistantAnalysis assistantId="assistant-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Pick endpoint' }));
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    fireEvent.click(screen.getByRole('button', { name: 'Configure analysis' }));

    await waitFor(() => {
      expect(CreateAnalysis).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        'Analysis added to assistant successfully',
      );
    });
    expect(mockGoToConfigureAssistantAnalysis).toHaveBeenCalledWith(
      'assistant-1',
    );
  });

  it('shows human error message when create API returns unsuccessful response', async () => {
    (CreateAnalysis as jest.Mock).mockResolvedValue({
      getSuccess: () => false,
      getError: () => ({ getHumanmessage: () => 'Name already used' }),
    });

    render(<CreateAssistantAnalysis assistantId="assistant-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Pick endpoint' }));
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    fireEvent.click(screen.getByRole('button', { name: 'Configure analysis' }));

    expect(await screen.findByText('Name already used')).toBeInTheDocument();
  });

  it('supports add and edit for parameter mapping before create', async () => {
    (CreateAnalysis as jest.Mock).mockResolvedValue({ getSuccess: () => true });

    render(<CreateAssistantAnalysis assistantId="assistant-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Pick endpoint' }));
    fireEvent.change(
      document.getElementById('tool-condition-key') as HTMLElement,
      {
        target: { value: 'conversation_mode' },
      },
    );
    fireEvent.change(
      document.getElementById('tool-condition-source-value') as HTMLElement,
      {
        target: { value: 'voice' },
      },
    );
    fireEvent.click(screen.getByRole('button', { name: 'Add parameter' }));
    fireEvent.change(document.getElementById('param-type-1') as HTMLElement, {
      target: { value: 'assistant' },
    });
    fireEvent.change(
      document.getElementById('param-key-1') as HTMLElement,
      {
        target: { value: 'name' },
      },
    );
    fireEvent.change(document.getElementById('param-val-1') as HTMLElement, {
      target: { value: 'assistantName' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
    fireEvent.click(screen.getByRole('button', { name: 'Configure analysis' }));

    await waitFor(() => expect(CreateAnalysis).toHaveBeenCalled());
    const request = (CreateAnalysis as jest.Mock).mock.calls[0][1];
    const mappedParams = request.getOptionsList().map((option: any) => ({
      key: option.getKey(),
      value: option.getValue(),
    }));
    expect(mappedParams).toEqual(
      expect.arrayContaining([
        { key: 'endpoint_id', value: 'endpoint-1' },
        { key: 'endpoint_version', value: 'latest' },
        {
          key: 'endpoint_parameters',
          value: JSON.stringify({
            'conversation.messages': 'messages',
            'assistant.name': 'assistantName',
          }),
        },
        {
          key: 'analysis.condition',
          value: JSON.stringify([
            {
              key: 'conversation_mode',
              condition: '=',
              value: 'voice',
            },
          ]),
        },
      ]),
    );
  });

  it('blocks reserved analysis option mapping key', () => {
    render(<CreateAssistantAnalysis assistantId="assistant-1" />);

    fireEvent.click(screen.getByRole('button', { name: 'Pick endpoint' }));
    fireEvent.change(document.getElementById('param-type-0') as HTMLElement, {
      target: { value: 'option' },
    });
    fireEvent.change(document.getElementById('param-key-0') as HTMLElement, {
      target: { value: 'endpoint_parameters' },
    });
    fireEvent.change(document.getElementById('param-val-0') as HTMLElement, {
      target: { value: 'shouldFail' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }));

    expect(
      screen.getByText(
        'option.endpoint_parameters is reserved and managed by analysis options.',
      ),
    ).toBeInTheDocument();
    expect(CreateAnalysis).not.toHaveBeenCalled();
  });
});
