import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, useNavigate, useRouteContext } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  getSetupStatus,
  meQueryKey,
  postSetup,
  setupStatusQueryKey,
  startGoogleSetup,
  type SetupStatus,
} from '@/lib/api'
import { messageForOAuthError } from '@/lib/oauth-errors'

type SetupSearch = {
  oauth_error?: string
}

export const Route = createFileRoute('/setup')({
  validateSearch: (search: Record<string, unknown>): SetupSearch => ({
    oauth_error:
      typeof search.oauth_error === 'string' ? search.oauth_error : undefined,
  }),
  component: SetupPage,
})

function SetupPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { oauth_error: oauthError } = Route.useSearch()
  const { setupStatus } = useRouteContext({ from: '__root__' })
  const googleConfigured = setupStatus?.google_oauth_configured === true
  const setupTokenRequired = setupStatus?.setup_token_required === true

  const [organizationName, setOrganizationName] = useState('')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [setupToken, setSetupToken] = useState('')
  const [formError, setFormError] = useState<string | null>(
    messageForOAuthError(oauthError),
  )

  const setupMutation = useMutation({
    mutationFn: postSetup,
    onSuccess: async (data) => {
      queryClient.setQueryData<SetupStatus>(setupStatusQueryKey, (previous) => ({
        needs_setup: false,
        google_oauth_configured: previous?.google_oauth_configured ?? false,
        setup_token_required: previous?.setup_token_required ?? false,
      }))
      queryClient.setQueryData(meQueryKey, data)
      await navigate({ to: '/' })
    },
    onError: async (error) => {
      if (error instanceof ApiError) {
        if (error.status === 409) {
          await queryClient.fetchQuery({
            queryKey: setupStatusQueryKey,
            queryFn: getSetupStatus,
          })
          await navigate({ to: '/login' })
          return
        }
        setFormError(error.message)
        return
      }
      setFormError('Setup failed. Try again.')
    },
  })

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFormError(null)

    const trimmedOrg = organizationName.trim()
    const trimmedEmail = email.trim()
    const trimmedDisplayName = displayName.trim()
    const trimmedToken = setupToken.trim()

    if (!trimmedOrg || !trimmedEmail || !trimmedDisplayName || !password) {
      setFormError('Fill in all fields to continue.')
      return
    }
    if (setupTokenRequired && !trimmedToken) {
      setFormError('Enter the setup token from the server configuration.')
      return
    }

    setupMutation.mutate({
      organization_name: trimmedOrg,
      email: trimmedEmail,
      display_name: trimmedDisplayName,
      password,
      setup_token: trimmedToken || undefined,
    })
  }

  function onContinueWithGoogle() {
    setFormError(null)
    const trimmedOrg = organizationName.trim()
    const trimmedToken = setupToken.trim()
    if (!trimmedOrg) {
      setFormError('Enter an organization name before continuing with Google.')
      return
    }
    if (setupTokenRequired && !trimmedToken) {
      setFormError('Enter the setup token from the server configuration.')
      return
    }
    startGoogleSetup(trimmedOrg, trimmedToken || undefined)
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <div className="w-full max-w-md space-y-8">
        <div className="space-y-2">
          <p className="text-muted-foreground text-sm font-medium tracking-wide uppercase">
            Farplane
          </p>
          <h1 className="text-3xl font-bold tracking-tight">First-time setup</h1>
          <p className="text-muted-foreground text-sm">
            Create the organization and owner account for this install. After
            this step, new users join by invite only.
          </p>
        </div>

        <form className="space-y-4" onSubmit={onSubmit}>
          <div className="space-y-2">
            <Label htmlFor="organization-name">Organization name</Label>
            <Input
              id="organization-name"
              name="organization_name"
              autoComplete="organization"
              value={organizationName}
              onChange={(event) => setOrganizationName(event.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="display-name">Your name</Label>
            <Input
              id="display-name"
              name="display_name"
              autoComplete="name"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              name="email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              name="password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              minLength={8}
            />
          </div>

          {setupTokenRequired ? (
            <div className="space-y-2">
              <Label htmlFor="setup-token">Setup token</Label>
              <Input
                id="setup-token"
                name="setup_token"
                type="password"
                autoComplete="off"
                value={setupToken}
                onChange={(event) => setSetupToken(event.target.value)}
                required
              />
              <p className="text-muted-foreground text-xs">
                Set <code className="text-foreground">SETUP_TOKEN</code> on the
                API host, then paste it here.
              </p>
            </div>
          ) : null}

          {formError ? (
            <p className="text-destructive text-sm" role="alert">
              {formError}
            </p>
          ) : null}

          <Button
            type="submit"
            className="w-full"
            size="lg"
            disabled={setupMutation.isPending}
          >
            {setupMutation.isPending ? 'Creating…' : 'Create organization'}
          </Button>
        </form>

        <div className="space-y-3">
          <div className="text-muted-foreground flex items-center gap-3 text-xs">
            <div className="bg-border h-px flex-1" />
            <span>or</span>
            <div className="bg-border h-px flex-1" />
          </div>

          {!googleConfigured ? (
            <p
              className="bg-muted text-muted-foreground rounded-lg px-3 py-2 text-sm"
              role="status"
            >
              Google sign-in is unavailable. Set{' '}
              <code className="text-foreground">GOOGLE_CLIENT_ID</code> and{' '}
              <code className="text-foreground">GOOGLE_CLIENT_SECRET</code> on
              the API, then restart it.
            </p>
          ) : null}

          <Button
            type="button"
            variant="outline"
            className="w-full"
            size="lg"
            disabled={!googleConfigured || setupMutation.isPending}
            onClick={onContinueWithGoogle}
          >
            Continue with Google
          </Button>
        </div>
      </div>
    </div>
  )
}
