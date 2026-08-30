import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  ALL,
  forgetNamespace,
  namespaceNow,
  opensOn,
  useNamespaceStore,
} from '../../src/store/namespace';
import { MK1, MK2, showing } from '../helpers-clusters';
import { namespaceAnswered, useSettingsStore } from '../../src/store/settings';
import { resetStored } from '../../src/lib/persist';

function state() {
  return useNamespaceStore.getState();
}

function scopeOf(cluster = MK1) {
  return useNamespaceStore.getState().byCluster[cluster];
}

function namesOf(cluster = MK1): string[] {
  return scopeOf(cluster)?.names ?? [];
}

beforeEach(() => {
  resetStored();
  useSettingsStore.setState({ namespaceStart: 'all' });
  useNamespaceStore.getState().reset();
  showing(MK1);
});

afterEach(() => {
  resetStored();
  useSettingsStore.setState({ namespaceStart: 'all' });
});

describe('the namespace the app works in', () => {
  it('starts on every namespace', () => {
    expect(namespaceNow()).toBe(ALL);
  });

  it('keeps what was chosen', () => {
    state().choose('shop');

    expect(namespaceNow()).toBe('shop');
  });

  it('takes the names the cluster reported without narrowing the view', () => {
    state().offer(['default', 'shop']);

    expect(namesOf()).toEqual(['default', 'shop']);
    expect(namespaceNow()).toBe(ALL);
  });

  it('widens back out when the kept namespace is not in this cluster', () => {
    state().choose('shop');

    state().offer(['default', 'kube-system']);

    expect(namespaceNow()).toBe(ALL);
  });

  it('keeps a kept namespace this cluster does have', () => {
    state().choose('shop');

    state().offer(['default', 'shop']);

    expect(namespaceNow()).toBe('shop');
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

    expect(namespaceNow()).toBe('default');
    expect(namesOf()).toEqual([]);
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

    expect(namespaceNow()).toBe('default');
  });

  it('leaves a namespace the user picked alone', () => {
    useSettingsStore.setState({
      namespaceStart: 'all',
      namespaceStarts: { 'gke-prod': 'default' },
    });
    state().choose('shop');

    state().openOn('gke-prod');

    expect(namespaceNow()).toBe('shop');
  });

  it('leaves the pick behind on the tab that made it', () => {
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: {} });
    state().choose('shop');

    showing(MK2);
    state().openOn('p-mk2');

    expect(namespaceNow()).toBe(ALL);
  });

  it('gives the pick back on the tab that made it', () => {
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: {} });
    state().choose('shop');
    showing(MK2);

    showing(MK1);

    expect(namespaceNow()).toBe('shop');
  });

  it('lets go of a closed tab', () => {
    state().choose('shop');

    forgetNamespace(MK1);

    expect(namespaceNow()).toBe(ALL);
  });
});

describe('the answer about which namespace a cluster opens on', () => {
  beforeEach(() => {
    resetStored();
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: {} });
    useNamespaceStore.getState().reset();
    showing(MK1);
  });

  it('is kept against the cluster, not the context that reached it', () => {
    useSettingsStore.getState().setNamespaceStart(MK1, 'default');

    expect(useSettingsStore.getState().namespaceStarts[MK1]).toBe('default');
    expect(namespaceAnswered(MK1)).toBe(true);
    expect(opensOn(MK1)).toBe('default');
  });

  it('still counts an answer written against a context name before the move', () => {
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: { 'p-mk1': 'default' } });

    expect(namespaceAnswered(MK1, 'p-mk1')).toBe(true);
    expect(opensOn(MK1, 'p-mk1')).toBe('default');
  });

  it("does not take another cluster's old answer", () => {
    useSettingsStore.setState({ namespaceStart: 'all', namespaceStarts: { 'p-mk1': 'default' } });

    expect(namespaceAnswered(MK2, 'p-mk2')).toBe(false);
    expect(opensOn(MK2, 'p-mk2')).toBe(ALL);
  });

  it("lets the cluster's own answer win over the older one", () => {
    useSettingsStore.setState({
      namespaceStart: 'all',
      namespaceStarts: { 'p-mk1': 'default', [MK1]: 'all' },
    });

    expect(opensOn(MK1, 'p-mk1')).toBe(ALL);
  });

  it('shares one answer between two contexts on the same cluster', () => {
    useSettingsStore.getState().setNamespaceStart(MK1, 'default');

    expect(opensOn(MK1, 'p-mk1')).toBe('default');
    expect(opensOn(MK1, 'p-mk1-admin')).toBe('default');
  });
});
