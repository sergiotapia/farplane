import { StreamLanguage } from '@codemirror/language'
import { dockerFile } from '@codemirror/legacy-modes/mode/dockerfile'
import { githubDark, githubLight } from '@uiw/codemirror-theme-github'
import CodeMirror from '@uiw/react-codemirror'
import { useEffect, useState } from 'react'

import { cn } from '@/lib/utils'

const dockerfileLanguage = StreamLanguage.define(dockerFile)

function useDocumentColorMode(): 'light' | 'dark' {
  const [mode, setMode] = useState<'light' | 'dark'>(() => {
    if (typeof document === 'undefined') return 'light'
    return document.documentElement.classList.contains('dark')
      ? 'dark'
      : 'light'
  })

  useEffect(() => {
    const root = document.documentElement
    const sync = () => {
      setMode(root.classList.contains('dark') ? 'dark' : 'light')
    }
    sync()
    const observer = new MutationObserver(sync)
    observer.observe(root, { attributes: true, attributeFilter: ['class'] })
    return () => observer.disconnect()
  }, [])

  return mode
}

type DockerfileEditorProps = {
  id?: string
  value: string
  onChange: (value: string) => void
  className?: string
}

export function DockerfileEditor({
  id,
  value,
  onChange,
  className,
}: DockerfileEditorProps) {
  const colorMode = useDocumentColorMode()

  return (
    <div
      id={id}
      className={cn(
        'overflow-hidden rounded-lg border border-input',
        'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50',
        className,
      )}
    >
      <CodeMirror
        value={value}
        height="320px"
        theme={colorMode === 'dark' ? githubDark : githubLight}
        extensions={[dockerfileLanguage]}
        basicSetup={{
          foldGutter: false,
          dropCursor: false,
          allowMultipleSelections: false,
          autocompletion: false,
        }}
        onChange={onChange}
        className="text-xs [&_.cm-scroller]:font-mono [&_.cm-scroller]:text-xs"
      />
    </div>
  )
}
