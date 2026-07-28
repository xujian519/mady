/**
 * CodeBlock — 代码块组件。
 *
 * 特性：
 * - 语法高亮（基础关键词/字符串/注释/数字）
 * - 语言标签显示
 * - 行号（可选）
 * - 复制按钮（带成功反馈）
 * - 最大高度限制 + 内部滚动（可选）
 *
 * Seam 设计：本组件通过稳定 Props 接口与消费方解耦。
 * 未来可替换为 highlight.js / CodeMirror / Monaco 实现。
 */

import React, { useState, useCallback } from 'react'
import { Copy, Check } from 'lucide-react'

// ── Props 接口（稳定的 Seam） ──────────────────────

export interface CodeBlockProps {
  /** 源代码。 */
  code: string
  /** 编程语言（用于语法高亮）。 */
  language?: string
  /** 是否显示行号。 */
  showLineNumbers?: boolean
  /** 最大高度（px），超出后内部滚动。 */
  maxHeight?: number
}

// ── 语法高亮 ──────────────────────────────────────

/** 一个高亮片段的类型。 */
type HighlightToken = { text: string; className: string }

/** 简单的语法高亮 Tokenizer。 */
function tokenize(code: string, _lang: string): HighlightToken[][] {
  const lines = code.split('\n')
  return lines.map((line) => {
    const tokens: HighlightToken[] = []
    let remaining = line

    while (remaining.length > 0) {
      // 行注释（// 或 #）
      const commentMatch = remaining.match(/^(\/\/|#).*/)
      if (commentMatch) {
        tokens.push({ text: remaining, className: 'text-mady-text-tertiary italic' })
        remaining = ''
        continue
      }

      // 字符串（双引号、单引号、反引号）
      const strMatch = remaining.match(/^("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)/)
      if (strMatch) {
        tokens.push({ text: strMatch[1], className: 'text-mady-warning' })
        remaining = remaining.slice(strMatch[1].length)
        continue
      }

      // 数字
      const numMatch = remaining.match(/^(\b\d+(?:\.\d+)?\b)/)
      if (numMatch) {
        tokens.push({ text: numMatch[1], className: 'text-mady-info' })
        remaining = remaining.slice(numMatch[1].length)
        continue
      }

      // 关键词
      const keywords = [
        'function', 'if', 'else', 'for', 'while', 'return', 'import',
        'export', 'const', 'let', 'var', 'class', 'interface', 'type',
        'extends', 'implements', 'new', 'this', 'async', 'await',
        'try', 'catch', 'finally', 'throw', 'switch', 'case', 'default',
        'break', 'continue', 'def', 'if', 'elif', 'else', 'for', 'in',
        'while', 'return', 'import', 'from', 'class', 'try', 'except',
        'with', 'as', 'pass', 'None', 'True', 'False', 'and', 'or', 'not',
        'package', 'func', 'go', 'defer', 'select', 'chan', 'map',
        'struct', 'nil', 'true', 'false', 'var', 'const', 'type',
        'if', 'else', 'for', 'range', 'switch', 'case', 'default',
        'break', 'continue', 'return', 'go', 'defer',
        'fn', 'let', 'mut', 'if', 'else', 'for', 'in', 'while',
        'match', 'return', 'pub', 'struct', 'enum', 'impl', 'trait',
        'use', 'mod', 'self', 'super', 'true', 'false',
      ]
      const kwMatch = remaining.match(new RegExp(`^\\b(${keywords.join('|')})\\b`))
      if (kwMatch) {
        tokens.push({ text: kwMatch[1], className: 'text-mady-accent font-medium' })
        remaining = remaining.slice(kwMatch[1].length)
        continue
      }

      // 一个普通字符
      tokens.push({ text: remaining[0], className: '' })
      remaining = remaining.slice(1)
    }

    return tokens
  })
}

// ── 组件 ──────────────────────────────────────────

export const CodeBlock: React.FC<CodeBlockProps> = ({
  code,
  language = '',
  showLineNumbers = false,
  maxHeight,
}) => {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // 剪贴板不可用时忽略
    }
  }, [code])

  const tokens = React.useMemo(() => tokenize(code, language), [code, language])

  return (
    <div className="group relative my-2 rounded-lg border border-mady-border bg-mady-bg-tertiary overflow-hidden">
      {/* 头部: 语言标签 + 复制按钮 */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-mady-border bg-mady-bg-secondary/50">
        <span className="text-mady-caption text-mady-text-tertiary font-mono">
          {language || 'code'}
        </span>
        <button
          onClick={handleCopy}
          className="flex items-center gap-1 text-mady-caption text-mady-text-tertiary hover:text-mady-text-primary transition-colors opacity-0 group-hover:opacity-100"
          title="复制代码"
        >
          {copied ? (
            <>
              <Check size={12} className="text-mady-success" />
              <span className="text-mady-success">已复制</span>
            </>
          ) : (
            <>
              <Copy size={12} />
              <span>复制</span>
            </>
          )}
        </button>
      </div>

      {/* 代码体 */}
      <div
        className="overflow-x-auto"
        style={maxHeight ? { maxHeight, overflowY: 'auto' } : undefined}
      >
        <pre className="p-3 text-mady-small font-mono leading-relaxed">
          {tokens.map((line, i) => (
            <div key={i} className="flex">
              {/* 行号 */}
              {showLineNumbers && (
                <span className="select-none text-mady-text-tertiary text-right pr-3 min-w-[2.5em] shrink-0">
                  {i + 1}
                </span>
              )}
              {/* 代码内容 */}
              <code className="whitespace-pre">
                {line.length === 0 ? (
                  <span>&nbsp;</span>
                ) : (
                  line.map((token, j) => (
                    <span key={j} className={token.className}>
                      {token.text}
                    </span>
                  ))
                )}
              </code>
            </div>
          ))}
        </pre>
      </div>
    </div>
  )
}
