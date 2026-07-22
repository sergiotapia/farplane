import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  createFileRoute,
  isRedirect,
  redirect,
  useNavigate,
  useRouteContext,
} from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  getMe,
  meQueryKey,
  postLogin,
  startGoogleLogin,
} from '@/lib/api'
import { messageForOAuthError } from '@/lib/oauth-errors'

type LoginSearch = {
  oauth_error?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    oauth_error:
      typeof search.oauth_error === 'string' ? search.oauth_error : undefined,
  }),
  beforeLoad: async ({ context }) => {
    try {
      await context.queryClient.fetchQuery({
        queryKey: meQueryKey,
        queryFn: getMe,
        staleTime: 0,
      })
      throw redirect({ to: '/' })
    } catch (error) {
      if (isRedirect(error)) {
        throw error
      }
      if (error instanceof ApiError && error.status === 401) {
        context.queryClient.removeQueries({ queryKey: meQueryKey })
        return
      }
      throw error
    }
  },
  component: LoginPage,
})

function LoginPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { oauth_error: oauthError } = Route.useSearch()
  const { setupStatus } = useRouteContext({ from: '__root__' })
  const googleConfigured = setupStatus?.google_oauth_configured === true

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [formError, setFormError] = useState<string | null>(
    messageForOAuthError(oauthError),
  )

  const loginMutation = useMutation({
    mutationFn: postLogin,
    onSuccess: async (data) => {
      queryClient.setQueryData(meQueryKey, data)
      await navigate({ to: '/' })
    },
    onError: (error) => {
      if (error instanceof ApiError) {
        setFormError(error.message)
        return
      }
      setFormError('Sign in failed. Try again.')
    },
  })

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFormError(null)

    const trimmedEmail = email.trim()
    if (!trimmedEmail || !password) {
      setFormError('Enter your email and password.')
      return
    }

    loginMutation.mutate({ email: trimmedEmail, password })
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <div className="w-full max-w-md space-y-8">
        <div className="space-y-2">
          <p className="text-muted-foreground text-sm font-medium tracking-wide uppercase">
            Farplane
          </p>
          <h1 className="text-3xl font-bold tracking-tight">Sign in</h1>
          <p className="text-muted-foreground text-sm">
            Sign in to your Farplane install.
          </p>
        </div>

        <form className="space-y-4" onSubmit={onSubmit}>
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
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
          </div>

          {formError ? (
            <p className="text-destructive text-sm" role="alert">
              {formError}
            </p>
          ) : null}

          <Button
            type="submit"
            className="w-full"
            size="lg"
            disabled={loginMutation.isPending}
          >
            {loginMutation.isPending ? 'Signing in…' : 'Sign in'}
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
            disabled={!googleConfigured || loginMutation.isPending}
            onClick={() => startGoogleLogin()}
          >
            Continue with Google
          </Button>
        </div>
      </div>
    </div>
  )
}
