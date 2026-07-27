import { create } from 'zustand';

export const MAX_LOG_LINES = 5000;

interface StreamState {
  lines: string[];
  ended: boolean;
}

interface LogsState {
  streams: Map<string, StreamState>;
  startStream: (subId: string) => void;
  appendLines: (subId: string, lines: string[]) => void;
  endStream: (subId: string) => void;
  clearStream: (subId: string) => void;
}

const EMPTY_LINES: string[] = [];

function trim(lines: string[]): string[] {
  if (lines.length <= MAX_LOG_LINES) {
    return lines;
  }
  return lines.slice(lines.length - MAX_LOG_LINES);
}

export const useLogsStore = create<LogsState>((set) => ({
  streams: new Map(),
  startStream: (subId) => {
    set((state) => {
      const streams = new Map(state.streams);
      streams.set(subId, { lines: [], ended: false });
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
      streams.set(subId, { ...existing, lines: trim([...existing.lines, ...lines]) });
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

export function useLogEnded(subId: string): boolean {
  const ended = useLogsStore((state) => state.streams.get(subId)?.ended);
  if (ended === undefined) {
    return false;
  }
  return ended;
}
