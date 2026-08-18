import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ALL, NAMESPACE_KEY, useNamespaceStore } from '../../src/store/namespace';
import { readStored, resetStored, writeStored } from '../../src/lib/persist';

function state() {
  return useNamespaceStore.getState();
}

async function freshStore() {
  const { useNamespaceStore: fresh } = await import('../../src/store/namespace');
  return fresh;
}

beforeEach(() => {
  resetStored();
  useNamespaceStore.setState({ namespace: ALL, names: [] });
});

afterEach(() => {
  resetStored();
});

describe('the namespace the app works in', () => {
  it('starts on every namespace', () => {
    expect(state().namespace).toBe(ALL);
  });

  it('keeps what was chosen', () => {
    state().choose('shop');

    expect(state().namespace).toBe('shop');
    expect(readStored(NAMESPACE_KEY)).toBe('shop');
  });

  it('takes the names the cluster reported without narrowing the view', () => {
    state().offer(['default', 'shop']);

    expect(state().names).toEqual(['default', 'shop']);
    expect(state().namespace).toBe(ALL);
  });

  it('widens back out when the kept namespace is not in this cluster', () => {
    state().choose('shop');

    state().offer(['default', 'kube-system']);

    expect(state().namespace).toBe(ALL);
  });

  it('keeps a kept namespace this cluster does have', () => {
    state().choose('shop');

    state().offer(['default', 'shop']);

    expect(state().namespace).toBe('shop');
  });

  it('starts where an earlier session left off', async () => {
    writeStored(NAMESPACE_KEY, 'argocd');
    vi.resetModules();

    const fresh = await freshStore();

    expect(fresh.getState().namespace).toBe('argocd');
  });
});
