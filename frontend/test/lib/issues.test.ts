import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { Issue } from '../../src/lib/types';
import { SEVERITIES } from '../../src/lib/types';
import {
  countBySeverity,
  fetchIssues,
  foldedLabel,
  hiddenChildren,
  severityClass,
  severityLabel,
  tallyCounts,
  tallyScope,
  useIssues,
  usePagedIssues,
} from '../../src/lib/issues';
import type { IssueOrder } from '../../src/lib/issues';
import { anySignal, rejectsWith } from '../helpers';

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

    expect(fetchMock).toHaveBeenCalledWith('/api/issues?sort=worst', { signal: anySignal() });
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

  it('reads the whole-queue tally the server sends', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({ rows: [], tally: { fatal: 111, degraded: 2, warning: 3, total: 116 } }),
      }),
    );

    const got = await fetchIssues();

    expect(got.tally).toEqual({ fatal: 111, degraded: 2, warning: 3, total: 116 });
  });

  it('reads a payload whose tally is absent or null', async () => {
    for (const body of [{ rows: [] }, { rows: [], tally: null }]) {
      vi.stubGlobal(
        'fetch',
        vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) }),
      );

      const got = await fetchIssues();

      expect(got.tally).toBeUndefined();
    }
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

    expect(counted).toEqual({ fatal: 1, degraded: 1, warning: 2, info: 0 });
  });

  it('carries an info bucket, because the wire has four levels and Issues use three', () => {
    const counted = countBySeverity([issue({ severity: 'info' })]);

    expect(counted.info).toBe(1);
  });

  it('names and colours every level the wire can send', () => {
    for (const severity of SEVERITIES) {
      expect(severityLabel(severity)).not.toBe('');
      expect(severityClass(severity)).not.toBe('');
    }
    expect(severityLabel('info')).toBe('Note');
    expect(severityLabel('info')).not.toBe(severityLabel('warning'));
    expect(severityClass('info')).not.toBe(severityClass('warning'));
  });
});

describe('what the tally is counting', () => {
  it('says nothing while the whole queue is loaded', () => {
    expect(tallyScope(12, false, undefined)).toBe('');
  });

  it('names how many rows it counted while the server sent no tally', () => {
    expect(tallyScope(50, true, undefined)).toBe('of the 50 loaded so far');
  });

  it('counts the whole cluster and says how much of it is on screen', () => {
    const whole = { fatal: 111, degraded: 0, warning: 0, total: 111 };

    expect(tallyCounts([], whole)).toEqual({ fatal: 111, degraded: 0, warning: 0, info: 0 });
    expect(tallyScope(50, true, whole)).toBe('50 of 111 on screen');
  });

  it('falls back to the rows it holds when the server sent no tally', () => {
    const rows = [issue({ severity: 'fatal' }), issue({ severity: 'warning' })];

    expect(tallyCounts(rows, undefined)).toEqual({ fatal: 1, degraded: 0, warning: 1, info: 0 });
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

function pagesStub(pages: Record<string, unknown>): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((url: string) => {
    const after = new URL(url, 'http://localhost').searchParams.get('after') ?? '';
    const body = pages[after];
    if (body === undefined) {
      return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve({}) });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('changing the sort while pages are loaded', () => {
  function sortedStub() {
    return vi.fn((url: string) => {
      const asked = new URL(url, 'http://localhost').searchParams;
      const sort = asked.get('sort') ?? '';
      const after = asked.get('after') ?? '';
      if (sort === 'worst' && after === '') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'w1' })], dropped: 0, next: 'wc1' }),
        });
      }
      if (sort === 'worst' && after === 'wc1') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'w2' })], dropped: 0 }),
        });
      }
      if (sort === 'newest' && after === '') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'n1' })], dropped: 0, next: 'nc1' }),
        });
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ rows: [issue({ id: 'wrong' })], dropped: 0 }),
      });
    });
  }

  it('never carries one order\u2019s cursor into another', async () => {
    const fetchMock = sortedStub();
    vi.stubGlobal('fetch', fetchMock);
    const { result, rerender } = renderHook(
      ({ order }: { order: IssueOrder }) => usePagedIssues(true, false, order),
      { initialProps: { order: 'worst' as IssueOrder } },
    );
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['w1', 'w2']);
    });

    rerender({ order: 'newest' as IssueOrder });

    expect(result.current.more).toBe('');
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['n1']);
    });
    expect(result.current.more).toBe('nc1');
    const asked = fetchMock.mock.calls.map((call) => call[0]);
    expect(asked.some((url) => url.includes('sort=newest') && url.includes('after=wc1'))).toBe(
      false,
    );
  });

  it('ignores a page from the previous order when it lands late', async () => {
    let landOldPage: (body: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        const asked = new URL(url, 'http://localhost').searchParams;
        const sort = asked.get('sort') ?? '';
        const after = asked.get('after') ?? '';
        if (sort === 'worst' && after === '') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ rows: [issue({ id: 'w1' })], next: 'wc1' }),
          });
        }
        if (sort === 'worst' && after === 'wc1') {
          return new Promise((resolve) => {
            landOldPage = (body) => {
              resolve({ ok: true, json: () => Promise.resolve(body) });
            };
          });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'n1' })], next: 'nc1' }),
        });
      }),
    );
    const { result, rerender } = renderHook(
      ({ order }: { order: IssueOrder }) => usePagedIssues(true, false, order),
      { initialProps: { order: 'worst' as IssueOrder } },
    );
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['w1']);
    });
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.loadingMore).toBe(true);
    });

    rerender({ order: 'newest' as IssueOrder });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['n1']);
    });
    act(() => {
      landOldPage({ rows: [issue({ id: 'w2' })] });
    });

    await waitFor(() => {
      expect(result.current.loadingMore).toBe(false);
    });
    expect(result.current.rows.map((row) => row.id)).toEqual(['n1']);
    expect(result.current.more).toBe('nc1');
  });

  it('ignores a page failure from the previous order when it lands late', async () => {
    let failOldPage: (reason?: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        const asked = new URL(url, 'http://localhost').searchParams;
        const sort = asked.get('sort') ?? '';
        const after = asked.get('after') ?? '';
        if (sort === 'worst' && after === '') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ rows: [issue({ id: 'w1' })], next: 'wc1' }),
          });
        }
        if (sort === 'worst' && after === 'wc1') {
          return new Promise((_resolve, reject) => {
            failOldPage = reject;
          });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'n1' })], next: 'nc1' }),
        });
      }),
    );
    const { result, rerender } = renderHook(
      ({ order }: { order: IssueOrder }) => usePagedIssues(true, false, order),
      { initialProps: { order: 'worst' as IssueOrder } },
    );
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['w1']);
    });
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.loadingMore).toBe(true);
    });

    rerender({ order: 'newest' as IssueOrder });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['n1']);
    });
    await act(async () => {
      failOldPage(new Error('the old page failed'));
      await Promise.resolve();
    });

    expect(result.current.moreError).toBe('');
    expect(result.current.rows.map((row) => row.id)).toEqual(['n1']);
    expect(result.current.more).toBe('nc1');
  });
});

describe('usePagedIssues', () => {
  it('joins the pages it has been asked for onto the first one', async () => {
    pagesStub({
      '': { rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' },
      c1: { rows: [issue({ id: 'two' })], dropped: 0, next: 'c2' },
      c2: { rows: [issue({ id: 'three' })], dropped: 0 },
    });

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });

    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['one', 'two']);
    });

    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['one', 'two', 'three']);
    });
    expect(result.current.more).toBe('');
  });

  it('drops the tail when the first page no longer ends where it did', async () => {
    let head = { rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' };
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        const after = new URL(url, 'http://localhost').searchParams.get('after') ?? '';
        if (after === '') {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(head) });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ rows: [issue({ id: 'tail' })], dropped: 0 }),
        });
      }),
    );

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['one', 'tail']);
    });

    head = { rows: [issue({ id: 'worse' })], dropped: 0, next: 'c9' };
    act(() => {
      result.current.reload();
    });

    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['worse']);
    });
    expect(result.current.more).toBe('c9');
  });

  it('asks the fleet endpoint for its pages when the fleet queue is showing', async () => {
    const fetchMock = pagesStub({
      '': { rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' },
      c1: { rows: [issue({ id: 'two' })], dropped: 0 },
    });

    const { result } = renderHook(() => usePagedIssues(true, true));
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(2);
    });

    expect(fetchMock.mock.calls.map((call) => String(call[0]))).toContain(
      '/api/issues/fleet?sort=worst&after=c1',
    );
  });

  it('ignores a second ask while the first is still in the air', async () => {
    let landed: (body: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        const after = new URL(url, 'http://localhost').searchParams.get('after') ?? '';
        if (after === '') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' }),
          });
        }
        return new Promise((resolve) => {
          landed = (body) => {
            resolve({ ok: true, json: () => Promise.resolve(body) });
          };
        });
      }),
    );

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
      result.current.loadMore();
    });
    await waitFor(() => {
      expect(result.current.loadingMore).toBe(true);
    });

    act(() => {
      landed({ rows: [issue({ id: 'two' })], dropped: 0 });
    });

    await waitFor(() => {
      expect(result.current.rows.map((row) => row.id)).toEqual(['one', 'two']);
    });
  });

  it('does nothing when there is no more to ask for', async () => {
    const fetchMock = pagesStub({ '': { rows: [issue({ id: 'one' })], dropped: 0 } });

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    const before = fetchMock.mock.calls.length;

    act(() => {
      result.current.loadMore();
    });

    expect(fetchMock.mock.calls).toHaveLength(before);
    expect(result.current.moreError).toBe('');
  });

  it('reports a page that will not load and keeps the ask available', async () => {
    pagesStub({ '': { rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' } });

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
    });

    await waitFor(() => {
      expect(result.current.moreError).toMatch(/status 404/);
    });
    expect(result.current.more).toBe('c1');
  });

  it('reports a rejection that is not an error as a failed request', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        const after = new URL(url, 'http://localhost').searchParams.get('after') ?? '';
        if (after === '') {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({ rows: [issue({ id: 'one' })], dropped: 0, next: 'c1' }),
          });
        }
        return rejectsWith('not an Error')();
      }),
    );

    const { result } = renderHook(() => usePagedIssues());
    await waitFor(() => {
      expect(result.current.rows).toHaveLength(1);
    });
    act(() => {
      result.current.loadMore();
    });

    await waitFor(() => {
      expect(result.current.moreError).toBe('issues request failed');
    });
  });
});
