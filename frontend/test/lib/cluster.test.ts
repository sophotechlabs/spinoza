import { afterEach, describe, expect, it, vi } from 'vitest';
import { activeCluster, onCluster, setActiveCluster } from '../../src/lib/cluster';
import { request } from '../../src/lib/http';

const mk2 = 'https://p-mk2:6443';

afterEach(() => {
  setActiveCluster('');
  vi.unstubAllGlobals();
});

describe('the cluster a request is for', () => {
  it('starts as whichever cluster the backend calls active', () => {
    expect(activeCluster()).toBe('');
  });

  it('leaves a url alone while no cluster has been picked', () => {
    expect(onCluster('/api/objects?resource=pods')).toBe('/api/objects?resource=pods');
  });

  it('names the picked cluster on a url that has no query yet', () => {
    setActiveCluster(mk2);

    expect(onCluster('/api/contexts')).toBe(`/api/contexts?cluster=${encodeURIComponent(mk2)}`);
  });

  it('adds the cluster to a url that already carries a query', () => {
    setActiveCluster(mk2);

    expect(onCluster('/api/objects?resource=pods')).toBe(
      `/api/objects?resource=pods&cluster=${encodeURIComponent(mk2)}`,
    );
  });

  it('does not overrule a url that already names a cluster', () => {
    setActiveCluster(mk2);

    expect(onCluster('/api/clusters?cluster=https%3A%2F%2Fp-mk1%3A6443')).toBe(
      '/api/clusters?cluster=https%3A%2F%2Fp-mk1%3A6443',
    );
  });

  it('does not mistake another parameter ending in cluster for the cluster', () => {
    setActiveCluster(mk2);

    expect(onCluster('/api/objects?name=mycluster')).toBe(
      `/api/objects?name=mycluster&cluster=${encodeURIComponent(mk2)}`,
    );
  });

  it('remembers what it was told', () => {
    setActiveCluster(mk2);

    expect(activeCluster()).toBe(mk2);
  });
});

describe('what the backend is asked', () => {
  let seen = '';

  it('sends every request to the cluster the window is looking at', async () => {
    const fetched = vi.fn((url: string) => {
      seen = url;
      return Promise.resolve(new Response('{}'));
    });
    vi.stubGlobal('fetch', fetched);
    setActiveCluster(mk2);

    await request('/api/objects?resource=pods');

    expect(seen).toBe(`/api/objects?resource=pods&cluster=${encodeURIComponent(mk2)}`);
  });

  it('asks for nothing in particular before a cluster is picked', async () => {
    const fetched = vi.fn((url: string) => {
      seen = url;
      return Promise.resolve(new Response('{}'));
    });
    vi.stubGlobal('fetch', fetched);

    await request('/api/objects?resource=pods');

    expect(seen).toBe('/api/objects?resource=pods');
  });
});
