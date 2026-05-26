import { useEffect, useState } from 'react';
import {
  Assistant,
  AssistantConversation,
  AssistantConversationTelephonyEvent,
  Criteria,
} from '@rapidaai/react';
import { useCredential } from '@/hooks/use-credential';
import { useRapidaStore } from '@/hooks/use-rapida-store';
import toast from 'react-hot-toast/headless';
import { toDate, toDateString, toHumanReadableDateTime } from '@/utils/date';
import { useAssistantConversationListPageStore } from '@/hooks/use-assistant-conversation-list-page-store';
import { CarbonStatusIndicator } from '@/app/components/carbon/status-indicator';
import SourceIndicator from '@/app/components/indicators/source';
import { getStatusMetric, getConversationDuration } from '@/utils/metadata';
import { useGlobalNavigation } from '@/hooks/use-global-navigator';
import { ConversationDirectionIndicator } from '@/app/components/indicators/conversation-direction';
import { ConversationTelemetryDialog } from '@/app/components/base/modal/conversation-telemetry-modal';
import { CONFIG } from '@/configs';
import { AssistantConversationTelephonyEventDialog } from '@/app/components/base/modal/assistant-conversation-telephony-event-modal';
import { ChannelIndicator } from './channel-indicator';
import { normalizeDisconnectReason } from './disconnect-reason';
import { DisconnectReasonIndicator } from './disconnect-reason-indicator';
import { DurationBreakdownToggletip } from './duration-breakdown-toggletip';
import {
  getChannelValue,
  getDisconnectReasonValue,
} from './session-list.helpers';

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
  Loading,
  Link,
} from '@carbon/react';
import { Pagination } from '@/app/components/carbon/pagination';
import { IconOnlyButton } from '@/app/components/carbon/button';
import { DateFilter } from '@/app/components/carbon/date-filter';
import { EmptyState } from '@/app/components/carbon/empty-state';
import {
  Renew,
  Download,
  Launch,
  DataCheck,
  Phone,
  Chat,
} from '@carbon/icons-react';

interface ConversationProps {
  currentAssistant: Assistant;
}

export function Conversations({ currentAssistant }: ConversationProps) {
  const [userId, token, projectId] = useCredential();
  const [criterias, setCriterias] = useState<Criteria[]>([]);
  const [isTelemetryDialogOpen, setTelemetryDialogOpen] = useState(false);
  const [isTelephonyStatusOpen, setTelephonyStatusOpen] = useState(false);
  const [telephonyEvents, setTelephonyEvents] = useState<
    AssistantConversationTelephonyEvent[]
  >([]);
  const rapidaContext = useRapidaStore();
  const navigation = useGlobalNavigation();
  const assistantConversationListAction =
    useAssistantConversationListPageStore();

  const [searchValue, setSearchValue] = useState('');
  const [downloading, setDownloading] = useState(false);

  const onDateSelect = (to: Date, from: Date) => {
    assistantConversationListAction.setCriterias([
      {
        k: 'assistant_conversations.created_date',
        v: toDateString(from),
        logic: '>=',
      },
      {
        k: 'assistant_conversations.created_date',
        v: toDateString(to),
        logic: '<=',
      },
    ]);
  };

  const applySearch = (value: string) => {
    setSearchValue(value);
    if (value === '') {
      assistantConversationListAction.setCriterias([]);
      return;
    }
    const criterias: { k: string; v: string; logic: string }[] = [];
    const filterRegex = /(id|source|status):(\S+)/g;
    let match;
    while ((match = filterRegex.exec(value)) !== null) {
      const [, filterType, filterValue] = match;
      switch (filterType) {
        case 'id':
          criterias.push({
            k: 'assistant_conversations.id',
            v: filterValue,
            logic: '=',
          });
          break;
        case 'source':
          criterias.push({
            k: 'assistant_conversations.source',
            v: filterValue,
            logic: '=',
          });
          break;
        case 'status':
          criterias.push({
            k: 'assistant_conversations.status',
            v: filterValue,
            logic: '=',
          });
          break;
      }
    }
    if (criterias.length > 0) {
      assistantConversationListAction.setCriterias(criterias);
    }
  };

  useEffect(() => {
    assistantConversationListAction.clear();
  }, []);

  const get = () => {
    rapidaContext.showLoader();
    assistantConversationListAction.getAssistantConversations(
      currentAssistant.getId(),
      projectId,
      token,
      userId,
      (err: string) => {
        rapidaContext.hideLoader();
        toast.error(err);
      },
      (data: AssistantConversation[]) => {
        rapidaContext.hideLoader();
      },
    );
  };

  useEffect(() => {
    get();
  }, [
    currentAssistant.getId(),
    projectId,
    assistantConversationListAction.page,
    assistantConversationListAction.pageSize,
    assistantConversationListAction.criteria,
  ]);

  const handleTraceClick = (assistantId: string, conversationID: string) => {
    const ctr = new Criteria();
    ctr.setKey('conversationId');
    ctr.setLogic('match');
    ctr.setValue(conversationID);

    setCriterias([ctr]);
    setTelemetryDialogOpen(true);
  };

  const csvEscape = (str: string): string => {
    return `"${str.replace(/"/g, '""')}"`;
  };

  const onDownloadAllConversation = () => {
    setDownloading(true);
    const csvContent = [
      assistantConversationListAction.columns
        .filter(column => column.visible)
        .map(column => column.name)
        .join(','),
      ...assistantConversationListAction.assistantConversations.map(
        (row: AssistantConversation) =>
          assistantConversationListAction.columns
            .filter(column => column.visible)
            .map(column => {
              switch (column.key) {
                case 'id':
                  return row.getId();
                case 'assistant_id':
                  return row.getAssistantid();
                case 'assistant_provider_model_id':
                  return `vrsn_${row.getAssistantprovidermodelid()}`;
                case 'channel':
                  return getChannelValue(row);
                case 'identifier':
                  return csvEscape(row.getIdentifier());
                case 'source':
                  return row.getSource();
                case 'status':
                  return getStatusMetric(row.getMetricsList());
                case 'disconnect_reason':
                  return normalizeDisconnectReason(
                    getDisconnectReasonValue(row),
                  ).label;
                case 'created_date':
                  return row.getCreateddate()
                    ? toDate(row.getCreateddate()!)
                    : '';
                default:
                  return '';
              }
            })
            .join(','),
      ),
    ].join('\n');
    const url = URL.createObjectURL(
      new Blob([csvContent], { type: 'text/csv;charset=utf-8;' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.setAttribute('download', currentAssistant.getId() + '-sessions.csv');
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    setDownloading(false);
  };

  const visibleColumns = assistantConversationListAction.columns.filter(
    c => c.visible,
  );

  return (
    <div className="h-full flex flex-col flex-1">
      {isTelemetryDialogOpen && (
        <ConversationTelemetryDialog
          modalOpen={isTelemetryDialogOpen}
          setModalOpen={setTelemetryDialogOpen}
          assistantId={currentAssistant.getId()}
          criterias={criterias}
        />
      )}

      <AssistantConversationTelephonyEventDialog
        modalOpen={isTelephonyStatusOpen}
        setModalOpen={setTelephonyStatusOpen}
        events={telephonyEvents}
      />

      <TableToolbar>
        <TableToolbarContent>
          <TableToolbarSearch
            placeholder="Search by id:session-id, source:web, status:completed"
            value={searchValue}
            onChange={(e: any) => applySearch(e.target?.value || '')}
          />
          <DateFilter
            onApply={(from, to) => onDateSelect(to, from)}
            onReset={() => assistantConversationListAction.setCriterias([])}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Download}
            iconDescription="Export as CSV"
            isLoading={downloading}
            onClick={() => onDownloadAllConversation()}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={() => get()}
          />
        </TableToolbarContent>
      </TableToolbar>

      {rapidaContext.loading ? (
        <div className="flex items-center justify-center py-16">
          <Loading withOverlay={false} small />
        </div>
      ) : assistantConversationListAction.assistantConversations.length > 0 ? (
        <div className="overflow-auto flex-1">
          <Table>
            <TableHead>
              <TableRow>
                {visibleColumns.map(col => (
                  <TableHeader key={col.key}>{col.name}</TableHeader>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {assistantConversationListAction.assistantConversations.map(
                (row, idx) => (
                  <TableRow key={idx}>
                    {assistantConversationListAction.visibleColumn('id') && (
                      <TableCell className="text-sm">
                        <Link
                          href={`/deployment/assistant/${row.getAssistantid()}/sessions/${row.getId()}`}
                          className="!text-sm !inline-flex !items-center !gap-1"
                        >
                          <span>{row.getId()}</span>
                          <Launch size={12} />
                        </Link>
                      </TableCell>
                    )}
                    {assistantConversationListAction.visibleColumn(
                      'assistant_id',
                    ) && (
                      <TableCell className="text-sm">
                        {row.getAssistantid()}
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'assistant_provider_model_id',
                    ) && (
                      <TableCell className="font-mono text-[13px]">
                        vrsn_{row.getAssistantprovidermodelid()}
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'direction',
                    ) && (
                      <TableCell className="text-sm">
                        <ConversationDirectionIndicator
                          direction={row.getDirection() || 'inbound'}
                        />
                      </TableCell>
                    )}
                    {assistantConversationListAction.visibleColumn(
                      'channel',
                    ) && (
                      <TableCell className="min-w-[130px] whitespace-nowrap text-sm">
                        <ChannelIndicator channel={getChannelValue(row)} />
                      </TableCell>
                    )}
                    {assistantConversationListAction.visibleColumn(
                      'identifier',
                    ) && (
                      <TableCell className="min-w-[220px] max-w-[280px] truncate text-sm">
                        {row.getIdentifier()}
                      </TableCell>
                    )}
                    {assistantConversationListAction.visibleColumn(
                      'source',
                    ) && (
                      <TableCell className="text-sm">
                        <SourceIndicator source={row.getSource()} />
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'duration',
                    ) && (
                      <TableCell className="min-w-[150px] whitespace-nowrap text-sm tabular-nums">
                        <div className="flex items-center gap-1.5">
                          <span>
                            {getConversationDuration(row.getMetricsList())}
                          </span>
                          <DurationBreakdownToggletip conversation={row} />
                        </div>
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'action',
                    ) && (
                      <TableCell className="text-sm">
                        <div className="flex items-center gap-0">
                          {row.getTelephonyeventsList().length > 0 && (
                            <IconOnlyButton
                              kind="ghost"
                              size="md"
                              renderIcon={Phone}
                              iconDescription="View telephony status"
                              onClick={() => {
                                setTelephonyEvents(
                                  row.getTelephonyeventsList(),
                                );
                                setTelephonyStatusOpen(true);
                              }}
                            />
                          )}
                          {CONFIG.workspace.features?.telemetry !== false && (
                            <IconOnlyButton
                              kind="ghost"
                              size="md"
                              renderIcon={DataCheck}
                              iconDescription="View telemetry"
                              onClick={() =>
                                handleTraceClick(
                                  row.getAssistantid(),
                                  row.getId(),
                                )
                              }
                            />
                          )}
                          <IconOnlyButton
                            kind="ghost"
                            size="md"
                            renderIcon={Launch}
                            iconDescription="View conversation"
                            onClick={event => {
                              event.stopPropagation();
                              navigation.goToAssistantSession(
                                row.getAssistantid(),
                                row.getId(),
                              );
                            }}
                          />
                        </div>
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'disconnect_reason',
                    ) && (
                      <TableCell className="min-w-[180px] whitespace-nowrap text-sm">
                        <DisconnectReasonIndicator
                          reason={getDisconnectReasonValue(row)}
                        />
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'status',
                    ) && (
                      <TableCell className="text-sm">
                        <CarbonStatusIndicator
                          state={getStatusMetric(row.getMetricsList())}
                        />
                      </TableCell>
                    )}

                    {assistantConversationListAction.visibleColumn(
                      'created_date',
                    ) && (
                      <TableCell className="text-[13px] whitespace-nowrap">
                        {row.getCreateddate()
                          ? toHumanReadableDateTime(row.getCreateddate()!)
                          : '—'}
                      </TableCell>
                    )}
                  </TableRow>
                ),
              )}
            </TableBody>
          </Table>
        </div>
      ) : (
        <EmptyState
          icon={Chat}
          title="No conversations found"
          subtitle="Any conversations made with the assistant will be listed here."
        />
      )}

      {assistantConversationListAction.assistantConversations.length > 0 && (
        <Pagination
          className="shrink-0"
          totalItems={assistantConversationListAction.totalCount}
          page={assistantConversationListAction.page}
          pageSize={assistantConversationListAction.pageSize}
          pageSizes={[10, 20, 25, 50, 100]}
          onChange={({ page: p, pageSize: ps }) => {
            if (ps !== assistantConversationListAction.pageSize) {
              assistantConversationListAction.setPageSize(ps);
            } else {
              assistantConversationListAction.setPage(p);
            }
          }}
        />
      )}
    </div>
  );
}
