import type { TrafficGraph, TrafficSupport } from './types';
import { request } from './http';
import { parseTrafficGraph } from './parse';
import { fetchCapabilities } from './capabilities';

export async function fetchTrafficSupport(): Promise<TrafficSupport> {
  return (await fetchCapabilities()).traffic;
}

export async function fetchTrafficGraph(): Promise<TrafficGraph> {
  const response = await request('/api/traffic');
  if (!response.ok) {
    throw new Error(`traffic graph request failed with status ${response.status}`);
  }
  return parseTrafficGraph(await response.json());
}
