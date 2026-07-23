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

/** Google sign-in / signup for a Lane email invite. */
export function startGoogleLaneInvite(inviteToken: string): void {
  const url = new URL(`${API_BASE_URL}/api/v1/auth/google/start`)
  url.searchParams.set('intent', 'lane_invite')
  url.searchParams.set('invite_token', inviteToken)
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

export const scratchEnvironmentQueryKey = ['scratch-environment'] as const
export const projectEnvironmentQueryKey = (projectId: string) =>
  ['projects', projectId, 'environment'] as const
export const secretsQueryKey = ['secrets'] as const
export const laneAgentsQueryKey = ['lane-agents'] as const
export const lanesQueryKey = ['lanes'] as const
export const projectLanesQueryKey = (projectId: string) =>
  ['projects', projectId, 'lanes'] as const
export const laneQueryKey = (laneId: string) => ['lanes', laneId] as const
export const laneMessagesQueryKey = (laneId: string) =>
  ['lanes', laneId, 'messages'] as const
export const laneAgentModelsQueryKey = (provider: string, source: string) =>
  ['lane-agents', provider, 'models', source] as const
export const laneParticipantsQueryKey = (laneId: string) =>
  ['lanes', laneId, 'participants'] as const
export const laneActiveInviteQueryKey = (laneId: string) =>
  ['lanes', laneId, 'invites', 'active'] as const
export const organizationMembersQueryKey = ['organization-members'] as const

export type ScratchEnvironment = {
  organization_id: string
  dockerfile_text: string
  validation_status: string
  validated_image_reference?: string | null
  last_validation_log?: string | null
  validated_at?: string | null
  updated_by_user_id?: string | null
  created_at: string
  updated_at: string
}

export type ProjectEnvironment = {
  project_id: string
  organization_id: string
  dockerfile_text: string
  validation_status: string
  validated_image_reference?: string | null
  last_validation_log?: string | null
  validated_at?: string | null
  generation_status: string
  generation_log?: string | null
  updated_by_user_id?: string | null
  created_at: string
  updated_at: string
}

export type OrganizationSecret = {
  name: string
  label: string
  is_set: boolean
  updated_at?: string | null
}

export type LaneAgentModelSource = {
  id: string
  label: string
}

export type LaneAgent = {
  provider: string
  label: string
  required_secret: string
  available: boolean
  model_sources: LaneAgentModelSource[]
}

/** Model option for a lane agent + model source. */
export type LaneAgentModel = {
  id: string
  label: string
  reasoning_efforts: string[]
  default_reasoning_effort?: string | null
  supports_reasoning: boolean
}

export type LaneKind = 'project' | 'scratch'

export type Lane = {
  id: string
  project_id?: string | null
  organization_id: string
  owner_user_id: string
  name: string
  lane_kind: LaneKind
  image_reference?: string | null
  runtime_kind: string
  runtime_id?: string | null
  agent_provider: string
  agent_provider_session_id?: string | null
  model_source?: string | null
  agent_model?: string | null
  reasoning_effort?: string | null
  status: string
  turn_running?: boolean
  has_other_participants?: boolean
  created_at: string
  updated_at: string
}

export type GroupedLanes = {
  projects: Array<{
    id: string
    name: string
    lanes: Lane[]
  }>
  scratch_lanes: Lane[]
}

export type LaneInvite = {
  id: string
  lane_id: string
  token: string
  invited_by_user_id?: string | null
  expires_at?: string | null
  revoked_at?: string | null
  created_at: string
  accept_url: string
}

export type LaneMessage = {
  id: string
  lane_id: string
  sequence_number: number
  event_type: string
  role?: string | null
  author_user_id?: string | null
  body?: string | null
  payload?: unknown
  created_at: string
}

export type LaneParticipant = {
  id: string
  lane_id: string
  user_id: string
  role: string
  joined_at: string
  display_name?: string
  email?: string
}

export function getScratchEnvironment(): Promise<ScratchEnvironment> {
  return apiFetch('/api/v1/scratch-environment')
}

export function upsertScratchEnvironment(payload: {
  dockerfile_text: string
}): Promise<ScratchEnvironment> {
  return apiFetch('/api/v1/scratch-environment', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

/** Validate Scratch Environment. HTTP 422 still returns the updated environment body. */
export async function validateScratchEnvironment(): Promise<ScratchEnvironment> {
  const headers = new Headers()
  headers.set('Content-Type', 'application/json')
  const response = await fetch(
    `${API_BASE_URL}/api/v1/scratch-environment/validate`,
    {
      method: 'POST',
      headers,
      credentials: 'include',
    },
  )
  const body = await parseJson(response)
  if (response.ok || response.status === 422) {
    if (!body || typeof body !== 'object') {
      throw new ApiError(response.status, 'Invalid validate response', body)
    }
    return body as ScratchEnvironment
  }
  throw new ApiError(
    response.status,
    errorMessage(body, response.statusText || 'Request failed'),
    body,
  )
}

export async function getProjectEnvironment(
  projectId: string,
): Promise<ProjectEnvironment | null> {
  const headers = new Headers()
  const response = await fetch(
    `${API_BASE_URL}/api/v1/projects/${projectId}/environment`,
    {
      headers,
      credentials: 'include',
    },
  )
  if (response.status === 404) return null
  const body = await parseJson(response)
  if (!response.ok) {
    throw new ApiError(
      response.status,
      errorMessage(body, response.statusText || 'Request failed'),
      body,
    )
  }
  return body as ProjectEnvironment
}

export function upsertProjectEnvironment(
  projectId: string,
  payload: { dockerfile_text: string },
): Promise<ProjectEnvironment> {
  return apiFetch(`/api/v1/projects/${projectId}/environment`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
}

/** Validate Project Environment. HTTP 422 still returns the updated environment body. */
export async function validateProjectEnvironment(
  projectId: string,
): Promise<ProjectEnvironment> {
  const headers = new Headers()
  headers.set('Content-Type', 'application/json')
  const response = await fetch(
    `${API_BASE_URL}/api/v1/projects/${projectId}/environment/validate`,
    {
      method: 'POST',
      headers,
      credentials: 'include',
    },
  )
  const body = await parseJson(response)
  if (response.ok || response.status === 422) {
    if (!body || typeof body !== 'object') {
      throw new ApiError(response.status, 'Invalid validate response', body)
    }
    return body as ProjectEnvironment
  }
  throw new ApiError(
    response.status,
    errorMessage(body, response.statusText || 'Request failed'),
    body,
  )
}

export function generateProjectEnvironment(
  projectId: string,
  payload: { model_source?: string; agent_model?: string } = {},
): Promise<ProjectEnvironment> {
  return apiFetch(`/api/v1/projects/${projectId}/environment/generate`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function getSecrets(): Promise<{ secrets: OrganizationSecret[] }> {
  return apiFetch('/api/v1/secrets')
}

export function setSecret(
  name: string,
  value: string,
): Promise<{ name: string; is_set: boolean; label: string }> {
  return apiFetch(`/api/v1/secrets/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify({ value }),
  })
}

export async function clearSecret(name: string): Promise<void> {
  await apiFetch(`/api/v1/secrets/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export function getLaneAgents(): Promise<{ agents: LaneAgent[] }> {
  return apiFetch('/api/v1/lane-agents')
}

export function getLaneAgentModels(
  provider: string,
  source: string,
): Promise<{ models: LaneAgentModel[] }> {
  const params = new URLSearchParams({ source })
  return apiFetch(
    `/api/v1/lane-agents/${encodeURIComponent(provider)}/models?${params}`,
  )
}

export function getLanes(): Promise<GroupedLanes> {
  return apiFetch('/api/v1/lanes')
}

export function getProjectLanes(
  projectId: string,
): Promise<{ lanes: Lane[] }> {
  return apiFetch(`/api/v1/projects/${projectId}/lanes`)
}

export function createLane(payload: {
  name: string
  agent_provider: string
  project_id?: string | null
}): Promise<Lane> {
  return apiFetch('/api/v1/lanes', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function getLane(laneId: string): Promise<Lane> {
  return apiFetch(`/api/v1/lanes/${laneId}`)
}

export async function destroyLane(laneId: string): Promise<void> {
  await apiFetch(`/api/v1/lanes/${laneId}`, {
    method: 'DELETE',
  })
}

export function patchLane(
  laneId: string,
  payload: {
    agent_provider?: string
    name?: string
    model_source?: string
    agent_model?: string
    reasoning_effort?: string | null
  },
): Promise<Lane> {
  return apiFetch(`/api/v1/lanes/${laneId}`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  })
}

export function getLaneMessages(
  laneId: string,
): Promise<{ messages: LaneMessage[] }> {
  return apiFetch(`/api/v1/lanes/${laneId}/messages`)
}

export function postLaneMessage(
  laneId: string,
  text: string,
): Promise<LaneMessage> {
  return apiFetch(`/api/v1/lanes/${laneId}/messages`, {
    method: 'POST',
    body: JSON.stringify({ text }),
  })
}

export function getLaneParticipants(
  laneId: string,
): Promise<{ participants: LaneParticipant[] }> {
  return apiFetch(`/api/v1/lanes/${laneId}/participants`)
}

export function addLaneParticipant(
  laneId: string,
  userId: string,
): Promise<LaneParticipant> {
  return apiFetch(`/api/v1/lanes/${laneId}/participants`, {
    method: 'POST',
    body: JSON.stringify({ user_id: userId }),
  })
}

export async function removeLaneParticipant(
  laneId: string,
  userId: string,
): Promise<void> {
  await apiFetch(`/api/v1/lanes/${laneId}/participants/${userId}`, {
    method: 'DELETE',
  })
}

export async function leaveLane(laneId: string): Promise<void> {
  await apiFetch(`/api/v1/lanes/${laneId}/leave`, {
    method: 'POST',
  })
}

export function getActiveLaneInvite(laneId: string): Promise<LaneInvite> {
  return apiFetch(`/api/v1/lanes/${laneId}/invites/active`)
}

export function createLaneInvite(laneId: string): Promise<LaneInvite> {
  return apiFetch(`/api/v1/lanes/${laneId}/invites`, {
    method: 'POST',
  })
}

export function regenerateLaneInvite(laneId: string): Promise<LaneInvite> {
  return apiFetch(`/api/v1/lanes/${laneId}/invites/regenerate`, {
    method: 'POST',
  })
}

export async function revokeActiveLaneInvite(laneId: string): Promise<void> {
  await apiFetch(`/api/v1/lanes/${laneId}/invites/active`, {
    method: 'DELETE',
  })
}

export type LaneInvitePreview = {
  token: string
  lane_id: string
  lane_name: string
  invited_by_display_name?: string
  expires_at?: string | null
  pending: boolean
  accept_url: string
}

export function getLaneInvite(token: string): Promise<LaneInvitePreview> {
  return apiFetch(`/api/v1/lane-invites/${token}`)
}

export function signupLaneInvite(
  token: string,
  payload: { email: string; display_name: string; password: string },
): Promise<{ lane_id: string }> {
  return apiFetch(`/api/v1/lane-invites/${token}/signup`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

export function acceptLaneInvite(
  token: string,
): Promise<{ lane_id?: string; id?: string }> {
  return apiFetch(`/api/v1/lane-invites/${token}/accept`, {
    method: 'POST',
  })
}

export function getOrganizationMembers(): Promise<{
  members: Array<{
    id: string
    email: string
    display_name: string
    avatar_url?: string | null
  }>
}> {
  return apiFetch('/api/v1/organization-members')
}

/** Browser WebSocket URL for a Lane timeline stream. */
export function laneWebSocketURL(laneId: string): string {
  const base = new URL(API_BASE_URL)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  base.pathname = `/api/v1/lanes/${laneId}/ws`
  base.search = ''
  return base.toString()
}

/** Browser WebSocket URL for sidebar turn_running updates across lanes. */
export function lanesTurnWebSocketURL(): string {
  const base = new URL(API_BASE_URL)
  base.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:'
  base.pathname = '/api/v1/lanes/turns/ws'
  base.search = ''
  return base.toString()
}
