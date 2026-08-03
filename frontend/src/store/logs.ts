import { create } from 'zustand';

export const MAX_LOG_LINES = 5000;

interface StreamState {
  lines: string[];
  dropped: number;
  ended: boolean;
  error?: string;
}

interface LogsState {
  streams: Map<string, StreamState>;
  startStream: (subId: string) => void;
  appendLines: (subId: string, lines: string[]) => void;
  endStream: (subId: string) => void;
  failStream: (subId: string, message: string) => void;
  clearStream: (subId: string) => void;
}

const EMPTY_LINES: string[] = [];

function trim(lines: string[]): { lines: string[]; dropped: number } {
  if (lines.length <= MAX_LOG_LINES) {
    return { lines, dropped: 0 };
  }
  const excess = lines.length - MAX_LOG_LINES;
  return { lines: lines.slice(excess), dropped: excess };
}

export const useLogsStore = create<LogsState>((set) => ({
  streams: new Map(),
  startStream: (subId) => {
    set((state) => {
      const streams = new Map(state.streams);
      streams.set(subId, { lines: [], dropped: 0, ended: false });
      return { streams };
    });
  },
  appendLines: (subId, lines) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const streams = new Map(state.streams);
      const trimmed = trim([...existing.lines, ...lines]);
      streams.set(subId, {
        ...existing,
        lines: trimmed.lines,
        dropped: existing.dropped + trimmed.dropped,
      });
      return { streams };
    });
  },
  failStream: (subId, message) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const streams = new Map(state.streams);
      streams.set(subId, { ...existing, ended: true, error: message });
      return { streams };
    });
  },
  endStream: (subId) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const streams = new Map(state.streams);
      streams.set(subId, { ...existing, ended: true });
      return { streams };
    });
  },
  clearStream: (subId) => {
    set((state) => {
      if (!state.streams.has(subId)) {
        return state;
      }
      const streams = new Map(state.streams);
      streams.delete(subId);
      return { streams };
    });
  },
}));

export function useLogLines(subId: string): string[] {
  const lines = useLogsStore((state) => state.streams.get(subId)?.lines);
  if (lines === undefined) {
    return EMPTY_LINES;
  }
  return lines;
}

export function useLogOffset(subId: string): number {
  const dropped = useLogsStore((state) => state.streams.get(subId)?.dropped);
  if (dropped === undefined) {
    return 0;
  }
  return dropped;
}

export function useLogError(subId: string): string | null {
  const message = useLogsStore((state) => state.streams.get(subId)?.error);
  if (message === undefined) {
    return null;
  }
  return message;
}

export function useLogEnded(subId: string): boolean {
  const ended = useLogsStore((state) => state.streams.get(subId)?.ended);
  if (ended === undefined) {
    return false;
  }
  return ended;
}
