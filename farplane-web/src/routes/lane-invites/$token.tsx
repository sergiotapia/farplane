import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Link,
  createFileRoute,
  useNavigate,
  useRouteContext,
} from '@tanstack/react-router'
import { useEffect, useState, type FormEvent } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  acceptLaneInvite,
  getLane,
  getLaneInvite,
  getMe,
  meQueryKey,
  signupLaneInvite,
  startGoogleLaneInvite,
} from '@/lib/api'
import { messageForOAuthError } from '@/lib/oauth-errors'

type InviteSearch = {
  oauth_error?: string
}

export const Route = createFileRoute('/lane-invites/$token')({
  validateSearch: (search: Record<string, unknown>): InviteSearch => ({
    oauth_error:
      typeof search.oauth_error === 'string' ? search.oauth_error : undefined,
  }),
  component: LaneInviteLandingPage,
})

function LaneInviteLandingPage() {
  const { token } = Route.useParams()
  const { oauth_error: oauthError } = Route.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { setupStatus } = useRouteContext({ from: '__root__' })
  const googleConfigured = setupStatus?.google_oauth_configured === true

  const inviteQuery = useQuery({
    queryKey: ['lane-invite', token],
    queryFn: () => getLaneInvite(token),
  })
  const meQuery = useQuery({
    queryKey: meQueryKey,
    queryFn: getMe,
    retry: false,
  })

  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [formError, setFormError] = useState<string | null>(
    messageForOAuthError(oauthError),
  )

  const invite = inviteQuery.data
  const loggedIn = meQuery.isSuccess

  const alreadyParticipantQuery = useQuery({
    queryKey: ['lane-invite-already', invite?.lane_id, meQuery.data?.user.id],
    queryFn: () => getLane(invite!.lane_id),
    enabled: Boolean(loggedIn && invite?.lane_id && invite.pending),
    retry: false,
  })

  useEffect(() => {
    if (alreadyParticipantQuery.isSuccess && invite?.lane_id) {
      void navigate({
        to: '/lanes/$laneId',
        params: { laneId: invite.lane_id },
      })
    }
  }, [alreadyParticipantQuery.isSuccess, invite?.lane_id, navigate])

  const acceptMutation = useMutation({
    mutationFn: () => acceptLaneInvite(token),
    onSuccess: async (body) => {
      const laneId = body.lane_id ?? inviteQuery.data?.lane_id
      if (laneId) {
        await navigate({ to: '/lanes/$laneId', params: { laneId } })
      }
    },
    onError: (error) => {
      setFormError(
        error instanceof ApiError ? error.message : 'Could not accept invite',
      )
    },
  })

  const signupMutation = useMutation({
    mutationFn: () =>
      signupLaneInvite(token, {
        email: email.trim(),
        display_name: displayName.trim(),
        password,
      }),
    onSuccess: async (body) => {
      await queryClient.invalidateQueries({ queryKey: meQueryKey })
      await navigate({
        to: '/lanes/$laneId',
        params: { laneId: body.lane_id },
      })
    },
    onError: (error) => {
      setFormError(
        error instanceof ApiError ? error.message : 'Signup failed',
      )
    },
  })

  function onSignup(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    signupMutation.mutate()
  }

  const inviter =
    invite?.invited_by_display_name?.trim() || 'Someone'

  return (
    <div className="mx-auto w-full max-w-md space-y-6 py-16 px-4">
      <div className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Lane invite</h1>
        {invite ? (
          <p className="text-muted-foreground text-sm">
            {inviter} invited you to join them on Farplane
            {invite.lane_name ? (
              <>
                {' '}
                in{' '}
                <span className="text-foreground font-medium">
                  {invite.lane_name}
                </span>
              </>
            ) : null}
            .
          </p>
        ) : inviteQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">Loading invite…</p>
        ) : (
          <p className="text-destructive text-sm">Invite not found.</p>
        )}
      </div>

      {formError ? <p className="text-destructive text-sm">{formError}</p> : null}

      {invite && !invite.pending ? (
        <p className="text-muted-foreground text-sm">
          This invite is no longer available.
        </p>
      ) : null}

      {invite?.pending && loggedIn && alreadyParticipantQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Checking membership…</p>
      ) : null}

      {invite?.pending &&
      loggedIn &&
      alreadyParticipantQuery.isError &&
      !alreadyParticipantQuery.isSuccess ? (
        <div className="space-y-3">
          <p className="text-sm">
            Signed in as {meQuery.data?.user.email}. Accept to join this Lane.
          </p>
          <Button
            type="button"
            disabled={acceptMutation.isPending}
            onClick={() => acceptMutation.mutate()}
          >
            {acceptMutation.isPending ? 'Accepting…' : 'Accept invite'}
          </Button>
        </div>
      ) : null}

      {invite?.pending && !loggedIn ? (
        <div className="space-y-6">
          <form className="space-y-4" onSubmit={onSignup}>
            <p className="text-muted-foreground text-sm">
              Create an account to join, or sign in if you already have one.
            </p>
            <div className="space-y-2">
              <Label htmlFor="invite-email">Email</Label>
              <Input
                id="invite-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="invite-display-name">Display name</Label>
              <Input
                id="invite-display-name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="invite-password">Password</Label>
              <Input
                id="invite-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                minLength={8}
                required
              />
            </div>
            <Button type="submit" disabled={signupMutation.isPending}>
              {signupMutation.isPending
                ? 'Creating account…'
                : 'Create account and join'}
            </Button>
          </form>

          {googleConfigured ? (
            <Button
              type="button"
              variant="outline"
              className="w-full"
              onClick={() => startGoogleLaneInvite(token)}
            >
              Continue with Google
            </Button>
          ) : null}

          <p className="text-muted-foreground text-xs">
            Already have an account?{' '}
            <Link to="/login" className="underline underline-offset-4">
              Sign in
            </Link>
            , then open this invite link again.
          </p>
        </div>
      ) : null}
    </div>
  )
}
