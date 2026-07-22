import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useRouteContext, useRouterState } from '@tanstack/react-router'
import { HomeIcon, InfoIcon, LogOutIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarSeparator,
} from '@/components/ui/sidebar'
import { meQueryKey, postLogout } from '@/lib/api'

const navItems = [
  { to: '/', label: 'Home', icon: HomeIcon },
  { to: '/about', label: 'About', icon: InfoIcon },
] as const

export function AppSidebar() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
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
              {navItems.map((item) => {
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
      </SidebarContent>

      <SidebarSeparator />

      <SidebarFooter>
        <div className="flex flex-col gap-2 px-2 group-data-[collapsible=icon]:px-0">
          <div className="min-w-0 px-1 group-data-[collapsible=icon]:hidden">
            <p className="truncate text-sm font-medium">{user.display_name}</p>
            <p className="text-muted-foreground truncate text-xs">{user.email}</p>
            <p className="text-muted-foreground truncate text-xs">
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
