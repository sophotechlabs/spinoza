import { describe, expect, it } from 'vitest';
import type { ObjectRef } from '../../src/lib/types';
import { VIEW_LABELS, matchItems, paletteItems } from '../../src/lib/palette';
import { makeCategory, makeDescriptor } from '../helpers';

const categories = [
  makeCategory('Workloads', [
    makeDescriptor({ resource: 'pods', kind: 'Pod' }),
    makeDescriptor({ group: 'apps', resource: 'deployments', kind: 'Deployment' }),
  ]),
  makeCategory('Config', [makeDescriptor({ resource: 'configmaps', kind: 'ConfigMap' })]),
];

const recent: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web-0',
};

describe('paletteItems', () => {
  it('puts recent objects first, then views, then every discovered kind', () => {
    const items = paletteItems(categories, [recent]);

    expect(items[0]).toMatchObject({ kind: 'object', label: 'prod/web-0', hint: 'recent · pods' });
    expect(items[1]).toMatchObject({ kind: 'view', label: VIEW_LABELS.resources });
    expect(items.filter((item) => item.kind === 'resource').map((item) => item.label)).toEqual([
      'Pod',
      'Deployment',
      'ConfigMap',
    ]);
  });

  it('names the api group of each kind, and the version alone for core', () => {
    const items = paletteItems(categories, []);
    const pod = items.find((item) => item.label === 'Pod');
    const deployment = items.find((item) => item.label === 'Deployment');

    expect(pod?.hint).toBe('Workloads · v1');
    expect(deployment?.hint).toBe('Workloads · apps/v1');
  });

  it('drops the namespace from a cluster-scoped recent object', () => {
    const items = paletteItems([], [{ ...recent, namespace: '', name: 'node-1' }]);

    expect(items[0].label).toBe('node-1');
  });
});

describe('matchItems', () => {
  const items = paletteItems(categories, [recent]);

  it('returns everything for an empty query', () => {
    expect(matchItems(items, '   ')).toHaveLength(items.length);
  });

  it('puts an exact label before a prefix before a substring', () => {
    const pods = paletteItems(
      [
        makeCategory('Workloads', [
          makeDescriptor({ resource: 'x', kind: 'CronPod' }),
          makeDescriptor({ resource: 'y', kind: 'PodDisruptionBudget' }),
          makeDescriptor({ resource: 'z', kind: 'Pod' }),
        ]),
      ],
      [],
    );

    const found = matchItems(pods, 'pod')
      .filter((item) => item.kind === 'resource')
      .map((item) => item.label);

    expect(found).toEqual(['Pod', 'PodDisruptionBudget', 'CronPod']);
  });

  it('matches on the hint too, so an api group finds its kinds', () => {
    const found = matchItems(items, 'apps/v1').map((item) => item.label);

    expect(found).toEqual(['Deployment']);
  });

  it('keeps catalog order between two equally good matches', () => {
    const both = paletteItems(
      [
        makeCategory('Workloads', [
          makeDescriptor({ resource: 'a', kind: 'CronPod' }),
          makeDescriptor({ resource: 'b', kind: 'BatchPod' }),
        ]),
      ],
      [],
    );

    const found = matchItems(both, 'pod')
      .filter((item) => item.kind === 'resource')
      .map((item) => item.label);

    expect(found).toEqual(['CronPod', 'BatchPod']);
  });

  it('is case insensitive', () => {
    expect(matchItems(items, 'CONFIGMAP').map((item) => item.label)).toEqual(['ConfigMap']);
  });

  it('returns nothing when nothing matches', () => {
    expect(matchItems(items, 'zzzz')).toEqual([]);
  });
});
