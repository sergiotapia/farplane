import * as fc from 'fast-check'
import { describe, expect, it } from 'vitest'

import { canManageGitHubApp } from '@/lib/roles.ts'

describe('canManageGitHubApp', () => {
  it('allows owner and admin', () => {
    expect(canManageGitHubApp('owner')).toBe(true)
    expect(canManageGitHubApp('admin')).toBe(true)
  })

  it('denies member and unknown roles', () => {
    expect(canManageGitHubApp('member')).toBe(false)
    expect(canManageGitHubApp('viewer')).toBe(false)
    expect(canManageGitHubApp('')).toBe(false)
  })

  it('is true only for owner/admin (property)', () => {
    fc.assert(
      fc.property(fc.string(), (role) => {
        const allowed = role === 'owner' || role === 'admin'
        expect(canManageGitHubApp(role)).toBe(allowed)
      }),
    )
  })
})
