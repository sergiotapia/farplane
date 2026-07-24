import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Link,
  useNavigate,
  useRouteContext,
  useRouterState,
} from '@tanstack/react-router'
import {
  ContainerIcon,
  FolderKanbanIcon,
  GitBranchIcon,
  HomeIcon,
  KeyRoundIcon,
  LogOutIcon,
  SettingsIcon,
} from 'lucide-react'

import { LanesNav } from '@/components/navigation/lanes-nav.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarSeparator,
} from '@/components/ui/sidebar.tsx'
import { meQueryKey, postLogout } from '@/lib/api.ts'

const mainNavItems = [
  { to: '/', label: 'Home', icon: HomeIcon },
  { to: '/projects', label: 'Projects', icon: FolderKanbanIcon },
] as const

const settingsNavItems = [
  { to: '/settings/github', label: 'GitHub', icon: GitBranchIcon },
  {
    to: '/settings/scratch-environment',
    label: 'Scratch Environment',
    icon: ContainerIcon,
  },
  { to: '/settings/secrets', label: 'Secrets', icon: KeyRoundIcon },
] as const

export function AppSidebar() {
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { me } = useRouteContext({ from: '/_app' })

  const logoutMutation = useMutation({
    mutationFn: postLogout,
    onSettled: async () => {
      queryClient.removeQueries({ queryKey: meQueryKey })
      await navigate({ to: '/login' })
    },
  })

  const user = me.user
  const organization = me.organization

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={<Link to="/" />}
              className="font-semibold"
            >
              Farplane
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {mainNavItems.map((item) => {
                const Icon = item.icon
                const isActive =
                  item.to === '/'
                    ? pathname === '/'
                    : pathname.startsWith(item.to)
                return (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton
                      render={<Link to={item.to} />}
                      isActive={isActive}
                      tooltip={item.label}
                    >
                      <Icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <LanesNav />

        <SidebarGroup>
          <SidebarGroupLabel>
            <SettingsIcon className="size-3.5" />
            <span>Settings</span>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {settingsNavItems.map((item) => {
                const Icon = item.icon
                const isActive = pathname.startsWith(item.to)
                return (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton
                      render={<Link to={item.to} />}
                      isActive={isActive}
                      tooltip={item.label}
                    >
                      <Icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarSeparator />

      <SidebarFooter>
        <div className="flex flex-col gap-2 px-2 group-data-[collapsible=icon]:px-0">
          <div className="min-w-0 px-1 group-data-[collapsible=icon]:hidden">
            <p className="truncate text-sm font-medium">{user.display_name}</p>
            <p className="text-muted-foreground truncate text-xs">
              {user.email}
            </p>
            <p
              data-testid="active-organization-name"
              className="text-muted-foreground truncate text-xs"
            >
              {organization.name}
            </p>
          </div>

          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-full justify-start group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:p-0"
            disabled={logoutMutation.isPending}
            onClick={() => logoutMutation.mutate()}
          >
            <LogOutIcon />
            <span className="group-data-[collapsible=icon]:hidden">
              {logoutMutation.isPending ? 'Signing out…' : 'Sign out'}
            </span>
          </Button>
        </div>
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}
