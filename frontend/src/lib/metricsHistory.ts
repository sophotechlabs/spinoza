import type { MetricHistory } from './types';
import { failure } from './object';
import { request } from './http';
import { parseMetricHistory } from './parse';

export const RANGES = ['15m', '1h', '6h', '24h'] as const;

export type MetricRange = (typeof RANGES)[number];

export const DEFAULT_RANGE: MetricRange = '1h';

export async function fetchMetricHistory(
  namespace: string,
  pod: string,
  span: MetricRange,
): Promise<MetricHistory> {
  const params = new URLSearchParams({ namespace, pod, range: span });
  const response = await request(`/api/metrics/history?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `metric history failed with status ${response.status}`);
  }
  return parseMetricHistory(await response.json());
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
