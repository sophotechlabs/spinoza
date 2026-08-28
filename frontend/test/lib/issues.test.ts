import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import type { Issue } from '../../src/lib/types';
import {
  countBySeverity,
  fetchIssues,
  foldedLabel,
  hiddenChildren,
  severityClass,
  severityLabel,
  useIssues,
} from '../../src/lib/issues';
import { anySignal } from '../helpers';

function issue(patch: Partial<Issue> = {}): Issue {
  return {
    id: 'pod-startup/uid-web',
    severity: 'fatal',
    detector: 'pod-startup',
    title: 'CrashLoopBackOff',
    detail: 'container app keeps exiting with exit code 1',
    action: "read the container's logs",
    object: {
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'web',
      name: 'api',
    },
    kind: 'Deployment',
    since: '2026-08-28T11:00:00Z',
    folded: 0,
    ...patch,
  };
}

const payload = {
  rows: [
    {
      id: 'pod-startup/uid-web',
      severity: 'fatal',
      detector: 'pod-startup',
      title: 'CrashLoopBackOff',
      detail: 'container app keeps exiting',
      action: 'read the logs',
      change: 'revision 4',
      changedAt: '2026-08-28T10:00:00Z',
      uncertain: false,
      object: {
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'web',
        name: 'api',
      },
      kind: 'Deployment',
      since: '2026-08-28T11:00:00Z',
      folded: 3,
      children: [
        {
          object: { group: '', version: 'v1', resource: 'pods', namespace: 'web', name: 'api-1' },
          kind: 'Pod',
          severity: 'fatal',
          detail: 'container app keeps exiting',
          since: '2026-08-28T11:00:00Z',
        },
      ],
    },
  ],
  dropped: 2,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchIssues', () => {
  it('requests /api/issues and parses what comes back', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchIssues();

    expect(fetchMock).toHaveBeenCalledWith('/api/issues', { signal: anySignal() });
    expect(got.rows[0].title).toBe('CrashLoopBackOff');
    expect(got.rows[0].change).toBe('revision 4');
    expect(got.rows[0].children?.[0].object.name).toBe('api-1');
    expect(got.dropped).toBe(2);
  });

  it('reads a payload missing every field without throwing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );

    const got = await fetchIssues();

    expect(got.rows).toEqual([]);
    expect(got.dropped).toBe(0);
    expect(got.error).toBeUndefined();
  });

  it('falls back to a warning when the severity is not one it knows', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            rows: [{ severity: 'catastrophic', children: [{ severity: 'nonsense' }] }],
          }),
      }),
    );

    const got = await fetchIssues();

    expect(got.rows[0].severity).toBe('warning');
    expect(got.rows[0].children?.[0].severity).toBe('warning');
  });

  it('reports a failed status', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchIssues()).rejects.toThrow('issues request failed with status 503');
  });
});

describe('useIssues', () => {
  it('polls once the view is mounted', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(payload) }),
    );

    const { result } = renderHook(() => useIssues());

    await waitFor(() => {
      expect(result.current.data?.rows[0].title).toBe('CrashLoopBackOff');
    });
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    renderHook(() => useIssues(false));

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('severity', () => {
  it('names each level in words', () => {
    expect(severityLabel('fatal')).toBe('Broken');
    expect(severityLabel('degraded')).toBe('Degraded');
    expect(severityLabel('warning')).toBe('Warning');
  });

  it('colours each level', () => {
    expect(severityClass('fatal')).toBe('text-error');
    expect(severityClass('degraded')).toBe('text-warn');
    expect(severityClass('warning')).toBe('text-fg-muted');
  });

  it('counts the rows at each level', () => {
    const counted = countBySeverity([
      issue(),
      issue({ severity: 'degraded' }),
      issue({ severity: 'warning' }),
      issue({ severity: 'warning' }),
    ]);

    expect(counted).toEqual({ fatal: 1, degraded: 1, warning: 2 });
  });
});

describe('the fold', () => {
  it('says nothing when a row folded nothing', () => {
    expect(foldedLabel(issue())).toBe('');
  });

  it('counts one object in the singular', () => {
    expect(foldedLabel(issue({ folded: 1 }))).toBe('1 object');
  });

  it('counts several objects', () => {
    expect(foldedLabel(issue({ folded: 200 }))).toBe('200 objects');
  });

  it('reports how many children the payload left out', () => {
    const row = issue({
      folded: 200,
      children: [
        {
          object: { group: '', version: 'v1', resource: 'pods', namespace: 'web', name: 'api-1' },
          kind: 'Pod',
          severity: 'fatal',
          detail: 'crashing',
          since: '2026-08-28T11:00:00Z',
        },
      ],
    });

    expect(hiddenChildren(row)).toBe(199);
  });

  it('reports nothing hidden when every child is listed', () => {
    expect(hiddenChildren(issue({ folded: 0 }))).toBe(0);
  });

  it('reports nothing hidden when a row has no child list at all', () => {
    expect(hiddenChildren(issue({ folded: 0, children: undefined }))).toBe(0);
  });
});
