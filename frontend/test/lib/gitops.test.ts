import { describe, expect, it } from 'vitest';
import { argoInstalled, argoTypes, fluxInstalled } from '../../src/lib/gitops';
import { makeCategory, makeDescriptor } from '../helpers';

const plain = [
  makeCategory('Workloads', [
    makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments', kind: 'Deployment' }),
  ]),
];

const flux = [
  ...plain,
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'source.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'gitrepositories',
      kind: 'GitRepository',
    }),
  ]),
];

const argo = [
  ...plain,
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'appprojects',
      kind: 'AppProject',
    }),
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      kind: 'Application',
    }),
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'workflows',
      kind: 'Workflow',
    }),
  ]),
];

describe('fluxInstalled', () => {
  it('is false on a cluster with no flux types', () => {
    expect(fluxInstalled(plain)).toBe(false);
    expect(fluxInstalled([])).toBe(false);
  });

  it('is true once any toolkit group is discovered', () => {
    expect(fluxInstalled(flux)).toBe(true);
  });
});

describe('argoInstalled', () => {
  it('is false on a cluster with no argo types', () => {
    expect(argoInstalled(plain)).toBe(false);
  });

  it('is true once argo types are discovered', () => {
    expect(argoInstalled(argo)).toBe(true);
  });
});

describe('argoTypes', () => {
  it('keeps the kinds worth a shortcut, in a fixed order', () => {
    expect(argoTypes(argo).map((descriptor) => descriptor.kind)).toEqual([
      'Application',
      'AppProject',
    ]);
  });

  it('has nothing to offer on a cluster without argo', () => {
    expect(argoTypes(plain)).toEqual([]);
  });
});
