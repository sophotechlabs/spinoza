import { useCallback } from 'react';
import type {
  CheckCategory,
  CheckFinding,
  CheckGroup,
  CheckReport,
  CheckSeverity,
  ObjectRef,
} from './types';
import { failure } from './object';
import { request } from './http';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const REFRESH_MS = 15000;

const SEVERITY_RANK: Record<CheckSeverity, number> = { high: 0, medium: 1, low: 2 };

export const CATEGORY_LABELS: Record<CheckCategory, string> = {
  security: 'Security',
  reliability: 'Reliability',
  efficiency: 'Efficiency',
};

export const CATEGORY_ORDER: CheckCategory[] = ['security', 'reliability', 'efficiency'];

function refOf(raw: unknown): ObjectRef {
  const item = raw as Partial<ObjectRef> | undefined;
  return {
    group: item?.group ?? '',
    version: item?.version ?? '',
    resource: item?.resource ?? '',
    namespace: item?.namespace ?? '',
    name: item?.name ?? '',
  };
}

function findingOf(raw: unknown): CheckFinding {
  const item = raw as Partial<CheckFinding>;
  return {
    object: refOf(item.object),
    kind: item.kind ?? '',
    container: item.container,
    detail: item.detail ?? '',
    patch: item.patch,
  };
}

function categoryOf(value: string | undefined): CheckCategory {
  if (value === 'security' || value === 'reliability' || value === 'efficiency') {
    return value;
  }
  return 'reliability';
}

function severityOf(value: string | undefined): CheckSeverity {
  if (value === 'high' || value === 'medium' || value === 'low') {
    return value;
  }
  return 'low';
}

function groupOf(raw: unknown): CheckGroup {
  const item = raw as Partial<CheckGroup>;
  return {
    id: item.id ?? '',
    title: item.title ?? '',
    category: categoryOf(item.category),
    severity: severityOf(item.severity),
    frameworks: item.frameworks,
    wrong: item.wrong ?? '',
    remedy: item.remedy ?? '',
    skipped: item.skipped,
    findings: (item.findings ?? []).map(findingOf),
  };
}

export async function fetchChecks(): Promise<CheckReport> {
  const response = await request('/api/checks');
  if (!response.ok) {
    throw await failure(response, `the checks request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<CheckReport>;
  return {
    groups: (body.groups ?? []).map(groupOf),
    scanned: body.scanned ?? 0,
    error: body.error,
  };
}

export function useChecks(): Polled<CheckReport> {
  const load = useCallback(() => fetchChecks(), []);
  return usePoll(load, { intervalMs: REFRESH_MS, fallback: 'the checks request failed' });
}

export function totalFindings(report: CheckReport): number {
  return report.groups.reduce((sum, group) => sum + group.findings.length, 0);
}

export function bySeverity(groups: CheckGroup[]): CheckGroup[] {
  return [...groups].sort((left, right) => {
    const rank = SEVERITY_RANK[left.severity] - SEVERITY_RANK[right.severity];
    if (rank !== 0) {
      return rank;
    }
    return right.findings.length - left.findings.length;
  });
}

export function inCategory(groups: CheckGroup[], category: CheckCategory): CheckGroup[] {
  return bySeverity(groups.filter((group) => group.category === category));
}

export function severityClass(severity: CheckSeverity): string {
  if (severity === 'high') {
    return 'text-error';
  }
  if (severity === 'medium') {
    return 'text-warn';
  }
  return 'text-fg-muted';
}

export function countLabel(group: CheckGroup): string {
  if (group.skipped !== undefined) {
    return 'no data';
  }
  if (group.findings.length === 0) {
    return 'clean';
  }
  return String(group.findings.length);
}

export function findingLabel(finding: CheckFinding): string {
  const parts = [finding.kind, refLabel(finding.object)];
  if (finding.container !== undefined && finding.container !== '') {
    parts.push(`container ${finding.container}`);
  }
  return parts.join(' · ');
}

export function refLabel(ref: ObjectRef): string {
  if (ref.namespace === '') {
    return ref.name;
  }
  return `${ref.namespace}/${ref.name}`;
}
