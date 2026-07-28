import { describe, expect, it } from 'vitest';
import { groupByApiGroup, isNested, NESTED_CATEGORY } from '../../src/lib/sidebarTree';
import { makeDescriptor } from '../helpers';

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
