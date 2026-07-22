const oauthErrorMessages: Record<string, string> = {
  google_denied: 'Google sign-in was cancelled.',
  invalid_state: 'Google sign-in expired. Try again.',
  missing_code: 'Google sign-in failed. Try again.',
  token_exchange_failed: 'Google sign-in failed. Try again.',
  userinfo_failed: 'Could not read your Google profile.',
  incomplete_profile: 'Your Google account needs a verified email.',
  account_not_found: 'No Farplane account is linked to that Google login.',
  login_failed: 'Google sign-in failed. Try again.',
  session_failed: 'Could not create a session. Try again.',
  setup_already_completed: 'This install is already set up. Sign in instead.',
  setup_failed: 'Google setup failed. Try again.',
  google_oauth_not_configured: 'Google sign-in is not configured on this install.',
  database_unavailable: 'The database is unavailable. Try again later.',
  invalid_intent: 'Google sign-in failed. Try again.',
}

const githubErrorMessages: Record<string, string> = {
  github_denied: 'GitHub install was cancelled.',
  invalid_state: 'GitHub install expired. Try again.',
  missing_installation: 'GitHub did not return an installation. Try again.',
  installation_lookup_failed: 'Could not read the GitHub installation.',
  install_save_failed: 'Could not save the GitHub connection.',
  repo_sync_failed: 'Connected, but repository sync failed. Refresh later.',
  github_app_not_configured: 'GitHub App is not configured on this install.',
  github_app_already_configured: 'A GitHub App is already configured for this install.',
  missing_manifest_code: 'GitHub App creation did not return a code. Try again.',
  manifest_exchange_failed: 'Could not finish GitHub App creation. Try again.',
  manifest_save_failed: 'Could not save GitHub App credentials. Try again.',
  manifest_forbidden: 'Only owners and admins can create the GitHub App.',
  manifest_client_failed: 'GitHub App was created but could not be loaded. Restart Farplane.',
  install_forbidden: 'You are not allowed to connect GitHub for this organization.',
  installation_owned: 'That GitHub installation belongs to another Farplane organization.',
  database_unavailable: 'The database is unavailable. Try again later.',
}

export function messageForOAuthError(code: string | undefined): string | null {
  if (!code) return null
  return oauthErrorMessages[code] ?? 'Google sign-in failed. Try again.'
}

export function messageForGitHubError(code: string | undefined): string | null {
  if (!code) return null
  return githubErrorMessages[code] ?? 'GitHub connect failed. Try again.'
}
