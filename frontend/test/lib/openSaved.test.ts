import { describe, expect, it } from 'vitest';
import { kindResource, openingKind, openingSaved } from '../../src/lib/openSaved';
import type { Category, SavedView } from '../../src/lib/types';
import type { Route } from '../../src/lib/router';

const route: Route = {
  context: 'kind-dev',
  view: 'checks',
  resource: null,
  selection: null,
  release: null,
};

const categories: Category[] = [
  {
    name: 'Workloads',
    resources: [
      {
        group: '',
        version: 'v1',
        resource: 'pods',
        kind: 'Pod',
        namespaced: true,
        category: 'Workloads',
      },
      {
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        kind: 'Deployment',
        namespaced: true,
        category: 'Workloads',
      },
    ],
  },
];

function saved(extra: Partial<SavedView> = {}): SavedView {
  return { id: 'a', name: 'one', view: 'resources', ...extra };
}

describe('opening a saved view', () => {
  it('goes to the kind it names, and drops the selection', () => {
    const opening = openingSaved(route, saved({ resource: 'pods' }), categories);

    expect(opening.route.view).toBe('resources');
    expect(opening.route.resource).toEqual({
      group: '',
      version: 'v1',
      resource: 'pods',
      kind: 'Pod',
    });
    expect(opening.route.selection).toBeNull();
  });

  it('carries the namespace it was saved with, and nothing when it had none', () => {
    expect(openingSaved(route, saved({ namespace: 'prod' }), categories).namespace).toBe('prod');
    expect(openingSaved(route, saved(), categories).namespace).toBeNull();
    expect(openingSaved(route, saved({ namespace: '' }), categories).namespace).toBeNull();
  });

  it('carries the filter under the kind it belongs to', () => {
    const opening = openingSaved(
      route,
      saved({ resource: 'pods', filter: 'status:CrashLoopBackOff' }),
      categories,
    );

    expect(opening.filter).toEqual({
      key: 'pods',
      chips: [{ field: 'status', value: 'CrashLoopBackOff' }],
    });
  });

  it('carries no filter for a view that names no kind', () => {
    expect(
      openingSaved(route, saved({ view: 'checks', filter: 'x' }), categories).filter,
    ).toBeNull();
    expect(openingSaved(route, saved({ resource: 'pods' }), categories).filter).toBeNull();
  });

  it('falls back to the view when this cluster does not have that kind', () => {
    const opening = openingSaved(route, saved({ view: 'checks', resource: 'widgets' }), categories);

    expect(opening.route.view).toBe('checks');
    expect(opening.route.resource).toBeNull();
  });

  it('goes straight to the view when the saved view names no kind at all', () => {
    const opening = openingSaved(route, saved({ view: 'issues' }), categories);

    expect(opening.route.view).toBe('issues');
  });
});

describe('opening a kind from a row', () => {
  it('reads a workload kind as its resource', () => {
    expect(kindResource('Deployment')).toBe('deployments');
    expect(kindResource('')).toBe('pods');
  });

  it('opens the kind the row names', () => {
    const next = openingKind(route, categories, 'Deployment');

    expect(next.view).toBe('resources');
    expect(next.resource?.kind).toBe('Deployment');
  });

  it('opens pods for a row that names no kind', () => {
    expect(openingKind(route, categories, '').resource?.kind).toBe('Pod');
  });

  it('opens the resources view when this cluster does not have that kind', () => {
    const next = openingKind(route, categories, 'Widget');

    expect(next.view).toBe('resources');
    expect(next.resource).toBeNull();
  });
});
