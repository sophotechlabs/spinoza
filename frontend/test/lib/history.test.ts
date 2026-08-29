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
  targetLabel,
  when,
} from '../../src/lib/history';
import type { History, HistoryEntry } from '../../src/lib/types';
import { clock } from '../../src/lib/time';

function entry(extra: Partial<HistoryEntry> = {}): HistoryEntry {
  return {
    id: 1,
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
      entries: [{ id: 4, at: 'now', verb: 'apply', name: 'web', outcome: 'done' }],
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
