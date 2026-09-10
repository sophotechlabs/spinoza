import { useCallback } from 'react';
import type { Polled } from './usePoll';
import type { WasteReport, WasteRow } from './types';
import { request } from './http';
import { failure } from './object';
import { usePoll } from './usePoll';

const WASTE_POLL_MS = 300000;

async function fetchWaste(namespace: string): Promise<WasteReport> {
  const params = new URLSearchParams();
  if (namespace !== '') {
    params.set('namespace', namespace);
  }
  const query = params.toString();
  const suffix = query === '' ? '' : `?${query}`;
  const response = await request(`/api/waste${suffix}`);
  if (!response.ok) {
    throw await failure(response, `reserved-against-used failed with status ${response.status}`);
  }
  return (await response.json()) as WasteReport;
}

export function useWaste(namespace: string): Polled<WasteReport> {
  const read = useCallback(() => fetchWaste(namespace), [namespace]);
  return usePoll(read, {
    intervalMs: WASTE_POLL_MS,
    fallback: 'reserved-against-used failed',
    resetKey: namespace,
  });
}

export function cpuText(milli: number): string {
  if (milli >= 1000) {
    return `${(milli / 1000).toFixed(milli % 1000 === 0 ? 0 : 1)} cores`;
  }
  return `${String(milli)}m`;
}

export function memText(mebibytes: number): string {
  if (mebibytes >= 1024) {
    return `${(mebibytes / 1024).toFixed(1)} Gi`;
  }
  return `${String(Math.round(mebibytes))} Mi`;
}

export function shareOf(used: number, requested: number): number {
  if (requested <= 0) {
    return 0;
  }
  return Math.min(1, used / requested);
}

export function rowKey(row: WasteRow): string {
  return `${row.namespace}/${row.kind ?? ''}/${row.name ?? ''}`;
}
