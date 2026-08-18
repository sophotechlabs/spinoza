import type { Row } from './types';
import type { FilterField } from './filterChips';
import { NAMESPACE_FIELD, fieldFor, fieldKey, fieldValue } from './filterChips';

export interface Suggestion {
  kind: 'field' | 'value';
  label: string;
  text: string;
  hint: string;
}

export const MAX_SUGGESTIONS = 10;

function ranked(found: string[], needle: string): string[] {
  const starts: string[] = [];
  const holds: string[] = [];
  for (const one of found) {
    const low = one.toLowerCase();
    if (low.startsWith(needle)) {
      starts.push(one);
      continue;
    }
    if (low.includes(needle)) {
      holds.push(one);
    }
  }
  starts.sort();
  holds.sort();
  return [...starts, ...holds].slice(0, MAX_SUGGESTIONS);
}

function fieldsMatching(fields: FilterField[], needle: string): Suggestion[] {
  const found: Suggestion[] = [];
  for (const field of fields) {
    if (!field.key.startsWith(needle) && !field.label.toLowerCase().includes(needle)) {
      continue;
    }
    found.push({ kind: 'field', label: `${field.label}:`, text: `${field.key}:`, hint: 'field' });
  }
  return found.slice(0, MAX_SUGGESTIONS);
}

function valuesFor(rows: Row[], field: FilterField, namespaces: string[]): string[] {
  if (field.key === NAMESPACE_FIELD) {
    return namespaces;
  }
  const seen = new Set<string>();
  for (const row of rows) {
    const value = fieldValue(row, field);
    if (value !== '') {
      seen.add(value);
    }
  }
  return [...seen];
}

export function suggest(
  text: string,
  fields: FilterField[],
  rows: Row[],
  namespaces: string[],
): Suggestion[] {
  const trimmed = text.trimStart();
  if (trimmed === '') {
    return [];
  }
  const at = trimmed.indexOf(':');
  if (at < 0) {
    return fieldsMatching(fields, trimmed.toLowerCase());
  }
  const prefix = trimmed.slice(0, at);
  const field = fieldFor(fields, fieldKey(prefix));
  if (field === null) {
    return [];
  }
  const needle = trimmed
    .slice(at + 1)
    .trim()
    .toLowerCase();
  return ranked(valuesFor(rows, field, namespaces), needle).map((value) => ({
    kind: 'value',
    label: value,
    text: `${prefix}:${value}`,
    hint: field.label,
  }));
}
