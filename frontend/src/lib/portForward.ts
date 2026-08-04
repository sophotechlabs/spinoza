import type { ObjectRef, PortForward } from './types';
import { failure } from './object';
import { request } from './http';
import { usePoll } from './usePoll';
import type { Polled } from './usePoll';
import { useForwardsStore } from '../store/forwards';

const FORWARDS_POLL_MS = 5000;

export function forwardKind(apiVersion: string, kind: string): string | null {
  if (apiVersion !== 'v1') {
    return null;
  }
  if (kind === 'Pod') {
    return 'Pod';
  }
  if (kind === 'Service') {
    return 'Service';
  }
  return null;
}

export async function listForwards(): Promise<PortForward[]> {
  const response = await request('/api/portforward');
  if (!response.ok) {
    throw await failure(response, `forward list failed with status ${response.status}`);
  }
  return (await response.json()) as PortForward[];
}

export async function startForward(
  kind: string,
  ref: ObjectRef,
  port: number,
): Promise<PortForward> {
  const params = new URLSearchParams({
    kind,
    namespace: ref.namespace,
    name: ref.name,
    port: String(port),
  });
  const response = await request(`/api/portforward?${params.toString()}`, { method: 'POST' });
  if (!response.ok) {
    throw await failure(response, `port forward failed with status ${response.status}`);
  }
  return (await response.json()) as PortForward;
}

export async function stopForward(id: string): Promise<void> {
  const params = new URLSearchParams({ id });
  const response = await request(`/api/portforward?${params.toString()}`, { method: 'DELETE' });
  if (!response.ok) {
    throw await failure(response, `stopping the forward failed with status ${response.status}`);
  }
}

export async function loadForwards(): Promise<PortForward[]> {
  const forwards = await listForwards();
  useForwardsStore.getState().setForwards(forwards);
  return forwards;
}

export async function refreshForwards(): Promise<void> {
  try {
    await loadForwards();
  } catch {
    return;
  }
}

export function clearForwards(): void {
  useForwardsStore.getState().setForwards([]);
}

export function useForwardPolling(enabled: boolean): Polled<PortForward[]> {
  return usePoll(loadForwards, {
    intervalMs: FORWARDS_POLL_MS,
    enabled,
    fallback: 'forward list failed',
  });
}
