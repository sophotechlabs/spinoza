import { test as base } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { BASE_URL, STATE_FILE } from './paths';

interface State {
  pid: number;
  baseURL: string;
  token: string;
}

export function state(): State {
  return JSON.parse(readFileSync(STATE_FILE, 'utf8')) as State;
}

export function authed(hash: string): string {
  return `${BASE_URL}/?token=${state().token}${hash}`;
}

export const test = base.extend<Record<string, never>, { view: void }>({
  view: [
    async ({ browser }, use) => {
      const context = await browser.newContext();
      const page = await context.newPage();
      await page.goto(authed(''));
      await page.waitForLoadState('domcontentloaded');
      await use();
      await context.close();
    },
    { scope: 'worker', auto: true },
  ],
});

export { expect } from '@playwright/test';
