import { create } from 'zustand';
import type { PortForward } from '../lib/types';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

const NONE: PortForward[] = [];

interface ForwardsState {
  byCluster: ByCluster<PortForward[]>;
  setForwards: (forwards: PortForward[]) => void;
  forget: (cluster: string) => void;
  clear: () => void;
}

export const useForwardsStore = create<ForwardsState>((set) => ({
  byCluster: {},
  setForwards: (forwards) => {
    const on = activeClusterNow();
    set((state) => ({ byCluster: put(state.byCluster, on, forwards) }));
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  clear: () => {
    set({ byCluster: {} });
  },
}));

export function useForwards(): PortForward[] {
  const on = useActiveCluster();
  return useForwardsStore((state) => held(state.byCluster, on, NONE));
}

export function setForwards(forwards: PortForward[]): void {
  useForwardsStore.getState().setForwards(forwards);
}

export function forwardsNow(): PortForward[] {
  return held(useForwardsStore.getState().byCluster, activeClusterNow(), NONE);
}

export function forgetForwards(cluster: string): void {
  useForwardsStore.getState().forget(cluster);
}
