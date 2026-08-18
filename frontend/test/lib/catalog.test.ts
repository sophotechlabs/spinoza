import { describe, expect, it } from 'vitest';
import { kindScope, scopedBy, typeFor } from '../../src/lib/catalog';
import { makeCategory, makeDescriptor } from '../helpers';

const podType = makeDescriptor({ resource: 'pods', kind: 'Pod', namespaced: true });

const nodeType = makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false });

const categories = [makeCategory('Workloads', [podType]), makeCategory('Cluster', [nodeType])];

describe('typeFor', () => {
  it('finds the discovered kind behind a group, version and resource', () => {
    expect(typeFor(categories, { group: '', version: 'v1', resource: 'pods' })).toEqual(podType);
  });

  it('has nothing for a resource that is not in the catalog', () => {
    expect(typeFor(categories, { group: '', version: 'v1', resource: 'widgets' })).toBeNull();
  });
});

describe('what discovery knows about a kind scope', () => {
  it('says a namespaced kind is namespaced', () => {
    expect(kindScope(categories, { group: '', version: 'v1', resource: 'pods' })).toBe(true);
  });

  it('says a cluster-scoped kind is not', () => {
    expect(kindScope(categories, { group: '', version: 'v1', resource: 'nodes' })).toBe(false);
  });

  it('knows nothing with no kind on screen', () => {
    expect(kindScope(categories, null)).toBeNull();
  });

  it('knows nothing before the catalog has arrived', () => {
    expect(kindScope([], { group: '', version: 'v1', resource: 'pods' })).toBeNull();
  });
});

describe('settling on a scope', () => {
  it('takes what discovery says over the snapshot', () => {
    expect(scopedBy(true, false)).toBe(true);
    expect(scopedBy(false, true)).toBe(false);
  });

  it('falls back to the snapshot while discovery has nothing to say', () => {
    expect(scopedBy(null, true)).toBe(true);
    expect(scopedBy(null, false)).toBe(false);
  });
});
