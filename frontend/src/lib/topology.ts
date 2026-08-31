import type { Graph, ObjectRef } from './types';
import { request } from './http';
import { parseGraph } from './parse';
import { failure } from './object';

export interface TopologyQuery {
  namespace: string;
  expanded: string[];
  root: ObjectRef | null;
}

export function topologyParams(query: TopologyQuery): string {
  const params = new URLSearchParams();
  if (query.namespace !== '') {
    params.set('namespace', query.namespace);
  }
  if (query.expanded.length > 0) {
    params.set('expand', query.expanded.join(','));
  }
  if (query.root !== null) {
    params.set('rootGroup', query.root.group);
    params.set('rootVersion', query.root.version);
    params.set('rootResource', query.root.resource);
    params.set('rootNamespace', query.root.namespace);
    params.set('rootName', query.root.name);
  }
  const text = params.toString();
  if (text === '') {
    return '';
  }
  return `?${text}`;
}

export async function fetchTopology(query: TopologyQuery): Promise<Graph> {
  const response = await request(`/api/topology${topologyParams(query)}`);
  if (!response.ok) {
    throw await failure(response, `topology request failed with status ${response.status}`);
  }
  return parseGraph(await response.json());
}
