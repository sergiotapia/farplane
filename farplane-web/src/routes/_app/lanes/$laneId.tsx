import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createFileRoute,
  useNavigate,
  useRouteContext,
} from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { LaneAgentStrip } from '@/components/lanes/lane-agent-strip'
import { LaneConversationPanel } from '@/components/lanes/lane-conversation-panel'
import { LaneMembersDialog } from '@/components/lanes/lane-members-dialog'
import { LanePreviewPanel } from '@/components/lanes/lane-preview-panel'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ApiError,
  destroyLane,
  getLane,
  laneQueryKey,
  laneWebSocketURL,
  lanesQueryKey,
  leaveLane,
} from '@/lib/api'

type LaneSearch = {
  panel?: 'members'
}

export const Route = createFileRoute('/_app/lanes/$laneId')({
  validateSearch: (search: Record<string, unknown>): LaneSearch => ({
    panel: search.panel === 'members' ? 'members' : undefined,
  }),
  component: LaneDetailPage,
})

function LaneDetailPage() {
  const { laneId } = Route.useParams()
  const { panel } = Route.useSearch()
  const navigate = useNavigate()
  const { me } = useRouteContext({ from: '/_app' })
  const queryClient = useQueryClient()
  const [destroyDialogOpen, setDestroyDialogOpen] = useState(false)
  const [wsReady, setWsReady] = useState<WebSocket | null>(null)
  const [turnRunning, setTurnRunning] = useState(false)

  const membersOpen = panel === 'members'

  const laneQuery = useQuery({
    queryKey: laneQueryKey(laneId),
    queryFn: () => getLane(laneId),
  })

  const lane = laneQuery.data
  const isOwner = lane?.owner_user_id === me.user.id

  useEffect(() => {
    setTurnRunning(Boolean(lane?.turn_running))
  }, [laneId, lane?.turn_running])

  useEffect(() => {
    const url = laneWebSocketURL(laneId)
    const ws = new WebSocket(url)
    setWsReady(ws)
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data as string) as {
          type?: string
          event?: {
            type?: string
            status?: string
          }
        }
        if (data.type === 'partial' && data.event?.type === 'status') {
          setTurnRunning(data.event.status === 'running')
        }
      } catch {
        // ignore malformed frames
      }
    }
    return () => {
      ws.close()
      setWsReady(null)
    }
  }, [laneId])

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

  return (
    <div className="-m-6 flex min-h-[calc(100dvh-3rem)] flex-col">
      <div className="flex items-start justify-between gap-6 px-6 pt-5 pb-4">
        <div className="flex min-w-0 flex-col gap-1.5">
          <h1 className="truncate text-2xl font-semibold tracking-[-0.02em] text-foreground">
            {lane?.name ?? 'Lane'}
          </h1>
          <div className="flex items-center gap-2">
            {turnRunning ? (
              <span className="size-2 shrink-0 rounded-full bg-[#16A34A]" />
            ) : (
              <span className="size-2 shrink-0 rounded-full bg-muted-foreground/40" />
            )}
            <p className="text-[13px] leading-[18px] text-muted-foreground">
              {turnRunning ? 'Agent running' : 'Idle'}
            </p>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Button
            type="button"
            variant="outline"
            className="h-9 px-3.5"
            onClick={openMembersPanel}
          >
            Members
          </Button>
          {!isOwner ? (
            <Button
              type="button"
              variant="outline"
              className="h-9 px-3.5"
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
              variant="destructive"
              className="h-9 bg-destructive px-3.5 text-white hover:bg-destructive/90"
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
      </div>

      {lane ? (
        <div className="px-6 pb-4">
          <LaneAgentStrip
            lane={lane}
            isOwner={!!isOwner}
            turnRunning={turnRunning}
          />
        </div>
      ) : null}

      <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 px-6 pb-6 lg:grid-cols-[minmax(320px,420px)_minmax(0,1fr)]">
        <LaneConversationPanel
          turnRunning={turnRunning}
          canInterrupt={
            !!wsReady && wsReady.readyState === WebSocket.OPEN
          }
          onInterrupt={() => {
            // Keep turnRunning until the hub status frame confirms idle.
            wsReady?.send(JSON.stringify({ type: 'interrupt' }))
          }}
        />
        <LanePreviewPanel />
      </div>

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

      <LaneMembersDialog
        laneId={laneId}
        open={membersOpen}
        currentUser={me.user}
        onOpenChange={(open) => {
          if (!open) closeMembersPanel()
        }}
      />
    </div>
  )
}
