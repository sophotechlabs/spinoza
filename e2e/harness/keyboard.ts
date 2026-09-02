import type { Page } from '@playwright/test';

export async function primaryShortcut(page: Page, key: string): Promise<void> {
  const mac = await page.evaluate(() => navigator.userAgent.includes('Macintosh'));
  let modifier = 'Control';
  if (mac) {
    modifier = 'Meta';
  }
  await page.keyboard.press(`${modifier}+${key}`);
}
