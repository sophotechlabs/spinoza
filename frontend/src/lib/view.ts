import type { ViewState, ViewSwitch } from './types';
import { VIEWS } from './types';
import { failure } from './object';
import { request } from './http';

export const DESKTOP = 'desktop';

export const BROWSER = 'browser';

declare global {
  interface Window {
    __SPINOZA_VIEW__?: string;
    __SPINOZA_START__?: { view?: string; context?: string };
  }
}

export function viewKind(): string {
  if (window.__SPINOZA_VIEW__ === DESKTOP) {
    return DESKTOP;
  }
  return BROWSER;
}

export function inDesktopWindow(): boolean {
  return viewKind() === DESKTOP;
}

// A window with no address bar cannot be pointed at a route, so a run may name
// one to open on. It only applies when nothing else has asked: a hash the user
// arrived with always wins, and a view this build does not know is dropped.
export function startRoute(): string {
  if (window.location.hash !== '') {
    return '';
  }
  const asked = window.__SPINOZA_START__;
  if (asked === undefined) {
    return '';
  }
  const params = new URLSearchParams();
  if (typeof asked.context === 'string' && asked.context !== '') {
    params.set('context', asked.context);
  }
  if (typeof asked.view === 'string' && (VIEWS as readonly string[]).includes(asked.view)) {
    params.set('view', asked.view);
  }
  const query = params.toString();
  if (query === '') {
    return '';
  }
  return `#${query}`;
}

export async function fetchView(): Promise<ViewState> {
  const response = await request('/api/view');
  if (!response.ok) {
    throw await failure(response, `the view request failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<ViewState>;
  return { window: body.window === true, hidden: body.hidden === true };
}

async function move(where: string): Promise<ViewSwitch> {
  const response = await request(`/api/view/${where}`, { method: 'POST', timeoutMs: 30000 });
  if (!response.ok) {
    throw await failure(response, `switching failed with status ${response.status}`);
  }
  const body = (await response.json()) as Partial<ViewSwitch>;
  return { switched: body.switched === true, reason: body.reason };
}

export async function moveToBrowser(): Promise<ViewSwitch> {
  return move(BROWSER);
}

export async function moveToDesktop(): Promise<ViewSwitch> {
  return move(DESKTOP);
}
