import { afterEach, describe, expect, it, vi } from 'vitest';
import { fetchCapabilities } from '../../src/lib/capabilities';
import { setActiveCluster } from '../../src/lib/cluster';
import { bumpClusterEpoch } from '../../src/store/cluster';

function stub(): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((url: string) => {
    void url;
    return Promise.resolve({
      ok: true,
      json: () => Promise.resolve({ helm: true, traffic: false, localShell: true }),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  setActiveCluster('');
});

describe('fetchCapabilities', () => {
  it('lets readers that start together share one request', async () => {
    const fetchMock = stub();

    const [first, second] = await Promise.all([fetchCapabilities(), fetchCapabilities()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(second).toBe(first);
  });

  it('asks again once the shared request has answered', async () => {
    const fetchMock = stub();

    await fetchCapabilities();
    await fetchCapabilities();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('asks each cluster for its own', async () => {
    const fetchMock = stub();

    setActiveCluster('https://one:6443');
    const one = fetchCapabilities();
    setActiveCluster('https://two:6443');
    const two = fetchCapabilities();
    await Promise.all([one, two]);

    expect(fetchMock.mock.calls.map((call) => String(call[0]))).toEqual([
      '/api/capabilities?cluster=https%3A%2F%2Fone%3A6443',
      '/api/capabilities?cluster=https%3A%2F%2Ftwo%3A6443',
    ]);
  });

  it('asks again after a reconnect to the same cluster', async () => {
    const fetchMock = stub();

    const before = fetchCapabilities();
    bumpClusterEpoch();
    const after = fetchCapabilities();
    await Promise.all([before, after]);

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('reports a refusal to every reader that shared it', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    const reads = [fetchCapabilities(), fetchCapabilities()];

    await expect(reads[0]).rejects.toThrow('capabilities request failed with status 503');
    await expect(reads[1]).rejects.toThrow('capabilities request failed with status 503');
  });

  it('asks again after a refusal', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));
    await expect(fetchCapabilities()).rejects.toThrow('status 503');
    const fetchMock = stub();

    await fetchCapabilities();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
