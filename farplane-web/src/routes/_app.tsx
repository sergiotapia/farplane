import {
  createFileRoute,
  isRedirect,
  Outlet,
  redirect,
} from '@tanstack/react-router'

import { AppSidebar } from '@/components/app-sidebar.tsx'
import { Separator } from '@/components/ui/separator.tsx'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from '@/components/ui/sidebar.tsx'
import { TooltipProvider } from '@/components/ui/tooltip.tsx'
import { ApiError, getMe, type MeResponse, meQueryKey } from '@/lib/api.ts'

export type AppRouteContext = {
  me: MeResponse
}

export const Route = createFileRoute('/_app')({
  beforeLoad: async ({ context }): Promise<AppRouteContext> => {
    try {
      const me = await context.queryClient.fetchQuery({
        queryKey: meQueryKey,
        queryFn: getMe,
        staleTime: 0,
      })
      return { me }
    } catch (error) {
      if (isRedirect(error)) {
        throw error
      }
      if (error instanceof ApiError && error.status === 401) {
        context.queryClient.removeQueries({ queryKey: meQueryKey })
        throw redirect({ to: '/login' })
      }
      throw error
    }
  },
  component: AppLayout,
})

function AppLayout() {
  return (
    <TooltipProvider>
      <SidebarProvider>
        <AppSidebar />
        <SidebarInset>
          <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            <span className="text-muted-foreground text-sm">Farplane</span>
          </header>
          <div className="flex flex-1 flex-col p-6">
            <Outlet />
          </div>
        </SidebarInset>
      </SidebarProvider>
    </TooltipProvider>
  )
}
