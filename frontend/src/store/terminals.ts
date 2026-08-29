import { create } from 'zustand';
import { activeClusterNow, useActiveCluster } from './clusters';
import type { ByCluster } from './perCluster';
import { drop, held, put } from './perCluster';

export type TerminalKind = 'pod' | 'local' | 'node';

export interface TerminalSession {
  id: string;
  kind: TerminalKind;
  namespace: string;
  pod: string;
  container: string;
}

interface Shells {
  sessions: TerminalSession[];
  active: string | null;
}

interface TerminalsState {
  byCluster: ByCluster<Shells>;
  open: (namespace: string, pod: string, container: string) => void;
  openLocal: () => void;
  openNode: (node: string) => void;
  focus: (id: string) => void;
  close: (id: string) => void;
  forget: (cluster: string) => void;
  reset: () => void;
}

const NO_SESSIONS: TerminalSession[] = [];

const NO_SHELLS: Shells = { sessions: NO_SESSIONS, active: null };

function change(state: TerminalsState, next: (shells: Shells) => Shells): Partial<TerminalsState> {
  const on = activeClusterNow();
  return { byCluster: put(state.byCluster, on, next(held(state.byCluster, on, NO_SHELLS))) };
}

export const LOCAL_SESSION = 'local';

function nodeSessionId(node: string): string {
  return `node/${node}`;
}

export function sessionId(namespace: string, pod: string, container: string): string {
  return `${namespace}/${pod}/${container}`;
}

function without(sessions: TerminalSession[], id: string): TerminalSession[] {
  return sessions.filter((session) => session.id !== id);
}

function nextActive(
  sessions: TerminalSession[],
  closed: string,
  active: string | null,
): string | null {
  if (active !== closed) {
    return active;
  }
  const left = without(sessions, closed);
  if (left.length === 0) {
    return null;
  }
  return left[left.length - 1].id;
}

export const useTerminalsStore = create<TerminalsState>((set) => ({
  byCluster: {},
  open: (namespace, pod, container) => {
    const id = sessionId(namespace, pod, container);
    set((state) =>
      change(state, (shells) => {
        if (shells.sessions.some((session) => session.id === id)) {
          return { ...shells, active: id };
        }
        const shell: TerminalSession = { id, kind: 'pod', namespace, pod, container };
        return { sessions: [...shells.sessions, shell], active: id };
      }),
    );
  },
  openLocal: () => {
    set((state) =>
      change(state, (shells) => {
        if (shells.sessions.some((session) => session.id === LOCAL_SESSION)) {
          return { ...shells, active: LOCAL_SESSION };
        }
        const local: TerminalSession = {
          id: LOCAL_SESSION,
          kind: 'local',
          namespace: '',
          pod: '',
          container: '',
        };
        return { sessions: [local, ...shells.sessions], active: LOCAL_SESSION };
      }),
    );
  },
  openNode: (node) => {
    const id = nodeSessionId(node);
    set((state) =>
      change(state, (shells) => {
        if (shells.sessions.some((session) => session.id === id)) {
          return { ...shells, active: id };
        }
        const shell: TerminalSession = {
          id,
          kind: 'node',
          namespace: '',
          pod: node,
          container: '',
        };
        return { sessions: [shell, ...shells.sessions], active: id };
      }),
    );
  },
  focus: (id) => {
    set((state) => change(state, (shells) => ({ ...shells, active: id })));
  },
  close: (id) => {
    set((state) =>
      change(state, (shells) => ({
        sessions: without(shells.sessions, id),
        active: nextActive(shells.sessions, id, shells.active),
      })),
    );
  },
  forget: (cluster) => {
    set((state) => ({ byCluster: drop(state.byCluster, cluster) }));
  },
  reset: () => {
    set({ byCluster: {} });
  },
}));

export function useTerminalSessions(): TerminalSession[] {
  const on = useActiveCluster();
  return useTerminalsStore((state) => state.byCluster[on]?.sessions ?? NO_SESSIONS);
}

export function useActiveTerminal(): string | null {
  const on = useActiveCluster();
  return useTerminalsStore((state) => state.byCluster[on]?.active ?? null);
}

export function terminalsNow(): TerminalSession[] {
  return useTerminalsStore.getState().byCluster[activeClusterNow()]?.sessions ?? NO_SESSIONS;
}

export function forgetTerminals(cluster: string): void {
  useTerminalsStore.getState().forget(cluster);
}

export function clearTerminals(): void {
  useTerminalsStore.getState().reset();
}
