import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchAccess, fetchBulkAccess, refusalsOf } from '../../src/lib/access';
import { refQuery } from '../../src/lib/object';
import type { ObjectRef } from '../../src/lib/types';
import { anySignal } from '../helpers';

const ref: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'kube-system',
  name: 'calico-node-2cv49',
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('asking the cluster what it would refuse', () => {
  it('asks about the object that is selected', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ refused: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await fetchAccess(refQuery(ref));

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/access?group=&version=v1&resource=pods&namespace=kube-system&name=calico-node-2cv49',
      { signal: anySignal() },
    );
  });

  it('reads the refusals and their reasons', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            refused: [
              { capability: 'logs', reason: 'requires container.pods.getLogs' },
              { capability: 'exec', reason: 'requires container.pods.exec' },
            ],
          }),
      }),
    );

    const access = await fetchAccess(refQuery(ref));

    expect(refusalsOf(access)).toEqual({
      logs: 'requires container.pods.getLogs',
      exec: 'requires container.pods.exec',
    });
  });

  it('reads an empty answer as nothing refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const access = await fetchAccess(refQuery(ref));

    expect(refusalsOf(access)).toEqual({});
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'the cluster went away' }),
      }),
    );

    await expect(fetchAccess(refQuery(ref))).rejects.toThrow('the cluster went away');
  });

  it('falls back to a status message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 502,
        json: () => Promise.reject(new Error('not json')),
      }),
    );

    await expect(fetchAccess(refQuery(ref))).rejects.toThrow(
      'access request failed with status 502',
    );
  });
});

describe('asking the cluster about a whole selection', () => {
  const rows: ObjectRef[] = [ref, { ...ref, name: 'calico-node-8xk21' }];

  it('puts one question naming every row', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ refused: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await fetchBulkAccess('delete', rows);

    expect(fetchMock).toHaveBeenCalledWith('/api/access', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ capability: 'delete', refs: rows }),
      signal: anySignal(),
    });
  });

  it('reads which rows were refused and why', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ refused: [{ at: 1, reason: 'no deleting that one' }] }),
      }),
    );

    const answer = await fetchBulkAccess('delete', rows);

    expect(answer.refused).toEqual([{ at: 1, reason: 'no deleting that one' }]);
  });

  it('reads an answer with nothing in it as nothing refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const answer = await fetchBulkAccess('delete', rows);

    expect(answer.refused).toEqual([]);
  });

  it('surfaces the server message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'every object needs a name' }),
      }),
    );

    await expect(fetchBulkAccess('delete', rows)).rejects.toThrow('every object needs a name');
  });

  it('falls back to a status message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        json: () => Promise.reject(new Error('not json')),
      }),
    );

    await expect(fetchBulkAccess('delete', rows)).rejects.toThrow(
      'access request failed with status 503',
    );
  });
});
