import { afterEach, describe, expect, it } from 'vitest';
import { clearFilters, imposeChips, useFiltersStore } from '../../src/store/filters';

const PODS = '/v1/pods';
const NODES = '/v1/nodes';

function chipsOf(key: string) {
  return useFiltersStore.getState().chips[key];
}

describe('the filter chip store', () => {
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

  it('drops everything when the cluster changes', () => {
    useFiltersStore.getState().add(PODS, { field: 'name', value: 'web' });
    useFiltersStore.getState().add(NODES, { field: 'name', value: 'gke' });

    clearFilters();

    expect(useFiltersStore.getState().chips).toEqual({});
  });
});
