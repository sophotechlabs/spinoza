import type { ObjectRef, Revisions, RevisionDiff } from './types';
import { request } from './http';
import { failure, refQuery } from './object';

export const REVERTIBLE_KINDS = ['Deployment', 'StatefulSet', 'DaemonSet'];

export async function fetchRevisions(ref: ObjectRef): Promise<Revisions> {
  const response = await request(`/api/rollout?${refQuery(ref)}`);
  if (!response.ok) {
    throw await failure(response, `revisions request failed with status ${response.status}`);
  }
  return (await response.json()) as Revisions;
}

export async function fetchRevisionDiff(
  ref: ObjectRef,
  from: number,
  to: number,
): Promise<RevisionDiff> {
  const params = new URLSearchParams({ from: String(from), to: String(to) });
  const response = await request(`/api/rollout/diff?${refQuery(ref)}&${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `diff request failed with status ${response.status}`);
  }
  return (await response.json()) as RevisionDiff;
}
