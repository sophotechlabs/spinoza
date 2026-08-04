import { useEffect, useRef, useState } from 'react';
import type { MetricHistory, MetricPoint } from '../lib/types';
import { DEFAULT_RANGE, RANGES, fetchMetricHistory, peak } from '../lib/metricsHistory';
import type { MetricRange } from '../lib/metricsHistory';
import { cpuFromCores, memFromBytes } from '../lib/units';
import { createChart } from '../lib/chart';
import type { ChartHandle } from '../lib/chart';
import { canvasColors } from '../lib/themeColors';
import { useResolvedTheme } from '../store/theme';
import Announce from './Announce';

interface InspectMetricsProps {
  namespace: string;
  pod: string;
}

export interface ChartProps {
  points: MetricPoint[];
  stroke: string;
  fill: string;
  axis: string;
  grid: string;
  format: (value: number) => string;
  metric: 'cpu' | 'memory';
}

function errorMessage(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'loading metrics failed';
}

export function Chart({ points, stroke, fill, axis, grid, format, metric }: ChartProps) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const chartRef = useRef<ChartHandle | null>(null);
  const pointsRef = useRef(points);
  pointsRef.current = points;
  const colorsRef = useRef({ stroke, fill, axis, grid });
  colorsRef.current = { stroke, fill, axis, grid };

  useEffect(() => {
    if (host === null) {
      return;
    }
    const chart = createChart(host, {
      colors: colorsRef.current,
      format,
      metric,
    });
    chartRef.current = chart;
    chart.update(pointsRef.current);

    const observer = new ResizeObserver(() => {
      chart.resize(host.clientWidth);
    });
    observer.observe(host);

    return () => {
      observer.disconnect();
      chart.destroy();
      chartRef.current = null;
    };
  }, [host, format, metric]);

  useEffect(() => {
    chartRef.current?.setColors({ stroke, fill, axis, grid });
  }, [stroke, fill, axis, grid]);

  useEffect(() => {
    chartRef.current?.update(points);
  }, [host, points]);

  return (
    <div ref={setHost} data-testid="metric-chart" className="w-full min-w-0 overflow-hidden" />
  );
}

export default function InspectMetrics({ namespace, pod }: InspectMetricsProps) {
  const colors = canvasColors(useResolvedTheme());
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
        <label className="text-fg-muted" htmlFor="metric-range">
          range
        </label>
        <select
          id="metric-range"
          aria-label="Metric range"
          value={span}
          onChange={handleSpan}
          className="rounded border border-edge-strong bg-surface-raised px-1 py-0.5 text-fg"
        >
          {RANGES.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        {history?.source !== undefined && (
          <span className="ml-auto truncate text-[10px] text-fg-muted">{history.source}</span>
        )}
      </div>

      <Announce message={error} urgent className="mt-3 break-words text-error" />
      {empty && (
        <p className="mt-3 text-fg-muted">
          Prometheus has no samples for this pod over the last {span}.
        </p>
      )}

      {history !== null && !empty && error === null && (
        <div className="mt-3">
          <div className="flex items-baseline justify-between">
            <span className="text-fg-soft">CPU</span>
            <span className="text-fg-muted">peak {cpuFromCores(peak(cpu))}</span>
          </div>
          <Chart
            points={cpu}
            stroke={colors.cpuStroke}
            fill={colors.cpuFill}
            axis={colors.chartAxis}
            grid={colors.chartGrid}
            format={cpuFromCores}
            metric="cpu"
          />

          <div className="mt-4 flex items-baseline justify-between">
            <span className="text-fg-soft">Memory</span>
            <span className="text-fg-muted">peak {memFromBytes(peak(memory))}</span>
          </div>
          <Chart
            points={memory}
            stroke={colors.memoryStroke}
            fill={colors.memoryFill}
            axis={colors.chartAxis}
            grid={colors.chartGrid}
            format={memFromBytes}
            metric="memory"
          />
        </div>
      )}
    </div>
  );
}
