import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  createProject,
  getGitHubRepositories,
  getProjects,
  githubRepositoriesQueryKey,
  projectsQueryKey,
} from '@/lib/api'

export const Route = createFileRoute('/_app/projects')({
  component: ProjectsPage,
})

function ProjectsPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [repositoryId, setRepositoryId] = useState('')

  const projectsQuery = useQuery({
    queryKey: projectsQueryKey,
    queryFn: getProjects,
  })

  const repositoriesQuery = useQuery({
    queryKey: githubRepositoriesQueryKey,
    queryFn: () => getGitHubRepositories(false),
  })

  const createMutation = useMutation({
    mutationFn: createProject,
    onSuccess: async () => {
      setName('')
      setRepositoryId('')
      await queryClient.invalidateQueries({ queryKey: projectsQueryKey })
    },
  })

  const projects = projectsQuery.data?.projects ?? []
  const repositories = repositoriesQuery.data?.repositories ?? []

  return (
    <div className="mx-auto w-full max-w-2xl space-y-8">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">Projects</h1>
        <p className="text-muted-foreground text-sm">
          A Project is one app repository. Connect GitHub in{' '}
          <Link
            to="/settings/github"
            search={{}}
            className="underline underline-offset-4"
          >
            Settings
          </Link>
          , then pick a repository here.
        </p>
      </div>

      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault()
          const githubRepositoryId = Number(repositoryId)
          if (!name.trim() || !Number.isFinite(githubRepositoryId)) return
          createMutation.mutate({
            name: name.trim(),
            github_repository_id: githubRepositoryId,
          })
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="project-name">Name</Label>
          <Input
            id="project-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Rails app"
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="project-repo">GitHub repository</Label>
          <select
            id="project-repo"
            className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
            value={repositoryId}
            onChange={(event) => setRepositoryId(event.target.value)}
            required
          >
            <option value="">Select a repository…</option>
            {repositories.map((repo) => (
              <option
                key={repo.github_repository_id}
                value={String(repo.github_repository_id)}
              >
                {repo.full_name}
                {repo.private ? ' (private)' : ''}
              </option>
            ))}
          </select>
          {repositories.length === 0 && !repositoriesQuery.isLoading ? (
            <p className="text-muted-foreground text-xs">
              No repositories yet. Connect GitHub first.
            </p>
          ) : null}
        </div>
        {createMutation.isError ? (
          <p className="text-destructive text-sm">
            {(createMutation.error as Error).message}
          </p>
        ) : null}
        <Button
          type="submit"
          disabled={
            createMutation.isPending ||
            !name.trim() ||
            !repositoryId ||
            repositories.length === 0
          }
        >
          {createMutation.isPending ? 'Creating…' : 'Create project'}
        </Button>
      </form>

      <div className="space-y-3">
        <h2 className="text-lg font-medium">Your projects</h2>
        {projectsQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">Loading…</p>
        ) : null}
        {projects.length === 0 && !projectsQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">No projects yet.</p>
        ) : null}
        <ul className="divide-border divide-y rounded-md border">
          {projects.map((project) => (
            <li key={project.id} className="space-y-1 px-4 py-3">
              <p className="font-medium">{project.name}</p>
              <p className="text-muted-foreground text-xs">
                {project.github_full_name} · {project.default_branch}
                {project.github_access_status === 'revoked'
                  ? ' · access revoked'
                  : ''}
              </p>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
