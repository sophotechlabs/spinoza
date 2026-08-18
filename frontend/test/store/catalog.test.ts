import { afterEach, describe, expect, it } from 'vitest';
import {
  clearCatalog,
  rememberCatalog,
  rememberCounts,
  useCatalogStore,
} from '../../src/store/catalog';
import { makeCategory, makeDescriptor } from '../helpers';

const categories = [makeCategory('Workloads', [makeDescriptor({ resource: 'pods', kind: 'Pod' })])];

describe('the discovered catalog', () => {
  afterEach(() => {
    clearCatalog();
  });

  it('starts with nothing discovered', () => {
    expect(useCatalogStore.getState().categories).toEqual([]);
  });

  it('holds what discovery found', () => {
    rememberCatalog(categories);

    expect(useCatalogStore.getState().categories).toEqual(categories);
  });

  it('forgets it when the cluster changes', () => {
    rememberCatalog(categories);

    clearCatalog();

    expect(useCatalogStore.getState().categories).toEqual([]);
  });
});

describe('the resource counts', () => {
  afterEach(() => {
    clearCatalog();
  });

  it('start out unknown', () => {
    expect(useCatalogStore.getState().counts).toEqual({});
  });

  it('hold the tally the sidebar fetched', () => {
    rememberCounts({ '/v1/pods': 2993 });

    expect(useCatalogStore.getState().counts['/v1/pods']).toBe(2993);
  });

  it('are forgotten with the rest of the catalog', () => {
    rememberCounts({ '/v1/pods': 2993 });

    clearCatalog();

    expect(useCatalogStore.getState().counts).toEqual({});
  });
});
