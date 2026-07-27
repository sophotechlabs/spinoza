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

export function filterRows(rows: Row[], query: string, namespace: string): Row[] {
  const needle = query.trim().toLowerCase();
  return rows.filter((row) => {
    if (namespace !== ALL_NAMESPACES && row.namespace !== namespace) {
      return false;
    }
    if (needle === '') {
      return true;
    }
    return row.name.toLowerCase().includes(needle);
  });
}
