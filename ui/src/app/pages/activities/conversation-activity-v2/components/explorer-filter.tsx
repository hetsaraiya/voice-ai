import { DatePicker, DatePickerInput, Dropdown } from '@carbon/react';
import { TableToolbarFilter } from '@/app/components/carbon/table-toolbar-filter';
import {
  ALL_EVENT_OPTION,
  COMPONENT_OPTIONS,
  FilterOption,
  SCOPE_OPTIONS,
  getOptionById,
  itemToString,
} from '../constants';

type ExplorerFilterProps = {
  activeFilterIds: Set<string>;
  dateRange: [Date, Date] | null;
  eventOptions: FilterOption[];
  selectedComponentId: string;
  selectedEvent: FilterOption;
  selectedScope: FilterOption;
  onComponentChange: (componentId: string) => void;
  onDateRangeChange: (dateRange: [Date, Date] | null) => void;
  onEventChange: (event: FilterOption) => void;
  onReset: () => void;
  onScopeChange: (scope: FilterOption) => void;
};

export const ExplorerFilter = ({
  activeFilterIds,
  dateRange,
  eventOptions,
  selectedComponentId,
  selectedEvent,
  selectedScope,
  onComponentChange,
  onDateRangeChange,
  onEventChange,
  onReset,
  onScopeChange,
}: ExplorerFilterProps) => (
  <TableToolbarFilter
    filters={[]}
    activeFilters={activeFilterIds}
    onApplyFilter={() => undefined}
    onResetFilter={onReset}
    panelClassName="w-[360px]"
    extraContent={
      <div className="-mx-4 -my-3">
        <section>
          <p className="px-4 py-3 text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            By scope
          </p>
          <div className="border-b border-gray-200 dark:border-gray-700" />
          <div className="px-4 py-3">
            <Dropdown
              id="conversation-explorer-scope"
              label="Select scope"
              titleText="Scope selection"
              size="md"
              items={SCOPE_OPTIONS}
              itemToString={itemToString}
              selectedItem={selectedScope}
              onChange={({ selectedItem }) =>
                onScopeChange(selectedItem || SCOPE_OPTIONS[2])
              }
            />
          </div>
        </section>

        <section className="border-t border-gray-200 dark:border-gray-700">
          <p className="px-4 py-3 text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            By event
          </p>
          <div className="border-b border-gray-200 dark:border-gray-700" />
          <div className="grid gap-3 px-4 py-3">
            <Dropdown
              id="conversation-explorer-component"
              label="Select component"
              titleText="Component"
              size="md"
              items={COMPONENT_OPTIONS}
              itemToString={itemToString}
              selectedItem={getOptionById(
                COMPONENT_OPTIONS,
                selectedComponentId,
              )}
              onChange={({ selectedItem }) =>
                onComponentChange(selectedItem?.id || 'all')
              }
            />
            <Dropdown
              id="conversation-explorer-event"
              label="Select event"
              titleText="Event"
              size="md"
              items={eventOptions}
              itemToString={itemToString}
              selectedItem={selectedEvent}
              onChange={({ selectedItem }) =>
                onEventChange(selectedItem || ALL_EVENT_OPTION)
              }
            />
          </div>
        </section>

        <section className="border-t border-gray-200 dark:border-gray-700">
          <p className="px-4 py-3 text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
            By date
          </p>
          <div className="border-b border-gray-200 dark:border-gray-700" />
          <div className="px-4 py-3">
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
                id="conversation-explorer-date-from"
                placeholder="Start date"
                labelText="Start date"
                size="md"
              />
              <DatePickerInput
                id="conversation-explorer-date-to"
                placeholder="End date"
                labelText="End date"
                size="md"
              />
            </DatePicker>
          </div>
        </section>
      </div>
    }
  />
);
