import { expect, test } from '../harness/test';
import { openView } from '../harness/app';

test('the graph draws what manages what, not just a legend', async ({ page }) => {
  await openView(page, 'gitops');
  await expect
    .poll(() => page.locator('.react-flow__node').count(), { timeout: 120_000 })
    .toBeGreaterThan(1);
  await expect
    .poll(() => page.locator('.react-flow__edge').count(), { timeout: 120_000 })
    .toBeGreaterThan(0);
});

test('an edge names the source it comes from and the applier it feeds', async ({ page }) => {
  await openView(page, 'gitops');
  const edge = page.getByRole('group', {
    name: /^Edge from source\.toolkit\.fluxcd\.io\/GitRepository\/.+ to kustomize\.toolkit\.fluxcd\.io\/Kustomization\/.+$/,
  });
  await expect(edge.first()).toBeAttached({ timeout: 120_000 });
});

test('both controllers land in one graph', async ({ page }) => {
  await openView(page, 'gitops');
  const main = page.locator('main');
  await expect(main).toContainText('podinfo', { timeout: 120_000 });
  await expect(main).toContainText('guestbook');
});

test('the graph says what its colours and edges mean', async ({ page }) => {
  await openView(page, 'gitops');
  const main = page.locator('main');
  await expect(main).toContainText('Manages', { timeout: 120_000 });
  await expect(main).toContainText('Depends on');
  await expect(main).toContainText('Source, not ready yet');
});
