import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'

import { DockerfileEditor } from '@/components/dockerfile-editor'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  ApiError,
  forkLaneTemplate,
  getLaneTemplates,
  laneTemplatesQueryKey,
  updateLaneTemplate,
  validateLaneTemplate,
  type LaneTemplate,
} from '@/lib/api'

export const Route = createFileRoute(
  '/_app/settings/lane-templates/$templateId',
)({
  component: LaneTemplateDetailPage,
})

function lintLogFromError(error: unknown): string | null {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== 'object') {
    return null
  }
  const log = (error.body as { last_validation_log?: unknown }).last_validation_log
  return typeof log === 'string' && log.trim() ? log : null
}

function LaneTemplateDetailPage() {
  const { templateId } = Route.useParams()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const templatesQuery = useQuery({
    queryKey: laneTemplatesQueryKey,
    queryFn: getLaneTemplates,
  })
  const templates = templatesQuery.data?.lane_templates ?? []
  const selected = templates.find((t) => t.id === templateId) ?? null

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [dockerfileText, setDockerfileText] = useState('')
  const [lintLog, setLintLog] = useState<string | null>(null)

  useEffect(() => {
    if (!selected) return
    setName(selected.name)
    setDescription(selected.description)
    setDockerfileText(selected.dockerfile_text)
    setLintLog(selected.last_validation_log ?? null)
  }, [selected?.id, selected?.updated_at])

  const saveMutation = useMutation({
    mutationFn: () =>
      updateLaneTemplate(templateId, {
        name,
        description,
        dockerfile_text: dockerfileText,
      }),
    onSuccess: async (t) => {
      setLintLog(t.last_validation_log ?? null)
      queryClient.setQueryData(
        laneTemplatesQueryKey,
        (prev: { lane_templates: LaneTemplate[] } | undefined) => {
          if (!prev) return prev
          return {
            lane_templates: prev.lane_templates.map((row) =>
              row.id === t.id ? t : row,
            ),
          }
        },
      )
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
    },
    onError: (error) => {
      const log = lintLogFromError(error)
      if (log) setLintLog(log)
    },
  })

  const validateMutation = useMutation({
    mutationFn: () => validateLaneTemplate(templateId),
    onSuccess: async (t) => {
      setLintLog(t.last_validation_log ?? null)
      queryClient.setQueryData(
        laneTemplatesQueryKey,
        (prev: { lane_templates: LaneTemplate[] } | undefined) => {
          if (!prev) return prev
          return {
            lane_templates: prev.lane_templates.map((row) =>
              row.id === t.id ? t : row,
            ),
          }
        },
      )
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
    },
  })

  const forkMutation = useMutation({
    mutationFn: () => forkLaneTemplate(templateId),
    onSuccess: async (t) => {
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
      await navigate({
        to: '/settings/lane-templates/$templateId',
        params: { templateId: t.id },
      })
    },
  })

  if (templatesQuery.isLoading) {
    return <p className="text-muted-foreground text-sm">Loading templates…</p>
  }

  if (!selected) {
    return (
      <p className="text-muted-foreground text-sm">
        Template not found.{' '}
        <Link
          to="/settings/lane-templates"
          className="underline underline-offset-4"
        >
          Back to list
        </Link>
      </p>
    )
  }

  const saveBlockedByLint =
    saveMutation.isError && saveMutation.error instanceof ApiError
      ? saveMutation.error.status === 422
      : false

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="template-name">Name</Label>
          <Input
            id="template-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="template-description">Description</Label>
          <Input
            id="template-description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <div className="flex items-baseline justify-between gap-2">
          <Label htmlFor="dockerfile">Dockerfile</Label>
          <span className="text-muted-foreground text-xs">
            Status: {selected.validation_status}
          </span>
        </div>
        <DockerfileEditor
          id="dockerfile"
          value={dockerfileText}
          onChange={(value) => {
            setDockerfileText(value)
            if (saveMutation.isError) saveMutation.reset()
          }}
        />
        <p className="text-muted-foreground text-xs">
          Save lints the Dockerfile and only persists when lint passes. Validate
          build marks the template valid or invalid.
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          disabled={saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
        >
          {saveMutation.isPending ? 'Saving…' : 'Save'}
        </Button>
        <Button
          type="button"
          variant="secondary"
          disabled={validateMutation.isPending}
          onClick={() => validateMutation.mutate()}
        >
          {validateMutation.isPending ? 'Validating…' : 'Validate build'}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={forkMutation.isPending}
          onClick={() => forkMutation.mutate()}
        >
          Fork
        </Button>
      </div>
      {saveBlockedByLint ? (
        <p className="text-destructive text-sm">
          Save blocked: Dockerfile lint failed. Fix the issues below and try
          again.
        </p>
      ) : null}
      {(saveMutation.isError && !saveBlockedByLint) ||
      validateMutation.isError ||
      forkMutation.isError ? (
        <p className="text-destructive text-sm">
          {(
            saveMutation.error ||
            validateMutation.error ||
            forkMutation.error
          )?.message}
        </p>
      ) : null}
      {validateMutation.isSuccess &&
      validateMutation.data?.validation_status === 'invalid' ? (
        <p className="text-destructive text-sm">
          Validate failed. The template is invalid until a build succeeds. See
          the log below.
        </p>
      ) : null}
      {validateMutation.isSuccess &&
      validateMutation.data?.validation_status === 'valid' ? (
        <p className="text-sm text-emerald-700 dark:text-emerald-400">
          Valid. This template can be used for new Lanes.
        </p>
      ) : null}
      {lintLog ? (
        <pre className="bg-muted max-h-64 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap">
          {lintLog}
        </pre>
      ) : null}
      <p className="text-muted-foreground text-xs">
        Need API keys for agents? Configure them in{' '}
        <Link to="/settings/secrets" className="underline underline-offset-4">
          Secrets
        </Link>
        .
      </p>
    </div>
  )
}
