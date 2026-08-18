import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchNamespaces } from '../../src/lib/namespaces';
import { ALL, settle } from '../../src/store/namespace';

function stub(body: unknown, ok = true, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) })),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchNamespaces', () => {
  it('reads the names', async () => {
    stub({ names: ['default', 'shop'] });

    await expect(fetchNamespaces()).resolves.toEqual({
      names: ['default', 'shop'],
      error: undefined,
    });
  });

  it('has no names when the backend sent none', async () => {
    stub({});

    expect((await fetchNamespaces()).names).toEqual([]);
  });

  it('carries a partial failure', async () => {
    stub({ names: [], error: 'namespaces is forbidden' });

    expect((await fetchNamespaces()).error).toBe('namespaces is forbidden');
  });

  it('reports a request the backend refused', async () => {
    stub({ message: 'spinoza has no cluster' }, false, 503);

    await expect(fetchNamespaces()).rejects.toThrow('no cluster');
  });
});

describe('settle', () => {
  it('keeps a namespace the cluster has', () => {
    expect(settle('shop', ['default', 'shop'])).toBe('shop');
  });

  it('keeps the all-namespaces choice', () => {
    expect(settle(ALL, ['default'])).toBe(ALL);
  });

  it('falls back to every namespace when the kept one is gone', () => {
    expect(settle('shop', ['default', 'kube-system'])).toBe(ALL);
  });

  it('waits rather than guessing before the names arrive', () => {
    expect(settle('shop', [])).toBe('shop');
  });
});
