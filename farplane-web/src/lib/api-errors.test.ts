import * as fc from 'fast-check'
import { describe, expect, it } from 'vitest'

import { errorMessage } from '@/lib/api-errors.ts'

describe('errorMessage', () => {
  it('prefers error over message', () => {
    expect(
      errorMessage(
        { error: 'from-error', message: 'from-message' },
        'fallback',
      ),
    ).toBe('from-error')
  })

  it('uses message when error is absent', () => {
    expect(errorMessage({ message: 'from-message' }, 'fallback')).toBe(
      'from-message',
    )
  })

  it('ignores empty error strings', () => {
    expect(errorMessage({ error: '' }, 'fallback')).toBe('fallback')
  })

  it('ignores empty message strings', () => {
    expect(errorMessage({ message: '' }, 'fallback')).toBe('fallback')
  })

  it('ignores non-string message values', () => {
    expect(errorMessage({ message: 0 }, 'fallback')).toBe('fallback')
    expect(errorMessage({ message: null }, 'fallback')).toBe('fallback')
  })

  it('returns fallback for non-objects', () => {
    expect(errorMessage(null, 'fallback')).toBe('fallback')
    expect(errorMessage('plain', 'fallback')).toBe('fallback')
    expect(errorMessage(42, 'fallback')).toBe('fallback')
  })

  it('never returns an empty string when fallback is non-empty (property)', () => {
    fc.assert(
      fc.property(
        fc.anything(),
        fc.string({ minLength: 1 }),
        (body, fallback) => {
          const result = errorMessage(body, fallback)
          expect(result.length).toBeGreaterThan(0)
        },
      ),
    )
  })
})
