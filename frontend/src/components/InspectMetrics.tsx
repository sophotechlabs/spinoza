import { useEffect, useRef, useState } from 'react';
import type { MetricHistory, MetricPoint } from '../lib/types';
import {
  DEFAULT_RANGE,
  RANGES,
  fetchMetricHistory,
  formatCpu,
  formatMemory,
  peak,
} from '../lib/metricsHistory';
import type { MetricRange } from '../lib/metricsHistory';
import { createChart } from '../lib/chart';
import type { ChartHandle } from '../lib/chart';

interface InspectMetricsProps {
  namespace: string;
  pod: string;
}

interface ChartProps {
  points: MetricPoint[];
  stroke: string;
  fill: string;
  format: (value: number) => string;
  metric: 'cpu' | 'memory';
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'loading metrics failed';
}

function Chart({ points, stroke, fill, format, metric }: ChartProps) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const chartRef = useRef<ChartHandle | null>(null);

  useEffect(() => {
    if (host === null) {
      return;
    }
    const chart = createChart(host, { stroke, fill, format, metric });
    chartRef.current = chart;

    const observer = new ResizeObserver(() => {
      chart.resize(host.clientWidth);
    });
    observer.observe(host);

    return () => {
      observer.disconnect();
      chart.destroy();
      chartRef.current = null;
    };
  }, [host, stroke, fill, format, metric]);

  useEffect(() => {
    chartRef.current?.update(points);
  }, [host, points]);

  return (
    <div ref={setHost} data-testid="metric-chart" className="w-full min-w-0 overflow-hidden" />
  );
}

export default function InspectMetrics({ namespace, pod }: InspectMetricsProps) {
  const [span, setSpan] = useState<MetricRange>(DEFAULT_RANGE);
  const [history, setHistory] = useState<MetricHistory | null>(null);
  const [error, setError] = useState<string | null>(null);

  const podKey = `${namespace}/${pod}`;
  const [lastPod, setLastPod] = useState(podKey);
  if (podKey !== lastPod) {
    setLastPod(podKey);
    setHistory(null);
    setError(null);
  }

  useEffect(() => {
    let live = true;
    setError(null);
    fetchMetricHistory(namespace, pod, span)
      .then((next) => {
        if (live) {
          setHistory(next);
        }
      })
      .catch((err: unknown) => {
        if (live) {
          setHistory(null);
          setError(errorMessage(err));
        }
      });
    return () => {
      live = false;
    };
  }, [namespace, pod, span]);

  function handleSpan(event: React.ChangeEvent<HTMLSelectElement>) {
    setSpan(event.target.value as MetricRange);
  }

  const cpu = history?.cpu ?? [];
  const memory = history?.memory ?? [];
  const empty = history !== null && cpu.length === 0 && memory.length === 0;

  return (
    <div className="overflow-y-auto p-3 text-xs">
      <div className="flex items-center gap-2">
        <label className="text-neutral-400" htmlFor="metric-range">
          range
        </label>
        <select
          id="metric-range"
          aria-label="Metric range"
          value={span}
          onChange={handleSpan}
          className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
        >
          {RANGES.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        {history?.source !== undefined && (
          <span className="ml-auto truncate text-[10px] text-neutral-400">{history.source}</span>
        )}
      </div>

      {error !== null && <p className="mt-3 break-words text-red-400">{error}</p>}
      {empty && (
        <p className="mt-3 text-neutral-400">
          Prometheus has no samples for this pod over the last {span}.
        </p>
      )}

      {history !== null && !empty && error === null && (
        <div className="mt-3">
          <div className="flex items-baseline justify-between">
            <span className="text-neutral-300">CPU</span>
            <span className="text-neutral-400">peak {formatCpu(peak(cpu))}</span>
          </div>
          <Chart
            points={cpu}
            stroke="#4ade80"
            fill="rgba(74,222,128,0.12)"
            format={formatCpu}
            metric="cpu"
          />

          <div className="mt-4 flex items-baseline justify-between">
            <span className="text-neutral-300">Memory</span>
            <span className="text-neutral-400">peak {formatMemory(peak(memory))}</span>
          </div>
          <Chart
            points={memory}
            stroke="#60a5fa"
            fill="rgba(96,165,250,0.12)"
            format={formatMemory}
            metric="memory"
          />
        </div>
      )}
    </div>
  );
}
