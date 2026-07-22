import { Link, useMatchRoute } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import {
  NavigationMenu,
  NavigationMenuItem,
  NavigationMenuList,
  navigationMenuTriggerStyle,
} from '@/components/ui/navigation-menu'

interface NavItem {
  to: string
  label: string
  exact?: boolean
}

interface MainNavProps {
  items: NavItem[]
  className?: string
}

export function MainNav({ items, className }: MainNavProps) {
  const matchRoute = useMatchRoute()

  return (
    <NavigationMenu className={className}>
      <NavigationMenuList>
        {items.map((item) => {
          const isActive = Boolean(
            matchRoute({ to: item.to, fuzzy: !item.exact }),
          )

          return (
            <NavigationMenuItem key={item.to}>
              <Link
                to={item.to}
                className={cn(
                  navigationMenuTriggerStyle(),
                  isActive && 'bg-accent text-accent-foreground font-medium',
                )}
              >
                {item.label}
              </Link>
            </NavigationMenuItem>
          )
        })}
      </NavigationMenuList>
    </NavigationMenu>
  )
}
