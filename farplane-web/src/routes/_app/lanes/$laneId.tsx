import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Link,
  createFileRoute,
  useNavigate,
  useRouteContext,
} from '@tanstack/react-router'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  addLaneParticipant,
  createLaneInvite,
  destroyLane,
  getActiveLaneInvite,
  getLane,
  getLaneAgents,
  getLaneMessages,
  getLaneParticipants,
  getOrganizationMembers,
  laneActiveInviteQueryKey,
  laneAgentsQueryKey,
  laneMessagesQueryKey,
  laneParticipantsQueryKey,
  laneQueryKey,
  laneWebSocketURL,
  lanesQueryKey,
  leaveLane,
  organizationMembersQueryKey,
  patchLane,
  postLaneMessage,
  regenerateLaneInvite,
  removeLaneParticipant,
  revokeActiveLaneInvite,
  type LaneAgent,
  type LaneMessage,
} from '@/lib/api'

type LaneSearch = {
  panel?: 'members'
}

export const Route = createFileRoute('/_app/lanes/$laneId')({
  validateSearch: (search: Record<string, unknown>): LaneSearch => ({
    panel: search.panel === 'members' ? 'members' : undefined,
  }),
  component: LaneChatPage,
})

function LaneChatPage() {
  const { laneId } = Route.useParams()
  const { panel } = Route.useSearch()
  const navigate = useNavigate()
  const { me } = useRouteContext({ from: '/_app' })
  const queryClient = useQueryClient()
  const [text, setText] = useState('')
  const [partial, setPartial] = useState('')
  const [addMember, setAddMember] = useState<{
    id: string
    display_name: string
    email: string
  } | null>(null)
  const [membersError, setMembersError] = useState<string | null>(null)
  const [destroyDialogOpen, setDestroyDialogOpen] = useState(false)

  const membersOpen = panel === 'members'

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
    enabled: membersOpen,
  })
  const activeInviteQuery = useQuery({
    queryKey: laneActiveInviteQueryKey(laneId),
    queryFn: () => getActiveLaneInvite(laneId),
    enabled: membersOpen,
    retry: false,
  })

  const lane = laneQuery.data
  const messages = messagesQuery.data?.messages ?? []
  const participants = participantsQuery.data?.participants ?? []
  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((a) => a.available),
    [agentsQuery.data],
  )
  const isOwner = lane?.owner_user_id === me.user.id
  const seatedIds = useMemo(
    () => new Set(participants.map((p) => p.user_id)),
    [participants],
  )
  const addableMembers = useMemo(
    () =>
      (membersQuery.data?.members ?? []).filter((m) => !seatedIds.has(m.id)),
    [membersQuery.data, seatedIds],
  )

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

  function closeMembersPanel() {
    void navigate({
      to: '/lanes/$laneId',
      params: { laneId },
      search: { panel: undefined },
    })
  }

  function openMembersPanel() {
    void navigate({
      to: '/lanes/$laneId',
      params: { laneId },
      search: { panel: 'members' },
    })
  }

  async function refreshAfterPeopleChange() {
    await queryClient.invalidateQueries({
      queryKey: laneParticipantsQueryKey(laneId),
    })
    await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
  }

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

  const addMutation = useMutation({
    mutationFn: (userId: string) => addLaneParticipant(laneId, userId),
    onSuccess: async () => {
      setAddMember(null)
      setMembersError(null)
      await refreshAfterPeopleChange()
    },
    onError: (err) => {
      setMembersError(
        err instanceof ApiError ? err.message : 'Could not add participant',
      )
    },
  })

  const removeMutation = useMutation({
    mutationFn: (userId: string) => removeLaneParticipant(laneId, userId),
    onSuccess: async () => {
      await refreshAfterPeopleChange()
    },
  })

  const leaveMutation = useMutation({
    mutationFn: () => leaveLane(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
      await navigate({ to: '/' })
    },
  })

  const destroyMutation = useMutation({
    mutationFn: () => destroyLane(laneId),
    onSuccess: async () => {
      setDestroyDialogOpen(false)
      await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
      await navigate({ to: '/' })
    },
  })

  const ensureInviteMutation = useMutation({
    mutationFn: () => createLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const regenerateInviteMutation = useMutation({
    mutationFn: () => regenerateLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const revokeInviteMutation = useMutation({
    mutationFn: () => revokeActiveLaneInvite(laneId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: laneActiveInviteQueryKey(laneId),
      })
    },
  })

  const activeInvite =
    activeInviteQuery.isSuccess ? activeInviteQuery.data : null

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
          {lane?.project_id ? (
            <p className="text-muted-foreground text-sm">
              <Link
                to="/projects/$projectId"
                params={{ projectId: lane.project_id }}
                className="underline underline-offset-4"
              >
                Project
              </Link>
            </p>
          ) : lane ? (
            <p className="text-muted-foreground text-sm">No project</p>
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

        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={openMembersPanel}
          >
            Members
          </Button>
          {!isOwner ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={leaveMutation.isPending}
              onClick={() => {
                if (!window.confirm('Leave this Lane?')) return
                leaveMutation.mutate()
              }}
            >
              {leaveMutation.isPending ? 'Leaving…' : 'Leave'}
            </Button>
          ) : (
            <Button
              type="button"
              size="sm"
              variant="destructive"
              disabled={destroyMutation.isPending}
              onClick={() => {
                if (destroyMutation.isError) destroyMutation.reset()
                setDestroyDialogOpen(true)
              }}
            >
              Destroy Lane
            </Button>
          )}
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
              disabled={
                !turnRunning || !wsReady || wsReady.readyState !== WebSocket.OPEN
              }
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
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-sm font-medium">Participants</h2>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={openMembersPanel}
            >
              Manage
            </Button>
          </div>
          <ul className="space-y-1 text-sm">
            {participants.map((p) => (
              <li key={p.id} className="truncate">
                {p.display_name || p.email || p.user_id.slice(0, 8)}
                {p.role === 'owner' ? ' · owner' : ''}
              </li>
            ))}
          </ul>
        </div>
      </aside>

      <Dialog
        open={destroyDialogOpen}
        onOpenChange={(open) => {
          if (destroyMutation.isPending) return
          setDestroyDialogOpen(open)
        }}
      >
        <DialogContent showCloseButton={!destroyMutation.isPending}>
          <DialogHeader>
            <DialogTitle>Destroy this Lane?</DialogTitle>
            <DialogDescription>
              {lane?.name ? (
                <>
                  “{lane.name}” will disappear for everyone, and its computer
                  will shut down. You can’t undo this.
                </>
              ) : (
                <>
                  This Lane will disappear for everyone, and its computer will
                  shut down. You can’t undo this.
                </>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={destroyMutation.isPending}
              onClick={() => setDestroyDialogOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={destroyMutation.isPending}
              onClick={() => destroyMutation.mutate()}
            >
              {destroyMutation.isPending ? 'Destroying…' : 'Destroy Lane'}
            </Button>
          </DialogFooter>
          {destroyMutation.isError ? (
            <p className="text-destructive text-sm">
              {destroyMutation.error instanceof ApiError
                ? destroyMutation.error.message
                : 'Could not destroy Lane'}
            </p>
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog
        open={membersOpen}
        onOpenChange={(open) => {
          if (!open) closeMembersPanel()
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Members</DialogTitle>
            <DialogDescription>
              Add Organization members immediately, or share an open Lane Invite
              link.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-6">
            <div className="space-y-2">
              <h3 className="text-sm font-medium">Participants</h3>
              <ul className="space-y-2 text-sm">
                {participants.map((p) => (
                  <li
                    key={p.id}
                    className="flex items-center justify-between gap-2"
                  >
                    <span className="min-w-0 truncate">
                      {p.display_name || p.email || p.user_id.slice(0, 8)}
                      {p.role === 'owner' ? ' · owner' : ''}
                    </span>
                    {p.role !== 'owner' && p.user_id !== me.user.id ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={removeMutation.isPending}
                        onClick={() => removeMutation.mutate(p.user_id)}
                      >
                        Remove
                      </Button>
                    ) : null}
                  </li>
                ))}
              </ul>
            </div>

            <div className="space-y-2">
              <Label>Add Organization member</Label>
              <Combobox
                items={addableMembers}
                value={addMember}
                onValueChange={setAddMember}
                itemToStringLabel={(m) => `${m.display_name} (${m.email})`}
                itemToStringValue={(m) => m.id}
                isItemEqualToValue={(a, b) => a.id === b.id}
              >
                <ComboboxInput
                  placeholder="Select a member"
                  className="w-full"
                />
                <ComboboxContent>
                  <ComboboxEmpty>No members to add</ComboboxEmpty>
                  <ComboboxList>
                    {(m) => (
                      <ComboboxItem key={m.id} value={m}>
                        {m.display_name} ({m.email})
                      </ComboboxItem>
                    )}
                  </ComboboxList>
                </ComboboxContent>
              </Combobox>
              <Button
                type="button"
                size="sm"
                disabled={!addMember || addMutation.isPending}
                onClick={() => {
                  if (!addMember) return
                  addMutation.mutate(addMember.id)
                }}
              >
                {addMutation.isPending ? 'Adding…' : 'Add participant'}
              </Button>
            </div>

            <div className="space-y-2">
              <h3 className="text-sm font-medium">Lane Invite link</h3>
              {activeInvite ? (
                <div className="space-y-2">
                  <Input readOnly value={activeInvite.accept_url} />
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        void navigator.clipboard.writeText(
                          activeInvite.accept_url,
                        )
                      }
                    >
                      Copy URL
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={regenerateInviteMutation.isPending}
                      onClick={() => {
                        if (
                          !window.confirm(
                            'Regenerate the invite link? The old link stops working.',
                          )
                        ) {
                          return
                        }
                        regenerateInviteMutation.mutate()
                      }}
                    >
                      Regenerate
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={revokeInviteMutation.isPending}
                      onClick={() => revokeInviteMutation.mutate()}
                    >
                      Revoke
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  disabled={
                    ensureInviteMutation.isPending ||
                    activeInviteQuery.isLoading
                  }
                  onClick={() => ensureInviteMutation.mutate()}
                >
                  {ensureInviteMutation.isPending
                    ? 'Creating…'
                    : 'Create invite link'}
                </Button>
              )}
            </div>

            {membersError ? (
              <p className="text-destructive text-sm">{membersError}</p>
            ) : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={closeMembersPanel}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
