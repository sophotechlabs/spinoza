import type { History, HistoryEntry, Memory, ObjectRef } from './types';
import { request } from './http';
import { failure } from './object';
import { clock } from './time';
import { parseHistory } from './parse';
import { useCallback } from 'react';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const HISTORY_POLL_MS = 15000;

export const HISTORY_LIMIT = 200;

export const SOURCES = ['all', 'change', 'action'] as const;

export type HistorySource = (typeof SOURCES)[number];

export interface HistoryAsk {
  source?: HistorySource;
  after?: number;
  fleet?: boolean;
}

export async function fetchHistory(ask: HistoryAsk = {}): Promise<History> {
  const params = new URLSearchParams({
    limit: String(HISTORY_LIMIT),
    source: ask.source ?? 'all',
  });
  if (ask.after !== undefined && ask.after > 0) {
    params.set('after', String(ask.after));
  }
  if (ask.fleet === true) {
    params.set('fleet', 'true');
  }
  const response = await request(`/api/history?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `history request failed with status ${response.status}`);
  }
  return parseHistory(await response.json());
}

export async function fetchMemory(): Promise<Memory> {
  const response = await request('/api/memory');
  if (!response.ok) {
    throw new Error(`memory request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<Memory>;
  return { heapMi: body.heapMi ?? 0, sysMi: body.sysMi ?? 0 };
}

const MEMORY_POLL_MS = 20000;

export function useMemory(enabled = true): Polled<Memory> {
  return usePoll(fetchMemory, {
    intervalMs: MEMORY_POLL_MS,
    enabled,
    fallback: 'memory request failed',
  });
}

export function useHistory(ask: HistoryAsk = {}, enabled = true): Polled<History> {
  const { source, fleet } = ask;
  const read = useCallback(async () => fetchHistory({ source, fleet }), [source, fleet]);
  return usePoll(read, {
    intervalMs: HISTORY_POLL_MS,
    enabled,
    fallback: 'history request failed',
  });
}

export function foldRepeats(entries: HistoryEntry[]): FoldedEntry[] {
  const out: FoldedEntry[] = [];
  for (const entry of entries) {
    const last = out.at(-1);
    if (last !== undefined && sameObject(last.entry, entry)) {
      last.repeats += 1;
      last.oldest = entry;
      continue;
    }
    out.push({ entry, repeats: 1, oldest: entry });
  }
  return out;
}

export interface FoldedEntry {
  entry: HistoryEntry;
  repeats: number;
  oldest: HistoryEntry;
}

function sameObject(left: HistoryEntry, right: HistoryEntry): boolean {
  if (left.source !== right.source || left.source !== 'change') {
    return false;
  }
  if (left.cluster !== right.cluster) {
    return false;
  }
  return left.name === right.name && left.namespace === right.namespace;
}

export function cursorOf(page: History, older: HistoryEntry[]): number {
  const last = older.at(-1);
  if (last !== undefined) {
    return last.id;
  }
  return page.next ?? 0;
}

export function reachable(page: History, older: HistoryEntry[]): boolean {
  if (cursorOf(page, older) === 0) {
    return false;
  }
  if (older.length === 0) {
    return page.more === true;
  }
  return older.length % HISTORY_LIMIT === 0;
}

export function olderFailure(err: unknown): string {
  if (err instanceof Error) {
    return `Reaching further back: ${err.message}`;
  }
  return 'Reaching further back failed';
}

export function repeatLabel(folded: FoldedEntry): string {
  if (folded.repeats < 2) {
    return '';
  }
  return `changed ${String(folded.repeats)} times`;
}

export function sourceLabel(source: HistorySource): string {
  if (source === 'change') {
    return 'What changed';
  }
  if (source === 'action') {
    return 'What I did';
  }
  return 'Everything';
}

export function verbLabel(entry: HistoryEntry): string {
  if (entry.source !== 'change') {
    return entry.verb;
  }
  if (entry.verb === 'added') {
    return 'appeared';
  }
  if (entry.verb === 'removed') {
    return 'went';
  }
  return 'changed';
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

export function wasText(entry: HistoryEntry): string {
  if (entry.was === undefined || entry.was === '') {
    return '';
  }
  if (entry.verb === 'removed') {
    return '';
  }
  return `${entry.was} → `;
}

export function detailText(entry: HistoryEntry): string {
  if (entry.message !== undefined && entry.message !== '') {
    return entry.message;
  }
  if (entry.verb === 'removed' && entry.was !== undefined && entry.was !== '') {
    return entry.was;
  }
  return entry.detail ?? '';
}

export function recordFailure(err: unknown): string {
  if (err instanceof Error) {
    return `Changing what is recorded: ${err.message}`;
  }
  return 'Changing what is recorded failed';
}

export function clearFailure(err: unknown): string {
  if (err instanceof Error) {
    return `Clearing history: ${err.message}`;
  }
  return 'Clearing history failed';
}
