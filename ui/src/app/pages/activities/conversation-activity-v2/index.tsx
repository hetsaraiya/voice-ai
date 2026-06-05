import { useEffect, useMemo, useState } from 'react';
import {
  CodeSnippet,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Tag,
} from '@carbon/react';
import { Activity, Renew, WarningAlt } from '@carbon/icons-react';
import { Helmet } from '@/app/components/helmet';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { Tabs } from '@/app/components/carbon/tabs';
import { IconOnlyButton } from '@/app/components/carbon/button';
import { ScrollableTableSection } from '@/app/components/sections/table-section';
import { ConversationWaterfall } from './components/conversation-waterfall';
import { ExplorerFilter } from './components/explorer-filter';
import { SpanCountChart } from './components/span-count-chart';
import {
  ALL_EVENT_OPTION,
  SCOPE_OPTIONS,
  getEventOptionsForComponent,
} from './constants';
import type { MetricSummary, TimelineDocument, TraceSummary } from './types';
import {
  formatDateTime,
  formatDurationMs,
  formatTime,
  getDocumentComponent,
  getLatencyBuckets,
  getMetricSummaries,
  getSpanCountBuckets,
  getTraceSummaries,
  groupTimelineItems,
  matchesTimelineSearch,
  sampleTimelineDocuments,
} from './utils';

const getOutcomeTagType = (outcome: string): 'green' | 'red' | 'gray' => {
  if (outcome === 'success') return 'green';
  if (outcome === 'failure') return 'red';
  return 'gray';
};

const SpanSamplesTable = ({
  selectedDocumentId,
  spans,
  onSelectSpan,
}: {
  selectedDocumentId: string | undefined;
  spans: TimelineDocument[];
  onSelectSpan: (document: TimelineDocument) => void;
}) => (
  <ScrollableTableSection>
    <Table className="min-w-[1040px]">
      <TableHead>
        <TableRow>
          <TableHeader>ID</TableHeader>
          <TableHeader>Span name</TableHeader>
          <TableHeader>Component</TableHeader>
          <TableHeader>Event</TableHeader>
          <TableHeader>Duration</TableHeader>
          <TableHeader>Outcome</TableHeader>
          <TableHeader>Context ID</TableHeader>
          <TableHeader>Timestamp</TableHeader>
        </TableRow>
      </TableHead>
      <TableBody>
        {spans.map(document => (
          <TableRow
            key={document.id}
            className={[
              'cursor-pointer',
              document.id === selectedDocumentId
                ? '!bg-blue-50 dark:!bg-blue-950/20'
                : '',
            ].join(' ')}
            tabIndex={0}
            onClick={() => onSelectSpan(document)}
            onKeyDown={event => {
              if (event.key === 'Enter') onSelectSpan(document);
            }}
          >
            <TableCell className="font-mono text-xs text-blue-600">
              {document.id}
            </TableCell>
            <TableCell>
              <div className="max-w-[280px]">
                <p className="truncate text-sm font-medium">
                  {document.title || document.name}
                </p>
                <p className="font-mono text-xs text-gray-500">
                  {document.kind}
                </p>
              </div>
            </TableCell>
            <TableCell>
              <Tag type="cool-gray">{getDocumentComponent(document)}</Tag>
            </TableCell>
            <TableCell className="font-mono text-xs">{document.name}</TableCell>
            <TableCell className="font-mono text-xs">
              {formatDurationMs(document.durationMs)}
            </TableCell>
            <TableCell>
              <Tag type={getOutcomeTagType(document.outcome)}>
                {document.outcome || 'unknown'}
              </Tag>
            </TableCell>
            <TableCell className="font-mono text-xs">
              {document.contextId}
            </TableCell>
            <TableCell className="whitespace-nowrap text-xs">
              {formatDateTime(document.occurredAt)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </ScrollableTableSection>
);

const TraceTable = ({
  selectedContextId,
  traces,
  onSelectTrace,
}: {
  selectedContextId: string;
  traces: TraceSummary[];
  onSelectTrace: (trace: TraceSummary) => void;
}) => (
  <ScrollableTableSection>
    <Table className="min-w-[980px]">
      <TableHead>
        <TableRow>
          <TableHeader>Trace</TableHeader>
          <TableHeader>Context ID</TableHeader>
          <TableHeader>Conversation</TableHeader>
          <TableHeader>Spans</TableHeader>
          <TableHeader>Components</TableHeader>
          <TableHeader>Duration</TableHeader>
          <TableHeader>Outcome</TableHeader>
          <TableHeader>Started</TableHeader>
        </TableRow>
      </TableHead>
      <TableBody>
        {traces.map(trace => (
          <TableRow
            key={trace.contextId}
            className={[
              'cursor-pointer',
              trace.contextId === selectedContextId
                ? '!bg-blue-50 dark:!bg-blue-950/20'
                : '',
            ].join(' ')}
            tabIndex={0}
            onClick={() => onSelectTrace(trace)}
            onKeyDown={event => {
              if (event.key === 'Enter') onSelectTrace(trace);
            }}
          >
            <TableCell>
              <div className="max-w-[280px]">
                <p className="truncate text-sm font-medium">{trace.title}</p>
                <p className="font-mono text-xs text-gray-500">
                  assistant:{trace.assistantId}
                </p>
              </div>
            </TableCell>
            <TableCell className="font-mono text-xs">
              {trace.contextId}
            </TableCell>
            <TableCell className="font-mono text-xs">
              {trace.assistantConversationId}
            </TableCell>
            <TableCell className="tabular-nums">{trace.spanCount}</TableCell>
            <TableCell>
              <div className="flex max-w-[260px] flex-wrap gap-1">
                {trace.components.map(component => (
                  <Tag key={component} type="cool-gray">
                    {component}
                  </Tag>
                ))}
              </div>
            </TableCell>
            <TableCell className="font-mono text-xs">
              {formatDurationMs(trace.durationMs)}
            </TableCell>
            <TableCell>
              <Tag type={getOutcomeTagType(trace.outcome)}>{trace.outcome}</Tag>
            </TableCell>
            <TableCell className="whitespace-nowrap text-xs">
              {formatDateTime(trace.startMs)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </ScrollableTableSection>
);

const MetricsTable = ({
  metrics,
  onOpenContext,
}: {
  metrics: MetricSummary[];
  onOpenContext: (contextId: string) => void;
}) => (
  <ScrollableTableSection>
    <Table className="min-w-[860px]">
      <TableHead>
        <TableRow>
          <TableHeader>Metric</TableHeader>
          <TableHeader>Component</TableHeader>
          <TableHeader>Count</TableHeader>
          <TableHeader>Avg duration</TableHeader>
          <TableHeader>P95 duration</TableHeader>
          <TableHeader>Failures</TableHeader>
          <TableHeader>Slowest context</TableHeader>
        </TableRow>
      </TableHead>
      <TableBody>
        {metrics.map(metric => (
          <TableRow
            key={metric.component}
            className="cursor-pointer"
            tabIndex={0}
            onClick={() => onOpenContext(metric.slowestContextId)}
            onKeyDown={event => {
              if (event.key === 'Enter') onOpenContext(metric.slowestContextId);
            }}
          >
            <TableCell>
              <p className="text-sm font-medium">duration</p>
              <p className="text-xs text-gray-500">component latency</p>
            </TableCell>
            <TableCell>
              <Tag type="cool-gray">{metric.component}</Tag>
            </TableCell>
            <TableCell className="tabular-nums">{metric.count}</TableCell>
            <TableCell className="font-mono text-xs">
              {formatDurationMs(metric.averageDurationMs)}
            </TableCell>
            <TableCell className="font-mono text-xs">
              {formatDurationMs(metric.p95DurationMs)}
            </TableCell>
            <TableCell className="tabular-nums">
              {metric.failureCount}
            </TableCell>
            <TableCell className="font-mono text-xs">
              {metric.slowestContextId}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </ScrollableTableSection>
);

const DetailPanel = ({ document }: { document: TimelineDocument | null }) => {
  if (!document) {
    return (
      <div className="flex h-full items-center justify-center border-l border-gray-200 bg-gray-50 px-6 text-center text-sm text-gray-500 dark:border-gray-800 dark:bg-gray-950">
        Select a timeline record to inspect attributes and payload.
      </div>
    );
  }

  return (
    <aside className="flex h-full min-w-[360px] max-w-[420px] flex-col border-l border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-950">
      <div className="border-b border-gray-200 px-4 py-3 dark:border-gray-800">
        <div className="mb-2 flex items-center gap-2">
          <Tag type={document.outcome === 'failure' ? 'red' : 'green'}>
            {document.outcome || 'unknown'}
          </Tag>
          <Tag type="cool-gray">{document.kind}</Tag>
          <Tag type="purple">{document.scope}</Tag>
        </div>
        <h2 className="text-base font-medium text-gray-900 dark:text-gray-100">
          {document.title || document.name}
        </h2>
        <p className="mt-1 break-all font-mono text-xs text-gray-500">
          {document.id}
        </p>
      </div>
      <div className="space-y-4 overflow-auto p-4">
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <p className="text-xs uppercase text-gray-500">Occurred</p>
            <p className="font-mono text-xs">
              {formatTime(document.occurredAt)}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase text-gray-500">Duration</p>
            <p className="font-mono text-xs">
              {formatDurationMs(document.durationMs)}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase text-gray-500">Category</p>
            <p className="font-mono text-xs">{document.category || '-'}</p>
          </div>
          <div>
            <p className="text-xs uppercase text-gray-500">Context</p>
            <p className="break-all font-mono text-xs">
              {document.contextId || '-'}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase text-gray-500">Message</p>
            <p className="break-all font-mono text-xs">
              {document.messageId || '-'}
            </p>
          </div>
          <div>
            <p className="text-xs uppercase text-gray-500">Role</p>
            <p className="font-mono text-xs">{document.messageRole || '-'}</p>
          </div>
        </div>
        <div>
          <p className="mb-2 text-xs font-medium uppercase text-gray-500">
            Attributes
          </p>
          <CodeSnippet type="multi" feedback="Copied">
            {JSON.stringify(document.attributes || {}, null, 2)}
          </CodeSnippet>
        </div>
        <div>
          <p className="mb-2 text-xs font-medium uppercase text-gray-500">
            Data
          </p>
          <CodeSnippet type="multi" feedback="Copied">
            {JSON.stringify(document.data || {}, null, 2)}
          </CodeSnippet>
        </div>
      </div>
    </aside>
  );
};

export const ListingPage = () => {
  const [searchText, setSearchText] = useState('');
  const [selectedScope, setSelectedScope] = useState(SCOPE_OPTIONS[2]);
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);
  const [selectedEvent, setSelectedEvent] = useState(ALL_EVENT_OPTION);
  const [dateRange, setDateRange] = useState<[Date, Date] | null>(null);
  const [activeTab, setActiveTab] = useState(0);
  const [selectedContextId, setSelectedContextId] = useState(
    sampleTimelineDocuments[0]?.contextId || '',
  );
  const [selectedDocument, setSelectedDocument] =
    useState<TimelineDocument | null>(sampleTimelineDocuments[0] || null);

  const selectedComponentId = selectedComponents[0] || 'all';

  const eventFilterOptions = useMemo(
    () => getEventOptionsForComponent(selectedComponentId),
    [selectedComponentId],
  );

  const filteredDocuments = useMemo(
    () =>
      sampleTimelineDocuments.filter(document => {
        const occurredMs = new Date(document.occurredAt).getTime();
        const matchesDate =
          !dateRange ||
          (occurredMs >= dateRange[0].getTime() &&
            occurredMs <= dateRange[1].getTime() + 24 * 60 * 60 * 1000);

        return (
          matchesTimelineSearch(document, searchText) &&
          matchesDate &&
          document.scope === selectedScope.id &&
          (selectedComponents.length === 0 ||
            selectedComponents.includes(getDocumentComponent(document))) &&
          (selectedEvent.id === ALL_EVENT_OPTION.id ||
            document.name === selectedEvent.id)
        );
      }),
    [dateRange, searchText, selectedComponents, selectedEvent, selectedScope],
  );

  const traceSummaries = useMemo(
    () => getTraceSummaries(filteredDocuments),
    [filteredDocuments],
  );
  const metricSummaries = useMemo(
    () => getMetricSummaries(filteredDocuments),
    [filteredDocuments],
  );
  const chartBuckets = useMemo(
    () => getSpanCountBuckets(filteredDocuments),
    [filteredDocuments],
  );
  const latencyBuckets = useMemo(
    () => getLatencyBuckets(filteredDocuments),
    [filteredDocuments],
  );
  const selectedTimelineDocuments = useMemo(
    () =>
      filteredDocuments.filter(
        document => document.contextId === selectedContextId,
      ),
    [filteredDocuments, selectedContextId],
  );
  const selectedGroups = useMemo(
    () => groupTimelineItems(selectedTimelineDocuments),
    [selectedTimelineDocuments],
  );

  useEffect(() => {
    const currentContextExists = traceSummaries.some(
      trace => trace.contextId === selectedContextId,
    );

    if (currentContextExists) return;

    const nextTrace = traceSummaries[0];
    setSelectedContextId(nextTrace?.contextId || '');
    setSelectedDocument(
      nextTrace
        ? filteredDocuments.find(
            document => document.contextId === nextTrace.contextId,
          ) || null
        : null,
    );
  }, [filteredDocuments, selectedContextId, traceSummaries]);

  const activeFilterIds = useMemo(() => {
    const ids = new Set<string>();
    if (selectedScope.id !== SCOPE_OPTIONS[2].id) ids.add('scope');
    if (selectedComponents.length > 0) ids.add('component');
    if (selectedEvent.id !== ALL_EVENT_OPTION.id) ids.add('event');
    if (dateRange) ids.add('date');
    return ids;
  }, [dateRange, selectedComponents, selectedEvent, selectedScope]);

  const resetExplorerFilters = () => {
    setSelectedScope(SCOPE_OPTIONS[2]);
    setSelectedComponents([]);
    setSelectedEvent(ALL_EVENT_OPTION);
    setDateRange(null);
  };

  useEffect(() => {
    if (
      selectedEvent.id !== ALL_EVENT_OPTION.id &&
      !eventFilterOptions.some(option => option.id === selectedEvent.id)
    ) {
      setSelectedEvent(ALL_EVENT_OPTION);
    }
  }, [eventFilterOptions, selectedEvent]);

  const openContext = (contextId: string) => {
    setSelectedContextId(contextId);
    setSelectedDocument(
      filteredDocuments.find(document => document.contextId === contextId) ||
        null,
    );
  };

  const selectSpan = (document: TimelineDocument) => {
    setSelectedContextId(document.contextId);
    setSelectedDocument(document);
  };

  const selectTrace = (trace: TraceSummary) => {
    openContext(trace.contextId);
    setActiveTab(1);
  };

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <Helmet title="Conversation Explorer" />

      <TableToolbar>
        <TableToolbarContent>
          <TableToolbarSearch
            placeholder="Search for spans, users, tags, and more"
            value={searchText}
            onChange={(event: any) => setSearchText(event.target?.value || '')}
          />
          <ExplorerFilter
            activeFilterIds={activeFilterIds}
            dateRange={dateRange}
            eventOptions={eventFilterOptions}
            selectedComponentId={selectedComponentId}
            selectedEvent={selectedEvent}
            selectedScope={selectedScope}
            onComponentChange={componentId =>
              setSelectedComponents(componentId === 'all' ? [] : [componentId])
            }
            onDateRangeChange={setDateRange}
            onEventChange={setSelectedEvent}
            onReset={resetExplorerFilters}
            onScopeChange={setSelectedScope}
          />
          <IconOnlyButton
            kind="ghost"
            size="lg"
            renderIcon={Renew}
            iconDescription="Refresh"
            onClick={() => undefined}
          />
        </TableToolbarContent>
      </TableToolbar>

      <SpanCountChart buckets={chartBuckets} latencyBuckets={latencyBuckets} />

      {filteredDocuments.length === 0 ? (
        <EmptyState
          icon={searchText ? WarningAlt : Activity}
          title="No telemetry records found"
          subtitle="Adjust search, scope, component, event, or date filters."
        />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">
          <Tabs
            tabs={[
              `Span Samples (${filteredDocuments.length})`,
              `Trace Samples (${traceSummaries.length})`,
              `Aggregates (${metricSummaries.length})`,
            ]}
            selectedIndex={activeTab}
            onChange={setActiveTab}
            contained
            fill
            panelClassName="!p-0"
            panelsClassName="border-t border-gray-200 dark:border-gray-800"
          >
            <SpanSamplesTable
              selectedDocumentId={selectedDocument?.id}
              spans={filteredDocuments}
              onSelectSpan={selectSpan}
            />
            <TraceTable
              selectedContextId={selectedContextId}
              traces={traceSummaries}
              onSelectTrace={selectTrace}
            />
            <MetricsTable
              metrics={metricSummaries}
              onOpenContext={openContext}
            />
          </Tabs>

          {selectedGroups.length > 0 && (
            <section className="flex min-h-[280px] max-h-[380px] border-t border-gray-200 dark:border-gray-800">
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="flex items-center justify-between border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-800 dark:bg-gray-900">
                  <div>
                    <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                      Timeline
                    </p>
                    <p className="font-mono text-xs text-gray-500">
                      contextId:{selectedContextId}
                    </p>
                  </div>
                  <Tag type="cool-gray">
                    {selectedTimelineDocuments.length} spans
                  </Tag>
                </div>
                <ConversationWaterfall
                  groups={selectedGroups}
                  selectedDocumentId={selectedDocument?.id}
                  onSelectDocument={setSelectedDocument}
                />
              </div>
              <DetailPanel document={selectedDocument} />
            </section>
          )}
        </div>
      )}
    </div>
  );
};
