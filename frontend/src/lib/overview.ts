import type { ClusterOverview } from './types';
import { request } from './http';
import { parseClusterOverview } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { failure } from './object';

const OVERVIEW_POLL_MS = 10000;

export async function fetchOverview(): Promise<ClusterOverview> {
  const response = await request('/api/overview');
  if (!response.ok) {
    throw await failure(response, `overview request failed with status ${response.status}`);
  }
  return parseClusterOverview(await response.json());
}

export function useOverview(enabled = true): Polled<ClusterOverview> {
  return usePoll(fetchOverview, {
    intervalMs: OVERVIEW_POLL_MS,
    enabled,
    fallback: 'overview request failed',
  });
}

export function percentOf(used: number, total: number): number {
  if (total <= 0) {
    return 0;
  }
  return Math.round((used / total) * 100);
}
