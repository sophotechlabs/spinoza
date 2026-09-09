import { describe, expect, it } from 'vitest';
import {
  filterCategories,
  groupByApiGroup,
  isNested,
  matchesKind,
  shownInTree,
  NESTED_CATEGORY,
} from '../../src/lib/sidebarTree';
import { makeCategory, makeDescriptor } from '../helpers';

describe('sidebarTree', () => {
  it('nests only the custom resources category', () => {
    expect(isNested(NESTED_CATEGORY)).toBe(true);
    expect(isNested('Workloads')).toBe(false);
    expect(isNested('Cluster')).toBe(false);
  });

  it('groups descriptors by api group, sorted by group name', () => {
    const resources = [
      makeDescriptor({ group: 'traefik.io', resource: 'ingressroutes', kind: 'IngressRoute' }),
      makeDescriptor({ group: 'cilium.io', resource: 'ciliumendpoints', kind: 'CiliumEndpoint' }),
      makeDescriptor({ group: 'traefik.io', resource: 'middlewares', kind: 'Middleware' }),
    ];

    const groups = groupByApiGroup(resources);

    expect(groups.map((group) => group.name)).toEqual(['cilium.io', 'traefik.io']);
    expect(groups[0].resources).toHaveLength(1);
    expect(groups[1].resources.map((r) => r.kind)).toEqual(['IngressRoute', 'Middleware']);
  });

  it('keeps the order the server sent within a group', () => {
    const resources = [
      makeDescriptor({ group: 'cilium.io', resource: 'b', kind: 'B' }),
      makeDescriptor({ group: 'cilium.io', resource: 'a', kind: 'A' }),
    ];

    expect(groupByApiGroup(resources)[0].resources.map((r) => r.kind)).toEqual(['B', 'A']);
  });

  it('returns nothing for an empty category', () => {
    expect(groupByApiGroup([])).toEqual([]);
  });
});

describe('finding a kind by name', () => {
  const pod = makeDescriptor({ group: '', resource: 'pods', kind: 'Pod' });
  const route = makeDescriptor({
    group: 'traefik.io',
    resource: 'ingressroutes',
    kind: 'IngressRoute',
  });

  it('matches everything when nothing is typed', () => {
    expect(matchesKind(pod, '')).toBe(true);
    expect(matchesKind(pod, '   ')).toBe(true);
  });

  it('matches the kind, the plural, or the api group', () => {
    expect(matchesKind(pod, 'po')).toBe(true);
    expect(matchesKind(pod, 'PODS')).toBe(true);
    expect(matchesKind(route, 'traefik')).toBe(true);
    expect(matchesKind(pod, 'traefik')).toBe(false);
  });

  it('keeps only the categories that still hold something', () => {
    const categories = [makeCategory('Workloads', [pod]), makeCategory(NESTED_CATEGORY, [route])];

    expect(filterCategories(categories, '')).toBe(categories);
    expect(filterCategories(categories, 'pod').map((one) => one.name)).toEqual(['Workloads']);
    expect(filterCategories(categories, 'zzz')).toEqual([]);
  });
});

describe('whether the kind on screen is already in the tree', () => {
  const pod = makeDescriptor({ group: '', resource: 'pods', kind: 'Pod' });
  const route = makeDescriptor({
    group: 'traefik.io',
    resource: 'ingressroutes',
    kind: 'IngressRoute',
  });
  const categories = [makeCategory('Workloads', [pod]), makeCategory(NESTED_CATEGORY, [route])];
  const open = () => true;
  const shut = () => false;

  it('says no when nothing is on screen', () => {
    expect(shownInTree(categories, open, null)).toBe(false);
  });

  it('says no when the kind is not in the list at all', () => {
    const other = makeDescriptor({ group: 'apps', resource: 'deployments', kind: 'Deployment' });

    expect(shownInTree(categories, open, other)).toBe(false);
  });

  it('follows the category it lives in', () => {
    expect(shownInTree(categories, open, pod)).toBe(true);
    expect(shownInTree(categories, shut, pod)).toBe(false);
  });

  it('follows the api group under custom resources', () => {
    expect(shownInTree(categories, open, route)).toBe(true);
    expect(shownInTree(categories, (section) => section === NESTED_CATEGORY, route)).toBe(false);
  });
});
