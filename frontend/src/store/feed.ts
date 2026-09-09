import { create } from 'zustand';
import type { ConnectionStatus } from '../lib/feed';

interface FeedState {
  status: ConnectionStatus;
  attempt: number;
  report: (status: ConnectionStatus, attempt: number) => void;
  reset: () => void;
}

export const useFeedStore = create<FeedState>((set) => ({
  status: 'connecting',
  attempt: 0,
  report: (status, attempt) => {
    set({ status, attempt });
  },
  reset: () => {
    set({ status: 'connecting', attempt: 0 });
  },
}));

export function reportFeed(status: ConnectionStatus, attempt: number): void {
  useFeedStore.getState().report(status, attempt);
}
