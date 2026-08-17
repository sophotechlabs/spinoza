import type { Row } from './types';

export interface ImposedFilter {
  text: string;
  at: number;
}

export const NO_FILTER: ImposedFilter = { text: '', at: 0 };

export function imposeFilter(current: ImposedFilter, text: string): ImposedFilter {
  return { text, at: current.at + 1 };
}

export function filterRows(rows: Row[], query: string): Row[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return rows;
  }
  return rows.filter((row) => row.name.toLowerCase().includes(needle));
}
