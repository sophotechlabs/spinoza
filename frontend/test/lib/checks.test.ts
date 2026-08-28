import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  bySeverity,
  countLabel,
  fetchChecks,
  findingLabel,
  inCategory,
  refLabel,
  severityClass,
  shownLabel,
  totalFindings,
  useChecks,
} from '../../src/lib/checks';
import type { CheckFindingView, CheckGroupView, CheckReportView } from '../../src/lib/checks';
import type { CheckObject, ObjectRef } from '../../src/lib/types';
import { useSettingsStore } from '../../src/store/settings';

function ref(name: string, namespace = 'apps'): ObjectRef {
  return { group: 'apps', version: 'v1', resource: 'deployments', namespace, name };
}

function wireObject(name: string): CheckObject {
  return {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    namespace: 'apps',
    name,
    kind: 'Deployment',
  };
}

function viewFinding(name: string, extra: Partial<CheckFindingView> = {}): CheckFindingView {
  return {
    object: ref(name),
    kind: 'Deployment',
    detail: 'securityContext.privileged is true',
    ...extra,
  };
}

function viewGroup(id: string, extra: Partial<CheckGroupView> = {}): CheckGroupView {
  const findings = extra.findings ?? [];
  return {
    id,
    title: 'Privileged containers',
    category: 'security',
    severity: 'high',
    wrong: 'it holds every capability on the node',
    remedy: 'drop it',
    total: findings.length,
    ...extra,
    findings,
  };
}

function viewReport(groups: CheckGroupView[]): CheckReportView {
  return { groups, scanned: groups.length };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useSettingsStore.getState().setChecksInterval(60);
});

describe('fetchChecks', () => {
  it('resolves each finding against the object dictionary', async () => {
    stub({
      scanned: 4,
      objects: [wireObject('api'), wireObject('web')],
      groups: [
        {
          id: 'privileged-containers',
          title: 'Privileged containers',
          category: 'security',
          severity: 'high',
          frameworks: ['PSS baseline'],
          wrong: 'it holds every capability',
          remedy: 'drop it',
          total: 2,
          findings: [
            { ref: 1, container: 'app', detail: 'privileged is true', patch: 'spec:\n' },
            { ref: 0, detail: 'privileged is true' },
          ],
        },
      ],
    });

    const found = await fetchChecks();

    expect(found.scanned).toBe(4);
    expect(found.groups[0].findings[0].object.name).toBe('web');
    expect(found.groups[0].findings[0].kind).toBe('Deployment');
    expect(found.groups[0].findings[1].object.name).toBe('api');
    expect(found.groups[0].total).toBe(2);
  });

  it('carries the truncation flag through', async () => {
    stub({
      objects: [wireObject('api')],
      groups: [{ id: 'limits-missing', total: 7087, truncated: true, findings: [{ ref: 0 }] }],
    });

    const found = await fetchChecks();

    expect(found.groups[0].total).toBe(7087);
    expect(found.groups[0].truncated).toBe(true);
    expect(found.groups[0].findings).toHaveLength(1);
  });

  it('falls back to the finding count when the backend sends no total', async () => {
    stub({
      objects: [wireObject('api')],
      groups: [{ id: 'a', findings: [{ ref: 0 }, { ref: 0 }] }],
    });

    expect((await fetchChecks()).groups[0].total).toBe(2);
  });

  it('survives a ref that points outside the dictionary', async () => {
    stub({ objects: [], groups: [{ id: 'a', findings: [{ ref: 9 }, {}] }] });

    const found = await fetchChecks();

    expect(found.groups[0].findings[0].object.name).toBe('');
    expect(found.groups[0].findings[0].kind).toBe('');
    expect(found.groups[0].findings[1].object.name).toBe('');
  });

  it('fills in everything the backend left out', async () => {
    stub({ groups: [{ findings: [{ ref: 0 }] }] });

    const found = await fetchChecks();

    expect(found.scanned).toBe(0);
    expect(found.groups[0].id).toBe('');
    expect(found.groups[0].wrong).toBe('');
    expect(found.groups[0].remedy).toBe('');
    expect(found.groups[0].category).toBe('reliability');
    expect(found.groups[0].severity).toBe('low');
    expect(found.groups[0].findings[0].detail).toBe('');
  });

  it('reads an object the backend only half sent', async () => {
    stub({ objects: [{ name: 'api' }], groups: [{ id: 'a', findings: [{ ref: 0 }] }] });

    const found = await fetchChecks();

    expect(found.groups[0].findings[0].object).toEqual({
      group: '',
      version: '',
      resource: '',
      namespace: '',
      name: 'api',
    });
  });

  it('keeps a report with no groups at all', async () => {
    stub({});

    expect(await fetchChecks()).toEqual({ groups: [], scanned: 0, error: undefined });
  });

  it('refuses a category and a severity it does not know', async () => {
    stub({ groups: [{ category: 'made up', severity: 'apocalyptic' }] });

    const found = await fetchChecks();

    expect(found.groups[0].category).toBe('reliability');
    expect(found.groups[0].severity).toBe('low');
  });

  it('reports a request the backend refused', async () => {
    stub({ message: 'no cluster' }, false, 503);

    await expect(fetchChecks()).rejects.toThrow('no cluster');
  });

  it('reports a refusal with no message of its own', async () => {
    stub(null, false, 500);

    await expect(fetchChecks()).rejects.toThrow('the checks request failed with status 500');
  });
});

describe('useChecks', () => {
  it('loads the report', async () => {
    const fetchMock = stub({ groups: [], objects: [], scanned: 0 });

    const { result } = renderHook(() => useChecks());

    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/checks', expect.anything());
  });

  it('refreshes on the interval the settings hold', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const fetchMock = stub({ groups: [], objects: [], scanned: 0 });
    act(() => {
      useSettingsStore.getState().setChecksInterval(15);
    });

    renderHook(() => useChecks());
    await vi.advanceTimersByTimeAsync(16000);
    const quick = fetchMock.mock.calls.length;
    await vi.advanceTimersByTimeAsync(16000);

    expect(fetchMock.mock.calls.length).toBeGreaterThan(quick);
    vi.useRealTimers();
  });

  it('defaults to a minute', () => {
    expect(useSettingsStore.getState().checksInterval).toBe(60);
  });
});

describe('grouping', () => {
  it('counts every finding the cluster has, not the ones that fit', () => {
    const report = viewReport([
      viewGroup('a', { findings: [viewFinding('api')], total: 7087 }),
      viewGroup('b', { findings: [viewFinding('db')] }),
    ]);

    expect(totalFindings(report)).toBe(7088);
  });

  it('puts the worst first and the busiest first within a severity', () => {
    const sorted = bySeverity([
      viewGroup('low', { severity: 'low' }),
      viewGroup('medium-quiet', { severity: 'medium', total: 1 }),
      viewGroup('medium-busy', { severity: 'medium', total: 900 }),
      viewGroup('high', { severity: 'high' }),
    ]);

    expect(sorted.map((group) => group.id)).toEqual(['high', 'medium-busy', 'medium-quiet', 'low']);
  });

  it('leaves the list it was given alone', () => {
    const groups = [viewGroup('low', { severity: 'low' }), viewGroup('high', { severity: 'high' })];

    bySeverity(groups);

    expect(groups.map((group) => group.id)).toEqual(['low', 'high']);
  });

  it('keeps only one category', () => {
    const groups = [
      viewGroup('a', { category: 'security' }),
      viewGroup('b', { category: 'efficiency' }),
    ];

    expect(inCategory(groups, 'efficiency').map((group) => group.id)).toEqual(['b']);
  });

  it('names every category it orders', () => {
    for (const category of CATEGORY_ORDER) {
      expect(CATEGORY_LABELS[category]).not.toBe('');
    }
  });
});

describe('labels', () => {
  it('colours a finding by how bad it is', () => {
    expect(severityClass('high')).toBe('text-error');
    expect(severityClass('medium')).toBe('text-warn');
    expect(severityClass('low')).toBe('text-fg-muted');
  });

  it('counts what the cluster has, not what was sent', () => {
    expect(countLabel(viewGroup('a'))).toBe('clean');
    expect(countLabel(viewGroup('a', { findings: [viewFinding('api')] }))).toBe('1');
    expect(countLabel(viewGroup('a', { total: 7087 }))).toBe('7087');
    expect(countLabel(viewGroup('a', { skipped: 'no metrics' }))).toBe('no data');
  });

  it('says how much of a capped group is on screen', () => {
    const capped = viewGroup('a', { findings: [viewFinding('api')], total: 7087, truncated: true });

    expect(shownLabel(capped)).toBe('Showing 1 of 7087.');
    expect(shownLabel(viewGroup('a', { findings: [viewFinding('api')] }))).toBe('');
  });

  it('names the object a finding landed on', () => {
    expect(findingLabel(viewFinding('api'))).toBe('Deployment · apps/api');
    expect(findingLabel(viewFinding('api', { container: 'app' }))).toBe(
      'Deployment · apps/api · container app',
    );
    expect(findingLabel(viewFinding('api', { container: '' }))).toBe('Deployment · apps/api');
  });

  it('drops the namespace from a cluster-scoped object', () => {
    expect(refLabel(ref('node-1', ''))).toBe('node-1');
  });
});
