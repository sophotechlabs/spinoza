import { create } from 'zustand';

export interface TerminalSession {
  id: string;
  namespace: string;
  pod: string;
  container: string;
}

interface TerminalsState {
  sessions: TerminalSession[];
  active: string | null;
  open: (namespace: string, pod: string, container: string) => void;
  focus: (id: string) => void;
  close: (id: string) => void;
  reset: () => void;
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
  sessions: [],
  active: null,
  open: (namespace, pod, container) => {
    const id = sessionId(namespace, pod, container);
    set((state) => {
      const known = state.sessions.some((session) => session.id === id);
      if (known) {
        return { active: id };
      }
      return {
        sessions: [...state.sessions, { id, namespace, pod, container }],
        active: id,
      };
    });
  },
  focus: (id) => {
    set({ active: id });
  },
  close: (id) => {
    set((state) => ({
      sessions: without(state.sessions, id),
      active: nextActive(state.sessions, id, state.active),
    }));
  },
  reset: () => {
    set({ sessions: [], active: null });
  },
}));

export function clearTerminals(): void {
  useTerminalsStore.getState().reset();
}
