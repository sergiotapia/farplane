/** Base URL for the Farplane control plane API (local default). */
export const API_BASE_URL =
  (typeof import.meta !== 'undefined' &&
    // Rsbuild exposes PUBLIC_* on import.meta.env when set at build time.
    (import.meta as ImportMeta & { env?: { PUBLIC_API_BASE_URL?: string } }).env
      ?.PUBLIC_API_BASE_URL) ||
  'http://localhost:8080'

export type SetupStatus = {
  needs_setup: boolean
  google_oauth_configured: boolean
  setup_token_required: boolean
}

export type SetupRequest = {
  organization_name: string
  email: string
  display_name: string
  password: string
  setup_token?: string
}

export type MeUser = {
  id: string
  email: string
  display_name: string
  avatar_url?: string | null
}

export type MeOrganization = {
  id: string
  name: string
  role: string
}

/** Response from POST /setup and GET /me. */
export type MeResponse = {
  user: MeUser
  organization: MeOrganization
}

export class ApiError extends Error {
  readonly status: number
  readonly body: unknown

  constructor(status: number, message: string, body: unknown = null) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

async function parseJson(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text) as unknown
  } catch {
    return text
  }
}

function errorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object' && 'error' in body) {
    const value = (body as { error: unknown }).error
    if (typeof value === 'string' && value.length > 0) return value
  }
  if (body && typeof body === 'object' && 'message' in body) {
    const value = (body as { message: unknown }).message
    if (typeof value === 'string' && value.length > 0) return value
  }
  return fallback
}

async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers,
    credentials: 'include',
  })

  const body = await parseJson(response)

  if (!response.ok) {
    throw new ApiError(
      response.status,
      errorMessage(body, response.statusText || 'Request failed'),
      body,
    )
  }

  return body as T
}

export function getSetupStatus(): Promise<SetupStatus> {
  return apiFetch<SetupStatus>('/api/v1/setup/status')
}

export function postSetup(payload: SetupRequest): Promise<MeResponse> {
  return apiFetch<MeResponse>('/api/v1/setup', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function getMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>('/api/v1/me')
}

export type LoginRequest = {
  email: string
  password: string
}

export function postLogin(payload: LoginRequest): Promise<MeResponse> {
  return apiFetch<MeResponse>('/api/v1/auth/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export async function postLogout(): Promise<void> {
  await apiFetch<null>('/api/v1/auth/logout', {
    method: 'POST',
  })
}

/**
 * Full-page navigation to Google OAuth start.
 * Backend reads intent + organization_name from query params and embeds them
 * in signed OAuth state (see handleGoogleStart).
 */
export function startGoogleSetup(
  organizationName: string,
  setupToken?: string,
): void {
  const url = new URL(`${API_BASE_URL}/api/v1/auth/google/start`)
  url.searchParams.set('intent', 'setup')
  url.searchParams.set('organization_name', organizationName)
  if (setupToken) {
    url.searchParams.set('setup_token', setupToken)
  }
  window.location.assign(url.toString())
}

/** Full-page navigation for Google sign-in after the install is set up. */
export function startGoogleLogin(): void {
  const url = new URL(`${API_BASE_URL}/api/v1/auth/google/start`)
  url.searchParams.set('intent', 'login')
  window.location.assign(url.toString())
}

export const setupStatusQueryKey = ['setup', 'status'] as const
export const meQueryKey = ['me'] as const
