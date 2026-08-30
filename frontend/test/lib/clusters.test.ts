import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  activateCluster,
  closeCluster,
  clusterFailure,
  fetchClusters,
  openCluster,
  parseClusters,
  recordCluster,
  renameCluster,
  reopenCluster,
  stillToOpen,
} from '../../src/lib/clusters';
import { useClustersStore } from '../../src/store/clusters';
import { MK1, MK2 } from '../helpers-clusters';

interface Call {
  url: string;
  method: string;
}

function stub(answer: unknown, ok = true): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: { method?: string }) => {
      calls.push({ url, method: init?.method ?? 'GET' });
      return Promise.resolve({
        ok,
        status: 500,
        json: () => Promise.resolve(answer),
      });
    }),
  );
  return calls;
}

const oneOpen = {
  clusters: [{ id: MK1, context: 'p-mk1', kubeconfig: '/work.yaml', active: true }],
  remembered: [{ id: MK2, context: 'p-mk2' }],
};

describe('what the server says is open', () => {
  beforeEach(() => {
    useClustersStore.getState().reset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('fills in everything the server left out', () => {
    const list = parseClusters({ clusters: [{}], remembered: [{}] });

    expect(list.clusters[0]).toEqual({
      id: '',
      context: '',
      kubeconfig: undefined,
      active: false,
      color: 1,
      label: undefined,
      grouping: undefined,
      reopen: true,
      timeline: undefined,
      protection: 'unknown',
      reachable: true,
      reason: undefined,
    });
    expect(list.remembered[0]).toEqual({ id: '', context: '', kubeconfig: undefined });
  });

  it('reads a body with nothing in it at all', () => {
    expect(parseClusters(null)).toEqual({ clusters: [], remembered: [] });
  });

  it('adopts what it fetched', async () => {
    stub(oneOpen);

    await fetchClusters();

    expect(useClustersStore.getState().tabs.map((one) => one.context)).toEqual(['p-mk1']);
  });

  it('says which remembered clusters are not open yet', () => {
    expect(stillToOpen(parseClusters(oneOpen)).map((one) => one.context)).toEqual(['p-mk2']);
  });

  it('leaves out a remembered cluster that is already open', () => {
    const both = { clusters: oneOpen.clusters, remembered: [{ id: MK1, context: 'p-mk1' }] };

    expect(stillToOpen(parseClusters(both))).toEqual([]);
  });

  it('opens a cluster by the context that names it', async () => {
    const calls = stub(oneOpen);

    await openCluster('/work.yaml', 'p-mk1');

    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toContain('name=p-mk1');
    expect(calls[0].url).toContain('kubeconfig=%2Fwork.yaml');
  });

  it('brings a cluster forward by its id', async () => {
    const calls = stub(oneOpen);

    await activateCluster(MK2);

    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toContain('/api/clusters/active');
    expect(calls[0].url).toContain(encodeURIComponent(MK2));
  });

  it('closes a cluster by its id', async () => {
    const calls = stub(oneOpen);

    await closeCluster(MK2);

    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].url).toContain(encodeURIComponent(MK2));
  });

  it('gives a tab a name and a group', async () => {
    const calls = stub(oneOpen);

    await renameCluster(MK1, 'client a prod', 'Client A');

    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toContain('/api/clusters/name');
    expect(calls[0].url).toContain('label=client+a+prod');
    expect(calls[0].url).toContain('grouping=Client+A');
  });

  it('says whether a tab comes back next time', async () => {
    const calls = stub(oneOpen);

    await reopenCluster(MK1, false);

    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toContain('/api/clusters/reopen');
    expect(calls[0].url).toContain('reopen=false');
  });

  it('says when a name is refused', async () => {
    stub({ message: 'a name may be 60 characters at most' }, false);

    await expect(renameCluster(MK1, 'x', '')).rejects.toThrow('60 characters');
  });

  it('says when a reopen flag is refused', async () => {
    stub({}, false);

    await expect(reopenCluster(MK1, true)).rejects.toThrow('remembering the cluster failed');
  });

  it('asks the server to record what a cluster does', async () => {
    const calls = stub(oneOpen);

    await recordCluster(MK1, 'workloads');

    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toContain('/api/clusters/timeline');
    expect(calls[0].url).toContain('kinds=workloads');
  });

  it('says so when what to record could not be stored', async () => {
    stub({}, false);

    await expect(recordCluster(MK1, 'workloads')).rejects.toThrow('changing what is recorded');
  });

  it('says which request failed', async () => {
    stub({ message: 'no route to host' }, false);

    await expect(openCluster('', 'p-mk9')).rejects.toThrow('no route to host');
  });

  it('says the list failed when the server refuses it', async () => {
    stub({}, false);

    await expect(fetchClusters()).rejects.toThrow('the cluster list failed');
  });

  it('has words for a rejection that is not an error', () => {
    expect(clusterFailure('nope', 'the request failed')).toBe('the request failed');
    expect(clusterFailure(new Error('gone'), 'the request failed')).toBe('gone');
  });
});
