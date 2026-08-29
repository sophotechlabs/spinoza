import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  clearCatalog,
  forgetCatalog,
  rememberCatalog,
  rememberCounts,
  useCatalogStore,
} from '../../src/store/catalog';
import { makeCategory, makeDescriptor } from '../helpers';
import { MK1, MK2, showing } from '../helpers-clusters';

const categories = [makeCategory('Workloads', [makeDescriptor({ resource: 'pods', kind: 'Pod' })])];

function categoriesOf(cluster: string) {
  return useCatalogStore.getState().categories[cluster] ?? [];
}

function countsOf(cluster: string) {
  return useCatalogStore.getState().counts[cluster] ?? {};
}

describe('the discovered catalog', () => {
  beforeEach(() => {
    clearCatalog();
    showing(MK1);
  });

  afterEach(() => {
    clearCatalog();
  });

  it('starts with nothing discovered', () => {
    expect(categoriesOf(MK1)).toEqual([]);
  });

  it('holds what discovery found', () => {
    rememberCatalog(categories);

    expect(categoriesOf(MK1)).toEqual(categories);
  });

  it('discovers each cluster separately', () => {
    rememberCatalog(categories);

    showing(MK2);

    expect(categoriesOf(MK2)).toEqual([]);
    expect(categoriesOf(MK1)).toEqual(categories);
  });

  it('lets go of a closed tab', () => {
    rememberCatalog(categories);
    rememberCounts({ '/v1/pods': 3 });

    forgetCatalog(MK1);

    expect(categoriesOf(MK1)).toEqual([]);
    expect(countsOf(MK1)).toEqual({});
  });

  it('forgets every cluster when the window is torn down', () => {
    rememberCatalog(categories);

    clearCatalog();

    expect(useCatalogStore.getState().categories).toEqual({});
  });
});

describe('the resource counts', () => {
  beforeEach(() => {
    clearCatalog();
    showing(MK1);
  });

  afterEach(() => {
    clearCatalog();
  });

  it('start out unknown', () => {
    expect(countsOf(MK1)).toEqual({});
  });

  it('hold the tally the sidebar fetched', () => {
    rememberCounts({ '/v1/pods': 2993 });

    expect(countsOf(MK1)['/v1/pods']).toBe(2993);
  });

  it('are counted per cluster', () => {
    rememberCounts({ '/v1/pods': 2993 });

    showing(MK2);
    rememberCounts({ '/v1/pods': 7 });

    expect(countsOf(MK1)['/v1/pods']).toBe(2993);
    expect(countsOf(MK2)['/v1/pods']).toBe(7);
  });
});
