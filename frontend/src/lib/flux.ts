import type { FluxDashboard } from './types';

export async function fetchFlux(): Promise<FluxDashboard> {
  const response = await fetch('/api/flux');
  if (!response.ok) {
    throw new Error(`flux request failed with status ${response.status}`);
  }
  const data = (await response.json()) as FluxDashboard;
  return data;
}
