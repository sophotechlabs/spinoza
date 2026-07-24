import { create } from 'zustand';
import type { PodRow, ServerMsg } from '../lib/types';

interface PodState {
  rows: Map<string, PodRow>;
  sorted: PodRow[];
  applySnapshot: (items: PodRow[]) => void;
  applyDelta: (msg: ServerMsg) => void;
}

function sortRows(rows: Map<string, PodRow>): PodRow[] {
  const arr = [...rows.values()];
  arr.sort((a, b) => {
    const byNamespace = a.namespace.localeCompare(b.namespace);
    if (byNamespace !== 0) {
      return byNamespace;
    }
    return a.name.localeCompare(b.name);
  });
  return arr;
}

export const usePodStore = create<PodState>((set) => ({
  rows: new Map(),
  sorted: [],
  applySnapshot: (items) => {
    const rows = new Map<string, PodRow>();
    for (const item of items) {
      rows.set(item.uid, item);
    }
    set({ rows, sorted: sortRows(rows) });
  },
  applyDelta: (msg) => {
    set((state) => {
      const rows = new Map(state.rows);
      if (msg.type === 'added' || msg.type === 'modified') {
        rows.set(msg.item.uid, msg.item);
      } else if (msg.type === 'deleted') {
        rows.delete(msg.uid);
      } else {
        return state;
      }
      return { rows, sorted: sortRows(rows) };
    });
  },
}));

export function usePodRows(): PodRow[] {
  return usePodStore((state) => state.sorted);
}
