import type { Category, ResourceDescriptor } from './types';

export const NESTED_CATEGORY = 'Custom Resources';

export interface ApiGroup {
  name: string;
  resources: ResourceDescriptor[];
}

export function groupByApiGroup(resources: ResourceDescriptor[]): ApiGroup[] {
  const byGroup = new Map<string, ResourceDescriptor[]>();
  for (const resource of resources) {
    const existing = byGroup.get(resource.group);
    if (existing === undefined) {
      byGroup.set(resource.group, [resource]);
    } else {
      existing.push(resource);
    }
  }
  return [...byGroup]
    .map(([name, group]) => ({ name, resources: group }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

export function isNested(category: string): boolean {
  return category === NESTED_CATEGORY;
}

function keyOf(descriptor: ResourceDescriptor): string {
  return `${descriptor.group}/${descriptor.version}/${descriptor.resource}`;
}

export function matchesKind(descriptor: ResourceDescriptor, query: string): boolean {
  const wanted = query.trim().toLowerCase();
  if (wanted === '') {
    return true;
  }
  if (descriptor.kind.toLowerCase().includes(wanted)) {
    return true;
  }
  if (descriptor.resource.toLowerCase().includes(wanted)) {
    return true;
  }
  return descriptor.group.toLowerCase().includes(wanted);
}

export function filterCategories(categories: Category[], query: string): Category[] {
  if (query.trim() === '') {
    return categories;
  }
  const out: Category[] = [];
  for (const category of categories) {
    const resources = category.resources.filter((one) => matchesKind(one, query));
    if (resources.length > 0) {
      out.push({ ...category, resources });
    }
  }
  return out;
}

export function shownInTree(
  categories: Category[],
  open: (section: string) => boolean,
  active: ResourceDescriptor | null,
): boolean {
  if (active === null) {
    return false;
  }
  const wanted = keyOf(active);
  for (const category of categories) {
    if (!category.resources.some((one) => keyOf(one) === wanted)) {
      continue;
    }
    if (!open(category.name)) {
      return false;
    }
    if (!isNested(category.name)) {
      return true;
    }
    for (const group of groupByApiGroup(category.resources)) {
      if (group.resources.some((one) => keyOf(one) === wanted)) {
        return open(`${category.name}/${group.name}`);
      }
    }
  }
  return false;
}
