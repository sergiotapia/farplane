import {
  Outlet,
  createRootRouteWithContext,
  redirect,
} from '@tanstack/react-router'
import { TanStackRouterDevtoolsPanel } from '@tanstack/react-router-devtools'
import { TanStackDevtools } from '@tanstack/react-devtools'
import type { QueryClient } from '@tanstack/react-query'

import { getSetupStatus, setupStatusQueryKey, type SetupStatus } from '@/lib/api'

import '../styles.css'

export type RouterContext = {
  queryClient: QueryClient
  setupStatus?: SetupStatus
}

export const Route = createRootRouteWithContext<RouterContext>()({
  beforeLoad: async ({ context, location }) => {
    // fetchQuery (not ensureQueryData): after setup we invalidate this query;
    // ensureQueryData returns stale cache forever if any data exists.
    const setupStatus = await context.queryClient.fetchQuery({
      queryKey: setupStatusQueryKey,
      queryFn: getSetupStatus,
      staleTime: 30_000,
    })

    const onSetup = location.pathname === '/setup'

    if (setupStatus.needs_setup) {
      if (!onSetup) {
        throw redirect({ to: '/setup' })
      }
    } else if (onSetup) {
      throw redirect({ to: '/login' })
    }

    return { setupStatus }
  },
  component: RootComponent,
})

function RootComponent() {
  return (
    <>
      <div id="root-content">
        <Outlet />
      </div>

      <div id="portal-root" />

      <TanStackDevtools
        config={{
          position: 'bottom-right',
        }}
        plugins={[
          {
            name: 'TanStack Router',
            render: <TanStackRouterDevtoolsPanel />,
          },
        ]}
      />
    </>
  )
}
