import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  createLane,
  getLaneAgents,
  getLaneTemplates,
  getProjectLanes,
  getProjects,
  laneAgentsQueryKey,
  laneTemplatesQueryKey,
  lanesQueryKey,
  projectLanesQueryKey,
  projectsQueryKey,
  type LaneAgent,
  type LaneTemplate,
} from '@/lib/api'

export const Route = createFileRoute('/_app/projects/$projectId')({
  component: ProjectLanesPage,
})

function ProjectLanesPage() {
  const { projectId } = Route.useParams()
  const queryClient = useQueryClient()
  const [name, setName] = useState('Lane')
  const [template, setTemplate] = useState<LaneTemplate | null>(null)
  const [agent, setAgent] = useState<LaneAgent | null>(null)

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
  })
  const project = projectsQuery.data?.projects.find((p) => p.id === projectId)

  const lanesQuery = useQuery({
    queryKey: projectLanesQueryKey(projectId),
    queryFn: () => getProjectLanes(projectId),
  })

  const templatesQuery = useQuery({
    queryKey: laneTemplatesQueryKey,
    queryFn: getLaneTemplates,
  })
  const agentsQuery = useQuery({
    queryKey: laneAgentsQueryKey,
    queryFn: getLaneAgents,
  })

  const validTemplates = useMemo(
    () =>
      (templatesQuery.data?.lane_templates ?? []).filter(
        (t) => t.validation_status === 'valid',
      ),
    [templatesQuery.data],
  )
  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((a) => a.available),
    [agentsQuery.data],
  )

  const createMutation = useMutation({
    mutationFn: () =>
      createLane({
        project_id: projectId,
        name,
        lane_template_id: template!.id,
        agent_provider: agent!.provider,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: projectLanesQueryKey(projectId),
      })
      await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
    },
  })

  const lanes = lanesQuery.data?.lanes ?? []

  return (
    <div className="mx-auto w-full max-w-2xl space-y-8">
      <div className="space-y-2">
        <p className="text-muted-foreground text-sm">
          <Link to="/projects" className="underline underline-offset-4">
            Projects
          </Link>
        </p>
        <h1 className="text-2xl font-semibold tracking-tight">
          {project?.name ?? 'Project'}
        </h1>
        <p className="text-muted-foreground text-sm">
          Create a Lane from a validated template and an agent whose secret is
          set.
        </p>
      </div>

      <form
        className="space-y-4 rounded-md border p-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!template || !agent) return
          createMutation.mutate()
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="lane-name">Name</Label>
          <Input
            id="lane-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label>Lane template</Label>
          <Combobox
            items={validTemplates}
            value={template}
            onValueChange={setTemplate}
            itemToStringLabel={(t) => t.name}
            itemToStringValue={(t) => t.id}
            isItemEqualToValue={(a, b) => a.id === b.id}
          >
            <ComboboxInput
              placeholder="Select a validated template"
              className="w-full"
            />
            <ComboboxContent>
              <ComboboxEmpty>
                {validTemplates.length === 0
                  ? 'No validated templates — validate one in Settings → Lane Templates'
                  : 'No match'}
              </ComboboxEmpty>
              <ComboboxList>
                {(t) => (
                  <ComboboxItem key={t.id} value={t}>
                    {t.name}
                  </ComboboxItem>
                )}
              </ComboboxList>
            </ComboboxContent>
          </Combobox>
        </div>
        <div className="space-y-2">
          <Label>Agent</Label>
          <Combobox
            items={availableAgents}
            value={agent}
            onValueChange={setAgent}
            itemToStringLabel={(a) => a.label}
            itemToStringValue={(a) => a.provider}
            isItemEqualToValue={(a, b) => a.provider === b.provider}
          >
            <ComboboxInput
              placeholder="Select an available agent"
              className="w-full"
            />
            <ComboboxContent>
              <ComboboxEmpty>
                {availableAgents.length === 0
                  ? 'No agents available — set keys in Settings → Secrets'
                  : 'No match'}
              </ComboboxEmpty>
              <ComboboxList>
                {(a) => (
                  <ComboboxItem key={a.provider} value={a}>
                    {a.label}
                  </ComboboxItem>
                )}
              </ComboboxList>
            </ComboboxContent>
          </Combobox>
        </div>
        <Button
          type="submit"
          disabled={!template || !agent || createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating…' : 'Create Lane'}
        </Button>
        {createMutation.isError ? (
          <p className="text-destructive text-sm">
            {createMutation.error.message}
          </p>
        ) : null}
      </form>

      <div className="space-y-3">
        <h2 className="text-lg font-medium">Lanes</h2>
        {lanes.length === 0 ? (
          <p className="text-muted-foreground text-sm">No lanes yet.</p>
        ) : (
          <ul className="space-y-2">
            {lanes.map((lane) => (
              <li key={lane.id}>
                <Link
                  to="/lanes/$laneId"
                  params={{ laneId: lane.id }}
                  className="hover:bg-muted/60 flex items-center justify-between rounded-md border px-3 py-2 text-sm"
                >
                  <span className="font-medium">{lane.name}</span>
                  <span className="text-muted-foreground text-xs">
                    {lane.agent_provider} · {lane.status}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
