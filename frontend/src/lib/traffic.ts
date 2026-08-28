import type { TrafficGraph, TrafficSupport } from './types';
import { request } from './http';
import { parseTrafficGraph, parseTrafficSupport } from './parse';

export async function fetchTrafficSupport(): Promise<TrafficSupport> {
  const response = await request('/api/traffic/support');
  if (!response.ok) {
    throw new Error(`traffic support request failed with status ${response.status}`);
  }
  return parseTrafficSupport(await response.json());
}

export async function fetchTrafficGraph(): Promise<TrafficGraph> {
  const response = await request('/api/traffic');
  if (!response.ok) {
    throw new Error(`traffic graph request failed with status ${response.status}`);
  }
  return parseTrafficGraph(await response.json());
}
