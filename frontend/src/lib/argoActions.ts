import type { ArgoActionResult, ObjectRef } from './types';
import { failure, refQuery } from './object';
import { groupOf } from './fluxActions';
import { parseArgoActionResult } from './parse';
import { request } from './http';

export type ArgoAction =
  'sync' | 'refresh' | 'hard-refresh' | 'terminate' | 'suspend' | 'resume' | 'rollback';

export interface ArgoResourceRef {
  group?: string;
  kind: string;
  name: string;
  namespace?: string;
}

export interface ArgoOptions {
  prune?: boolean;
  dryRun?: boolean;
  force?: boolean;
  replace?: boolean;
  serverSide?: boolean;
  applyOnly?: boolean;
  revision?: number;
  resources?: ArgoResourceRef[];
}

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
  options?: ArgoOptions,
): Promise<ArgoActionResult> {
  const params = new URLSearchParams(refQuery(ref));
  params.set('action', action);
  if (confirm !== undefined) {
    params.set('confirm', confirm);
  }
  const init: RequestInit = { method: 'POST' };
  if (options !== undefined) {
    init.body = JSON.stringify(options);
  }
  const response = await request(`/api/argocd/action?${params.toString()}`, init);
  if (!response.ok) {
    throw await failure(response, `${action} failed with status ${response.status}`);
  }
  return parseArgoActionResult(await response.json());
}
