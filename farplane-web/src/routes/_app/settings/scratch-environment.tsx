import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Check, Hammer, Save } from 'lucide-react'
import { useEffect, useState } from 'react'

import { DockerfileEditor } from '@/components/dockerfile-editor.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  ApiError,
  getScratchEnvironment,
  scratchEnvironmentQueryKey,
  upsertScratchEnvironment,
  validateScratchEnvironment,
} from '@/lib/api.ts'

export const Route = createFileRoute('/_app/settings/scratch-environment')({
  component: ScratchEnvironmentPage,
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

function ScratchEnvironmentPage() {
  const queryClient = useQueryClient()
  const envQuery = useQuery({
    queryKey: scratchEnvironmentQueryKey,
    queryFn: getScratchEnvironment,
  })
  const env = envQuery.data ?? null

  const [dockerfileText, setDockerfileText] = useState('')
  const [lintLog, setLintLog] = useState<string | null>(null)

  useEffect(() => {
    if (!env) return
    setDockerfileText(env.dockerfile_text)
    setLintLog(env.last_validation_log ?? null)
  }, [env?.updated_at])

  const saveMutation = useMutation({
    mutationFn: () =>
      upsertScratchEnvironment({ dockerfile_text: dockerfileText }),
    onSuccess: async (next) => {
      setLintLog(next.last_validation_log ?? null)
      queryClient.setQueryData(scratchEnvironmentQueryKey, next)
      await queryClient.invalidateQueries({
        queryKey: scratchEnvironmentQueryKey,
      })
    },
    onError: (error) => {
      const log = lintLogFromError(error)
      if (log) setLintLog(log)
    },
  })

  const validateMutation = useMutation({
    mutationFn: () => validateScratchEnvironment(),
    onSuccess: async (next) => {
      setLintLog(next.last_validation_log ?? null)
      queryClient.setQueryData(scratchEnvironmentQueryKey, next)
      await queryClient.invalidateQueries({
        queryKey: scratchEnvironmentQueryKey,
      })
    },
  })

  const dirty = env ? dockerfileText !== env.dockerfile_text : false
  const status = env?.validation_status ?? 'invalid'

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">
          Scratch Environment
        </h1>
        <p className="text-muted-foreground text-sm">
          Organization-wide Dockerfile for Scratch Lanes (no Project). Edit,
          save, then validate with a real docker build before creating Scratch
          Lanes.
        </p>
      </div>

      {envQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading…</p>
      ) : envQuery.isError ? (
        <p className="text-destructive text-sm">{envQuery.error.message}</p>
      ) : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-muted-foreground text-sm">
              Status:{' '}
              <span className="text-foreground font-medium">
                {status === 'valid' ? 'Valid' : 'Invalid — validate before use'}
              </span>
            </span>
            <div className="ml-auto flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={!dirty || saveMutation.isPending}
                onClick={() => saveMutation.mutate()}
              >
                <Save className="size-4" />
                {saveMutation.isPending ? 'Saving…' : 'Save'}
              </Button>
              <Button
                type="button"
                disabled={dirty || validateMutation.isPending || !env}
                onClick={() => validateMutation.mutate()}
              >
                {status === 'valid' ? (
                  <Check className="size-4" />
                ) : (
                  <Hammer className="size-4" />
                )}
                {validateMutation.isPending ? 'Validating…' : 'Validate'}
              </Button>
            </div>
          </div>

          {saveMutation.isError ? (
            <p className="text-destructive text-sm">
              {saveMutation.error.message}
            </p>
          ) : null}
          {validateMutation.isError ? (
            <p className="text-destructive text-sm">
              {validateMutation.error.message}
            </p>
          ) : null}

          <DockerfileEditor
            id="scratch-dockerfile"
            value={dockerfileText}
            onChange={setDockerfileText}
          />

          {lintLog ? (
            <pre className="bg-muted max-h-64 overflow-auto rounded-md p-3 text-xs whitespace-pre-wrap">
              {lintLog}
            </pre>
          ) : null}
        </div>
      )}
    </div>
  )
}
