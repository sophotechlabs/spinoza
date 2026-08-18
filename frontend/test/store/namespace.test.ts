import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { ALL, opensOn, useNamespaceStore } from '../../src/store/namespace';
import { useSettingsStore } from '../../src/store/settings';
import { resetStored } from '../../src/lib/persist';

function state() {
  return useNamespaceStore.getState();
}

beforeEach(() => {
  resetStored();
  useSettingsStore.setState({ namespaceStart: 'all' });
  useNamespaceStore.setState({ namespace: ALL, names: [] });
});

afterEach(() => {
  resetStored();
  useSettingsStore.setState({ namespaceStart: 'all' });
});

describe('the namespace the app works in', () => {
  it('starts on every namespace', () => {
    expect(state().namespace).toBe(ALL);
  });

  it('keeps what was chosen', () => {
    state().choose('shop');

    expect(state().namespace).toBe('shop');
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
});

describe('the namespace a new cluster opens on', () => {
  it('is every namespace while that is the setting', () => {
    expect(opensOn()).toBe(ALL);
  });

  it('is default once the setting says so', () => {
    useSettingsStore.setState({ namespaceStart: 'default' });

    expect(opensOn()).toBe('default');
  });

  it('is what a reset goes back to', () => {
    useSettingsStore.setState({ namespaceStart: 'default' });
    state().choose('shop');
    state().offer(['shop', 'default']);

    state().reset();

    expect(state().namespace).toBe('default');
    expect(state().names).toEqual([]);
  });
});
