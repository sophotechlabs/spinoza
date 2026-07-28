import type { ResourceCatalog } from './types';

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
