import { expect } from '@playwright/test'
import { createBdd } from 'playwright-bdd'

import { test } from '../fixtures.ts'

const { When, Then } = createBdd(test)

When('I open GitHub settings', async ({ page }) => {
  await page.goto('/settings/github')
})

Then('I see the GitHub settings heading', async ({ page }) => {
  // exact: string name is a substring match by default and also hits the h2
  // "Create the Farplane AI GitHub App".
  await expect(
    page.getByRole('heading', { name: 'GitHub', exact: true }),
  ).toBeVisible()
})

Then(
  'I see GitHub connect actions or configuration guidance',
  async ({ page }) => {
    const connect = page.getByRole('button', { name: /Connect GitHub/i })
    const createApp = page.getByRole('button', {
      name: /Create Farplane AI GitHub App/i,
    })
    const guidance = page.getByText(/GitHub App/i)
    await expect(connect.or(createApp).or(guidance).first()).toBeVisible()
  },
)
