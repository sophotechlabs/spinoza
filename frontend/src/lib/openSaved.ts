import type { Category, SavedView, View } from './types';
import type { Route } from './router';
import type { Chip } from './filterChips';
import { chipsFromText } from './filterChips';

export interface Opening {
  route: Route;
  namespace: string | null;
  filter: { key: string; chips: Chip[] } | null;
}

function routeToKind(route: Route, categories: Category[], resource: string): Route | null {
  const found = categories
    .flatMap((category) => category.resources)
    .find((one) => one.resource === resource);
  if (found === undefined) {
    return null;
  }
  return {
    ...route,
    view: 'resources',
    resource: {
      group: found.group,
      version: found.version,
      resource: found.resource,
      kind: found.kind,
    },
    selection: null,
  };
}

export function openingSaved(route: Route, saved: SavedView, categories: Category[]): Opening {
  const namespace =
    saved.namespace !== undefined && saved.namespace !== '' ? saved.namespace : null;
  const wanted = saved.resource ?? '';
  const filter =
    saved.filter !== undefined && wanted !== ''
      ? { key: wanted, chips: chipsFromText(saved.filter) }
      : null;
  const asKind = wanted === '' ? null : routeToKind(route, categories, wanted);
  if (asKind !== null) {
    return { route: asKind, namespace, filter };
  }
  return { route: { ...route, view: saved.view as View, selection: null }, namespace, filter };
}

export function kindResource(kind: string): string {
  if (kind === '') {
    return 'pods';
  }
  return `${kind.toLowerCase()}s`;
}

export function openingKind(route: Route, categories: Category[], kind: string): Route {
  const asKind = routeToKind(route, categories, kindResource(kind));
  if (asKind === null) {
    return { ...route, view: 'resources' };
  }
  return asKind;
}
