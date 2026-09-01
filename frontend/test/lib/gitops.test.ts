import { describe, expect, it } from 'vitest';
import { CLUSTER_ABSENCE, argoInstalled, fluxInstalled, gitopsAbsence } from '../../src/lib/gitops';
import { makeCategory, makeDescriptor } from '../helpers';
import { ARGO_VIEWS, FLUX_VIEWS } from '../../src/lib/types';

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

describe('gitopsAbsence', () => {
  it('gives every Flux view the same missing-controller sentence', () => {
    for (const view of FLUX_VIEWS) {
      expect(gitopsAbsence(view, plain)).toBe(CLUSTER_ABSENCE.flux);
      expect(gitopsAbsence(view, flux)).toBeNull();
    }
  });

  it('gives every Argo CD view the same missing-controller sentence', () => {
    for (const view of ARGO_VIEWS) {
      expect(gitopsAbsence(view, plain)).toBe(CLUSTER_ABSENCE.argo);
      expect(gitopsAbsence(view, argo)).toBeNull();
    }
  });

  it('does not call an unrelated view missing', () => {
    expect(gitopsAbsence('issues', plain)).toBeNull();
  });
});
