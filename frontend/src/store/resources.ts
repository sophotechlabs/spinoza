import { useMemo } from 'react';
import { create } from 'zustand';
import type { Column, Row, ServerMsg } from '../lib/types';

interface SubState {
  columns: Column[];
  namespaced: boolean;
  rows: Map<string, Row>;
}

interface ResourcesState {
  subs: Map<string, SubState>;
  errors: Map<string, string>;
  applySnapshot: (subId: string, columns: Column[], namespaced: boolean, rows: Row[]) => void;
  applyDelta: (subId: string, msg: ServerMsg) => void;
  failSub: (subId: string, message: string) => void;
  clearSub: (subId: string) => void;
}

const EMPTY_COLUMNS: Column[] = [];
const EMPTY_ROWS: Row[] = [];

function sortRows(rows: Map<string, Row>): Row[] {
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
      subs.set(subId, { columns, namespaced, rows: rowMap });
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
  applyDelta: (subId, msg) => {
    set((state) => {
      const existing = state.subs.get(subId);
      if (existing === undefined) {
        return state;
      }
      if (msg.type === 'added' || msg.type === 'modified') {
        const rows = new Map(existing.rows);
        rows.set(msg.row.uid, msg.row);
        const subs = new Map(state.subs);
        subs.set(subId, { ...existing, rows });
        return { subs };
      }
      if (msg.type === 'deleted') {
        const rows = new Map(existing.rows);
        rows.delete(msg.uid);
        const subs = new Map(state.subs);
        subs.set(subId, { ...existing, rows });
        return { subs };
      }
      return state;
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

export function useSubError(subId: string): string | null {
  const message = useResourcesStore((state) => state.errors.get(subId));
  if (message === undefined) {
    return null;
  }
  return message;
}

export function useSubRows(subId: string): Row[] {
  const rows = useResourcesStore((state) => state.subs.get(subId)?.rows);
  return useMemo(() => {
    if (rows === undefined) {
      return EMPTY_ROWS;
    }
    return sortRows(rows);
  }, [rows]);
}
