import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  HISTORY_LIMIT,
  clearFailure,
  detailText,
  fetchHistory,
  forgetHistory,
  outcomeClass,
  outcomeLabel,
  refOf,
  scopeLabel,
  cursorOf,
  fetchMemory,
  foldRepeats,
  olderFailure,
  reachable,
  recordFailure,
  repeatLabel,
  sourceLabel,
  targetLabel,
  verbLabel,
  wasText,
  when,
} from '../../src/lib/history';
import type { History, HistoryEntry } from '../../src/lib/types';
import { clock } from '../../src/lib/time';

function entry(extra: Partial<HistoryEntry> = {}): HistoryEntry {
  return {
    id: 1,
    source: 'action',
    at: '2026-08-29T09:30:00Z',
    verb: 'delete',
    name: 'web',
    outcome: 'done',
    ...extra,
  };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    void url;
    void init;
    return Promise.resolve({
      ok,
      status,
      json: () => Promise.resolve(body),
      text: () => Promise.resolve(JSON.stringify(body)),
    });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('outcomeLabel', () => {
  it('names each outcome a person can read', () => {
    expect(outcomeLabel('done')).toBe('Done');
    expect(outcomeLabel('refused')).toBe('Refused');
    expect(outcomeLabel('failed')).toBe('Failed');
  });

  it('falls back to done for anything it does not know', () => {
    expect(outcomeLabel('something-new')).toBe('Done');
  });
});

describe('outcomeClass', () => {
  it('colours a refusal as a warning and a failure as an error', () => {
    expect(outcomeClass('refused')).toBe('text-warn');
    expect(outcomeClass('failed')).toBe('text-error');
    expect(outcomeClass('done')).toBe('text-fg-muted');
  });

  it('leaves an unknown outcome unremarkable', () => {
    expect(outcomeClass('something-new')).toBe('text-fg-muted');
  });
});

describe('targetLabel', () => {
  it('prefers the kind', () => {
    expect(targetLabel(entry({ kind: 'Deployment', resource: 'deployments' }))).toBe(
      'Deployment web',
    );
  });

  it('falls back to the resource when there is no kind', () => {
    expect(targetLabel(entry({ resource: 'deployments' }))).toBe('deployments web');
  });

  it('is just the name when neither is known', () => {
    expect(targetLabel(entry())).toBe('web');
  });
});

describe('scopeLabel', () => {
  it('names the namespace', () => {
    expect(scopeLabel(entry({ namespace: 'default' }))).toBe('default');
  });

  it('says cluster-wide when there is no namespace', () => {
    expect(scopeLabel(entry())).toBe('cluster-wide');
    expect(scopeLabel(entry({ namespace: '' }))).toBe('cluster-wide');
  });
});

describe('fetchHistory', () => {
  it('asks for the newest page and parses it', async () => {
    const body: History = {
      entries: [
        { id: 4, source: 'action', at: 'now', verb: 'apply', name: 'web', outcome: 'done' },
      ],
      more: true,
    };
    const fetcher = stub(body);

    const got = await fetchHistory();

    expect(got.entries).toHaveLength(1);
    expect(got.entries[0].id).toBe(4);
    expect(got.more).toBe(true);
    expect(fetcher.mock.calls[0][0]).toContain(`limit=${String(HISTORY_LIMIT)}`);
  });

  it('reports a failure with its status', async () => {
    stub({ message: 'no' }, false, 500);

    await expect(fetchHistory()).rejects.toThrow();
  });

  it('asks for everything unless told otherwise', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory();

    expect(fetcher.mock.calls[0][0]).toContain('source=all');
  });

  it('asks for only what it was told to', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory({ source: 'change' });

    expect(fetcher.mock.calls[0][0]).toContain('source=change');
  });

  it('carries how many changes were not kept', async () => {
    stub({ entries: [], dropped: 12 });

    const got = await fetchHistory({ source: 'change' });

    expect(got.dropped).toBe(12);
  });
});

describe('recordFailure', () => {
  it('says what failed and why', () => {
    expect(recordFailure(new Error('nowhere to keep that'))).toBe(
      'Changing what is recorded: nowhere to keep that',
    );
  });

  it('says what failed when there is no why', () => {
    expect(recordFailure('nope')).toBe('Changing what is recorded failed');
  });
});

describe('folding repeats', () => {
  it('keeps one row per object and counts the rest', () => {
    const folded = foldRepeats([
      entry({ id: 3, source: 'change', name: 'calico-node', namespace: 'kube-system' }),
      entry({ id: 2, source: 'change', name: 'calico-node', namespace: 'kube-system' }),
      entry({ id: 1, source: 'change', name: 'calico-node', namespace: 'kube-system' }),
    ]);

    expect(folded).toHaveLength(1);
    expect(folded[0].repeats).toBe(3);
    expect(folded[0].entry.id).toBe(3);
    expect(folded[0].oldest.id).toBe(1);
  });

  it('does not fold two different objects together', () => {
    const folded = foldRepeats([
      entry({ id: 2, source: 'change', name: 'calico-node' }),
      entry({ id: 1, source: 'change', name: 'netd' }),
    ]);

    expect(folded).toHaveLength(2);
  });

  it('does not fold the same name on two clusters', () => {
    const folded = foldRepeats([
      entry({ id: 2, source: 'change', name: 'calico-node', cluster: 'one' }),
      entry({ id: 1, source: 'change', name: 'calico-node', cluster: 'two' }),
    ]);

    expect(folded).toHaveLength(2);
  });

  it('never folds what spinoza did', () => {
    const folded = foldRepeats([
      entry({ id: 2, source: 'action', name: 'web' }),
      entry({ id: 1, source: 'action', name: 'web' }),
    ]);

    expect(folded).toHaveLength(2);
  });

  it('says nothing about a row that stands alone', () => {
    const folded = foldRepeats([entry({ source: 'change', name: 'web' })]);

    expect(repeatLabel(folded[0])).toBe('');
  });

  it('counts the repeats in words', () => {
    const folded = foldRepeats([
      entry({ id: 2, source: 'change', name: 'web' }),
      entry({ id: 1, source: 'change', name: 'web' }),
    ]);

    expect(repeatLabel(folded[0])).toBe('changed 2 times');
  });
});

describe('what a change moved from', () => {
  it('reads as a move when there is a before', () => {
    expect(wasText(entry({ was: '2/2 · Running' }))).toBe('2/2 · Running → ');
  });

  it('says nothing when there is not', () => {
    expect(wasText(entry())).toBe('');
    expect(wasText(entry({ was: '' }))).toBe('');
  });

  it('points at nothing for something that went', () => {
    expect(wasText(entry({ verb: 'removed', was: '1/1 · Running' }))).toBe('');
  });

  it('shows what was there for something that went', () => {
    expect(detailText(entry({ verb: 'removed', was: '1/1 · Running', detail: '' }))).toBe(
      '1/1 · Running',
    );
  });
});

describe('asking for a page', () => {
  it('continues below a cursor', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory({ after: 40 });

    expect(fetcher.mock.calls[0][0]).toContain('after=40');
  });

  it('leaves the cursor out when there is none', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory({ after: 0 });

    expect(fetcher.mock.calls[0][0]).not.toContain('after=');
  });

  it('asks for every cluster when told to', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory({ fleet: true });

    expect(fetcher.mock.calls[0][0]).toContain('fleet=true');
  });

  it('asks for one cluster otherwise', async () => {
    const fetcher = stub({ entries: [] });

    await fetchHistory({});

    expect(fetcher.mock.calls[0][0]).not.toContain('fleet=');
  });
});

describe('what a row is called', () => {
  it('names each filter', () => {
    expect(sourceLabel('all')).toBe('Everything');
    expect(sourceLabel('change')).toBe('What changed');
    expect(sourceLabel('action')).toBe('What I did');
  });

  it('leaves what spinoza did in its own words', () => {
    expect(verbLabel(entry({ verb: 'delete' }))).toBe('delete');
  });

  it('says what the cluster did in plain words', () => {
    expect(verbLabel(entry({ source: 'change', verb: 'added' }))).toBe('appeared');
    expect(verbLabel(entry({ source: 'change', verb: 'removed' }))).toBe('went');
    expect(verbLabel(entry({ source: 'change', verb: 'changed' }))).toBe('changed');
  });
});

describe('forgetHistory', () => {
  it('asks the server to clear it', async () => {
    const fetcher = stub({});

    await forgetHistory();

    expect(fetcher.mock.calls[0][1]).toMatchObject({ method: 'DELETE' });
  });

  it('reports a refusal', async () => {
    stub({ message: 'no' }, false, 503);

    await expect(forgetHistory()).rejects.toThrow();
  });
});

describe('refOf', () => {
  it('builds a ref when the row names a resource', () => {
    expect(refOf(entry({ group: 'apps', version: 'v1', resource: 'deployments' }))).toEqual({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: '',
      name: 'web',
    });
  });

  it('keeps the namespace the row carried', () => {
    expect(refOf(entry({ resource: 'deployments', namespace: 'default' }))?.namespace).toBe(
      'default',
    );
  });

  it('has nothing to open for a row with no resource', () => {
    expect(refOf(entry())).toBeNull();
    expect(refOf(entry({ resource: '' }))).toBeNull();
  });
});

describe('detailText', () => {
  it('prefers the failure message', () => {
    expect(detailText(entry({ detail: 'to 3 replicas', message: 'forbidden' }))).toBe('forbidden');
  });

  it('falls back to the detail', () => {
    expect(detailText(entry({ detail: 'to 3 replicas' }))).toBe('to 3 replicas');
    expect(detailText(entry({ detail: 'to 3 replicas', message: '' }))).toBe('to 3 replicas');
  });

  it('is empty when there is neither', () => {
    expect(detailText(entry())).toBe('');
  });
});

describe('clearFailure', () => {
  it('repeats what the error said', () => {
    expect(clearFailure(new Error('the database is read-only'))).toBe(
      'Clearing history: the database is read-only',
    );
  });

  it('still says something when it was handed anything else', () => {
    expect(clearFailure('not an error')).toBe('Clearing history failed');
    expect(clearFailure(undefined)).toBe('Clearing history failed');
  });
});

describe('when', () => {
  const noon = new Date('2026-08-29T12:00:00Z').getTime();

  it('shows only the clock for something from today', () => {
    const at = new Date('2026-08-29T09:30:00Z').toISOString();

    expect(when(at, noon)).toBe(clock(at));
  });

  it('adds the day for something from another day', () => {
    const at = new Date('2026-08-27T09:30:00Z').toISOString();

    const got = when(at, noon);

    expect(got).toContain(clock(at));
    expect(got).toMatch(/^\d{2}-\d{2} /);
  });

  it('pads a single-digit month and day', () => {
    const at = new Date('2026-01-05T09:30:00Z').toISOString();

    expect(when(at, noon).startsWith('01-05 ')).toBe(true);
  });

  it('says nothing for a stamp it cannot read', () => {
    expect(when('not a time', noon)).toBe('');
    expect(when('', noon)).toBe('');
  });
});

describe('reaching back through the pages', () => {
  it('starts from the cursor the newest page ended on', () => {
    expect(cursorOf({ entries: [], next: 40 }, [])).toBe(40);
  });

  it('continues from the last row it already holds', () => {
    expect(cursorOf({ entries: [], next: 40 }, [entry({ id: 9 })])).toBe(9);
  });

  it('says there is nothing to reach when the page named no cursor', () => {
    expect(reachable({ entries: [] }, [])).toBe(false);
  });

  it('offers to reach back while the first page said there was more', () => {
    expect(reachable({ entries: [], more: true, next: 40 }, [])).toBe(true);
  });

  it('stops offering once a page came back short', () => {
    expect(reachable({ entries: [], more: true, next: 40 }, [entry({ id: 9 })])).toBe(false);
  });

  it('says what failed when reaching back does', () => {
    expect(olderFailure(new Error('the store went away'))).toBe(
      'Reaching further back: the store went away',
    );
    expect(olderFailure('nope')).toBe('Reaching further back failed');
  });
});

describe('what spinoza is holding', () => {
  it('reads the numbers back', async () => {
    stub({ heapMi: 325, sysMi: 689 });

    const got = await fetchMemory();

    expect(got.heapMi).toBe(325);
    expect(got.sysMi).toBe(689);
  });

  it('fills in what the server left out', async () => {
    stub({});

    const got = await fetchMemory();

    expect(got.heapMi).toBe(0);
  });

  it('reports a failure with its status', async () => {
    stub({}, false, 500);

    await expect(fetchMemory()).rejects.toThrow();
  });
});
