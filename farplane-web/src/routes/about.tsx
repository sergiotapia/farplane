import { createFileRoute } from '@tanstack/react-router'
import { MainNav } from '@/components/navigation/main-nav'
import { RouterButton } from '@/components/ui/router-button'

export const Route = createFileRoute('/about')({ component: About })

const navItems = [
  { to: '/', label: 'Home', exact: true },
  { to: '/about', label: 'About' },
]

function About() {
  return (
    <div className="container mx-auto space-y-8 p-8">
      <MainNav items={navItems} />

      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight">About</h1>
        <p className="text-muted-foreground">
          Demo route for typed navigation with shadcn components.
        </p>
      </div>

      <RouterButton to="/" variant="outline">
        Back home
      </RouterButton>
    </div>
  )
}
