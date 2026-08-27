import { useEffect, useRef, useState } from 'react';
import type { MetricHistory, MetricPoint } from '../lib/types';
import {
  DEFAULT_RANGE,
  collectedFor,
  fetchMetricHistory,
  peak,
  rangesFor,
} from '../lib/metricsHistory';
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

const SAMPLED_REFRESH_MS = 15000;

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
  const [sampled, setSampled] = useState(false);

  const podKey = `${namespace}/${pod}`;
  const [lastPod, setLastPod] = useState(podKey);
  if (podKey !== lastPod) {
    setLastPod(podKey);
    setHistory(null);
    setError(null);
  }

  const offered = rangesFor(sampled);
  // A span chosen before the answer said who was measuring may be one spinoza
  // cannot reach back to, which would leave the control showing nothing.
  if (!offered.includes(span)) {
    setSpan(DEFAULT_RANGE);
  }

  useEffect(() => {
    let live = true;
    setError(null);

    function load() {
      fetchMetricHistory(namespace, pod, span)
        .then((next) => {
          if (!live) {
            return;
          }
          setHistory(next);
          setSampled(next.sampled === true);
        })
        .catch((err: unknown) => {
          if (!live) {
            return;
          }
          setHistory(null);
          setError(errorMessage(err));
        });
    }

    load();
    if (!sampled) {
      return () => {
        live = false;
      };
    }
    // Readings taken here arrive while the panel is open, so it has to come back
    // for them. A metrics database already holds the whole span at once.
    const timer = setInterval(load, SAMPLED_REFRESH_MS);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [namespace, pod, span, sampled]);

  function handleSpan(event: React.ChangeEvent<HTMLSelectElement>) {
    setSpan(event.target.value as MetricRange);
  }

  const cpu = history?.cpu ?? [];
  const memory = history?.memory ?? [];
  const empty = history !== null && cpu.length === 0 && memory.length === 0;
  const collected = collectedFor(history?.since, Date.now());

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
          {offered.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
        {history?.source !== undefined && (
          <span className="ml-auto truncate text-[10px] text-fg-muted">{history.source}</span>
        )}
      </div>

      {sampled && (
        <p data-testid="sampled-notice" className="mt-2 text-fg-muted">
          Spinoza is measuring this itself — it found no Prometheus to ask. It reads the cluster
          every 15 seconds while this window is open, and remembers nothing between runs.
          {collected !== '' && <> Collected so far: {collected}.</>}
        </p>
      )}

      <Announce message={error} urgent className="mt-3 break-words text-error" />
      {empty && sampled && (
        <p className="mt-3 text-fg-muted">
          Nothing measured yet. The first readings appear within a few seconds.
        </p>
      )}
      {empty && !sampled && (
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
