import process from 'node:process'
import { defineConfig, devices } from '@playwright/test'
import { defineBddConfig } from 'playwright-bdd'

/**
 * Playwright + playwright-bdd for critical UI journeys.
 *
 * Prerequisites:
 * - API on http://localhost:8080 (`make backend`)
 * - SPA on http://localhost:3000 (auto-started via webServer unless already up)
 * - global-setup completes first-time install when needs_setup is true
 *   (defaults: e2e-owner@farplane.test / password1). Override with E2E_EMAIL
 *   and E2E_PASSWORD when the install is already configured.
 *
 * Org switch is not in the UI yet; organization.feature asserts the active
 * organization name in the sidebar after sign-in.
 */
const testDir = defineBddConfig({
  features: 'e2e/features/**/*.feature',
  steps: ['e2e/steps/**/*.ts', 'e2e/fixtures.ts'],
  outputDir: '.features-gen',
})

const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:3000'

export default defineConfig({
  testDir,
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['list'], ['html', { open: 'never' }]],
  timeout: 30_000,
  expect: {
    timeout: 5000,
  },
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    actionTimeout: 10_000,
    navigationTimeout: 15_000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'bun run dev',
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
