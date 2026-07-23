import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
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
  getProjectEnvironment,
  getProjects,
  getScratchEnvironment,
  laneAgentsQueryKey,
  lanesQueryKey,
  projectEnvironmentQueryKey,
  projectLanesQueryKey,
  projectsQueryKey,
  scratchEnvironmentQueryKey,
  type LaneAgent,
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
  const [agent, setAgent] = useState<LaneAgent | null>(null)
  const [error, setError] = useState<string | null>(null)

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
    enabled: open,
  })
  const scratchEnvQuery = useQuery({
    queryKey: scratchEnvironmentQueryKey,
    queryFn: getScratchEnvironment,
    enabled: open,
  })
  const projectEnvQuery = useQuery({
    queryKey: projectEnvironmentQueryKey(projectChoice?.id ?? ''),
    queryFn: () => getProjectEnvironment(projectChoice!.id),
    enabled: open && projectChoice?.kind === 'project',
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

  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((a) => a.available),
    [agentsQuery.data],
  )

  const environmentReady = useMemo(() => {
    if (!projectChoice) return false
    if (projectChoice.kind === 'scratch') {
      return scratchEnvQuery.data?.validation_status === 'valid'
    }
    return projectEnvQuery.data?.validation_status === 'valid'
  }, [projectChoice, scratchEnvQuery.data, projectEnvQuery.data])

  useEffect(() => {
    if (!open) return
    setName('Lane')
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
    !!agent &&
    environmentReady &&
    !createMutation.isPending

  const environmentHint = (() => {
    if (!projectChoice) return null
    if (projectChoice.kind === 'scratch') {
      if (scratchEnvQuery.isLoading) return 'Checking Scratch Environment…'
      if (scratchEnvQuery.data?.validation_status === 'valid') return null
      return (
        <>
          Validate the Scratch Environment in{' '}
          <Link
            to="/settings/scratch-environment"
            className="underline underline-offset-4"
          >
            Settings → Scratch Environment
          </Link>{' '}
          first.
        </>
      )
    }
    if (projectEnvQuery.isLoading) return 'Checking Project Environment…'
    if (projectEnvQuery.data?.validation_status === 'valid') return null
    if (projectEnvQuery.data == null) {
      return (
        <>
          This Project has no environment yet. Set one up on the{' '}
          <Link
            to="/projects/$projectId"
            params={{ projectId: projectChoice.id }}
            className="underline underline-offset-4"
          >
            project page
          </Link>
          .
        </>
      )
    }
    return (
      <>
        Validate the Project Environment on the{' '}
        <Link
          to="/projects/$projectId"
          params={{ projectId: projectChoice.id }}
          className="underline underline-offset-4"
        >
          project page
        </Link>{' '}
        first.
      </>
    )
  })()

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
            Start a shared chat and Runtime from a validated environment.
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
          {environmentHint ? (
            <p className="text-muted-foreground text-sm">{environmentHint}</p>
          ) : null}
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
