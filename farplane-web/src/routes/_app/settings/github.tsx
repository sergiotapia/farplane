import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useRouteContext } from '@tanstack/react-router'
import { useMemo, useState } from 'react'

import { Button } from '@/components/ui/button.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Label } from '@/components/ui/label.tsx'
import {
  disconnectGitHubInstallation,
  getGitHubInstallations,
  githubInstallationsQueryKey,
  githubRepositoriesQueryKey,
  startGitHubAppManifest,
  startGitHubInstall,
} from '@/lib/api.ts'
import { messageForGitHubError } from '@/lib/oauth-errors.ts'
import { canManageGitHubApp } from '@/lib/roles.ts'

type GitHubSettingsSearch = {
  github?: string
  github_error?: string
}

export const Route = createFileRoute('/_app/settings/github')({
  validateSearch: (search: Record<string, unknown>): GitHubSettingsSearch => {
    const out: GitHubSettingsSearch = {}
    if (typeof search.github === 'string') out.github = search.github
    if (typeof search.github_error === 'string') {
      out.github_error = search.github_error
    }
    return out
  },
  component: GitHubSettingsPage,
})

function GitHubSettingsPage() {
  const { github, github_error: githubError } = Route.useSearch()
  const queryClient = useQueryClient()
  const { me } = useRouteContext({ from: '/_app' })
  const [githubOrgLogin, setGitHubOrgLogin] = useState('')

  const installationsQuery = useQuery({
    queryKey: githubInstallationsQueryKey,
    queryFn: getGitHubInstallations,
  })

  const canManageApp = canManageGitHubApp(me.organization.role)

  const manifestMutation = useMutation({
    mutationFn: () => startGitHubAppManifest(githubOrgLogin),
  })

  const connectMutation = useMutation({
    mutationFn: startGitHubInstall,
  })

  const disconnectMutation = useMutation({
    mutationFn: disconnectGitHubInstallation,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: githubInstallationsQueryKey,
      })
      await queryClient.invalidateQueries({
        queryKey: githubRepositoriesQueryKey,
      })
    },
  })

  const errorMessage = useMemo(
    () => messageForGitHubError(githubError),
    [githubError],
  )

  const installations = installationsQuery.data?.installations ?? []
  const configured = installationsQuery.data?.configured ?? false
  const apiBaseURL = installationsQuery.data?.api_base_url ?? ''
  const apiBaseURLPublic = installationsQuery.data?.api_base_url_public ?? false

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">GitHub</h1>
        <p className="text-muted-foreground text-sm">
          This Farplane install needs its own GitHub App. Create it once (owner
          or admin), then connect repositories. Connected repos become available
          to every member of {me.organization.name}.
        </p>
      </div>

      {github === 'connected' ? (
        <p className="text-sm text-emerald-700 dark:text-emerald-400">
          GitHub connected. Repositories are ready to pick for Projects.
        </p>
      ) : null}
      {github === 'app_created' ? (
        <p className="text-sm text-emerald-700 dark:text-emerald-400">
          Farplane GitHub App created for this install. Next, click Connect
          GitHub to install it on repositories.
        </p>
      ) : null}
      {errorMessage ? (
        <p className="text-destructive text-sm">{errorMessage}</p>
      ) : null}
      {manifestMutation.isError ? (
        <p className="text-destructive text-sm">
          {(manifestMutation.error as Error).message}
        </p>
      ) : null}
      {connectMutation.isError ? (
        <p className="text-destructive text-sm">
          {(connectMutation.error as Error).message}
        </p>
      ) : null}

      {configured || installationsQuery.isLoading ? null : (
        <div className="space-y-4 rounded-md border p-4">
          <div className="space-y-1">
            <h2 className="text-lg font-medium">
              Create the Farplane AI GitHub App
            </h2>
            <p className="text-muted-foreground text-sm">
              GitHub will open with the correct permissions, webhook, and
              callback URLs for this install. The App is named uniquely for this
              Farplane org (GitHub App names are global). You can edit the name
              on GitHub before creating. Credentials are stored encrypted here —
              you do not paste a private key into env.
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="github-org-login">
              GitHub organization (optional)
            </Label>
            <Input
              id="github-org-login"
              value={githubOrgLogin}
              onChange={(event) => setGitHubOrgLogin(event.target.value)}
              placeholder="Leave blank to create on your personal GitHub account"
            />
          </div>
          <Button
            type="button"
            disabled={
              !(canManageApp && apiBaseURLPublic) || manifestMutation.isPending
            }
            onClick={() => manifestMutation.mutate()}
          >
            {manifestMutation.isPending
              ? 'Opening GitHub…'
              : 'Create Farplane AI GitHub App'}
          </Button>
          {canManageApp ? null : (
            <p className="text-muted-foreground text-xs">
              Ask an owner or admin to create the GitHub App for this install.
            </p>
          )}
          <p className="text-muted-foreground text-xs">
            Manifest webhook/callback base:{' '}
            <code className="text-xs break-all">
              {apiBaseURL || '(unknown)'}
            </code>
          </p>
          {apiBaseURLPublic ? null : (
            <p className="text-destructive text-xs">
              That URL is not a public https URL. Set{' '}
              <code className="text-xs">APP_API_BASE_URL</code> in the repo{' '}
              <code className="text-xs">.env</code> to your ngrok https URL,
              then restart <code className="text-xs">make backend</code>.
            </p>
          )}
        </div>
      )}

      {configured ? (
        <div className="space-y-3">
          <Button
            type="button"
            disabled={connectMutation.isPending}
            onClick={() => connectMutation.mutate()}
          >
            {connectMutation.isPending ? 'Opening GitHub…' : 'Connect GitHub'}
          </Button>
          <p className="text-muted-foreground text-sm">
            Install the Farplane App on a personal account or organization, and
            choose which repositories Farplane can use.
          </p>
        </div>
      ) : null}

      <div className="space-y-3">
        <h2 className="text-lg font-medium">Installations</h2>
        {installationsQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">Loading…</p>
        ) : null}
        {installationsQuery.isError ? (
          <p className="text-destructive text-sm">
            Could not load installations.
          </p>
        ) : null}
        {installations.length === 0 && !installationsQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">
            No GitHub installations yet.
          </p>
        ) : null}
        <ul className="divide-border divide-y rounded-md border">
          {installations.map((installation) => {
            const canDisconnect =
              installation.connected_by_user_id === me.user.id ||
              canManageGitHubApp(me.organization.role)
            return (
              <li
                key={installation.id}
                className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">
                    {installation.github_account_login}
                  </p>
                  <p className="text-muted-foreground text-xs">
                    {installation.github_account_type}
                    {installation.suspended ? ' · suspended' : ''}
                    {' · '}
                    {installation.repository_selection} repos
                  </p>
                </div>
                {canDisconnect ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={disconnectMutation.isPending}
                    onClick={() => disconnectMutation.mutate(installation.id)}
                  >
                    Disconnect
                  </Button>
                ) : null}
              </li>
            )
          })}
        </ul>
      </div>
    </div>
  )
}
