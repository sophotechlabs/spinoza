import type { FluxResource, GraphNode, ObjectRef, ResourceDescriptor, Row } from './types';

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
