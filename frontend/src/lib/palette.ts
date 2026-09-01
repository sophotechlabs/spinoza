import type { Category, ObjectRef, ResourceDescriptor, SearchHit, View } from './types';
import { ARGO_VIEWS, FLEET_VIEWS, FLUX_VIEWS } from './types';
import { refOf } from './search';
import { typeFor } from './catalog';
import { argoInstalled, fluxInstalled } from './gitops';
import { useClustersStore } from '../store/clusters';
import { contextOf } from './tabs';
import { VIEW_LABELS } from './views';

const VIEW_ORDER: View[] = [
  'fleet',
  'cluster',
  'resources',
  'issues',
  'topology',
  'helm',
  'checks',
  'history',
  'flux-roles',
  'gitops',
  'flux-list',
  'argo-apps',
  'argo-graph',
  'argo-list',
  'traffic',
  'rbac',
];

export type PaletteItem =
  | { id: string; label: string; hint: string; kind: 'view'; view: View }
  | { id: string; label: string; hint: string; kind: 'resource'; descriptor: ResourceDescriptor }
  | {
      id: string;
      label: string;
      hint: string;
      kind: 'object';
      ref: ObjectRef;
      cluster?: string;
      type: ResourceDescriptor | null;
    };

export interface PaletteOpen {
  ref: ObjectRef;
  type: ResourceDescriptor | null;
  filter: string;
  cluster?: string;
}

function groupLabel(descriptor: ResourceDescriptor): string {
  if (descriptor.group === '') {
    return descriptor.version;
  }
  return `${descriptor.group}/${descriptor.version}`;
}

function refLabel(ref: ObjectRef): string {
  if (ref.namespace === '') {
    return ref.name;
  }
  return `${ref.namespace}/${ref.name}`;
}

export function clusterItems(hits: SearchHit[], categories: Category[]): PaletteItem[] {
  return hits.map((hit) => ({
    id: `found:${hit.cluster ?? ''}/${hit.group}/${hit.version}/${hit.resource}/${hit.namespace}/${hit.name}`,
    label: refLabel(refOf(hit)),
    hint: hintFor(hit),
    kind: 'object' as const,
    ref: refOf(hit),
    cluster: hit.cluster,
    type: typeFor(categories, hit),
  }));
}

function hintFor(hit: SearchHit): string {
  if (hit.cluster === undefined || hit.cluster === '') {
    return hit.kind.toLowerCase();
  }
  return `${hit.kind.toLowerCase()} · ${contextOf(useClustersStore.getState().tabs, hit.cluster)}`;
}

function offered(categories: Category[], traffic: boolean): View[] {
  const hidden: View[] = [];
  if (!fluxInstalled(categories)) {
    hidden.push(...FLUX_VIEWS);
  }
  if (!argoInstalled(categories)) {
    hidden.push(...ARGO_VIEWS);
  }
  if (!traffic) {
    hidden.push('traffic');
  }
  if (useClustersStore.getState().tabs.length < 2) {
    hidden.push(...FLEET_VIEWS);
  }
  return VIEW_ORDER.filter((view) => !hidden.includes(view));
}

export function paletteItems(
  categories: Category[],
  recents: ObjectRef[],
  traffic: boolean,
): PaletteItem[] {
  const items: PaletteItem[] = [];
  for (const ref of recents) {
    items.push({
      id: `object:${ref.group}/${ref.version}/${ref.resource}/${ref.namespace}/${ref.name}`,
      label: refLabel(ref),
      hint: `recent ${ref.resource}`,
      kind: 'object',
      ref,
      type: typeFor(categories, ref),
    });
  }
  for (const view of offered(categories, traffic)) {
    items.push({ id: `view:${view}`, label: VIEW_LABELS[view], hint: 'view', kind: 'view', view });
  }
  for (const category of categories) {
    for (const descriptor of category.resources) {
      items.push({
        id: `resource:${descriptor.group}/${descriptor.version}/${descriptor.resource}`,
        label: descriptor.kind,
        hint: `${category.name} ${groupLabel(descriptor)}`,
        kind: 'resource',
        descriptor,
      });
    }
  }
  return items;
}

function rank(item: PaletteItem, needle: string): number {
  const label = item.label.toLowerCase();
  if (label === needle) {
    return 0;
  }
  if (label.startsWith(needle)) {
    return 1;
  }
  if (label.includes(needle)) {
    return 2;
  }
  if (item.hint.toLowerCase().includes(needle)) {
    return 3;
  }
  return -1;
}

export function matchItems(items: PaletteItem[], query: string): PaletteItem[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return items;
  }
  const scored: { item: PaletteItem; score: number; at: number }[] = [];
  items.forEach((item, at) => {
    const score = rank(item, needle);
    if (score < 0) {
      return;
    }
    scored.push({ item, score, at });
  });
  scored.sort((a, b) => {
    if (a.score !== b.score) {
      return a.score - b.score;
    }
    return a.at - b.at;
  });
  return scored.map((entry) => entry.item);
}
