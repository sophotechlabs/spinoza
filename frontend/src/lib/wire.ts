export function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object') {
    return {};
  }
  if (value === null) {
    return {};
  }
  if (Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

export function asList(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value as unknown[];
  }
  return [];
}

export function asString(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  return '';
}

export function optionalString(value: unknown): string | undefined {
  if (typeof value === 'string') {
    return value;
  }
  return undefined;
}

export function asNumber(value: unknown): number {
  if (typeof value !== 'number') {
    return 0;
  }
  if (!Number.isFinite(value)) {
    return 0;
  }
  return value;
}

export function optionalNumber(value: unknown): number | undefined {
  if (typeof value !== 'number') {
    return undefined;
  }
  if (!Number.isFinite(value)) {
    return undefined;
  }
  return value;
}

export function asBoolean(value: unknown): boolean {
  return value === true;
}

export function optionalBoolean(value: unknown): boolean | undefined {
  if (typeof value === 'boolean') {
    return value;
  }
  return undefined;
}

export function oneOf<T extends string>(value: unknown, allowed: readonly T[], fallback: T): T {
  if (typeof value !== 'string') {
    return fallback;
  }
  const known = allowed as readonly string[];
  if (known.includes(value)) {
    return value as T;
  }
  return fallback;
}

export function stringMap(value: unknown): Record<string, string> | undefined {
  if (typeof value !== 'object') {
    return undefined;
  }
  if (value === null) {
    return undefined;
  }
  if (Array.isArray(value)) {
    return undefined;
  }
  const out: Record<string, string> = {};
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry === 'string') {
      out[key] = entry;
    }
  }
  return out;
}

export function stringList(value: unknown): string[] {
  const out: string[] = [];
  for (const entry of asList(value)) {
    if (typeof entry === 'string') {
      out.push(entry);
    }
  }
  return out;
}

export function optionalStringList(value: unknown): string[] | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  return stringList(value);
}

export function listOf<T>(value: unknown, parse: (item: Record<string, unknown>) => T): T[] {
  return asList(value).map((item) => parse(asRecord(item)));
}

export function optionalListOf<T>(
  value: unknown,
  parse: (item: Record<string, unknown>) => T,
): T[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  return listOf(value, parse);
}

export function numberMap(value: unknown): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [key, entry] of Object.entries(asRecord(value))) {
    if (typeof entry === 'number' && Number.isFinite(entry)) {
      out[key] = entry;
    }
  }
  return out;
}

export function optionalNumberMap(value: unknown): Record<string, number> | undefined {
  if (typeof value !== 'object') {
    return undefined;
  }
  if (value === null) {
    return undefined;
  }
  if (Array.isArray(value)) {
    return undefined;
  }
  return numberMap(value);
}

export function recordMap<T>(
  value: unknown,
  parse: (item: Record<string, unknown>) => T,
): Record<string, T> {
  const out: Record<string, T> = {};
  for (const [key, entry] of Object.entries(asRecord(value))) {
    out[key] = parse(asRecord(entry));
  }
  return out;
}
