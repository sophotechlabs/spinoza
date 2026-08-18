import type { Failure, K8sEvent, ObjectDetail, ObjectRef } from './types';
import { request } from './http';
import { parseEvents, parseObjectDetail } from './parse';

export function refQuery(ref: ObjectRef): string {
  const params = new URLSearchParams({
    group: ref.group,
    version: ref.version,
    resource: ref.resource,
    namespace: ref.namespace,
    name: ref.name,
  });
  return params.toString();
}

export function sameRef(a: ObjectRef | null, b: ObjectRef | null): boolean {
  if (a === null || b === null) {
    return a === b;
  }
  return refQuery(a) === refQuery(b);
}

export async function failure(response: Response, fallback: string): Promise<Error> {
  try {
    const body = (await response.json()) as Partial<Failure>;
    if (typeof body.message === 'string' && body.message !== '') {
      return new Error(body.message);
    }
  } catch {
    return new Error(fallback);
  }
  return new Error(fallback);
}

export async function fetchObject(ref: ObjectRef): Promise<ObjectDetail> {
  const response = await request(`/api/object?${refQuery(ref)}`);
  if (!response.ok) {
    throw await failure(response, `object request failed with status ${response.status}`);
  }
  return parseObjectDetail(await response.json());
}

export async function applyObject(ref: ObjectRef, doc: string): Promise<ObjectDetail> {
  const response = await request(`/api/object?${refQuery(ref)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/yaml' },
    body: doc,
  });
  if (!response.ok) {
    throw await failure(response, `apply failed with status ${response.status}`);
  }
  return parseObjectDetail(await response.json());
}

export async function deleteObject(ref: ObjectRef, confirm?: string): Promise<void> {
  const params = new URLSearchParams(refQuery(ref));
  if (confirm !== undefined) {
    params.set('confirm', confirm);
  }
  const response = await request(`/api/object?${params.toString()}`, { method: 'DELETE' });
  if (!response.ok) {
    throw await failure(response, `delete failed with status ${response.status}`);
  }
}

export async function fetchEvents(namespace: string, uid: string): Promise<K8sEvent[]> {
  const params = new URLSearchParams({ namespace, uid });
  const response = await request(`/api/events?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `events request failed with status ${response.status}`);
  }
  return parseEvents(await response.json());
}
