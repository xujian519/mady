/**
 * sessionExport — 会话导出工具。
 *
 * 支持格式：
 * - Markdown: 对话记录 + 代码块 + 时间戳
 * - JSON: 结构化数据供后续处理
 */

import type { Message, ToolCall } from '@/stores/chat'

// ── Types ─────────────────────────────────────────

export type ExportFormat = 'markdown' | 'json'

export interface ExportOptions {
  format: ExportFormat
  title?: string
  includeTimestamps?: boolean
  includeToolCalls?: boolean
}

// ── Markdown 导出 ─────────────────────────────────

function messageRoleLabel(role: string): string {
  return role === 'user' ? '用户' : 'Mady'
}

function formatTimestamp(ts: number): string {
  const d = new Date(ts)
  return d.toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

/** 将单条消息导出为 Markdown 片段。 */
function messageToMarkdown(
  msg: Message,
  includeTimestamp: boolean,
  _toolCalls: ToolCall[],
): string {
  const role = messageRoleLabel(msg.role)
  const ts = includeTimestamp ? ` *(${formatTimestamp(msg.timestamp)})*` : ''
  let content = msg.content

  // 如果内容包含 thinking-fold，提取摘要
  const thinkingMatch = content.match(/<details class="thinking-fold">[\s\S]*?<\/details>/)
  if (thinkingMatch) {
    const thinkingText = thinkingMatch[0].replace(/<[^>]*>/g, '').trim()
    const lines = thinkingText.split('\n').filter((l) => l.trim())
    if (lines.length > 0) {
      content = content.replace(thinkingMatch[0], `> 💭 ${lines[0]}`)
    }
  }

  return `### **${role}**${ts}\n\n${content}\n\n---\n`
}

// ── 主导出函数 ────────────────────────────────────

export function exportSession(
  messages: Message[],
  options: ExportOptions = { format: 'markdown' },
): string {
  const {
    format,
    title = '对话导出',
    includeTimestamps = true,
  } = options

  if (format === 'json') {
    return JSON.stringify(
      {
        title,
        exportedAt: new Date().toISOString(),
        messages: messages.map((m) => ({
          role: m.role,
          content: m.content,
          timestamp: includeTimestamps ? new Date(m.timestamp).toISOString() : undefined,
        })),
      },
      null,
      2,
    )
  }

  // Markdown 导出
  const lines: string[] = [
    `# ${title}`,
    '',
    `> 导出时间：${new Date().toLocaleString('zh-CN')}`,
    `> 消息数：${messages.length} 条`,
    '',
    '---',
    '',
  ]

  for (const msg of messages) {
    lines.push(messageToMarkdown(msg, includeTimestamps, []))
  }

  return lines.join('\n')
}

// ── 下载 ──────────────────────────────────────────

export function downloadSession(
  content: string,
  filename: string,
  format: ExportFormat,
): void {
  const mimeType = format === 'json' ? 'application/json' : 'text/markdown'
  const blob = new Blob([content], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/** 生成默认文件名。 */
export function generateExportFilename(format: ExportFormat): string {
  const now = new Date()
  const dateStr = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, '0')}${String(now.getDate()).padStart(2, '0')}`
  const ext = format === 'json' ? 'json' : 'md'
  return `mady-会话-${dateStr}.${ext}`
}
