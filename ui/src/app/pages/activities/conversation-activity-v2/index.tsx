import { useEffect, useMemo, useState } from 'react';
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
import { Helmet } from '@/app/components/helmet';
import { EmptyState } from '@/app/components/carbon/empty-state';
import { Pagination } from '@/app/components/carbon/pagination';
import { ScrollableTableSection } from '@/app/components/sections/table-section';
import { connectionConfig } from '@/configs';
import { useCurrentCredential } from '@/hooks/use-credential';
import { ConversationWaterfall } from './components/conversation-waterfall';
import { ExplorerFilter } from './components/explorer-filter';
import {
  ALL_EVENT_OPTION,
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
    <Table className="min-w-[900px]">
      <TableHead>
        <TableRow>
          <TableHeader>ID</TableHeader>
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
  const [searchText, setSearchText] = useState('');
  const [selectedKind, setSelectedKind] = useState(KIND_OPTIONS[0]);
  const [selectedLevel, setSelectedLevel] = useState(LEVEL_OPTIONS[0]);
  const [selectedScope, setSelectedScope] = useState(SCOPE_OPTIONS[0]);
  const [selectedRole, setSelectedRole] = useState(ROLE_OPTIONS[0]);
  const [selectedComponents, setSelectedComponents] = useState<string[]>([]);
  const [selectedEvent, setSelectedEvent] = useState(ALL_EVENT_OPTION);
  const [metricNameInput, setMetricNameInput] = useState(
    METRIC_NAME_OPTIONS[0].id,
  );
  const [assistantIdInput, setAssistantIdInput] = useState('');
  const [conversationIdInput, setConversationIdInput] = useState('');
  const [messageIdInput, setMessageIdInput] = useState('');
  const [dateRange, setDateRange] = useState<[Date, Date] | null>(null);
  const [documents, setDocuments] = useState<TimelineDocument[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [errorText, setErrorText] = useState('');
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

    addCriteria('search', searchText.trim(), 'match');
    if (selectedKind.id !== KIND_OPTIONS[0].id) {
      addCriteria('kind', selectedKind.id);
    }
    if (selectedKind.id === 'log' && selectedLevel.id !== LEVEL_OPTIONS[0].id) {
      addCriteria('level', selectedLevel.id);
    }
    if (selectedKind.id === 'event') {
      if (selectedEvent.id !== ALL_EVENT_OPTION.id) {
        addCriteria('event', selectedEvent.id);
      }
      if (selectedComponents.length > 0) {
        addCriteria('component', selectedComponents[0]);
      }
    }
    if (
      selectedKind.id === 'metric' &&
      metricNameInput &&
      metricNameInput !== METRIC_NAME_OPTIONS[0].id
    ) {
      addCriteria('name', metricNameInput.trim());
    }
    if (selectedScope.id !== SCOPE_OPTIONS[0].id) {
      addCriteria('scope', selectedScope.id);
    }
    if (selectedScope.id !== SCOPE_OPTIONS[0].id) {
      addCriteria('assistantId', assistantIdInput.trim());
    }
    if (selectedScope.id === 'conversation' || selectedScope.id === 'message') {
      addCriteria('assistantConversationId', conversationIdInput.trim());
    }
    if (selectedScope.id === 'message') {
      addCriteria('messageId', messageIdInput.trim());
      if (selectedRole.id !== ROLE_OPTIONS[0].id) {
        addCriteria('messageRole', selectedRole.id);
      }
    }
    if (dateRange) {
      addCriteria('occurredAtFrom', dateRange[0].toISOString(), '>=');
      const endDate = new Date(dateRange[1]);
      endDate.setHours(23, 59, 59, 999);
      addCriteria('occurredAtTo', endDate.toISOString(), '<=');
    }

    return next;
  }, [
    assistantIdInput,
    conversationIdInput,
    dateRange,
    metricNameInput,
    messageIdInput,
    searchText,
    selectedComponents,
    selectedEvent,
    selectedKind,
    selectedLevel,
    selectedRole,
    selectedScope,
  ]);

  useEffect(() => {
    setPage(1);
  }, [
    assistantIdInput,
    conversationIdInput,
    dateRange,
    metricNameInput,
    messageIdInput,
    searchText,
    selectedComponents,
    selectedEvent,
    selectedKind,
    selectedLevel,
    selectedRole,
    selectedScope,
  ]);

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
    if (selectedScope.id === 'all') {
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
      setMessageIdInput('');
      setSelectedRole(ROLE_OPTIONS[0]);
    }
  }, [selectedScope]);

  useEffect(() => {
    let active = true;

    const fetchTelemetry = async () => {
      setIsLoading(true);
      setErrorText('');

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

        const nextDocuments = response
          .getDataList()
          .map(telemetryRecordToTimelineDocument)
          .filter(Boolean) as TimelineDocument[];

        setDocuments(nextDocuments);
        setTotalItem(
          response.getPaginated()?.getTotalitem() || nextDocuments.length,
        );
      } catch {
        if (!active) return;
        setDocuments([]);
        setTotalItem(0);
        setErrorText('Unable to load trace records.');
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
          !dateRange ||
          (occurredMs >= dateRange[0].getTime() &&
            occurredMs <= dateRange[1].getTime() + 24 * 60 * 60 * 1000);

        return (
          matchesTimelineSearch(document, searchText) &&
          matchesDate &&
          (selectedKind.id === KIND_OPTIONS[0].id ||
            document.kind === selectedKind.id) &&
          (selectedKind.id !== 'log' ||
            selectedLevel.id === LEVEL_OPTIONS[0].id ||
            document.level === selectedLevel.id) &&
          (selectedKind.id !== 'event' ||
            selectedComponents.length === 0 ||
            selectedComponents.includes(getDocumentComponent(document))) &&
          (selectedKind.id !== 'event' ||
            selectedEvent.id === ALL_EVENT_OPTION.id ||
            document.name === selectedEvent.id) &&
          (selectedKind.id !== 'metric' ||
            !metricNameInput ||
            metricNameInput === METRIC_NAME_OPTIONS[0].id ||
            (document.data?.metrics as Array<{ name: string }> | undefined)
              ?.map(metric => metric.name)
              .includes(metricNameInput) ||
            document.name === metricNameInput) &&
          (selectedScope.id === SCOPE_OPTIONS[0].id ||
            document.scope === selectedScope.id) &&
          (selectedScope.id === SCOPE_OPTIONS[0].id ||
            !assistantIdInput.trim() ||
            String(document.assistantId) === assistantIdInput.trim()) &&
          (selectedScope.id !== 'conversation' ||
            !conversationIdInput.trim() ||
            String(document.assistantConversationId) ===
              conversationIdInput.trim()) &&
          (selectedScope.id !== 'message' ||
            ((!conversationIdInput.trim() ||
              String(document.assistantConversationId) ===
                conversationIdInput.trim()) &&
              (!messageIdInput.trim() ||
                document.messageId === messageIdInput.trim()) &&
              (selectedRole.id === ROLE_OPTIONS[0].id ||
                document.messageRole === selectedRole.id)))
        );
      }),
    [
      assistantIdInput,
      conversationIdInput,
      dateRange,
      documents,
      metricNameInput,
      messageIdInput,
      searchText,
      selectedComponents,
      selectedEvent,
      selectedKind,
      selectedLevel,
      selectedRole,
      selectedScope,
    ],
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
    setSelectedKind(KIND_OPTIONS[0]);
    setSelectedLevel(LEVEL_OPTIONS[0]);
    setSelectedScope(SCOPE_OPTIONS[0]);
    setSelectedRole(ROLE_OPTIONS[0]);
    setSelectedComponents([]);
    setSelectedEvent(ALL_EVENT_OPTION);
    setMetricNameInput(METRIC_NAME_OPTIONS[0].id);
    setAssistantIdInput('');
    setConversationIdInput('');
    setMessageIdInput('');
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
              setRefreshKey(key => key + 1);
              setIsFilterOpen(false);
            }}
            onReset={resetExplorerFilters}
            onRoleChange={setSelectedRole}
            onScopeChange={setSelectedScope}
          />
        </div>

        <div className="flex min-h-0 flex-1">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loading withOverlay={false} small />
            </div>
          ) : filteredDocuments.length === 0 ? (
            <EmptyState
              icon={searchText || errorText ? WarningAlt : Activity}
              title="No traces found"
              subtitle={
                errorText ||
                'Adjust search, scope, component, event, or date filters.'
              }
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
