import type { Column, Row } from './types';

export interface Chip {
  field: string;
  value: string;
}

export interface FilterField {
  key: string;
  label: string;
  cell: number;
}

const NAME_FIELD = 'name';

export const NAMESPACE_FIELD = 'namespace';

const NAME_CELL = -1;

const NAMESPACE_CELL = -2;

export function fieldKey(label: string): string {
  return label.toLowerCase().replace(/[^a-z0-9]/g, '');
}

export function fieldsOf(columns: Column[], namespaced: boolean): FilterField[] {
  const out: FilterField[] = [{ key: NAME_FIELD, label: 'Name', cell: NAME_CELL }];
  if (namespaced) {
    out.push({ key: NAMESPACE_FIELD, label: 'Namespace', cell: NAMESPACE_CELL });
  }
  columns.forEach((column, index) => {
    const key = fieldKey(column.name);
    if (key === '') {
      return;
    }
    if (out.some((one) => one.key === key)) {
      return;
    }
    out.push({ key, label: column.name, cell: index });
  });
  return out;
}

const SCOPE_KEYS = ['ns', NAMESPACE_FIELD];

function aliasOf(key: string): string {
  if (key === 'ns') {
    return NAMESPACE_FIELD;
  }
  return key;
}

export function fieldFor(fields: FilterField[], key: string): FilterField | null {
  const wanted = aliasOf(key);
  for (const field of fields) {
    if (field.key === wanted) {
      return field;
    }
  }
  return null;
}

export function labelOf(chip: Chip, fields: FilterField[]): string {
  const field = fieldFor(fields, chip.field);
  if (field === null) {
    return chip.field;
  }
  return field.label;
}

export function chipKey(chip: Chip): string {
  return `${chip.field}:${chip.value.toLowerCase()}`;
}

export function parseChip(text: string, fields: FilterField[]): Chip | null {
  const trimmed = text.trim();
  if (trimmed === '') {
    return null;
  }
  const at = trimmed.indexOf(':');
  if (at < 0) {
    return { field: NAME_FIELD, value: trimmed };
  }
  const key = fieldKey(trimmed.slice(0, at));
  const field = fieldFor(fields, key);
  if (field === null && SCOPE_KEYS.includes(key)) {
    return null;
  }
  if (field === null) {
    return { field: NAME_FIELD, value: trimmed };
  }
  const value = trimmed.slice(at + 1).trim();
  if (value === '') {
    return null;
  }
  return { field: field.key, value };
}

export function nameChips(text: string): Chip[] {
  const value = text.trim();
  if (value === '') {
    return [];
  }
  return [{ field: NAME_FIELD, value }];
}

export function fieldValue(row: Row, field: FilterField): string {
  if (field.cell === NAME_CELL) {
    return row.name;
  }
  if (field.cell === NAMESPACE_CELL) {
    return row.namespace;
  }
  if (field.cell >= row.cells.length) {
    return '';
  }
  return row.cells[field.cell];
}

export function matches(row: Row, chip: Chip, fields: FilterField[]): boolean {
  const field = fieldFor(fields, chip.field);
  if (field === null) {
    return true;
  }
  return fieldValue(row, field).toLowerCase().includes(chip.value.toLowerCase());
}

export function filterRows(rows: Row[], chips: Chip[], fields: FilterField[]): Row[] {
  if (chips.length === 0) {
    return rows;
  }
  return rows.filter((row) => chips.every((chip) => matches(row, chip, fields)));
}
