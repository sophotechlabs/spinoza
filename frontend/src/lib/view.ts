import type { ViewState, ViewSwitch } from './types';
import { failure } from './object';
import { request } from './http';

export const DESKTOP = 'desktop';

export const BROWSER = 'browser';

declare global {
  interface Window {
    __SPINOZA_VIEW__?: string;
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
