import { CheckCircle2, Expand, Info } from 'lucide-react'
import { useState } from 'react'

import {
  MOCK_PREVIEW_PATH,
  MOCK_PREVIEW_URL,
  MockPreviewEmbed,
} from '@/components/lanes/mock-preview-embed.tsx'
import { Button } from '@/components/ui/button.tsx'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog.tsx'

export function LanePreviewPanel() {
  const [expandOpen, setExpandOpen] = useState(false)
  const [copied, setCopied] = useState(false)

  async function copyUrl() {
    try {
      await navigator.clipboard.writeText(MOCK_PREVIEW_URL)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // ignore clipboard failures in restricted contexts
    }
  }

  return (
    <>
      <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border border-border bg-background">
        <div className="shrink-0 border-b border-border bg-muted">
          <div className="flex h-[52px] items-center gap-2.5 px-3">
            <span className="shrink-0 text-xs font-semibold text-muted-foreground">
              Preview
            </span>
            <div className="flex h-8 min-w-0 grow items-center gap-2 rounded-md border border-border bg-background px-2.5">
              <CheckCircle2 className="size-3.5 shrink-0 text-[#16A34A]" />
              <span className="truncate text-xs text-foreground">
                {MOCK_PREVIEW_PATH}
              </span>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              data-icon="inline-start"
              onClick={() => setExpandOpen(true)}
            >
              <Expand />
              Expand
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() => void copyUrl()}
            >
              {copied ? 'Copied' : 'Copy'}
            </Button>
            <Button
              type="button"
              size="sm"
              className="shrink-0 bg-[#2563EB] text-white hover:bg-[#1D4ED8]"
              onClick={() =>
                window.open(MOCK_PREVIEW_URL, '_blank', 'noopener')
              }
            >
              Open
            </Button>
          </div>
          <div className="flex items-center gap-1.5 px-3 pb-2.5">
            <Info className="size-3 shrink-0 text-muted-foreground" />
            <p className="text-xs text-muted-foreground">
              Public preview · anyone with the link can open it · no sign-in
            </p>
          </div>
        </div>
        <MockPreviewEmbed />
      </div>

      <Dialog open={expandOpen} onOpenChange={setExpandOpen}>
        <DialogContent
          className="flex h-[min(820px,90vh)] w-[min(1120px,95vw)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
          showCloseButton={true}
        >
          <DialogHeader className="flex h-14 shrink-0 flex-row items-center gap-3 space-y-0 border-b border-border px-4 pr-12">
            <DialogTitle className="shrink-0 text-sm font-semibold">
              Preview
            </DialogTitle>
            <div className="flex h-8 min-w-0 grow items-center gap-2 rounded-md border border-border bg-muted/40 px-2.5">
              <CheckCircle2 className="size-3.5 shrink-0 text-[#16A34A]" />
              <span className="truncate text-xs text-foreground">
                {MOCK_PREVIEW_PATH}
              </span>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() => void copyUrl()}
            >
              {copied ? 'Copied' : 'Copy'}
            </Button>
            <Button
              type="button"
              size="sm"
              className="shrink-0 bg-[#2563EB] text-white hover:bg-[#1D4ED8]"
              onClick={() =>
                window.open(MOCK_PREVIEW_URL, '_blank', 'noopener')
              }
            >
              Open
            </Button>
          </DialogHeader>
          <div className="flex min-h-0 flex-1 flex-col">
            <MockPreviewEmbed />
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
