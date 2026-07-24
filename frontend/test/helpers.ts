import type {
  Category,
  Column,
  FluxResource,
  GraphEdge,
  GraphNode,
  ResourceDescriptor,
  Row,
} from '../src/lib/types';

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

export function makeGraphNode(overrides: Partial<GraphNode>): GraphNode {
  const base: GraphNode = {
    id: 'node-0',
    kind: 'GitRepository',
    group: 'source.toolkit.fluxcd.io',
    name: 'flux-system',
    namespace: 'flux-system',
    status: 'Ready',
    category: 'source',
  };
  return { ...base, ...overrides };
}

export function makeFluxResource(overrides: Partial<FluxResource>): FluxResource {
  const base: FluxResource = {
    kind: 'Kustomization',
    name: 'apps',
    namespace: 'flux-system',
    ready: 'True',
    suspended: false,
    revision: 'main@sha1:abc',
    source: 'GitRepository/app-repo',
    message: '',
    createdAt: '2026-07-24T09:00:00Z',
  };
  return { ...base, ...overrides };
}

export function makeGraphEdge(overrides: Partial<GraphEdge>): GraphEdge {
  const base: GraphEdge = {
    from: 'node-0',
    to: 'node-1',
    kind: 'source',
  };
  return { ...base, ...overrides };
}
