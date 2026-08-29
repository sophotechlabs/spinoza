import { test as base } from '@playwright/test';
import type { Browser } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { BASE_URL, CONTEXT, STATE_FILE, STORAGE_STATE } from './paths';

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

export async function holdSide(browser: Browser, name: string): Promise<() => Promise<void>> {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(sideAuthed(name, ''));
  await page.waitForLoadState('domcontentloaded');
  return async () => {
    await context.close();
  };
}

export const test = base.extend<{}, { view: void }>({
  view: [
    async ({ browser }, use) => {
      const context = await browser.newContext({ storageState: STORAGE_STATE });
      const page = await context.newPage();
      await page.goto(
        authed(`#context=${CONTEXT}&version=v1&resource=pods&kind=Pod&namespace=e2e&name=noshell`),
      );
      await page.waitForLoadState('domcontentloaded');
      const proof = await page.evaluate(async () => {
        const response = await fetch('/api/version');
        return response.status;
      });
      if (proof !== 200) {
        throw new Error(`the keep-alive view is not authenticated: /api/cluster answered ${String(proof)}`);
      }
      await page
        .getByRole('tab', { name: 'Overview' })
        .first()
        .waitFor({ state: 'visible', timeout: 60_000 })
        .catch(() => undefined);
      await use();
      await context.close();
    },
    { scope: 'worker', auto: true },
  ],
});

export { expect } from '@playwright/test';
