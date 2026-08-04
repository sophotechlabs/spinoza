import type { FluxDashboard } from './types';
import { request } from './http';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const FLUX_POLL_MS = 5000;

export async function fetchFlux(): Promise<FluxDashboard> {
  const response = await request('/api/flux');
  if (!response.ok) {
    throw new Error(`flux request failed with status ${response.status}`);
  }
  const data = (await response.json()) as FluxDashboard;
  return data;
}

export function useFlux(): Polled<FluxDashboard> {
  return usePoll(fetchFlux, { intervalMs: FLUX_POLL_MS, fallback: 'flux request failed' });
}
