import { defineConfig } from '@playwright/test';
import type { ReporterDescription } from '@playwright/test';

const browser = process.env.SPINOZA_CM_BROWSER;
const mode = process.env.SPINOZA_CM_AUTH_MODE;
let baseURL = process.env.SPINOZA_CM_BASE_URL;
if (baseURL === undefined || baseURL === '') {
  baseURL = 'https://spinoza.localtest.me:8443';
}
const isCI = process.env.CI !== undefined;

if (browser === undefined || !['chromium', 'firefox', 'webkit'].includes(browser)) {
  throw new Error(`SPINOZA_CM_BROWSER is ${String(browser)}, want chromium, firefox or webkit`);
}
if (mode === undefined || !['oidc', 'proxy'].includes(mode)) {
  throw new Error(`SPINOZA_CM_AUTH_MODE is ${String(mode)}, want oidc or proxy`);
}

function reporters(): ReporterDescription[] {
  if (!isCI) {
    return [['list'], ['html', { open: 'never' }]];
  }
  return [
    ['github'],
    ['html', { open: 'never' }],
    ['junit', { outputFile: 'test-results/cluster-mode-junit.xml' }],
  ];
}

export default defineConfig({
  testDir: './cluster-mode',
  testMatch: `${mode}.spec.ts`,
  workers: 1,
  fullyParallel: false,
  forbidOnly: isCI,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 30_000 },
  reporter: reporters(),
  projects: [
    {
      name: browser,
      use: { browserName: browser as 'chromium' | 'firefox' | 'webkit' },
    },
  ],
  use: {
    baseURL,
    ignoreHTTPSErrors: true,
    viewport: { width: 1600, height: 1000 },
    actionTimeout: 30_000,
    navigationTimeout: 60_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
});
