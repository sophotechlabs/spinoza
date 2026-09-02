import { expect, holdSide, sideAuthed, test } from '../harness/test';
import { openHome, openResource, selectRow } from '../harness/app';
import { kubectl } from '../harness/cluster';
import { CONTEXT } from '../harness/paths';
import type { Page } from '@playwright/test';
import type { Held } from '../harness/keepalive';

let releaseGuarded: Held;

test.beforeAll(() => {
  releaseGuarded = holdSide('guarded');
});

test.afterAll(async () => {
  await releaseGuarded.close();
});

async function openSettings(page: Page): Promise<void> {
  await openHome(page);
  await page.getByRole('button', { name: 'Settings' }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
}

async function openSection(page: Page, section: string): Promise<void> {
  await openSettings(page);
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: section, exact: true })
    .click();
}

function settingsWrite(page: Page) {
  return page.waitForResponse(
    (response) => response.url().includes('/api/settings') && response.request().method() === 'PUT',
    { timeout: 30_000 },
  );
}

function surface(page: Page): Promise<string> {
  return page.evaluate(() => {
    const root = document.documentElement;
    const style = getComputedStyle(root);
    return `${root.dataset.theme ?? ''}|${style.getPropertyValue('--surface').trim()}|${style.getPropertyValue('--fg').trim()}`;
  });
}

function differentOption(current: string, choices: string[], excluded = ''): string {
  for (const choice of choices) {
    if (choice !== current && choice !== excluded) {
      return choice;
    }
  }
  throw new Error(`no option differs from ${current}`);
}

async function restoreTheme(page: Page, preference: string): Promise<void> {
  if (!(await page.getByRole('dialog', { name: 'Settings' }).isVisible())) {
    await openSettings(page);
  }
  const themes = page.getByRole('combobox', { name: 'Theme preference' });
  if ((await themes.inputValue()) === preference) {
    return;
  }
  const saved = settingsWrite(page);
  await themes.selectOption(preference);
  await saved;
}

async function restoreSetting(
  page: Page,
  section: string,
  label: string,
  value: string,
): Promise<void> {
  await openSection(page, section);
  const control = page.getByLabel(label);
  if ((await control.inputValue()) === value) {
    return;
  }
  const saved = settingsWrite(page);
  await control.selectOption(value);
  await saved;
}

async function restoreCheckbox(
  page: Page,
  section: string,
  label: string,
  checked: boolean,
): Promise<void> {
  await openSection(page, section);
  const control = page.getByLabel(label);
  if ((await control.isChecked()) === checked) {
    return;
  }
  const saved = settingsWrite(page);
  await control.click();
  await saved;
  await expect(control).toBeChecked({ checked });
}

async function removeE2EColumn(page: Page): Promise<void> {
  await openSection(page, 'Columns');
  await page.getByLabel('Kind', { exact: true }).selectOption({ label: 'Pod' });
  const remove = page.getByRole('button', { name: 'Remove E2E app', exact: true });
  if ((await remove.count()) === 0) {
    return;
  }
  const saved = settingsWrite(page);
  await remove.click();
  await saved;
}

async function openSideClusterSettings(page: Page): Promise<void> {
  const dialog = page.getByRole('dialog', { name: 'Settings' });
  if (!(await dialog.isVisible())) {
    await page.getByRole('button', { name: 'Settings' }).click();
  }
  await expect(dialog).toBeVisible();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Cluster', exact: true })
    .click();
}

async function setSideNodeShell(page: Page, enabled: boolean): Promise<void> {
  await openSideClusterSettings(page);
  const control = page.getByLabel('Node shell');
  if ((await control.isChecked()) !== enabled) {
    const saved = settingsWrite(page);
    await control.click();
    await saved;
    await expect(control).toBeChecked({ checked: enabled });
  }
  await page
    .getByRole('dialog', { name: 'Settings' })
    .getByRole('button', { name: 'Close', exact: true })
    .click();
}

async function openSideNode(page: Page, name: string): Promise<void> {
  await page.goto(sideAuthed('guarded', `#context=${CONTEXT}&version=v1&resource=nodes&kind=Node`));
  await page.waitForFunction(() => document.title.startsWith('nodes'), undefined, {
    timeout: 60_000,
  });
  await selectRow(page, name);
  await page.getByRole('tab', { name: 'Overview', exact: true }).click();
}

async function nodeShellSupport(page: Page, name: string) {
  return page.evaluate(async (node) => {
    const params = new URLSearchParams({ node });
    const response = await fetch(`/api/nodeshell/support?${params.toString()}`);
    if (!response.ok) {
      throw new Error(`node shell support returned ${response.status}`);
    }
    return (await response.json()) as {
      node: string;
      enabled: boolean;
      allowed: boolean;
      reason?: string;
    };
  }, name);
}

test('the dialog opens on the cog and closes on escape', async ({ page }) => {
  await openSettings(page);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeHidden();
});

test('the dialog names every section it can show', async ({ page }) => {
  await openSettings(page);
  const sections = page.getByRole('navigation', { name: 'Settings sections' });
  for (const name of [
    'Appearance',
    'Cluster',
    'Columns',
    'Logs',
    'Terminal',
    'Panels',
    'Keyboard',
    'About',
  ]) {
    await expect(sections.getByRole('button', { name, exact: true })).toBeVisible();
  }
});

test('picking a theme repaints the page, not just the dropdown', async ({ page }) => {
  await openSettings(page);
  const themes = page.getByRole('combobox', { name: 'Theme preference' });
  const original = await themes.inputValue();
  const first = differentOption(original, ['light', 'matrix', 'nord']);
  const second = differentOption(original, ['light', 'matrix', 'nord'], first);
  try {
    const before = await surface(page);
    let saved = settingsWrite(page);
    await themes.selectOption(first);
    await saved;
    await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(before);
    const painted = await surface(page);
    saved = settingsWrite(page);
    await themes.selectOption(second);
    await saved;
    await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(painted);
  } finally {
    await restoreTheme(page, original);
  }
});

test('the theme survives a reload because spinoza kept it', async ({ page }) => {
  await openSettings(page);
  const themes = page.getByRole('combobox', { name: 'Theme preference' });
  const original = await themes.inputValue();
  const changed = differentOption(original, ['nord', 'light']);
  try {
    const saved = settingsWrite(page);
    await themes.selectOption(changed);
    const painted = await surface(page);
    await saved;
    await page.reload();
    await page.waitForLoadState('domcontentloaded');
    await expect.poll(() => surface(page), { timeout: 20_000 }).toBe(painted);
    await page.getByRole('button', { name: 'Settings' }).click();
    await expect(page.getByRole('combobox', { name: 'Theme preference' })).toHaveValue(changed);
  } finally {
    await restoreTheme(page, original);
  }
});

test('a theme pasted as json is applied and survives a reload', async ({ page }) => {
  await openSettings(page);
  const themes = page.getByRole('combobox', { name: 'Theme preference' });
  const original = await themes.inputValue();
  const before = await surface(page);
  try {
    await page
      .getByRole('textbox', { name: 'Import a theme' })
      .fill('{"id":"e2e-ink","name":"E2E Ink","base":"dark","tokens":{"surface":"#123456"}}');
    const saved = settingsWrite(page);
    await page.getByRole('button', { name: 'Import', exact: true }).click();
    await saved;
    await expect(themes).toContainText('E2E Ink', { timeout: 20_000 });
    await expect(themes).toHaveValue('e2e-ink');
    await expect.poll(() => surface(page), { timeout: 20_000 }).not.toBe(before);
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await expect(themes).toHaveValue('e2e-ink');
    await expect(themes).toContainText('E2E Ink');
  } finally {
    const remove = page.getByRole('button', { name: 'Remove E2E Ink', exact: true });
    const needsRemove = (await remove.count()) > 0;
    const needsRestore = (await themes.inputValue()) !== original;
    if (needsRemove || needsRestore) {
      const saved = settingsWrite(page);
      if (needsRestore) {
        await themes.selectOption(original);
      }
      if (needsRemove) {
        await remove.click();
      }
      await saved;
    }
  }
});

test('a theme that is not a theme is refused rather than applied', async ({ page }) => {
  await openSettings(page);
  await page.getByRole('textbox', { name: 'Import a theme' }).fill('{"nope":true}');
  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Settings' })).toBeVisible();
  await expect(page.getByRole('combobox', { name: 'Theme preference' })).not.toContainText('nope');
});

test('invalid theme json says what is wrong without changing the theme', async ({ page }) => {
  await openSettings(page);
  const before = await surface(page);
  await page.getByRole('textbox', { name: 'Import a theme' }).fill('{not json');
  await page.getByRole('button', { name: 'Import', exact: true }).click();
  await expect(page.getByText('that is not valid JSON', { exact: true })).toBeVisible();
  await expect.poll(() => surface(page)).toBe(before);
});

test('removing an imported file theme selects a built-in fallback that survives reload', async ({
  page,
}) => {
  await openSettings(page);
  const themes = page.getByRole('combobox', { name: 'Theme preference' });
  const original = await themes.inputValue();
  const existing = await themes
    .locator('option')
    .evaluateAll((options) => options.map((option) => (option as HTMLOptionElement).value));
  try {
    const saved = settingsWrite(page);
    await page.getByLabel('Theme file').setInputFiles({
      name: 'e2e-file-theme.json',
      mimeType: 'application/json',
      buffer: Buffer.from(
        JSON.stringify({
          id: 'e2e-file-theme',
          name: 'E2E File Theme',
          base: 'dark',
          tokens: { surface: '#203040' },
        }),
      ),
    });
    await saved;
    await expect(themes).toHaveValue('e2e-file-theme', { timeout: 30_000 });
    await expect(themes).toContainText('E2E File Theme');
    const removed = settingsWrite(page);
    await page.getByRole('button', { name: 'Remove E2E File Theme', exact: true }).click();
    await removed;
    await expect(themes).not.toContainText('E2E File Theme');
    const fallback = await themes.inputValue();
    expect(existing).toContain(fallback);
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await expect(themes).toHaveValue(fallback);
  } finally {
    const remove = page.getByRole('button', { name: 'Remove E2E File Theme', exact: true });
    const needsRemove = (await remove.count()) > 0;
    const needsRestore = (await themes.inputValue()) !== original;
    if (needsRemove || needsRestore) {
      const saved = settingsWrite(page);
      if (needsRestore) {
        await themes.selectOption(original);
      }
      if (needsRemove) {
        await remove.click();
      }
      await saved;
    }
  }
});

test('the default log presentation survives a new document', async ({ page }) => {
  await openSection(page, 'Logs');
  const control = page.getByLabel('Default log view');
  const original = await control.inputValue();
  const changed = differentOption(original, ['raw', 'pretty']);
  try {
    const saved = settingsWrite(page);
    await control.selectOption(changed);
    await saved;
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await page.getByRole('button', { name: 'Logs', exact: true }).click();
    await expect(page.getByLabel('Default log view')).toHaveValue(changed);
  } finally {
    await restoreSetting(page, 'Logs', 'Default log view', original);
  }
});

test('the audit refresh interval is stored by the server', async ({ page }) => {
  await openSection(page, 'Cluster');
  const control = page.getByLabel('Check refresh interval');
  const original = await control.inputValue();
  const changed = differentOption(original, ['15', '30', '60', '300']);
  try {
    const saved = settingsWrite(page);
    await control.selectOption(changed);
    await saved;
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await page.getByRole('button', { name: 'Cluster', exact: true }).click();
    await expect(page.getByLabel('Check refresh interval')).toHaveValue(changed);
  } finally {
    await restoreSetting(page, 'Cluster', 'Check refresh interval', original);
  }
});

test('the node shell setting changes the live backend capability and survives reload', async ({
  browser,
}) => {
  const context = await browser.newContext();
  const page = await context.newPage();
  const node = kubectl(['get', 'nodes', '-o', 'jsonpath={.items[0].metadata.name}']).trim();
  let original: boolean | undefined;
  try {
    await openSideNode(page, node);
    await openSideClusterSettings(page);
    original = await page.getByLabel('Node shell').isChecked();
    await page
      .getByRole('dialog', { name: 'Settings' })
      .getByRole('button', { name: 'Close', exact: true })
      .click();

    await setSideNodeShell(page, false);
    const disabled = await nodeShellSupport(page, node);
    expect(disabled).toMatchObject({ node, enabled: false, allowed: false });
    expect(disabled.reason).toContain('turn them on in Settings under Cluster');
    const shell = page.getByRole('button', { name: 'Node shell', exact: true });
    await expect(shell).toBeDisabled({ timeout: 30_000 });
    await expect(shell.locator('..')).toHaveAttribute(
      'title',
      /turn them on in Settings under Cluster/,
    );

    await setSideNodeShell(page, true);
    const enabled = await nodeShellSupport(page, node);
    expect(enabled).toMatchObject({ node, enabled: true, allowed: true });
    await expect(shell).toBeEnabled({ timeout: 30_000 });

    await page.reload();
    await openSideNode(page, node);
    await expect(page.getByRole('button', { name: 'Node shell', exact: true })).toBeEnabled({
      timeout: 30_000,
    });
    await openSideClusterSettings(page);
    await expect(page.getByLabel('Node shell')).toBeChecked();
    await page
      .getByRole('dialog', { name: 'Settings' })
      .getByRole('button', { name: 'Close', exact: true })
      .click();
  } finally {
    try {
      if (original !== undefined) {
        await setSideNodeShell(page, original);
      }
    } finally {
      await context.close();
    }
  }
});

test('screen reader mode survives a reload and can be restored', async ({ page }) => {
  await openSection(page, 'Terminal');
  const control = page.getByLabel('Screen reader mode');
  const before = await control.isChecked();
  try {
    const saved = settingsWrite(page);
    await control.click();
    await saved;
    await expect(control).toBeChecked({ checked: !before });
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await page.getByRole('button', { name: 'Terminal', exact: true }).click();
    await expect(page.getByLabel('Screen reader mode')).toBeChecked({ checked: !before });
  } finally {
    await restoreCheckbox(page, 'Terminal', 'Screen reader mode', before);
  }
});

test('the update check preference survives a reload and can be restored', async ({ page }) => {
  await openSection(page, 'About');
  const control = page.getByLabel('Check for updates');
  const before = await control.isChecked();
  try {
    const saved = settingsWrite(page);
    await control.click();
    await saved;
    await expect(control).toBeChecked({ checked: !before });
    await page.reload();
    await page.getByRole('button', { name: 'Settings' }).click();
    await page.getByRole('button', { name: 'About', exact: true }).click();
    await expect(page.getByLabel('Check for updates')).toBeChecked({ checked: !before });
  } finally {
    await restoreCheckbox(page, 'About', 'Check for updates', before);
  }
});

test('about reports the version served by the backend', async ({ page }) => {
  await openSection(page, 'About');
  const version = await page.evaluate(async () => {
    const response = await fetch('/api/version');
    const body = (await response.json()) as { version: string };
    return body.version;
  });
  expect(version).not.toBe('');
  const dialog = page.getByRole('dialog', { name: 'Settings' });
  const backend = dialog.getByText('Backend', { exact: true }).locator('..');
  await expect(backend).toContainText(version, { timeout: 30_000 });
  await expect(dialog.getByRole('button', { name: 'Update', exact: true })).toBeVisible();
});

test('the namespace start preference changes the live cluster scope', async ({ page }) => {
  await openSection(page, 'Cluster');
  const control = page.getByLabel('Namespace to open on');
  const original = await control.inputValue();
  const changed = differentOption(original, ['default', 'all']);
  try {
    const saved = settingsWrite(page);
    await control.selectOption(changed);
    await saved;
    const namespace = page.getByRole('combobox', { name: 'Namespace', exact: true });
    if (changed === 'default') {
      await expect(namespace).toHaveValue('default');
    }
    if (changed === 'all') {
      await expect(namespace).toHaveValue('');
    }
  } finally {
    await restoreSetting(page, 'Cluster', 'Namespace to open on', original);
  }
});

test('a custom column reaches the real resource table and can be removed', async ({ page }) => {
  await removeE2EColumn(page);
  try {
    await page.getByLabel('Column name').fill('E2E app');
    await page.getByLabel('Field path').fill('.metadata.labels.app');
    const saved = settingsWrite(page);
    await page.getByRole('button', { name: 'Add', exact: true }).click();
    await saved;
    await page.getByRole('button', { name: 'Close', exact: true }).click();
    await openResource(page, 'pods', 'Pod');
    await expect(page.getByRole('columnheader', { name: /E2E app/ })).toBeVisible({
      timeout: 60_000,
    });
    const healthy = page.locator('main tbody tr').filter({ hasText: 'healthy-' }).first();
    await expect(healthy).toContainText('healthy');
  } finally {
    await removeE2EColumn(page);
  }
});

test('the keyboard section reports the shortcuts that work in the application', async ({
  page,
}) => {
  await openSection(page, 'Keyboard');
  const table = page.getByRole('table', { name: 'Keyboard shortcuts' });
  await expect(table).toContainText('Open the command palette');
  await expect(table).toContainText('Jump to the resource filter');
  await expect(table).toContainText('Close the palette or dialog, then the inspector');
});
