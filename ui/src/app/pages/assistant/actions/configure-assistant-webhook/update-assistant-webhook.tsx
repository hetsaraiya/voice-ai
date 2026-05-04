import React, { FC, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useConfirmDialog } from '@/app/pages/assistant/actions/hooks/use-confirmation';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { PrimaryButton, SecondaryButton } from '@/app/components/carbon/button';
import { TextInput, TextArea, Stack } from '@/app/components/carbon/form';
import { MultiSelect } from '@/app/components/carbon/dropdown';
import { InputGroup } from '@/app/components/input-group';
import {
  ButtonSet,
  Select as CarbonSelect,
  SelectItem,
  NumberInput,
  Checkbox,
  Tooltip,
} from '@carbon/react';
import { Information } from '@carbon/icons-react';
import { Slider } from '@/app/components/form/slider';
import { APiHeader } from '@/app/components/external-api/api-header';
import { AssistantMappingTable } from '@/app/components/tools/common';
import {
  ASSISTANT_CONDITION_KEY_OPTIONS,
  ASSISTANT_CONDITION_OPERATOR_OPTIONS,
  ASSISTANT_CONDITION_SOURCE_OPTIONS,
  ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY,
  normalizeAssistantConditionEntries,
} from '@/app/components/tools/common';
import {
  GetAssistantWebhook,
  GetAssistantWebhookRequest,
  Metadata,
  UpdateAssistantWebhookRequest,
  UpdateWebhook,
} from '@rapidaai/react';
import { useCurrentCredential } from '@/hooks/use-credential';
import toast from 'react-hot-toast/headless';
import { useRapidaStore } from '@/hooks';
import { connectionConfig } from '@/configs';
import { TabForm } from '@/app/components/form/tab-form';
import { SourceConditionRule } from '@/app/components/conditions/source-condition-rule';

const webhookEvents = [
  {
    id: 'conversation.begin',
    name: 'conversation.begin',
    description: 'Triggered when a new conversation begins.',
  },
  {
    id: 'conversation.completed',
    name: 'conversation.completed',
    description: 'Triggered when a conversation ends successfully.',
  },
  {
    id: 'conversation.failed',
    name: 'conversation.failed',
    description: 'Triggered when a conversation fails.',
  },
];

const renderLabelWithTooltip = (label: string, tooltip: string) => (
  <span className="inline-flex items-center gap-1">
    {label}
    <Tooltip align="right" label={tooltip}>
      <Information size={14} />
    </Tooltip>
  </span>
);

type WebhookParameterType =
  | 'event'
  | 'assistant'
  | 'client'
  | 'conversation'
  | 'argument'
  | 'metadata'
  | 'option'
  | 'analysis'
  | 'custom';

const WEBHOOK_TYPE_OPTIONS = [
  { value: 'event', name: 'Event' },
  { value: 'assistant', name: 'Assistant' },
  { value: 'client', name: 'Client' },
  { value: 'conversation', name: 'Conversation' },
  { value: 'argument', name: 'Argument' },
  { value: 'metadata', name: 'Metadata' },
  { value: 'option', name: 'Option' },
  { value: 'analysis', name: 'Analysis' },
  { value: 'custom', name: 'Custom' },
];

const WEBHOOK_KEY_OPTIONS_BY_TYPE = {
  event: [
    { value: 'type', name: 'Type' },
    { value: 'data', name: 'Data' },
  ],
  assistant: [
    { value: 'id', name: 'ID' },
    { value: 'name', name: 'Name' },
    { value: 'version', name: 'Version' },
  ],
  client: [
    { value: 'phone', name: 'Phone' },
    { value: 'assistantPhone', name: 'Assistant Phone' },
    { value: 'direction', name: 'Direction' },
    { value: 'provider', name: 'Provider' },
    { value: 'providerCallId', name: 'Provider Call ID' },
  ],
  conversation: [
    { value: 'messages', name: 'Messages' },
    { value: 'id', name: 'ID' },
  ],
};

const getDefaultParameterKey = (type: WebhookParameterType): string => {
  switch (type) {
    case 'event':
      return 'type';
    case 'assistant':
      return 'id';
    case 'client':
      return 'phone';
    case 'conversation':
      return 'messages';
    default:
      return '';
  }
};

const getWebhookOptionMap = (webhook: any): Map<string, string> => {
  const map = new Map<string, string>();
  const options = webhook?.getOptionsList?.() || [];
  options.forEach((option: any) => {
    const key = option?.getKey?.();
    const value = option?.getValue?.();
    if (key && typeof value === 'string') {
      map.set(key, value);
    }
  });
  return map;
};

const parseStringList = (raw?: string): string[] => {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.filter((item): item is string => typeof item === 'string');
    }
  } catch {}
  return [];
};

const parseStringMap = (raw?: string): Record<string, string> => {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.fromEntries(
        Object.entries(parsed)
          .filter(([, value]) => typeof value === 'string')
          .map(([key, value]) => [key, value as string]),
      );
    }
  } catch {}
  return {};
};

const WEBHOOK_OPTION_KEYS = {
  method: 'http_method',
  url: 'http_url',
  headers: 'http_headers',
  body: 'http_body',
  retryStatusCodes: 'retry_status_codes',
  maxRetryCount: 'max_retry_count',
  timeoutSeconds: 'timeout_seconds',
};
const WEBHOOK_CONDITION_HEADER = 'webhook.condition';
const DEFAULT_SOURCE_CONDITIONS = [
  {
    key: 'source',
    condition: '=',
    value: 'all',
  },
];

const toJsonMap = (rows: { key: string; value: string }[]) => {
  return JSON.stringify(
    rows.reduce<Record<string, string>>((acc, current) => {
      if (!current.key) {
        return acc;
      }
      acc[current.key] = current.value;
      return acc;
    }, {}),
  );
};

const buildWebhookOptions = ({
  method,
  endpoint,
  headers,
  parameterKeyValuePairs,
  retryOnStatus,
  maxRetries,
  requestTimeout,
  sourceConditions,
}: {
  method: string;
  endpoint: string;
  headers: { key: string; value: string }[];
  parameterKeyValuePairs: { key: string; value: string }[];
  retryOnStatus: string[];
  maxRetries: number;
  requestTimeout: number;
  sourceConditions: Array<{
    key: string;
    condition: string;
    value: string;
  }>;
}): Metadata[] => {
  const headerRows = [
    ...headers.filter(
      header => header.key.trim().toLowerCase() !== WEBHOOK_CONDITION_HEADER,
    ),
    {
      key: WEBHOOK_CONDITION_HEADER,
      value: JSON.stringify(sourceConditions),
    },
  ];
  return [
    { key: WEBHOOK_OPTION_KEYS.method, value: method || 'POST' },
    { key: WEBHOOK_OPTION_KEYS.url, value: endpoint || '' },
    { key: WEBHOOK_OPTION_KEYS.headers, value: toJsonMap(headerRows) },
    {
      key: WEBHOOK_OPTION_KEYS.body,
      value: toJsonMap(parameterKeyValuePairs),
    },
    {
      key: WEBHOOK_OPTION_KEYS.retryStatusCodes,
      value: JSON.stringify(retryOnStatus || []),
    },
    { key: WEBHOOK_OPTION_KEYS.maxRetryCount, value: String(maxRetries || 0) },
    {
      key: WEBHOOK_OPTION_KEYS.timeoutSeconds,
      value: String(requestTimeout || 0),
    },
  ].map(({ key, value }) => {
    const option = new Metadata();
    option.setKey(key);
    option.setValue(value);
    return option;
  });
};

export const UpdateAssistantWebhook: FC<{ assistantId: string }> = ({
  assistantId,
}) => {
  const navigator = useGlobalNavigation();
  const { webhookId } = useParams();
  const { authId, token, projectId } = useCurrentCredential();
  const { showDialog, ConfirmDialogComponent } = useConfirmDialog({});
  const { loading, showLoader, hideLoader } = useRapidaStore();

  const [activeTab, setActiveTab] = useState('destination');
  const [errorMessage, setErrorMessage] = useState('');

  const [method, setMethod] = useState('POST');
  const [endpoint, setEndpoint] = useState('');
  const [description, setDescription] = useState('');
  const [retryOnStatus, setRetryOnStatus] = useState<string[]>(['50X']);
  const [maxRetries, setMaxRetries] = useState(3);
  const [requestTimeout, setRequestTimeout] = useState(180);
  const [headers, setHeaders] = useState<{ key: string; value: string }[]>([]);
  const [sourceConditions, setSourceConditions] = useState<
    Array<{
      key: string;
      condition: string;
      value: string;
    }>
  >(DEFAULT_SOURCE_CONDITIONS);
  const [priority, setPriority] = useState<number>(0);
  const [parameters, setParameters] = useState<
    {
      type: WebhookParameterType;
      key: string;
      value: string;
    }[]
  >([]);
  const [events, setEvents] = useState<string[]>([]);

  useEffect(() => {
    const load = async () => {
      showLoader();
      const request = new GetAssistantWebhookRequest();
      request.setAssistantid(assistantId);
      request.setId(webhookId!);

      try {
        const res = await GetAssistantWebhook(connectionConfig, request, {
          'x-auth-id': authId,
          authorization: token,
          'x-project-id': projectId,
        });

        hideLoader();
        if (!res?.getData()) {
          toast.error('Unable to load webhook, please try again later.');
          return;
        }
        const wb = res.getData();
        if (wb) {
          const optionMap = getWebhookOptionMap(wb as any);
          const optionsRetryCount = Number(optionMap.get('max_retry_count') || '0');
          const optionsTimeout = Number(optionMap.get('timeout_seconds') || '0');

          setMethod(optionMap.get('http_method') || 'POST');
          setEndpoint(optionMap.get('http_url') || '');
          setDescription(wb.getDescription());
          setRetryOnStatus(
            parseStringList(optionMap.get('retry_status_codes')),
          );
          setMaxRetries(Number.isFinite(optionsRetryCount) ? optionsRetryCount : 0);
          setRequestTimeout(Number.isFinite(optionsTimeout) ? optionsTimeout : 0);
          setPriority(wb.getExecutionpriority());
          const optionsHeaders = parseStringMap(optionMap.get('http_headers'));
          const rawCondition =
            optionsHeaders[WEBHOOK_CONDITION_HEADER] || undefined;
          if (rawCondition) {
            try {
              setSourceConditions(
                normalizeAssistantConditionEntries(JSON.parse(rawCondition)),
              );
            } catch {
              setSourceConditions(DEFAULT_SOURCE_CONDITIONS);
            }
          } else {
            setSourceConditions(DEFAULT_SOURCE_CONDITIONS);
          }

          const filteredHeaders = Object.entries(optionsHeaders).filter(
            ([key]) => key.toLowerCase() !== WEBHOOK_CONDITION_HEADER,
          );
          setHeaders(
            filteredHeaders.map(([key, value]) => ({
              key,
              value,
            })),
          );
          const bodyMap = parseStringMap(optionMap.get('http_body'));
          setParameters(
            Object.entries(bodyMap).map(([key, value]) => {
              const [type, paramKey] = key.split('.');
              return {
                type: type as WebhookParameterType,
                key: paramKey,
                value,
              };
            }),
          );
          setEvents(wb.getAssistanteventsList());
        }
      } catch {
        hideLoader();
        toast.error('Unable to load webhook, please try again later.');
      }
    };

    load();
  }, [assistantId, webhookId, authId, token, projectId]);

  const validateDestination = (): boolean => {
    setErrorMessage('');
    if (!endpoint) {
      setErrorMessage('Please provide a server URL for the webhook.');
      return false;
    }
    if (!/^https?:\/\/.+/.test(endpoint)) {
      setErrorMessage('Please provide a valid server URL for the webhook.');
      return false;
    }
    return true;
  };

  const validatePayload = (): boolean => {
    setErrorMessage('');
    const headersMissingValue = headers.some(
      header => header.key.trim() !== '' && header.value.trim() === '',
    );
    if (headersMissingValue) {
      setErrorMessage('Headers with a key must also include a value.');
      return false;
    }
    if (parameters.length === 0) {
      setErrorMessage(
        'Please provide one or more parameters which can be passed as data to your server.',
      );
      return false;
    }
    const keys = parameters.map(param => `${param.type}.${param.key}`);
    const uniqueKeys = new Set(keys);
    if (keys.length !== uniqueKeys.size) {
      setErrorMessage('Duplicate parameter keys are not allowed.');
      return false;
    }
    const emptyKeysOrValues = parameters.filter(
      param => param.key.trim() === '' || param.value.trim() === '',
    );
    if (emptyKeysOrValues.length > 0) {
      setErrorMessage('Empty parameter keys or values are not allowed.');
      return false;
    }
    const values = parameters.map(param => param.value.trim());
    const uniqueValues = new Set(values);
    if (values.length !== uniqueValues.size) {
      setErrorMessage('Duplicate parameter values are not allowed.');
      return false;
    }
    return true;
  };

  const onSubmit = async () => {
    setErrorMessage('');
    if (events.length === 0) {
      setErrorMessage(
        'Please select at least one event when the webhook will get triggered.',
      );
      return;
    }
    showLoader();
    const parameterKeyValuePairs = parameters.map(param => ({
      key: `${param.type}.${param.key}`,
      value: param.value,
    }));
    const request = new UpdateAssistantWebhookRequest();
    request.setAssistantid(assistantId);
    request.setId(webhookId!);
    request.setAssistanteventsList(events);
    request.setExecutionpriority(priority);
    request.setDescription(description);
    request.setOptionsList(
      buildWebhookOptions({
        method,
        endpoint,
        headers,
        parameterKeyValuePairs,
        retryOnStatus,
        maxRetries,
        requestTimeout,
        sourceConditions,
      }),
    );

    try {
      const response = await UpdateWebhook(connectionConfig, request, {
        'x-auth-id': authId,
        authorization: token,
        'x-project-id': projectId,
      });

      hideLoader();
      if (response?.getSuccess()) {
        toast.success(`Assistant's webhook updated successfully`);
        navigator.goToAssistantWebhook(assistantId);
        return;
      }
      if (response?.getError()) {
        const message = response.getError()?.getHumanmessage();
        if (message) {
          setErrorMessage(message);
          return;
        }
      }
      setErrorMessage(
        'Unable to update assistant webhook, please check and try again.',
      );
    } catch {
      hideLoader();
      setErrorMessage(
        'Unable to update assistant webhook, please check and try again.',
      );
    }
  };

  return (
    <>
      <ConfirmDialogComponent />
      <TabForm
        formHeading="Update all steps to reconfigure your webhook."
        activeTab={activeTab}
        onChangeActiveTab={() => {}}
        errorMessage={errorMessage}
        form={[
          {
            code: 'destination',
            name: 'Destination',
            description:
              'Configure the HTTP endpoint that will receive webhook events.',
            actions: [
              <ButtonSet className="!w-full [&>button]:!flex-1 [&>button]:!max-w-none">
                <SecondaryButton
                  size="lg"
                  onClick={() => showDialog(navigator.goBack)}
                >
                  Cancel
                </SecondaryButton>
                <PrimaryButton
                  size="lg"
                  onClick={() => {
                    if (validateDestination() && validatePayload())
                      setActiveTab('events');
                  }}
                >
                  Continue
                </PrimaryButton>
              </ButtonSet>,
            ],
            body: (
              <div className="pb-8 flex flex-col">
                <InputGroup title="Condition">
                  <SourceConditionRule
                    conditions={sourceConditions}
                    onChangeConditions={setSourceConditions}
                    conditionOptions={ASSISTANT_CONDITION_OPERATOR_OPTIONS}
                    sourceOptions={ASSISTANT_CONDITION_SOURCE_OPTIONS}
                    keyOptions={ASSISTANT_CONDITION_KEY_OPTIONS}
                    valueOptionsByKey={ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY}
                    keyTooltipText="The variable to evaluate before triggering this webhook."
                  />
                </InputGroup>

                <InputGroup
                  title={renderLabelWithTooltip(
                    'Destination',
                    'Configure the HTTP destination that receives the webhook request.',
                  )}
                >
                  <Stack gap={6}>
                    <div className="flex gap-2">
                      <div className="w-36 shrink-0">
                        <CarbonSelect
                          id="webhook-method"
                          labelText="Method"
                          value={method}
                          onChange={e => setMethod(e.target.value)}
                        >
                          <SelectItem value="POST" text="POST" />
                          <SelectItem value="PUT" text="PUT" />
                          <SelectItem value="PATCH" text="PATCH" />
                        </CarbonSelect>
                      </div>
                      <div className="flex-1">
                        <TextInput
                          id="webhook-endpoint"
                          labelText="Server URL"
                          value={endpoint}
                          onChange={e => setEndpoint(e.target.value)}
                          placeholder="https://your-domain.com/webhook"
                        />
                      </div>
                    </div>
                    <TextArea
                      id="webhook-description"
                      labelText="Description (Optional)"
                      value={description}
                      onChange={e => setDescription(e.target.value)}
                      placeholder="An optional description of this webhook destination..."
                      rows={2}
                    />
                  </Stack>
                </InputGroup>
                <InputGroup
                  title={renderLabelWithTooltip(
                    `Headers (${headers.length})`,
                    'HTTP headers included with every webhook request.',
                  )}
                >
                  <APiHeader headers={headers} setHeaders={setHeaders} />
                </InputGroup>

                <InputGroup
                  title={renderLabelWithTooltip(
                    `Payload Mapping (${parameters.length})`,
                    'Map assistant, client, event, and conversation values into the webhook request body.',
                  )}
                  childClass="space-y-4"
                >
                  <AssistantMappingTable
                    parameters={parameters}
                    onChange={setParameters}
                    typeOptions={WEBHOOK_TYPE_OPTIONS}
                    getDefaultParameterKey={type =>
                      getDefaultParameterKey(type as WebhookParameterType)
                    }
                    keyOptionsByType={WEBHOOK_KEY_OPTIONS_BY_TYPE}
                    includeEmptyKeyOption
                    resetValueOnTypeChange
                    createNewParameter={() => ({
                      type: 'assistant',
                      key: 'id',
                      value: '',
                    })}
                    title="Payload Mapping"
                    addButtonLabel="Add parameter"
                    valuePlaceholder="Value"
                    removeButtonKind="danger--ghost"
                  />
                </InputGroup>
              </div>
            ),
          },
          {
            code: 'events',
            name: 'Events & Settings',
            description:
              'Choose which events trigger the webhook and configure retry behavior.',
            actions: [
              <ButtonSet className="!w-full [&>button]:!flex-1 [&>button]:!max-w-none">
                <SecondaryButton
                  size="lg"
                  onClick={() => showDialog(navigator.goBack)}
                >
                  Cancel
                </SecondaryButton>
                <PrimaryButton size="lg" isLoading={loading} onClick={onSubmit}>
                  Update webhook
                </PrimaryButton>
              </ButtonSet>,
            ],
            body: (
              <div className="pb-8 flex flex-col">
                <InputGroup
                  title={renderLabelWithTooltip(
                    'Events',
                    'Choose which assistant lifecycle events trigger this webhook.',
                  )}
                >
                  <MultiSelect
                    id="webhook-events"
                    titleText="Select events"
                    label="Select events"
                    items={webhookEvents}
                    selectedItems={webhookEvents.filter(event =>
                      events.includes(event.id),
                    )}
                    itemToString={item => item?.name || ''}
                    onChange={({ selectedItems }) =>
                      setEvents((selectedItems || []).map(event => event.id))
                    }
                    helperText="Select which assistant lifecycle events should send a webhook."
                  />
                </InputGroup>

                <div className="grid lg:grid-cols-2">
                  <InputGroup
                    title={renderLabelWithTooltip(
                      'Retry',
                      'Control how the webhook retries after failed responses.',
                    )}
                    childClass="space-y-4"
                  >
                    <Stack gap={5}>
                      <div className="max-w-xs">
                        <CarbonSelect
                          id="webhook-max-retries"
                          labelText={renderLabelWithTooltip(
                            'Max retry count',
                            'Maximum number of retry attempts after a matching failure response.',
                          )}
                          value={maxRetries.toString()}
                          onChange={e =>
                            setMaxRetries(parseInt(e.target.value))
                          }
                        >
                          <SelectItem value="1" text="1" />
                          <SelectItem value="2" text="2" />
                          <SelectItem value="3" text="3" />
                        </CarbonSelect>
                      </div>
                      <div className="flex flex-wrap gap-4">
                        {['40X', '50X'].map(status => (
                          <Checkbox
                            key={status}
                            id={`retry-status-${status}`}
                            labelText={status}
                            checked={retryOnStatus.includes(status)}
                            onChange={(_, { checked }) => {
                              if (checked) {
                                setRetryOnStatus([...retryOnStatus, status]);
                              } else {
                                setRetryOnStatus(
                                  retryOnStatus.filter(s => s !== status),
                                );
                              }
                            }}
                          />
                        ))}
                      </div>
                    </Stack>
                  </InputGroup>

                  <div className="grid gap-6">
                    <InputGroup
                      title={renderLabelWithTooltip(
                        'Timeout',
                        'Set how long the webhook waits before the request times out.',
                      )}
                      childClass="space-y-4"
                    >
                      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
                        <Slider
                          min={180}
                          max={300}
                          step={1}
                          value={requestTimeout}
                          onSlide={value => setRequestTimeout(value)}
                          className="w-full sm:flex-1"
                        />
                        <NumberInput
                          id="webhook-timeout"
                          hideLabel
                          label={renderLabelWithTooltip(
                            'Timeout (seconds)',
                            'Webhook request timeout in seconds.',
                          )}
                          min={180}
                          max={300}
                          step={1}
                          value={requestTimeout}
                          onChange={(e: any, { value }: any) =>
                            setRequestTimeout(Number(value))
                          }
                          className="!w-full sm:!w-24"
                        />
                      </div>

                      <div className="max-w-[12rem]">
                        <NumberInput
                          id="webhook-priority"
                          label={renderLabelWithTooltip(
                            'Priority',
                            'Execution order when multiple webhooks trigger at the same time.',
                          )}
                          min={0}
                          value={priority}
                          onChange={(e: any, { value }: any) =>
                            setPriority(Number(value))
                          }
                          helperText="Lower numbers execute first when multiple webhooks are triggered."
                        />
                      </div>
                    </InputGroup>
                  </div>
                </div>
              </div>
            ),
          },
        ]}
      />
    </>
  );
};
