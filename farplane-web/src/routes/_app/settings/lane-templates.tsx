import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Link,
  Outlet,
  createFileRoute,
  useNavigate,
  useRouterState,
} from '@tanstack/react-router'

import { Button } from '@/components/ui/button'
import {
  createLaneTemplate,
  getLaneTemplates,
  laneTemplatesQueryKey,
  type LaneTemplate,
} from '@/lib/api'

export const Route = createFileRoute('/_app/settings/lane-templates')({
  component: LaneTemplatesLayout,
})

function LaneTemplatesLayout() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const templatesQuery = useQuery({
    queryKey: laneTemplatesQueryKey,
    queryFn: getLaneTemplates,
  })
  const templates = templatesQuery.data?.lane_templates ?? []

  const createMutation = useMutation({
    mutationFn: () => {
      const existing = new Set(templates.map((t) => t.name))
      let name = 'Custom template'
      let n = 2
      while (existing.has(name)) {
        name = `Custom template ${n}`
        n += 1
      }
      return createLaneTemplate({
        name,
        description: '',
        dockerfile_text: 'FROM debian:bookworm-slim\n',
      })
    },
    onSuccess: async (t) => {
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
      await navigate({
        to: '/settings/lane-templates/$templateId',
        params: { templateId: t.id },
      })
    },
  })

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 lg:flex-row">
      <aside className="w-full shrink-0 space-y-3 lg:w-64">
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            Lane Templates
          </h1>
          <p className="text-muted-foreground text-sm">
            Edit Dockerfiles for Lane computers. Validate with a real docker
            build before using a template on a new Lane.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={createMutation.isPending}
          onClick={() => createMutation.mutate()}
        >
          New template
        </Button>
        {createMutation.isError ? (
          <p className="text-destructive text-sm">
            {createMutation.error.message}
          </p>
        ) : null}
        <ul className="space-y-1">
          {templates.map((t) => {
            const href = `/settings/lane-templates/${t.id}`
            const isActive =
              pathname === href || pathname.endsWith(`/${t.id}`)
            return (
              <li key={t.id}>
                <Link
                  to="/settings/lane-templates/$templateId"
                  params={{ templateId: t.id }}
                  className={`block w-full rounded-md px-3 py-2 text-left text-sm ${
                    isActive
                      ? 'bg-muted font-medium'
                      : 'hover:bg-muted/60'
                  }`}
                >
                  <span className="block truncate">{t.name}</span>
                  <span className="text-muted-foreground text-xs">
                    {statusLabel(t)}
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      </aside>

      <section className="min-w-0 flex-1">
        <Outlet />
      </section>
    </div>
  )
}

function statusLabel(t: LaneTemplate): string {
  const base = t.validation_status === 'valid' ? 'valid' : 'invalid'
  return t.is_system_default ? `${base} · default` : base
}
