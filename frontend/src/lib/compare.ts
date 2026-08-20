import type { Comparison, KindComparison, ObjectRef, ResourceDescriptor } from './types';
import { failure, refQuery } from './object';
import { parseComparison, parseKindComparison } from './parse';
import { request, SLOW_REQUEST_TIMEOUT_MS } from './http';

export interface CompareTarget {
  kubeconfig: string;
  name: string;
  namespace: string;
  object: string;
}

export function differingLines(left: string, right: string): number {
  const here = left.split('\n');
  const there = right.split('\n');
  const kept = new Set(here.filter((line) => there.includes(line)));
  let differing = 0;
  for (const line of [...here, ...there]) {
    if (!kept.has(line)) {
      differing += 1;
    }
  }
  return differing;
}

export function changedSections(left: string, right: string): string[] {
  const sections = new Set<string>();
  const here = left.split('\n');
  const there = right.split('\n');
  let section = '';
  for (const line of [...here, ...there]) {
    if (/^\S/.test(line)) {
      section = line.split(':')[0];
    }
    if (section !== '' && !here.includes(line)) {
      sections.add(section);
      continue;
    }
    if (section !== '' && !there.includes(line)) {
      sections.add(section);
    }
  }
  return [...sections].sort((left_, right_) => left_.localeCompare(right_));
}

export async function fetchComparison(
  ref: ObjectRef,
  target: CompareTarget,
  raw: boolean,
): Promise<Comparison> {
  const params = new URLSearchParams(refQuery(ref));
  params.set('against', target.name);
  params.set('againstKubeconfig', target.kubeconfig);
  if (target.namespace !== '' && target.namespace !== ref.namespace) {
    params.set('againstNamespace', target.namespace);
  }
  if (target.object !== '' && target.object !== ref.name) {
    params.set('againstName', target.object);
  }
  if (raw) {
    params.set('raw', 'true');
  }
  const response = await request(`/api/compare?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `compare failed with status ${response.status}`);
  }
  return parseComparison(await response.json());
}

export async function fetchKindComparison(
  kind: ResourceDescriptor,
  namespace: string,
  target: CompareTarget,
): Promise<KindComparison> {
  const params = new URLSearchParams({
    group: kind.group,
    version: kind.version,
    resource: kind.resource,
    namespace,
    against: target.name,
    againstKubeconfig: target.kubeconfig,
  });
  if (target.namespace !== '' && target.namespace !== namespace) {
    params.set('againstNamespace', target.namespace);
  }
  const response = await request(`/api/compare/kind?${params.toString()}`, {
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `comparing the kind failed with status ${response.status}`);
  }
  return parseKindComparison(await response.json());
}

export function summaryOf(result: KindComparison): string {
  const parts = [`${String(result.same)} same`, `${String(result.differs)} differ`];
  if (result.onlyHere > 0) {
    parts.push(`${String(result.onlyHere)} only on ${result.leftContext}`);
  }
  if (result.onlyThere > 0) {
    parts.push(`${String(result.onlyThere)} only on ${result.rightContext}`);
  }
  return parts.join(' · ');
}
