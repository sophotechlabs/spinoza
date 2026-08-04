import type { ResourceCatalog, ResourceCounts } from './types';

async function request(method: string): Promise<ResourceCatalog> {
  const response = await fetch('/api/resources', { method });
  if (!response.ok) {
    throw new Error(`discovery request failed with status ${response.status}`);
  }
  return (await response.json()) as ResourceCatalog;
}

export async function fetchResources(): Promise<ResourceCatalog> {
  return request('GET');
}

export async function refreshResources(): Promise<ResourceCatalog> {
  return request('POST');
}

export async function fetchResourceCounts(): Promise<Record<string, number>> {
  const response = await fetch('/api/resources/counts');
  if (!response.ok) {
    throw new Error(`resource counts request failed with status ${response.status}`);
  }
  const data = (await response.json()) as Partial<ResourceCounts>;
  return data.counts ?? {};
}
