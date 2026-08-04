import type { FluxResource, GraphNode, ObjectRef, ResourceDescriptor, Row } from './types';
import { useResourcesStore } from '../store/resources';

export interface Selection {
  ref: ObjectRef;
  row: Row | null;
}

interface Gvr {
  group: string;
  version: string;
  resource: string;
}

export function sameGvr(a: Gvr, b: Gvr): boolean {
  if (a.group !== b.group) {
    return false;
  }
  if (a.version !== b.version) {
    return false;
  }
  return a.resource === b.resource;
}

export function refFromRow(descriptor: ResourceDescriptor | null, row: Row): ObjectRef | null {
  if (descriptor === null) {
    return null;
  }
  return {
    group: descriptor.group,
    version: descriptor.version,
    resource: descriptor.resource,
    namespace: row.namespace,
    name: row.name,
  };
}

export function refFromNode(node: GraphNode): ObjectRef | null {
  if (node.resource === '') {
    return null;
  }
  return {
    group: node.group,
    version: node.version,
    resource: node.resource,
    namespace: node.namespace,
    name: node.name,
  };
}

export function refFromFlux(resource: FluxResource): ObjectRef {
  return {
    group: resource.group,
    version: resource.version,
    resource: resource.resource,
    namespace: resource.namespace,
    name: resource.name,
  };
}

export function useRowForRef(
  subId: string,
  descriptor: ResourceDescriptor | null,
  ref: ObjectRef | null,
): Row | null {
  return useResourcesStore((state) => {
    if (ref === null) {
      return null;
    }
    if (descriptor === null) {
      return null;
    }
    if (!sameGvr(descriptor, ref)) {
      return null;
    }
    const sub = state.subs.get(subId);
    if (sub === undefined) {
      return null;
    }
    for (const row of sub.rows.values()) {
      if (row.namespace !== ref.namespace) {
        continue;
      }
      if (row.name === ref.name) {
        return row;
      }
    }
    return null;
  });
}
