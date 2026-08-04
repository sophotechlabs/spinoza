import type { ResourceCatalog } from './types';
import { request } from './http';
import { parseCatalog, parseCounts } from './parse';

async function catalog(method: string): Promise<ResourceCatalog> {
  const response = await request('/api/resources', { method });
  if (!response.ok) {
    throw new Error(`discovery request failed with status ${response.status}`);
  }
  return parseCatalog(await response.json());
}

export async function fetchResources(): Promise<ResourceCatalog> {
  return catalog('GET');
}

export async function refreshResources(): Promise<ResourceCatalog> {
  return catalog('POST');
}

export async function fetchResourceCounts(): Promise<Record<string, number>> {
  const response = await request('/api/resources/counts');
  if (!response.ok) {
    throw new Error(`resource counts request failed with status ${response.status}`);
  }
  return parseCounts(await response.json());
}
