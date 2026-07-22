import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useRouteContext } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  createLaneInvite,
  getLane,
  getLaneAgents,
  getLaneMessages,
  getLaneParticipants,
  getOrganizationMembers,
  kickLaneParticipant,
  laneAgentsQueryKey,
  laneMessagesQueryKey,
  laneParticipantsQueryKey,
  laneQueryKey,
  laneWebSocketURL,
  organizationMembersQueryKey,
  patchLane,
  postLaneMessage,
  type LaneAgent,
  type LaneMessage,
} from '@/lib/api'

export const Route = createFileRoute('/_app/lanes/$laneId')({
  component: LaneChatPage,
})

function LaneChatPage() {
  const { laneId } = Route.useParams()
  const { me } = useRouteContext({ from: '/_app' })
  const queryClient = useQueryClient()
  const [text, setText] = useState('')
  const [partial, setPartial] = useState('')
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteUserId, setInviteUserId] = useState('')

  const laneQuery = useQuery({
    queryKey: laneQueryKey(laneId),
    queryFn: () => getLane(laneId),
  })
  const messagesQuery = useQuery({
    queryKey: laneMessagesQueryKey(laneId),
    queryFn: () => getLaneMessages(laneId),
  })
  const participantsQuery = useQuery({
    queryKey: laneParticipantsQueryKey(laneId),
    queryFn: () => getLaneParticipants(laneId),
  })
  const agentsQuery = useQuery({
    queryKey: laneAgentsQueryKey,
    queryFn: getLaneAgents,
  })
  const membersQuery = useQuery({
    queryKey: organizationMembersQueryKey,
    queryFn: getOrganizationMembers,
  })

  const lane = laneQuery.data
  const messages = messagesQuery.data?.messages ?? []
  const participants = participantsQuery.data?.participants ?? []
  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((a) => a.available),
    [agentsQuery.data],
  )
  const isOwner = lane?.owner_user_id === me.user.id

  const [wsReady, setWsReady] = useState<WebSocket | null>(null)
  const [turnRunning, setTurnRunning] = useState(false)

  useEffect(() => {
    const url = laneWebSocketURL(laneId)
    const ws = new WebSocket(url)
    setWsReady(ws)
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data as string) as {
          type?: string
          message?: LaneMessage
          event?: {
            type?: string
            body?: string
            done?: boolean
            status?: string
          }
        }
        if (data.type === 'timeline' && data.message) {
          void queryClient.invalidateQueries({
            queryKey: laneMessagesQueryKey(laneId),
          })
          setPartial('')
        }
        if (data.type === 'partial' && data.event?.type === 'status') {
          setTurnRunning(data.event.status === 'running')
          if (data.event.status === 'idle') {
            setPartial('')
            void queryClient.invalidateQueries({
              queryKey: laneMessagesQueryKey(laneId),
            })
          }
        }
        if (data.type === 'partial' && data.event?.type === 'assistant_message') {
          if (data.event.body) {
            setPartial((prev) => prev + data.event!.body)
          }
          if (data.event.done) {
            void queryClient.invalidateQueries({
              queryKey: laneMessagesQueryKey(laneId),
            })
            setPartial('')
          }
        }
      } catch {
        // ignore malformed frames
      }
    }
    return () => {
      ws.close()
      setWsReady(null)
    }
  }, [laneId, queryClient])

  const sendMutation = useMutation({
    mutationFn: () => postLaneMessage(laneId, text),
    onSuccess: async () => {
      setText('')
      await queryClient.invalidateQueries({
        queryKey: laneMessagesQueryKey(laneId),
      })
    },
  })

  const switchMutation = useMutation({
    mutationFn: (agent: LaneAgent) =>
      patchLane(laneId, { agent_provider: agent.provider }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: laneQueryKey(laneId) })
      await queryClient.invalidateQueries({
        queryKey: laneMessagesQueryKey(laneId),
      })
    },
  })

  const inviteMutation = useMutation({
    mutationFn: () =>
      createLaneInvite(laneId, {
        email: inviteEmail.trim() || undefined,
        user_id: inviteUserId.trim() || undefined,
      }),
    onSuccess: async () => {
      setInviteEmail('')
      setInviteUserId('')
      await queryClient.invalidateQueries({
        queryKey: laneParticipantsQueryKey(laneId),
      })
    },
  })

  const kickMutation = useMutation({
    mutationFn: (userId: string) => kickLaneParticipant(laneId, userId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneParticipantsQueryKey(laneId),
      })
    },
  })

  const timeline = messages.filter(
    (m) =>
      m.event_type === 'user_message' ||
      m.event_type === 'assistant_message' ||
      m.event_type === 'agent_changed' ||
      m.event_type === 'participant_joined' ||
      m.event_type === 'participant_removed',
  )

  return (
    <div className="mx-auto grid w-full max-w-5xl gap-6 lg:grid-cols-[1fr_16rem]">
      <div className="space-y-4">
        <div className="space-y-1">
          {lane ? (
            <p className="text-muted-foreground text-sm">
              <Link
                to="/projects/$projectId"
                params={{ projectId: lane.project_id }}
                className="underline underline-offset-4"
              >
                Project
              </Link>
            </p>
          ) : null}
          <h1 className="text-2xl font-semibold tracking-tight">
            {lane?.name ?? 'Lane'}
          </h1>
          <p className="text-muted-foreground text-sm">
            Agent:{' '}
            <span className="text-foreground font-medium">
              {lane?.agent_provider ?? '…'}
            </span>{' '}
            · {lane?.status}
          </p>
        </div>

        <div className="flex min-h-[360px] flex-col gap-3 rounded-md border p-3">
          <div className="flex-1 space-y-3 overflow-y-auto">
            {timeline.map((m) => (
              <div key={m.id} className="text-sm">
                <p className="text-muted-foreground text-xs">
                  {m.event_type}
                  {m.author_user_id ? ` · ${m.author_user_id.slice(0, 8)}` : ''}
                </p>
                <p className="whitespace-pre-wrap">{m.body}</p>
              </div>
            ))}
            {partial ? (
              <div className="text-sm">
                <p className="text-muted-foreground text-xs">assistant…</p>
                <p className="whitespace-pre-wrap">{partial}</p>
              </div>
            ) : null}
          </div>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (!text.trim()) return
              sendMutation.mutate()
            }}
          >
            <Input
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Message the Lane…"
              disabled={sendMutation.isPending || turnRunning}
            />
            <Button
              type="submit"
              disabled={!text.trim() || sendMutation.isPending || turnRunning}
            >
              Send
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={!turnRunning || !wsReady || wsReady.readyState !== WebSocket.OPEN}
              onClick={() => {
                wsReady?.send(JSON.stringify({ type: 'interrupt' }))
                setTurnRunning(false)
              }}
            >
              Stop
            </Button>
          </form>
          {sendMutation.isError ? (
            <p className="text-destructive text-sm">
              {sendMutation.error.message}
            </p>
          ) : null}
        </div>
      </div>

      <aside className="space-y-6">
        {isOwner ? (
          <div className="space-y-2">
            <Label>Switch agent</Label>
            <Combobox
              items={availableAgents}
              onValueChange={(agent: LaneAgent | null) => {
                if (!agent) return
                if (
                  !window.confirm(
                    'Switching agents starts a new AI session. Farplane will hand off a summary of this chat. Files in the Lane computer stay as they are.',
                  )
                ) {
                  return
                }
                switchMutation.mutate(agent)
              }}
              itemToStringLabel={(a: LaneAgent) => a.label}
              itemToStringValue={(a: LaneAgent) => a.provider}
              isItemEqualToValue={(a: LaneAgent, b: LaneAgent) =>
                a.provider === b.provider
              }
            >
              <ComboboxInput placeholder="Choose agent" className="w-full" />
              <ComboboxContent>
                <ComboboxEmpty>No available agents</ComboboxEmpty>
                <ComboboxList>
                  {(a: LaneAgent) => (
                    <ComboboxItem key={a.provider} value={a}>
                      {a.label}
                    </ComboboxItem>
                  )}
                </ComboboxList>
              </ComboboxContent>
            </Combobox>
            {switchMutation.isError ? (
              <p className="text-destructive text-xs">
                {switchMutation.error.message}
              </p>
            ) : null}
          </div>
        ) : null}

        <div className="space-y-2">
          <h2 className="text-sm font-medium">Participants</h2>
          <ul className="space-y-1 text-sm">
            {participants.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between gap-2"
              >
                <span className="truncate">
                  {p.display_name || p.email || p.user_id.slice(0, 8)}
                  {p.role === 'owner' ? ' · owner' : ''}
                </span>
                {isOwner && p.role !== 'owner' ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={kickMutation.isPending}
                    onClick={() => kickMutation.mutate(p.user_id)}
                  >
                    Kick
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </div>

        {isOwner ? (
          <div className="space-y-2">
            <h2 className="text-sm font-medium">Invite</h2>
            <Label htmlFor="invite-user">Org member user id</Label>
            <Input
              id="invite-user"
              list="org-members"
              value={inviteUserId}
              onChange={(e) => setInviteUserId(e.target.value)}
              placeholder="Pick or paste user id"
            />
            <datalist id="org-members">
              {(membersQuery.data?.members ?? []).map((m) => (
                <option key={m.id} value={m.id}>
                  {m.display_name} ({m.email})
                </option>
              ))}
            </datalist>
            <Label htmlFor="invite-email">Or email</Label>
            <Input
              id="invite-email"
              type="email"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
            />
            <Button
              type="button"
              size="sm"
              disabled={
                inviteMutation.isPending ||
                (!inviteEmail.trim() && !inviteUserId.trim())
              }
              onClick={() => inviteMutation.mutate()}
            >
              Send invite
            </Button>
            {inviteMutation.isError ? (
              <p className="text-destructive text-xs">
                {inviteMutation.error.message}
              </p>
            ) : null}
          </div>
        ) : null}
      </aside>
    </div>
  )
}
