import { defineConfig } from '@playwright/test';
import { BASE_URL, STORAGE_STATE } from './harness/paths';

const isCI = process.env.CI !== undefined;

export default defineConfig({
  projects: [
    { name: 'core', testDir: './specs' },
    { name: 'full', testDir: './specs-full' },
  ],
  globalSetup: './harness/globalSetup.ts',
  globalTeardown: './harness/globalTeardown.ts',
  workers: 1,
  fullyParallel: false,
  forbidOnly: isCI,
  timeout: 90_000,
  expect: { timeout: 20_000 },
  reporter: isCI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: BASE_URL,
    storageState: STORAGE_STATE,
    viewport: { width: 1600, height: 1000 },
    actionTimeout: 20_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
});
