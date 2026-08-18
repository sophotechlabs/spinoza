import { describe, expect, it } from 'vitest';
import type { ObjectRef } from '../../src/lib/types';
import { VIEW_LABELS, clusterItems, matchItems, paletteItems } from '../../src/lib/palette';
import { makeCategory, makeDescriptor } from '../helpers';
import { VIEWS } from '../../src/lib/types';

const podType = makeDescriptor({ resource: 'pods', kind: 'Pod' });
const deploymentType = makeDescriptor({
  group: 'apps',
  resource: 'deployments',
  kind: 'Deployment',
});

const categories = [
  makeCategory('Workloads', [podType, deploymentType]),
  makeCategory('Config', [makeDescriptor({ resource: 'configmaps', kind: 'ConfigMap' })]),
];

const everyCategory = [
  ...categories,
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'kustomize.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'kustomizations',
      kind: 'Kustomization',
    }),
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      kind: 'Application',
    }),
  ]),
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

    expect(items[0]).toMatchObject({ kind: 'object', label: 'prod/web-0', hint: 'recent pods' });
    expect(items[1]).toMatchObject({ kind: 'view', label: VIEW_LABELS.cluster });
    expect(items.filter((item) => item.kind === 'view').map((item) => item.label)).toContain(
      VIEW_LABELS.helm,
    );
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

    expect(pod?.hint).toBe('Workloads v1');
    expect(deployment?.hint).toBe('Workloads apps/v1');
  });

  it('drops the namespace from a cluster-scoped recent object', () => {
    const items = paletteItems([], [{ ...recent, namespace: '', name: 'node-1' }]);

    expect(items[0].label).toBe('node-1');
  });

  it('carries the kind a recent object belongs to, so its list can be opened', () => {
    const items = paletteItems(categories, [recent]);

    expect(items[0]).toMatchObject({ type: { kind: 'Pod', resource: 'pods' } });
  });

  it('carries no kind for a recent object discovery no longer knows', () => {
    const items = paletteItems(categories, [{ ...recent, resource: 'widgets' }]);

    expect(items[0]).toMatchObject({ type: null });
  });
});

describe('the view registry', () => {
  it('offers every registered view, in its own order', () => {
    const offered = paletteItems(everyCategory, [])
      .filter((item) => item.kind === 'view')
      .map((item) => item.view);

    expect([...offered].sort()).toEqual([...VIEWS].sort());
  });

  it('labels every registered view', () => {
    for (const view of VIEWS) {
      expect(VIEW_LABELS[view]).not.toBe('');
    }
  });
});

describe('clusterItems', () => {
  const hit = {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    kind: 'Deployment',
    namespace: 'airbyte',
    name: 'airbyte-server',
  };

  it('labels a hit with its namespace and names its kind', () => {
    const items = clusterItems([hit], categories);

    expect(items[0]).toMatchObject({
      label: 'airbyte/airbyte-server',
      hint: 'deployment',
      type: deploymentType,
    });
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

  it('leaves the flux views out of a cluster without flux', () => {
    const items = paletteItems([makeCategory('Workloads', [makeDescriptor({})])], []);

    expect(items.filter((item) => item.kind === 'view').map((item) => item.label)).not.toContain(
      'Flux graph',
    );
  });

  it('offers the flux views once flux is there', () => {
    const items = paletteItems(
      [
        makeCategory('Custom resources', [
          makeDescriptor({
            group: 'helm.toolkit.fluxcd.io',
            version: 'v2',
            resource: 'helmreleases',
            kind: 'HelmRelease',
          }),
        ]),
      ],
      [],
    );

    expect(items.filter((item) => item.kind === 'view').map((item) => item.label)).toContain(
      'Flux graph',
    );
  });
});
