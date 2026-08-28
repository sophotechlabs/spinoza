import type { ColumnSizingState, SortingState, VisibilityState } from '@tanstack/react-table';
import type { ResourceDescriptor } from './types';
import { asList, asNumber, asRecord, asString, optionalBoolean } from './wire';
import { readStored, writeStored } from './persist';

export const TABLE_STATE_KEY = 'spinoza.tables.v1';

export const METRIC_BASES = ['used', 'total'] as const;

export type MetricBasis = (typeof METRIC_BASES)[number];

export interface TableState {
  sorting: SortingState;
  visibility: VisibilityState;
  sizing: ColumnSizingState;
  // Which side of a used-of-total cell a metric column sorts on, by column id.
  bases: Partial<Record<string, MetricBasis>>;
}

export function emptyTableState(): TableState {
  return { sorting: [], visibility: {}, sizing: {}, bases: {} };
}

export function columnLabel(header: unknown, id: string): string {
  if (typeof header === 'string') {
    return header;
  }
  return id;
}

export function tableKey(active: ResourceDescriptor | null): string {
  if (active === null) {
    return '';
  }
  return `${active.group}/${active.version}/${active.resource}`;
}

function parseSorting(value: unknown): SortingState {
  const out: SortingState = [];
  for (const entry of asList(value)) {
    const item = asRecord(entry);
    const id = asString(item.id);
    if (id === '') {
      continue;
    }
    out.push({ id, desc: item.desc === true });
  }
  return out;
}

function parseVisibility(value: unknown): VisibilityState {
  const out: VisibilityState = {};
  for (const [id, entry] of Object.entries(asRecord(value))) {
    const flag = optionalBoolean(entry);
    if (flag === undefined) {
      continue;
    }
    out[id] = flag;
  }
  return out;
}

function parseSizing(value: unknown): ColumnSizingState {
  const out: ColumnSizingState = {};
  for (const [id, entry] of Object.entries(asRecord(value))) {
    if (typeof entry !== 'number') {
      continue;
    }
    out[id] = asNumber(entry);
  }
  return out;
}

function parseBases(value: unknown): Partial<Record<string, MetricBasis>> {
  const out: Partial<Record<string, MetricBasis>> = {};
  for (const [id, entry] of Object.entries(asRecord(value))) {
    for (const basis of METRIC_BASES) {
      if (entry === basis) {
        out[id] = basis;
      }
    }
  }
  return out;
}

export function parseTables(raw: string | null): Record<string, TableState> {
  if (raw === null) {
    return {};
  }
  let parsed: unknown = null;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {};
  }
  const out: Record<string, TableState> = {};
  for (const [key, entry] of Object.entries(asRecord(parsed))) {
    const item = asRecord(entry);
    out[key] = {
      sorting: parseSorting(item.sorting),
      visibility: parseVisibility(item.visibility),
      sizing: parseSizing(item.sizing),
      bases: parseBases(item.bases),
    };
  }
  return out;
}

function readAll(): Record<string, TableState> {
  return parseTables(readStored(TABLE_STATE_KEY));
}

export function readTableState(key: string): TableState {
  if (key === '') {
    return emptyTableState();
  }
  const all: Partial<Record<string, TableState>> = readAll();
  const stored = all[key];
  if (stored === undefined) {
    return emptyTableState();
  }
  return stored;
}

export function writeTableState(key: string, state: TableState): void {
  if (key === '') {
    return;
  }
  const all = readAll();
  all[key] = state;
  writeStored(TABLE_STATE_KEY, JSON.stringify(all));
}

interface MetricSort {
  sorting: SortingState;
  basis: MetricBasis;
}

// nextMetricSort walks a metric column through its four sorted states and back
// to none: most consumed, least consumed, largest, smallest.
export function nextMetricSort(id: string, sorting: SortingState, basis: MetricBasis): MetricSort {
  const current = sorting.find((entry) => entry.id === id);
  if (current === undefined) {
    return { sorting: [{ id, desc: true }], basis: 'used' };
  }
  if (current.desc) {
    return { sorting: [{ id, desc: false }], basis };
  }
  if (basis === 'used') {
    return { sorting: [{ id, desc: true }], basis: 'total' };
  }
  return { sorting: [], basis: 'used' };
}

// A node cell shows how much of how much, so its header says which of the two
// it is ordering by.
export function metricHeader(label: string, sorted: boolean, basis: MetricBasis): string {
  if (sorted && basis === 'total') {
    return `${label} total`;
  }
  return label;
}
