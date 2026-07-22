import { createFileRoute } from '@tanstack/react-router'
import { MainNav } from '@/components/navigation/main-nav'
import { Button } from '@/components/ui/button'
import { RouterButton } from '@/components/ui/router-button'
import { RouterDialog } from '@/components/ui/router-dialog'
import { RouterSheet } from '@/components/ui/router-sheet'

export const Route = createFileRoute('/')({ component: Home })

const navItems = [
  { to: '/', label: 'Home', exact: true },
  { to: '/about', label: 'About' },
]

function Home() {
  return (
    <div className="container mx-auto space-y-8 p-8">
      <MainNav items={navItems} />

      <div className="space-y-2">
        <h1 className="text-4xl font-bold tracking-tight">Farplane</h1>
        <p className="text-muted-foreground text-lg">
          shadcn/ui is wired up with TanStack Router.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <Button>Primary</Button>
        <Button variant="outline">Outline</Button>
        <Button variant="secondary">Secondary</Button>
        <Button variant="ghost">Ghost</Button>

        <RouterButton to="/about" variant="default">
          Go to About
        </RouterButton>

        <RouterSheet
          trigger={<Button variant="outline">Open sheet</Button>}
          title="Navigation"
          description="Sheet overlays animate correctly with the router."
        >
          <p className="text-muted-foreground text-sm">
            Use this pattern for mobile menus and side panels.
          </p>
          <RouterButton to="/about" variant="default" className="mt-4 w-full">
            About page
          </RouterButton>
        </RouterSheet>

        <RouterDialog
          trigger={<Button variant="outline">Open dialog</Button>}
          title="Confirm"
          description="Dialog overlays animate correctly with the router."
        >
          <p className="text-muted-foreground text-sm">
            Controlled dialogs keep animations stable across route changes.
          </p>
        </RouterDialog>
      </div>
    </div>
  )
}
