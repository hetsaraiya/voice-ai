import {
  PrimaryButton,
  GhostButton,
  IconOnlyButton,
} from '@/app/components/carbon/button';
import { Dropdown } from '@/app/components/carbon/dropdown';
import { Form, Stack, TextInput } from '@/app/components/carbon/form';
import { ArrowLeft, PhoneOutgoing } from '@carbon/icons-react';
import { Notification } from '@/app/components/carbon/notification';
import { Tabs } from '@/app/components/carbon/tabs';
import { Text } from '@/app/components/carbon/text';
import {
  ArgumentList,
  ConfigEmpty,
  ConfigBlock,
  DebuggerTabHeader,
  InfoRow,
  PreviewAgentHeader,
  VoiceAgent,
} from '@/app/pages/preview-agent/voice-agent/voice-agent';
import {
  PHONE_COUNTRIES,
  DEFAULT_COUNTRY,
  Country,
} from '@/app/pages/preview-agent/voice-agent/phone-agent-constants';
import { CONFIG } from '@/configs';
import { useCurrentCredential } from '@/hooks/use-credential';
import { randomMeaningfullName } from '@/utils';
import { getStatusMetric } from '@/utils/metadata';
import {
  AgentConfig,
  Channel,
  ConnectionConfig,
  InputOptions,
  StringToAny,
  CreatePhoneCall,
  AssistantDefinition,
  CreatePhoneCallRequest,
  Assistant,
  GetAssistant,
  GetAssistantRequest,
  Variable,
} from '@rapidaai/react';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Navigate, useParams, useSearchParams } from 'react-router-dom';

/**
 *
 * @returns
 */
export const PublicPreviewVoiceAgent = () => {
  const [searchParams] = useSearchParams();
  const { assistantId } = useParams();
  const authId = searchParams.get('authId');
  const token = searchParams.get('token');

  if (!assistantId || !token) {
    return <Navigate to="/404" replace />;
  }

  return (
    <VoiceAgent
      debug={false}
      connectConfig={ConnectionConfig.DefaultConnectionConfig(
        ConnectionConfig.WithSDK({
          ApiKey: token,
          UserId: '' + (authId || 'public_user'),
        }),
      ).withCustomEndpoint(CONFIG.connection)}
      agentConfig={new AgentConfig(
        assistantId,
        new InputOptions([Channel.Audio, Channel.Text], Channel.Text),
      )
        .addMetadata('authId', StringToAny('' + (authId || 'public_user')))
        .setUserIdentifier(authId || randomMeaningfullName('public'))}
    />
  );
};

//
export const PreviewVoiceAgent = () => {
  const { user, authId, token, projectId } = useCurrentCredential();
  const { assistantId } = useParams();

  if (!assistantId || !user?.name) {
    return <Navigate to="/404" replace />;
  }

  return (
    <VoiceAgent
      debug={true}
      connectConfig={ConnectionConfig.DefaultConnectionConfig(
        ConnectionConfig.WithDebugger({
          authorization: token,
          userId: authId,
          projectId: projectId,
        }),
      ).withCustomEndpoint(CONFIG.connection)}
      agentConfig={new AgentConfig(
        assistantId,
        new InputOptions([Channel.Audio, Channel.Text], Channel.Text),
      )
        .setUserIdentifier(authId, user.name)
        .addKeywords([user.name])
        .addMetadata('authId', StringToAny(authId))
        .addMetadata('projectId', StringToAny(projectId))}
      // .addCustomOption('listen.language', StringToAny('en'))
      // .addCustomOption('speak.language', StringToAny('en'))
      // .addCustomOption('listen.model', StringToAny('nova-3'))}
    />
  );
};

// ---------------------------------------------------------------------------
// Phone Agent
// ---------------------------------------------------------------------------

type PhoneCallStatus = 'idle' | 'calling' | 'success' | 'failed';
type PhoneDebugTab = 'configuration' | 'arguments';
const PHONE_DEBUG_TABS: PhoneDebugTab[] = ['configuration', 'arguments'];
const PHONE_DEBUG_TAB_LABELS = ['Configuration', 'Arguments'];

//
export const PreviewPhoneAgent = () => {
  const { authId, token, projectId } = useCurrentCredential();
  const connectionCfg = useMemo(
    () =>
      ConnectionConfig.DefaultConnectionConfig(
        ConnectionConfig.WithPersonalToken({
          Authorization: token,
          AuthId: authId,
          ProjectId: projectId,
        }),
      ).withCustomEndpoint(CONFIG.connection),
    [authId, projectId, token],
  );

  const { assistantId } = useParams();
  const [assistant, setAssistant] = useState<Assistant | null>(null);
  const [variables, setVariables] = useState<Variable[]>([]);
  const [country, setCountry] = useState<Country>(DEFAULT_COUNTRY);
  const [phoneNumber, setPhoneNumber] = useState('');
  const [callStatus, setCallStatus] = useState<PhoneCallStatus>('idle');
  const [errorMessage, setErrorMessage] = useState('');
  const [argumentMap, setArgumentMap] = useState<Map<string, string>>(
    new Map(),
  );

  const onChangeArgument = useCallback((k: string, vl: string) => {
    setArgumentMap(prev => {
      const m = new Map(prev);
      m.set(k, vl);
      return m;
    });
  }, []);

  useEffect(() => {
    if (!assistantId) return;
    let isMounted = true;
    setAssistant(null);
    setVariables([]);
    setArgumentMap(new Map());
    setErrorMessage('');

    const request = new GetAssistantRequest();
    const assistantDef = new AssistantDefinition();
    assistantDef.setAssistantid(assistantId);
    request.setAssistantdefinition(assistantDef);
    GetAssistant(connectionCfg, request)
      .then(response => {
        if (!isMounted) return;
        if (response?.getSuccess()) {
          setAssistant(response.getData()!);
          const pmtVars = response
            .getData()
            ?.getAssistantprovidermodel()
            ?.getTemplate()
            ?.getPromptvariablesList();
          if (pmtVars) {
            setVariables(pmtVars);
            const defaults = new Map<string, string>();
            pmtVars.forEach(v => {
              if (v.getDefaultvalue()) {
                defaults.set(v.getName(), v.getDefaultvalue());
              }
            });
            setArgumentMap(defaults);
          }
        }
      })
      .catch(() => {
        if (isMounted) {
          setErrorMessage('Unable to load assistant configuration.');
        }
      });

    return () => {
      isMounted = false;
    };
  }, [assistantId, connectionCfg]);

  if (!assistantId) {
    return <Navigate to="/404" replace />;
  }

  const validatePhoneNumber = () => {
    if (!country.name) {
      setErrorMessage('Please select a country.');
      return false;
    }
    if (
      (country.name !== 'Other' && phoneNumber.length < 7) ||
      phoneNumber.length > 15
    ) {
      setErrorMessage('Please enter a valid phone number.');
      return false;
    }
    return true;
  };

  const handleSubmit = () => {
    if (!validatePhoneNumber()) return;
    setErrorMessage('');
    setCallStatus('calling');

    const phoneCallRequest = new CreatePhoneCallRequest();
    const assistantDef = new AssistantDefinition();
    assistantDef.setAssistantid(assistantId);
    assistantDef.setVersion('latest');
    phoneCallRequest.setAssistant(assistantDef);
    argumentMap.forEach((value, key) => {
      phoneCallRequest.getArgsMap().set(key, StringToAny(value));
    });
    phoneCallRequest.setTonumber(country.value + phoneNumber);

    CreatePhoneCall(connectionCfg, phoneCallRequest)
      .then(x => {
        if (x.getSuccess()) {
          const status = getStatusMetric(x.getData()?.getMetricsList());
          if (status === 'FAILED') {
            setCallStatus('failed');
            setErrorMessage('Unable to start the call, please try again.');
            return;
          }
          setCallStatus('success');
          return;
        }
        setCallStatus('failed');
        const err = x.getError();
        setErrorMessage(
          err?.getHumanmessage() ||
            'Unable to start the call, please try again.',
        );
      })
      .catch(() => {
        setCallStatus('failed');
        setErrorMessage('Unable to start the call, please try again.');
      });
  };

  const handleReset = () => {
    setPhoneNumber('');
    setCallStatus('idle');
    setErrorMessage('');
  };

  const deployment = assistant?.getPhonedeployment() ?? null;
  const stt = deployment?.getInputaudio() ?? null;
  const tts = deployment?.getOutputaudio() ?? null;
  const model = assistant?.getAssistantprovidermodel() ?? null;

  return (
    <div className="flex h-screen min-h-0 w-full flex-col text-sm/6">
      <PreviewAgentHeader />
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* ── Left: phone call form ───────────────────────────────────── */}
        <div className="flex flex-col overflow-hidden h-full w-full lg:w-[70%] border-r border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-950">
          {/* Header */}
          <div className="flex items-center gap-1.5 px-3 py-2 border-b border-gray-200 dark:border-gray-800 shrink-0">
            <IconOnlyButton
              kind="ghost"
              size="sm"
              renderIcon={ArrowLeft}
              iconDescription="Back to Assistant"
              onClick={() => {
                window.location.href = `/deployment/assistant/${assistantId}/overview`;
              }}
            />
            <span className="text-sm font-medium text-gray-900 dark:text-gray-100">
              Back to Assistant
            </span>
          </div>

          {/* Body */}
          <div className="flex-1 flex flex-col items-center justify-center px-5 md:px-8">
            <Form
              className="w-full max-w-[42rem]"
              onSubmit={e => e.preventDefault()}
            >
              <Stack gap={7}>
                <div className="space-y-1">
                  <Text
                    as="h2"
                    isLoading={!assistant}
                    heading
                    skeletonWidth="60%"
                    className="text-xl font-semibold text-gray-900 dark:text-gray-100"
                  >
                    Debug Phone Call
                  </Text>
                  <Text
                    as="p"
                    isLoading={!assistant}
                    skeletonWidth="80%"
                    className="text-gray-500 dark:text-gray-400"
                  >
                    Place a live test call to validate your phone deployment
                    end-to-end.
                  </Text>
                </div>

                <div>
                  <Text as="label" className="block mb-2 text-sm font-medium">
                    Phone number
                  </Text>
                  <div className="grid grid-cols-[16rem_minmax(0,1fr)] gap-3">
                    <Dropdown<Country>
                      id="phone-country"
                      titleText=""
                      label="Select country code"
                      items={PHONE_COUNTRIES}
                      selectedItem={country}
                      onChange={({ selectedItem }) =>
                        setCountry(selectedItem ?? DEFAULT_COUNTRY)
                      }
                      itemToString={item =>
                        item ? `${item.name} (${item.value})` : ''
                      }
                      hideLabel
                    />
                    <TextInput
                      id="phone-number"
                      labelText="Phone number"
                      hideLabel
                      type="tel"
                      placeholder="Enter your phone number"
                      value={phoneNumber}
                      onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                        setPhoneNumber(e.target.value);
                        setErrorMessage('');
                      }}
                      invalid={Boolean(errorMessage)}
                    />
                  </div>
                </div>

                {errorMessage && (
                  <Notification
                    kind="error"
                    title="Error"
                    subtitle={errorMessage}
                  />
                )}

                {callStatus === 'success' && (
                  <Notification
                    kind="success"
                    title="Success"
                    subtitle="Call has been created successfully."
                  />
                )}

                <div className="flex items-center justify-between">
                  {callStatus === 'success' ? (
                    <GhostButton size="sm" onClick={handleReset}>
                      Make another call
                    </GhostButton>
                  ) : (
                    <span />
                  )}
                  <PrimaryButton
                    size="md"
                    renderIcon={PhoneOutgoing}
                    onClick={handleSubmit}
                    isLoading={callStatus === 'calling'}
                  >
                    Start Call
                  </PrimaryButton>
                </div>
              </Stack>
            </Form>
          </div>
        </div>

        {/* ── Right: debugger panel ───────────────────────────────────── */}
        <div className="shrink-0 flex flex-col overflow-hidden border-t lg:border-t-0 border-gray-200 dark:border-gray-800 w-full lg:w-[30%] bg-white dark:bg-gray-950">
          <PhoneAgentDebugger
            assistant={assistant}
            deployment={deployment ? deployment : undefined}
            stt={stt}
            tts={tts}
            model={model}
            variables={variables}
            onChangeArgument={onChangeArgument}
          />
        </div>
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// Phone Agent Debugger (right panel)
// ---------------------------------------------------------------------------

const PhoneAgentDebugger: React.FC<{
  assistant: Assistant | null;
  deployment: ReturnType<Assistant['getPhonedeployment']>;
  stt: any;
  tts: any;
  model: any;
  variables: Variable[];
  onChangeArgument: (k: string, v: string) => void;
}> = ({
  assistant,
  deployment,
  stt,
  tts,
  model,
  variables,
  onChangeArgument,
}) => {
  const [tab, setTab] = useState<PhoneDebugTab>('configuration');
  const loading = !assistant;
  const inputMode = 'Text' + (deployment?.getInputaudio() ? ', Audio' : '');
  const outputMode = 'Text' + (deployment?.getOutputaudio() ? ', Audio' : '');

  return (
    <div className="flex flex-col h-full overflow-hidden text-sm">
      {/* Tab bar */}
      <div className="border-b border-gray-200 dark:border-gray-800">
        <Tabs
          tabs={PHONE_DEBUG_TAB_LABELS}
          selectedIndex={PHONE_DEBUG_TABS.indexOf(tab)}
          onChange={idx => setTab(PHONE_DEBUG_TABS[idx])}
          contained
          aria-label="Phone debugger tabs"
          isLoading={loading}
        />
      </div>

      {/* ── configuration tab ── */}
      {tab === 'configuration' && (
        <div className="flex-1 min-h-0 overflow-y-auto">
          <ConfigBlock title="deployment" isLoading={loading} skeletonRows={4}>
            {assistant && (
              <>
                <InfoRow
                  label="telephony"
                  value={deployment?.getPhoneprovidername() || 'not configured'}
                />
                <InfoRow
                  label="model"
                  value={model?.getModelprovidername() || 'not configured'}
                />
                <InfoRow label="input mode" value={inputMode} />
                <InfoRow label="output mode" value={outputMode} />
              </>
            )}
          </ConfigBlock>

          <ConfigBlock title="assistant" isLoading={loading} skeletonRows={2}>
            {assistant && (
              <>
                <InfoRow label="name" value={assistant.getName()} />
                <InfoRow label="arguments" value={String(variables.length)} />
                {assistant.getDescription() && (
                  <InfoRow
                    label="description"
                    value={assistant.getDescription()}
                  />
                )}
              </>
            )}
          </ConfigBlock>

          <ConfigBlock title="stt" isLoading={loading} skeletonRows={2}>
            {stt ? (
              <>
                <InfoRow label="provider" value={stt.getAudioprovider()} />
                {stt.getAudiooptionsList().map((d: any) => (
                  <InfoRow
                    key={d.getKey()}
                    label={d.getKey()}
                    value={d.getValue()}
                  />
                ))}
              </>
            ) : (
              <ConfigEmpty label="Audio input is not configured." />
            )}
          </ConfigBlock>

          <ConfigBlock title="tts" isLoading={loading} skeletonRows={2}>
            {tts ? (
              <>
                <InfoRow label="provider" value={tts.getAudioprovider()} />
                {tts.getAudiooptionsList().map((d: any) => (
                  <InfoRow
                    key={d.getKey()}
                    label={d.getKey()}
                    value={d.getValue()}
                  />
                ))}
              </>
            ) : (
              <ConfigEmpty label="Audio output is not configured." />
            )}
          </ConfigBlock>

          <ConfigBlock title="llm" isLoading={loading} skeletonRows={2}>
            {model ? (
              <>
                <InfoRow
                  label="provider"
                  value={model.getModelprovidername()}
                />
                {model.getAssistantmodeloptionsList().map((m: any) => (
                  <InfoRow
                    key={m.getKey()}
                    label={m.getKey()}
                    value={m.getValue()}
                  />
                ))}
              </>
            ) : (
              <ConfigEmpty label="Model configuration is not available." />
            )}
          </ConfigBlock>
        </div>
      )}

      {/* ── arguments tab ── */}
      {tab === 'arguments' && (
        <div className="flex-1 min-h-0 overflow-y-auto">
          <DebuggerTabHeader
            title="Arguments"
            subtitle={`${variables.length} prompt variables`}
          />
          <ArgumentList
            variables={variables}
            onChangeArgument={onChangeArgument}
          />
        </div>
      )}
    </div>
  );
};
