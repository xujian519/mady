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

import React, { useEffect, useState, useMemo } from 'react'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { CodeBlock } from './CodeBlock'
import { readFile } from '@/lib/backend'

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

/**
 * 图片节点（粘贴图片 / 项目附件）。
 * - http(s) 外链：直接 <img>（依赖 CSP 白名单）
 * - data: URL：直接 <img>
 * - 相对路径（attachments/ 等项目内附件）：经 ReadFile 读 base64 渲染
 */
const ImageNode: React.FC<{ src: string; alt: string }> = ({ src, alt }) => {
  const isRemote = /^(https?:|data:)/i.test(src)
  if (isRemote) {
    return (
      <img
        src={src}
        alt={alt}
        className="max-w-full max-h-96 rounded-lg border border-mady-border my-1"
      />
    )
  }
  return <ProjectImage src={src} alt={alt} />
}

/** 项目内相对路径图片：经后端 ReadFile 读取。 */
const ProjectImage: React.FC<{ src: string; alt: string }> = ({ src, alt }) => {
  const [dataUrl, setDataUrl] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    setFailed(false)
    setDataUrl(null)
    readFile(src)
      .then((fc) => {
        if (cancelled) return
        if (fc.data && fc.mime) setDataUrl(`data:${fc.mime};base64,${fc.data}`)
        else setFailed(true)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [src])

  if (failed) {
    return (
      <span className="text-mady-caption text-mady-text-tertiary">[图片加载失败: {alt || src}]</span>
    )
  }
  if (!dataUrl) {
    return <span className="text-mady-caption text-mady-text-tertiary">图片加载中…</span>
  }
  return (
    <img
      src={dataUrl}
      alt={alt}
      className="max-w-full max-h-96 rounded-lg border border-mady-border my-1"
    />
  )
}

/** 解析粗体、斜体、行内代码、链接、图片。 */
function parseInline(text: string): React.ReactNode[] {
  const parts: React.ReactNode[] = []
  // 顺序：图片（![]()）→ 行内代码 → 粗斜体 → 粗体 → 斜体 → 链接
  const regex = /!\[([^\]]*)\]\(([^)]+)\)|(`+)(.+?)\3|(\*\*\*|___)(.+?)\5|(\*\*|__)(.+?)\7|(\*|_)(.+?)\9|\[([^\]]+)\]\(([^)]+)\)/g
  let last = 0
  let match: RegExpExecArray | null

  while ((match = regex.exec(text)) !== null) {
    if (match.index > last) {
      parts.push(text.slice(last, match.index))
    }

    if (match[1] !== undefined) {
      // 图片：![alt](src)
      parts.push(<ImageNode key={match.index} src={match[2]} alt={match[1]} />)
    } else if (match[4] !== undefined) {
      // 行内代码
      parts.push(<code key={match.index} className="bg-mady-bg-tertiary px-1 rounded text-mady-small font-mono">{match[4]}</code>)
    } else if (match[6] !== undefined) {
      // 粗斜体
      parts.push(<em key={match.index} className="font-bold italic">{match[6]}</em>)
    } else if (match[8] !== undefined) {
      // 粗体
      parts.push(<strong key={match.index} className="font-semibold">{match[8]}</strong>)
    } else if (match[10] !== undefined) {
      // 斜体
      parts.push(<em key={match.index} className="italic">{match[10]}</em>)
    } else if (match[12] !== undefined) {
      // 链接（F-I11）：Wails WebView 不支持 _blank 新窗口，
      // 改为点击时经 BrowserOpenURL 交给系统浏览器；仅放行 http/https（M-DSK-SEC-003）。
      const href = match[13]
      parts.push(
        <a
          key={match.index}
          href={href}
          onClick={(e) => {
            if (/^https?:\/\//i.test(href)) {
              e.preventDefault()
              BrowserOpenURL(href)
            }
            // 非 http(s)（如 javascript:）不打开，依赖 CSP 兜底 + 白名单拦截
          }}
          className="text-mady-text-link underline hover:opacity-80"
        >
          {match[12]}
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
        <CodeBlock
          key={`code-${elements.length}`}
          code={codeLines.join('\n')}
          language={codeLang || undefined}
          showLineNumbers={codeLines.length > 5}
        />
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
