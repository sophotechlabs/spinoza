import { test as base } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { BASE_URL, STATE_FILE } from './paths';
import { hold, holdMain } from './keepalive';
import type { Held } from './keepalive';

export interface Side {
  pid: number;
  addr: string;
  baseURL: string;
  token: string;
}

interface State {
  pid: number;
  baseURL: string;
  token: string;
  sides: Record<string, Side>;
}

export function state(): State {
  return JSON.parse(readFileSync(STATE_FILE, 'utf8')) as State;
}

export function authed(hash: string): string {
  return `${BASE_URL}/?token=${state().token}${hash}`;
}

export function side(name: string): Side {
  const found = state().sides[name];
  if (found === undefined) {
    throw new Error(`no side instance named ${name}`);
  }
  return found;
}

export function sideAuthed(name: string, hash: string): string {
  const one = side(name);
  return `${one.baseURL}/?token=${one.token}${hash}`;
}

export function holdSide(name: string): Held {
  const one = side(name);
  return hold(one.baseURL, one.token);
}

export const test = base.extend<{}, { view: void }>({
  view: [
    async ({}, use) => {
      const held = holdMain(state().token);
      await use();
      await held.close();
    },
    { scope: 'worker', auto: true },
  ],
});

export { expect } from '@playwright/test';
