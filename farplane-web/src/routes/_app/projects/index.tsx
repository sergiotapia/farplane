import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { Lock } from 'lucide-react'
import { useState } from 'react'

import { Button } from '@/components/ui/button.tsx'
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from '@/components/ui/combobox.tsx'
import { Input } from '@/components/ui/input.tsx'
import { Label } from '@/components/ui/label.tsx'
import {
  createProject,
  type GitHubRepository,
  getGitHubRepositories,
  getProjects,
  githubRepositoriesQueryKey,
  projectsQueryKey,
} from '@/lib/api.ts'

export const Route = createFileRoute('/_app/projects/')({
  component: ProjectsPage,
})

function repositoryShortName(fullName: string): string {
  const slash = fullName.lastIndexOf('/')
  return slash === -1 ? fullName : fullName.slice(slash + 1)
}

function ProjectsPage() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [selectedRepository, setSelectedRepository] =
    useState<GitHubRepository | null>(null)

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
      setSelectedRepository(null)
      await queryClient.invalidateQueries({ queryKey: projectsQueryKey })
    },
  })

  const projects = projectsQuery.data?.projects ?? []
  const repositories = repositoriesQuery.data?.repositories ?? []
  // Wait for projects before filtering; otherwise every repo looks free
  // while projects are still loading and duplicates can be submitted.
  const pickerReady = projectsQuery.isSuccess && repositoriesQuery.isSuccess
  const linkedRepositoryIds = new Set(
    projects.map((project) => project.github_repository_id),
  )
  const availableRepositories = pickerReady
    ? repositories.filter(
        (repo) => !linkedRepositoryIds.has(repo.github_repository_id),
      )
    : []
  const pickerDisabled = !pickerReady || availableRepositories.length === 0

  if (
    selectedRepository &&
    pickerReady &&
    !availableRepositories.some(
      (repo) =>
        repo.github_repository_id === selectedRepository.github_repository_id,
    )
  ) {
    setSelectedRepository(null)
  }

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
          if (!(selectedRepository && name.trim()) || pickerDisabled) return
          createMutation.mutate({
            name: name.trim(),
            github_repository_id: selectedRepository.github_repository_id,
          })
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="project-repo">GitHub repository</Label>
          <Combobox
            items={availableRepositories}
            value={selectedRepository}
            onValueChange={(repo) => {
              setSelectedRepository(repo)
              if (repo) {
                setName(repositoryShortName(repo.full_name))
              } else {
                setName('')
              }
            }}
            itemToStringLabel={(repo) => repo.full_name}
            itemToStringValue={(repo) => String(repo.github_repository_id)}
            isItemEqualToValue={(a, b) =>
              a.github_repository_id === b.github_repository_id
            }
            disabled={pickerDisabled}
          >
            <ComboboxInput
              id="project-repo"
              placeholder={
                pickerReady ? 'Search repositories…' : 'Loading repositories…'
              }
              className="w-full"
              showClear={true}
              disabled={pickerDisabled}
            />
            <ComboboxContent>
              <ComboboxEmpty>No repositories found.</ComboboxEmpty>
              <ComboboxList>
                {(repo) => (
                  <ComboboxItem key={repo.github_repository_id} value={repo}>
                    {repo.private ? (
                      <Lock
                        className="text-muted-foreground"
                        aria-label="Private repository"
                      />
                    ) : null}
                    <span className="truncate">{repo.full_name}</span>
                  </ComboboxItem>
                )}
              </ComboboxList>
            </ComboboxContent>
          </Combobox>
          {repositoriesQuery.isSuccess && repositories.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              No repositories yet. Connect GitHub first.
            </p>
          ) : null}
          {pickerReady &&
          repositories.length > 0 &&
          availableRepositories.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              Every connected repository already has a project.
            </p>
          ) : null}
          {projectsQuery.isError || repositoriesQuery.isError ? (
            <p className="text-destructive text-xs">
              Failed to load repositories. Refresh and try again.
            </p>
          ) : null}
        </div>
        <div className="space-y-2">
          <Label htmlFor="project-name">Name</Label>
          <Input
            id="project-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Rails app"
            required={true}
          />
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
            !selectedRepository ||
            pickerDisabled
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
            <li key={project.id}>
              <Link
                to="/projects/$projectId"
                params={{ projectId: project.id }}
                className="hover:bg-muted/50 block space-y-1 px-4 py-3"
              >
                <p className="font-medium">{project.name}</p>
                <p className="text-muted-foreground text-xs">
                  {project.github_full_name} · {project.default_branch}
                  {project.github_access_status === 'revoked'
                    ? ' · access revoked'
                    : ''}
                </p>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
