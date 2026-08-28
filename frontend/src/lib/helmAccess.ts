import type { Access } from './types';
import { failure } from './object';
import { request } from './http';
import { parseAccess } from './parse';

export type HelmCapability = 'install' | 'upgrade' | 'rollback' | 'uninstall';

export type HelmRefusals = Partial<Record<HelmCapability, string>>;

export async function fetchHelmAccess(namespace: string, name: string): Promise<Access> {
  const params = new URLSearchParams({ namespace });
  if (name !== '') {
    params.set('name', name);
  }
  const response = await request(`/api/helm/access?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `helm access request failed with status ${response.status}`);
  }
  return parseAccess(await response.json());
}

export function helmRefusalsOf(access: Access): HelmRefusals {
  const out: HelmRefusals = {};
  for (const refusal of access.refused) {
    out[refusal.capability as HelmCapability] = refusal.reason;
  }
  return out;
}
