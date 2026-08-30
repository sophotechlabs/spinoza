import type { ObjectRef, SearchHit, SearchResults } from './types';
import { failure } from './object';
import { request } from './http';

export const SHORTEST_QUERY = 2;

export const SEARCH_DELAY_MS = 250;

function hitOf(raw: unknown): SearchHit {
  const item = raw as Partial<SearchHit>;
  return {
    group: item.group ?? '',
    version: item.version ?? '',
    resource: item.resource ?? '',
    kind: item.kind ?? '',
    namespace: item.namespace ?? '',
    name: item.name ?? '',
    cluster: item.cluster,
  };
}

export function worthSearching(query: string): boolean {
  return query.trim().length >= SHORTEST_QUERY;
}

export function refOf(hit: SearchHit): ObjectRef {
  return {
    group: hit.group,
    version: hit.version,
    resource: hit.resource,
    namespace: hit.namespace,
    name: hit.name,
  };
}

export async function searchObjects(query: string, fleet = false): Promise<SearchResults> {
  const params = new URLSearchParams({ q: query.trim() });
  const where = fleet ? '/api/search/fleet' : '/api/search';
  const response = await request(`${where}?${params.toString()}`);
  if (!response.ok) {
    throw await failure(response, `search failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<SearchResults>;
  return {
    hits: (body.hits ?? []).map(hitOf),
    truncated: body.truncated === true,
    errors: body.errors,
  };
}
