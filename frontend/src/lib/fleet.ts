import type {
  FleetGitops,
  FleetImages,
  FleetInventory,
  FleetOverview,
  HelmReleases,
  NodeSummary,
  PodSummary,
} from './types';
import { request } from './http';
import { failure } from './object';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';

const FLEET_POLL_MS = 15000;

async function readFleet<T>(path: string, what: string): Promise<T> {
  const response = await request(path);
  if (!response.ok) {
    throw await failure(response, `${what} failed with status ${response.status}`);
  }
  return (await response.json()) as T;
}

export async function fetchFleetOverview(): Promise<FleetOverview> {
  const body = await readFleet<Partial<FleetOverview>>('/api/overview/fleet', 'the fleet overview');
  return {
    clusters: body.clusters ?? [],
    nodes: body.nodes ?? emptyNodes(),
    pods: body.pods ?? emptyPods(),
    error: body.error,
  };
}

export async function fetchFleetInventory(): Promise<FleetInventory> {
  const body = await readFleet<Partial<FleetInventory>>(
    '/api/resources/fleet',
    'the fleet inventory',
  );
  return { kinds: body.kinds ?? [], error: body.error };
}

export async function fetchFleetImages(): Promise<FleetImages> {
  const body = await readFleet<Partial<FleetImages>>('/api/images/fleet', 'the fleet images');
  return { images: body.images ?? [], error: body.error };
}

export async function fetchFleetReleases(): Promise<HelmReleases> {
  const body = await readFleet<Partial<HelmReleases>>('/api/helm/fleet', 'the fleet releases');
  return { releases: body.releases ?? [], error: body.error };
}

export async function fetchFleetGitops(): Promise<FleetGitops> {
  const body = await readFleet<Partial<FleetGitops>>('/api/gitops/fleet', 'the fleet delivery');
  return { apps: body.apps ?? [], error: body.error };
}

export function useFleetReleases(enabled = true): Polled<HelmReleases> {
  return usePoll(fetchFleetReleases, {
    intervalMs: FLEET_POLL_MS,
    enabled,
    fallback: 'the fleet releases failed',
  });
}

export function useFleetGitops(enabled = true): Polled<FleetGitops> {
  return usePoll(fetchFleetGitops, {
    intervalMs: FLEET_POLL_MS,
    enabled,
    fallback: 'the fleet delivery failed',
  });
}

export function useFleetOverview(enabled = true): Polled<FleetOverview> {
  return usePoll(fetchFleetOverview, {
    intervalMs: FLEET_POLL_MS,
    enabled,
    fallback: 'the fleet overview failed',
  });
}

export function useFleetInventory(enabled = true): Polled<FleetInventory> {
  return usePoll(fetchFleetInventory, {
    intervalMs: FLEET_POLL_MS,
    enabled,
    fallback: 'the fleet inventory failed',
  });
}

export function useFleetImages(enabled = true): Polled<FleetImages> {
  return usePoll(fetchFleetImages, {
    intervalMs: FLEET_POLL_MS,
    enabled,
    fallback: 'the fleet images failed',
  });
}

function emptyNodes() {
  return {
    total: 0,
    ready: 0,
    unschedulable: 0,
    cpuAllocatableMilli: 0,
    cpuUsedMilli: 0,
    memAllocatableMi: 0,
    memUsedMi: 0,
    usageKnown: false,
  };
}

function emptyPods() {
  return { total: 0, running: 0, pending: 0, failed: 0, succeeded: 0, known: false, capped: [] };
}

export function nodesLabel(nodes: NodeSummary): string {
  return `${String(nodes.ready)}/${String(nodes.total)}`;
}

export function podsLabel(pods: PodSummary): string {
  if (!pods.known) {
    return '—';
  }
  return `${String(pods.running)}/${String(pods.total)}`;
}

export function shortKey(key: string): string {
  const parts = key.split('/');
  return parts[parts.length - 1];
}

export function spreadLabel(spread: number | undefined, open: number): string {
  if (spread === undefined || open < 2) {
    return '';
  }
  if (spread >= open) {
    return 'everywhere';
  }
  return `${String(spread)} of ${String(open)}`;
}

export function skewLabel(tags: string[] | undefined): string {
  if (tags === undefined || tags.length < 2) {
    return '';
  }
  return tags.join(' · ');
}
