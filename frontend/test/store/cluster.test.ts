import { describe, expect, it } from 'vitest';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

describe('the cluster epoch', () => {
  it('starts at zero', () => {
    expect(useClusterStore.getState().epoch).toBe(0);
  });

  it('counts up once per context switch', () => {
    bumpClusterEpoch();
    bumpClusterEpoch();
    expect(useClusterStore.getState().epoch).toBe(2);
  });

  it('goes back to zero on reset', () => {
    bumpClusterEpoch();
    useClusterStore.getState().reset();
    expect(useClusterStore.getState().epoch).toBe(0);
  });
});
