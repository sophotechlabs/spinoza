import type { Comparison, ObjectRef } from './types';
import { failure, refQuery } from './object';
import { parseComparison } from './parse';
import { request } from './http';

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
