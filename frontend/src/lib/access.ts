import type { Access, AccessQuery, BulkAccess, ObjectRef } from './types';
import { failure } from './object';
import { request } from './http';
import { parseAccess, parseBulkAccess } from './parse';

// The capabilities the server answers about. Each one stands for a button or a
// tab, not for a verb.
export type Capability =
  | 'edit'
  | 'delete'
  | 'scale'
  | 'restart'
  | 'cordon'
  | 'drain'
  | 'logs'
  | 'exec'
  | 'portForward'
  | 'reconcile';

export async function fetchAccess(query: string): Promise<Access> {
  const response = await request(`/api/access?${query}`);
  if (!response.ok) {
    throw await failure(response, `access request failed with status ${response.status}`);
  }
  return parseAccess(await response.json());
}

// fetchBulkAccess asks one capability of a whole selection at once, which is
// how a row that is refused can be told from the rows beside it.
export async function fetchBulkAccess(
  capability: Capability,
  refs: ObjectRef[],
): Promise<BulkAccess> {
  const query: AccessQuery = { capability, refs };
  const response = await request('/api/access', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(query),
  });
  if (!response.ok) {
    throw await failure(response, `access request failed with status ${response.status}`);
  }
  return parseBulkAccess(await response.json());
}

export function refusalsOf(access: Access): Partial<Record<Capability, string>> {
  const out: Partial<Record<Capability, string>> = {};
  for (const refusal of access.refused) {
    out[refusal.capability as Capability] = refusal.reason;
  }
  return out;
}
