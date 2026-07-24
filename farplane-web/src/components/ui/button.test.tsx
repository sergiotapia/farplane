import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { Button } from '@/components/ui/button.tsx'

describe('Button', () => {
  it('renders children and handles clicks', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()

    render(<Button onClick={onClick}>Continue</Button>)

    const button = screen.getByRole('button', { name: 'Continue' })
    await user.click(button)
    expect(onClick).toHaveBeenCalledOnce()
  })
})
