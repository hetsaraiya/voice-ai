import React, { useCallback, useEffect, useState } from 'react';
import { Helmet } from '@/app/components/helmet';
import { useCredential } from '@/hooks/use-credential';
import { useRapidaStore } from '@/hooks';
import { useNavigate, useSearchParams } from 'react-router-dom';
import toast from 'react-hot-toast/headless';
import SingleAssistant from './single-assistant';
import { useAssistantPageStore } from '@/hooks/use-assistant-page-store';
import { Assistant } from '@rapidaai/react';
import { PageLoading } from '@/app/components/carbon/loading';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { Pagination } from '@/app/components/carbon/pagination';
import {
  Add,
  ArrowRight,
  Bot,
  PromptTemplate,
  Renew,
} from '@carbon/icons-react';
import {
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Button,
  ClickableTile,
} from '@carbon/react';
import { PageHeaderBlock } from '@/app/components/blocks/page-header-block';
import { PageTitleBlock } from '@/app/components/blocks/page-title-block';
import { Modal, ModalBody, ModalHeader } from '@/app/components/carbon/modal';

export function AssistantPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [userId, token, projectId] = useCredential();
  const assistantAction = useAssistantPageStore();
  const { loading, showLoader, hideLoader } = useRapidaStore();
  const [createAssistantModalOpen, setCreateAssistantModalOpen] =
    useState(false);

  useEffect(() => {
    if (searchParams) {
      const searchParamMap = Object.fromEntries(searchParams.entries());
      Object.entries(searchParamMap).forEach(([key, value]) =>
        assistantAction.addCriteria(key, value, '='),
      );
    }
  }, [searchParams]);

  const onError = useCallback((err: string) => {
    hideLoader();
    toast.error(err);
  }, []);

  const onSuccess = useCallback((data: Assistant[]) => {
    hideLoader();
  }, []);

  const getAssistants = useCallback((projectId, token, userId) => {
    showLoader();
    assistantAction.onGetAllAssistant(
      projectId,
      token,
      userId,
      onError,
      onSuccess,
    );
  }, []);

  useEffect(() => {
    getAssistants(projectId, token, userId);
  }, [
    projectId,
    assistantAction.page,
    assistantAction.pageSize,
    assistantAction.criteria,
  ]);

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <Helmet title="Assistant" />
      <PageHeaderBlock>
        <div className="flex items-center gap-3">
          <PageTitleBlock>Assistants</PageTitleBlock>
          <span className="text-xs text-gray-500 dark:text-gray-400 tabular-nums">
            {assistantAction.pageSize}/{assistantAction.totalCount}
          </span>
        </div>
      </PageHeaderBlock>
      <TableToolbar>
        <TableToolbarContent>
          <TableToolbarSearch placeholder="Search assistants..." />
          <Button
            hasIconOnly
            renderIcon={Renew}
            iconDescription="Refresh"
            kind="ghost"
            onClick={() => getAssistants(projectId, token, userId)}
            tooltipPosition="bottom"
          />
          <Button
            renderIcon={Add}
            onClick={() => setCreateAssistantModalOpen(true)}
          >
            Create new Assistant
          </Button>
        </TableToolbarContent>
      </TableToolbar>

      {/* Content */}
      {loading ? (
        <PageLoading className="h-full" />
      ) : assistantAction.assistants &&
        assistantAction.assistants.length > 0 ? (
        <section className="grid content-start grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4 flex-1 overflow-auto p-4">
          {assistantAction.assistants.map((ast, idx) => (
            <SingleAssistant key={idx} assistant={ast} />
          ))}
        </section>
      ) : assistantAction.criteria.length > 0 ? (
        <div className="h-full flex justify-center items-center">
          <EmptyState
            title="No Assistant"
            subtitle="No assistants match your current filters."
            action="Create new Assistant"
            actionIcon={Add}
            onAction={() => setCreateAssistantModalOpen(true)}
          />
        </div>
      ) : (
        <div className="h-full flex justify-center items-center">
          <EmptyState
            title="No Assistant"
            subtitle="Create assistants for each client, brand, or business unit from one controlled platform."
            action="Create new Assistant"
            actionIcon={Add}
            onAction={() => setCreateAssistantModalOpen(true)}
          />
        </div>
      )}

      {/* Pagination */}
      {assistantAction.assistants && assistantAction.assistants.length > 0 && (
        <Pagination
          className="shrink-0"
          totalItems={assistantAction.totalCount}
          page={assistantAction.page}
          pageSize={assistantAction.pageSize}
          pageSizes={[10, 20, 50]}
          onChange={({ page, pageSize }) => {
            if (pageSize !== assistantAction.pageSize) {
              assistantAction.setPageSize(pageSize);
            } else {
              assistantAction.setPage(page);
            }
          }}
        />
      )}

      <CreateAssistantDialog
        open={createAssistantModalOpen}
        onClose={() => setCreateAssistantModalOpen(false)}
        onSelect={path => {
          setCreateAssistantModalOpen(false);
          navigate(path);
        }}
      />
    </div>
  );
}

const createAssistantOptions = [
  {
    title: 'From Prompt',
    eyebrow: 'Prompting',
    description:
      'Create a voice assistant from instructions, model configuration, tools, and deployment settings.',
    icon: PromptTemplate,
    path: '/deployment/assistant/create-assistant',
  },
  {
    title: 'AgentKit',
    eyebrow: 'Agents',
    description:
      'Connect an AgentKit assistant and manage it alongside your voice deployments and integrations.',
    icon: Bot,
    path: '/deployment/assistant/connect-agentkit',
  },
];

function CreateAssistantDialog({
  open,
  onClose,
  onSelect,
}: {
  open: boolean;
  onClose: () => void;
  onSelect: (path: string) => void;
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      size="lg"
      containerClassName="!max-w-[960px]"
    >
      <ModalHeader
        label="Assistant"
        title="Create an assistant"
        onClose={onClose}
      />
      <ModalBody>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {createAssistantOptions.map(option => {
            const Icon = option.icon;
            return (
              <ClickableTile
                key={option.title}
                onClick={() => onSelect(option.path)}
                className="group relative !min-h-[260px] !overflow-hidden !rounded-none !border !border-gray-200 !bg-white !p-6 dark:!border-gray-700 dark:!bg-gray-900"
              >
                <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(135deg,color-mix(in_oklab,var(--color-primary)_12%,transparent)_0%,transparent_46%)] dark:bg-[linear-gradient(135deg,color-mix(in_oklab,var(--color-primary)_18%,transparent)_0%,transparent_52%)]" />
                <div className="relative z-[1] flex h-full flex-col">
                  <div className="flex items-center justify-between gap-4">
                    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-gray-500 dark:text-gray-400">
                      <Icon size={20} />
                      <span>{option.eyebrow}</span>
                    </div>
                    <ArrowRight
                      size={24}
                      className="text-primary transition-transform group-hover:translate-x-1"
                    />
                  </div>
                  <h2 className="mt-8 text-2xl font-semibold leading-tight text-gray-900 dark:text-white">
                    {option.title}
                  </h2>
                  <p className="mt-5 max-w-[24rem] text-base leading-6 text-gray-600 dark:text-gray-300">
                    {option.description}
                  </p>
                </div>
              </ClickableTile>
            );
          })}
        </div>
      </ModalBody>
    </Modal>
  );
}
