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
    expect(opensOn('kind-dev')).toBe(ALL);
  });

  it('is default once the setting says so', () => {
    useSettingsStore.setState({ namespaceStart: 'default' });

    expect(opensOn('kind-dev')).toBe('default');
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

describe('opening a cluster on its own answer', () => {
  it('takes the answer recorded for that cluster over the general one', () => {
    useSettingsStore.setState({
      namespaceStart: 'all',
      namespaceStarts: { 'gke-prod': 'default' },
    });

    expect(opensOn('gke-prod')).toBe('default');
    expect(opensOn('p-mk2')).toBe(ALL);
  });

  it('applies that answer when the cluster name arrives', () => {
    useSettingsStore.setState({
      namespaceStart: 'all',
      namespaceStarts: { 'gke-prod': 'default' },
    });

    state().openOn('gke-prod');

    expect(state().namespace).toBe('default');
  });

  it('leaves a namespace the user picked alone', () => {
    useSettingsStore.setState({
      namespaceStart: 'all',
      namespaceStarts: { 'gke-prod': 'default' },
    });
    state().choose('shop');

    state().openOn('gke-prod');

    expect(state().namespace).toBe('shop');
  });

  it('forgets that pick when the cluster changes', () => {
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: {} });
    state().choose('shop');

    state().reset();
    state().openOn('p-mk2');

    expect(state().namespace).toBe(ALL);
  });
});
