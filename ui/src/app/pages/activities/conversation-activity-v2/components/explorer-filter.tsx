import {
  Button,
  ButtonSet,
  ComboBox,
  DatePicker,
  DatePickerInput,
  Dropdown,
  TextInput,
} from '@carbon/react';
import {
  ALL_EVENT_OPTION,
  COMPONENT_OPTIONS,
  FilterOption,
  KIND_OPTIONS,
  LEVEL_OPTIONS,
  METRIC_NAME_OPTIONS,
  ROLE_OPTIONS,
  SCOPE_OPTIONS,
  getOptionById,
  itemToString,
} from '../constants';

type ExplorerFilterProps = {
  assistantId: string;
  conversationId: string;
  dateRange: [Date, Date] | null;
  eventOptions: FilterOption[];
  level: FilterOption;
  messageId: string;
  metricName: string;
  role: FilterOption;
  selectedComponentId: string;
  selectedEvent: FilterOption;
  selectedKind: FilterOption;
  selectedScope: FilterOption;
  traceId: string;
  onAssistantIdChange: (value: string) => void;
  onComponentChange: (componentId: string) => void;
  onConversationIdChange: (value: string) => void;
  onDateRangeChange: (dateRange: [Date, Date] | null) => void;
  onEventChange: (event: FilterOption) => void;
  onKindChange: (kind: FilterOption) => void;
  onLevelChange: (level: FilterOption) => void;
  onMessageIdChange: (value: string) => void;
  onMetricNameChange: (value: string) => void;
  onApply: () => void;
  onReset: () => void;
  onRoleChange: (role: FilterOption) => void;
  onScopeChange: (scope: FilterOption) => void;
  onTraceIdChange: (value: string) => void;
};

export const ExplorerFilter = ({
  assistantId,
  conversationId,
  dateRange,
  eventOptions,
  level,
  messageId,
  metricName,
  role,
  selectedComponentId,
  selectedEvent,
  selectedKind,
  selectedScope,
  traceId,
  onAssistantIdChange,
  onComponentChange,
  onConversationIdChange,
  onDateRangeChange,
  onEventChange,
  onKindChange,
  onLevelChange,
  onMessageIdChange,
  onMetricNameChange,
  onApply,
  onReset,
  onRoleChange,
  onScopeChange,
  onTraceIdChange,
}: ExplorerFilterProps) => {
  const selectedMetricName =
    metricName && metricName !== 'all'
      ? METRIC_NAME_OPTIONS.find(option => option.id === metricName) || {
          id: metricName,
          text: metricName,
        }
      : METRIC_NAME_OPTIONS[0];
  const showAssistantId = selectedScope.id === 'assistant';
  const showConversationId = selectedScope.id === 'conversation';
  const showMessageFields = selectedScope.id === 'message';

  return (
    <aside className="flex h-full w-full shrink-0 flex-col border-l border-gray-200 bg-gray-50 dark:border-gray-800 dark:bg-gray-950">
      <div className="min-h-0 flex-1 space-y-5 overflow-auto px-4 pb-4">
        <section className="grid gap-3">
          <p className="-mx-4 border-b border-gray-200 px-4 py-2 text-xs font-medium uppercase text-gray-500 dark:border-gray-800 dark:text-gray-400">
            Record
          </p>
          <Dropdown
            id="trace-explorer-kind"
            label="Select record type"
            titleText="Record type"
            size="md"
            items={KIND_OPTIONS}
            itemToString={itemToString}
            selectedItem={selectedKind}
            onChange={({ selectedItem }) =>
              onKindChange(selectedItem || KIND_OPTIONS[0])
            }
          />
          <TextInput
            id="trace-explorer-trace-id"
            labelText="traceID"
            placeholder="trace id"
            size="md"
            value={traceId}
            onChange={event => onTraceIdChange(event.target.value)}
          />

          {selectedKind.id === 'log' && (
            <Dropdown
              id="trace-explorer-level"
              label="Select level"
              titleText="Level"
              size="md"
              items={LEVEL_OPTIONS}
              itemToString={itemToString}
              selectedItem={level}
              onChange={({ selectedItem }) =>
                onLevelChange(selectedItem || LEVEL_OPTIONS[0])
              }
            />
          )}

          {selectedKind.id === 'event' && (
            <>
              <ComboBox
                id="trace-explorer-event"
                titleText="Event"
                placeholder="Search event"
                size="md"
                items={eventOptions}
                itemToString={itemToString}
                selectedItem={selectedEvent}
                onChange={({ selectedItem }) => {
                  onEventChange(selectedItem || ALL_EVENT_OPTION);
                }}
              />
              <ComboBox
                id="trace-explorer-component"
                titleText="Component"
                placeholder="Search component"
                size="md"
                items={COMPONENT_OPTIONS}
                itemToString={itemToString}
                selectedItem={getOptionById(
                  COMPONENT_OPTIONS,
                  selectedComponentId,
                )}
                onChange={({ selectedItem }) => {
                  onComponentChange(selectedItem?.id || 'all');
                }}
              />
            </>
          )}

          {selectedKind.id === 'metric' && (
            <ComboBox
              id="trace-explorer-metric-name"
              titleText="Metric name"
              placeholder="Search metric"
              size="md"
              items={METRIC_NAME_OPTIONS}
              itemToString={itemToString}
              selectedItem={selectedMetricName}
              allowCustomValue
              onChange={({ selectedItem, inputValue }: any) => {
                onMetricNameChange(selectedItem?.id || inputValue || 'all');
              }}
            />
          )}
        </section>

        <section className="grid gap-3">
          <p className="-mx-4 border-y border-gray-200 px-4 py-2 text-xs font-medium uppercase text-gray-500 dark:border-gray-800 dark:text-gray-400">
            Scope
          </p>
          <Dropdown
            id="trace-explorer-scope"
            label="Select scope"
            titleText="Scope"
            size="md"
            items={SCOPE_OPTIONS}
            itemToString={itemToString}
            selectedItem={selectedScope}
            onChange={({ selectedItem }) =>
              onScopeChange(selectedItem || SCOPE_OPTIONS[0])
            }
          />

          {showAssistantId && (
            <TextInput
              id="trace-explorer-assistant-id"
              labelText="Assistant ID"
              placeholder="assistant id"
              size="md"
              value={assistantId}
              onChange={event => onAssistantIdChange(event.target.value)}
            />
          )}
          {showConversationId && (
            <TextInput
              id="trace-explorer-conversation-id"
              labelText="Conversation ID"
              placeholder="conversation id"
              size="md"
              value={conversationId}
              onChange={event => onConversationIdChange(event.target.value)}
            />
          )}
          {showMessageFields && (
            <>
              <TextInput
                id="trace-explorer-message-id"
                labelText="Message ID"
                placeholder="message id"
                size="md"
                value={messageId}
                onChange={event => onMessageIdChange(event.target.value)}
              />
              <Dropdown
                id="trace-explorer-message-role"
                label="Select role"
                titleText="Role"
                size="md"
                items={ROLE_OPTIONS}
                itemToString={itemToString}
                selectedItem={role}
                onChange={({ selectedItem }) =>
                  onRoleChange(selectedItem || ROLE_OPTIONS[0])
                }
              />
            </>
          )}
        </section>

        <section>
          <p className="-mx-4 border-y border-gray-200 px-4 py-2 text-xs font-medium uppercase text-gray-500 dark:border-gray-800 dark:text-gray-400">
            Date
          </p>
          <div className="pt-3">
            <DatePicker
              datePickerType="range"
              dateFormat="m/d/Y"
              value={dateRange || undefined}
              onChange={(selectedDates: Date[]) =>
                onDateRangeChange(
                  selectedDates.length === 2
                    ? [selectedDates[0], selectedDates[1]]
                    : null,
                )
              }
            >
              <DatePickerInput
                id="trace-explorer-date-from"
                placeholder="Start date"
                labelText="Start date"
                size="md"
              />
              <DatePickerInput
                id="trace-explorer-date-to"
                placeholder="End date"
                labelText="End date"
                size="md"
              />
            </DatePicker>
          </div>
        </section>
      </div>

      <ButtonSet className="shrink-0 border-t border-gray-200 dark:border-gray-800 [&>button]:!flex-1 [&>button]:!max-w-none">
        <Button kind="secondary" size="lg" onClick={onReset}>
          Reset
        </Button>
        <Button kind="primary" size="lg" onClick={onApply}>
          Apply
        </Button>
      </ButtonSet>
    </aside>
  );
};
