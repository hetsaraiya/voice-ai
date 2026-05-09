import React, { useState, useEffect } from 'react';
import { Helmet } from '@/app/components/helmet';
import { DateFilter } from '@/app/components/carbon/date-filter';
import { useCredential } from '@/hooks/use-credential';
import toast from 'react-hot-toast/headless';
import { useRapidaStore } from '@/hooks';
import { Metadata } from '@rapidaai/react';
import { TableLink } from '@/app/components/carbon/table-link';
import { useActivityLogPage } from '@/hooks/use-activity-log-page-store';
import {
  formatNanoToReadableMilli,
  toDateString,
  toHumanReadableDateTime,
} from '@/utils/date';
import { getMetadataValue, getMetricValueOrDefault } from '@/utils/metadata';
import { LLMLogDialog } from '@/app/components/base/modal/llm-log-modal';
import { HttpStatusSpanIndicator } from '@/app/components/indicators/http-status';
import { PageTitleWithCount } from '@/app/components/blocks/page-title-with-count';
import { PageHeaderBlock } from '@/app/components/blocks/page-header-block';
import { CarbonStatusIndicator } from '@/app/components/carbon/status-indicator';
import { Pagination } from '@/app/components/carbon/pagination';
import { IconOnlyButton } from '@/app/components/carbon/button';
import { Renew, View, Launch, Ai } from '@carbon/icons-react';
import { ProviderTag } from '@/app/components/carbon/provider-tag';
import { EmptyState } from '@/app/components/carbon/empty-state';
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
} from '@carbon/react';

export function ListingPage() {
  const { loading, showLoader, hideLoader } = useRapidaStore();
  const [userId, token, projectId] = useCredential();
  const [currentActivityId, setCurrentActivityId] = useState('');
  const [showLogModal, setShowLogModal] = useState(false);

  const {
    getActivities,
    addCriterias,
    activities,
    columns,
    page,
    setPage,
    totalCount,
    criteria,
    pageSize,
    visibleColumn,
    setPageSize,
  } = useActivityLogPage();

  const onDateSelect = (to: Date, from: Date) => {
    addCriterias([
      { k: 'created_date', v: toDateString(to), logic: '<=' },
      { k: 'created_date', v: toDateString(from), logic: '>=' },
    ]);
  };

  useEffect(() => {
    showLoader();
    onGetAcitvities();
  }, [projectId, page, pageSize, JSON.stringify(criteria)]);

  const onGetAcitvities = () => {
    getActivities(
      projectId,
      token,
      userId,
      err => {
        hideLoader();
        toast.error(err);
      },
      logs => {
        hideLoader();
      },
    );
  };

  const visibleColumns = columns.filter(c => c.visible);

  return (
    <>
      {currentActivityId && (
        <LLMLogDialog
          modalOpen={showLogModal}
          setModalOpen={setShowLogModal}
          currentActivityId={currentActivityId}
        />
      )}

      <Helmet title="LLM Logs" />
      <PageHeaderBlock>
        <PageTitleWithCount count={activities.length} total={totalCount}>
          LLM Logs
        </PageTitleWithCount>
      </PageHeaderBlock>

      <TableToolbar>
        <TableToolbarContent>
          <TableToolbarSearch placeholder="Search LLM logs" />
          <DateFilter
            onApply={(from, to) => onDateSelect(to, from)}
            onReset={() => addCriterias([])}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={() => onGetAcitvities()}
          />
        </TableToolbarContent>
      </TableToolbar>

      {loading ? (
        <div className="flex items-center justify-center py-16">
          <Loading withOverlay={false} small />
        </div>
      ) : activities.length > 0 ? (
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
              {activities.map((at, idx) => (
                <TableRow key={idx}>
                  {visibleColumn('Source') && (
                    <TableCell>
                      <ActivitySource
                        metadatas={at.getExternalauditmetadatasList()}
                      />
                    </TableCell>
                  )}
                  {visibleColumn('Provider Name') && (
                    <TableCell>
                      <ProviderTag
                        provider={getMetadataValue(
                          at.getExternalauditmetadatasList(),
                          'provider_name',
                        )}
                      />
                    </TableCell>
                  )}
                  {visibleColumn('Model Name') && (
                    <TableCell>
                      {getMetadataValue(
                        at.getExternalauditmetadatasList(),
                        'model_name',
                      )}
                    </TableCell>
                  )}
                  {visibleColumn('Created Date') && (
                    <TableCell className="!text-xs whitespace-nowrap">
                      {at.getCreateddate() &&
                        toHumanReadableDateTime(at.getCreateddate()!)}
                    </TableCell>
                  )}
                  {visibleColumn('Action') && (
                    <TableCell>
                      <div className="flex items-center gap-0">
                        <IconOnlyButton
                          kind="ghost"
                          size="md"
                          renderIcon={View}
                          iconDescription="View detail"
                          onClick={() => {
                            setCurrentActivityId(at.getId());
                            setShowLogModal(true);
                          }}
                        />
                        <IconOnlyButton
                          kind="ghost"
                          size="md"
                          renderIcon={Launch}
                          iconDescription="View conversation"
                          onClick={() => {
                            const link = getActivityLink(
                              at.getExternalauditmetadatasList(),
                            ).link;
                            if (link) window.location.href = link;
                          }}
                        />
                      </div>
                    </TableCell>
                  )}
                  {visibleColumn('Status') && (
                    <TableCell>
                      <CarbonStatusIndicator state={at.getStatus()} />
                    </TableCell>
                  )}
                    {visibleColumn('TTFT') && (
                    <TableCell className="font-mono text-xs">
                      {formatNanoToReadableMilli(
                        getMetricValueOrDefault(
                        at.getMetricsList(),
                        'time_to_first_token',
                        "0",
                      )) }
                    </TableCell>
                  )}
                  {visibleColumn('TRT') && (
                    <TableCell className="font-mono text-xs">
                      {formatNanoToReadableMilli(at.getTimetaken())}
                    </TableCell>
                  )}

                  {visibleColumn('Http_status') && (
                    <TableCell>
                      <HttpStatusSpanIndicator
                        status={at.getResponsestatus()}
                      />
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <EmptyState
          icon={Ai}
          title="No LLM activities found"
          subtitle="Requests made to LLM providers like OpenAI, Anthropic, and Google will appear here as your assistants process conversations."
        />
      )}

      {activities.length > 0 && (
        <Pagination
          totalItems={totalCount}
          page={page}
          pageSize={pageSize}
          pageSizes={[10, 20, 25, 50, 100]}
          onChange={({ page: p, pageSize: ps }) => {
            if (ps !== pageSize) setPageSize(ps);
            else setPage(p);
          }}
        />
      )}
    </>
  );
}

function ActivitySource(props: { metadatas: Metadata[] }) {
  const { source, link } = getActivityLink(props.metadatas);
  return link ? (
    <TableLink href={link}>{source}</TableLink>
  ) : (
    <span className="text-xs">{source}</span>
  );
}

function getActivityLink(metadatas: Metadata[]): {
  source: string;
  link: string;
} {
  const endpoint = getMetadataValue(metadatas, 'endpoint_id');
  if (endpoint)
    return { source: endpoint, link: `/deployment/endpoint/${endpoint}` };

  const assistant = getMetadataValue(metadatas, 'assistant_id');
  if (assistant)
    return { source: assistant, link: `/deployment/assistant/${assistant}` };

  const knowledge = getMetadataValue(metadatas, 'knowledge_id');
  if (knowledge) return { source: knowledge, link: `/knowledge/${knowledge}` };

  const source = getMetadataValue(metadatas, 'source');
  if (source) return { source, link: '' };

  return { source: '', link: '' };
}
