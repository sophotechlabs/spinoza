import { expect, test } from '../harness/test';
import { openHome, openView } from '../harness/app';
import type { Page } from '@playwright/test';

const FLOOR = { width: 1280, height: 720 };

const LAPTOP = { width: 1440, height: 900 };

const ZOOMED = { width: 640, height: 360 };

const VIEWS = ['resources', 'issues', 'checks', 'history'];

async function settled(page: Page): Promise<void> {
  const main = page.locator('main');
  await expect(main).not.toBeEmpty({ timeout: 60_000 });
  await expect(main).not.toContainText('Loading', { timeout: 60_000 });
}

async function overflow(page: Page): Promise<number> {
  return page.evaluate(() => {
    const root = document.documentElement;
    return root.scrollWidth - root.clientWidth;
  });
}

async function widest(page: Page): Promise<{ tag: string; width: number } | null> {
  return page.evaluate(() => {
    const room = document.documentElement.clientWidth;
    for (const node of Array.from(document.querySelectorAll('body *'))) {
      const box = node.getBoundingClientRect();
      if (box.right > room + 1 && box.width <= room) {
        return { tag: node.tagName.toLowerCase(), width: Math.round(box.width) };
      }
    }
    return null;
  });
}

async function walk(page: Page): Promise<void> {
  for (const view of VIEWS) {
    await openView(page, view);
    await settled(page);
    expect(
      await overflow(page),
      `${view} pushed the page sideways: ${JSON.stringify(await widest(page))}`,
    ).toBeLessThanOrEqual(1);
  }
}

test.describe('at the supported floor', () => {
  test.use({ viewport: FLOOR });

  test('every workspace fits 1280 by 720 without scrolling sideways', async ({ page }) => {
    await openHome(page);
    await settled(page);
    expect(
      await overflow(page),
      `the overview pushed the page sideways: ${JSON.stringify(await widest(page))}`,
    ).toBeLessThanOrEqual(1);
    await walk(page);
  });

  test('the chrome a person steers with is all on screen', async ({ page }) => {
    await openHome(page);
    await openView(page, 'resources');
    await settled(page);

    await expect(page.getByRole('navigation', { name: 'Clusters already open' })).toBeVisible();
    await expect(page.getByLabel('Kubernetes context')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Workflows' })).toBeVisible();
    await expect(page.getByRole('status', { name: 'The cluster feed is connected' })).toBeVisible();
    await expect(page.locator('main')).toBeVisible();
  });
});

test.describe('on a laptop screen', () => {
  test.use({ viewport: LAPTOP });

  test('every workspace fits 1440 by 900 without scrolling sideways', async ({ page }) => {
    await openHome(page);
    await settled(page);
    await walk(page);
  });
});

test.describe('at 200% browser zoom', () => {
  test.use({ viewport: ZOOMED, deviceScaleFactor: 2 });

  test('the whole workspace stays reachable by scrolling', async ({ page }) => {
    await openHome(page);
    await openView(page, 'resources');
    await settled(page);

    await expect(page.getByRole('heading', { name: 'Workflows' })).toBeVisible();
    await expect(page.locator('main')).toBeVisible();

    const reach = await page.evaluate(() => {
      const root = document.documentElement;
      return { scrollWidth: root.scrollWidth, clientWidth: root.clientWidth };
    });
    expect(reach.scrollWidth).toBeGreaterThanOrEqual(reach.clientWidth);
    expect(await page.evaluate(() => document.body.style.overflowX)).not.toBe('hidden');
  });
});
