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

export function messageForOAuthError(code: string | undefined): string | null {
  if (!code) return null
  return oauthErrorMessages[code] ?? 'Google sign-in failed. Try again.'
}
