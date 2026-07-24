import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Check, Hammer, Save, Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'

import { DockerfileEditor } from '@/components/dockerfile-editor.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  ApiError,
  generateProjectEnvironment,
  getProjectEnvironment,
  getProjects,
  projectEnvironmentQueryKey,
  projectsQueryKey,
  upsertProjectEnvironment,
  validateProjectEnvironment,
} from '@/lib/api.ts'

export const Route = createFileRoute('/_app/projects/$projectId')({
  component: ProjectEnvironmentPage,
})

function lintLogFromError(error: unknown): string | null {
  if (
    !(error instanceof ApiError && error.body) ||
    typeof error.body !== 'object'
  ) {
    return null
  }
  const log = (error.body as { last_validation_log?: unknown })
    .last_validation_log
  return typeof log === 'string' && log.trim() ? log : null
}

function ProjectEnvironmentPage() {
  const { projectId } = Route.useParams()
  const queryClient = useQueryClient()
  const [dockerfileText, setDockerfileText] = useState('')
  const [lintLog, setLintLog] = useState<string | null>(null)
  const [generationLog, setGenerationLog] = useState<string | null>(null)

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
  })
  const project = projectsQuery.data?.projects.find((p) => p.id === projectId)

  const envQuery = useQuery({
    queryKey: projectEnvironmentQueryKey(projectId),
    queryFn: () => getProjectEnvironment(projectId),
  })
  const env = envQuery.data ?? null

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
      if (
        error instanceof ApiError &&
        error.body &&
        typeof error.body === 'object'
      ) {
        const pe = (error.body as { project_environment?: unknown })
          .project_environment
        if (pe && typeof pe === 'object') {
          queryClient.setQueryData(projectEnvironmentQueryKey(projectId), pe)
        }
      }
    },
  })

  const hasEnvironment = !!env
  const envValid = env?.validation_status === 'valid'
  const dirty = env
    ? dockerfileText !== env.dockerfile_text
    : dockerfileText.trim().length > 0

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
          Configure the Project Environment Dockerfile that Lanes for this
          Project run in.
        </p>
      </div>

      <section className="space-y-4">
        <div className="space-y-1">
          <h2 className="text-lg font-medium">Project Environment</h2>
          <p className="text-muted-foreground text-sm">
            {hasEnvironment
              ? 'Edit the Dockerfile for this Project, then validate it.'
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
              <span className="text-foreground font-medium">
                Not configured
              </span>
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
              disabled={
                !hasEnvironment || dirty || validateEnvMutation.isPending
              }
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
          <p className="text-destructive text-sm">
            {generateMutation.error.message}
          </p>
        ) : null}
        {saveEnvMutation.isError ? (
          <p className="text-destructive text-sm">
            {saveEnvMutation.error.message}
          </p>
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
          lines={36}
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
    </div>
  )
}
