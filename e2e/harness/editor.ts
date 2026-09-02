import type { Page } from '@playwright/test';
import { expect } from '@playwright/test';
import { primaryShortcut } from './keyboard';

export async function editorText(page: Page): Promise<string> {
  const raw = await page.locator('.view-lines').first().innerText();
  return raw.replace(/\u00a0/g, ' ');
}

export async function replaceEditor(page: Page, text: string): Promise<void> {
  await page
    .locator('.view-lines')
    .first()
    .click({ position: { x: 5, y: 5 } });
  await page.getByRole('textbox', { name: 'Editor content' }).focus();
  await primaryShortcut(page, 'a');
  await page.keyboard.press('Delete');
  await page.keyboard.insertText(text);
  await expect
    .poll(async () => (await editorText(page)).trim(), { timeout: 20_000 })
    .toBe(text.trim());
}
