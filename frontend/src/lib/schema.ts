import type { ObjectDetail } from './types';

export interface SchemaGVK {
  group: string;
  version: string;
  kind: string;
}

export type JsonSchema = Record<string, unknown>;

export interface SchemaEntry {
  uri: string;
  fileMatch: string[];
  schema: JsonSchema;
}

export type SchemaApplier = (schemas: SchemaEntry[]) => void;

export function gvkOf(detail: ObjectDetail): SchemaGVK {
  const parts = detail.apiVersion.split('/');
  if (parts.length === 2) {
    return { group: parts[0], version: parts[1], kind: detail.kind };
  }
  return { group: '', version: parts[0], kind: detail.kind };
}

export function schemaPath(gvk: SchemaGVK): string {
  let group = gvk.group;
  if (group === '') {
    group = 'core';
  }
  return `spinoza/${group}/${gvk.version}/${gvk.kind}.yaml`;
}

export async function fetchSchema(gvk: SchemaGVK): Promise<JsonSchema> {
  const params = new URLSearchParams({
    group: gvk.group,
    version: gvk.version,
    kind: gvk.kind,
  });
  const response = await fetch(`/api/schema?${params.toString()}`);
  if (!response.ok) {
    throw new Error(`schema request failed with status ${response.status}`);
  }
  return (await response.json()) as JsonSchema;
}

const registry = new Map<string, JsonSchema>();
let applier: SchemaApplier | null = null;

function entries(): SchemaEntry[] {
  return [...registry].map(([path, schema]) => ({
    uri: `spinoza://${path}`,
    fileMatch: [path],
    schema,
  }));
}

function apply(): void {
  if (applier === null) {
    return;
  }
  applier(entries());
}

export function setSchemaApplier(next: SchemaApplier): void {
  applier = next;
  apply();
}

export function registerSchema(path: string, schema: JsonSchema): void {
  if (registry.get(path) === schema) {
    return;
  }
  registry.set(path, schema);
  apply();
}

export function resetSchemas(): void {
  registry.clear();
  applier = null;
}
