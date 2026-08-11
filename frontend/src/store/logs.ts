import { create } from 'zustand';

export const MAX_LOG_LINES = 5000;

interface StreamState {
  lines: string[];
  dropped: number;
  revision: number;
  ended: boolean;
  resumed: boolean;
  error?: string;
}

interface LogsState {
  streams: Map<string, StreamState>;
  startStream: (subId: string) => void;
  resumeStream: (subId: string) => void;
  appendLines: (subId: string, lines: string[]) => void;
  clearLines: (subId: string) => void;
  endStream: (subId: string) => void;
  failStream: (subId: string, message: string) => void;
  clearStream: (subId: string) => void;
}

const EMPTY_LINES: string[] = [];

function append(buffer: string[], lines: string[]): number {
  for (const line of lines) {
    buffer.push(line);
  }
  if (buffer.length <= MAX_LOG_LINES) {
    return 0;
  }
  const excess = buffer.length - MAX_LOG_LINES;
  buffer.splice(0, excess);
  return excess;
}

export const useLogsStore = create<LogsState>((set) => ({
  streams: new Map(),
  startStream: (subId) => {
    set((state) => {
      const streams = new Map(state.streams);
      streams.set(subId, { lines: [], dropped: 0, revision: 0, ended: false, resumed: false });
      return { streams };
    });
  },
  resumeStream: (subId) => {
    set((state) => {
      const existing = state.streams.get(subId);
      const streams = new Map(state.streams);
      if (existing === undefined) {
        streams.set(subId, { lines: [], dropped: 0, revision: 0, ended: false, resumed: false });
        return { streams };
      }
      streams.set(subId, {
        ...existing,
        ended: false,
        error: undefined,
        resumed: existing.lines.length > 0,
        revision: existing.revision + 1,
      });
      return { streams };
    });
  },
  appendLines: (subId, lines) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const dropped = append(existing.lines, lines);
      const streams = new Map(state.streams);
      streams.set(subId, {
        ...existing,
        dropped: existing.dropped + dropped,
        revision: existing.revision + 1,
      });
      return { streams };
    });
  },
  clearLines: (subId) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      existing.lines.length = 0;
      const streams = new Map(state.streams);
      streams.set(subId, { ...existing, resumed: false, revision: existing.revision + 1 });
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

export function useLogRevision(subId: string): number {
  const revision = useLogsStore((state) => state.streams.get(subId)?.revision);
  if (revision === undefined) {
    return 0;
  }
  return revision;
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

export function useLogResumed(subId: string): boolean {
  const resumed = useLogsStore((state) => state.streams.get(subId)?.resumed);
  if (resumed === undefined) {
    return false;
  }
  return resumed;
}

export function useLogEnded(subId: string): boolean {
  const ended = useLogsStore((state) => state.streams.get(subId)?.ended);
  if (ended === undefined) {
    return false;
  }
  return ended;
}
