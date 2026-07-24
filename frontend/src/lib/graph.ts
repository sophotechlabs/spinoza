import type { Graph } from './types';

export async function fetchGraph(): Promise<Graph> {
  const response = await fetch('/api/gitops/graph');
  if (!response.ok) {
    throw new Error(`gitops graph request failed with status ${response.status}`);
  }
  const data = (await response.json()) as Graph;
  return data;
}
