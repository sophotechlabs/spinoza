import type { ResourceDescriptor } from './types';

const CORE = 'core';

function groupOf(descriptor: ResourceDescriptor): string {
  if (descriptor.group === '') {
    return CORE;
  }
  return descriptor.group;
}

export function kindLabels(resources: ResourceDescriptor[]): Record<string, string> {
  const seen = new Map<string, number>();
  for (const resource of resources) {
    seen.set(resource.kind, (seen.get(resource.kind) ?? 0) + 1);
  }
  const out: Record<string, string> = {};
  for (const resource of resources) {
    const key = `${resource.group}/${resource.version}/${resource.resource}`;
    if ((seen.get(resource.kind) ?? 0) > 1) {
      out[key] = `${resource.kind} (${groupOf(resource)})`;
      continue;
    }
    out[key] = resource.kind;
  }
  return out;
}
