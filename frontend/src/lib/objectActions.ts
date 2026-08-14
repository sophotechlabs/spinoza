import type { ActionResult, ObjectDetail, ObjectRef } from './types';
import { failure, refQuery } from './object';
import { request, SLOW_REQUEST_TIMEOUT_MS } from './http';
import { parseActionResult } from './parse';

export type ObjectAction = 'scale' | 'restart' | 'cordon' | 'uncordon' | 'drain';

const SCALABLE = [
  'apps/deployments',
  'apps/statefulsets',
  'apps/replicasets',
  '/replicationcontrollers',
];

const RESTARTABLE = ['apps/deployments', 'apps/statefulsets', 'apps/daemonsets'];

function key(ref: ObjectRef): string {
  return `${ref.group}/${ref.resource}`;
}

export function canScale(ref: ObjectRef): boolean {
  return SCALABLE.includes(key(ref));
}

export function canRestart(ref: ObjectRef): boolean {
  return RESTARTABLE.includes(key(ref));
}

export function isNode(ref: ObjectRef): boolean {
  return key(ref) === '/nodes';
}

export function hasActions(ref: ObjectRef): boolean {
  if (canScale(ref)) {
    return true;
  }
  if (canRestart(ref)) {
    return true;
  }
  return isNode(ref);
}

export interface ActionOptions {
  replicas?: number;
  force?: boolean;
  dryRun?: boolean;
  confirm?: string;
}

function query(ref: ObjectRef, action: ObjectAction, options: ActionOptions): string {
  const params = new URLSearchParams(refQuery(ref));
  params.set('action', action);
  if (options.replicas !== undefined) {
    params.set('replicas', String(options.replicas));
  }
  if (options.force === true) {
    params.set('force', 'true');
  }
  if (options.dryRun === true) {
    params.set('dryRun', 'true');
  }
  if (options.confirm !== undefined) {
    params.set('confirm', options.confirm);
  }
  return params.toString();
}

export async function runAction(
  ref: ObjectRef,
  action: ObjectAction,
  options: ActionOptions = {},
): Promise<ActionResult> {
  const response = await request(`/api/action?${query(ref, action, options)}`, {
    method: 'POST',
    timeoutMs: SLOW_REQUEST_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `${action} failed with status ${response.status}`);
  }
  return parseActionResult(await response.json());
}

export function replicasOf(detail: ObjectDetail | null): number {
  const workload = detail?.workload;
  if (workload === undefined) {
    return 0;
  }
  return workload.replicas;
}

export function isCordoned(detail: ObjectDetail | null): boolean {
  const node = detail?.node;
  if (node === undefined) {
    return false;
  }
  return !node.schedulable;
}

export function countBy(result: ActionResult, outcome: string): number {
  const pods = result.pods ?? [];
  return pods.filter((pod) => pod.outcome === outcome).length;
}
