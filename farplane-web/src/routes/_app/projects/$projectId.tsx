import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { Check, Hammer, Save, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { DockerfileEditor } from '@/components/dockerfile-editor'
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
  ApiError,
  createLane,
  generateProjectEnvironment,
  getLaneAgents,
  getProjectEnvironment,
  getProjectLanes,
  getProjects,
  laneAgentsQueryKey,
  lanesQueryKey,
  projectEnvironmentQueryKey,
  projectLanesQueryKey,
  projectsQueryKey,
  upsertProjectEnvironment,
  validateProjectEnvironment,
  type LaneAgent,
} from '@/lib/api'

export const Route = createFileRoute('/_app/projects/$projectId')({
  component: ProjectLanesPage,
})

function lintLogFromError(error: unknown): string | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== 'object') {
    return null
  }
  const log = (error.body as { last_validation_log?: unknown }).last_validation_log
  return typeof log === 'string' && log.trim() ? log : null
}

function ProjectLanesPage() {
  const { projectId } = Route.useParams()
  const queryClient = useQueryClient()
  const [name, setName] = useState('Lane')
  const [agent, setAgent] = useState<LaneAgent | null>(null)
  const [dockerfileText, setDockerfileText] = useState('')
  const [lintLog, setLintLog] = useState<string | null>(null)
  const [generationLog, setGenerationLog] = useState<string | null>(null)

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
  })
  const project = projectsQuery.data?.projects.find((p) => p.id === projectId)

  const lanesQuery = useQuery({
    queryKey: projectLanesQueryKey(projectId),
    queryFn: () => getProjectLanes(projectId),
  })

  const envQuery = useQuery({
    queryKey: projectEnvironmentQueryKey(projectId),
    queryFn: () => getProjectEnvironment(projectId),
  })
  const env = envQuery.data ?? null

  const agentsQuery = useQuery({
    queryKey: laneAgentsQueryKey,
    queryFn: getLaneAgents,
  })

  const availableAgents = useMemo(
    () => (agentsQuery.data?.agents ?? []).filter((a) => a.available),
    [agentsQuery.data],
  )

  useEffect(() => {
    if (!env) {
      setDockerfileText('')
      setLintLog(null)
      setGenerationLog(null)
      return
    }
    setDockerfileText(env.dockerfile_text)
    setLintLog(env.last_validation_log ?? null)
    setGenerationLog(env.generation_log ?? null)
  }, [env?.updated_at, env?.project_id])

  const saveEnvMutation = useMutation({
    mutationFn: () =>
      upsertProjectEnvironment(projectId, { dockerfile_text: dockerfileText }),
    onSuccess: async (next) => {
      setLintLog(next.last_validation_log ?? null)
      setGenerationLog(next.generation_log ?? null)
      queryClient.setQueryData(projectEnvironmentQueryKey(projectId), next)
      await queryClient.invalidateQueries({
        queryKey: projectEnvironmentQueryKey(projectId),
      })
    },
    onError: (error) => {
      const log = lintLogFromError(error)
      if (log) setLintLog(log)
    },
  })

  const validateEnvMutation = useMutation({
    mutationFn: () => validateProjectEnvironment(projectId),
    onSuccess: async (next) => {
      setLintLog(next.last_validation_log ?? null)
      queryClient.setQueryData(projectEnvironmentQueryKey(projectId), next)
      await queryClient.invalidateQueries({
        queryKey: projectEnvironmentQueryKey(projectId),
      })
    },
  })

  const generateMutation = useMutation({
    mutationFn: () => generateProjectEnvironment(projectId),
    onSuccess: async (next) => {
      setDockerfileText(next.dockerfile_text)
      setLintLog(next.last_validation_log ?? null)
      setGenerationLog(next.generation_log ?? null)
      queryClient.setQueryData(projectEnvironmentQueryKey(projectId), next)
      await queryClient.invalidateQueries({
        queryKey: projectEnvironmentQueryKey(projectId),
      })
    },
    onError: (error) => {
      if (error instanceof ApiError && error.body && typeof error.body === 'object') {
        const pe = (error.body as { project_environment?: unknown }).project_environment
        if (pe && typeof pe === 'object') {
          queryClient.setQueryData(projectEnvironmentQueryKey(projectId), pe)
        }
      }
    },
  })

  const createMutation = useMutation({
    mutationFn: () =>
      createLane({
        project_id: projectId,
        name,
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
  const hasEnvironment = !!env
  const envValid = env?.validation_status === 'valid'
  const dirty = env ? dockerfileText !== env.dockerfile_text : dockerfileText.trim().length > 0
  const canCreateLane = hasEnvironment && envValid && !!agent && !dirty

  return (
    <div className="mx-auto w-full max-w-4xl space-y-8">
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
          Set up the Project Environment Dockerfile, validate it, then create
          Lanes that run in that sandbox.
        </p>
      </div>

      <section className="space-y-4">
        <div className="space-y-1">
          <h2 className="text-lg font-medium">Project Environment</h2>
          <p className="text-muted-foreground text-sm">
            {hasEnvironment
              ? 'Edit the Dockerfile for this Project, then validate before creating Lanes.'
              : 'This Project has no environment yet. Generate one from the GitHub repository, or paste a Dockerfile and save.'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-sm">
            {hasEnvironment ? (
              <>
                Status:{' '}
                <span className="text-foreground font-medium">
                  {envValid ? 'Valid' : 'Invalid — validate before use'}
                </span>
              </>
            ) : (
              <span className="text-foreground font-medium">Not configured</span>
            )}
          </span>
          <div className="ml-auto flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={generateMutation.isPending}
              onClick={() => generateMutation.mutate()}
            >
              <Sparkles className="size-4" />
              {generateMutation.isPending ? 'Generating…' : 'Generate with AI'}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={
                !dockerfileText.trim() ||
                (!dirty && hasEnvironment) ||
                saveEnvMutation.isPending
              }
              onClick={() => saveEnvMutation.mutate()}
            >
              <Save className="size-4" />
              {saveEnvMutation.isPending ? 'Saving…' : 'Save'}
            </Button>
            <Button
              type="button"
              disabled={!hasEnvironment || dirty || validateEnvMutation.isPending}
              onClick={() => validateEnvMutation.mutate()}
            >
              {envValid ? (
                <Check className="size-4" />
              ) : (
                <Hammer className="size-4" />
              )}
              {validateEnvMutation.isPending ? 'Validating…' : 'Validate'}
            </Button>
          </div>
        </div>

        {generateMutation.isError ? (
          <p className="text-destructive text-sm">{generateMutation.error.message}</p>
        ) : null}
        {saveEnvMutation.isError ? (
          <p className="text-destructive text-sm">{saveEnvMutation.error.message}</p>
        ) : null}
        {validateEnvMutation.isError ? (
          <p className="text-destructive text-sm">
            {validateEnvMutation.error.message}
          </p>
        ) : null}

        <DockerfileEditor
          id="project-dockerfile"
          value={dockerfileText}
          onChange={setDockerfileText}
        />

        {generationLog ? (
          <pre className="bg-muted max-h-48 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap">
            {generationLog}
          </pre>
        ) : null}
        {lintLog ? (
          <pre className="bg-muted max-h-64 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap">
            {lintLog}
          </pre>
        ) : null}
      </section>

      <form
        className="space-y-4 rounded-md border p-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!canCreateLane) return
          createMutation.mutate()
        }}
      >
        <div className="space-y-1">
          <h2 className="text-lg font-medium">Create Lane</h2>
          <p className="text-muted-foreground text-sm">
            Requires a validated Project Environment and an agent whose secret
            is set.
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="lane-name">Name</Label>
          <Input
            id="lane-name"
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
        <Button type="submit" disabled={!canCreateLane || createMutation.isPending}>
          {createMutation.isPending ? 'Creating…' : 'Create Lane'}
        </Button>
        {!hasEnvironment ? (
          <p className="text-muted-foreground text-sm">
            Generate or save a Project Environment first.
          </p>
        ) : !envValid || dirty ? (
          <p className="text-muted-foreground text-sm">
            Save and validate the Project Environment before creating a Lane.
          </p>
        ) : null}
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
