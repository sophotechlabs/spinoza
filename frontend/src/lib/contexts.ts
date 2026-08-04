import type { ContextList } from './types';
import { failure } from './object';
import { request } from './http';

interface WireContexts {
  contexts?: string[];
  current?: string;
  error?: string;
}

function normalize(body: WireContexts): ContextList {
  return {
    contexts: body.contexts ?? [],
    current: body.current ?? '',
    error: body.error,
  };
}

export async function fetchContexts(): Promise<ContextList> {
  const response = await request('/api/contexts');
  if (!response.ok) {
    throw await failure(response, `contexts request failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}

export async function switchContext(name: string): Promise<ContextList> {
  const params = new URLSearchParams({ name });
  const response = await request(`/api/contexts?${params.toString()}`, { method: 'POST' });
  if (!response.ok) {
    throw await failure(response, `switching context failed with status ${response.status}`);
  }
  return normalize((await response.json()) as WireContexts);
}
