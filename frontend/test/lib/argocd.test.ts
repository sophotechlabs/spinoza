import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchArgo, refOf, tree } from '../../src/lib/argocd';
import type { ArgoApp } from '../../src/lib/types';

function makeApp(name: string, extra: Partial<ArgoApp> = {}): ArgoApp {
  return {
    kind: 'Application',
    group: 'argoproj.io',
    version: 'v1alpha1',
    resource: 'applications',
    name,
    namespace: 'argocd',
    project: 'default',
    sync: 'Synced',
    health: 'Healthy',
    revision: 'abc123',
    repo: 'https://git/apps',
    path: `apps/${name}`,
    destination: 'in-cluster shop',
    message: '',
    createdAt: '2026-08-17T09:00:00Z',
    ...extra,
  };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('refOf', () => {
  it('turns an app into something the inspector can open', () => {
    expect(refOf(makeApp('web'))).toEqual({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      namespace: 'argocd',
      name: 'web',
    });
  });
});

describe('fetchArgo', () => {
  it('reads the dashboard', async () => {
    stub({ apps: [makeApp('web')], applicationSets: [] });

    const found = await fetchArgo();

    expect(found.apps).toHaveLength(1);
    expect(found.apps[0].name).toBe('web');
  });

  it('fills in what the backend left out', async () => {
    stub({ apps: [{ name: 'bare' }] });

    const found = await fetchArgo();

    expect(found.apps[0]).toMatchObject({ name: 'bare', sync: '', health: '', project: '' });
    expect(found.applicationSets).toEqual([]);
  });

  it('carries a partial failure', async () => {
    stub({ apps: [], applicationSets: [], error: 'applications is forbidden' });

    expect((await fetchArgo()).error).toBe('applications is forbidden');
  });

  it('reports a request the backend refused', async () => {
    stub({ message: 'spinoza has no cluster' }, false, 503);

    await expect(fetchArgo()).rejects.toThrow('no cluster');
  });
});

describe('tree', () => {
  it('keeps standalone apps flat', () => {
    const rows = tree([makeApp('a'), makeApp('b')]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([
      ['a', 0],
      ['b', 0],
    ]);
  });

  it('nests an app of apps under its parent', () => {
    const rows = tree([
      makeApp('root'),
      makeApp('web', { owner: 'root' }),
      makeApp('api', { owner: 'root' }),
    ]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([
      ['root', 0],
      ['web', 1],
      ['api', 1],
    ]);
  });

  it('nests deeper than one level', () => {
    const rows = tree([
      makeApp('root'),
      makeApp('team', { owner: 'root' }),
      makeApp('web', { owner: 'team' }),
    ]);

    expect(rows.map((row) => row.depth)).toEqual([0, 1, 2]);
  });

  it('still lists an app whose parent is missing', () => {
    const rows = tree([makeApp('orphan', { owner: 'gone' })]);

    expect(rows.map((row) => [row.app.name, row.depth])).toEqual([['orphan', 0]]);
  });

  it('does not loop when two apps own each other', () => {
    const rows = tree([makeApp('a', { owner: 'b' }), makeApp('b', { owner: 'a' })]);

    expect(rows).toHaveLength(2);
  });

  it('has nothing to show for no apps', () => {
    expect(tree([])).toEqual([]);
  });
});
