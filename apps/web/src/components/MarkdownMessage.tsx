'use client'

import { useState, useEffect, useRef } from 'react'
import { Check, Copy } from 'lucide-react'

interface Props {
  content: string
  isStreaming?: boolean
}

interface Block {
  type: 'code' | 'text'
  lang?: string
  value: string
}

function parseBlocks(text: string): Block[] {
  const blocks: Block[] = []
  const fence = /^```(\w*)\n?([\s\S]*?)```$/gm
  let last = 0
  let m: RegExpExecArray | null

  while ((m = fence.exec(text)) !== null) {
    if (m.index > last) {
      blocks.push({ type: 'text', value: text.slice(last, m.index) })
    }
    blocks.push({ type: 'code', lang: m[1] || '', value: m[2].replace(/\n$/, '') })
    last = m.index + m[0].length
  }
  if (last < text.length) {
    blocks.push({ type: 'text', value: text.slice(last) })
  }
  return blocks
}

// Split text by inline patterns: `code`, **bold**, *italic*, [link](url)
const INLINE = /(`[^`\n]+`|\*\*[^*\n]+\*\*|\*[^*\n]+\*|\[([^\]\n]+)\]\(([^)\n]+)\))/g

function renderInline(text: string, baseKey: string) {
  const nodes: React.ReactNode[] = []
  let last = 0
  let m: RegExpExecArray | null
  let i = 0
  INLINE.lastIndex = 0

  while ((m = INLINE.exec(text)) !== null) {
    if (m.index > last) {
      nodes.push(text.slice(last, m.index))
    }
    const full = m[0]
    const key = `${baseKey}-${i++}`
    if (full.startsWith('`')) {
      nodes.push(
        <code key={key} className="bg-muted rounded px-1 py-0.5 font-mono text-xs text-crit dark:text-crit whitespace-nowrap">
          {full.slice(1, -1)}
        </code>
      )
    } else if (full.startsWith('**')) {
      nodes.push(<strong key={key}>{full.slice(2, -2)}</strong>)
    } else if (full.startsWith('*')) {
      nodes.push(<em key={key}>{full.slice(1, -1)}</em>)
    } else if (m[2] && m[3]) {
      nodes.push(
        <a key={key} href={m[3]} target="_blank" rel="noopener noreferrer" className="text-accent dark:text-accent-bright underline hover:text-accent dark:text-accent-bright">
          {m[2]}
        </a>
      )
    }
    last = m.index + full.length
  }
  if (last < text.length) {
    nodes.push(text.slice(last))
  }
  return nodes
}

function renderTextBlock(text: string): React.ReactNode[] {
  const lines = text.split('\n')
  const elements: React.ReactNode[] = []
  let i = 0

  while (i < lines.length) {
    const line = lines[i]

    // ATX headers
    const hMatch = line.match(/^(#{1,3})\s+(.+)/)
    if (hMatch) {
      const level = hMatch[1].length
      const cls = level === 1
        ? 'text-lg font-semibold mt-3 mb-1'
        : level === 2
        ? 'text-base font-semibold mt-2 mb-1'
        : 'text-sm font-semibold mt-2 mb-0.5'
      const Tag = (`h${level}`) as 'h1' | 'h2' | 'h3'
      elements.push(<Tag key={i} className={cls}>{renderInline(hMatch[2], `h-${i}`)}</Tag>)
      i++
      continue
    }

    // Unordered list — collect consecutive items
    if (/^[-*+]\s+/.test(line)) {
      const items: React.ReactNode[] = []
      while (i < lines.length && /^[-*+]\s+/.test(lines[i])) {
        items.push(
          <li key={i} className="ml-5 list-disc">
            {renderInline(lines[i].replace(/^[-*+]\s+/, ''), `ul-${i}`)}
          </li>
        )
        i++
      }
      elements.push(<ul key={`ul-${i}`} className="my-1 space-y-0.5">{items}</ul>)
      continue
    }

    // Ordered list — collect consecutive items
    if (/^\d+\.\s+/.test(line)) {
      const items: React.ReactNode[] = []
      while (i < lines.length && /^\d+\.\s+/.test(lines[i])) {
        items.push(
          <li key={i} className="ml-5 list-decimal">
            {renderInline(lines[i].replace(/^\d+\.\s+/, ''), `ol-${i}`)}
          </li>
        )
        i++
      }
      elements.push(<ol key={`ol-${i}`} className="my-1 space-y-0.5">{items}</ol>)
      continue
    }

    // Horizontal rule
    if (/^---+$/.test(line.trim())) {
      elements.push(<hr key={i} className="my-3 border-border-strong" />)
      i++
      continue
    }

    // Empty line — small gap
    if (line.trim() === '') {
      elements.push(<div key={i} className="h-2" />)
      i++
      continue
    }

    // Regular paragraph
    elements.push(
      <p key={i} className="leading-relaxed">
        {renderInline(line, `p-${i}`)}
      </p>
    )
    i++
  }

  return elements
}

function CodeBlock({ lang, code }: { lang: string; code: string }) {
  const [copied, setCopied] = useState(false)
  const codeRef = useRef<HTMLElement>(null)

  useEffect(() => {
    // Lazy-load highlight.js only in the browser to avoid SSR issues
    if (!codeRef.current) return
    import('highlight.js').then(({ default: hljs }) => {
      if (!codeRef.current) return
      if (lang && hljs.getLanguage(lang)) {
        codeRef.current.innerHTML = hljs.highlight(code, { language: lang }).value
      } else {
        codeRef.current.innerHTML = hljs.highlightAuto(code).value
      }
    })
  }, [code, lang])

  const copy = async () => {
    await navigator.clipboard.writeText(code)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="my-3 rounded-lg overflow-hidden border border-border-strong text-sm">
      <div className="flex items-center justify-between px-4 py-1.5 bg-gray-800 text-xs text-faint">
        <span className="font-mono">{lang || 'code'}</span>
        <button onClick={copy} className="flex items-center gap-1 hover:text-white transition-colors">
          {copied ? <Check size={11} /> : <Copy size={11} />}
          <span>{copied ? 'Copied' : 'Copy'}</span>
        </button>
      </div>
      <pre className="overflow-x-auto bg-gray-900 p-4 leading-relaxed m-0">
        <code ref={codeRef} className={`hljs language-${lang || 'plaintext'} font-mono text-foreground`}>
          {code}
        </code>
      </pre>
    </div>
  )
}

export function MarkdownMessage({ content, isStreaming }: Props) {
  // During streaming, render as plain pre-wrap text to avoid layout thrashing
  if (isStreaming) {
    return (
      <p className="whitespace-pre-wrap leading-relaxed text-sm">{content}</p>
    )
  }

  const blocks = parseBlocks(content)

  return (
    <div className="text-sm space-y-0.5 min-w-0">
      {blocks.map((block, idx) =>
        block.type === 'code' ? (
          <CodeBlock key={idx} lang={block.lang ?? ''} code={block.value} />
        ) : (
          <div key={idx}>{renderTextBlock(block.value)}</div>
        )
      )}
    </div>
  )
}
