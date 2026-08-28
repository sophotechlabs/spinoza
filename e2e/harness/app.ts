import type { Page } from '@playwright/test';
import { CONTEXT } from './paths';
import { authed } from './test';

export function viewHash(view: string): string {
  return `#context=${CONTEXT}&view=${view}`;
}

export function resourceHash(resource: string, kind: string, version = 'v1'): string {
  return `#context=${CONTEXT}&version=${version}&resource=${resource}&kind=${kind}`;
}

async function settle(page: Page, wanted: string): Promise<void> {
  await page.waitForFunction((prefix) => document.title.startsWith(prefix), wanted, {
    timeout: 60_000,
  });
}

export async function openView(page: Page, view: string): Promise<void> {
  await page.goto(authed(viewHash(view)));
  await settle(page, view);
}

export async function openResource(
  page: Page,
  resource: string,
  kind: string,
  version = 'v1',
): Promise<void> {
  await page.goto(authed(resourceHash(resource, kind, version)));
  await settle(page, resource);
}

export async function openHome(page: Page): Promise<void> {
  await page.goto(authed(''));
  await page.waitForLoadState('domcontentloaded');
}

export function sidebar(page: Page, label: string | RegExp) {
  if (typeof label === 'string') {
    return page.getByRole('button', { name: label, exact: true });
  }
  return page.getByRole('button', { name: label });
}
