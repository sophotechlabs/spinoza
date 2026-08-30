import { create } from 'zustand';

interface IssuesState {
  fleet: boolean;
  setFleet: (fleet: boolean) => void;
}

export const useIssuesStore = create<IssuesState>((set) => ({
  fleet: false,
  setFleet: (fleet) => {
    set({ fleet });
  },
}));

export function useFleetIssues(): boolean {
  return useIssuesStore((state) => state.fleet);
}
