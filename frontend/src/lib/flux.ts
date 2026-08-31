import type { FluxDashboard, FluxOverview } from './types';
import { request } from './http';
import { parseFluxDashboard, parseFluxOverview } from './parse';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { failure } from './object';

const FLUX_POLL_MS = 5000;

export async function fetchFlux(): Promise<FluxDashboard> {
  const response = await request('/api/flux');
  if (!response.ok) {
    throw await failure(response, `flux request failed with status ${response.status}`);
  }
  return parseFluxDashboard(await response.json());
}

export function useFlux(): Polled<FluxDashboard> {
  return usePoll(fetchFlux, { intervalMs: FLUX_POLL_MS, fallback: 'flux request failed' });
}

const OVERVIEW_POLL_MS = 10000;

export async function fetchFluxOverview(): Promise<FluxOverview> {
  const response = await request('/api/flux/overview');
  if (!response.ok) {
    throw await failure(
      response,
      `the flux overview request failed with status ${response.status}`,
    );
  }
  return parseFluxOverview(await response.json());
}

export function useFluxOverview(): Polled<FluxOverview> {
  return usePoll(fetchFluxOverview, {
    intervalMs: OVERVIEW_POLL_MS,
    fallback: 'the flux overview request failed',
  });
}
