import { expect, vi } from 'vitest';
import type {
  Category,
  Column,
  FluxResource,
  GraphEdge,
  GraphNode,
  ResourceDescriptor,
  Row,
} from '../src/lib/types';

export function anySignal(): AbortSignal {
  return expect.any(AbortSignal) as AbortSignal;
}

export function parentOf(node: HTMLElement): HTMLElement {
  const parent = node.parentElement;
  if (parent === null) {
    throw new Error('expected the element to have a parent');
  }
  return parent;
}

export function rejectsWith(value: unknown): () => Promise<never> {
  return vi.fn<() => Promise<never>>().mockRejectedValue(value);
}

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
    version: 'v1',
    resource: 'gitrepositories',
    name: 'flux-system',
    namespace: 'flux-system',
    status: 'Ready',
    ready: 'True',
    category: 'source',
  };
  return { ...base, ...overrides };
}

export function makeFluxResource(overrides: Partial<FluxResource>): FluxResource {
  const base: FluxResource = {
    kind: 'Kustomization',
    group: 'kustomize.toolkit.fluxcd.io',
    version: 'v1',
    resource: 'kustomizations',
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

type MediaListener = (event: MediaQueryListEvent) => void;

const mediaListeners = new Set<MediaListener>();
let systemDark = false;

export function installMatchMedia(): void {
  systemDark = false;
  mediaListeners.clear();
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: (query: string): MediaQueryList =>
      ({
        get matches() {
          return systemDark;
        },
        media: query,
        onchange: null,
        addListener: () => undefined,
        removeListener: () => undefined,
        addEventListener: (_type: string, listener: MediaListener) => {
          mediaListeners.add(listener);
        },
        removeEventListener: (_type: string, listener: MediaListener) => {
          mediaListeners.delete(listener);
        },
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList,
  });
}

export function setSystemDark(matches: boolean): void {
  systemDark = matches;
}

export function emitSystemDark(matches: boolean): void {
  systemDark = matches;
  const event = { matches } as MediaQueryListEvent;
  for (const listener of mediaListeners) {
    listener(event);
  }
}
