import { setupServer } from 'msw/node'

import { handlers } from '@/test/msw/handlers.ts'

export const server = setupServer(...handlers)
