import { useMemo, useState } from 'react';
import { Dropdown } from '@carbon/react';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { LatencyBucket, SpanCountBucket } from '../types';
import { formatDurationMs } from '../utils';

type ChartType = 'bar' | 'area' | 'line';

type ChartTypeOption = {
  id: ChartType;
  text: string;
};

const CHART_TYPE_OPTIONS: ChartTypeOption[] = [
  { id: 'bar', text: 'Bar' },
  { id: 'area', text: 'Area' },
  { id: 'line', text: 'Line' },
];

const INTERVAL_OPTIONS = [
  { id: 'auto', text: 'Auto interval' },
  { id: '1m', text: '1 minute' },
  { id: '5m', text: '5 minutes' },
  { id: '1h', text: '1 hour' },
];

const LATENCY_SERIES = [
  { key: 'stt', label: 'STT', color: '#198038' },
  { key: 'tts', label: 'TTS', color: '#fa4d56' },
  { key: 'llm', label: 'LLM', color: '#f1c21b' },
  { key: 'eos', label: 'EOS', color: '#007d79' },
] as const;

const itemToString = (item: { text: string } | null) => item?.text || '';

const getChartOption = (chartType: ChartType) =>
  CHART_TYPE_OPTIONS.find(option => option.id === chartType) ||
  CHART_TYPE_OPTIONS[0];

const SpanTooltip = ({ active, payload, label }: any) => {
  if (!active || !payload?.length) return null;
  const spanCount = payload.find((item: any) => item.dataKey === 'spanCount');
  const failureCount = payload.find(
    (item: any) => item.dataKey === 'failureCount',
  );

  return (
    <div className="border border-gray-200 bg-white px-3 py-2 text-sm shadow-lg dark:border-gray-800 dark:bg-gray-950">
      <p className="mb-1 text-xs text-gray-500">{label}</p>
      <p className="tabular-nums">
        Spans: <span className="font-medium">{spanCount?.value || 0}</span>
      </p>
      <p className="tabular-nums">
        Failed: <span className="font-medium">{failureCount?.value || 0}</span>
      </p>
    </div>
  );
};

const LatencyTooltip = ({ active, payload, label }: any) => {
  if (!active || !payload?.length) return null;
  const visiblePayload = payload.filter((item: any) =>
    Number.isFinite(Number(item.value)),
  );
  const total = visiblePayload.reduce(
    (sum: number, item: any) => sum + Number(item.value || 0),
    0,
  );

  return (
    <div className="min-w-[180px] border border-gray-200 bg-white px-3 py-2 text-sm shadow-lg dark:border-gray-800 dark:bg-gray-950">
      <p className="mb-1.5 text-xs text-gray-500">{label}</p>
      {visiblePayload.map((item: any) => (
        <div key={item.dataKey} className="flex items-center gap-2">
          <span className="h-2 w-2" style={{ backgroundColor: item.color }} />
          <span className="text-xs uppercase text-gray-600 dark:text-gray-300">
            {String(item.dataKey)}
          </span>
          <span className="ml-auto font-mono text-xs">
            {formatDurationMs(Number(item.value))}
          </span>
        </div>
      ))}
      <div className="mt-2 flex border-t border-gray-200 pt-1.5 text-xs dark:border-gray-700">
        <span className="uppercase text-gray-500">Total</span>
        <span className="ml-auto font-mono">{formatDurationMs(total)}</span>
      </div>
    </div>
  );
};

const ChartControls = ({
  chartType,
  idPrefix,
  onChartTypeChange,
}: {
  chartType: ChartType;
  idPrefix: string;
  onChartTypeChange: (chartType: ChartType) => void;
}) => (
  <div className="flex items-center gap-2">
    <div className="w-[132px]">
      <Dropdown
        id={`${idPrefix}-chart-type`}
        label="Chart type"
        hideLabel
        size="sm"
        items={CHART_TYPE_OPTIONS}
        itemToString={itemToString}
        selectedItem={getChartOption(chartType)}
        onChange={({ selectedItem }) =>
          onChartTypeChange(selectedItem?.id || 'bar')
        }
      />
    </div>
    <div className="w-[148px]">
      <Dropdown
        id={`${idPrefix}-interval`}
        label="Interval"
        hideLabel
        size="sm"
        items={INTERVAL_OPTIONS}
        itemToString={itemToString}
        selectedItem={INTERVAL_OPTIONS[0]}
        onChange={() => undefined}
      />
    </div>
  </div>
);

const ChartWidget = ({
  children,
  chartType,
  idPrefix,
  subtitle,
  title,
  onChartTypeChange,
}: {
  children: React.ReactNode;
  chartType: ChartType;
  idPrefix: string;
  subtitle: string;
  title: string;
  onChartTypeChange: (chartType: ChartType) => void;
}) => (
  <section className="border border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-950">
    <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 px-4 py-3 dark:border-gray-800">
      <div>
        <h2 className="text-sm font-medium text-gray-900 dark:text-gray-100">
          {title}
        </h2>
        <p className="text-xs text-gray-500">{subtitle}</p>
      </div>
      <ChartControls
        chartType={chartType}
        idPrefix={idPrefix}
        onChartTypeChange={onChartTypeChange}
      />
    </div>
    <div className="h-[220px] px-3 py-3">{children}</div>
  </section>
);

const SpanCountVisualization = ({
  buckets,
  chartType,
}: {
  buckets: SpanCountBucket[];
  chartType: ChartType;
}) => {
  if (buckets.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-gray-500">
        No span count data
      </div>
    );
  }

  const commonProps = {
    data: buckets,
    margin: { top: 8, right: 12, left: 0, bottom: 0 },
  };

  const axes = (
    <>
      <CartesianGrid vertical={false} stroke="#e0e0e0" />
      <XAxis
        dataKey="label"
        axisLine={false}
        tickLine={false}
        tick={{ fontSize: 11, fill: '#6f6f6f' }}
      />
      <YAxis
        axisLine={false}
        tickLine={false}
        width={40}
        tick={{ fontSize: 11, fill: '#6f6f6f' }}
      />
      <RechartsTooltip content={<SpanTooltip />} />
      <Legend iconType="square" wrapperStyle={{ fontSize: 12 }} />
    </>
  );

  if (chartType === 'line') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <LineChart {...commonProps}>
          {axes}
          <Line
            dataKey="spanCount"
            name="Spans"
            stroke="#0f62fe"
            strokeWidth={2}
            dot={false}
            type="monotone"
          />
          <Line
            dataKey="failureCount"
            name="Failed"
            stroke="#da1e28"
            strokeWidth={2}
            dot={false}
            type="monotone"
          />
        </LineChart>
      </ResponsiveContainer>
    );
  }

  if (chartType === 'area') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart {...commonProps}>
          {axes}
          <Area
            dataKey="spanCount"
            name="Spans"
            stroke="#0f62fe"
            fill="#0f62fe"
            fillOpacity={0.18}
            type="monotone"
          />
          <Area
            dataKey="failureCount"
            name="Failed"
            stroke="#da1e28"
            fill="#da1e28"
            fillOpacity={0.18}
            type="monotone"
          />
        </AreaChart>
      </ResponsiveContainer>
    );
  }

  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart {...commonProps}>
        {axes}
        <Bar dataKey="spanCount" name="Spans" fill="#0f62fe" stackId="count" />
        <Bar
          dataKey="failureCount"
          name="Failed"
          fill="#da1e28"
          stackId="count"
        />
      </BarChart>
    </ResponsiveContainer>
  );
};

const LatencyVisualization = ({
  buckets,
  chartType,
}: {
  buckets: LatencyBucket[];
  chartType: ChartType;
}) => {
  const hasLatencyData = buckets.some(bucket => bucket.total > 0);
  const commonProps = {
    data: buckets,
    margin: { top: 8, right: 12, left: 0, bottom: 0 },
  };

  if (!hasLatencyData) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-gray-500">
        No STT, TTS, LLM, or EOS latency data
      </div>
    );
  }

  const axes = (
    <>
      <CartesianGrid vertical={false} stroke="#e0e0e0" />
      <XAxis
        dataKey="label"
        axisLine={false}
        tickLine={false}
        tick={{ fontSize: 11, fill: '#6f6f6f' }}
      />
      <YAxis
        axisLine={false}
        tickFormatter={value => formatDurationMs(Number(value))}
        tickLine={false}
        width={52}
        tick={{ fontSize: 11, fill: '#6f6f6f' }}
      />
      <RechartsTooltip content={<LatencyTooltip />} />
      <Legend iconType="square" wrapperStyle={{ fontSize: 12 }} />
    </>
  );

  if (chartType === 'line') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <LineChart {...commonProps}>
          {axes}
          {LATENCY_SERIES.map(series => (
            <Line
              key={series.key}
              dataKey={series.key}
              name={series.label}
              stroke={series.color}
              strokeWidth={2}
              dot={false}
              type="monotone"
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    );
  }

  if (chartType === 'bar') {
    return (
      <ResponsiveContainer width="100%" height="100%">
        <BarChart {...commonProps}>
          {axes}
          {LATENCY_SERIES.map(series => (
            <Bar
              key={series.key}
              dataKey={series.key}
              name={series.label}
              fill={series.color}
              stackId="latency"
            />
          ))}
        </BarChart>
      </ResponsiveContainer>
    );
  }

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart {...commonProps}>
        {axes}
        {LATENCY_SERIES.map(series => (
          <Area
            key={series.key}
            dataKey={series.key}
            name={series.label}
            stroke={series.color}
            fill={series.color}
            fillOpacity={0.18}
            stackId="latency"
            type="monotone"
          />
        ))}
      </AreaChart>
    </ResponsiveContainer>
  );
};

export const SpanCountChart = ({
  buckets,
  latencyBuckets,
}: {
  buckets: SpanCountBucket[];
  latencyBuckets: LatencyBucket[];
}) => {
  const [spanChartType, setSpanChartType] = useState<ChartType>('bar');
  const [latencyChartType, setLatencyChartType] = useState<ChartType>('area');

  const totalSpans = useMemo(
    () => buckets.reduce((sum, bucket) => sum + bucket.spanCount, 0),
    [buckets],
  );
  const totalLatencyMs = useMemo(
    () => latencyBuckets.reduce((sum, bucket) => sum + bucket.total, 0),
    [latencyBuckets],
  );

  return (
    <div className="grid gap-3 bg-gray-50 px-3 py-3 dark:bg-gray-900">
      <ChartWidget
        chartType={spanChartType}
        idPrefix="span-count"
        title="span.count()"
        subtitle={`${totalSpans} spans over the selected window`}
        onChartTypeChange={setSpanChartType}
      >
        <SpanCountVisualization buckets={buckets} chartType={spanChartType} />
      </ChartWidget>

      <ChartWidget
        chartType={latencyChartType}
        idPrefix="latency"
        title="sum(duration) by component"
        subtitle={`${formatDurationMs(totalLatencyMs)} across STT, TTS, LLM, and EOS`}
        onChartTypeChange={setLatencyChartType}
      >
        <LatencyVisualization
          buckets={latencyBuckets}
          chartType={latencyChartType}
        />
      </ChartWidget>
    </div>
  );
};
