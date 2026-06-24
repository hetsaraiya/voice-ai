import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  Button,
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
  Loading,
} from '@carbon/react';
import {
  Activity,
  Close,
  Filter,
  Renew,
  WarningAlt,
} from '@carbon/icons-react';
import {
  ConnectionConfig,
  Criteria,
  GetAllTelemetry,
  GetAllTelemetryRequest,
  Ordering,
  Paginate,
} from '@rapidaai/react';
import toast from 'react-hot-toast/headless';
import { Helmet } from '@/app/components/helmet';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { Pagination } from '@/app/components/carbon/pagination';
import { ScrollableTableSection } from '@/app/components/sections/table-section';
import { CopyButton } from '@/app/components/carbon/button/copy-button';
import { connectionConfig } from '@/configs';
import { useCurrentCredential } from '@/hooks/use-credential';
import { ConversationWaterfall } from './components/conversation-waterfall';
import { ExplorerFilter } from './components/explorer-filter';
import {
  ALL_EVENT_OPTION,
  FilterOption,
  KIND_OPTIONS,
  LEVEL_OPTIONS,
  METRIC_NAME_OPTIONS,
  ROLE_OPTIONS,
  SCOPE_OPTIONS,
  getEventOptionsForComponent,
} from './constants';
import type { TimelineDocument } from './types';
import {
  formatDateTime,
  formatDurationMs,
  formatTime,
  getDocumentComponent,
  groupTimelineItems,
  matchesTimelineSearch,
  telemetryRecordToTimelineDocument,
} from './utils';

type MetricValue = {
  description?: string;
  name?: string;
  value?: number | string;
};

type TraceFilterState = {
  assistantIdInput: string;
  conversationIdInput: string;
  dateRange: [Date, Date] | null;
  messageIdInput: string;
  metricNameInput: string;
  searchText: string;
  selectedComponents: string[];
  selectedEvent: FilterOption;
  selectedKind: FilterOption;
  selectedLevel: FilterOption;
  selectedRole: FilterOption;
  selectedScope: FilterOption;
  traceIdInput: string;
};

const DEFAULT_TRACE_FILTERS: TraceFilterState = {
  assistantIdInput: '',
  conversationIdInput: '',
  dateRange: null,
  messageIdInput: '',
  metricNameInput: METRIC_NAME_OPTIONS[0].id,
  searchText: '',
  selectedComponents: [],
  selectedEvent: ALL_EVENT_OPTION,
  selectedKind: KIND_OPTIONS[0],
  selectedLevel: LEVEL_OPTIONS[0],
  selectedRole: ROLE_OPTIONS[0],
  selectedScope: SCOPE_OPTIONS[0],
  traceIdInput: '',
};

const getQueryValue = (
  searchParams: URLSearchParams,
  keys: string[],
): string => {
  for (const key of keys) {
    const value = searchParams.get(key)?.trim();
    if (value) return value;
  }
  return '';
};

const getFilterOption = (
  options: FilterOption[],
  id: string,
  fallback: FilterOption,
): FilterOption => options.find(option => option.id === id) || fallback;

const getTraceFiltersFromSearchParams = (
  searchParams: URLSearchParams,
): TraceFilterState => {
  const assistantId = getQueryValue(searchParams, [
    'assistant_id',
    'assistantId',
  ]);
  const conversationId = getQueryValue(searchParams, [
    'conversation_id',
    'conversationId',
    'assistant_conversation_id',
    'assistantConversationId',
  ]);
  const messageId = getQueryValue(searchParams, ['message_id', 'messageId']);
  const scopeQuery = getQueryValue(searchParams, ['scope']);

  let selectedScope = getFilterOption(
    SCOPE_OPTIONS,
    scopeQuery,
    DEFAULT_TRACE_FILTERS.selectedScope,
  );

  if (!scopeQuery) {
    if (messageId)
      selectedScope = getFilterOption(
        SCOPE_OPTIONS,
        'message',
        SCOPE_OPTIONS[0],
      );
    else if (conversationId)
      selectedScope = getFilterOption(
        SCOPE_OPTIONS,
        'conversation',
        SCOPE_OPTIONS[0],
      );
    else if (assistantId)
      selectedScope = getFilterOption(
        SCOPE_OPTIONS,
        'assistant',
        SCOPE_OPTIONS[0],
      );
  }

  return {
    ...DEFAULT_TRACE_FILTERS,
    assistantIdInput: selectedScope.id === 'assistant' ? assistantId : '',
    conversationIdInput:
      selectedScope.id === 'conversation' ? conversationId : '',
    messageIdInput: selectedScope.id === 'message' ? messageId : '',
    searchText: getQueryValue(searchParams, ['q', 'search']),
    selectedRole:
      selectedScope.id === 'message'
        ? getFilterOption(
            ROLE_OPTIONS,
            getQueryValue(searchParams, [
              'message_role',
              'messageRole',
              'role',
            ]),
            DEFAULT_TRACE_FILTERS.selectedRole,
          )
        : DEFAULT_TRACE_FILTERS.selectedRole,
    selectedScope,
    traceIdInput: getQueryValue(searchParams, [
      'trace_id',
      'traceId',
      'traceID',
    ]),
  };
};

const TRACE_LOAD_ERROR_MESSAGE = 'Unable to load trace records.';

const getTelemetryErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message) return error.message;
  if (typeof error === 'string' && error.trim()) return error;
  return TRACE_LOAD_ERROR_MESSAGE;
};

const getMetricValues = (document: TimelineDocument): MetricValue[] =>
  (
    (document.data?.metrics as MetricValue[] | undefined)?.filter(Boolean) || [
      {
        description: document.data?.description as string | undefined,
        name: document.name,
      },
    ]
  ).filter(metric => metric.name || metric.value || metric.description);

const getMetricSummary = (document: TimelineDocument) =>
  getMetricValues(document)[0];

const getRelatedRecords = (
  document: TimelineDocument,
  records: TimelineDocument[],
) => {
  const relatedRecords = records.filter(
    record => record.kind === document.kind,
  );
  return relatedRecords.length > 0 ? relatedRecords : [document];
};

const getLeftPanelTitle = (document: TimelineDocument) => {
  if (document.kind === 'log') return 'Logs';
  if (document.kind === 'metric') return 'Metrics';
  return 'Timeline';
};

const getRecordCountLabel = (document: TimelineDocument, count: number) => {
  if (document.kind === 'log') {
    return `${count} ${count === 1 ? 'log' : 'logs'}`;
  }
  if (document.kind === 'metric') {
    return `${count} ${count === 1 ? 'metric' : 'metrics'}`;
  }
  return `${count} ${count === 1 ? 'event' : 'events'}`;
};

const LogRecordList = ({
  records,
  selectedDocumentId,
  onSelectDocument,
}: {
  records: TimelineDocument[];
  selectedDocumentId?: string;
  onSelectDocument: (document: TimelineDocument) => void;
}) => (
  <div className="min-h-0 flex-1 overflow-auto border-t border-gray-200 dark:border-gray-800">
    {records.map(record => (
      <button
        key={record.id}
        type="button"
        className={[
          'w-full border-t border-gray-100 bg-white px-4 py-3 text-left first:border-t-0 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-950 dark:hover:bg-gray-900',
          record.id === selectedDocumentId
            ? 'outline outline-2 -outline-offset-2 outline-[var(--cds-border-interactive)]'
            : '',
        ].join(' ')}
        onClick={() => onSelectDocument(record)}
      >
        <div className="mb-2 flex min-w-0 items-center gap-3 font-mono text-xs text-gray-500">
          <span
            className={
              record.level.toLowerCase() === 'error'
                ? 'text-red-600 dark:text-red-400'
                : ''
            }
          >
            {record.level.toLowerCase()}
          </span>
          <span className="truncate">{getDocumentComponent(record)}</span>
          <span className="ml-auto whitespace-nowrap">
            {formatTime(record.occurredAt)}
          </span>
        </div>
        <p className="truncate font-mono text-sm text-gray-900 dark:text-gray-100">
          [{record.level.toLowerCase()}] {record.title || record.name}
        </p>
      </button>
    ))}
  </div>
);

const MetricRecordList = ({
  records,
  selectedDocumentId,
  onSelectDocument,
}: {
  records: TimelineDocument[];
  selectedDocumentId?: string;
  onSelectDocument: (document: TimelineDocument) => void;
}) => (
  <div className="min-h-0 flex-1 overflow-auto border-t border-gray-200 dark:border-gray-800">
    {records.map(record => (
      <button
        key={record.id}
        type="button"
        className={[
          'w-full border-t border-gray-100 bg-white px-4 py-3 text-left first:border-t-0 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-950 dark:hover:bg-gray-900',
          record.id === selectedDocumentId
            ? 'outline outline-2 -outline-offset-2 outline-[var(--cds-border-interactive)]'
            : '',
        ].join(' ')}
        onClick={() => onSelectDocument(record)}
      >
        <div className="mb-2 flex min-w-0 items-center gap-2">
          <Tag type="cool-gray">{getDocumentComponent(record)}</Tag>
          <span className="ml-auto whitespace-nowrap font-mono text-xs text-gray-500">
            {formatTime(record.occurredAt)}
          </span>
        </div>
        <div className="space-y-1">
          {getMetricValues(record).map((metric, index) => (
            <p
              key={`${record.id}-${metric.name || index}`}
              className="truncate font-mono text-sm text-gray-900 dark:text-gray-100"
            >
              [{metric.name || record.name}] {metric.value ?? '-'}
              {metric.description && (
                <span className="text-gray-500"> {metric.description}</span>
              )}
            </p>
          ))}
        </div>
      </button>
    ))}
  </div>
);

const InspectorPrimaryPanel = ({
  document,
  records,
  selectedDocumentId,
  onSelectDocument,
}: {
  document: TimelineDocument;
  records: TimelineDocument[];
  selectedDocumentId?: string;
  onSelectDocument: (document: TimelineDocument) => void;
}) => {
  const relatedRecords = getRelatedRecords(document, records);
  const groups = groupTimelineItems(relatedRecords);

  return (
    <div className="flex min-w-0 min-h-0 flex-col border-r border-gray-200 dark:border-gray-800">
      <div className="flex items-center justify-between border-b border-gray-200 bg-gray-50 px-4 py-2 dark:border-gray-800 dark:bg-gray-900">
        <div className="min-w-0">
          <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
            {getLeftPanelTitle(document)}
          </p>
        </div>
        <Tag type="cool-gray">
          {getRecordCountLabel(document, relatedRecords.length)}
        </Tag>
      </div>
      {document.kind === 'log' ? (
        <LogRecordList
          records={relatedRecords}
          selectedDocumentId={selectedDocumentId}
          onSelectDocument={onSelectDocument}
        />
      ) : document.kind === 'metric' ? (
        <MetricRecordList
          records={relatedRecords}
          selectedDocumentId={selectedDocumentId}
          onSelectDocument={onSelectDocument}
        />
      ) : (
        <ConversationWaterfall
          groups={groups}
          selectedDocumentId={selectedDocumentId}
          onSelectDocument={onSelectDocument}
        />
      )}
    </div>
  );
};

const TelemetryStreamTable = ({
  selectedDocumentId,
  records,
  onSelectRecord,
}: {
  selectedDocumentId: string | undefined;
  records: TimelineDocument[];
  onSelectRecord: (document: TimelineDocument) => void;
}) => (
  <ScrollableTableSection>
    <Table className="min-w-[1040px]">
      <TableHead>
        <TableRow>
          <TableHeader>ID</TableHeader>
          <TableHeader>traceID</TableHeader>
          <TableHeader>Kind</TableHeader>
          <TableHeader>Scope</TableHeader>
          <TableHeader>Summary</TableHeader>
          <TableHeader>Occurred at</TableHeader>
        </TableRow>
      </TableHead>
      <TableBody>
        {records.map(document => (
          <TableRow
            key={document.id}
            className={[
              'cursor-pointer',
              document.id === selectedDocumentId
                ? 'outline outline-2 -outline-offset-2 outline-[var(--cds-border-interactive)]'
                : '',
            ].join(' ')}
            tabIndex={0}
            onClick={() => onSelectRecord(document)}
            onKeyDown={event => {
              if (event.key === 'Enter') onSelectRecord(document);
            }}
          >
            <TableCell className="max-w-[220px] truncate font-mono text-sm text-blue-600">
              {document.id}
            </TableCell>
            <TableCell className="max-w-[260px]">
              <div className="flex min-w-0 items-center gap-1">
                <span className="truncate font-mono text-[13px]">
                  {document.traceId || '-'}
                </span>
                {document.traceId && (
                  <span
                    className="shrink-0"
                    onClick={event => event.stopPropagation()}
                  >
                    <CopyButton className="h-6 w-6">
                      {document.traceId}
                    </CopyButton>
                  </span>
                )}
              </div>
            </TableCell>
            <TableCell>
              <Tag type="cool-gray">{document.kind}</Tag>
            </TableCell>
            <TableCell>
              <Tag type="purple">{document.scope}</Tag>
            </TableCell>
            <TableCell>
              <div className="max-w-[520px]">
                {document.kind === 'metric' ? (
                  <p className="truncate font-mono text-[13px]">
                    [{document.name}] {getMetricSummary(document)?.value}
                    <span className="text-gray-500">
                      {' '}
                      {getMetricSummary(document)?.description}
                    </span>
                  </p>
                ) : (
                  <p className="truncate font-mono text-[13px]">
                    {document.kind === 'log'
                      ? `[${document.level.toLowerCase()}] ${
                          document.title || document.name
                        }`
                      : `[${getDocumentComponent(document)}] ${
                          document.title || document.name
                        }`}
                  </p>
                )}
              </div>
            </TableCell>
            <TableCell className="text-[13px]! whitespace-nowrap">
              {formatDateTime(document.occurredAt)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </ScrollableTableSection>
);

const TraceInspectorPanel = ({
  document,
  records,
  recordCount,
  selectedContextId,
  selectedDocumentId,
  onSelectDocument,
  onClose,
}: {
  document: TimelineDocument | null;
  records: TimelineDocument[];
  recordCount: number;
  selectedContextId: string;
  selectedDocumentId?: string;
  onSelectDocument: (document: TimelineDocument) => void;
  onClose: () => void;
}) => (
  <div
    className={[
      'absolute inset-x-0 bottom-0 z-30 border-t border-gray-200 bg-white shadow-2xl transition-transform duration-200 ease-out dark:border-gray-800 dark:bg-gray-950',
      document ? 'translate-y-0' : 'pointer-events-none translate-y-full',
    ].join(' ')}
    aria-hidden={!document}
  >
    {document && (
      <section className="relative flex max-h-[62vh] min-h-[360px] flex-col">
        <Button
          hasIconOnly
          kind="ghost"
          size="lg"
          renderIcon={Close}
          iconDescription="Close"
          tooltipPosition="left"
          className="!absolute !right-0 !top-0 !z-10 !h-12 !min-h-12 !w-12 !min-w-12 !p-0 border-l border-gray-200 dark:border-gray-800"
          onClick={onClose}
        />
        <div className="border-b border-gray-200 px-4 py-3 pr-14 dark:border-gray-800">
          <div className="min-w-0">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <Tag type={document.outcome === 'failure' ? 'red' : 'green'}>
                {document.outcome || 'unknown'}
              </Tag>
              <Tag type="cool-gray">{document.kind}</Tag>
              <Tag type="purple">{document.scope}</Tag>
            </div>
            <h2 className="truncate text-base font-medium text-gray-900 dark:text-gray-100">
              {document.title || document.name}
            </h2>
            <p className="mt-1 font-mono text-xs text-gray-500">
              contextId:{selectedContextId || '-'} · {recordCount} records
            </p>
          </div>
        </div>
        <div className="grid min-h-0 flex-1 overflow-hidden lg:grid-cols-[minmax(0,1.3fr)_minmax(420px,0.7fr)]">
          <InspectorPrimaryPanel
            document={document}
            records={records}
            selectedDocumentId={selectedDocumentId}
            onSelectDocument={onSelectDocument}
          />
          <div className="min-h-0 overflow-auto p-4">
            <div className="mb-4 grid grid-cols-2 gap-3 text-sm">
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
                <p className="text-xs uppercase text-gray-500">Role</p>
                <p className="font-mono text-xs">
                  {document.messageRole || '-'}
                </p>
              </div>
              <div className="col-span-2">
                <p className="text-xs uppercase text-gray-500">Record ID</p>
                <p className="break-all font-mono text-xs">{document.id}</p>
              </div>
              <div className="col-span-2">
                <p className="text-xs uppercase text-gray-500">traceID</p>
                <div className="flex min-w-0 items-center gap-1">
                  <p className="break-all font-mono text-xs">
                    {document.traceId || '-'}
                  </p>
                  {document.traceId && (
                    <CopyButton className="h-6 w-6 shrink-0">
                      {document.traceId}
                    </CopyButton>
                  )}
                </div>
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
            </div>
            <div className="space-y-4">
              <div className="min-w-0">
                <p className="mb-2 text-xs font-medium uppercase text-gray-500">
                  Attributes
                </p>
                <CodeSnippet type="multi" feedback="Copied">
                  {JSON.stringify(document.attributes || {}, null, 2)}
                </CodeSnippet>
              </div>
              <div className="min-w-0">
                <p className="mb-2 text-xs font-medium uppercase text-gray-500">
                  Data
                </p>
                <CodeSnippet type="multi" feedback="Copied">
                  {JSON.stringify(document.data || {}, null, 2)}
                </CodeSnippet>
              </div>
            </div>
          </div>
        </div>
      </section>
    )}
  </div>
);

export const ListingPage = () => {
  const { token, authId, projectId } = useCurrentCredential();
  const [searchParams] = useSearchParams();
  const searchParamsKey = searchParams.toString();
  const queryFilters = useMemo(
    () => getTraceFiltersFromSearchParams(new URLSearchParams(searchParamsKey)),
    [searchParamsKey],
  );
  const lastSearchParamsKey = useRef(searchParamsKey);
  const [searchText, setSearchText] = useState(queryFilters.searchText);
  const [selectedKind, setSelectedKind] = useState(queryFilters.selectedKind);
  const [selectedLevel, setSelectedLevel] = useState(
    queryFilters.selectedLevel,
  );
  const [selectedScope, setSelectedScope] = useState(
    queryFilters.selectedScope,
  );
  const [selectedRole, setSelectedRole] = useState(queryFilters.selectedRole);
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);
  const [selectedEvent, setSelectedEvent] = useState(
    queryFilters.selectedEvent,
  );
  const [metricNameInput, setMetricNameInput] = useState(
    queryFilters.metricNameInput,
  );
  const [assistantIdInput, setAssistantIdInput] = useState(
    queryFilters.assistantIdInput,
  );
  const [conversationIdInput, setConversationIdInput] = useState(
    queryFilters.conversationIdInput,
  );
  const [messageIdInput, setMessageIdInput] = useState(
    queryFilters.messageIdInput,
  );
  const [traceIdInput, setTraceIdInput] = useState(queryFilters.traceIdInput);
  const [dateRange, setDateRange] = useState<[Date, Date] | null>(
    queryFilters.dateRange,
  );
  const [appliedFilters, setAppliedFilters] =
    useState<TraceFilterState>(queryFilters);
  const [documents, setDocuments] = useState<TimelineDocument[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [totalItem, setTotalItem] = useState(0);
  const [refreshKey, setRefreshKey] = useState(0);
  const [isFilterOpen, setIsFilterOpen] = useState(false);
  const [selectedContextId, setSelectedContextId] = useState('');
  const [selectedDocument, setSelectedDocument] =
    useState<TimelineDocument | null>(null);

  const selectedComponentId = selectedComponents[0] || 'all';

  const eventFilterOptions = useMemo(
    () => getEventOptionsForComponent(selectedComponentId),
    [selectedComponentId],
  );

  const currentFilters = useMemo<TraceFilterState>(
    () => ({
      assistantIdInput,
      conversationIdInput,
      dateRange,
      messageIdInput,
      metricNameInput,
      searchText,
      selectedComponents,
      selectedEvent,
      selectedKind,
      selectedLevel,
      selectedRole,
      selectedScope,
      traceIdInput,
    }),
    [
      assistantIdInput,
      conversationIdInput,
      dateRange,
      messageIdInput,
      metricNameInput,
      searchText,
      selectedComponents,
      selectedEvent,
      selectedKind,
      selectedLevel,
      selectedRole,
      selectedScope,
      traceIdInput,
    ],
  );

  useEffect(() => {
    if (lastSearchParamsKey.current === searchParamsKey) return;
    lastSearchParamsKey.current = searchParamsKey;

    setSearchText(queryFilters.searchText);
    setSelectedKind(queryFilters.selectedKind);
    setSelectedLevel(queryFilters.selectedLevel);
    setSelectedScope(queryFilters.selectedScope);
    setSelectedRole(queryFilters.selectedRole);
    setSelectedComponents(queryFilters.selectedComponents);
    setSelectedEvent(queryFilters.selectedEvent);
    setMetricNameInput(queryFilters.metricNameInput);
    setAssistantIdInput(queryFilters.assistantIdInput);
    setConversationIdInput(queryFilters.conversationIdInput);
    setMessageIdInput(queryFilters.messageIdInput);
    setTraceIdInput(queryFilters.traceIdInput);
    setDateRange(queryFilters.dateRange);
    setAppliedFilters(queryFilters);
    setPage(1);
    setRefreshKey(key => key + 1);
  }, [queryFilters, searchParamsKey]);

  const requestCriteria = useMemo(() => {
    const next: Criteria[] = [];
    const addCriteria = (key: string, value: string, logic = '=') => {
      if (!value) return;
      const criteria = new Criteria();
      criteria.setKey(key);
      criteria.setValue(value);
      criteria.setLogic(logic);
      next.push(criteria);
    };

    addCriteria('search', appliedFilters.searchText.trim(), 'match');
    addCriteria('traceId', appliedFilters.traceIdInput.trim());
    if (appliedFilters.selectedKind.id !== KIND_OPTIONS[0].id) {
      addCriteria('kind', appliedFilters.selectedKind.id);
    }
    if (
      appliedFilters.selectedKind.id === 'log' &&
      appliedFilters.selectedLevel.id !== LEVEL_OPTIONS[0].id
    ) {
      addCriteria('level', appliedFilters.selectedLevel.id);
    }
    if (appliedFilters.selectedKind.id === 'event') {
      if (appliedFilters.selectedEvent.id !== ALL_EVENT_OPTION.id) {
        addCriteria('event', appliedFilters.selectedEvent.id);
      }
      if (appliedFilters.selectedComponents.length > 0) {
        addCriteria('component', appliedFilters.selectedComponents[0]);
      }
    }
    if (
      appliedFilters.selectedKind.id === 'metric' &&
      appliedFilters.metricNameInput &&
      appliedFilters.metricNameInput !== METRIC_NAME_OPTIONS[0].id
    ) {
      addCriteria('name', appliedFilters.metricNameInput.trim());
    }
    if (appliedFilters.selectedScope.id !== SCOPE_OPTIONS[0].id) {
      addCriteria('scope', appliedFilters.selectedScope.id);
    }
    if (appliedFilters.selectedScope.id === 'assistant') {
      addCriteria('assistantId', appliedFilters.assistantIdInput.trim());
    }
    if (appliedFilters.selectedScope.id === 'conversation') {
      addCriteria(
        'assistantConversationId',
        appliedFilters.conversationIdInput.trim(),
      );
    }
    if (appliedFilters.selectedScope.id === 'message') {
      addCriteria('messageId', appliedFilters.messageIdInput.trim());
      if (appliedFilters.selectedRole.id !== ROLE_OPTIONS[0].id) {
        addCriteria('messageRole', appliedFilters.selectedRole.id);
      }
    }
    if (appliedFilters.dateRange) {
      addCriteria(
        'occurredAtFrom',
        appliedFilters.dateRange[0].toISOString(),
        '>=',
      );
      const endDate = new Date(appliedFilters.dateRange[1]);
      endDate.setHours(23, 59, 59, 999);
      addCriteria('occurredAtTo', endDate.toISOString(), '<=');
    }

    return next;
  }, [appliedFilters]);

  useEffect(() => {
    if (selectedKind.id !== 'log') setSelectedLevel(LEVEL_OPTIONS[0]);
    if (selectedKind.id !== 'event') {
      setSelectedComponents([]);
      setSelectedEvent(ALL_EVENT_OPTION);
    }
    if (selectedKind.id !== 'metric') {
      setMetricNameInput(METRIC_NAME_OPTIONS[0].id);
    }
  }, [selectedKind]);

  useEffect(() => {
    if (selectedScope.id === 'all' || selectedScope.id === 'project') {
      setAssistantIdInput('');
      setConversationIdInput('');
      setMessageIdInput('');
      setSelectedRole(ROLE_OPTIONS[0]);
    }
    if (selectedScope.id === 'assistant') {
      setConversationIdInput('');
      setMessageIdInput('');
      setSelectedRole(ROLE_OPTIONS[0]);
    }
    if (selectedScope.id === 'conversation') {
      setAssistantIdInput('');
      setMessageIdInput('');
      setSelectedRole(ROLE_OPTIONS[0]);
    }
    if (selectedScope.id === 'message') {
      setAssistantIdInput('');
      setConversationIdInput('');
    }
  }, [selectedScope]);

  useEffect(() => {
    let active = true;

    const fetchTelemetry = async () => {
      setIsLoading(true);

      const request = new GetAllTelemetryRequest();
      const paginate = new Paginate();
      paginate.setPage(page);
      paginate.setPagesize(pageSize);
      request.setPaginate(paginate);
      request.setCriteriasList(requestCriteria);

      const order = new Ordering();
      order.setColumn('occurredAt');
      order.setOrder('desc');
      request.setOrder(order);

      try {
        const response = await GetAllTelemetry(
          connectionConfig,
          request,
          ConnectionConfig.WithDebugger({
            authorization: token,
            userId: authId,
            projectId,
          }),
        );
        if (!active) return;

        if (!response.getSuccess()) {
          const message =
            response.getError()?.getHumanmessage() || TRACE_LOAD_ERROR_MESSAGE;
          toast.error(message);
          return;
        }

        const nextDocuments = response
          .getDataList()
          .map(telemetryRecordToTimelineDocument)
          .filter(Boolean) as TimelineDocument[];

        setDocuments(nextDocuments);
        setTotalItem(
          response.getPaginated()?.getTotalitem() || nextDocuments.length,
        );
      } catch (error) {
        if (!active) return;
        const message = getTelemetryErrorMessage(error);
        toast.error(message);
      } finally {
        if (active) setIsLoading(false);
      }
    };

    fetchTelemetry();

    return () => {
      active = false;
    };
  }, [authId, page, pageSize, projectId, refreshKey, requestCriteria, token]);

  const filteredDocuments = useMemo(
    () =>
      documents.filter(document => {
        const occurredMs = new Date(document.occurredAt).getTime();
        const matchesDate =
          !appliedFilters.dateRange ||
          (occurredMs >= appliedFilters.dateRange[0].getTime() &&
            occurredMs <=
              appliedFilters.dateRange[1].getTime() + 24 * 60 * 60 * 1000);

        return (
          matchesTimelineSearch(document, appliedFilters.searchText) &&
          matchesDate &&
          (!appliedFilters.traceIdInput.trim() ||
            document.traceId === appliedFilters.traceIdInput.trim()) &&
          (appliedFilters.selectedKind.id === KIND_OPTIONS[0].id ||
            document.kind === appliedFilters.selectedKind.id) &&
          (appliedFilters.selectedKind.id !== 'log' ||
            appliedFilters.selectedLevel.id === LEVEL_OPTIONS[0].id ||
            document.level === appliedFilters.selectedLevel.id) &&
          (appliedFilters.selectedKind.id !== 'event' ||
            appliedFilters.selectedComponents.length === 0 ||
            appliedFilters.selectedComponents.includes(
              getDocumentComponent(document),
            )) &&
          (appliedFilters.selectedKind.id !== 'event' ||
            appliedFilters.selectedEvent.id === ALL_EVENT_OPTION.id ||
            document.name === appliedFilters.selectedEvent.id) &&
          (appliedFilters.selectedKind.id !== 'metric' ||
            !appliedFilters.metricNameInput ||
            appliedFilters.metricNameInput === METRIC_NAME_OPTIONS[0].id ||
            (document.data?.metrics as Array<{ name: string }> | undefined)
              ?.map(metric => metric.name)
              .includes(appliedFilters.metricNameInput) ||
            document.name === appliedFilters.metricNameInput) &&
          (appliedFilters.selectedScope.id === SCOPE_OPTIONS[0].id ||
            document.scope === appliedFilters.selectedScope.id) &&
          (appliedFilters.selectedScope.id !== 'assistant' ||
            !appliedFilters.assistantIdInput.trim() ||
            String(document.assistantId) ===
              appliedFilters.assistantIdInput.trim()) &&
          (appliedFilters.selectedScope.id !== 'conversation' ||
            !appliedFilters.conversationIdInput.trim() ||
            String(document.assistantConversationId) ===
              appliedFilters.conversationIdInput.trim()) &&
          (appliedFilters.selectedScope.id !== 'message' ||
            ((!appliedFilters.messageIdInput.trim() ||
              document.messageId === appliedFilters.messageIdInput.trim()) &&
              (appliedFilters.selectedRole.id === ROLE_OPTIONS[0].id ||
                document.messageRole === appliedFilters.selectedRole.id)))
        );
      }),
    [appliedFilters, documents],
  );

  const selectedTimelineDocuments = useMemo(
    () =>
      filteredDocuments.filter(
        document => document.contextId === selectedContextId,
      ),
    [filteredDocuments, selectedContextId],
  );

  useEffect(() => {
    const currentContextExists = filteredDocuments.some(
      document => document.contextId === selectedContextId,
    );

    if (currentContextExists) return;

    const nextDocument = filteredDocuments[0] || null;
    setSelectedContextId(nextDocument?.contextId || '');
    setSelectedDocument(null);
  }, [filteredDocuments, selectedContextId]);

  const resetExplorerFilters = () => {
    setSearchText(DEFAULT_TRACE_FILTERS.searchText);
    setSelectedKind(DEFAULT_TRACE_FILTERS.selectedKind);
    setSelectedLevel(DEFAULT_TRACE_FILTERS.selectedLevel);
    setSelectedScope(DEFAULT_TRACE_FILTERS.selectedScope);
    setSelectedRole(DEFAULT_TRACE_FILTERS.selectedRole);
    setSelectedComponents([]);
    setSelectedEvent(DEFAULT_TRACE_FILTERS.selectedEvent);
    setMetricNameInput(DEFAULT_TRACE_FILTERS.metricNameInput);
    setAssistantIdInput(DEFAULT_TRACE_FILTERS.assistantIdInput);
    setConversationIdInput(DEFAULT_TRACE_FILTERS.conversationIdInput);
    setMessageIdInput(DEFAULT_TRACE_FILTERS.messageIdInput);
    setTraceIdInput(DEFAULT_TRACE_FILTERS.traceIdInput);
    setDateRange(DEFAULT_TRACE_FILTERS.dateRange);
  };

  useEffect(() => {
    if (
      selectedEvent.id !== ALL_EVENT_OPTION.id &&
      !eventFilterOptions.some(option => option.id === selectedEvent.id)
    ) {
      setSelectedEvent(ALL_EVENT_OPTION);
    }
  }, [eventFilterOptions, selectedEvent]);

  const selectRecord = (document: TimelineDocument) => {
    setSelectedContextId(document.contextId);
    setSelectedDocument(document);
  };

  return (
    <div className="flex h-full overflow-hidden">
      <Helmet title="Trace" />

      <div className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
        <TableToolbar>
          <TableToolbarContent>
            <TableToolbarSearch
              placeholder="Search traces, spans, context IDs, and events"
              value={searchText}
              onChange={(event: any) =>
                setSearchText(event.target?.value || '')
              }
            />
            <Button
              hasIconOnly
              kind="ghost"
              renderIcon={Renew}
              iconDescription="Reload"
              tooltipPosition="bottom"
              onClick={() => setRefreshKey(key => key + 1)}
            />
            <Button
              hasIconOnly
              kind="ghost"
              renderIcon={Filter}
              iconDescription="Filter"
              tooltipPosition="bottom"
              className={isFilterOpen ? 'cds--btn--selected' : ''}
              aria-pressed={isFilterOpen}
              onClick={() => setIsFilterOpen(open => !open)}
            />
          </TableToolbarContent>
        </TableToolbar>

        <div
          className={[
            'absolute bottom-0 right-0 top-12 z-20 w-[440px] transform transition-transform duration-200 ease-out',
            isFilterOpen
              ? 'translate-x-0 shadow-xl'
              : 'pointer-events-none translate-x-full',
          ].join(' ')}
        >
          <ExplorerFilter
            assistantId={assistantIdInput}
            conversationId={conversationIdInput}
            dateRange={dateRange}
            eventOptions={eventFilterOptions}
            level={selectedLevel}
            messageId={messageIdInput}
            metricName={metricNameInput}
            role={selectedRole}
            selectedComponentId={selectedComponentId}
            selectedEvent={selectedEvent}
            selectedKind={selectedKind}
            selectedScope={selectedScope}
            traceId={traceIdInput}
            onAssistantIdChange={setAssistantIdInput}
            onComponentChange={componentId =>
              setSelectedComponents(componentId === 'all' ? [] : [componentId])
            }
            onConversationIdChange={setConversationIdInput}
            onDateRangeChange={setDateRange}
            onEventChange={setSelectedEvent}
            onKindChange={setSelectedKind}
            onLevelChange={setSelectedLevel}
            onMessageIdChange={setMessageIdInput}
            onMetricNameChange={setMetricNameInput}
            onApply={() => {
              setAppliedFilters(currentFilters);
              setPage(1);
              setRefreshKey(key => key + 1);
              setIsFilterOpen(false);
            }}
            onReset={resetExplorerFilters}
            onRoleChange={setSelectedRole}
            onScopeChange={setSelectedScope}
            onTraceIdChange={setTraceIdInput}
          />
        </div>

        <div className="flex min-h-0 flex-1">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loading withOverlay={false} small />
            </div>
          ) : filteredDocuments.length === 0 ? (
            <EmptyState
              icon={appliedFilters.searchText ? WarningAlt : Activity}
              title="No traces found"
              subtitle="Adjust search, scope, component, event, or date filters."
            />
          ) : (
            <div className="flex min-w-0 flex-1 flex-col">
              <TelemetryStreamTable
                selectedDocumentId={selectedDocument?.id}
                records={filteredDocuments}
                onSelectRecord={selectRecord}
              />

              <Pagination
                className="shrink-0 border-t border-gray-200 dark:border-gray-800"
                totalItems={totalItem}
                page={page}
                pageSize={pageSize}
                pageSizes={[25, 50, 100]}
                onChange={({ page: nextPage, pageSize: nextPageSize }) => {
                  if (nextPageSize !== pageSize) {
                    setPageSize(nextPageSize);
                    setPage(1);
                    return;
                  }
                  setPage(nextPage);
                }}
              />
            </div>
          )}
        </div>
        <TraceInspectorPanel
          document={selectedDocument}
          records={selectedTimelineDocuments}
          recordCount={selectedTimelineDocuments.length}
          selectedContextId={selectedContextId}
          selectedDocumentId={selectedDocument?.id}
          onSelectDocument={setSelectedDocument}
          onClose={() => setSelectedDocument(null)}
        />
      </div>
    </div>
  );
};
