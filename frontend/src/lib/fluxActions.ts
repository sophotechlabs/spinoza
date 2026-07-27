import type { ObjectRef } from './types';
import { failure, refQuery } from './object';

export type FluxAction = 'reconcile' | 'suspend' | 'resume';

const FLUX_GROUP_SUFFIX = '.toolkit.fluxcd.io';

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

export async function runFluxAction(ref: ObjectRef, action: FluxAction): Promise<void> {
  const response = await fetch(`/api/flux/action?${refQuery(ref)}&action=${action}`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw await failure(response, `${action} failed with status ${response.status}`);
  }
}
