import * as React from 'react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'

interface RouterSheetProps {
  children: React.ReactNode
  trigger: React.ReactNode
  title: string
  description?: string
  onOpenChange?: (open: boolean) => void
}

export function RouterSheet({
  children,
  trigger,
  title,
  description,
  onOpenChange,
}: RouterSheetProps) {
  const [open, setOpen] = React.useState(false)

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen)
    onOpenChange?.(newOpen)
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetTrigger render={trigger as React.ReactElement} />
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          {description ? <SheetDescription>{description}</SheetDescription> : null}
        </SheetHeader>
        <div className="mt-4 px-4">{children}</div>
      </SheetContent>
    </Sheet>
  )
}
