import type { Condition, FluxActionResult, ObjectDetail, ObjectRef } from './types';
import { failure, fetchObject, refQuery } from './object';
import { parseFluxActionResult } from './parse';
import { request } from './http';

export type FluxAction = 'reconcile' | 'reconcile-with-source' | 'suspend' | 'resume';

const FLUX_GROUP_SUFFIX = '.toolkit.fluxcd.io';

export const RECONCILE_POLL_MS = 1000;
export const RECONCILE_TIMEOUT_MS = 90000;

export function groupOf(apiVersion: string): string {
  const parts = apiVersion.split('/');
  if (parts.length === 2) {
    return parts[0];
  }
  return '';
}

export function isFluxObject(apiVersion: string): boolean {
  return groupOf(apiVersion).endsWith(FLUX_GROUP_SUFFIX);
}

export async function runFluxAction(ref: ObjectRef, action: FluxAction): Promise<FluxActionResult> {
  const response = await request(`/api/flux/action?${refQuery(ref)}&action=${action}`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await failure(response, `${action} failed with status ${response.status}`);
  }
  return parseFluxActionResult(await response.json());
}

export function readyCondition(detail: ObjectDetail): Condition | null {
  const conditions = detail.conditions ?? [];
  for (const condition of conditions) {
    if (condition.type === 'Ready') {
      return condition;
    }
  }
  return null;
}

export type ReconcileState = 'requested' | 'running' | 'succeeded' | 'failed';

export interface ReconcileProgress {
  state: ReconcileState;
  message: string;
}

export function reconcileProgress(detail: ObjectDetail, requestedAt: string): ReconcileProgress {
  if (detail.flux?.handledAt !== requestedAt) {
    return { state: 'requested', message: 'Reconciliation requested' };
  }
  const ready = readyCondition(detail);
  if (ready === null) {
    return { state: 'running', message: 'Reconciliation running' };
  }
  if (ready.status === 'True') {
    return { state: 'succeeded', message: readyMessage('Reconciliation succeeded', ready) };
  }
  if (ready.status === 'False') {
    return { state: 'failed', message: readyMessage('Reconciliation failed', ready) };
  }
  return { state: 'running', message: readyMessage('Reconciliation running', ready) };
}

function readyMessage(prefix: string, ready: Condition): string {
  if (ready.message !== undefined && ready.message !== '') {
    return `${prefix}: ${ready.message}`;
  }
  return `${prefix}.`;
}

export function isSettled(state: ReconcileState): boolean {
  if (state === 'succeeded') {
    return true;
  }
  return state === 'failed';
}

export async function pollReconcile(
  ref: ObjectRef,
  requestedAt: string,
  report: (progress: ReconcileProgress) => boolean,
): Promise<void> {
  const deadline = Date.now() + RECONCILE_TIMEOUT_MS;
  while (Date.now() < deadline) {
    await sleep(RECONCILE_POLL_MS);
    let detail: ObjectDetail;
    try {
      detail = await fetchObject(ref);
    } catch {
      continue;
    }
    const progress = reconcileProgress(detail, requestedAt);
    const wanted = report(progress);
    if (!wanted) {
      return;
    }
    if (isSettled(progress.state)) {
      return;
    }
  }
  report({ state: 'running', message: 'Reconciliation is still running.' });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
