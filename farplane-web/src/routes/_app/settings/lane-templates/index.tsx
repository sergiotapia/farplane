import { useQuery } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'

import { getLaneTemplates, laneTemplatesQueryKey } from '@/lib/api'

export const Route = createFileRoute('/_app/settings/lane-templates/')({
  beforeLoad: async ({ context }) => {
    const data = await context.queryClient.ensureQueryData({
      queryKey: laneTemplatesQueryKey,
      queryFn: getLaneTemplates,
    })
    const first = data.lane_templates[0]
    if (first) {
      throw redirect({
        to: '/settings/lane-templates/$templateId',
        params: { templateId: first.id },
      })
    }
  },
  component: LaneTemplatesEmpty,
})

function LaneTemplatesEmpty() {
  const templatesQuery = useQuery({
    queryKey: laneTemplatesQueryKey,
    queryFn: getLaneTemplates,
  })
  if (templatesQuery.isLoading) {
    return <p className="text-muted-foreground text-sm">Loading templates…</p>
  }
  return (
    <p className="text-muted-foreground text-sm">
      No templates yet. Create one to get started.
    </p>
  )
}
