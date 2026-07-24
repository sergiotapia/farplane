import { writeFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

/**
 * Ensures the local API has completed first-time setup so /login is reachable.
 * When needs_setup is true, creates an owner using E2E_EMAIL / E2E_PASSWORD
 * (defaults below). Does not reset an already-configured install.
 *
 * Credentials for workers are written to e2e/.credentials.json (gitignored).
 */
const apiBaseURL =
  process.env.PLAYWRIGHT_API_BASE_URL ?? 'http://localhost:8080'

const defaultEmail = 'e2e-owner@farplane.test'
const defaultPassword = 'password1'
const defaultOrg = 'E2E Org'
const defaultName = 'E2E Owner'

const credentialsPath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  '.credentials.json',
)

function writeCredentials(email: string, password: string): void {
  writeFileSync(
    credentialsPath,
    `${JSON.stringify({ email, password }, null, 2)}\n`,
    'utf8',
  )
}

export default async function globalSetup(): Promise<void> {
  const email = process.env.E2E_EMAIL?.trim() || defaultEmail
  const password = process.env.E2E_PASSWORD?.trim() || defaultPassword

  let statusResponse: Response
  try {
    statusResponse = await fetch(`${apiBaseURL}/api/v1/setup/status`)
  } catch (error) {
    throw new Error(
      `E2E global setup: API not reachable at ${apiBaseURL} (${String(error)}). Start it with \`make backend\`.`,
    )
  }

  if (!statusResponse.ok) {
    throw new Error(
      `E2E global setup: setup/status returned ${statusResponse.status}`,
    )
  }

  const status = (await statusResponse.json()) as {
    needs_setup?: boolean
    setup_token_required?: boolean
  }

  if (!status.needs_setup) {
    writeCredentials(email, password)
    return
  }

  if (status.setup_token_required) {
    throw new Error(
      'E2E global setup: API requires SETUP_TOKEN. Set it in the environment and re-run, or complete setup manually.',
    )
  }

  const setupResponse = await fetch(`${apiBaseURL}/api/v1/setup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      organization_name: process.env.E2E_ORG_NAME?.trim() || defaultOrg,
      email,
      display_name: process.env.E2E_DISPLAY_NAME?.trim() || defaultName,
      password,
    }),
  })

  if (setupResponse.status !== 201 && setupResponse.status !== 200) {
    const body = await setupResponse.text()
    throw new Error(
      `E2E global setup: POST /setup failed (${setupResponse.status}): ${body}`,
    )
  }

  writeCredentials(email, password)
}
