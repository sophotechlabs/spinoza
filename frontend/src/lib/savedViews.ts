import type { SavedView, SavedViews } from './types';
import { request } from './http';
import { failure } from './object';

export async function fetchSavedViews(): Promise<SavedViews> {
  const response = await request('/api/views');
  if (!response.ok) {
    throw await failure(response, `saved views failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<SavedViews>;
  return { views: body.views ?? [], mayShare: body.mayShare === true };
}

export async function saveView(view: SavedView): Promise<SavedView> {
  const response = await request('/api/views', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(view),
  });
  if (!response.ok) {
    throw await failure(response, `saving that view failed with status ${response.status}`);
  }
  return (await response.json()) as SavedView;
}

export async function forgetView(id: string, shared: boolean): Promise<void> {
  const params = new URLSearchParams({ id });
  if (shared) {
    params.set('shared', 'true');
  }
  const response = await request(`/api/views?${params.toString()}`, { method: 'DELETE' });
  if (!response.ok) {
    throw await failure(response, `forgetting that view failed with status ${response.status}`);
  }
}

export function describeView(view: SavedView): string {
  const parts: string[] = [];
  if (view.resource !== undefined && view.resource !== '') {
    parts.push(view.resource);
  }
  if (view.namespace !== undefined && view.namespace !== '') {
    parts.push(`in ${view.namespace}`);
  }
  if (view.filter !== undefined && view.filter !== '') {
    parts.push(`matching ${view.filter}`);
  }
  if (parts.length === 0) {
    return view.view;
  }
  return parts.join(' ');
}
