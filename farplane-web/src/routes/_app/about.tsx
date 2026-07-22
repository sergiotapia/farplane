import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_app/about')({
  component: AboutPage,
})

function AboutPage() {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold tracking-tight">About</h1>
      <p className="text-muted-foreground text-sm">
        Demo route inside the app layout.
      </p>
    </div>
  )
}
