import type { Category, ResourceDescriptor } from './types';

export interface Gvr {
  group: string;
  version: string;
  resource: string;
}

function gvrKey(gvr: Gvr): string {
  return `${gvr.group}/${gvr.version}/${gvr.resource}`;
}

export function typeFor(categories: Category[], gvr: Gvr): ResourceDescriptor | null {
  const wanted = gvrKey(gvr);
  for (const category of categories) {
    for (const descriptor of category.resources) {
      if (gvrKey(descriptor) === wanted) {
        return descriptor;
      }
    }
  }
  return null;
}

export function kindScope(categories: Category[], gvr: Gvr | null): boolean | null {
  if (gvr === null) {
    return null;
  }
  const found = typeFor(categories, gvr);
  if (found === null) {
    return null;
  }
  return found.namespaced;
}

export function scopedBy(scope: boolean | null, namespaced: boolean): boolean {
  if (scope === null) {
    return namespaced;
  }
  return scope;
}
