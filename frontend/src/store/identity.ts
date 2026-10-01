import { create } from 'zustand';
import type { Session } from '../lib/types';
import { OWN_WINDOW } from '../lib/identity';
import { stopSaving } from '../lib/persist';

interface IdentityState {
  session: Session;
  known: boolean;
  adopt: (session: Session) => void;
}

export const useIdentityStore = create<IdentityState>((set) => ({
  session: OWN_WINDOW,
  known: false,
  adopt: (session: Session) => {
    set({ session, known: true });
  },
}));

function nobodyToSaveFor(session: Session): boolean {
  if (!session.cluster) {
    return false;
  }
  return session.user === undefined || session.user === '';
}

export function adoptSession(session: Session): void {
  if (nobodyToSaveFor(session)) {
    stopSaving();
  }
  useIdentityStore.getState().adopt(session);
}

export function useSession(): Session {
  return useIdentityStore((state) => state.session);
}

export function useSessionKnown(): boolean {
  return useIdentityStore((state) => state.known);
}

export function useClusterMode(): boolean {
  return useIdentityStore((state) => state.session.cluster);
}
