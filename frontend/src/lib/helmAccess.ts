import type { Access } from './types';
import { failure } from './object';
import { request } from './http';
import { parseAccess } from './parse';

// The helm buttons the cluster answers about. A release is not a kubernetes
// object, so these are asked about on their own rather than through an object
// reference.
export type HelmCapability = 'install' | 'upgrade' | 'rollback' | 'uninstall';

export type HelmRefusals = Partial<Record<HelmCapability, string>>;

// fetchHelmAccess asks what the cluster would refuse in one namespace. A release
// name is optional: without one the question is about installing something that
// is not there yet.
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
