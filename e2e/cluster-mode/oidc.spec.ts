import { expect, test } from '@playwright/test';
import {
  api,
  authenticated,
  BASE_URL,
  endProviderSession,
  feedClosed,
  kubectl,
  openFeed,
  providerSessionID,
  restartSpinoza,
  session,
  signIn,
} from './harness';

test('native oidc turns anonymous browsers away', async ({ page }) => {
  await page.goto(BASE_URL);
  await expect(page.getByTestId('sign-in')).toBeVisible();
  const result = await api(page, 'GET', '/api/overview');
  expect(result.status).toBe(401);
  expect(result.body).toContain('sign in');
});

test('an administrator signs in through keycloak', async ({ page }) => {
  await signIn(page, 'alice');
  const current = await session(page);
  expect(current.user).toBe('alice');
  expect(current.role).toBe('admin');
  expect(current.mode).toBe('oidc');
  expect(current.scope.everywhere).toBe(true);
  await page.locator('summary[aria-label="Account"]').click();
  await expect(page.getByText('Role admin, reading every namespace.')).toBeVisible();
  const overview = await api(page, 'GET', '/api/overview');
  expect(overview.status).toBe(200);
});

test('a namespace reader sees only that namespace', async ({ page }) => {
  await signIn(page, 'carol');
  const current = await session(page);
  expect(current.role).toBe('viewer');
  expect(current.scope.everywhere).toBe(false);
  expect(current.scope.namespaces).toEqual(['payments']);
  const namespaces = await api(page, 'GET', '/api/namespaces');
  expect(namespaces.status).toBe(200);
  expect(namespaces.body).toContain('payments');
  expect(namespaces.body).not.toContain('storefront');
  const checks = await api(page, 'GET', '/api/checks');
  expect(checks.status).toBe(403);
  expect(checks.body).toContain('reads the whole cluster');
  await openFeed(page);
});

test('an editor can write only where kubernetes binds them', async ({ page }) => {
  await signIn(page, 'bob');
  try {
    const allowed = await api(
      page,
      'POST',
      '/api/action?action=scale&group=apps&version=v1&resource=deployments&namespace=payments&name=web&replicas=2',
    );
    expect(allowed.status).toBe(200);
    const denied = await api(
      page,
      'POST',
      '/api/action?action=scale&group=apps&version=v1&resource=deployments&namespace=default&name=other&replicas=2',
    );
    expect(denied.status).toBe(403);
    expect(denied.body).toContain('User \\"bob\\"');
  } finally {
    kubectl(['-n', 'payments', 'scale', 'deployment/web', '--replicas=1']);
  }
});

test('personal settings stay isolated through a pod replacement', async ({ browser }) => {
  const aliceContext = await browser.newContext({ ignoreHTTPSErrors: true });
  const bobContext = await browser.newContext({ ignoreHTTPSErrors: true });
  const alice = await aliceContext.newPage();
  const bob = await bobContext.newPage();
  try {
    await signIn(alice, 'alice');
    await signIn(bob, 'bob');
    const aliceWrite = await api(
      alice,
      'PUT',
      '/api/settings',
      '{"values":{"spinoza.theme.v1":"borg"}}',
      { 'Content-Type': 'application/json' },
    );
    expect(aliceWrite.status).toBe(200);
    const bobWrite = await api(
      bob,
      'PUT',
      '/api/settings',
      '{"values":{"spinoza.theme.v1":"matrix"}}',
      { 'Content-Type': 'application/json' },
    );
    expect(bobWrite.status).toBe(200);
    restartSpinoza();
    await expect.poll(() => authenticated(alice)).toBe(true);
    await expect.poll(() => authenticated(bob)).toBe(true);
    const aliceSettings = await api(alice, 'GET', '/api/settings');
    expect(aliceSettings.body).toContain('borg');
    expect(aliceSettings.body).not.toContain('matrix');
    const bobSettings = await api(bob, 'GET', '/api/settings');
    expect(bobSettings.body).toContain('matrix');
    expect(bobSettings.body).not.toContain('borg');
  } finally {
    await aliceContext.close();
    await bobContext.close();
  }
});

test('provider logout ends the browser session and feed', async ({ page }) => {
  await signIn(page, 'bob');
  await openFeed(page);
  endProviderSession(await providerSessionID(page));
  await expect.poll(() => authenticated(page), { timeout: 120_000 }).toBe(false);
  await expect.poll(() => feedClosed(page), { timeout: 120_000 }).toBe(true);
  await page.reload();
  await expect(page.getByTestId('sign-in')).toBeVisible();
});

test('signing out reaches keycloak and clears the spinoza session', async ({ page }) => {
  await signIn(page, 'alice');
  await page.locator('summary[aria-label="Account"]').click();
  const providerLogout = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.hostname === 'keycloak.localtest.me' &&
      url.pathname.endsWith('/protocol/openid-connect/logout')
    );
  });
  await page.getByRole('button', { name: 'Sign out' }).click();
  await providerLogout;
  await expect(page).toHaveURL(/keycloak\.localtest\.me/);
  const logout = page.locator('#kc-logout');
  await expect(logout).toBeVisible();
  await logout.click();
  await expect(page).toHaveURL(`${BASE_URL}/`);
  await expect(page.getByTestId('sign-in')).toBeVisible();
  await page.getByTestId('sign-in').click();
  await expect(page.locator('#username')).toBeVisible();
});
