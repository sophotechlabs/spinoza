import type { Access, ObjectRef } from './types';
import { failure, refQuery } from './object';
import { request } from './http';
import { parseAccess } from './parse';

// The capabilities the server answers about. Each one stands for a button or a
// tab, not for a verb.
export type Capability =
  'edit' | 'delete' | 'scale' | 'restart' | 'cordon' | 'drain' | 'logs' | 'exec' | 'portForward';

export function accessQuery(ref: ObjectRef): string {
  return refQuery(ref);
}

export async function fetchAccess(query: string): Promise<Access> {
  const response = await request(`/api/access?${query}`);
  if (!response.ok) {
    throw await failure(response, `access request failed with status ${response.status}`);
  }
  return parseAccess(await response.json());
}

export function refusalsOf(access: Access): Partial<Record<Capability, string>> {
  const out: Partial<Record<Capability, string>> = {};
  for (const refusal of access.refused) {
    out[refusal.capability as Capability] = refusal.reason;
  }
  return out;
}
