import type { ClusterList, OpenCluster, RememberedCluster } from './types';
import { failure } from './object';
import { SLOW_REQUEST_TIMEOUT_MS, request } from './http';
import { adoptClusters } from '../store/clusters';

interface WireOpenCluster {
  id?: string;
  context?: string;
  kubeconfig?: string;
  active?: boolean;
  color?: number;
  protection?: string;
  reachable?: boolean;
  reason?: string;
}

interface WireRemembered {
  id?: string;
  context?: string;
  kubeconfig?: string;
}

interface WireClusters {
  clusters?: WireOpenCluster[];
  remembered?: WireRemembered[];
}

function openClusterOf(entry: WireOpenCluster): OpenCluster {
  return {
    id: entry.id ?? '',
    context: entry.context ?? '',
    kubeconfig: entry.kubeconfig,
    active: entry.active ?? false,
    color: entry.color ?? 1,
    protection: entry.protection ?? 'unknown',
    reachable: entry.reachable ?? true,
    reason: entry.reason,
  };
}

function rememberedOf(entry: WireRemembered): RememberedCluster {
  return {
    id: entry.id ?? '',
    context: entry.context ?? '',
    kubeconfig: entry.kubeconfig,
  };
}

export function parseClusters(body: unknown): ClusterList {
  const wire = (body ?? {}) as WireClusters;
  return {
    clusters: (wire.clusters ?? []).map(openClusterOf),
    remembered: (wire.remembered ?? []).map(rememberedOf),
  };
}

async function clustersFrom(response: Response, what: string): Promise<ClusterList> {
  if (!response.ok) {
    throw await failure(response, `${what} failed with status ${response.status}`);
  }
  const list = parseClusters(await response.json());
  adoptClusters(list);
  return list;
}

export async function fetchClusters(): Promise<ClusterList> {
  return clustersFrom(await request('/api/clusters'), 'the cluster list');
}

export async function openCluster(kubeconfig: string, name: string): Promise<ClusterList> {
  const params = new URLSearchParams({ kubeconfig, name });
  const response = await request(`/api/clusters?${params.toString()}`, {
    method: 'POST',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  return clustersFrom(response, `opening ${name}`);
}

export async function activateCluster(id: string): Promise<ClusterList> {
  const params = new URLSearchParams({ cluster: id });
  const response = await request(`/api/clusters/active?${params.toString()}`, { method: 'POST' });
  return clustersFrom(response, 'switching cluster');
}

export async function closeCluster(id: string): Promise<ClusterList> {
  const params = new URLSearchParams({ cluster: id });
  const response = await request(`/api/clusters?${params.toString()}`, {
    method: 'DELETE',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  return clustersFrom(response, 'closing the cluster');
}

export async function recolorCluster(id: string, color: number): Promise<ClusterList> {
  const params = new URLSearchParams({ cluster: id, color: String(color) });
  const response = await request(`/api/clusters/color?${params.toString()}`, { method: 'POST' });
  return clustersFrom(response, 'changing the colour');
}

export function stillToOpen(list: ClusterList): RememberedCluster[] {
  return list.remembered.filter((one) => !list.clusters.some((open) => open.id === one.id));
}

export function clusterFailure(err: unknown, fallback: string): string {
  if (err instanceof Error) {
    return err.message;
  }
  return fallback;
}
