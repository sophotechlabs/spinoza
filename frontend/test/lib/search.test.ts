import { afterEach, describe, expect, it, vi } from 'vitest';
import { SHORTEST_QUERY, refOf, searchObjects, worthSearching } from '../../src/lib/search';
import type { SearchHit } from '../../src/lib/types';

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn((url: string) => {
    void url;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const hit: SearchHit = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  namespace: 'airbyte',
  name: 'airbyte-server',
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('worthSearching', () => {
  it('waits for more than a single letter', () => {
    expect(worthSearching('a')).toBe(false);
    expect(worthSearching(' a ')).toBe(false);
    expect(worthSearching('ai')).toBe(true);
    expect(worthSearching('')).toBe(false);
  });

  it('agrees with the shortest query it accepts', () => {
    expect(worthSearching('x'.repeat(SHORTEST_QUERY))).toBe(true);
  });
});

describe('refOf', () => {
  it('turns a hit into something the inspector can open', () => {
    expect(refOf(hit)).toEqual({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'airbyte',
      name: 'airbyte-server',
    });
  });
});

describe('searchObjects', () => {
  it('asks the server for the trimmed query', async () => {
    const fetchMock = stub({ hits: [hit], truncated: false });

    const found = await searchObjects('  airbyte ');

    expect(fetchMock.mock.calls[0][0]).toContain('q=airbyte');
    expect(found.hits).toEqual([hit]);
    expect(found.truncated).toBe(false);
  });

  it('fills in what the backend left out', async () => {
    stub({ hits: [{ name: 'lonely' }] });

    const found = await searchObjects('lonely');

    expect(found.hits[0]).toEqual({
      group: '',
      version: '',
      resource: '',
      kind: '',
      namespace: '',
      name: 'lonely',
    });
    expect(found.truncated).toBe(false);
  });

  it('has no hits when the backend sent none', async () => {
    stub({});

    expect((await searchObjects('airbyte')).hits).toEqual([]);
  });

  it('passes on that the sweep was cut short', async () => {
    stub({ hits: [], truncated: true, errors: { '/v1/secrets': 'forbidden' } });

    const found = await searchObjects('airbyte');

    expect(found.truncated).toBe(true);
    expect(found.errors).toEqual({ '/v1/secrets': 'forbidden' });
  });

  it('reports a search the backend refused', async () => {
    stub({ message: 'spinoza has no cluster' }, false, 503);

    await expect(searchObjects('airbyte')).rejects.toThrow('no cluster');
  });
});
