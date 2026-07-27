import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  applyObject,
  deleteObject,
  fetchEvents,
  fetchObject,
  refQuery,
  sameRef,
} from '../../src/lib/object';
import type { K8sEvent, ObjectDetail, ObjectRef } from '../../src/lib/types';

const ref: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'flux-system',
  name: 'web',
};

const detail: ObjectDetail = {
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  name: 'web',
  namespace: 'flux-system',
  uid: 'uid-web',
  createdAt: '2026-07-27T09:00:00Z',
  yaml: 'kind: Deployment\n',
};

function stubFetch(impl: (url: string, init?: RequestInit) => unknown): ReturnType<typeof vi.fn> {
  const mock = vi.fn().mockImplementation(impl);
  vi.stubGlobal('fetch', mock);
  return mock;
}

function ok(payload: unknown) {
  return Promise.resolve({ ok: true, json: () => Promise.resolve(payload) });
}

function failWith(status: number, payload: unknown) {
  return Promise.resolve({ ok: false, status, json: () => Promise.resolve(payload) });
}

function failWithoutBody(status: number) {
  return Promise.resolve({
    ok: false,
    status,
    json: () => Promise.reject(new Error('not json')),
  });
}

describe('object client', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('serialises a ref into query parameters', () => {
    expect(refQuery(ref)).toBe(
      'group=apps&version=v1&resource=deployments&namespace=flux-system&name=web',
    );
  });

  it('compares refs by value', () => {
    expect(sameRef(ref, { ...ref })).toBe(true);
    expect(sameRef(ref, { ...ref, name: 'other' })).toBe(false);
    expect(sameRef(null, null)).toBe(true);
    expect(sameRef(ref, null)).toBe(false);
    expect(sameRef(null, ref)).toBe(false);
  });

  it('fetches an object detail', async () => {
    const mock = stubFetch(() => ok(detail));

    await expect(fetchObject(ref)).resolves.toEqual(detail);
    expect(mock).toHaveBeenCalledWith(`/api/object?${refQuery(ref)}`);
  });

  it('surfaces the server message on a failed fetch', async () => {
    stubFetch(() => failWith(404, { message: 'deployments "web" not found' }));

    await expect(fetchObject(ref)).rejects.toThrow('deployments "web" not found');
  });

  it('falls back to a status message when the error body is not json', async () => {
    stubFetch(() => failWithoutBody(500));

    await expect(fetchObject(ref)).rejects.toThrow('object request failed with status 500');
  });

  it('falls back to a status message when the error body has no message', async () => {
    stubFetch(() => failWith(500, {}));

    await expect(fetchObject(ref)).rejects.toThrow('object request failed with status 500');
  });

  it('falls back when the error message is empty', async () => {
    stubFetch(() => failWith(500, { message: '' }));

    await expect(fetchObject(ref)).rejects.toThrow('object request failed with status 500');
  });

  it('applies a yaml document', async () => {
    const mock = stubFetch(() => ok(detail));

    await expect(applyObject(ref, 'kind: Deployment\n')).resolves.toEqual(detail);
    expect(mock).toHaveBeenCalledWith(`/api/object?${refQuery(ref)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/yaml' },
      body: 'kind: Deployment\n',
    });
  });

  it('reports an apply failure', async () => {
    stubFetch(() => failWith(409, { message: 'the object has been modified' }));

    await expect(applyObject(ref, 'x')).rejects.toThrow('the object has been modified');
  });

  it('reports an apply failure without a body', async () => {
    stubFetch(() => failWithoutBody(500));

    await expect(applyObject(ref, 'x')).rejects.toThrow('apply failed with status 500');
  });

  it('deletes an object', async () => {
    const mock = stubFetch(() => Promise.resolve({ ok: true, json: () => Promise.resolve({}) }));

    await expect(deleteObject(ref)).resolves.toBeUndefined();
    expect(mock).toHaveBeenCalledWith(`/api/object?${refQuery(ref)}`, { method: 'DELETE' });
  });

  it('reports a delete failure', async () => {
    stubFetch(() => failWithoutBody(403));

    await expect(deleteObject(ref)).rejects.toThrow('delete failed with status 403');
  });

  it('fetches events for an object', async () => {
    const events: K8sEvent[] = [
      {
        type: 'Warning',
        reason: 'BackOff',
        message: 'restarting',
        source: 'kubelet',
        count: 3,
        firstSeen: '2026-07-27T09:00:00Z',
        lastSeen: '2026-07-27T09:30:00Z',
      },
    ];
    const mock = stubFetch(() => ok(events));

    await expect(fetchEvents('flux-system', 'uid-web')).resolves.toEqual(events);
    expect(mock).toHaveBeenCalledWith('/api/events?namespace=flux-system&uid=uid-web');
  });

  it('reports an events failure', async () => {
    stubFetch(() => failWithoutBody(500));

    await expect(fetchEvents('flux-system', 'uid-web')).rejects.toThrow(
      'events request failed with status 500',
    );
  });
});
