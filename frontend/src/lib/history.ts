import type { History, HistoryEntry, ObjectRef } from './types';
import { request } from './http';
import { failure } from './object';
import { clock } from './time';
import { parseHistory } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const HISTORY_POLL_MS = 15000;

export const HISTORY_LIMIT = 200;

export async function fetchHistory(): Promise<History> {
  const params = new URLSearchParams({ limit: String(HISTORY_LIMIT) });
  const response = await request(`/api/history?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `history request failed with status ${response.status}`);
  }
  return parseHistory(await response.json());
}

export function useHistory(enabled = true): Polled<History> {
  return usePoll(fetchHistory, {
    intervalMs: HISTORY_POLL_MS,
    enabled,
    fallback: 'history request failed',
  });
}

export async function forgetHistory(): Promise<void> {
  const response = await request('/api/history', { method: 'DELETE' });
  if (!response.ok) {
    throw await failure(response, `clearing history failed with status ${response.status}`);
  }
}

export function outcomeLabel(outcome: string): string {
  if (outcome === 'refused') {
    return 'Refused';
  }
  if (outcome === 'failed') {
    return 'Failed';
  }
  return 'Done';
}

export function outcomeClass(outcome: string): string {
  if (outcome === 'refused') {
    return 'text-warn';
  }
  if (outcome === 'failed') {
    return 'text-error';
  }
  return 'text-fg-muted';
}

export function targetLabel(entry: HistoryEntry): string {
  const what = entry.kind ?? entry.resource ?? '';
  if (what === '') {
    return entry.name;
  }
  return `${what} ${entry.name}`;
}

export function when(at: string, now: number): string {
  const stamp = new Date(at);
  if (Number.isNaN(stamp.getTime())) {
    return '';
  }
  if (stamp.toDateString() === new Date(now).toDateString()) {
    return clock(at);
  }
  return `${dayLabel(stamp)} ${clock(at)}`;
}

function dayLabel(stamp: Date): string {
  const month = String(stamp.getMonth() + 1).padStart(2, '0');
  const day = String(stamp.getDate()).padStart(2, '0');
  return `${month}-${day}`;
}

export function scopeLabel(entry: HistoryEntry): string {
  if (entry.namespace === undefined) {
    return 'cluster-wide';
  }
  if (entry.namespace === '') {
    return 'cluster-wide';
  }
  return entry.namespace;
}

export function refOf(entry: HistoryEntry): ObjectRef | null {
  if (entry.resource === undefined) {
    return null;
  }
  if (entry.resource === '') {
    return null;
  }
  return {
    group: entry.group ?? '',
    version: entry.version ?? '',
    resource: entry.resource,
    namespace: entry.namespace ?? '',
    name: entry.name,
  };
}

export function detailText(entry: HistoryEntry): string {
  if (entry.message !== undefined && entry.message !== '') {
    return entry.message;
  }
  return entry.detail ?? '';
}

export function clearFailure(err: unknown): string {
  if (err instanceof Error) {
    return `Clearing history: ${err.message}`;
  }
  return 'Clearing history failed';
}
