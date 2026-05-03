import {
  AssistantDefinition,
  ConnectionConfig,
  DeleteAssistant,
  GetAssistant,
  GetAssistantRequest,
} from '@rapidaai/react';
import { GetAssistantResponse } from '@rapidaai/react';
import { ServiceError } from '@rapidaai/react';
import { ErrorContainer } from '@/app/components/error-container';
import { useDeleteConfirmDialog } from '@/app/pages/assistant/actions/hooks/use-delete-confirmation';
import { useRapidaStore } from '@/hooks';
import { useCurrentCredential } from '@/hooks/use-credential';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { FC, useEffect, useState } from 'react';
import toast from 'react-hot-toast/headless';
import { useParams } from 'react-router-dom';
import { UpdateAssistantDetail } from '@rapidaai/react';
import { connectionConfig } from '@/configs';
import { Notification } from '@/app/components/carbon/notification';
import { Form, Stack, TextInput, TextArea } from '@/app/components/carbon/form';
import { PrimaryButton, DangerButton } from '@/app/components/carbon/button';
import { Breadcrumb, BreadcrumbItem } from '@carbon/react';
import { WarningAlt } from '@carbon/icons-react';
import { InputGroup } from '../../../../components/input-group/index';

export function EditAssistantPage() {
  const { assistantId } = useParams();
  const { goToAssistantListing } = useGlobalNavigation();

  if (!assistantId)
    return (
      <div className="flex flex-1">
        <ErrorContainer
          onAction={goToAssistantListing}
          code="403"
          actionLabel="Go to listing"
          title="Assistant not available"
          description="This assistant may be archived or you don't have access to it."
        />
      </div>
    );

  return <EditAssistant assistantId={assistantId!} />;
}

export const EditAssistant: FC<{ assistantId: string }> = ({ assistantId }) => {
  const { authId, token, projectId } = useCurrentCredential();
  const { loading, showLoader, hideLoader } = useRapidaStore();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const { goToAssistantListing } = useGlobalNavigation();

  useEffect(() => {
    showLoader('block');
    const request = new GetAssistantRequest();
    const assistantDef = new AssistantDefinition();
    assistantDef.setAssistantid(assistantId);
    request.setAssistantdefinition(assistantDef);
    GetAssistant(
      connectionConfig,
      request,
      ConnectionConfig.WithDebugger({
        authorization: token,
        userId: authId,
        projectId: projectId,
      }),
    )
      .then(car => {
        hideLoader();
        if (car?.getSuccess()) {
          const assistant = car.getData();
          if (assistant) {
            setName(assistant.getName());
            setDescription(assistant.getDescription());
          }
        } else {
          const error = car?.getError();
          if (error) {
            toast.error(error.getHumanmessage());
            return;
          }
          toast.error('Unable to load assistant. Please try again later.');
        }
      })
      .catch(() => {
        hideLoader();
      });
  }, [assistantId]);

  const onUpdateAssistantDetail = () => {
    setErrorMessage('');
    showLoader('block');
    const afterUpdateAssistant = (
      err: ServiceError | null,
      car: GetAssistantResponse | null,
    ) => {
      hideLoader();
      if (car?.getSuccess()) {
        toast.success('The assistant has been successfully updated.');
        const assistant = car.getData();
        if (assistant) {
          setName(assistant.getName());
          setDescription(assistant.getDescription());
        }
      } else {
        const error = car?.getError();
        if (error) {
          setErrorMessage(error.getHumanmessage());
          return;
        }
        setErrorMessage('Unable to update assistant. Please try again later.');
      }
    };
    UpdateAssistantDetail(
      connectionConfig,
      assistantId,
      name,
      description,
      afterUpdateAssistant,
      {
        authorization: token,
        'x-auth-id': authId,
        'x-project-id': projectId,
      },
    );
  };

  const Deletion = useDeleteConfirmDialog({
    onConfirm: () => {
      showLoader('block');
      const afterDeleteAssistant = (
        err: ServiceError | null,
        car: GetAssistantResponse | null,
      ) => {
        if (car?.getSuccess()) {
          toast.success('The assistant has been deleted successfully.');
          goToAssistantListing();
        } else {
          hideLoader();
          const error = car?.getError();
          if (error) {
            toast.error(error.getHumanmessage());
            return;
          }
          toast.error('Unable to delete assistant. Please try again later.');
        }
      };
      DeleteAssistant(connectionConfig, assistantId, afterDeleteAssistant, {
        authorization: token,
        'x-auth-id': authId,
        'x-project-id': projectId,
      });
    },
    name: name,
  });

  return (
    <div className="w-full flex flex-col flex-1 overflow-auto bg-white dark:bg-gray-900">
      <Deletion.ConfirmDeleteDialogComponent />

      {/* Page header */}
      <div className="px-4 pt-4 pb-6 border-b border-gray-200 dark:border-gray-800">
        <Breadcrumb noTrailingSlash className="mb-2">
          <BreadcrumbItem
            href={`/deployment/assistant/${assistantId}/overview`}
          >
            Assistant
          </BreadcrumbItem>
        </Breadcrumb>
        <h1 className="text-2xl font-light tracking-tight">General Settings</h1>
      </div>

      <InputGroup title="Identity">
        <TextInput
          id="assistant-id"
          labelText="Assistant ID"
          value={assistantId}
          readOnly
          helperText="Your assistant's unique identifier. This cannot be changed."
        />
      </InputGroup>
      <InputGroup title="General Information">
        <Form
          className="flex flex-col gap-8 max-w-2xl"
          onSubmit={e => {
            e.preventDefault();
            onUpdateAssistantDetail();
          }}
        >
          <Stack gap={6}>
            <TextInput
              id="assistant-name"
              labelText="Name"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. Customer support bot"
              helperText="The display name shown across the platform."
            />
            <TextArea
              id="assistant-description"
              labelText="Description"
              value={description}
              rows={4}
              onChange={e => setDescription(e.target.value)}
              placeholder="What's the purpose of this assistant?"
              helperText="Describe what this assistant does and its intended use case."
            />
            {errorMessage && (
              <Notification
                kind="error"
                title="Error"
                subtitle={errorMessage}
              />
            )}
            <div>
              <PrimaryButton
                size="md"
                isLoading={loading}
                onClick={onUpdateAssistantDetail}
              >
                Save changes
              </PrimaryButton>
            </div>
          </Stack>
        </Form>
      </InputGroup>
      <div className="px-4 pt-6 pb-12 flex flex-col gap-8 max-w-2xl">
        {/* ── General Information ── */}

        {/* ── Danger Zone ── */}
        <div>
          <h2 className="text-sm font-semibold uppercase tracking-wider text-red-600 mb-4">
            Danger Zone
          </h2>
          <div>
            <div className="flex items-start justify-between gap-6">
              <div className="flex flex-col gap-1">
                <p className="text-sm font-semibold flex items-center gap-2">
                  <WarningAlt size={16} className="text-red-600" />
                  Delete this assistant
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
                  Once deleted, all active connections will be terminated
                  immediately and data will be permanently removed. This action
                  cannot be undone.
                </p>
              </div>
              <DangerButton
                size="md"
                isLoading={loading}
                onClick={Deletion.showDialog}
              >
                Delete assistant
              </DangerButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
