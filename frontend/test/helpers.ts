import type { Category, Column, ResourceDescriptor, Row } from '../src/lib/types';

export function makeRow(overrides: Partial<Row>): Row {
  const base: Row = {
    uid: 'uid-0',
    name: 'row-0',
    namespace: 'default',
    createdAt: '2026-07-01T00:00:00Z',
    cells: [],
  };
  return { ...base, ...overrides };
}

export function makeDescriptor(overrides: Partial<ResourceDescriptor>): ResourceDescriptor {
  const base: ResourceDescriptor = {
    group: '',
    version: 'v1',
    resource: 'pods',
    kind: 'Pod',
    namespaced: true,
    category: 'Workloads',
  };
  return { ...base, ...overrides };
}

export function makeColumns(names: string[]): Column[] {
  return names.map((name) => ({ name }));
}

export function makeCategory(name: string, resources: ResourceDescriptor[]): Category {
  return { name, resources };
}
