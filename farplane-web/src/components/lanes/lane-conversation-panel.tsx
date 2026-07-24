import { useState } from 'react'

import { Button } from '@/components/ui/button.tsx'
import { Input } from '@/components/ui/input.tsx'

type MockMessage = {
  id: string
  author: string
  body: string
  side: 'human' | 'agent'
}

const MOCK_MESSAGES: MockMessage[] = [
  {
    id: '1',
    author: 'Sergio',
    body: 'Can you redesign the homepage with a clearer hero and a single CTA?',
    side: 'human',
  },
  {
    id: '2',
    author: 'OpenCode',
    body: 'Updated the hero copy and CTA. Preview is live — take a look on the right.',
    side: 'agent',
  },
  {
    id: '3',
    author: 'Maya',
    body: 'Looks good — can we make the CTA say "Start free" instead?',
    side: 'human',
  },
]

type Props = {
  turnRunning: boolean
  canInterrupt: boolean
  onInterrupt: () => void
}

export function LaneConversationPanel({
  turnRunning,
  canInterrupt,
  onInterrupt,
}: Props) {
  const [text, setText] = useState('')

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border border-border bg-background">
      <div className="flex h-11 shrink-0 items-center justify-between border-b border-border bg-muted px-4">
        <h2 className="text-[13px] font-semibold text-foreground">
          Conversation
        </h2>
        <span className="text-xs text-muted-foreground">
          {MOCK_MESSAGES.length} messages
        </span>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-3.5 overflow-y-auto p-4">
        {MOCK_MESSAGES.map((message) => (
          <div
            key={message.id}
            className={
              message.side === 'human'
                ? 'flex w-full flex-col items-end gap-1'
                : 'flex w-full flex-col items-start gap-1'
            }
          >
            <span className="text-xs font-medium text-muted-foreground">
              {message.author}
            </span>
            <div
              className={
                message.side === 'human'
                  ? 'max-w-[280px] rounded-md bg-primary px-3 py-2.5 text-sm leading-5 text-primary-foreground'
                  : 'max-w-[280px] rounded-md border border-border bg-muted px-3 py-2.5 text-sm leading-5 text-foreground'
              }
            >
              {message.body}
            </div>
          </div>
        ))}
      </div>

      <form
        className="flex shrink-0 flex-col gap-2.5 border-t border-border p-3"
        onSubmit={(event) => {
          event.preventDefault()
          // Mock Send — does not post.
          setText('')
        }}
      >
        <Input
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder="Message the lane…"
          className="h-11"
        />
        <div className="flex items-center justify-end gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={!(turnRunning && canInterrupt)}
            onClick={onInterrupt}
          >
            Stop
          </Button>
          <Button type="submit" size="sm" disabled={!text.trim()}>
            Send
          </Button>
        </div>
      </form>
    </div>
  )
}
