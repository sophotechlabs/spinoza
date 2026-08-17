import type { Row } from './types';

export const ALL_NAMESPACES = '';

export function namespacesOf(rows: Row[]): string[] {
  const seen = new Set<string>();
  for (const row of rows) {
    if (row.namespace !== '') {
      seen.add(row.namespace);
    }
  }
  return [...seen].sort((a, b) => a.localeCompare(b));
}

export function filterRows(rows: Row[], query: string): Row[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return rows;
  }
  return rows.filter((row) => row.name.toLowerCase().includes(needle));
}
