import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  fetchFleetImages,
  fetchFleetInventory,
  fetchFleetOverview,
  nodesLabel,
  podsLabel,
  shortKey,
  skewLabel,
} from '../../src/lib/fleet';

function stub(body: unknown, ok = true, status = 200) {
  const fetcher = vi.fn((url: string) => {
    void url;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('the fleet overview', () => {
  it('reads back what every cluster reported', async () => {
    stub({
      clusters: [{ cluster: 'a', context: 'p-mk1', version: 'v1.34.1' }],
      nodes: { total: 6, ready: 6 },
      pods: { total: 60, running: 57, known: true },
    });

    const got = await fetchFleetOverview();

    expect(got.clusters).toHaveLength(1);
    expect(got.nodes.total).toBe(6);
  });

  it('fills in what the server left out', async () => {
    stub({});

    const got = await fetchFleetOverview();

    expect(got.clusters).toEqual([]);
    expect(got.nodes.total).toBe(0);
    expect(got.pods.known).toBe(false);
  });

  it('reports a failure with its status', async () => {
    stub({ message: 'no' }, false, 500);

    await expect(fetchFleetOverview()).rejects.toThrow();
  });
});

describe('the fleet inventory', () => {
  it('reads the kinds back', async () => {
    stub({ kinds: [{ key: '/v1/pods', total: 60, perCluster: {} }] });

    expect((await fetchFleetInventory()).kinds).toHaveLength(1);
  });

  it('fills in an empty answer', async () => {
    stub({});

    expect((await fetchFleetInventory()).kinds).toEqual([]);
  });

  it('reports a failure', async () => {
    stub({}, false, 503);

    await expect(fetchFleetInventory()).rejects.toThrow();
  });
});

describe('the fleet images', () => {
  it('reads the images back', async () => {
    stub({ images: [{ image: 'nginx:1.27', repo: 'nginx', pods: 3, clusters: ['a'] }] });

    expect((await fetchFleetImages()).images).toHaveLength(1);
  });

  it('fills in an empty answer', async () => {
    stub({});

    expect((await fetchFleetImages()).images).toEqual([]);
  });

  it('reports a failure', async () => {
    stub({}, false, 500);

    await expect(fetchFleetImages()).rejects.toThrow();
  });
});

describe('what the rows say', () => {
  it('reads nodes as ready out of total', () => {
    expect(nodesLabel({ ready: 3, total: 4 } as never)).toBe('3/4');
  });

  it('reads pods as running out of total', () => {
    expect(podsLabel({ running: 39, total: 40, known: true } as never)).toBe('39/40');
  });

  it('says nothing about pods a cluster could not count', () => {
    expect(podsLabel({ running: 0, total: 0, known: false } as never)).toBe('—');
  });

  it('shortens a resource key to its kind', () => {
    expect(shortKey('apps/v1/deployments')).toBe('deployments');
    expect(shortKey('pods')).toBe('pods');
  });

  it('names the drift when a repo runs at two tags', () => {
    expect(skewLabel(['1.25', '1.27'])).toBe('1.25 · 1.27');
  });

  it('says nothing when a repo runs at one tag or none', () => {
    expect(skewLabel(['1.27'])).toBe('');
    expect(skewLabel(undefined)).toBe('');
  });
});
