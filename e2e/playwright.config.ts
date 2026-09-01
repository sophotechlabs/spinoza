import { defineConfig } from '@playwright/test';
import type { ReporterDescription } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { BASE_URL, STORAGE_STATE } from './harness/paths';

const isCI = process.env.CI !== undefined;

interface SuiteGroup {
  id: string;
  runner: string;
  specs: string[];
}

interface Suite {
  groups: SuiteGroup[];
}

function browserProjects() {
  const groupID = process.env.SPINOZA_E2E_GROUP;
  if (groupID === undefined || groupID === '') {
    return [
      { name: 'core', testDir: './specs' },
      { name: 'full', testDir: './specs-full' },
      {
        name: 'shots',
        testDir: './shots',
        use: { viewport: { width: 1780, height: 1000 }, deviceScaleFactor: 2 },
      },
    ];
  }
  const suite = JSON.parse(readFileSync(resolve(import.meta.dirname, 'suite.json'), 'utf8')) as Suite;
  const group = suite.groups.find((candidate) => candidate.id === groupID);
  if (group === undefined) {
    throw new Error(`unknown E2E group ${groupID}`);
  }
  if (group.runner !== 'playwright') {
    throw new Error(`E2E group ${groupID} uses ${group.runner}, not Playwright`);
  }
  let browser = 'chromium';
  if (process.env.SPINOZA_E2E_BROWSER !== undefined) {
    browser = process.env.SPINOZA_E2E_BROWSER;
  }
  if (!['chromium', 'firefox', 'webkit'].includes(browser)) {
    throw new Error(`unknown E2E browser ${browser}`);
  }
  return [
    {
      name: browser,
      testDir: '.',
      testMatch: group.specs,
      use: { browserName: browser as 'chromium' | 'firefox' | 'webkit' },
    },
  ];
}

function reporters(): ReporterDescription[] {
  if (!isCI) {
    return [['list'], ['html', { open: 'never' }]];
  }
  return [
    ['github'],
    ['html', { open: 'never' }],
    ['junit', { outputFile: 'test-results/junit.xml' }],
  ];
}

export default defineConfig({
  projects: browserProjects(),
  globalSetup: './harness/globalSetup.ts',
  globalTeardown: './harness/globalTeardown.ts',
  workers: 1,
  fullyParallel: false,
  forbidOnly: isCI,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 20_000 },
  reporter: reporters(),
  use: {
    baseURL: BASE_URL,
    storageState: STORAGE_STATE,
    viewport: { width: 1600, height: 1000 },
    actionTimeout: 20_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
});
