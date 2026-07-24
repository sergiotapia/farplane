import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ChevronRightIcon,
  FolderIcon,
  FolderOpenIcon,
  PlusIcon,
  RouteIcon,
  UsersIcon,
} from 'lucide-react'
import { type ReactElement, useEffect, useMemo, useState } from 'react'

import {
  CreateLaneDialog,
  type CreateLanePrefill,
} from '@/components/lanes/create-lane-dialog.tsx'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible.tsx'
import {
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar.tsx'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip.tsx'
import {
  type GroupedLanes,
  getLanes,
  type Lane,
  lanesQueryKey,
  lanesTurnWebSocketURL,
} from '@/lib/api.ts'
import { cn } from '@/lib/utils.ts'

type LaneStatus = 'idle' | 'working'

function laneStatus(lane: Lane): LaneStatus {
  return lane.turn_running ? 'working' : 'idle'
}

function patchTurnRunning(
  data: GroupedLanes,
  turns: Record<string, boolean>,
): GroupedLanes {
  const patch = (lane: Lane): Lane => {
    if (!(lane.id in turns)) return lane
    return { ...lane, turn_running: turns[lane.id] }
  }
  return {
    projects: data.projects.map((group) => ({
      ...group,
      lanes: group.lanes.map(patch),
    })),
    scratch_lanes: data.scratch_lanes.map(patch),
  }
}

export function LanesNav() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [createPrefill, setCreatePrefill] = useState<CreateLanePrefill>({
    mode: 'pick',
  })
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({})

  const lanesQuery = useQuery({
    queryKey: lanesQueryKey,
    queryFn: getLanes,
  })

  const projectGroups = lanesQuery.data?.projects ?? []
  const scratchLanes = lanesQuery.data?.scratch_lanes ?? []

  const laneIdsKey = useMemo(() => {
    const ids: string[] = []
    for (const group of projectGroups) {
      for (const lane of group.lanes) {
        ids.push(lane.id)
      }
    }
    for (const lane of scratchLanes) {
      ids.push(lane.id)
    }
    return ids.join(',')
  }, [projectGroups, scratchLanes])

  useEffect(() => {
    const laneIds = laneIdsKey === '' ? [] : laneIdsKey.split(',')
    const ws = new WebSocket(lanesTurnWebSocketURL())
    ws.onopen = () => {
      ws.send(JSON.stringify({ type: 'watch', lane_ids: laneIds }))
    }
    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data as string) as {
          type?: string
          lane_id?: string
          turn_running?: boolean
          turns?: Record<string, boolean>
        }
        if (data.type === 'snapshot' && data.turns) {
          const turns = data.turns
          queryClient.setQueryData<GroupedLanes>(lanesQueryKey, (prev) =>
            prev ? patchTurnRunning(prev, turns) : prev,
          )
          return
        }
        if (
          data.type === 'turn' &&
          data.lane_id &&
          typeof data.turn_running === 'boolean'
        ) {
          const laneId = data.lane_id
          const turnRunning = data.turn_running
          queryClient.setQueryData<GroupedLanes>(lanesQueryKey, (prev) =>
            prev
              ? patchTurnRunning(prev, {
                  [laneId]: turnRunning,
                })
              : prev,
          )
        }
      } catch {
        // ignore malformed frames
      }
    }
    return () => {
      ws.close()
    }
  }, [laneIdsKey, queryClient])

  const defaultOpen = useMemo(() => {
    const entries = projectGroups.map((group) => [
      group.id,
      openGroups[group.id] ?? true,
    ])
    return Object.fromEntries(entries) as Record<string, boolean>
  }, [projectGroups, openGroups])

  function openCreate(prefill: CreateLanePrefill) {
    setCreatePrefill(prefill)
    setCreateOpen(true)
  }

  return (
    <TooltipProvider delay={200} timeout={400}>
      <SidebarGroup>
        <SidebarGroupLabel>
          <RouteIcon className="size-3.5" />
          <span>Lanes</span>
        </SidebarGroupLabel>
        <SidebarGroupAction
          title="New lane"
          aria-label="New lane"
          onClick={() => openCreate({ mode: 'pick' })}
        >
          <PlusIcon />
        </SidebarGroupAction>
        <SidebarGroupContent>
          <SidebarMenu>
            {lanesQuery.isLoading ? (
              <SidebarMenuItem>
                <span className="px-2 text-xs text-sidebar-foreground/55">
                  Loading lanes…
                </span>
              </SidebarMenuItem>
            ) : null}

            {projectGroups.map((group, groupIndex) => {
              const isOpen = defaultOpen[group.id] ?? true
              return (
                <Collapsible
                  key={group.id}
                  open={isOpen}
                  onOpenChange={(open) =>
                    setOpenGroups((current) => ({
                      ...current,
                      [group.id]: open,
                    }))
                  }
                  className="group/collapsible"
                >
                  <SidebarMenuItem
                    className={groupIndex > 0 ? 'mt-2.5' : undefined}
                  >
                    <CollapsibleTrigger
                      render={
                        <SidebarMenuButton
                          tooltip={group.name}
                          className="group/folder"
                        />
                      }
                    >
                      <FolderIcon className="group-data-panel-open/folder:hidden" />
                      <FolderOpenIcon className="hidden group-data-panel-open/folder:block" />
                      <span className="min-w-0 truncate">{group.name}</span>
                      <ChevronRightIcon className="size-3! opacity-0 transition-[transform,opacity] duration-150 group-hover/menu-item:opacity-100 group-focus-within/menu-item:opacity-100 group-data-panel-open/folder:rotate-90" />
                    </CollapsibleTrigger>
                    <SidebarMenuAction
                      showOnHover={true}
                      title={`New lane in ${group.name}`}
                      aria-label={`New lane in ${group.name}`}
                      onClick={() =>
                        openCreate({ mode: 'project', projectId: group.id })
                      }
                    >
                      <PlusIcon />
                    </SidebarMenuAction>
                    <CollapsibleContent>
                      <SidebarMenuSub className="mx-0 translate-x-0 border-0 px-0">
                        {group.lanes.map((lane) => (
                          <SidebarMenuSubItem key={lane.id}>
                            <LaneTooltip label={lane.name}>
                              <SidebarMenuSubButton
                                render={
                                  <Link
                                    to="/lanes/$laneId"
                                    params={{ laneId: lane.id }}
                                  />
                                }
                                className="w-full translate-x-0"
                              >
                                <LaneStatusIcon status={laneStatus(lane)} />
                                <span className="min-w-0 flex-1 truncate">
                                  {lane.name}
                                </span>
                              </SidebarMenuSubButton>
                            </LaneTooltip>
                            {lane.has_other_participants ? (
                              <SidebarMenuBadge
                                title="Members"
                                className="pointer-events-auto"
                              >
                                <Link
                                  to="/lanes/$laneId"
                                  params={{ laneId: lane.id }}
                                  search={{ panel: 'members' }}
                                >
                                  <UsersIcon className="size-3 opacity-70" />
                                </Link>
                              </SidebarMenuBadge>
                            ) : null}
                          </SidebarMenuSubItem>
                        ))}
                      </SidebarMenuSub>
                    </CollapsibleContent>
                  </SidebarMenuItem>
                </Collapsible>
              )
            })}

            <SidebarMenuItem className="mt-3">
              <div className="flex items-center justify-between px-2">
                <span className="text-[11px] font-medium tracking-wide text-sidebar-foreground/45 uppercase">
                  No project
                </span>
                <button
                  type="button"
                  title="New scratch lane"
                  aria-label="New scratch lane"
                  className="rounded-md p-0.5 text-sidebar-foreground/55 hover:bg-sidebar-accent hover:text-sidebar-foreground"
                  onClick={() => openCreate({ mode: 'scratch' })}
                >
                  <PlusIcon className="size-3.5" />
                </button>
              </div>
            </SidebarMenuItem>
            {scratchLanes.map((lane) => (
              <LaneMenuItem key={lane.id} lane={lane} />
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <CreateLaneDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        prefill={createPrefill}
      />
    </TooltipProvider>
  )
}

function LaneMenuItem({ lane }: { lane: Lane }) {
  return (
    <SidebarMenuItem>
      <LaneTooltip label={lane.name}>
        <SidebarMenuButton
          render={<Link to="/lanes/$laneId" params={{ laneId: lane.id }} />}
          className="text-sidebar-foreground/90"
        >
          <LaneStatusIcon status={laneStatus(lane)} />
          <span className="min-w-0 flex-1 truncate">{lane.name}</span>
        </SidebarMenuButton>
      </LaneTooltip>
      {lane.has_other_participants ? (
        <SidebarMenuBadge title="Members" className="pointer-events-auto">
          <Link
            to="/lanes/$laneId"
            params={{ laneId: lane.id }}
            search={{ panel: 'members' }}
          >
            <UsersIcon className="size-3 opacity-70" />
          </Link>
        </SidebarMenuBadge>
      ) : null}
    </SidebarMenuItem>
  )
}

function LaneTooltip({
  label,
  children,
}: {
  label: string
  children: ReactElement
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent side="right" align="center">
        {label}
      </TooltipContent>
    </Tooltip>
  )
}

function LaneStatusIcon({ status }: { status: LaneStatus }) {
  if (status === 'working') {
    return (
      <span
        aria-hidden="true"
        className="flex size-3.5 shrink-0 items-center justify-center text-sidebar-foreground"
      >
        <svg
          viewBox="0 0 16 16"
          className="size-3.5"
          fill="currentColor"
          aria-hidden="true"
        >
          <rect
            x="2.5"
            y="4"
            width="2.2"
            height="8"
            rx="1"
            className="lane-bar"
            style={{ animationDelay: '0ms' }}
          />
          <rect
            x="6.9"
            y="4"
            width="2.2"
            height="8"
            rx="1"
            className="lane-bar"
            style={{ animationDelay: '120ms' }}
          />
          <rect
            x="11.3"
            y="4"
            width="2.2"
            height="8"
            rx="1"
            className="lane-bar"
            style={{ animationDelay: '240ms' }}
          />
        </svg>
      </span>
    )
  }

  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex size-3.5 shrink-0 items-center justify-center',
        'text-sidebar-foreground/55',
      )}
    >
      <span className="size-1.5 rounded-full bg-current" />
    </span>
  )
}
