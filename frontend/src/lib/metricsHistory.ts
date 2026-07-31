import type { MetricHistory } from './types';
import { failure } from './object';

export const RANGES = ['15m', '1h', '6h', '24h'] as const;

export type MetricRange = (typeof RANGES)[number];

export const DEFAULT_RANGE: MetricRange = '1h';

export async function fetchMetricHistory(
  namespace: string,
  pod: string,
  span: MetricRange,
): Promise<MetricHistory> {
  const params = new URLSearchParams({ namespace, pod, range: span });
  const response = await fetch(`/api/metrics/history?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `metric history failed with status ${response.status}`);
  }
  return (await response.json()) as MetricHistory;
}

export function formatCpu(value: number): string {
  return `${(value * 1000).toFixed(0)}m`;
}

export function formatMemory(value: number): string {
  const mib = value / (1024 * 1024);
  if (mib >= 1024) {
    return `${(mib / 1024).toFixed(2)} GiB`;
  }
  return `${mib.toFixed(0)} MiB`;
}

export function peak(points: { value: number }[]): number {
  let highest = 0;
  for (const point of points) {
    if (point.value > highest) {
      highest = point.value;
    }
  }
  return highest;
}
