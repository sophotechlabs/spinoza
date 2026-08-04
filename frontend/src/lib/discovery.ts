import type { ResourceCatalog, ResourceCounts } from './types';
import { request } from './http';

async function catalog(method: string): Promise<ResourceCatalog> {
  const response = await request('/api/resources', { method });
  if (!response.ok) {
    throw new Error(`discovery request failed with status ${response.status}`);
  }
  return (await response.json()) as ResourceCatalog;
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
  const data = (await response.json()) as Partial<ResourceCounts>;
  return data.counts ?? {};
}
