import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { clearFilters, forgetFilters, imposeChips, useFiltersStore } from '../../src/store/filters';
import { MK1, MK2, showing } from '../helpers-clusters';

const PODS = '/v1/pods';
const NODES = '/v1/nodes';

function chipsOf(key: string, cluster = MK1) {
  return useFiltersStore.getState().byCluster[cluster]?.[key];
}

describe('the filter chip store', () => {
  beforeEach(() => {
    clearFilters();
    showing(MK1);
  });

  afterEach(() => {
    clearFilters();
  });

  it('keeps chips per kind', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(NODES, { field: 'name', value: 'gke' });

    expect(chipsOf(PODS)).toEqual([{ field: 'name', value: 'web' }]);
    expect(chipsOf(NODES)).toEqual([{ field: 'name', value: 'gke' }]);
  });

  it('appends in the order they were added', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(PODS, { field: 'status', value: 'Running' });

    expect(chipsOf(PODS)).toEqual([
      { field: 'name', value: 'web' },
      { field: 'status', value: 'Running' },
    ]);
  });

  it('ignores a chip that is already there, whatever its case', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'WEB' });

    expect(chipsOf(PODS)).toHaveLength(1);
  });

  it('removes the chip at a position', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(PODS, { field: 'status', value: 'Running' });

    useFiltersStore.getState().removeAt(PODS, 0);

    expect(chipsOf(PODS)).toEqual([{ field: 'status', value: 'Running' }]);
  });

  it('forgets a kind whose last chip was removed', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });

    useFiltersStore.getState().removeAt(PODS, 0);

    expect(chipsOf(PODS)).toBeUndefined();
  });

  it('replaces what a kind had when a filter is imposed', () => {
    useFiltersStore.getState().add(PODS, { field: 'status', value: 'Running' });

    imposeChips(PODS, [{ field: 'name', value: 'coredns' }]);

    expect(chipsOf(PODS)).toEqual([{ field: 'name', value: 'coredns' }]);
  });

  it('imposing nothing leaves the kind unfiltered', () => {
    useFiltersStore.getState().add(PODS, { field: 'status', value: 'Running' });

    imposeChips(PODS, []);

    expect(chipsOf(PODS)).toBeUndefined();
  });

  it('clears one kind and leaves the others alone', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(NODES, { field: 'name', value: 'gke' });

    useFiltersStore.getState().clearKind(PODS);

    expect(chipsOf(PODS)).toBeUndefined();
    expect(chipsOf(NODES)).toHaveLength(1);
  });

  it('drops every cluster when the window is torn down', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });

    clearFilters();

    expect(useFiltersStore.getState().byCluster).toEqual({});
  });
});

describe('filter chips on another tab', () => {
  beforeEach(() => {
    clearFilters();
    showing(MK1);
  });

  afterEach(() => {
    clearFilters();
  });

  it("are the other tab's own", () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });

    showing(MK2);
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'api' });

    expect(chipsOf(PODS)).toEqual([{ field: 'name', value: 'web' }]);
    expect(chipsOf(PODS, MK2)).toEqual([{ field: 'name', value: 'api' }]);
  });

  it('go when the tab is closed', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });

    forgetFilters(MK1);

    expect(chipsOf(PODS)).toBeUndefined();
  });
});
