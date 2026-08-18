import { afterEach, describe, expect, it } from 'vitest';
import { clearCatalog, rememberCatalog, useCatalogStore } from '../../src/store/catalog';
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
