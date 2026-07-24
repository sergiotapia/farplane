import { existsSync, readFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

export type E2ECredentials = {
  email: string
  password: string
}

const defaultEmail = 'e2e-owner@farplane.test'
const defaultPassword = 'password1'

const credentialsPath = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  '.credentials.json',
)

export function loadE2ECredentials(): E2ECredentials {
  const emailFromEnv = process.env.E2E_EMAIL?.trim()
  const passwordFromEnv = process.env.E2E_PASSWORD?.trim()
  if (emailFromEnv && passwordFromEnv) {
    return { email: emailFromEnv, password: passwordFromEnv }
  }

  if (existsSync(credentialsPath)) {
    const parsed = JSON.parse(readFileSync(credentialsPath, 'utf8')) as {
      email?: string
      password?: string
    }
    if (parsed.email && parsed.password) {
      return { email: parsed.email, password: parsed.password }
    }
  }

  return { email: defaultEmail, password: defaultPassword }
}
