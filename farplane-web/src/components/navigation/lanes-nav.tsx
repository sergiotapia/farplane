import { useState, type ReactElement } from 'react'
import {
  ChevronRightIcon,
  FolderIcon,
  FolderOpenIcon,
  PlusIcon,
  RouteIcon,
  UsersIcon,
} from 'lucide-react'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
} from '@/components/ui/sidebar'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

type LaneStatus = 'idle' | 'working'

type MockLane = {
  id: string
  name: string
  status: LaneStatus
  /** True when the current user was invited rather than creating the lane. */
  invited?: boolean
}

type MockProjectGroup = {
  id: string
  name: string
  lanes: MockLane[]
  defaultOpen?: boolean
}

/** Placeholder data for sidebar UX exploration only. */
const mockProjectGroups: MockProjectGroup[] = [
  {
    id: 'proj-farplane',
    name: 'farplane',
    defaultOpen: true,
    lanes: [
      { id: 'lane-sidebar', name: 'Sidebar lanes UX', status: 'working' },
      {
        id: 'lane-templates',
        name: 'Lane template editor sync',
        status: 'idle',
      },
      { id: 'lane-chat', name: 'AI chat data flow', status: 'idle' },
      {
        id: 'lane-projects',
        name: 'Projects page repository picker',
        status: 'idle',
      },
      {
        id: 'lane-github',
        name: 'GitHub App code review',
        status: 'idle',
        invited: true,
      },
    ],
  },
  {
    id: 'proj-docs',
    name: 'docs-site',
    defaultOpen: true,
    lanes: [
      { id: 'lane-landing', name: 'landing-copy', status: 'working' },
      {
        id: 'lane-api-ref',
        name: 'api-reference',
        status: 'idle',
        invited: true,
      },
    ],
  },
  {
    id: 'proj-billing',
    name: 'billing-api',
    defaultOpen: false,
    lanes: [{ id: 'lane-invoices', name: 'invoice-export', status: 'idle' }],
  },
]

const mockUngroupedLanes: MockLane[] = [
  { id: 'lane-scratch', name: 'scratchpad', status: 'idle' },
  {
    id: 'lane-spike',
    name: 'friday-spike',
    status: 'working',
    invited: true,
  },
]

export function LanesNav() {
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(
      mockProjectGroups.map((group) => [group.id, group.defaultOpen ?? true]),
    ),
  )

  return (
    // First hover waits; once open, sibling lane tooltips show instantly.
    // Leaving the group long enough (timeout) restores the delay.
    <TooltipProvider delay={200} timeout={400}>
      <SidebarGroup>
        <SidebarGroupLabel>
          <RouteIcon className="size-3.5" />
          <span>Lanes</span>
        </SidebarGroupLabel>
        <SidebarGroupAction title="New lane" aria-label="New lane">
          <PlusIcon />
        </SidebarGroupAction>
        <SidebarGroupContent>
          <SidebarMenu>
            {mockProjectGroups.map((group, groupIndex) => {
              const isOpen = openGroups[group.id] ?? true
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
                  <SidebarMenuItem className={groupIndex > 0 ? 'mt-2.5' : undefined}>
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
                      showOnHover
                      title={`New lane in ${group.name}`}
                      aria-label={`New lane in ${group.name}`}
                    >
                      <PlusIcon />
                    </SidebarMenuAction>
                    <CollapsibleContent>
                      <SidebarMenuSub className="mx-0 translate-x-0 border-0 px-0">
                        {group.lanes.map((lane) => (
                          <SidebarMenuSubItem key={lane.id}>
                            <LaneTooltip label={lane.name}>
                              <SidebarMenuSubButton
                                render={<button type="button" />}
                                className="w-full translate-x-0"
                              >
                                <LaneStatusIcon status={lane.status} />
                                <span className="min-w-0 flex-1 truncate">
                                  {lane.name}
                                </span>
                                {lane.invited ? (
                                  <UsersIcon className="ml-auto shrink-0 opacity-60" />
                                ) : null}
                              </SidebarMenuSubButton>
                            </LaneTooltip>
                          </SidebarMenuSubItem>
                        ))}
                      </SidebarMenuSub>
                    </CollapsibleContent>
                  </SidebarMenuItem>
                </Collapsible>
              )
            })}

            {mockUngroupedLanes.length > 0 ? (
              <>
              <SidebarMenuItem className="pointer-events-none mt-3">
                <span className="px-2 text-[11px] font-medium tracking-wide text-sidebar-foreground/45 uppercase">
                  No project
                </span>
              </SidebarMenuItem>
                {mockUngroupedLanes.map((lane) => (
                  <LaneMenuItem key={lane.id} lane={lane} />
                ))}
              </>
            ) : null}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </TooltipProvider>
  )
}

function LaneMenuItem({ lane }: { lane: MockLane }) {
  return (
    <SidebarMenuItem>
      <LaneTooltip label={lane.name}>
        <SidebarMenuButton
          render={<button type="button" />}
          className="text-sidebar-foreground/90"
        >
          <LaneStatusIcon status={lane.status} />
          <span className="min-w-0 flex-1 truncate">{lane.name}</span>
        </SidebarMenuButton>
      </LaneTooltip>
      {lane.invited ? (
        <SidebarMenuBadge title="Invited">
          <UsersIcon className="size-3 opacity-70" />
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
        <svg viewBox="0 0 16 16" className="size-3.5" fill="currentColor">
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
