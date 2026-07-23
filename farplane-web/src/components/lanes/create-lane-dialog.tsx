import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo, useState } from 'react'

import { Button } from '@/components/ui/button'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  createLane,
  getLaneAgents,
  getLaneTemplates,
  getProjects,
  laneAgentsQueryKey,
  laneTemplatesQueryKey,
  lanesQueryKey,
  projectLanesQueryKey,
  projectsQueryKey,
  type LaneAgent,
  type LaneTemplate,
  type Project,
} from '@/lib/api'

export type CreateLanePrefill =
  | { mode: 'pick' }
  | { mode: 'project'; projectId: string }
  | { mode: 'scratch' }

type ProjectChoice =
  | { kind: 'scratch'; id: 'scratch'; name: string }
  | { kind: 'project'; id: string; name: string; project: Project }

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  prefill: CreateLanePrefill
}

export function CreateLaneDialog({ open, onOpenChange, prefill }: Props) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [name, setName] = useState('Lane')
  const [projectChoice, setProjectChoice] = useState<ProjectChoice | null>(null)
  const [template, setTemplate] = useState<LaneTemplate | null>(null)
  const [agent, setAgent] = useState<LaneAgent | null>(null)
  const [error, setError] = useState<string | null>(null)

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
    enabled: open,
  })
  const templatesQuery = useQuery({
    queryKey: laneTemplatesQueryKey,
    queryFn: getLaneTemplates,
    enabled: open,
  })
  const agentsQuery = useQuery({
    queryKey: laneAgentsQueryKey,
    queryFn: getLaneAgents,
    enabled: open,
  })

  const projectChoices = useMemo((): ProjectChoice[] => {
    const scratch: ProjectChoice = {
      kind: 'scratch',
      id: 'scratch',
      name: 'No project',
    }
    const projects = (projectsQuery.data?.projects ?? []).map(
      (project): ProjectChoice => ({
        kind: 'project',
        id: project.id,
        name: project.name,
        project,
      }),
    )
    return [scratch, ...projects]
  }, [projectsQuery.data])

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

  useEffect(() => {
    if (!open) return
    setName('Lane')
    setTemplate(null)
    setAgent(null)
    setError(null)
    if (prefill.mode === 'scratch') {
      setProjectChoice({
        kind: 'scratch',
        id: 'scratch',
        name: 'No project',
      })
      return
    }
    if (prefill.mode === 'project') {
      const project = projectsQuery.data?.projects.find(
        (p) => p.id === prefill.projectId,
      )
      if (project) {
        setProjectChoice({
          kind: 'project',
          id: project.id,
          name: project.name,
          project,
        })
      } else {
        setProjectChoice(null)
      }
      return
    }
    setProjectChoice(null)
  }, [open, prefill, projectsQuery.data])

  const createMutation = useMutation({
    mutationFn: () =>
      createLane({
        name: name.trim() || 'Lane',
        lane_template_id: template!.id,
        agent_provider: agent!.provider,
        project_id:
          projectChoice?.kind === 'project' ? projectChoice.id : null,
      }),
    onSuccess: async (lane) => {
      await queryClient.invalidateQueries({ queryKey: lanesQueryKey })
      if (lane.project_id) {
        await queryClient.invalidateQueries({
          queryKey: projectLanesQueryKey(lane.project_id),
        })
      }
      onOpenChange(false)
      await navigate({ to: '/lanes/$laneId', params: { laneId: lane.id } })
    },
    onError: (err) => {
      setError(
        err instanceof ApiError ? err.message : 'Could not create Lane',
      )
    },
  })

  const canSubmit =
    !!projectChoice &&
    !!template &&
    !!agent &&
    !createMutation.isPending

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (createMutation.isPending) return
        onOpenChange(next)
      }}
    >
      <DialogContent
        className="sm:max-w-md"
        showCloseButton={!createMutation.isPending}
      >
        <DialogHeader>
          <DialogTitle>Create Lane</DialogTitle>
          <DialogDescription>
            Start a shared chat and Runtime from a validated template.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!canSubmit) return
            setError(null)
            createMutation.mutate()
          }}
        >
          <div className="space-y-2">
            <Label>Project</Label>
            <Combobox
              items={projectChoices}
              value={projectChoice}
              onValueChange={setProjectChoice}
              itemToStringLabel={(p) => p.name}
              itemToStringValue={(p) => p.id}
              isItemEqualToValue={(a, b) => a.id === b.id}
            >
              <ComboboxInput
                placeholder="Select a project or No project"
                className="w-full"
              />
              <ComboboxContent>
                <ComboboxEmpty>No projects</ComboboxEmpty>
                <ComboboxList>
                  {(p) => (
                    <ComboboxItem key={p.id} value={p}>
                      {p.name}
                    </ComboboxItem>
                  )}
                </ComboboxList>
              </ComboboxContent>
            </Combobox>
          </div>
          <div className="space-y-2">
            <Label htmlFor="create-lane-name">Name</Label>
            <Input
              id="create-lane-name"
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
              <ComboboxInput placeholder="Select an agent" className="w-full" />
              <ComboboxContent>
                <ComboboxEmpty>
                  {availableAgents.length === 0
                    ? 'No agents available — set a secret in Settings → Secrets'
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
          {error ? <p className="text-destructive text-sm">{error}</p> : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={createMutation.isPending}
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={!canSubmit}>
              {createMutation.isPending ? 'Creating…' : 'Create Lane'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
