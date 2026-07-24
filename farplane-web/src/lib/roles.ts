/** Owners and admins can create/manage the Farplane GitHub App. */
export function canManageGitHubApp(role: string): boolean {
  return role === 'owner' || role === 'admin'
}
