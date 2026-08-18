import { describe, expect, it } from 'vitest';
import { PODS_WORTH_ASKING, bigCluster, worthAsking } from '../../src/lib/namespaceOffer';

describe('whether a cluster is big enough to ask about', () => {
  it('needs a pod count to judge by', () => {
    expect(bigCluster({})).toBe(false);
  });

  it('says no below the threshold and yes at it', () => {
    expect(bigCluster({ '/v1/pods': PODS_WORTH_ASKING - 1 })).toBe(false);
    expect(bigCluster({ '/v1/pods': PODS_WORTH_ASKING })).toBe(true);
  });

  it('measures the clusters this was chosen for', () => {
    expect(bigCluster({ '/v1/pods': 81 })).toBe(false);
    expect(bigCluster({ '/v1/pods': 68 })).toBe(false);
    expect(bigCluster({ '/v1/pods': 582 })).toBe(false);
    expect(bigCluster({ '/v1/pods': 2993 })).toBe(true);
  });
});

describe('whether to make the offer', () => {
  const big = { '/v1/pods': 2993 };

  it('waits until it knows which cluster it is on', () => {
    expect(worthAsking('', big, false)).toBe(false);
  });

  it('stays quiet once that cluster has an answer', () => {
    expect(worthAsking('gke-prod', big, true)).toBe(false);
  });

  it('asks on a big cluster it has never asked about', () => {
    expect(worthAsking('gke-prod', big, false)).toBe(true);
  });

  it('stays quiet on a small one', () => {
    expect(worthAsking('p-mk2', { '/v1/pods': 68 }, false)).toBe(false);
  });
});
