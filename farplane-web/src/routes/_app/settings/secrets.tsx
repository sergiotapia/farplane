import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useState } from 'react'

import { Button } from '@/components/ui/button.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Label } from '@/components/ui/label.tsx'
import {
  clearSecret,
  getSecrets,
  type OrganizationSecret,
  secretsQueryKey,
  setSecret,
} from '@/lib/api.ts'

export const Route = createFileRoute('/_app/settings/secrets')({
  component: SecretsPage,
})

function SecretsPage() {
  const queryClient = useQueryClient()
  const secretsQuery = useQuery({
    queryKey: secretsQueryKey,
    queryFn: getSecrets,
  })
  const secrets = secretsQuery.data?.secrets ?? []

  return (
    <div className="mx-auto w-full max-w-2xl space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">Secrets</h1>
        <p className="text-muted-foreground text-sm">
          Organization API keys for Lane agents. Values are encrypted at rest
          and never returned by the API. Agents in the Lane picker stay disabled
          until their required key is set. Edit the Scratch Environment in{' '}
          <Link
            to="/settings/scratch-environment"
            className="underline underline-offset-4"
          >
            Scratch Environment
          </Link>
          .
        </p>
      </div>

      {secretsQuery.isLoading ? (
        <p className="text-muted-foreground text-sm">Loading…</p>
      ) : null}

      <div className="space-y-4">
        {secrets.map((secret) => (
          <SecretRow
            key={secret.name}
            secret={secret}
            onChanged={async () => {
              await queryClient.invalidateQueries({
                queryKey: secretsQueryKey,
              })
              await queryClient.invalidateQueries({
                queryKey: ['lane-agents'],
              })
            }}
          />
        ))}
      </div>
    </div>
  )
}

function SecretRow({
  secret,
  onChanged,
}: {
  secret: OrganizationSecret
  onChanged: () => Promise<void>
}) {
  const [value, setValue] = useState('')
  const setMutation = useMutation({
    mutationFn: () => setSecret(secret.name, value),
    onSuccess: async () => {
      setValue('')
      await onChanged()
    },
  })
  const clearMutation = useMutation({
    mutationFn: () => clearSecret(secret.name),
    onSuccess: onChanged,
  })

  return (
    <div className="space-y-3 rounded-md border p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium">{secret.label}</h2>
          <p className="text-muted-foreground font-mono text-xs">
            {secret.name}
          </p>
        </div>
        <span
          className={`text-xs ${secret.is_set ? 'text-emerald-700 dark:text-emerald-400' : 'text-muted-foreground'}`}
        >
          {secret.is_set ? 'Set' : 'Not set'}
        </span>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`secret-${secret.name}`}>
          {secret.is_set ? 'Rotate value' : 'Set value'}
        </Label>
        <Input
          id={`secret-${secret.name}`}
          type="password"
          autoComplete="off"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={secret.is_set ? '••••••••' : 'Paste API key'}
        />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          disabled={!value.trim() || setMutation.isPending}
          onClick={() => setMutation.mutate()}
        >
          {setMutation.isPending ? 'Saving…' : 'Save'}
        </Button>
        {secret.is_set ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={clearMutation.isPending}
            onClick={() => clearMutation.mutate()}
          >
            Clear
          </Button>
        ) : null}
      </div>
      {setMutation.isError || clearMutation.isError ? (
        <p className="text-destructive text-sm">
          {(setMutation.error || clearMutation.error)?.message}
        </p>
      ) : null}
    </div>
  )
}
