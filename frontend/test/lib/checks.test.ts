import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
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
  totalFindings,
  useChecks,
} from '../../src/lib/checks';
import type { CheckFinding, CheckGroup, CheckReport, ObjectRef } from '../../src/lib/types';

function ref(name: string, namespace = 'apps'): ObjectRef {
  return { group: 'apps', version: 'v1', resource: 'deployments', namespace, name };
}

function makeFinding(name: string, extra: Partial<CheckFinding> = {}): CheckFinding {
  return {
    object: ref(name),
    kind: 'Deployment',
    detail: 'securityContext.privileged is true',
    ...extra,
  };
}

function makeGroup(id: string, extra: Partial<CheckGroup> = {}): CheckGroup {
  return {
    id,
    title: 'Privileged containers',
    category: 'security',
    severity: 'high',
    wrong: 'it holds every capability on the node',
    remedy: 'drop it',
    findings: [],
    ...extra,
  };
}

function makeReport(groups: CheckGroup[], extra: Partial<CheckReport> = {}): CheckReport {
  return { groups, scanned: groups.length, ...extra };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn(() => Promise.resolve({ ok, status, json: () => Promise.resolve(body) }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchChecks', () => {
  it('reads a report the backend sent', async () => {
    stub({
      scanned: 4,
      groups: [
        {
          id: 'privileged-containers',
          title: 'Privileged containers',
          category: 'security',
          severity: 'high',
          frameworks: ['PSS baseline'],
          wrong: 'it holds every capability',
          remedy: 'drop it',
          findings: [
            {
              object: {
                group: 'apps',
                version: 'v1',
                resource: 'deployments',
                namespace: 'apps',
                name: 'api',
              },
              kind: 'Deployment',
              container: 'app',
              detail: 'securityContext.privileged is true',
              patch: 'spec:\n',
            },
          ],
        },
      ],
    });

    const found = await fetchChecks();

    expect(found.scanned).toBe(4);
    expect(found.groups[0].findings[0].object.name).toBe('api');
    expect(found.groups[0].findings[0].container).toBe('app');
    expect(found.groups[0].frameworks).toEqual(['PSS baseline']);
  });

  it('fills in everything the backend left out', async () => {
    stub({ groups: [{ findings: [{}] }] });

    const found = await fetchChecks();

    expect(found.scanned).toBe(0);
    expect(found.groups[0].id).toBe('');
    expect(found.groups[0].title).toBe('');
    expect(found.groups[0].wrong).toBe('');
    expect(found.groups[0].remedy).toBe('');
    expect(found.groups[0].category).toBe('reliability');
    expect(found.groups[0].severity).toBe('low');
    expect(found.groups[0].findings[0]).toEqual({
      object: { group: '', version: '', resource: '', namespace: '', name: '' },
      kind: '',
      container: undefined,
      detail: '',
      patch: undefined,
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
  it('loads the report and keeps polling', async () => {
    const fetchMock = stub(makeReport([makeGroup('privileged-containers')]));

    const { result } = renderHook(() => useChecks());

    await waitFor(() => {
      expect(result.current.data).not.toBeNull();
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/checks', expect.anything());
  });
});

describe('grouping', () => {
  it('counts every finding in the report', () => {
    const report = makeReport([
      makeGroup('a', { findings: [makeFinding('api'), makeFinding('web')] }),
      makeGroup('b', { findings: [makeFinding('db')] }),
    ]);

    expect(totalFindings(report)).toBe(3);
  });

  it('puts the worst first and the busiest first within a severity', () => {
    const sorted = bySeverity([
      makeGroup('low', { severity: 'low' }),
      makeGroup('medium-quiet', { severity: 'medium', findings: [makeFinding('api')] }),
      makeGroup('medium-busy', {
        severity: 'medium',
        findings: [makeFinding('api'), makeFinding('web')],
      }),
      makeGroup('high', { severity: 'high' }),
    ]);

    expect(sorted.map((group) => group.id)).toEqual(['high', 'medium-busy', 'medium-quiet', 'low']);
  });

  it('leaves the list it was given alone', () => {
    const groups = [makeGroup('low', { severity: 'low' }), makeGroup('high', { severity: 'high' })];

    bySeverity(groups);

    expect(groups.map((group) => group.id)).toEqual(['low', 'high']);
  });

  it('keeps only one category', () => {
    const groups = [
      makeGroup('a', { category: 'security' }),
      makeGroup('b', { category: 'efficiency' }),
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

  it('says what a check found, or that it found nothing', () => {
    expect(countLabel(makeGroup('a'))).toBe('clean');
    expect(countLabel(makeGroup('a', { findings: [makeFinding('api')] }))).toBe('1');
    expect(countLabel(makeGroup('a', { skipped: 'no metrics' }))).toBe('no data');
  });

  it('names the object a finding landed on', () => {
    expect(findingLabel(makeFinding('api'))).toBe('Deployment · apps/api');
    expect(findingLabel(makeFinding('api', { container: 'app' }))).toBe(
      'Deployment · apps/api · container app',
    );
    expect(findingLabel(makeFinding('api', { container: '' }))).toBe('Deployment · apps/api');
  });

  it('drops the namespace from a cluster-scoped object', () => {
    expect(refLabel(ref('node-1', ''))).toBe('node-1');
  });
});
