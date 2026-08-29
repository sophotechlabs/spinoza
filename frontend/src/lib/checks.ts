import { useCallback } from 'react';
import type {
  CheckCategory,
  CheckFinding,
  CheckGroup,
  CheckObject,
  CheckOrigin,
  CheckPage,
  CheckReport,
  CheckSeverity,
  ObjectRef,
} from './types';
import { failure } from './object';
import { request } from './http';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { useChecksFilter, useChecksInterval } from '../store/settings';
import type { SeverityFloor } from './settings';
import { SEVERITY_FLOORS } from './settings';

const SEVERITY_RANK: Record<CheckSeverity, number> = { high: 0, medium: 1, low: 2 };

export const CATEGORY_LABELS: Record<CheckCategory, string> = {
  security: 'Security',
  reliability: 'Reliability',
  efficiency: 'Efficiency',
};

export const CATEGORY_ORDER: CheckCategory[] = ['security', 'reliability', 'efficiency'];

export interface CheckFindingView {
  object: ObjectRef;
  kind: string;
  container?: string;
  detail: string;
  patch?: string;
  origin?: CheckOrigin;
  managedBy?: string;
}

export type CheckGroupView = Omit<CheckGroup, 'findings'> & { findings: CheckFindingView[] };

export interface CheckPageView {
  findings: CheckFindingView[];
  next: string;
}

export interface CheckReportView {
  groups: CheckGroupView[];
  scanned: number;
  error?: string;
}

function objectOf(raw: unknown): CheckObject {
  const item = raw as Partial<CheckObject>;
  return {
    group: item.group ?? '',
    version: item.version ?? '',
    resource: item.resource ?? '',
    namespace: item.namespace ?? '',
    name: item.name ?? '',
    kind: item.kind ?? '',
    origin: originOf(item.origin),
    managedBy: item.managedBy,
  };
}

const UNKNOWN: CheckObject = {
  group: '',
  version: '',
  resource: '',
  namespace: '',
  name: '',
  kind: '',
};

function findingOf(raw: unknown, objects: CheckObject[]): CheckFindingView {
  const item = raw as Partial<CheckFinding>;
  const held = objects[item.ref ?? -1] ?? UNKNOWN;
  return {
    object: {
      group: held.group,
      version: held.version,
      resource: held.resource,
      namespace: held.namespace,
      name: held.name,
    },
    kind: held.kind,
    container: item.container,
    detail: item.detail ?? '',
    patch: item.patch,
    origin: held.origin,
    managedBy: held.managedBy,
  };
}

function originOf(value: string | undefined): CheckOrigin | undefined {
  if (value === 'packaged' || value === 'system') {
    return value;
  }
  return undefined;
}

export function originLabel(finding: CheckFindingView): string {
  if (finding.managedBy !== undefined && finding.managedBy !== '') {
    return finding.managedBy;
  }
  if (finding.origin === 'system') {
    return 'cluster';
  }
  return '';
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

function groupOf(raw: unknown, objects: CheckObject[]): CheckGroupView {
  const item = raw as Partial<CheckGroup>;
  const findings = (item.findings ?? []).map((entry) => findingOf(entry, objects));
  return {
    id: item.id ?? '',
    title: item.title ?? '',
    category: categoryOf(item.category),
    severity: severityOf(item.severity),
    frameworks: item.frameworks,
    wrong: item.wrong ?? '',
    remedy: item.remedy ?? '',
    skipped: item.skipped,
    total: item.total ?? findings.length,
    truncated: item.truncated,
    next: item.next,
    findings,
  };
}

export interface ChecksFilter {
  disabled: string[];
  skipNamespaces: string[];
  minSeverity: SeverityFloor;
  wholeCluster: boolean;
}

export const NO_FILTER: ChecksFilter = {
  disabled: [],
  skipNamespaces: [],
  minSeverity: '',
  wholeCluster: true,
};

export function fromParams(query: string): ChecksFilter {
  const params = new URLSearchParams(query);
  const floor = params.get('minSeverity') ?? '';
  return {
    disabled: namesIn(params.get('disabled')),
    skipNamespaces: namesIn(params.get('skipNamespaces')),
    minSeverity: SEVERITY_FLOORS.find((one) => one === floor) ?? '',
    wholeCluster: params.get('wholeCluster') !== '0',
  };
}

function namesIn(raw: string | null): string[] {
  if (raw === null || raw === '') {
    return [];
  }
  return raw.split(',');
}

export function filterParams(keep: ChecksFilter): URLSearchParams {
  const params = new URLSearchParams();
  if (keep.disabled.length > 0) {
    params.set('disabled', keep.disabled.join(','));
  }
  if (keep.skipNamespaces.length > 0) {
    params.set('skipNamespaces', keep.skipNamespaces.join(','));
  }
  if (keep.minSeverity !== '') {
    params.set('minSeverity', keep.minSeverity);
  }
  if (!keep.wholeCluster) {
    params.set('wholeCluster', '0');
  }
  return params;
}

function withParams(path: string, params: URLSearchParams): string {
  const query = params.toString();
  if (query === '') {
    return path;
  }
  return `${path}?${query}`;
}

export async function fetchChecks(keep: ChecksFilter = NO_FILTER): Promise<CheckReportView> {
  const response = await request(withParams('/api/checks', filterParams(keep)));
  if (!response.ok) {
    throw await failure(response, `the checks request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<CheckReport>;
  const objects = (body.objects ?? []).map(objectOf);
  return {
    groups: (body.groups ?? []).map((entry) => groupOf(entry, objects)),
    scanned: body.scanned ?? 0,
    error: body.error,
  };
}

export async function fetchCheckPage(
  check: string,
  after: string,
  keep: ChecksFilter = NO_FILTER,
): Promise<CheckPageView> {
  const params = filterParams(keep);
  params.set('check', check);
  params.set('after', after);
  const response = await request(`/api/checks/findings?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `the findings request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<CheckPage>;
  const objects = (body.objects ?? []).map(objectOf);
  return {
    findings: (body.findings ?? []).map((entry) => findingOf(entry, objects)),
    next: body.next ?? '',
  };
}

export function useChecks(): Polled<CheckReportView> {
  const seconds = useChecksInterval();
  const keep = useChecksFilter();
  const query = filterParams(keep).toString();
  const load = useCallback(() => fetchChecks(fromParams(query)), [query]);
  return usePoll(load, { intervalMs: seconds * 1000, fallback: 'the checks request failed' });
}

export function totalFindings(report: CheckReportView): number {
  return report.groups.reduce((sum, group) => sum + group.total, 0);
}

export function bySeverity(groups: CheckGroupView[]): CheckGroupView[] {
  return [...groups].sort((left, right) => {
    const rank = SEVERITY_RANK[left.severity] - SEVERITY_RANK[right.severity];
    if (rank !== 0) {
      return rank;
    }
    return right.total - left.total;
  });
}

export function inCategory(groups: CheckGroupView[], category: CheckCategory): CheckGroupView[] {
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

export function countLabel(group: CheckGroupView): string {
  if (group.skipped !== undefined) {
    return 'no data';
  }
  if (group.total === 0) {
    return 'clean';
  }
  return String(group.total);
}

export function shownLabel(group: CheckGroupView, loaded: number): string {
  if (loaded >= group.total) {
    return '';
  }
  return `Showing ${String(loaded)} of ${String(group.total)}.`;
}

export function findingLabel(finding: CheckFindingView): string {
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
