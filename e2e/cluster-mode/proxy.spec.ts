import { expect, test } from '@playwright/test';
import { api, BASE_URL, kubectl, openFeed, session, signIn } from './harness';

test('oauth2-proxy supplies the signed keycloak identity', async ({ page }) => {
  await signIn(page, 'alice');
  const current = await session(page);
  expect(current.user).toBe('alice');
  expect(current.role).toBe('admin');
  expect(current.mode).toBe('proxy');
  const forged = await session(page, {
    'X-Forwarded-User': 'mallory',
    'X-Forwarded-Groups': 'guests',
    'X-Spinoza-Proxy-Secret': 'not-the-shared-secret',
  });
  expect(forged.user).toBe('alice');
  expect(forged.role).toBe('admin');
  const underscored = await session(page, {
    X_Forwarded_User: 'mallory',
    X_Forwarded_Groups: 'guests',
    X_Spinoza_Proxy_Secret: 'not-the-shared-secret',
  });
  expect(underscored.user).toBe('alice');
  expect(underscored.role).toBe('admin');
  await openFeed(page);
});

test('oauth2-proxy preserves namespace scope', async ({ page }) => {
  await signIn(page, 'carol');
  const current = await session(page);
  expect(current.role).toBe('viewer');
  expect(current.scope.everywhere).toBe(false);
  expect(current.scope.namespaces).toEqual(['payments']);
  const namespaces = await api(page, 'GET', '/api/namespaces');
  expect(namespaces.status).toBe(200);
  expect(namespaces.body).toContain('payments');
  expect(namespaces.body).not.toContain('storefront');
});

test('oauth2-proxy identity controls kubernetes writes', async ({ page }) => {
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

test('signing out clears the oauth2-proxy session', async ({ page, context }) => {
  await signIn(page, 'alice');
  await page.locator('summary[aria-label="Account"]').click();
  await page.getByRole('button', { name: 'Sign out' }).click();
  await expect
    .poll(async () => {
      const auth = await context.request.get(`${BASE_URL}/oauth2/auth`, { maxRedirects: 0 });
      return auth.status();
    })
    .toBe(401);
  await page.goto(BASE_URL);
  await expect(page.locator('#username')).toBeVisible();
});
