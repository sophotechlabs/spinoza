import type { ResourceDescriptor } from './types';

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
