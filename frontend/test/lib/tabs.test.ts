import { beforeEach, describe, expect, it } from 'vitest';
import { attachedTo, contextOf, displayName, forgetTab, tabWidth } from '../../src/lib/tabs';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { rememberObject, useRecentsStore } from '../../src/store/recents';
import { rememberCatalog, useCatalogStore } from '../../src/store/catalog';
import { useFiltersStore } from '../../src/store/filters';
import { setForwards, useForwardsStore } from '../../src/store/forwards';
import { reportHealth, useClusterHealthStore } from '../../src/store/clusterHealth';
import { useNamespaceStore } from '../../src/store/namespace';
import { useTerminalsStore } from '../../src/store/terminals';
import { makeCategory, makeDescriptor } from '../helpers';
import { MK1, MK2, listOf, showing } from '../helpers-clusters';

describe('the context a tab shows', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  it('is the one the tab was opened on', () => {
    adoptClusters(listOf(MK1));

    expect(contextOf(useClustersStore.getState().tabs, MK2)).toBe('p-mk2');
  });

  it('is nothing when no tab has that id', () => {
    expect(contextOf([], MK1)).toBe('');
  });
});

describe('closing a tab', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
    showing(MK1);
  });

  it('lets go of everything that belonged to it', () => {
    rememberObject({ group: '', version: 'v1', resource: 'pods', namespace: 'prod', name: 'web' });
    rememberCatalog([makeCategory('Workloads', [makeDescriptor({ resource: 'pods' })])]);
    useFiltersStore.getState().add('/v1/pods', { field: 'name', value: 'web' });
    setForwards([
      {
        id: '1',
        kind: 'pods',
        namespace: 'prod',
        name: 'web',
        localPort: 8080,
        remotePort: 80,
        state: 'running',
        startedAt: '2026-08-29T12:00:00Z',
      },
    ]);
    reportHealth(MK1, false, false, 'gone');
    useNamespaceStore.getState().choose('shop');
    useTerminalsStore.getState().open('prod', 'web', 'app');

    forgetTab(MK1);

    expect(useRecentsStore.getState().byCluster[MK1]).toBeUndefined();
    expect(useCatalogStore.getState().categories[MK1]).toBeUndefined();
    expect(useFiltersStore.getState().byCluster[MK1]).toBeUndefined();
    expect(useForwardsStore.getState().byCluster[MK1]).toBeUndefined();
    expect(useClusterHealthStore.getState().byCluster[MK1]).toBeUndefined();
    expect(useNamespaceStore.getState().byCluster[MK1]).toBeUndefined();
    expect(useTerminalsStore.getState().byCluster[MK1]).toBeUndefined();
  });
});

function forward(id: string) {
  return {
    id,
    kind: 'pods',
    namespace: 'prod',
    name: 'web',
    localPort: 8080,
    remotePort: 80,
    state: 'running' as const,
    startedAt: '2026-08-29T12:00:00Z',
  };
}

describe('what a tab still has attached', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
    useTerminalsStore.getState().reset();
    useForwardsStore.getState().clear();
    showing(MK1);
  });

  it('is nothing on a tab you only looked at', () => {
    expect(attachedTo(MK1)).toEqual([]);
  });

  it('counts one shell and one forward in the singular', () => {
    useTerminalsStore.getState().open('prod', 'web', 'app');
    setForwards([forward('1')]);

    expect(attachedTo(MK1)).toEqual(['1 shell', '1 port-forward']);
  });

  it('counts several in the plural', () => {
    useTerminalsStore.getState().open('prod', 'web', 'app');
    useTerminalsStore.getState().open('prod', 'api', 'app');
    setForwards([forward('1'), forward('2')]);

    expect(attachedTo(MK1)).toEqual(['2 shells', '2 port-forwards']);
  });

  it('says nothing about another tab', () => {
    useTerminalsStore.getState().open('prod', 'web', 'app');

    expect(attachedTo(MK2)).toEqual([]);
  });
});

describe('the name a tab goes by', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  it('is the context it was opened on until someone renames it', () => {
    adoptClusters(listOf(MK1));

    expect(displayName(useClustersStore.getState().tabs, MK1, 'p-mk1')).toBe('p-mk1');
  });

  it('is the name that was put on it', () => {
    adoptClusters(listOf(MK1));
    const tabs = useClustersStore.getState().tabs.map((tab) => {
      if (tab.id === MK1) {
        return { ...tab, label: 'client a prod' };
      }
      return tab;
    });

    expect(displayName(tabs, MK1, 'p-mk1')).toBe('client a prod');
  });

  it('falls back for a cluster no tab holds', () => {
    expect(displayName([], MK1, 'p-mk1')).toBe('p-mk1');
  });
});

describe('how much room a tab gets', () => {
  it('leaves it the full width while the strip has room', () => {
    expect(tabWidth(2)).toBe('max-w-56');
    expect(tabWidth(6)).toBe('max-w-56');
  });

  it('gives up label width before the strip grows the window', () => {
    expect(tabWidth(7)).toBe('max-w-32');
    expect(tabWidth(20)).toBe('max-w-32');
  });
});
