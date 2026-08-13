/**
 * FileViewerOverlay — PilotDeck 风格文件查看/编辑器浮层。
 *
 * 右侧停靠面板，按文件类型分派：
 * - md：编辑（CodeMirror）/ 预览（MarkdownRenderer）双模式切换
 * - text：CodeMirror 纯文本编辑
 * - image：base64 内嵌预览 + 缩放
 * - pdf：pdf.js 内嵌查看器（T5.5）
 *
 * 交互：Cmd/Ctrl+S 保存；Esc 关闭（有未保存修改时先确认）；
 * 底部状态栏显示行列计数与保存提示（对齐 PilotDeck）。
 */

import React, { useCallback, useEffect, useState } from 'react'
import { X, Loader2, AlertCircle, FileText, Eye, Pencil, Save } from 'lucide-react'
import { useFilesStore } from '@/stores/files'
import { CodeEditor } from './CodeEditor'
import { ImagePreview } from './ImagePreview'
import { MarkdownPreview } from './MarkdownPreview'
import { PdfViewer } from './PdfViewer'
import { ConfirmDialog } from '@/components/ConfirmDialog'

/** 格式化文件大小。 */
function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

export const FileViewerOverlay: React.FC = () => {
  const { current, loading, saving, error, draft, closeFile, setDraft, saveFile } = useFilesStore()
  const dirty = draft !== null
  // 未保存修改时关闭文件需确认（WKWebView 不支持 window.confirm，统一走 ConfirmDialog，M-11）
  const [confirmClose, setConfirmClose] = useState(false)
  // md 文件的查看模式：preview（默认）或 edit
  const [mode, setMode] = useState<'preview' | 'edit'>('preview')

  // 打开新文件时重置为预览模式
  const currentPath = current?.path
  useEffect(() => {
    setMode('preview')
  }, [currentPath])

  const handleSave = useCallback(() => {
    void saveFile()
  }, [saveFile])

  // Esc 关闭
  useEffect(() => {
    if (!current && !loading) return
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (dirty) {
          setConfirmClose(true)
          return
        }
        closeFile()
      }
    }
    document.addEventListener('keydown', handleEsc)
    return () => document.removeEventListener('keydown', handleEsc)
  }, [current, loading, dirty, closeFile])

  if (!current && !loading && !error) return null

  const handleClose = () => {
    if (dirty) {
      setConfirmClose(true)
      return
    }
    closeFile()
  }

  const handleConfirmClose = () => {
    setConfirmClose(false)
    closeFile()
  }

  const isTextLike = current?.kind === 'text' || current?.kind === 'md'
  const text = draft ?? current?.text ?? ''
  const lines = text ? text.split('\n').length : 0

  return (
    <>
      <aside className={`${current?.kind === 'pdf' ? 'w-[680px]' : 'w-[460px]'} shrink-0 h-full flex flex-col bg-mady-bg-primary border-l border-mady-separator shadow-mady-modal`}>
      {/* 标题栏 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-mady-separator bg-mady-bg-secondary/50">
        <div className="flex items-center gap-2 min-w-0">
          <FileText size={14} className="shrink-0 text-mady-accent" />
          <div className="min-w-0">
            <div className="text-mady-ui font-medium text-mady-text-primary truncate">
              {current?.name ?? '加载中…'}
              {dirty && <span className="text-mady-warning ml-1" title="未保存">●</span>}
            </div>
            {current && (
              <div className="text-mady-caption text-mady-text-tertiary truncate">{current.path}</div>
            )}
          </div>
        </div>

        <div className="flex items-center gap-1 shrink-0">
          {/* md 编辑/预览切换 */}
          {current?.kind === 'md' && (
            <button
              onClick={() => setMode((m) => (m === 'preview' ? 'edit' : 'preview'))}
              className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
              title={mode === 'preview' ? '编辑' : '预览'}
            >
              {mode === 'preview' ? <Pencil size={13} /> : <Eye size={13} />}
            </button>
          )}
          {/* 保存按钮 */}
          {isTextLike && dirty && (
            <button
              onClick={handleSave}
              disabled={saving}
              className="p-1 rounded hover:bg-mady-bg-secondary text-mady-accent disabled:opacity-50"
              title="保存 (⌘S)"
            >
              {saving ? <Loader2 size={13} className="animate-spin" /> : <Save size={13} />}
            </button>
          )}
          <button
            onClick={handleClose}
            className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
            title="关闭 (Esc)"
          >
            <X size={14} />
          </button>
        </div>
      </div>

      {/* 内容区 */}
      <div className="flex-1 overflow-hidden">
        {loading ? (
          <div className="h-full flex items-center justify-center gap-2 text-mady-text-tertiary text-mady-ui">
            <Loader2 size={16} className="animate-spin" />
            加载中…
          </div>
        ) : error ? (
          <div className="h-full flex flex-col items-center justify-center gap-2 px-6 text-center">
            <AlertCircle size={20} className="text-mady-warning" />
            <span className="text-mady-ui text-mady-text-secondary">{error}</span>
          </div>
        ) : current?.kind === 'image' && current.data ? (
          <ImagePreview data={current.data} mime={current.mime ?? 'image/png'} name={current.name} />
        ) : current?.kind === 'pdf' && current.data ? (
          <PdfViewer data={current.data} name={current.name} />
        ) : current?.kind === 'md' && mode === 'preview' ? (
          <MarkdownPreview content={text} />
        ) : isTextLike ? (
          <CodeEditor
            value={text}
            markdown={current?.kind === 'md'}
            onChange={setDraft}
            onSave={handleSave}
          />
        ) : null}
      </div>

      {/* 状态栏 */}
      {current && (
        <div className="flex items-center justify-between px-3 py-1 border-t border-mady-separator text-mady-caption text-mady-text-tertiary">
          <span>
            {isTextLike && `Lines: ${lines} · Characters: ${text.length}`}
            {current.kind === 'image' && current.mime}
            {current.kind === 'pdf' && 'PDF'}
          </span>
          <span>
            {isTextLike && (saving ? '保存中…' : dirty ? '⌘S 保存 · Esc 关闭' : '已保存 · Esc 关闭')}
            {!isTextLike && formatSize(current.size)}
          </span>
        </div>
      )}
      </aside>
      <ConfirmDialog
        open={confirmClose}
        title="未保存的修改"
        message="有未保存的修改，确定放弃并关闭？"
        confirmLabel="放弃修改"
        onConfirm={handleConfirmClose}
        onCancel={() => setConfirmClose(false)}
      />
    </>
  )
}
