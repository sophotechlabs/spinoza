import type { Graph } from './types';
import { request } from './http';
import { parseGraph } from './parse';
import { failure } from './object';

export async function fetchGraph(): Promise<Graph> {
  const response = await request('/api/gitops/graph');
  if (!response.ok) {
    throw await failure(response, `gitops graph request failed with status ${response.status}`);
  }
  return parseGraph(await response.json());
}
