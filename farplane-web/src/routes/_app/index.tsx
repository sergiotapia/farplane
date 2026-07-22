import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_app/')({
  component: HomePage,
})

function HomePage() {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold tracking-tight">Home</h1>
      <p className="text-muted-foreground text-sm">
        Your Farplane install is ready.
      </p>
    </div>
  )
}
