import { createFileRoute, redirect } from '@tanstack/react-router'

// Legacy accept URL → public invite landing (handles signup + accept).
export const Route = createFileRoute('/_app/lane-invites/$token/accept')({
  beforeLoad: ({ params }) => {
    throw redirect({
      to: '/lane-invites/$token',
      params: { token: params.token },
    })
  },
  component: () => null,
})
