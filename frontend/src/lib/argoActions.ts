import type { ArgoActionResult, ObjectRef } from './types';
import { failure, refQuery } from './object';
import { groupOf } from './fluxActions';
import { parseArgoActionResult } from './parse';
import { request } from './http';

export type ArgoAction = 'sync' | 'refresh';

const ARGO_GROUP = 'argoproj.io';

const APPLICATION = 'Application';

export function isArgoApplication(apiVersion: string, kind: string): boolean {
  if (groupOf(apiVersion) !== ARGO_GROUP) {
    return false;
  }
  return kind === APPLICATION;
}

export async function runArgoAction(
  ref: ObjectRef,
  action: ArgoAction,
  confirm?: string,
): Promise<ArgoActionResult> {
  const params = new URLSearchParams(refQuery(ref));
  params.set('action', action);
  if (confirm !== undefined) {
    params.set('confirm', confirm);
  }
  const response = await request(`/api/argocd/action?${params.toString()}`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await failure(response, `${action} failed with status ${response.status}`);
  }
  return parseArgoActionResult(await response.json());
}
