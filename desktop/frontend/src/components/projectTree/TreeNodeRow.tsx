/**
 * 项目树单行渲染（图标 / 展开箭头 / 重命名输入框 / 错误指示）。
 *
 * 从 ProjectTree.tsx（M55）拆出；props 与渲染逻辑保持不变。
 */

import React, { useEffect, useRef } from 'react'
import { Folder, File, FileText, ChevronRight, ChevronDown, Loader2, AlertCircle } from 'lucide-react'
import type { TreeNode } from './tree'

// ── TreeNodeRow Sub-component ─────────────────────

interface TreeNodeRowProps {
  node: TreeNode
  depth: number
  onToggle: (path: string) => void
  onContextMenu: (e: React.MouseEvent, node: TreeNode) => void
  /** 文件节点点击：打开查看器。 */
  onOpenFile: (path: string) => void
  /** 当前是否正处于 rename 编辑状态（只有匹配时显示 input）。 */
  renaming: boolean
  onFinishRename: (path: string, value: string) => void
  onCancelEdit: () => void
}

export const TreeNodeRow: React.FC<TreeNodeRowProps> = ({
  node,
  depth,
  onToggle,
  onContextMenu,
  onOpenFile,
  renaming,
  onFinishRename,
  onCancelEdit,
}) => {
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (renaming && inputRef.current) {
      inputRef.current.focus()
      // 文件：选中扩展名之前的部分；文件夹：全选
      if (!node.isDir) {
        const dotIdx = node.name.lastIndexOf('.')
        inputRef.current.setSelectionRange(0, dotIdx > 0 ? dotIdx : node.name.length)
      } else {
        inputRef.current.select()
      }
    }
  }, [renaming, node.name, node.isDir])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      onFinishRename(node.path, e.currentTarget.value)
    } else if (e.key === 'Escape') {
      onCancelEdit()
    }
  }

  // 选择图标
  const icon = node.isDir ? (
    <Folder size={14} className="text-mady-accent shrink-0" />
  ) : node.name.endsWith('.md') ? (
    <FileText size={14} className="text-mady-text-tertiary shrink-0" />
  ) : (
    <File size={14} className="text-mady-text-tertiary shrink-0" />
  )

  return (
    <div
      onContextMenu={(e) => onContextMenu(e, node)}
      onClick={() => {
        if (!node.isDir && !renaming) onOpenFile(node.path)
      }}
      className="group flex items-center gap-1 px-2 py-0.5 rounded-md cursor-pointer hover:bg-mady-bg-primary text-mady-ui text-mady-text-primary transition-colors"
      style={{ paddingLeft: `${12 + depth * 14}px` }}
    >
      {/* 展开/折叠箭头（仅文件夹） */}
      {node.isDir ? (
        <button
          onClick={() => onToggle(node.path)}
          className="p-0.5 rounded hover:bg-mady-accent-soft transition-colors shrink-0"
          tabIndex={-1}
        >
          {node.loading ? (
            <Loader2 size={12} className="animate-spin text-mady-text-tertiary" />
          ) : node.expanded ? (
            <ChevronDown size={12} className="text-mady-text-tertiary" />
          ) : (
            <ChevronRight size={12} className="text-mady-text-tertiary" />
          )}
        </button>
      ) : (
        <span className="w-5 shrink-0" />
      )}

      {/* 图标 */}
      {icon}

      {/* 名字或重命名输入框 */}
      {renaming ? (
        <input
          ref={inputRef}
          defaultValue={node.name}
          onKeyDown={handleKeyDown}
          onBlur={(e) => onFinishRename(node.path, e.target.value)}
          className="flex-1 min-w-0 bg-mady-bg-primary border border-mady-accent rounded px-1 py-0.5 text-mady-ui text-mady-text-primary outline-none"
          onClick={(e) => e.stopPropagation()}
        />
      ) : (
        <span className="truncate flex-1">{node.name}</span>
      )}

      {/* 错误指示 */}
      {node.error && (
        <span title={node.error}>
          <AlertCircle size={12} className="text-mady-warning shrink-0" />
        </span>
      )}
    </div>
  )
}
