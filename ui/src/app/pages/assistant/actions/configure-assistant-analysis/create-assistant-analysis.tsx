import React, { FC, useState } from 'react';
import { useConfirmDialog } from '@/app/pages/assistant/actions/hooks/use-confirmation';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { PrimaryButton, SecondaryButton } from '@/app/components/carbon/button';
import { Stack, TextInput, TextArea } from '@/app/components/carbon/form';
import { ButtonSet, NumberInput } from '@carbon/react';
import { useCurrentCredential } from '@/hooks/use-credential';
import { randomMeaningfullName } from '@/utils';
import { EndpointDropdown } from '@/app/components/dropdown/endpoint-dropdown';
import { CreateAnalysis, Endpoint } from '@rapidaai/react';
import toast from 'react-hot-toast/headless';
import { connectionConfig } from '@/configs';
import { TabForm } from '@/app/components/form/tab-form';
import {
  ASSISTANT_CONDITION_KEY_OPTIONS,
  ASSISTANT_CONDITION_OPERATOR_OPTIONS,
  ASSISTANT_CONDITION_SOURCE_OPTIONS,
  ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY,
  AssistantMappingTable,
} from '@/app/components/tools/common';
import { SourceConditionRule } from '@/app/components/conditions/source-condition-rule';
import { InputGroup } from '../../../../components/input-group/index';

// ── Parameter types ──────────────────────────────────────────────────────────

type ParamType =
  | 'client'
  | 'assistant'
  | 'conversation'
  | 'argument'
  | 'metadata'
  | 'option'
  | 'custom'
  | 'analysis';

interface Parameter {
  type: ParamType;
  key: string;
  value: string;
}

const PARAM_TYPE_OPTIONS = [
  { value: 'client', name: 'Client' },
  { value: 'assistant', name: 'Assistant' },
  { value: 'conversation', name: 'Conversation' },
  { value: 'argument', name: 'Argument' },
  { value: 'metadata', name: 'Metadata' },
  { value: 'option', name: 'Option' },
  { value: 'custom', name: 'Custom' },
  { value: 'analysis', name: 'Analysis' },
];
const RESERVED_CONDITION_MAPPING_KEY = 'metadata.condition';
// ── Main component ───────────────────────────────────────────────────────────

export const CreateAssistantAnalysis: FC<{ assistantId: string }> = ({
  assistantId,
}) => {
  const navigator = useGlobalNavigation();
  const { authId, token, projectId } = useCurrentCredential();
  const { showDialog, ConfirmDialogComponent } = useConfirmDialog({});

  const [activeTab, setActiveTab] = useState('configure');
  const [errorMessage, setErrorMessage] = useState('');
  const [name, setName] = useState(randomMeaningfullName('analysis'));
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<number>(0);
  const [endpointId, setEndpointId] = useState<string>('');
  const [parameters, setParameters] = useState<Parameter[]>([
    { type: 'conversation', key: 'messages', value: 'messages' },
  ]);
  const [sourceConditions, setSourceConditions] = useState([
    { key: 'source', condition: '=', value: 'all' },
  ]);

  const validateConfigure = (): boolean => {
    setErrorMessage('');
    if (!endpointId) {
      setErrorMessage(
        'Please select a valid endpoint to be executed for analysis.',
      );
      return false;
    }
    if (parameters.length === 0) {
      setErrorMessage('Please provide one or more parameters.');
      return false;
    }
    const keys = parameters.map(p => `${p.type}.${p.key}`);
    if (keys.includes(RESERVED_CONDITION_MAPPING_KEY)) {
      setErrorMessage(
        'metadata.condition is reserved and managed by the rule section.',
      );
      return false;
    }
    if (new Set(keys).size !== keys.length) {
      setErrorMessage('Duplicate parameter keys are not allowed.');
      return false;
    }
    if (parameters.some(p => !p.key.trim() || !p.value.trim())) {
      setErrorMessage('Empty parameter keys or values are not allowed.');
      return false;
    }
    const values = parameters.map(p => p.value.trim());
    if (new Set(values).size !== values.length) {
      setErrorMessage('Duplicate parameter values are not allowed.');
      return false;
    }
    return true;
  };

  const onSubmit = () => {
    setErrorMessage('');
    if (!name) {
      setErrorMessage('Please provide a valid name.');
      return;
    }
    const keys = parameters.map(p => `${p.type}.${p.key}`);
    if (keys.includes(RESERVED_CONDITION_MAPPING_KEY)) {
      setErrorMessage(
        'metadata.condition is reserved and managed by the rule section.',
      );
      return;
    }

    CreateAnalysis(
      connectionConfig,
      assistantId,
      name,
      endpointId,
      'latest',
      priority,
      [
        ...parameters.map(p => ({ key: `${p.type}.${p.key}`, value: p.value })),
        {
          key: 'metadata.condition',
          value: JSON.stringify(sourceConditions),
        },
      ],
      (err, response) => {
        if (err) {
          setErrorMessage('Unable to create analysis. Please try again.');
          return;
        }
        if (response?.getSuccess()) {
          toast.success('Analysis added to assistant successfully');
          navigator.goToConfigureAssistantAnalysis(assistantId);
        } else {
          setErrorMessage(
            response?.getError()?.getHumanmessage() ||
              'Unable to create analysis.',
          );
        }
      },
      { 'x-auth-id': authId, authorization: token, 'x-project-id': projectId },
      description,
    );
  };

  return (
    <>
      <ConfirmDialogComponent />
      <TabForm
        formHeading="Complete all steps to configure your analysis."
        activeTab={activeTab}
        onChangeActiveTab={() => {}}
        errorMessage={errorMessage}
        form={[
          {
            code: 'configure',
            name: 'Configure',
            description:
              'Select the endpoint and map data parameters for analysis.',
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
                    if (validateConfigure()) setActiveTab('profile');
                  }}
                >
                  Continue
                </PrimaryButton>
              </ButtonSet>,
            ],
            body: (
              <div>
                <InputGroup title="Execution conditions (Optional)">
                  <SourceConditionRule
                    conditions={sourceConditions}
                    onChangeConditions={setSourceConditions}
                    conditionOptions={ASSISTANT_CONDITION_OPERATOR_OPTIONS}
                    sourceOptions={ASSISTANT_CONDITION_SOURCE_OPTIONS}
                    keyOptions={ASSISTANT_CONDITION_KEY_OPTIONS}
                    valueOptionsByKey={ASSISTANT_CONDITION_VALUE_OPTIONS_BY_KEY}
                    keyTooltipText="The variable to evaluate before running this analysis."
                  />
                </InputGroup>
                <Stack gap={7} className="px-8 pt-6 pb-8 max-w-4xl">
                  <EndpointDropdown
                    currentEndpoint={endpointId}
                    onChangeEndpoint={(e: Endpoint) => {
                      if (e) setEndpointId(e.getId());
                    }}
                  />
                  <AssistantMappingTable
                    parameters={parameters}
                    onChange={setParameters}
                    typeOptions={PARAM_TYPE_OPTIONS}
                    includeEmptyKeyOption
                    createNewParameter={() => ({
                      type: 'assistant',
                      key: '',
                      value: '',
                    })}
                    title="Parameters"
                    addButtonLabel="Add parameter"
                    valuePlaceholder="Variable name"
                    removeButtonKind="danger--ghost"
                  />
                </Stack>
              </div>
            ),
          },
          {
            code: 'profile',
            name: 'Profile',
            description: 'Provide a name and set the execution priority.',
            actions: [
              <ButtonSet className="!w-full [&>button]:!flex-1 [&>button]:!max-w-none">
                <SecondaryButton
                  size="lg"
                  onClick={() => showDialog(navigator.goBack)}
                >
                  Cancel
                </SecondaryButton>
                <PrimaryButton size="lg" onClick={onSubmit}>
                  Configure analysis
                </PrimaryButton>
              </ButtonSet>,
            ],
            body: (
              <div className="px-8 pt-6 pb-8 max-w-2xl">
                <Stack gap={6}>
                  <TextInput
                    id="analysis-name"
                    labelText="Name"
                    value={name}
                    onChange={e => setName(e.target.value)}
                    placeholder="A name for your analysis"
                    helperText="A unique name to identify this analysis configuration."
                  />
                  <TextArea
                    id="analysis-description"
                    labelText="Description (Optional)"
                    value={description}
                    onChange={e => setDescription(e.target.value)}
                    placeholder="An optional description of this analysis..."
                    rows={2}
                  />
                  <NumberInput
                    id="analysis-priority"
                    label="Execution Priority"
                    min={0}
                    value={priority}
                    onChange={(e: any, { value }: any) => setPriority(value)}
                    helperText="Lower numbers execute first when multiple analyses are triggered."
                  />
                </Stack>
              </div>
            ),
          },
        ]}
      />
    </>
  );
};
