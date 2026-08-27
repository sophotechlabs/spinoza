import type { MetricHistory } from './types';
import { failure } from './object';
import { request } from './http';
import { parseMetricHistory } from './parse';

export const RANGES = ['15m', '1h', '6h', '24h'] as const;

export type MetricRange = (typeof RANGES)[number];

export const DEFAULT_RANGE: MetricRange = '1h';

// What spinoza collects itself reaches back an hour at most, so the longer
// ranges are not offered: a chart labelled 24h showing eleven minutes says the
// pod started eleven minutes ago, which is not what happened.
export const SAMPLED_RANGES: readonly MetricRange[] = ['15m', '1h'];

export function rangesFor(sampled: boolean): readonly MetricRange[] {
  if (sampled) {
    return SAMPLED_RANGES;
  }
  return RANGES;
}

// collectedFor says how long the readings on hand cover, in the words a person
// would use. It is deliberately not the span asked for — that is the point of
// showing it.
export function collectedFor(since: number | undefined, now: number): string {
  if (since === undefined || since <= 0) {
    return '';
  }
  const minutes = Math.floor((now - since) / 60000);
  if (minutes < 1) {
    return 'less than a minute';
  }
  if (minutes === 1) {
    return '1 minute';
  }
  if (minutes < 60) {
    return `${minutes} minutes`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours === 1) {
    return '1 hour';
  }
  return `${hours} hours`;
}

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
