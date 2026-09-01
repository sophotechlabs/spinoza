import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  fetchFleetGitops,
  fetchFleetImages,
  fetchFleetInventory,
  fetchFleetOverview,
  fetchFleetReleases,
  nodesLabel,
  podsLabel,
  shortKey,
  skewLabel,
  spreadLabel,
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
    stub({
      images: [{ image: 'nginx:1.27', repo: 'nginx', pods: 3, clusters: ['a'] }],
      total: 1200,
      truncated: true,
    });

    const got = await fetchFleetImages();
    expect(got.images).toHaveLength(1);
    expect(got.total).toBe(1200);
    expect(got.truncated).toBe(true);
  });

  it('fills in an empty answer', async () => {
    stub({});

    const got = await fetchFleetImages();
    expect(got.images).toEqual([]);
    expect(got.total).toBe(0);
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

describe('the fleet releases and delivery', () => {
  it('reads the releases back', async () => {
    stub({ releases: [{ name: 'loki', namespace: 'monitoring' }] });

    expect((await fetchFleetReleases()).releases).toHaveLength(1);
  });

  it('fills in an empty release answer', async () => {
    stub({});

    expect((await fetchFleetReleases()).releases).toEqual([]);
  });

  it('reports a release failure', async () => {
    stub({}, false, 500);

    await expect(fetchFleetReleases()).rejects.toThrow();
  });

  it('reads the delivered apps back', async () => {
    stub({ apps: [{ name: 'platform', cluster: 'a' }] });

    expect((await fetchFleetGitops()).apps).toHaveLength(1);
  });

  it('fills in an empty delivery answer', async () => {
    stub({});

    expect((await fetchFleetGitops()).apps).toEqual([]);
  });

  it('reports a delivery failure', async () => {
    stub({}, false, 500);

    await expect(fetchFleetGitops()).rejects.toThrow();
  });
});

describe('how far an app spreads', () => {
  it('says everywhere when it is on every open cluster', () => {
    expect(spreadLabel(3, 3)).toBe('everywhere');
  });

  it('says how many of how many when it is not', () => {
    expect(spreadLabel(2, 3)).toBe('2 of 3');
  });

  it('says nothing with one cluster open', () => {
    expect(spreadLabel(1, 1)).toBe('');
  });

  it('says nothing when the server did not count', () => {
    expect(spreadLabel(undefined, 3)).toBe('');
  });
});
