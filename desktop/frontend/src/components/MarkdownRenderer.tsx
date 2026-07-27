/**
 * MarkdownRenderer — 轻量级 Markdown → React 渲染器。
 *
 * 不使用外部 Markdown 库，采用按需支持的标记子集：
 * - 段落 / 换行
 * - 标题 (h1-h4)
 * - 粗体 / 斜体 / 行内代码
 * - 代码块 (含语言标签)
 * - 无序/有序列表
 * - 链接
 * - 引用块
 * - 水平分割线
 *
 * 已知限制（不在本版本支持）：
 * - 不支持嵌套列表（`- item\n  - subitem` 会被展平）
 * - 不支持表格
 * - 不支持 HTML 标签
 *
 * goldmark 兼容：以上子集与 goldmark 默认渲染行为一致。
 * 超出此范围的标记保持原样输出。
 */

import React, { useMemo } from 'react'

interface MarkdownRendererProps {
  content: string
  className?: string
}

export const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({
  content,
  className = '',
}) => {
  const elements = useMemo(() => renderMarkdown(content), [content])

  return (
    <div className={`space-y-2 ${className}`}>
      {elements.map((el, i) => (
        <React.Fragment key={i}>{el}</React.Fragment>
      ))}
    </div>
  )
}

// ── 行内格式化 ────────────────────────────────────

/** 解析粗体、斜体、行内代码、链接。 */
function parseInline(text: string): React.ReactNode[] {
  const parts: React.ReactNode[] = []
  const regex = /(`+)(.+?)\1|(\*\*\*|___)(.+?)\3|(\*\*|__)(.+?)\5|(\*|_)(.+?)\7|\[([^\]]+)\]\(([^)]+)\)/g
  let last = 0
  let match: RegExpExecArray | null

  while ((match = regex.exec(text)) !== null) {
    if (match.index > last) {
      parts.push(text.slice(last, match.index))
    }

    if (match[2] !== undefined) {
      // 行内代码
      parts.push(<code key={match.index} className="bg-mady-bg-tertiary px-1 rounded text-mady-small font-mono">{match[2]}</code>)
    } else if (match[4] !== undefined) {
      // 粗斜体
      parts.push(<em key={match.index} className="font-bold italic">{match[4]}</em>)
    } else if (match[6] !== undefined) {
      // 粗体
      parts.push(<strong key={match.index} className="font-semibold">{match[6]}</strong>)
    } else if (match[8] !== undefined) {
      // 斜体
      parts.push(<em key={match.index} className="italic">{match[8]}</em>)
    } else if (match[10] !== undefined) {
      // 链接
      parts.push(
        <a
          key={match.index}
          href={match[11]}
          target="_blank"
          rel="noopener noreferrer"
          className="text-mady-text-link underline hover:opacity-80"
        >
          {match[10]}
        </a>
      )
    }

    last = match.index + match[0].length
  }

  if (last < text.length) {
    parts.push(text.slice(last))
  }

  return parts.length > 0 ? parts : [text]
}

// ── 块级解析 ────────────────────────────────────

function renderMarkdown(content: string): React.ReactNode[] {
  const lines = content.split('\n')
  const elements: React.ReactNode[] = []
  let inCodeBlock = false
  let codeLang = ''
  let codeLines: string[] = []
  let inList: 'ol' | 'ul' | null = null
  let listItems: React.ReactNode[] = []
  let listItemIndex = 0

  function flushCodeBlock() {
    if (codeLines.length > 0) {
      elements.push(
        <pre key={`code-${elements.length}`} className="bg-mady-bg-tertiary rounded-lg p-3 overflow-x-auto text-mady-small font-mono">
          {codeLang && (
            <div className="text-mady-text-tertiary text-mady-caption mb-1">{codeLang}</div>
          )}
          <code>{codeLines.join('\n')}</code>
        </pre>
      )
      codeLines = []
      codeLang = ''
    }
    inCodeBlock = false
  }

  function flushList() {
    if (inList && listItems.length > 0) {
      const Tag = inList === 'ol' ? 'ol' : 'ul'
      elements.push(
        <Tag key={`list-${elements.length}`} className={`list-inside space-y-1 ${inList === 'ol' ? 'list-decimal' : 'list-disc'}`}>
          {listItems}
        </Tag>
      )
      listItems = []
      inList = null
      listItemIndex = 0
    }
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // 代码块围栏
    if (line.startsWith('```')) {
      if (inCodeBlock) {
        flushCodeBlock()
      } else {
        flushList()
        inCodeBlock = true
        codeLang = line.slice(3).trim()
      }
      continue
    }

    if (inCodeBlock) {
      codeLines.push(line)
      continue
    }

    const trimmed = line.trim()

    // 空行
    if (!trimmed) {
      flushList()
      continue
    }

    // 标题
    const heading = trimmed.match(/^(#{1,4})\s+(.+)$/)
    if (heading) {
      flushList()
      const level = heading[1].length as 1 | 2 | 3 | 4
      const Tag = `h${level}` as 'h1' | 'h2' | 'h3' | 'h4'
      const sizeClass = ['text-mady-h1', 'text-mady-heading', 'text-mady-body font-semibold', 'text-mady-ui font-semibold'][level - 1]
      elements.push(
        <Tag key={`h-${i}`} className={`${sizeClass} mt-3 mb-1`}>
          {parseInline(heading[2])}
        </Tag>
      )
      continue
    }

    // 分割线
    if (/^[-*_]{3,}$/.test(trimmed)) {
      flushList()
      elements.push(<hr key={`hr-${i}`} className="border-mady-separator my-2" />)
      continue
    }

    // 引用块
    if (trimmed.startsWith('> ')) {
      flushList()
      elements.push(
        <blockquote key={`bq-${i}`} className="border-l-2 border-mady-accent pl-3 text-mady-text-secondary italic">
          {parseInline(trimmed.slice(2))}
        </blockquote>
      )
      continue
    }

    // 无序列表
    const ulMatch = trimmed.match(/^[-*+]\s+(.+)$/)
    if (ulMatch) {
      if (inList !== 'ul') flushList()
      inList = 'ul'
      listItems.push(<li key={`li-${listItemIndex++}`}>{parseInline(ulMatch[1])}</li>)
      continue
    }

    // 有序列表
    const olMatch = trimmed.match(/^\d+[.)]\s+(.+)$/)
    if (olMatch) {
      if (inList !== 'ol') flushList()
      inList = 'ol'
      listItems.push(<li key={`li-${listItemIndex++}`}>{parseInline(olMatch[1])}</li>)
      continue
    }

    // 普通段落
    flushList()
    // 合并连续段（非空行分隔）
    let paragraph = trimmed
    let j = i + 1
    while (j < lines.length && lines[j].trim().length > 0) {
      paragraph += ' ' + lines[j].trim()
      j++
    }
    if (j > i + 1) i = j - 1

    elements.push(
      <p key={`p-${i}`} className="text-mady-body leading-relaxed">
        {parseInline(paragraph)}
      </p>
    )
  }

  flushCodeBlock()
  flushList()

  return elements
}
