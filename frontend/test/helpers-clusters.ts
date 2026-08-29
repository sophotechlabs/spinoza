import type { ClusterList } from '../src/lib/types';
import { adoptClusters } from '../src/store/clusters';

export const MK1 = 'https://p-mk1:6443';

export const MK2 = 'https://p-mk2:6443';

export function listOf(active: string): ClusterList {
  return {
    clusters: [
      {
        id: MK1,
        context: 'p-mk1',
        active: active === MK1,
        color: 1,
        reopen: true,
        protection: 'open',
        reachable: true,
      },
      {
        id: MK2,
        context: 'p-mk2',
        active: active === MK2,
        color: 2,
        reopen: true,
        protection: 'open',
        reachable: true,
      },
    ],
    remembered: [],
  };
}

export function showing(cluster: string): void {
  adoptClusters(listOf(cluster));
}
