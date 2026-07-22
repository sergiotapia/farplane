import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Check, GitFork, Hammer, Save, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import { DockerfileEditor } from '@/components/dockerfile-editor'
import { Button } from '@/components/ui/button'
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
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ApiError,
  deleteLaneTemplate,
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
  const [forkDialogOpen, setForkDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

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
      setForkDialogOpen(false)
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
      await navigate({
        to: '/settings/lane-templates/$templateId',
        params: { templateId: t.id },
      })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteLaneTemplate(templateId),
    onSuccess: async () => {
      setDeleteDialogOpen(false)
      await queryClient.invalidateQueries({ queryKey: laneTemplatesQueryKey })
      await navigate({ to: '/settings/lane-templates' })
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

  const deleteBlockedReason = selected.is_system_default
    ? 'The default template cannot be deleted.'
    : selected.in_use
      ? 'This template is used by a Lane, so it cannot be deleted.'
      : null

  return (
    <div className="space-y-4">
      <div className="space-y-3">
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
          <Textarea
            id="template-description"
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <div className="flex items-baseline justify-between gap-2">
          <Label htmlFor="dockerfile">Dockerfile</Label>
          <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
            Status:{' '}
            {selected.validation_status === 'valid' ? (
              <span className="inline-flex items-center gap-1 text-emerald-700 dark:text-emerald-400">
                valid
                <Check className="size-3.5" aria-hidden />
              </span>
            ) : (
              selected.validation_status
            )}
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
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            disabled={validateMutation.isPending}
            onClick={() => validateMutation.mutate()}
          >
            <Hammer data-icon="inline-start" />
            {validateMutation.isPending ? 'Validating…' : 'Validate build'}
          </Button>
          <Button
            type="button"
            disabled={saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            <Save data-icon="inline-start" />
            {saveMutation.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              if (forkMutation.isError) forkMutation.reset()
              setForkDialogOpen(true)
            }}
          >
            <GitFork data-icon="inline-start" />
            Fork
          </Button>
          {deleteBlockedReason ? (
            <Tooltip>
              <TooltipTrigger
                render={
                  <span className="inline-flex">
                    <Button type="button" variant="destructive" disabled>
                      <Trash2 data-icon="inline-start" />
                      Delete
                    </Button>
                  </span>
                }
              />
              <TooltipContent>{deleteBlockedReason}</TooltipContent>
            </Tooltip>
          ) : (
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                if (deleteMutation.isError) deleteMutation.reset()
                setDeleteDialogOpen(true)
              }}
            >
              <Trash2 data-icon="inline-start" />
              Delete
            </Button>
          )}
        </div>
      </div>
      <Dialog
        open={forkDialogOpen}
        onOpenChange={(open) => {
          if (forkMutation.isPending) return
          setForkDialogOpen(open)
        }}
      >
        <DialogContent showCloseButton={!forkMutation.isPending}>
          <DialogHeader>
            <DialogTitle>Fork this template?</DialogTitle>
            <DialogDescription>
              This creates a copy of “{selected.name}” that you can edit on your
              own.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={forkMutation.isPending}
              onClick={() => setForkDialogOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              disabled={forkMutation.isPending}
              onClick={() => forkMutation.mutate()}
            >
              {forkMutation.isPending ? 'Forking…' : 'Fork template'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={deleteDialogOpen}
        onOpenChange={(open) => {
          if (deleteMutation.isPending) return
          setDeleteDialogOpen(open)
        }}
      >
        <DialogContent showCloseButton={!deleteMutation.isPending}>
          <DialogHeader>
            <DialogTitle>Delete this template?</DialogTitle>
            <DialogDescription>
              This will permanently remove “{selected.name}”. You cannot undo
              this.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={deleteMutation.isPending}
              onClick={() => setDeleteDialogOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              {deleteMutation.isPending ? 'Deleting…' : 'Delete template'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {saveBlockedByLint ? (
        <p className="text-destructive text-sm">
          Save blocked: Dockerfile lint failed. Fix the issues below and try
          again.
        </p>
      ) : null}
      {(saveMutation.isError && !saveBlockedByLint) ||
      validateMutation.isError ||
      forkMutation.isError ||
      deleteMutation.isError ? (
        <p className="text-destructive text-sm">
          {(
            saveMutation.error ||
            validateMutation.error ||
            forkMutation.error ||
            deleteMutation.error
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
