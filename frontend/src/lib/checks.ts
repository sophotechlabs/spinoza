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
  Baseline,
  Mute,
  Mutes,
  RuleFault,
  RuleFaults,
  NamespaceCount,
  ObjectRef,
} from './types';
import { failure } from './object';
import { request, SLOW_REQUEST_TIMEOUT_MS } from './http';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { useChecksFilter, useChecksInterval } from '../store/settings';
import type { ChecksFilter } from '../store/settings';
import { SEVERITY_FLOORS } from './settings';

const BASELINE_TIMEOUT_MS = SLOW_REQUEST_TIMEOUT_MS;

const SEVERITY_RANK: Record<CheckSeverity, number> = { high: 0, medium: 1, low: 2 };

export const CATEGORY_LABELS: Record<CheckCategory, string> = {
  security: 'Security',
  reliability: 'Reliability',
  efficiency: 'Efficiency',
};

export const CATEGORY_ORDER: CheckCategory[] = ['security', 'reliability', 'efficiency'];

export interface CheckFindingView {
  object: ObjectRef;
  cluster?: string;
  kind: string;
  container?: string;
  detail: string;
  patch?: string;
  severity: CheckSeverity;
  origin?: CheckOrigin;
  managedBy?: string;
  fresh: boolean;
  muted: boolean;
  mutedBy?: string;
  reason?: string;
}

export type CheckGroupView = Omit<CheckGroup, 'findings'> & { findings: CheckFindingView[] };

export interface CheckPageView {
  findings: CheckFindingView[];
  next: string;
}

export interface CheckReportView {
  groups: CheckGroupView[];
  namespaces: NamespaceCount[];
  baseline: string;
  baselineFrom: string;
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
    cluster: item.cluster,
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
    cluster: held.cluster,
    kind: held.kind,
    container: item.container,
    detail: item.detail ?? '',
    patch: item.patch,
    severity: severityOf(item.severity),
    origin: held.origin,
    managedBy: held.managedBy,
    fresh: item.new === true,
    muted: item.muted === true,
    mutedBy: item.mutedBy,
    reason: item.reason,
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

export function severityReason(finding: CheckFindingView): string {
  const parts = [`${finding.severity} for this object`];
  if (finding.origin === 'system') {
    parts.push('a cluster component, which you do not own');
  }
  if (finding.origin === 'packaged') {
    parts.push(`installed by ${finding.managedBy ?? 'a chart'}, so you change it through values`);
  }
  return parts.join(' — ');
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
    muted: item.muted,
    new: item.new,
    fixed: item.fixed,
    gone: item.gone,
    baselined: item.baselined,
    measured: item.measured,
    truncated: item.truncated,
    next: item.next,
    findings,
  };
}

export const NO_FILTER: ChecksFilter = {
  disabled: [],
  skipNamespaces: [],
  namespace: '',
  minSeverity: '',
  wholeCluster: true,
  everyKind: false,
  onlyNew: false,
  showMuted: false,
};

function fromParams(query: string): ChecksFilter {
  const params = new URLSearchParams(query);
  const floor = params.get('minSeverity') ?? '';
  return {
    disabled: namesIn(params.get('disabled')),
    skipNamespaces: namesIn(params.get('skipNamespaces')),
    minSeverity: SEVERITY_FLOORS.find((one) => one === floor) ?? '',
    namespace: params.get('namespace') ?? '',
    wholeCluster: params.get('wholeCluster') !== '0',
    everyKind: params.get('everyKind') === '1',
    onlyNew: params.get('onlyNew') === '1',
    showMuted: params.get('showMuted') === '1',
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
  if (keep.everyKind) {
    params.set('everyKind', '1');
  }
  if (keep.namespace !== '') {
    params.set('namespace', keep.namespace);
  }
  if (keep.onlyNew) {
    params.set('onlyNew', '1');
  }
  if (keep.showMuted) {
    params.set('showMuted', '1');
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

export async function fetchChecks(
  keep: ChecksFilter = NO_FILTER,
  fleet = false,
): Promise<CheckReportView> {
  const where = fleet ? '/api/checks/fleet' : '/api/checks';
  const response = await request(withParams(where, filterParams(keep)));
  if (!response.ok) {
    throw await failure(response, `the checks request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<CheckReport>;
  const objects = (body.objects ?? []).map(objectOf);
  return {
    groups: (body.groups ?? []).map((entry) => groupOf(entry, objects)),
    namespaces: body.namespaces ?? [],
    baseline: body.baseline ?? '',
    baselineFrom: body.baselineFrom ?? '',
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

async function sendMute(method: string, mute: Mute): Promise<Mute[]> {
  const response = await request('/api/checks/mutes', {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(mute),
  });
  if (!response.ok) {
    throw await failure(response, `the mute request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<Mutes>;
  return body.mutes ?? [];
}

export function muteFinding(mute: Mute): Promise<Mute[]> {
  return sendMute('POST', mute);
}

export function unmuteFinding(mute: Mute): Promise<Mute[]> {
  return sendMute('DELETE', mute);
}

async function sendBaseline(method: string): Promise<string> {
  const response = await request('/api/checks/baseline', {
    method,
    timeoutMs: BASELINE_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the baseline request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<Baseline>;
  return body.takenAt ?? '';
}

export function takeBaseline(): Promise<string> {
  return sendBaseline('POST');
}

export async function saveBaselineFile(): Promise<Blob> {
  const response = await request('/api/checks/baseline/file', {
    timeoutMs: BASELINE_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the baseline could not be saved (${String(response.status)})`);
  }
  return response.blob();
}

export async function loadBaselineFile(body: string): Promise<string> {
  const response = await request('/api/checks/baseline/file', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body,
    timeoutMs: BASELINE_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the baseline could not be loaded (${String(response.status)})`);
  }
  const held = (await response.json()) as Partial<Baseline>;
  return held.takenAt ?? '';
}

export function clearBaseline(): Promise<string> {
  return sendBaseline('DELETE');
}

export async function fetchMutes(): Promise<Mute[]> {
  const response = await request('/api/checks/mutes');
  if (!response.ok) {
    throw await failure(response, `the mutes request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<Mutes>;
  return body.mutes ?? [];
}

// The audit is built fresh for an export: what a browser holds stops at the
// findings it shows, so building the file here would truncate every capped
// check without saying so.
export async function exportChecks(keep: ChecksFilter): Promise<Blob> {
  const response = await request(withParams('/api/checks/export', filterParams(keep)));
  if (!response.ok) {
    throw await failure(response, `the export failed with status ${response.status}`);
  }
  return response.blob();
}

export async function ruleFaults(rules: string): Promise<RuleFault[]> {
  const response = await request('/api/checks/rules/faults', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: rules,
  });
  if (!response.ok) {
    throw await failure(response, `the rules request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<RuleFaults>;
  return body.faults ?? [];
}

// The ref a mute needs to name one object, which is the shape the audit files
// them under on the other side.
export function refKeyOf(object: ObjectRef): string {
  return [object.group, object.version, object.resource, object.namespace, object.name].join('/');
}

export function useChecks(fleet = false): Polled<CheckReportView> {
  const seconds = useChecksInterval();
  const keep = useChecksFilter();
  const query = filterParams(keep).toString();
  const load = useCallback(() => fetchChecks(fromParams(query), fleet), [query, fleet]);
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

// changeLabel says what moved since the baseline. A check the baseline never ran
// says so, rather than reporting every one of its findings as new.
export function changeLabel(group: CheckGroupView, baseline: string): string {
  if (baseline === '') {
    return '';
  }
  if (group.measured === true) {
    return 'measured, not compared';
  }
  if (group.baselined !== true) {
    return 'not in the baseline';
  }
  const parts: string[] = [];
  if ((group.new ?? 0) > 0) {
    parts.push(`${String(group.new)} new`);
  }
  if ((group.fixed ?? 0) > 0) {
    parts.push(`${String(group.fixed)} fixed`);
  }
  return parts.join(' · ');
}

export function mutedLabel(group: CheckGroupView): string {
  const muted = group.muted ?? 0;
  if (muted === 0) {
    return '';
  }
  return `${String(muted)} muted`;
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
