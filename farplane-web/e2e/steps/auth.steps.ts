import process from 'node:process'
import { expect } from '@playwright/test'
import { createBdd } from 'playwright-bdd'
import { loadE2ECredentials } from '../credentials.ts'
import { test } from '../fixtures.ts'

const { Given, When, Then } = createBdd(test)

const apiBaseURL =
  process.env.PLAYWRIGHT_API_BASE_URL ?? 'http://localhost:8080'

Given('I open the login page', async ({ page }) => {
  await page.goto('/login')
  // Root redirects to /setup when the install is unfinished; global-setup
  // should have completed setup before workers start.
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible({
    timeout: 15_000,
  })
})

Given('the API is reachable', async () => {
  const response = await fetch(`${apiBaseURL}/api/v1/setup/status`)
  test.skip(!response.ok, `API not reachable at ${apiBaseURL}`)
  const status = (await response.json()) as { needs_setup?: boolean }
  test.skip(
    status.needs_setup === true,
    'Install still needs setup; global-setup should have completed it',
  )
})

Given('authenticated e2e credentials are configured', () => {
  const creds = loadE2ECredentials()
  process.env.E2E_EMAIL = creds.email
  process.env.E2E_PASSWORD = creds.password
})

When(
  'I fill email with {string} and password with {string}',
  async ({ page }, email: string, password: string) => {
    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Password').fill(password)
  },
)

When('I submit the sign-in form', async ({ page }) => {
  await page.getByRole('button', { name: 'Sign in' }).click()
})

When(
  'I sign in with {string} and {string}',
  async ({ page }, email: string, password: string) => {
    await page.getByLabel('Email').fill(email)
    await page.getByLabel('Password').fill(password)
    await page.getByRole('button', { name: 'Sign in' }).click()
  },
)

When('I sign in with E2E credentials', async ({ page }) => {
  const creds = loadE2ECredentials()
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible({
    timeout: 15_000,
  })
  await page.getByLabel('Email').fill(creds.email)
  await page.getByLabel('Password').fill(creds.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/($|\?)/)
})

Then('I see the sign-in heading', async ({ page }) => {
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
})

Then('I see email and password fields', async ({ page }) => {
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password')).toBeVisible()
})

Then(
  'I see a sign-in alert containing {string}',
  async ({ page }, text: string) => {
    await expect(page.getByRole('alert')).toContainText(text)
  },
)

Then('I see a sign-in alert', async ({ page }) => {
  await expect(page.getByRole('alert')).toBeVisible()
})

Then('I see the home page', async ({ page }) => {
  await expect(page.getByRole('link', { name: 'Farplane' })).toBeVisible()
})

Then('the sidebar shows an organization name', async ({ page }) => {
  const org = page.getByTestId('active-organization-name')
  await expect(org).toBeVisible()
  await expect(org).not.toHaveText('')
})
