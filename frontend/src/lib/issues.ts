import { useCallback, useState } from 'react';
import type { Issue, IssueQueue, Severity } from './types';
import { request } from './http';
import { parseIssueQueue } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const ISSUES_POLL_MS = 5000;

export const ISSUE_ORDERS = ['worst', 'newest', 'oldest'] as const;

export type IssueOrder = (typeof ISSUE_ORDERS)[number];

export function orderLabel(order: IssueOrder): string {
  if (order === 'newest') {
    return 'Newest first';
  }
  if (order === 'oldest') {
    return 'Oldest first';
  }
  return 'Worst first';
}

function queuePath(fleet: boolean): string {
  if (fleet) {
    return '/api/issues/fleet';
  }
  return '/api/issues';
}

export async function fetchIssues(order: IssueOrder = 'worst'): Promise<IssueQueue> {
  return queueFrom(`/api/issues?sort=${order}`);
}

async function queueFrom(path: string): Promise<IssueQueue> {
  const response = await request(path);
  if (!response.ok) {
    throw new Error(`issues request failed with status ${response.status}`);
  }
  return parseIssueQueue(await response.json());
}

export function useIssues(
  enabled = true,
  fleet = false,
  order: IssueOrder = 'worst',
): Polled<IssueQueue> {
  const read = useCallback(
    async () => queueFrom(`${queuePath(fleet)}?sort=${order}`),
    [fleet, order],
  );
  return usePoll(read, {
    intervalMs: ISSUES_POLL_MS,
    enabled,
    fallback: 'issues request failed',
  });
}

async function fetchIssuePage(
  after: string,
  fleet: boolean,
  order: IssueOrder,
): Promise<IssueQueue> {
  return queueFrom(`${queuePath(fleet)}?sort=${order}&after=${encodeURIComponent(after)}`);
}

export interface PagedIssues extends Polled<IssueQueue> {
  rows: Issue[];
  more: string;
  loadingMore: boolean;
  moreError: string;
  loadMore: () => void;
}

export function usePagedIssues(
  enabled = true,
  fleet = false,
  order: IssueOrder = 'worst',
): PagedIssues {
  const polled = useIssues(enabled, fleet, order);
  const [tail, setTail] = useState<Issue[]>([]);
  const [builtOn, setBuiltOn] = useState('');
  const [next, setNext] = useState('');
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState('');

  const head = polled.data?.next ?? '';
  const first = polled.data?.rows ?? [];
  const joined = builtOn === head;
  let rows = first;
  let more = head;
  if (joined) {
    rows = [...first, ...tail];
    more = next;
  }

  function loadMore() {
    if (more === '' || loadingMore) {
      return;
    }
    setLoadingMore(true);
    setMoreError('');
    fetchIssuePage(more, fleet, order)
      .then((page) => {
        if (joined) {
          setTail((current) => [...current, ...page.rows]);
        } else {
          setTail(page.rows);
          setBuiltOn(head);
        }
        setNext(page.next ?? '');
      })
      .catch((reason: unknown) => {
        if (reason instanceof Error) {
          setMoreError(reason.message);
          return;
        }
        setMoreError('issues request failed');
      })
      .finally(() => {
        setLoadingMore(false);
      });
  }

  return { ...polled, rows, more, loadingMore, moreError, loadMore };
}

export function countBySeverity(rows: Issue[]): Record<Severity, number> {
  const out: Record<Severity, number> = { fatal: 0, degraded: 0, warning: 0, info: 0 };
  for (const row of rows) {
    out[row.severity] += 1;
  }
  return out;
}

export function severityLabel(severity: Severity): string {
  if (severity === 'fatal') {
    return 'Broken';
  }
  if (severity === 'degraded') {
    return 'Degraded';
  }
  if (severity === 'info') {
    return 'Note';
  }
  return 'Warning';
}

export function severityClass(severity: Severity): string {
  if (severity === 'fatal') {
    return 'text-error';
  }
  if (severity === 'degraded') {
    return 'text-warn';
  }
  if (severity === 'info') {
    return 'text-fg-soft';
  }
  return 'text-fg-muted';
}

export function foldedLabel(row: Issue): string {
  if (row.folded === 0) {
    return '';
  }
  if (row.folded === 1) {
    return '1 object';
  }
  return `${String(row.folded)} objects`;
}

export function hiddenChildren(row: Issue): number {
  const shown = row.children?.length ?? 0;
  if (row.folded <= shown) {
    return 0;
  }
  return row.folded - shown;
}
