import { useMemo } from 'react';
import { create } from 'zustand';
import type { Column, Row, ServerMsg } from '../lib/types';

interface SubState {
  columns: Column[];
  namespaced: boolean;
  rows: Map<string, Row>;
  revision: number;
}

interface ResourcesState {
  subs: Map<string, SubState>;
  errors: Map<string, string>;
  applySnapshot: (subId: string, columns: Column[], namespaced: boolean, rows: Row[]) => void;
  applyDeltas: (subId: string, msgs: ServerMsg[]) => void;
  failSub: (subId: string, message: string) => void;
  clearSub: (subId: string) => void;
}

const EMPTY_COLUMNS: Column[] = [];
const EMPTY_ROWS: Row[] = [];

const collator = new Intl.Collator();

function sortRows(rows: Map<string, Row>): Row[] {
  const arr = [...rows.values()];
  arr.sort((a, b) => {
    const byNamespace = collator.compare(a.namespace, b.namespace);
    if (byNamespace !== 0) {
      return byNamespace;
    }
    return collator.compare(a.name, b.name);
  });
  return arr;
}

function applyOne(rows: Map<string, Row>, msg: ServerMsg): boolean {
  if (msg.type === 'added' || msg.type === 'modified') {
    rows.set(msg.row.uid, msg.row);
    return true;
  }
  if (msg.type === 'deleted') {
    return rows.delete(msg.uid);
  }
  return false;
}

export const useResourcesStore = create<ResourcesState>((set) => ({
  subs: new Map(),
  errors: new Map(),
  applySnapshot: (subId, columns, namespaced, rows) => {
    set((state) => {
      const rowMap = new Map<string, Row>();
      for (const row of rows) {
        rowMap.set(row.uid, row);
      }
      const subs = new Map(state.subs);
      subs.set(subId, { columns, namespaced, rows: rowMap, revision: 0 });
      const errors = new Map(state.errors);
      errors.delete(subId);
      return { subs, errors };
    });
  },
  failSub: (subId, message) => {
    set((state) => {
      const errors = new Map(state.errors);
      errors.set(subId, message);
      return { errors };
    });
  },
  applyDeltas: (subId, msgs) => {
    set((state) => {
      const existing = state.subs.get(subId);
      if (existing === undefined) {
        return state;
      }
      let touched = false;
      for (const msg of msgs) {
        if (applyOne(existing.rows, msg)) {
          touched = true;
        }
      }
      if (!touched) {
        return state;
      }
      const subs = new Map(state.subs);
      subs.set(subId, { ...existing, revision: existing.revision + 1 });
      return { subs };
    });
  },
  clearSub: (subId) => {
    set((state) => {
      if (!state.subs.has(subId) && !state.errors.has(subId)) {
        return state;
      }
      const subs = new Map(state.subs);
      subs.delete(subId);
      const errors = new Map(state.errors);
      errors.delete(subId);
      return { subs, errors };
    });
  },
}));

export function useSubColumns(subId: string): Column[] {
  const columns = useResourcesStore((state) => state.subs.get(subId)?.columns);
  if (columns === undefined) {
    return EMPTY_COLUMNS;
  }
  return columns;
}

export function useSubNamespaced(subId: string): boolean {
  const namespaced = useResourcesStore((state) => state.subs.get(subId)?.namespaced);
  if (namespaced === undefined) {
    return false;
  }
  return namespaced;
}

export function useSubLoaded(subId: string): boolean {
  return useResourcesStore((state) => state.subs.has(subId));
}

export function useSubError(subId: string): string | null {
  const message = useResourcesStore((state) => state.errors.get(subId));
  if (message === undefined) {
    return null;
  }
  return message;
}

export function useSubRows(subId: string): Row[] {
  const sub = useResourcesStore((state) => state.subs.get(subId));
  return useMemo(() => {
    if (sub === undefined) {
      return EMPTY_ROWS;
    }
    return sortRows(sub.rows);
  }, [sub]);
}
