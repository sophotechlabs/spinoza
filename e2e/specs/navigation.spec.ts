import { expect, test } from '../harness/test';
import { CONTEXT } from '../harness/paths';
import { openHome, openView, sidebar } from '../harness/app';

const VIEWS = [
  { label: 'Cluster Overview', title: 'kind-spinoza-e2e' },
  { label: 'Issues', title: 'issues' },
  { label: 'Topology', title: 'topology' },
  { label: 'Helm releases', title: 'helm' },
  { label: 'Cluster checks', title: 'checks' },
];

test('the app opens on the cluster it was pointed at', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('banner')).toContainText(CONTEXT);
  await expect(page).toHaveTitle(new RegExp(CONTEXT));
});

test('the app says the cluster feed is connected', async ({ page }) => {
  await openHome(page);
  await expect(
    page.getByRole('status', { name: 'The cluster feed is connected' }),
  ).toBeVisible();
});

for (const view of VIEWS) {
  test(`the sidebar reaches ${view.label} and the title follows`, async ({ page }) => {
    await openHome(page);
    await sidebar(page, view.label).click();
    await expect(page).toHaveTitle(new RegExp(`^${view.title} `));
  });
}

test('a view survives a reload because it lives in the URL', async ({ page }) => {
  await openView(page, 'checks');
  await page.reload();
  await expect(page).toHaveTitle(/^checks /);
});

test('a view spinoza cannot serve is offered but refused', async ({ page }) => {
  await openHome(page);
  for (const label of ['Traffic', 'Flux', 'Argo CD']) {
    await expect(sidebar(page, label)).toBeDisabled();
  }
});

test('the desktop switch explains why it is unavailable in a browser', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, 'Desktop')).toBeDisabled();
  await expect(page.getByRole('banner')).toContainText('only available when Spinoza starts');
});

test('every namespace in the cluster is offered for scoping', async ({ page }) => {
  await openHome(page);
  const picker = page.getByRole('combobox', { name: 'Namespace' });
  await expect(picker).toBeVisible();
  await expect(picker.getByRole('option', { name: 'e2e' })).toHaveCount(1);
  await expect(picker.getByRole('option', { name: 'All namespaces' })).toHaveCount(1);
});

test('the command palette opens on its shortcut', async ({ page }) => {
  await openHome(page);
  await page.keyboard.press('ControlOrMeta+k');
  await expect(page.getByPlaceholder('Search')).toBeVisible();
});

test('the page offers a skip link into its content', async ({ page }) => {
  await openHome(page);
  await expect(page.getByRole('link', { name: 'Skip to the content' })).toHaveAttribute(
    'href',
    '#content',
  );
});

test('the resource tree counts what it found', async ({ page }) => {
  await openHome(page);
  await expect(sidebar(page, /^Pod \d/)).toBeVisible();
  await expect(sidebar(page, /^Deployment \d/)).toBeVisible();
  await expect(sidebar(page, /^CronJob \d/)).toBeVisible();
});
