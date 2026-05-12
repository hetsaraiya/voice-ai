import { FC, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';
import {
  Breadcrumb,
  BreadcrumbItem,
  ButtonSet,
  CheckboxGroup,
  Slider,
  Select as CarbonSelect,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
} from '@carbon/react';
import { TextInput, Stack } from '@/app/components/carbon/form';
import { Notification } from '@/app/components/carbon/notification';
import { InputCheckbox } from '@/app/components/carbon/form/input-checkbox';
import { PrimaryButton, SecondaryButton } from '@/app/components/carbon/button';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { useConfirmDialog } from '@/app/pages/assistant/actions/hooks/use-confirmation';
import { InputGroup } from '@/app/components/input-group';
import { useCurrentCredential } from '@/hooks/use-credential';
import toast from 'react-hot-toast/headless';
import { APiStringHeader } from '@/app/components/external-api/api-header';
import {
  ASSISTANT_CONDITION_KEY_OPTIONS,
  ASSISTANT_CONDITION_OPERATOR_OPTIONS,
  ASSISTANT_CONDITION_SOURCE_OPTIONS,
  ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY,
  ParameterEditor,
  normalizeAssistantConditionEntries,
} from '@/app/components/tools/common';
import { connectionConfig } from '@/configs';
import { SourceConditionRule } from '@/app/components/conditions/source-condition-rule';
import {
    AssistantAuthentication,
  CreateAssistantAuthentication,
  CreateAssistantAuthenticationRequest,
  DisableAssistantAuthentication,
  DisableAssistantAuthenticationRequest,
  GetAssistantAuthentication,
  GetAssistantAuthenticationRequest,
  Metadata,
} from '@rapidaai/react';
import { Renew, Add, Edit, DisableStep } from '@carbon/icons-react';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { IconOnlyButton } from '@/app/components/carbon/button';
import { SectionLoader } from '@/app/components/loader/section-loader';
import { TableSection } from '@/app/components/sections/table-section';
import { toHumanReadableDateTime } from '@/utils/date';
import { CarbonShapeIndicator } from '@/app/components/carbon/shape-indicator';

type HttpMethod = 'POST' | 'GET';
type FailBehavior = 'block' | 'do_nothing';
type LoadState = 'loading' | 'ready' | 'error';
type FormMode = 'create' | 'edit';

const AUTH_OPTION_METHOD = 'http_method';
const AUTH_OPTION_ENDPOINT = 'http_url';
const AUTH_OPTION_HEADERS = 'http_headers';
const AUTH_OPTION_BODY = 'http_body';
const AUTH_OPTION_CONDITION = 'authentication.condition';
const FAIL_BEHAVIOR_BLOCK = 'BLOCK';
const FAIL_BEHAVIOR_DO_NOTHING = 'DO_NOTHING';

const AUTH_PARAMETER_TYPE_OPTIONS = [
  { value: 'client', name: 'Client' },
  { value: 'assistant', name: 'Assistant' },
  { value: 'conversation', name: 'Conversation' },
  { value: 'argument', name: 'Argument' },
  { value: 'metadata', name: 'Metadata' },
  { value: 'option', name: 'Option' },
  { value: 'custom', name: 'Custom' },
];

const AUTH_KEY_OPTIONS_BY_TYPE = {
  assistant: [
    { value: 'id', name: 'ID' },
    { value: 'name', name: 'Name' },
    { value: 'prompt', name: 'Prompt' },
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

const fromApiFailBehavior = (value?: string): FailBehavior => {
  const normalized = (value || '').trim().toLowerCase();
  if (
    normalized === 'do_nothing' ||
    normalized === 'do-nothing' ||
    normalized === 'none'
  ) {
    return 'do_nothing';
  }
  return 'block';
};

const toApiFailBehavior = (value: FailBehavior): string =>
  value === 'do_nothing' ? FAIL_BEHAVIOR_DO_NOTHING : FAIL_BEHAVIOR_BLOCK;

const toOptionMap = (options: Metadata[] = []) =>
  options.reduce(
    (acc, opt) => {
      acc[opt.getKey()] = opt.getValue();
      return acc;
    },
    {} as Record<string, string>,
  );

const getStatus = (data: any) => (data?.getStatus?.() || '').toLowerCase();

export function ConfigureAssistantAuthenticationPage() {
  const { assistantId } = useParams();
  return (
    <>
      {assistantId && <ConfigureAssistantAuthentication assistantId={assistantId} />}
    </>
  );
}

export function CreateAssistantAuthenticationPage() {
  const { assistantId } = useParams();
  return (
    <>
      {assistantId && <AuthenticationForm assistantId={assistantId} mode="create" />}
    </>
  );
}

export function UpdateAssistantAuthenticationPage() {
  const { assistantId } = useParams();
  return (
    <>
      {assistantId && <AuthenticationForm assistantId={assistantId} mode="edit" />}
    </>
  );
}

const ConfigureAssistantAuthentication: FC<{ assistantId: string }> = ({
  assistantId,
}) => {
  const navigator = useGlobalNavigation();
  const { authId, token, projectId } = useCurrentCredential();
  const { showDialog, ConfirmDialogComponent } = useConfirmDialog({
    title: 'Disable authentication?',
    content: 'Authentication will be inactive for this assistant.',
  });

  const [loading, setLoading] = useState(true);
  const [authentication, setAuthentication] = useState<AssistantAuthentication | null>(null);

  const load = () => {
    setLoading(true);
    const request = new GetAssistantAuthenticationRequest();
    request.setAssistantid(assistantId);

    GetAssistantAuthentication(connectionConfig, request, {
      'x-auth-id': authId,
      authorization: token,
      'x-project-id': projectId,
    })
      .then(response => {
        if (!response?.getSuccess()) {
          setAuthentication(null);
          setLoading(false);
          return;
        }
        const data = response.getData();
        if (data) {
          setAuthentication(data);
        }
        setLoading(false);
      })
      .catch(() => {
        setAuthentication(null);
        setLoading(false);
      });
  };

  useEffect(() => {
    load();
  }, [assistantId, authId, token, projectId]);

  const optionMap = useMemo(
    () => toOptionMap(authentication?.getOptionsList?.() || []),
    [authentication],
  );

  const onDisable = () => {
    if (!authentication) return;
    const request = new DisableAssistantAuthenticationRequest();
    request.setAssistantid(assistantId);
    DisableAssistantAuthentication(connectionConfig, request, {
      'x-auth-id': authId,
      authorization: token,
      'x-project-id': projectId,
    })
      .then(response => {
        if (response?.getSuccess()) {
          toast.success('Assistant authentication disabled successfully.');
          load();
          return;
        }
        toast.error(
          response?.getError?.()?.getHumanmessage?.() ||
            'Unable to disable assistant authentication.',
        );
      })
      .catch(err => {
        toast.error(
          err?.message || 'Unable to disable assistant authentication.',
        );
      });
  };

  if (loading) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center">
        <SectionLoader />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col flex-1">
      <ConfirmDialogComponent />
      <div className="px-4 pt-4 pb-6 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <div>
          <Breadcrumb noTrailingSlash className="mb-2">
            <BreadcrumbItem href={`/deployment/assistant/${assistantId}/overview`}>
              Assistant
            </BreadcrumbItem>
          </Breadcrumb>
          <h1 className="text-2xl font-light tracking-tight">Authentication</h1>
        </div>
      </div>

      <TableToolbar>
        <TableToolbarContent>
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={load}
          />
          <PrimaryButton
            size="md"
            renderIcon={Add}
            onClick={() =>
              navigator.goTo(
                `/deployment/assistant/${assistantId}/configure-authentication/${
                  authentication ? 'edit' : 'create'
                }`,
              )
            }
          >
            {authentication ? 'Configure authentication' : 'Create authentication'}
          </PrimaryButton>
        </TableToolbarContent>
      </TableToolbar>

      <TableSection>
        {authentication ? (
          <Table>
            <TableHead>
              <TableRow>
                <TableHeader>Provider Type</TableHeader>
                <TableHeader>Method</TableHeader>
                <TableHeader>URL</TableHeader>
                <TableHeader>Status</TableHeader>
                <TableHeader>Date</TableHeader>
                <TableHeader>Actions</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              <TableRow>
                <TableCell className="text-sm whitespace-nowrap">HTTP</TableCell>
                <TableCell className="text-sm whitespace-nowrap">
                  {optionMap[AUTH_OPTION_METHOD] || '-'}
                </TableCell>
                <TableCell className="text-sm max-w-[360px] truncate">
                  {optionMap[AUTH_OPTION_ENDPOINT] || '-'}
                </TableCell>
                <TableCell className="text-sm whitespace-nowrap">
                  <CarbonShapeIndicator
                    state={authentication.getStatus()}
                    textSize={14}
                  />
                </TableCell>
                <TableCell className="text-[13px] whitespace-nowrap">
                  {authentication.getCreateddate() && toHumanReadableDateTime(authentication.getCreateddate()!)}
                </TableCell>
                <TableCell
                  className="text-sm whitespace-nowrap"
                  onClick={e => e.stopPropagation()}
                >
                  <div className="flex items-center gap-0">
                    <IconOnlyButton
                      kind="ghost"
                      size="md"
                      renderIcon={Edit}
                      iconDescription="Configure authentication"
                      onClick={() =>
                        navigator.goTo(
                          `/deployment/assistant/${assistantId}/configure-authentication/edit`,
                        )
                      }
                    />
                    <IconOnlyButton
                      size="md"
                      kind='ghost'
                      renderIcon={DisableStep}
                      iconDescription="Disable authentication"
                      disabled={getStatus(authentication) !== 'active'}
                      onClick={() => showDialog(onDisable)}
                    />
                  </div>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        ) : (
          <EmptyState
            icon={Add}
            title="No authentication configured"
            subtitle="Create authentication to verify sessions before initialization."
           
          />
        )}
      </TableSection>
    </div>
  );
};

const AuthenticationForm: FC<{ assistantId: string; mode: FormMode }> = ({
  assistantId,
  mode,
}) => {
  const navigator = useGlobalNavigation();
  const { showDialog, ConfirmDialogComponent } = useConfirmDialog({});
  const { authId, token, projectId } = useCurrentCredential();

  const [enabled, setEnabled] = useState(false);
  const [endpoint, setEndpoint] = useState('');
  const [method, setMethod] = useState<HttpMethod>('POST');
  const [timeout, setTimeoutValue] = useState(5000);
  const [failBehavior, setFailBehavior] = useState<FailBehavior>('block');
  const [headers, setHeaders] = useState('{}');
  const [body, setBody] = useState(
    '{"assistant.id":"assistantId","client.phone":"clientPhone"}',
  );
  const [errorMessage, setErrorMessage] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [loadState, setLoadState] = useState<LoadState>('loading');
  const [sourceConditions, setSourceConditions] = useState([
    {
      key: 'source',
      condition: '=',
      value: 'all',
    },
  ]);

  useEffect(() => {
    const request = new GetAssistantAuthenticationRequest();
    request.setAssistantid(assistantId);

    GetAssistantAuthentication(connectionConfig, request, {
      'x-auth-id': authId,
      authorization: token,
      'x-project-id': projectId,
    })
      .then(response => {
        if (!response?.getSuccess()) {
          if (mode === 'edit') {
            setLoadState('error');
            setErrorMessage(
              response?.getError?.()?.getHumanmessage?.() ||
                'Unable to load assistant authentication. Please try again.',
            );
            return;
          }
          setLoadState('ready');
          return;
        }
        const data = response.getData();
        if (!data) {
          if (mode === 'edit') {
            setLoadState('error');
            setErrorMessage(
              'Unable to load assistant authentication. Please try again.',
            );
            return;
          }
          setLoadState('ready');
          return;
        }

        const status = (data.getStatus() || '').toLowerCase();
        setEnabled(status === 'active');
        setFailBehavior(fromApiFailBehavior(data.getFailbehavior()));

        const persistedTimeout = Number(data.getTimeoutms());
        setTimeoutValue(
          Number.isFinite(persistedTimeout) && persistedTimeout > 0
            ? persistedTimeout
            : 5000,
        );

        const optionMap = toOptionMap(data.getOptionsList() || []);

        const persistedMethod = optionMap[AUTH_OPTION_METHOD];
        if (persistedMethod === 'POST' || persistedMethod === 'GET') {
          setMethod(persistedMethod);
        }

        if (optionMap[AUTH_OPTION_ENDPOINT]) {
          setEndpoint(optionMap[AUTH_OPTION_ENDPOINT]);
        }
        if (optionMap[AUTH_OPTION_HEADERS]) {
          setHeaders(optionMap[AUTH_OPTION_HEADERS]);
        }
        if (optionMap[AUTH_OPTION_BODY]) {
          setBody(optionMap[AUTH_OPTION_BODY]);
        }
        if (optionMap[AUTH_OPTION_CONDITION]) {
          try {
            setSourceConditions(
              normalizeAssistantConditionEntries(
                JSON.parse(optionMap[AUTH_OPTION_CONDITION]),
              ),
            );
          } catch {
            setSourceConditions([
              { key: 'source', condition: '=', value: 'all' },
            ]);
          }
        }
        setLoadState('ready');
      })
      .catch(() => {
        setLoadState('error');
        setErrorMessage(
          'Unable to load assistant authentication. Please try again.',
        );
      });
  }, [assistantId, authId, token, projectId, mode]);

  const validateConfigure = (): boolean => {
    setErrorMessage('');
    if (!enabled) return true;

    if (!endpoint.trim()) {
      setErrorMessage('Please provide a server URL for authentication.');
      return false;
    }

    if (!/^https?:\/\/.+/.test(endpoint.trim())) {
      setErrorMessage('Please provide a valid server URL for authentication.');
      return false;
    }

    let parsedHeaders: unknown;
    try {
      parsedHeaders = JSON.parse(headers || '{}');
    } catch {
      setErrorMessage('Please provide valid values for headers key and value.');
      return false;
    }

    if (
      typeof parsedHeaders !== 'object' ||
      parsedHeaders === null ||
      Array.isArray(parsedHeaders)
    ) {
      setErrorMessage('Please provide valid values for headers key and value.');
      return false;
    }

    const invalidHeader = Object.entries(
      parsedHeaders as Record<string, unknown>,
    ).some(
      ([key, value]) =>
        !key.trim() || typeof value !== 'string' || !value.trim(),
    );
    if (invalidHeader) {
      setErrorMessage('Please provide valid values for headers key and value.');
      return false;
    }

    let parsedBody: unknown;
    try {
      parsedBody = JSON.parse(body || '{}');
    } catch {
      setErrorMessage(
        'Please provide valid values for parameters key and value.',
      );
      return false;
    }

    if (
      typeof parsedBody !== 'object' ||
      parsedBody === null ||
      Array.isArray(parsedBody)
    ) {
      setErrorMessage(
        'Please provide valid values for parameters key and value.',
      );
      return false;
    }

    const bodyEntries = Object.entries(parsedBody as Record<string, unknown>);
    if (bodyEntries.length === 0) {
      setErrorMessage(
        'Please provide one or more parameters for authentication.',
      );
      return false;
    }

    const invalidBodyEntry = bodyEntries.some(
      ([key, value]) =>
        !key.trim() || typeof value !== 'string' || !value.trim(),
    );
    if (invalidBodyEntry) {
      setErrorMessage(
        'Please provide valid values for parameters key and value.',
      );
      return false;
    }

    return true;
  };

  const onSubmit = () => {
    if (loadState !== 'ready') return;
    if (!validateConfigure()) return;
    setIsSaving(true);

    if (!enabled) {
      const request = new DisableAssistantAuthenticationRequest();
      request.setAssistantid(assistantId);
      DisableAssistantAuthentication(connectionConfig, request, {
        'x-auth-id': authId,
        authorization: token,
        'x-project-id': projectId,
      })
        .then(response => {
          if (response?.getSuccess()) {
            toast.success('Assistant authentication disabled successfully.');
            navigator.goTo(
              `/deployment/assistant/${assistantId}/configure-authentication`,
            );
            return;
          }
          setErrorMessage(
            response?.getError()?.getHumanmessage() ||
              'Unable to disable assistant authentication.',
          );
        })
        .catch(err => {
          setErrorMessage(
            err?.message || 'Unable to disable assistant authentication.',
          );
        })
        .finally(() => setIsSaving(false));
      return;
    }

    const request = new CreateAssistantAuthenticationRequest();
    request.setAssistantid(assistantId);
    request.setProvider('http');
    request.setStatus('ACTIVE');
    request.setFailbehavior(toApiFailBehavior(failBehavior));
    request.setTimeoutms(String(timeout));

    const options: Metadata[] = [];
    const addOption = (key: string, value: string) => {
      const metadata = new Metadata();
      metadata.setKey(key);
      metadata.setValue(value);
      options.push(metadata);
    };

    addOption(AUTH_OPTION_METHOD, method);
    addOption(AUTH_OPTION_ENDPOINT, endpoint.trim());
    addOption(AUTH_OPTION_HEADERS, headers || '{}');
    addOption(AUTH_OPTION_BODY, body || '{}');
    addOption(AUTH_OPTION_CONDITION, JSON.stringify(sourceConditions));
    request.setOptionsList(options);

    CreateAssistantAuthentication(connectionConfig, request, {
      'x-auth-id': authId,
      authorization: token,
      'x-project-id': projectId,
    })
      .then(response => {
        if (response?.getSuccess()) {
          toast.success('Assistant authentication saved successfully.');
          navigator.goTo(
            `/deployment/assistant/${assistantId}/configure-authentication`,
          );
          return;
        }
        setErrorMessage(
          response?.getError()?.getHumanmessage() ||
            'Unable to save assistant authentication.',
        );
      })
      .catch(err => {
        setErrorMessage(
          err?.message || 'Unable to save assistant authentication.',
        );
      })
      .finally(() => setIsSaving(false));
  };

  return (
    <>
      <ConfirmDialogComponent />
      <div className="flex flex-col flex-1 min-h-0 bg-white dark:bg-gray-900">
        <div className="px-4 pt-4 pb-6 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
          <div>
            <Breadcrumb noTrailingSlash className="mb-2">
              <BreadcrumbItem href={`/deployment/assistant/${assistantId}/overview`}>
                Assistant
              </BreadcrumbItem>
            </Breadcrumb>
            <h1 className="text-2xl font-light tracking-tight">
              {mode === 'create' ? 'Create Authentication' : 'Authentication'}
            </h1>
          </div>
        </div>

        <div className="flex-1 min-h-0 overflow-auto">
          <div className="p-6">
            <CheckboxGroup
              legendText=""
              warn
              warnText={
                enabled
                  ? 'All sessions must be verified before initialization.'
                  : 'Authentication is disabled. Sessions will continue without verification.'
              }
            >
              <InputCheckbox
                id="assistant-auth-enabled"
                checked={enabled}
                disabled={isSaving || loadState !== 'ready'}
                onChange={e => {
                  setEnabled(e.target.checked);
                  setErrorMessage('');
                }}
              >
                Enable Session Authentication
              </InputCheckbox>
            </CheckboxGroup>
          </div>

          {enabled && (
            <>
              <InputGroup title="Condition">
                <SourceConditionRule
                  conditions={sourceConditions}
                  onChangeConditions={setSourceConditions}
                  conditionOptions={ASSISTANT_CONDITION_OPERATOR_OPTIONS}
                  sourceOptions={ASSISTANT_CONDITION_SOURCE_OPTIONS}
                  keyOptions={ASSISTANT_CONDITION_KEY_OPTIONS}
                  valueOptionsByKey={ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY}
                  keyTooltipText="The variable to evaluate for this condition."
                />
              </InputGroup>
              <InputGroup title="Definition">
                <Stack gap={7}>
                  <div className="flex space-x-2">
                    <div className="relative w-40">
                      <CarbonSelect
                        id="assistant-auth-method"
                        labelText="Method"
                        value={method}
                        onChange={e => {
                          setMethod(e.target.value as HttpMethod);
                          setErrorMessage('');
                        }}
                      >
                        <SelectItem value="POST" text="POST" />
                        <SelectItem value="GET" text="GET" />
                      </CarbonSelect>
                    </div>
                    <div className="relative w-full">
                      <TextInput
                        id="assistant-auth-endpoint"
                        labelText="Server URL"
                        value={endpoint}
                        onChange={e => {
                          setEndpoint(e.target.value);
                          setErrorMessage('');
                        }}
                        placeholder="https://auth.example.com/resolve"
                      />
                    </div>
                  </div>

                  <div className="flex space-x-2">
                    <div className="relative w-40">
                      <CarbonSelect
                        id="assistant-auth-fail-behavior"
                        labelText="On Error"
                        value={failBehavior}
                        onChange={e => {
                          setFailBehavior(e.target.value as FailBehavior);
                          setErrorMessage('');
                        }}
                      >
                        <SelectItem value="block" text="Block" />
                        <SelectItem value="do_nothing" text="Do nothing" />
                      </CarbonSelect>
                    </div>
                    <div className="relative w-full">
                      <Slider
                        id="assistant-auth-timeout"
                        labelText="Timeout (ms)"
                        value={timeout}
                        min={500}
                        max={10000}
                        step={100}
                        onChange={(data: { value: number | number[] }) => {
                          setTimeoutValue(
                            Array.isArray(data.value)
                              ? data.value[0]
                              : data.value,
                          );
                          setErrorMessage('');
                        }}
                      />
                    </div>
                  </div>

                  <div>
                    <p className="text-xs font-medium mb-2">Headers</p>
                    <APiStringHeader
                      headerValue={headers}
                      setHeaderValue={value => {
                        setHeaders(value);
                        setErrorMessage('');
                      }}
                    />
                  </div>

                  <ParameterEditor
                    value={body}
                    onChange={value => {
                      setBody(value);
                      setErrorMessage('');
                    }}
                    typeOptions={AUTH_PARAMETER_TYPE_OPTIONS}
                    keyOptionsByType={AUTH_KEY_OPTIONS_BY_TYPE}
                    includeEmptyKeyOption
                  />
                </Stack>
              </InputGroup>
            </>
          )}
        </div>

        <div className="shrink-0 w-full">
          {errorMessage && (
            <Notification kind="error" title="Error" subtitle={errorMessage} />
          )}
          <ButtonSet className="!w-full [&>button]:!flex-1 [&>button]:!max-w-none">
            <SecondaryButton
              size="lg"
              onClick={() =>
                showDialog(() =>
                  navigator.goTo(
                    `/deployment/assistant/${assistantId}/configure-authentication`,
                  ),
                )
              }
            >
              Cancel
            </SecondaryButton>
            <PrimaryButton
              size="lg"
              onClick={onSubmit}
              isLoading={isSaving}
              disabled={loadState !== 'ready'}
            >
              Save authentication
            </PrimaryButton>
          </ButtonSet>
        </div>
      </div>
    </>
  );
};
