import { create } from 'zustand';

interface SessionState {
  expired: boolean;
  expire: () => void;
  reset: () => void;
}

export const useSessionStore = create<SessionState>((set) => ({
  expired: false,
  expire: () => {
    set({ expired: true });
  },
  reset: () => {
    set({ expired: false });
  },
}));

export function expireSession(): void {
  if (useSessionStore.getState().expired) {
    return;
  }
  useSessionStore.getState().expire();
}

export function sessionExpired(): boolean {
  return useSessionStore.getState().expired;
}

export function useSessionExpired(): boolean {
  return useSessionStore((state) => state.expired);
}
