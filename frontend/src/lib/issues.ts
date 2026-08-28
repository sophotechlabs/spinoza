import type { Issue, IssueQueue, Severity } from './types';
import { request } from './http';
import { parseIssueQueue } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const ISSUES_POLL_MS = 5000;

export async function fetchIssues(): Promise<IssueQueue> {
  const response = await request('/api/issues');
  if (!response.ok) {
    throw new Error(`issues request failed with status ${response.status}`);
  }
  return parseIssueQueue(await response.json());
}

export function useIssues(enabled = true): Polled<IssueQueue> {
  return usePoll(fetchIssues, {
    intervalMs: ISSUES_POLL_MS,
    enabled,
    fallback: 'issues request failed',
  });
}

export function countBySeverity(rows: Issue[]): Record<Severity, number> {
  const out: Record<Severity, number> = { fatal: 0, degraded: 0, warning: 0 };
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
  return 'Warning';
}

export function severityClass(severity: Severity): string {
  if (severity === 'fatal') {
    return 'text-error';
  }
  if (severity === 'degraded') {
    return 'text-warn';
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
