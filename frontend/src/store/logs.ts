import { create } from 'zustand';

// The server sizes the backlog a stream opens with to fit in here — tailBudget
// in internal/logs/many.go. Lowering this below that number would throw away
// part of the history as it arrived, so a Go test fails if it ever does.
export const MAX_LOG_LINES = 5000;

interface StreamState {
  lines: string[];
  // sources sits alongside lines, one entry each, holding the pod a line came
  // from. A single-pod stream leaves them empty.
  sources: string[];
  dropped: number;
  revision: number;
  ended: boolean;
  resumed: boolean;
  attached: number;
  matched: number;
  // opened separates a stream that is reading nothing from one that has not
  // been answered yet: both have no pods, and only one is worth saying.
  opened: boolean;
  error?: string;
}

interface LogsState {
  streams: Map<string, StreamState>;
  startStream: (subId: string) => void;
  resumeStream: (subId: string) => void;
  openedStream: (subId: string, attached: number, matched: number) => void;
  appendLines: (subId: string, lines: string[], source: string) => void;
  clearLines: (subId: string) => void;
  endStream: (subId: string) => void;
  failStream: (subId: string, message: string) => void;
  clearStream: (subId: string) => void;
}

const EMPTY_LINES: string[] = [];

function fresh(): StreamState {
  return {
    lines: [],
    sources: [],
    dropped: 0,
    revision: 0,
    ended: false,
    resumed: false,
    attached: 0,
    matched: 0,
    opened: false,
  };
}

function append(held: StreamState, lines: string[], source: string): number {
  for (const line of lines) {
    held.lines.push(line);
    held.sources.push(source);
  }
  if (held.lines.length <= MAX_LOG_LINES) {
    return 0;
  }
  const excess = held.lines.length - MAX_LOG_LINES;
  held.lines.splice(0, excess);
  held.sources.splice(0, excess);
  return excess;
}

export const useLogsStore = create<LogsState>((set) => ({
  streams: new Map(),
  startStream: (subId) => {
    set((state) => {
      const streams = new Map(state.streams);
      streams.set(subId, fresh());
      return { streams };
    });
  },
  resumeStream: (subId) => {
    set((state) => {
      const existing = state.streams.get(subId);
      const streams = new Map(state.streams);
      if (existing === undefined) {
        streams.set(subId, fresh());
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
  openedStream: (subId, attached, matched) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const streams = new Map(state.streams);
      streams.set(subId, { ...existing, attached, matched, opened: true });
      return { streams };
    });
  },
  appendLines: (subId, lines, source) => {
    set((state) => {
      const existing = state.streams.get(subId);
      if (existing === undefined) {
        return state;
      }
      const dropped = append(existing, lines, source);
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
      existing.sources.length = 0;
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

export function useLogSources(subId: string): string[] {
  const sources = useLogsStore((state) => state.streams.get(subId)?.sources);
  if (sources === undefined) {
    return EMPTY_LINES;
  }
  return sources;
}

export function useLogPods(subId: string): {
  attached: number;
  matched: number;
  opened: boolean;
} {
  const attached = useLogsStore((state) => state.streams.get(subId)?.attached ?? 0);
  const matched = useLogsStore((state) => state.streams.get(subId)?.matched ?? 0);
  const opened = useLogsStore((state) => state.streams.get(subId)?.opened ?? false);
  return { attached, matched, opened };
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
