import type {
  ContextList,
  ContextRef,
  FilePicker,
  KubeContext,
  Kubeconfig,
  PickedFile,
} from './types';
import { failure } from './object';
import { SLOW_REQUEST_TIMEOUT_MS, request } from './http';

export interface ContextEntry {
  cluster: string;
  kubeconfig: string;
  name: string;
  value: string;
}

export interface ContextGroup {
  entries: ContextEntry[];
  error?: string;
  label: string;
  path: string;
}

interface WireKubeconfig {
  contexts?: KubeContext[];
  error?: string;
  label?: string;
  path?: string;
  removable?: boolean;
}

interface WireContexts {
  current?: ContextRef;
  error?: string;
  kubeconfigs?: WireKubeconfig[];
}

interface WirePicker {
  available?: boolean;
  reason?: string;
}

function normalizeKubeconfig(entry: WireKubeconfig): Kubeconfig {
  return {
    contexts: entry.contexts ?? [],
    error: entry.error,
    label: entry.label ?? '',
    path: entry.path ?? '',
    removable: entry.removable ?? false,
  };
}

function normalize(body: WireContexts): ContextList {
  return {
    current: body.current ?? { kubeconfig: '', name: '' },
    error: body.error,
    kubeconfigs: (body.kubeconfigs ?? []).map(normalizeKubeconfig),
  };
}

function describe(entry: KubeContext): string {
  if (entry.namespace === undefined || entry.namespace === '') {
    return `cluster ${entry.cluster}`;
  }
  return `cluster ${entry.cluster} · namespace ${entry.namespace}`;
}

export function contextGroups(list: ContextList): ContextGroup[] {
  return list.kubeconfigs.map((kubeconfig, group) => ({
    entries: kubeconfig.contexts.map((entry, index) => ({
      cluster: describe(entry),
      kubeconfig: kubeconfig.path,
      name: entry.name,
      value: `${group}.${index}`,
    })),
    error: kubeconfig.error,
    label: kubeconfig.label,
    path: kubeconfig.path,
  }));
}

export function everyContext(groups: ContextGroup[]): ContextEntry[] {
  return groups.flatMap((group) => group.entries);
}

export function sameContext(entry: ContextEntry, current: ContextRef): boolean {
  return entry.kubeconfig === current.kubeconfig && entry.name === current.name;
}

export function entryFor(groups: ContextGroup[], value: string): ContextEntry | undefined {
  return everyContext(groups).find((entry) => entry.value === value);
}

export async function fetchContexts(): Promise<ContextList> {
  const response = await request('/api/contexts');
  if (!response.ok) {
    throw await failure(response, `contexts request failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}

export async function switchContext(entry: ContextEntry): Promise<ContextList> {
  const params = new URLSearchParams({ kubeconfig: entry.kubeconfig, name: entry.name });
  const response = await request(`/api/contexts?${params.toString()}`, { method: 'POST' });
  if (!response.ok) {
    throw await failure(response, `switching context failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}

export async function addKubeconfig(path: string): Promise<ContextList> {
  const params = new URLSearchParams({ path });
  const response = await request(`/api/kubeconfigs?${params.toString()}`, { method: 'POST' });
  if (!response.ok) {
    throw await failure(response, `adding the kubeconfig failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}

export async function removeKubeconfig(path: string): Promise<ContextList> {
  const params = new URLSearchParams({ path });
  const response = await request(`/api/kubeconfigs?${params.toString()}`, { method: 'DELETE' });
  if (!response.ok) {
    throw await failure(response, `removing the kubeconfig failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}

export async function fetchFilePicker(): Promise<FilePicker> {
  const response = await request('/api/kubeconfigs/picker');
  if (!response.ok) {
    throw await failure(response, `the file dialog is not available: ${response.status}`);
  }
  const body = (await response.json()) as WirePicker;
  return { available: body.available ?? false, reason: body.reason };
}

export async function pickKubeconfigFile(): Promise<string> {
  const response = await request('/api/kubeconfigs/picker', {
    method: 'POST',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the file dialog did not open: ${response.status}`);
  }
  const body = (await response.json()) as Partial<PickedFile>;
  return body.path ?? '';
}
