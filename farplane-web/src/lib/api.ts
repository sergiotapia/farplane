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
export const githubInstallationsQueryKey = ['github', 'installations'] as const
export const githubRepositoriesQueryKey = ['github', 'repositories'] as const
export const projectsQueryKey = ['projects'] as const

export type GitHubInstallation = {
  id: string
  github_installation_id: number
  github_account_id: number
  github_account_login: string
  github_account_type: 'User' | 'Organization' | string
  repository_selection: string
  connected_by_user_id: string
  suspended: boolean
  created_at: string
}

export type GitHubInstallationsResponse = {
  configured: boolean
  api_base_url: string
  api_base_url_public: boolean
  installations: GitHubInstallation[]
}

export type GitHubRepository = {
  github_repository_id: number
  full_name: string
  default_branch: string
  private: boolean
  html_url: string
  github_installation_id: string
  github_account_type: string
  github_account_login: string
}

export type GitHubRepositoriesResponse = {
  repositories: GitHubRepository[]
}

export type Project = {
  id: string
  organization_id: string
  name: string
  github_repository_id: number
  github_installation_id: string
  default_branch: string
  github_full_name: string
  github_access_status: 'active' | 'revoked' | string
  created_by_user_id: string
  created_at: string
  updated_at: string
}

export type ProjectsResponse = {
  projects: Project[]
}

export function getGitHubInstallations(): Promise<GitHubInstallationsResponse> {
  return apiFetch<GitHubInstallationsResponse>('/api/v1/github/installations')
}

export type GitHubManifestStartResponse = {
  action: string
  manifest: string
  state: string
}

function assertGitHubHostURL(raw: string, label: string): URL {
  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    throw new ApiError(500, `${label} is not a valid URL`, { url: raw })
  }
  if (parsed.protocol !== 'https:' || parsed.hostname !== 'github.com') {
    throw new ApiError(500, `${label} must be an https://github.com URL`, {
      url: raw,
    })
  }
  return parsed
}

/** Start GitHub App Manifest registration (self-hosted App create). */
export async function startGitHubAppManifest(
  githubOrganizationLogin?: string,
): Promise<void> {
  const body = await apiFetch<GitHubManifestStartResponse>(
    '/api/v1/github/app/manifest/start',
    {
      method: 'POST',
      body: JSON.stringify({
        github_organization_login: githubOrganizationLogin?.trim() || undefined,
      }),
    },
  )
  if (!body.action || !body.manifest) {
    throw new ApiError(500, 'GitHub App manifest response incomplete', body)
  }
  const actionURL = assertGitHubHostURL(body.action, 'GitHub App create URL')
  if (body.state) {
    actionURL.searchParams.set('state', body.state)
  }
  const form = document.createElement('form')
  form.method = 'POST'
  form.action = actionURL.toString()
  const input = document.createElement('input')
  input.type = 'hidden'
  input.name = 'manifest'
  input.value = body.manifest
  form.appendChild(input)
  document.body.appendChild(form)
  form.submit()
}

export async function startGitHubInstall(): Promise<void> {
  const body = await apiFetch<{ url: string }>('/api/v1/github/install/start', {
    method: 'POST',
  })
  if (!body.url) {
    throw new ApiError(500, 'GitHub install URL missing', body)
  }
  const installURL = assertGitHubHostURL(body.url, 'GitHub install URL')
  window.location.assign(installURL.toString())
}

export async function disconnectGitHubInstallation(
  installationId: string,
): Promise<void> {
  await apiFetch<null>(`/api/v1/github/installations/${installationId}`, {
    method: 'DELETE',
  })
}

export function getGitHubRepositories(
  refresh = false,
): Promise<GitHubRepositoriesResponse> {
  const path = refresh
    ? '/api/v1/github/repositories?refresh=1'
    : '/api/v1/github/repositories'
  return apiFetch<GitHubRepositoriesResponse>(path)
}

export function getProjects(): Promise<ProjectsResponse> {
  return apiFetch<ProjectsResponse>('/api/v1/projects')
}

export function createProject(payload: {
  name: string
  github_repository_id: number
}): Promise<Project> {
  return apiFetch<Project>('/api/v1/projects', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}
