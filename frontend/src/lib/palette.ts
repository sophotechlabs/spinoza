import type { Category, ObjectRef, ResourceDescriptor, SearchHit, View } from './types';
import { refOf } from './search';
import { argoInstalled, fluxInstalled } from './gitops';

export const VIEW_LABELS: Record<View, string> = {
  resources: 'Resources',
  cluster: 'Cluster overview',
  helm: 'Helm releases',
  'flux-roles': 'Flux overview',
  gitops: 'Flux graph',
  'flux-list': 'Flux resources',
  'argo-apps': 'Argo CD applications',
};

const VIEW_ORDER: View[] = ['cluster', 'resources', 'helm', 'flux-roles', 'gitops', 'flux-list'];

export type PaletteItem =
  | { id: string; label: string; hint: string; kind: 'view'; view: View }
  | { id: string; label: string; hint: string; kind: 'resource'; descriptor: ResourceDescriptor }
  | { id: string; label: string; hint: string; kind: 'object'; ref: ObjectRef };

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

export function clusterItems(hits: SearchHit[]): PaletteItem[] {
  return hits.map((hit) => ({
    id: `found:${hit.group}/${hit.version}/${hit.resource}/${hit.namespace}/${hit.name}`,
    label: refLabel(refOf(hit)),
    hint: hit.kind.toLowerCase(),
    kind: 'object' as const,
    ref: refOf(hit),
  }));
}

const FLUX_VIEWS: View[] = ['flux-roles', 'gitops', 'flux-list'];

function offered(categories: Category[]): View[] {
  const hidden: View[] = [];
  if (!fluxInstalled(categories)) {
    hidden.push(...FLUX_VIEWS);
  }
  if (!argoInstalled(categories)) {
    hidden.push('argo-apps');
  }
  return VIEW_ORDER.filter((view) => !hidden.includes(view));
}

export function paletteItems(categories: Category[], recents: ObjectRef[]): PaletteItem[] {
  const items: PaletteItem[] = [];
  for (const ref of recents) {
    items.push({
      id: `object:${ref.group}/${ref.version}/${ref.resource}/${ref.namespace}/${ref.name}`,
      label: refLabel(ref),
      hint: `recent ${ref.resource}`,
      kind: 'object',
      ref,
    });
  }
  for (const view of offered(categories)) {
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
