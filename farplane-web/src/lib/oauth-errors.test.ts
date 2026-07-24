import * as fc from 'fast-check'
import { describe, expect, it } from 'vitest'

import {
  messageForGitHubError,
  messageForOAuthError,
} from '@/lib/oauth-errors.ts'

const oauthCases: Array<[string, string]> = [
  ['google_denied', 'Google sign-in was cancelled.'],
  ['invalid_state', 'Google sign-in expired. Try again.'],
  ['missing_code', 'Google sign-in failed. Try again.'],
  ['token_exchange_failed', 'Google sign-in failed. Try again.'],
  ['userinfo_failed', 'Could not read your Google profile.'],
  ['incomplete_profile', 'Your Google account needs a verified email.'],
  ['account_not_found', 'No Farplane account is linked to that Google login.'],
  ['login_failed', 'Google sign-in failed. Try again.'],
  ['session_failed', 'Could not create a session. Try again.'],
  [
    'setup_already_completed',
    'This install is already set up. Sign in instead.',
  ],
  ['setup_failed', 'Google setup failed. Try again.'],
  [
    'google_oauth_not_configured',
    'Google sign-in is not configured on this install.',
  ],
  ['database_unavailable', 'The database is unavailable. Try again later.'],
  ['invalid_intent', 'Google sign-in failed. Try again.'],
  ['invite_unavailable', 'This Lane invite is no longer available.'],
  ['invite_failed', 'Could not accept the Lane invite with Google.'],
  ['invalid_invite', 'This Lane invite link is invalid.'],
  ['invite_email_mismatch', 'Could not accept the Lane invite with Google.'],
]

const githubCases: Array<[string, string]> = [
  ['github_denied', 'GitHub install was cancelled.'],
  ['invalid_state', 'GitHub install expired. Try again.'],
  ['missing_installation', 'GitHub did not return an installation. Try again.'],
  ['installation_lookup_failed', 'Could not read the GitHub installation.'],
  ['install_save_failed', 'Could not save the GitHub connection.'],
  ['repo_sync_failed', 'Connected, but repository sync failed. Refresh later.'],
  [
    'github_app_not_configured',
    'GitHub App is not configured on this install.',
  ],
  [
    'github_app_already_configured',
    'A GitHub App is already configured for this install.',
  ],
  [
    'missing_manifest_code',
    'GitHub App creation did not return a code. Try again.',
  ],
  [
    'manifest_exchange_failed',
    'Could not finish GitHub App creation. Try again.',
  ],
  ['manifest_save_failed', 'Could not save GitHub App credentials. Try again.'],
  ['manifest_forbidden', 'Only owners and admins can create the GitHub App.'],
  [
    'manifest_client_failed',
    'GitHub App was created but could not be loaded. Restart Farplane.',
  ],
  [
    'install_forbidden',
    'You are not allowed to connect GitHub for this organization.',
  ],
  [
    'installation_owned',
    'That GitHub installation belongs to another Farplane organization.',
  ],
  ['database_unavailable', 'The database is unavailable. Try again later.'],
]

describe('messageForOAuthError', () => {
  it('returns null for missing codes', () => {
    expect(messageForOAuthError(undefined)).toBeNull()
  })

  it.each(oauthCases)('maps %s', (code, message) => {
    expect(messageForOAuthError(code)).toBe(message)
  })

  it('falls back for unknown codes', () => {
    expect(messageForOAuthError('not_a_real_code')).toBe(
      'Google sign-in failed. Try again.',
    )
  })

  it('always returns a non-empty string for any non-empty code (property)', () => {
    fc.assert(
      fc.property(fc.string({ minLength: 1 }), (code) => {
        const message = messageForOAuthError(code)
        expect(message).toBeTypeOf('string')
        expect(message?.length).toBeGreaterThan(0)
      }),
    )
  })
})

describe('messageForGitHubError', () => {
  it('returns null for missing codes', () => {
    expect(messageForGitHubError(undefined)).toBeNull()
  })

  it.each(githubCases)('maps %s', (code, message) => {
    expect(messageForGitHubError(code)).toBe(message)
  })

  it('falls back for unknown codes', () => {
    expect(messageForGitHubError('weird')).toBe(
      'GitHub connect failed. Try again.',
    )
  })

  it('always returns a non-empty string for any non-empty code (property)', () => {
    fc.assert(
      fc.property(fc.string({ minLength: 1 }), (code) => {
        const message = messageForGitHubError(code)
        expect(message).toBeTypeOf('string')
        expect(message?.length).toBeGreaterThan(0)
      }),
    )
  })
})
