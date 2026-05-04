import React, { FC, useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { toHumanReadableDateTime } from '@/utils/date';
import { useCurrentCredential } from '@/hooks/use-credential';
import { useRapidaStore } from '@/hooks';
import { SectionLoader } from '@/app/components/loader/section-loader';
import { CreateAssistantWebhook } from './create-assistant-webhook';
import toast from 'react-hot-toast/headless';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { UpdateAssistantWebhook } from '@/app/pages/assistant/actions/configure-assistant-webhook/update-assistant-webhook';
import { useAssistantWebhookPageStore } from '@/app/pages/assistant/actions/store/use-webhook-page-store';
import { IconOnlyButton, PrimaryButton } from '@/app/components/carbon/button';
import { Pagination } from '@/app/components/carbon/pagination';
import { Add, Renew, Webhook, Edit, TrashCan } from '@carbon/icons-react';
import { Tag } from '@carbon/react';
import {
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Button,
  TableBatchActions,
  TableBatchAction,
  RadioButton,
  Breadcrumb,
  BreadcrumbItem,
} from '@carbon/react';
import { TableSection } from '@/app/components/sections/table-section';

const getWebhookOptionMap = (row: any): Map<string, string> => {
  const map = new Map<string, string>();
  const options = row?.getOptionsList?.() || [];
  options.forEach((option: any) => {
    const key = option?.getKey?.();
    const value = option?.getValue?.();
    if (key && typeof value === 'string') {
      map.set(key, value);
    }
  });
  return map;
};

const getWebhookMethod = (row: any): string =>
  getWebhookOptionMap(row).get('http_method') || '';

const getWebhookUrl = (row: any): string =>
  getWebhookOptionMap(row).get('http_url') || '';

const getWebhookEvents = (row: any): string[] =>
  row?.getAssistanteventsList?.() || [];

const getWebhookRetryCount = (row: any): number => {
  const value = Number(getWebhookOptionMap(row).get('max_retry_count'));
  return Number.isFinite(value) ? value : 0;
};

const getWebhookTimeoutSecond = (row: any): number => {
  const value = Number(getWebhookOptionMap(row).get('timeout_seconds'));
  return Number.isFinite(value) ? value : 0;
};

export function ConfigureAssistantWebhookPage() {
  const { assistantId } = useParams();
  return (
    <>
      {assistantId && <ConfigureAssistantWebhook assistantId={assistantId} />}
    </>
  );
}

export function CreateAssistantWebhookPage() {
  const { assistantId } = useParams();
  return (
    <>{assistantId && <CreateAssistantWebhook assistantId={assistantId} />}</>
  );
}

export function UpdateAssistantWebhookPage() {
  const { assistantId } = useParams();
  return (
    <>{assistantId && <UpdateAssistantWebhook assistantId={assistantId} />}</>
  );
}

const ConfigureAssistantWebhook: FC<{ assistantId: string }> = ({
  assistantId,
}) => {
  const navigation = useGlobalNavigation();
  const axtion = useAssistantWebhookPageStore();
  const { authId, token, projectId } = useCurrentCredential();
  const { loading, showLoader, hideLoader } = useRapidaStore();
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedWebhookId, setSelectedWebhookId] = useState<string | null>(
    null,
  );

  useEffect(() => {
    showLoader('block');
    get();
  }, [assistantId, projectId, token, authId, axtion.page, axtion.pageSize]);

  const get = () => {
    axtion.getAssistantWebhook(
      assistantId,
      projectId,
      token,
      authId,
      e => {
        toast.error(e);
        hideLoader();
      },
      v => {
        hideLoader();
      },
    );
  };

  const deleteAssistantWebhook = (assistantId: string, webhookId: string) => {
    showLoader('block');
    axtion.deleteAssistantWebhook(
      assistantId,
      webhookId,
      projectId,
      token,
      authId,
      e => {
        toast.error(e);
        hideLoader();
      },
      v => {
        get();
      },
    );
  };

  const filteredWebhooks = searchTerm.trim()
    ? axtion.webhooks.filter(row =>
        [
          getWebhookMethod(row),
          getWebhookUrl(row),
          row.getExecutionpriority(),
          row.getStatus(),
          ...getWebhookEvents(row),
        ]
          .join(' ')
          .toLowerCase()
          .includes(searchTerm.trim().toLowerCase()),
      )
    : axtion.webhooks;

  const selectedWebhook = filteredWebhooks.find(
    row => row.getId() === selectedWebhookId,
  );

  if (loading) {
    return (
      <div className="h-full w-full flex flex-col items-center justify-center">
        <SectionLoader />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col flex-1">
      <div className="px-4 pt-4 pb-6 border-b border-gray-200 dark:border-gray-800 bg-white dark:bg-gray-900">
        <div>
          <Breadcrumb noTrailingSlash className="mb-2">
            <BreadcrumbItem
              href={`/deployment/assistant/${assistantId}/overview`}
            >
              Assistant
            </BreadcrumbItem>
          </Breadcrumb>
          <h1 className="text-2xl font-light tracking-tight">Webhooks</h1>
        </div>
      </div>
      <TableToolbar>
        <TableBatchActions
          shouldShowBatchActions={!!selectedWebhook}
          totalSelected={selectedWebhook ? 1 : 0}
          totalCount={filteredWebhooks.length}
          onCancel={() => setSelectedWebhookId(null)}
          className="[&_[class*=divider]]:hidden [&_.cds--btn]:transition-colors [&_.cds--btn:hover]:!bg-primary [&_.cds--btn:hover]:!text-white"
        >
          {selectedWebhook && (
            <>
              <TableBatchAction
                renderIcon={Edit}
                kind="ghost"
                onClick={() => {
                  navigation.goToEditAssistantWebhook(
                    assistantId,
                    selectedWebhook.getId(),
                  );
                  setSelectedWebhookId(null);
                }}
              >
                Edit webhook
              </TableBatchAction>
              <TableBatchAction
                renderIcon={TrashCan}
                kind="ghost"
                onClick={() => {
                  deleteAssistantWebhook(assistantId, selectedWebhook.getId());
                  setSelectedWebhookId(null);
                }}
              >
                Delete webhook
              </TableBatchAction>
            </>
          )}
        </TableBatchActions>
        <TableToolbarContent>
          <TableToolbarSearch
            placeholder="Search webhooks..."
            onChange={(e: any) => setSearchTerm(e.target?.value || '')}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={get}
          />
          <PrimaryButton
            size="md"
            renderIcon={Add}
            onClick={() => navigation.goToCreateAssistantWebhook(assistantId)}
          >
            Create new webhook
          </PrimaryButton>
        </TableToolbarContent>
      </TableToolbar>
      <TableSection>
        {axtion.webhooks.length > 0 && filteredWebhooks.length > 0 ? (
          <>
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader className="!w-12" />
                  <TableHeader>Endpoint</TableHeader>
                  <TableHeader>Events</TableHeader>
                  <TableHeader>Retries</TableHeader>
                  <TableHeader>Timeout (s)</TableHeader>
                  <TableHeader>Priority</TableHeader>
                  <TableHeader>Created</TableHeader>
                  <TableHeader>Actions</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredWebhooks.map(row => {
                  const selected = selectedWebhookId === row.getId();
                  return (
                    <TableRow
                      key={row.getId()}
                      isSelected={selected}
                      onClick={() =>
                        setSelectedWebhookId(selected ? null : row.getId())
                      }
                      className="cursor-pointer"
                    >
                      <TableCell
                        className="!w-12 !pr-0"
                        onClick={e => e.stopPropagation()}
                      >
                        <RadioButton
                          id={`webhook-select-${row.getId()}`}
                          name="webhook-select"
                          labelText=""
                          hideLabel
                          checked={selected}
                          onChange={() =>
                            setSelectedWebhookId(selected ? null : row.getId())
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <span className="font-mono text-xs">
                          {getWebhookMethod(row)}
                        </span>{' '}
                        <span className="truncate">{getWebhookUrl(row)}</span>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-1">
                          {getWebhookEvents(row).map((event, index) => (
                            <Tag key={index} type="blue" size="sm">
                              {event}
                            </Tag>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell>{getWebhookRetryCount(row)}</TableCell>
                      <TableCell>{getWebhookTimeoutSecond(row)}</TableCell>
                      <TableCell>{row.getExecutionpriority()}</TableCell>
                      <TableCell>
                        {row.getCreateddate()
                          ? toHumanReadableDateTime(row.getCreateddate()!)
                          : '—'}
                      </TableCell>
                      <TableCell onClick={e => e.stopPropagation()}>
                        <div className="flex items-center gap-0">
                          <Button
                            hasIconOnly
                            renderIcon={Edit}
                            iconDescription="Edit webhook"
                            kind="ghost"
                            size="sm"
                            onClick={() =>
                              navigation.goToEditAssistantWebhook(
                                assistantId,
                                row.getId(),
                              )
                            }
                          />
                          <Button
                            hasIconOnly
                            renderIcon={TrashCan}
                            iconDescription="Delete webhook"
                            kind="danger--ghost"
                            size="sm"
                            onClick={() =>
                              deleteAssistantWebhook(assistantId, row.getId())
                            }
                          />
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            <Pagination
              totalItems={axtion.totalCount}
              page={axtion.page}
              pageSize={axtion.pageSize}
              pageSizes={[10, 20, 50]}
              onChange={({ page: newPage, pageSize: newSize }) => {
                if (newSize !== axtion.pageSize) {
                  axtion.setPageSize(newSize);
                  return;
                }
                axtion.setPage(newPage);
              }}
            />
          </>
        ) : axtion.webhooks.length > 0 ? (
          <EmptyState
            className="w-full"
            icon={Webhook}
            title="No webhooks found"
            subtitle="No webhook matched your search."
          />
        ) : (
          <EmptyState
            className="w-full"
            icon={Webhook}
            title="No Webhook"
            subtitle="There are no assistant webhook found."
          />
        )}
      </TableSection>
    </div>
  );
};
