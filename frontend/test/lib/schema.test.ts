import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  fetchSchema,
  gvkOf,
  registerSchema,
  resetSchemas,
  schemaPath,
  setSchemaApplier,
} from '../../src/lib/schema';
import type { JsonSchema, SchemaEntry } from '../../src/lib/schema';
import type { ObjectDetail } from '../../src/lib/types';

function detail(apiVersion: string, kind: string): ObjectDetail {
  return {
    apiVersion,
    kind,
    name: 'web',
    namespace: 'flux-system',
    uid: 'uid-web',
    createdAt: '2026-07-27T09:00:00Z',
    yaml: '',
  };
}

describe('schema', () => {
  beforeEach(() => {
    resetSchemas();
  });

  afterEach(() => {
    resetSchemas();
    vi.unstubAllGlobals();
  });

  it('splits a grouped apiVersion into group and version', () => {
    expect(gvkOf(detail('apps/v1', 'Deployment'))).toEqual({
      group: 'apps',
      version: 'v1',
      kind: 'Deployment',
    });
  });

  it('treats an ungrouped apiVersion as the core group', () => {
    expect(gvkOf(detail('v1', 'Pod'))).toEqual({ group: '', version: 'v1', kind: 'Pod' });
  });

  it('builds a distinct model path per kind', () => {
    expect(schemaPath({ group: '', version: 'v1', kind: 'Pod' })).toBe('spinoza/core/v1/Pod.yaml');
    expect(schemaPath({ group: 'apps', version: 'v1', kind: 'Deployment' })).toBe(
      'spinoza/apps/v1/Deployment.yaml',
    );
  });

  it('requests the schema endpoint with the gvk', async () => {
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ a: 1 }) });
    vi.stubGlobal('fetch', mock);

    await expect(
      fetchSchema({ group: 'apps', version: 'v1', kind: 'Deployment' }),
    ).resolves.toEqual({ a: 1 });
    expect(mock).toHaveBeenCalledWith('/api/schema?group=apps&version=v1&kind=Deployment');
  });

  it('reports a schema request failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404 }));

    await expect(fetchSchema({ group: '', version: 'v1', kind: 'Widget' })).rejects.toThrow(
      'schema request failed with status 404',
    );
  });

  it('applies registered schemas to the editor', () => {
    const applied: SchemaEntry[][] = [];
    setSchemaApplier((schemas) => {
      applied.push(schemas);
    });

    const schema: JsonSchema = { type: 'object' };
    registerSchema('spinoza/core/v1/Pod.yaml', schema);

    expect(applied[applied.length - 1]).toEqual([
      {
        uri: 'spinoza://spinoza/core/v1/Pod.yaml',
        fileMatch: ['spinoza/core/v1/Pod.yaml'],
        schema,
      },
    ]);
  });

  it('accumulates schemas across kinds', () => {
    const applied: SchemaEntry[][] = [];
    setSchemaApplier((schemas) => {
      applied.push(schemas);
    });

    registerSchema('spinoza/core/v1/Pod.yaml', { type: 'object' });
    registerSchema('spinoza/apps/v1/Deployment.yaml', { type: 'object' });

    expect(applied[applied.length - 1]).toHaveLength(2);
  });

  it('does not reapply an unchanged schema', () => {
    const applied: SchemaEntry[][] = [];
    setSchemaApplier((schemas) => {
      applied.push(schemas);
    });
    const schema: JsonSchema = { type: 'object' };

    registerSchema('spinoza/core/v1/Pod.yaml', schema);
    const count = applied.length;
    registerSchema('spinoza/core/v1/Pod.yaml', schema);

    expect(applied).toHaveLength(count);
  });

  it('replaces a schema when the content changes', () => {
    const applied: SchemaEntry[][] = [];
    setSchemaApplier((schemas) => {
      applied.push(schemas);
    });

    registerSchema('spinoza/core/v1/Pod.yaml', { type: 'object' });
    registerSchema('spinoza/core/v1/Pod.yaml', { type: 'string' });

    const latest = applied[applied.length - 1];
    expect(latest).toHaveLength(1);
    expect(latest[0].schema).toEqual({ type: 'string' });
  });

  it('flushes already-registered schemas when an applier arrives late', () => {
    registerSchema('spinoza/core/v1/Pod.yaml', { type: 'object' });
    const applied: SchemaEntry[][] = [];

    setSchemaApplier((schemas) => {
      applied.push(schemas);
    });

    expect(applied).toHaveLength(1);
    expect(applied[0]).toHaveLength(1);
  });

  it('registering without an applier does not throw', () => {
    expect(() => {
      registerSchema('spinoza/core/v1/Pod.yaml', { type: 'object' });
    }).not.toThrow();
  });
});
