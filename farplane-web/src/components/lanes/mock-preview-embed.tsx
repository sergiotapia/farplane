/** Static Northwind homepage mock used in the preview panel and expand dialog. */
export function MockPreviewEmbed() {
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-auto bg-[#FAFAFA]">
      <div className="flex h-14 shrink-0 items-center justify-between border-b border-[#EEEEEE] bg-background px-8">
        <div className="text-base font-semibold text-foreground">Northwind</div>
        <div className="flex items-center gap-5">
          <span className="text-[13px] text-[#525252]">Product</span>
          <span className="text-[13px] text-[#525252]">Pricing</span>
          <span className="text-[13px] text-[#525252]">About</span>
          <span className="inline-flex h-8 items-center rounded-md bg-foreground px-3 text-[13px] font-medium text-background">
            Start free
          </span>
        </div>
      </div>
      <div className="flex flex-col items-start gap-5 px-8 pt-16 pb-10">
        <h2 className="max-w-xl text-[40px] leading-[48px] font-semibold tracking-[-0.03em] text-foreground">
          Ship work that looks finished.
        </h2>
        <p className="max-w-[420px] text-base leading-6 text-[#525252]">
          Northwind helps small teams turn rough ideas into a live site —
          without a file tree in sight.
        </p>
        <div className="flex items-center gap-3">
          <span className="inline-flex h-10 items-center rounded-lg bg-foreground px-[18px] text-sm font-medium text-background">
            Start free
          </span>
          <span className="inline-flex h-10 items-center rounded-lg border border-border bg-background px-[18px] text-sm font-medium text-foreground">
            See how it works
          </span>
        </div>
      </div>
    </div>
  )
}

export const MOCK_PREVIEW_PATH =
  'farplane.app/p/a7f3c91e-4b2d-4e8a-9c1f-6d0e5b8a2f14'

export const MOCK_PREVIEW_URL = `https://${MOCK_PREVIEW_PATH}`
