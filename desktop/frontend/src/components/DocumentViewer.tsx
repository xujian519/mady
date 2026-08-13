/**
 * DocumentViewer — 分屏文档预览组件。
 *
 * 位于 ChatView 右侧面板，用于预览文档内容。
 * 阶段 3 实现基础版：
 * - HTML 内容内嵌渲染（通过 iframe srcdoc）
 * - PDF 调用系统默认应用（macOS Preview / QuickLook）
 * - 纯文本/Markdown 直接渲染
 *
 * 阶段 5 再评估 pdf.js 内嵌渲染。
 */

import React from 'react'
import { FileText, ExternalLink, X } from 'lucide-react'

export interface DocViewerFile {
  /** 文件名。 */
  name: string
  /** 文件类型：html / pdf / text / md。 */
  type: 'html' | 'pdf' | 'text' | 'md'
  /** 文件内容（html/text/md 类型的原始内容）。 */
  content?: string
  /** PDF 文件的本地路径。 */
  path?: string
}

/**
 * Wails Runtime 类型声明（避免滥用 @ts-expect-error）。
 * 仅在 Wails 生产环境中可用，开发期/CI 为 undefined。
 */
interface WailsRuntime {
  BrowserOpenURL?: (url: string) => void
}

interface DocumentViewerProps {
  file: DocViewerFile | null
  onClose: () => void
}

export const DocumentViewer: React.FC<DocumentViewerProps> = ({ file, onClose }) => {
  if (!file) {
    return (
      <aside className="w-[var(--mady-context-width)] h-full flex flex-col bg-mady-bg-secondary border-l border-mady-separator">
        <div className="flex-1 flex items-center justify-center text-mady-text-tertiary">
          <div className="text-center px-4">
            <FileText size={24} className="mx-auto mb-2 opacity-50" />
            <p className="text-mady-caption">选择文档查看</p>
          </div>
        </div>
      </aside>
    )
  }

  const isPdf = file.type === 'pdf'
  const isHtml = file.type === 'html'
  const isText = file.type === 'text' || file.type === 'md'

  const openInSystem = () => {
    if (file.path) {
      // 在 Wails 环境中调用系统默认应用
      const wr = (window as unknown as { runtime?: WailsRuntime }).runtime
      if (wr?.BrowserOpenURL) {
        wr.BrowserOpenURL(`file://${file.path}`)
      } else {
        window.open(`file://${file.path}`, '_blank')
      }
    }
  }

  return (
    <aside className="w-[var(--mady-context-width)] h-full flex flex-col bg-mady-bg-secondary border-l border-mady-separator">
      {/* 标题栏 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-mady-separator">
        <div className="flex items-center gap-2 min-w-0">
          <FileText size={14} className="shrink-0 text-mady-accent" />
          <span className="text-mady-ui text-mady-text-primary truncate">{file.name}</span>
        </div>
        <div className="flex items-center gap-1">
          {isPdf && (
            <button
              onClick={openInSystem}
              className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-secondary"
              title="在系统应用中打开"
            >
              <ExternalLink size={13} />
            </button>
          )}
          <button
            onClick={onClose}
            className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-secondary"
            title="关闭"
          >
            <X size={13} />
          </button>
        </div>
      </div>

      {/* 内容区 */}
      <div className="flex-1 overflow-y-auto">
        {isHtml && file.content && (
          <iframe
            srcDoc={file.content}
            className="w-full h-full border-0"
            title={file.name}
            sandbox="allow-same-origin"
          />
        )}

        {isPdf && (
          <div className="flex flex-col items-center justify-center h-full text-center px-6">
            <FileText size={40} className="text-mady-text-tertiary mb-3 opacity-50" />
            <p className="text-mady-ui text-mady-text-secondary mb-2">
              PDF 预览将在系统 PDF 应用中打开
            </p>
            <button
              onClick={openInSystem}
              className="px-4 py-1.5 rounded-lg bg-mady-accent text-white text-mady-ui hover:bg-mady-accent-hover transition-colors"
            >
              打开
            </button>
          </div>
        )}

        {isText && file.content && (
          <pre className="p-4 text-mady-small font-mono text-mady-text-primary whitespace-pre-wrap break-words">
            {file.content}
          </pre>
        )}
      </div>
    </aside>
  )
}
