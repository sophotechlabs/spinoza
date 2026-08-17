import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { NAMESPACE_KEY, useNamespaceStore } from '../../src/store/namespace';
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
  useNamespaceStore.setState({ namespace: 'default', names: [] });
});

afterEach(() => {
  resetStored();
});

describe('the namespace the app works in', () => {
  it('starts on default', () => {
    expect(state().namespace).toBe('default');
  });

  it('keeps what was chosen', () => {
    state().choose('shop');

    expect(state().namespace).toBe('shop');
    expect(readStored(NAMESPACE_KEY)).toBe('shop');
  });

  it('takes the names the cluster reported', () => {
    state().offer(['default', 'shop']);

    expect(state().names).toEqual(['default', 'shop']);
    expect(state().namespace).toBe('default');
  });

  it('falls back when the kept namespace is not in this cluster', () => {
    state().choose('shop');

    state().offer(['default', 'kube-system']);

    expect(state().namespace).toBe('default');
  });

  it('starts where an earlier session left off', async () => {
    writeStored(NAMESPACE_KEY, 'argocd');
    vi.resetModules();

    const fresh = await freshStore();

    expect(fresh.getState().namespace).toBe('argocd');
  });
});
