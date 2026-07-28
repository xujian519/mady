/**
 * ProjectTree — 可折叠的项目文件树。
 *
 * 展示项目根目录下的文件和文件夹结构，支持：
 * - 展开/折叠文件夹（懒加载子内容）
 * - 右键菜单：创建新文件夹
 * - 右键菜单：重命名文件夹
 * - Folder/File/FileText 图标（lucide-react）
 * - 加载状态和错误处理
 *
 * 用法：直接放在 Sidebar 中，不接受 props。
 *
 * ```tsx
 * import { ProjectTree } from '@/components/ProjectTree'
 *
 * <Sidebar>
 *   <ProjectTree />
 * </Sidebar>
 * ```
 */

import React, { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import {
  Folder,
  FolderOpen,
  File,
  FileText,
  ChevronRight,
  ChevronDown,
  Loader2,
  AlertCircle,
  Plus,
  Pencil,
  FilePlus,
  FolderPlus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { listDirectory, createFolder, renameFolder, deleteEntry, writeFile } from '@/lib/backend'
import type { FileEntry } from '@/lib/backend'
import { useFilesStore } from '@/stores/files'
import { useProjectStore, type ProjectInfo } from '@/stores/project'

// ── Types ─────────────────────────────────────────

/** 树节点（内部表示）。 */
interface TreeNode {
  path: string
  name: string
  isDir: boolean
  expanded: boolean
  loading: boolean
  error: string | null
  children: TreeNode[]
}

/** 右键菜单位置与目标节点。 */
interface ContextMenuState {
  x: number
  y: number
  node: TreeNode
}

/** 内联编辑状态。 */
interface EditingState {
  /** 编辑所在节点的 path（create/create-file 时为父节点 path，rename 时为节点自身 path；根目录为 ''）。 */
  path: string
  type: 'create' | 'create-file' | 'rename'
}

type TreeAction =
  | { type: 'SET_ROOT'; nodes: TreeNode[] }
  | { type: 'TOGGLE'; path: string }
  | { type: 'SET_LOADING'; path: string }
  | { type: 'SET_CHILDREN'; path: string; entries: FileEntry[] }
  | { type: 'SET_ERROR'; path: string; message: string }
  | { type: 'ADD_CHILD'; parentPath: string; name: string; fullPath: string }
  | { type: 'ADD_FILE_CHILD'; parentPath: string; name: string }
  | { type: 'REMOVE_NODE'; path: string }
  | { type: 'RENAME'; path: string; newName: string }

// ── Helpers ───────────────────────────────────────

function fileEntryToNode(e: FileEntry, parentPath: string): TreeNode {
  const path = parentPath ? `${parentPath}/${e.name}` : e.name
  return {
    path,
    name: e.name,
    isDir: e.isDir,
    expanded: false,
    loading: false,
    error: null,
    children: [],
  }
}

/** 按 path 更新树中的单个节点（不变处共享引用）。 */
function updateNode(
  nodes: TreeNode[],
  path: string,
  updater: (n: TreeNode) => TreeNode,
): TreeNode[] {
  return nodes.map((n) => {
    if (n.path === path) return updater(n)
    if (n.children.length > 0) {
      return { ...n, children: updateNode(n.children, path, updater) }
    }
    return n
  })
}

/** 在树中按 path 查找节点。 */
function findNode(nodes: TreeNode[], path: string): TreeNode | null {
  for (const n of nodes) {
    if (n.path === path) return n
    if (n.children.length > 0) {
      const found = findNode(n.children, path)
      if (found) return found
    }
  }
  return null
}

function treeReducer(state: TreeNode[], action: TreeAction): TreeNode[] {
  switch (action.type) {
    case 'SET_ROOT':
      return action.nodes

    case 'TOGGLE':
      return updateNode(state, action.path, (n) => ({ ...n, expanded: !n.expanded }))

    case 'SET_LOADING':
      return updateNode(state, action.path, (n) => ({ ...n, loading: true, error: null }))

    case 'SET_CHILDREN':
      return updateNode(state, action.path, (n) => ({
        ...n,
        expanded: true,
        loading: false,
        children: action.entries.map((e) => fileEntryToNode(e, n.path)),
        error: null,
      }))

    case 'SET_ERROR':
      return updateNode(state, action.path, (n) => ({
        ...n,
        loading: false,
        error: action.message,
      }))

    case 'ADD_CHILD': {
      const newDir: TreeNode = {
        path: action.fullPath,
        name: action.name,
        isDir: true,
        expanded: false,
        loading: false,
        error: null,
        children: [],
      }
      // 根目录新建：parentPath 为 ''
      if (action.parentPath === '') {
        if (state.some((c) => c.name === action.name)) return state
        return [...state, newDir]
      }
      return updateNode(state, action.parentPath, (n) => {
        if (!n.isDir) return n
        const exists = n.children.some((c) => c.name === action.name)
        if (exists) return n
        return { ...n, expanded: true, children: [...n.children, newDir] }
      })
    }

    case 'ADD_FILE_CHILD': {
      const parentPath = action.parentPath
      const fullPath = parentPath ? `${parentPath}/${action.name}` : action.name
      const newFile: TreeNode = {
        path: fullPath,
        name: action.name,
        isDir: false,
        expanded: false,
        loading: false,
        error: null,
        children: [],
      }
      if (parentPath === '') {
        if (state.some((c) => c.name === action.name)) return state
        return [...state, newFile]
      }
      return updateNode(state, parentPath, (n) => {
        if (!n.isDir) return n
        const exists = n.children.some((c) => c.name === action.name)
        if (exists) return n
        return { ...n, expanded: true, children: [...n.children, newFile] }
      })
    }

    case 'REMOVE_NODE':
      return removeNode(state, action.path)

    case 'RENAME':
      return updateNode(state, action.path, (n) => ({
        ...n,
        name: action.newName,
        path: n.path.replace(/\/[^/]*$/, `/${action.newName}`),
      }))

    default:
      return state
  }
}

/** 按 path 从树中移除节点（不变处共享引用）。 */
function removeNode(nodes: TreeNode[], path: string): TreeNode[] {
  return nodes
    .filter((n) => n.path !== path)
    .map((n) =>
      n.children.length > 0 ? { ...n, children: removeNode(n.children, path) } : n,
    )
}

/** 排序：目录在前，之后按名称字母序。 */
function sortEntries(entries: FileEntry[]): FileEntry[] {
  return [...entries].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

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

const TreeNodeRow: React.FC<TreeNodeRowProps> = ({
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

// ── Main Component ─────────────────────────────────

export const ProjectTree: React.FC = () => {
  const [tree, dispatch] = useReducer(treeReducer, [])
  const [rootLoading, setRootLoading] = useState(true)
  const [rootError, setRootError] = useState<string | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [editing, setEditing] = useState<EditingState | null>(null)
  const [showNewProject, setShowNewProject] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')
  const createInputRef = useRef<HTMLInputElement>(null)
  const newProjectInputRef = useRef<HTMLInputElement>(null)

  // 项目状态
  const projectCurrent = useProjectStore((s) => s.current)
  const projectProjects = useProjectStore((s) => s.projects)
  const projectLoading = useProjectStore((s) => s.loading)
  const projectError = useProjectStore((s) => s.error)
  const loadProjects = useProjectStore((s) => s.load)
  const createProjectFolder = useProjectStore((s) => s.createProjectFolder)
  const selectProjectFolder = useProjectStore((s) => s.selectProjectFolder)
  const switchProject = useProjectStore((s) => s.switchProject)

  useEffect(() => {
    loadProjects()
  }, [loadProjects])

  // ── 根目录加载（初始 + 刷新） ──────────────────────

  const loadRoot = useCallback(() => {
    setRootLoading(true)
    setRootError(null)

    listDirectory('')
      .then((entries) => {
        dispatch({ type: 'SET_ROOT', nodes: sortEntries(entries).map((e) => fileEntryToNode(e, '')) })
        setRootLoading(false)
      })
      .catch((err: unknown) => {
        setRootError(err instanceof Error ? err.message : String(err))
        setRootLoading(false)
      })
  }, [])

  useEffect(() => {
    loadRoot()
  }, [loadRoot])

  // ── 右键菜单：点击外部关闭 ─────────────────────────

  useEffect(() => {
    if (!contextMenu) return
    const handleClick = () => setContextMenu(null)
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setContextMenu(null)
    }
    document.addEventListener('click', handleClick)
    document.addEventListener('keydown', handleEsc)
    return () => {
      document.removeEventListener('click', handleClick)
      document.removeEventListener('keydown', handleEsc)
    }
  }, [contextMenu])

  // ── 新建输入框自动聚焦 ────────────────────────────

  useEffect(() => {
    if ((editing?.type === 'create' || editing?.type === 'create-file') && createInputRef.current) {
      createInputRef.current.focus()
    }
  }, [editing])

  useEffect(() => {
    if (showNewProject && newProjectInputRef.current) {
      newProjectInputRef.current.focus()
    }
  }, [showNewProject])

  // ── Handlers ──────────────────────────────────────

  const handleToggle = useCallback(
    async (path: string) => {
      const node = findNode(tree, path)
      if (!node || !node.isDir) return

      // 已展开则折叠
      if (node.expanded) {
        dispatch({ type: 'TOGGLE', path })
        return
      }

      // 懒加载
      dispatch({ type: 'SET_LOADING', path })
      try {
        const entries = await listDirectory(path)
        dispatch({ type: 'SET_CHILDREN', path, entries: sortEntries(entries) })
      } catch (err: unknown) {
        dispatch({
          type: 'SET_ERROR',
          path,
          message: err instanceof Error ? err.message : String(err),
        })
      }
    },
    [tree],
  )

  const handleContextMenu = useCallback((e: React.MouseEvent, node: TreeNode) => {
    // 编辑进行中时不显示菜单
    if (editing) return
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({ x: e.clientX, y: e.clientY, node })
  }, [editing])

  const handleCreateFolder = useCallback(() => {
    if (!contextMenu) return
    setEditing({ path: contextMenu.node.path, type: 'create' })
    setContextMenu(null)
  }, [contextMenu])

  const handleCreateFile = useCallback(() => {
    if (!contextMenu) return
    setEditing({ path: contextMenu.node.path, type: 'create-file' })
    setContextMenu(null)
  }, [contextMenu])

  const handleRenameFolder = useCallback(() => {
    if (!contextMenu) return
    setEditing({ path: contextMenu.node.path, type: 'rename' })
    setContextMenu(null)
  }, [contextMenu])

  /** 删除文件或空目录（二次确认）。 */
  const handleDelete = useCallback(() => {
    if (!contextMenu) return
    const node = contextMenu.node
    setContextMenu(null)
    const label = node.isDir ? '文件夹（须为空）' : '文件'
    if (!window.confirm(`确定删除${label}「${node.name}」？此操作不可撤销。`)) return
    deleteEntry(node.path)
      .then(() => dispatch({ type: 'REMOVE_NODE', path: node.path }))
      .catch((err: unknown) => {
        window.alert(err instanceof Error ? err.message : String(err))
      })
  }, [contextMenu])

  /** 完成编辑（新建文件夹或重命名）。 */
  const handleFinishEdit = useCallback(
    async (path: string, value: string) => {
      const trimmed = value.trim()
      if (!trimmed) {
        setEditing(null)
        return
      }

      if (editing?.type === 'create') {
        // path 是父节点路径（'' 表示根目录）
        try {
          const fullPath = await createFolder(path, trimmed)
          dispatch({ type: 'ADD_CHILD', parentPath: path, name: trimmed, fullPath })
        } catch {
          // 静默失败
        }
      } else if (editing?.type === 'create-file') {
        // 写入空文件即完成创建
        const fullPath = path ? `${path}/${trimmed}` : trimmed
        try {
          await writeFile(fullPath, '')
          dispatch({ type: 'ADD_FILE_CHILD', parentPath: path, name: trimmed })
        } catch {
          // 静默失败
        }
      } else if (editing?.type === 'rename') {
        // path 是节点自身路径
        try {
          await renameFolder(path, trimmed)
          dispatch({ type: 'RENAME', path, newName: trimmed })
        } catch {
          // 静默失败
        }
      }
      setEditing(null)
    },
    [editing],
  )

  const handleCancelEdit = useCallback(() => {
    setEditing(null)
  }, [])

  /** 文件点击：在查看器浮层中打开。 */
  const handleOpenFile = useCallback((path: string) => {
    void useFilesStore.getState().openFile(path)
  }, [])

  // ── 递归渲染 ──────────────────────────────────────

  const renderNodes = (nodes: TreeNode[], depth: number): React.ReactNode[] => {
    return nodes.flatMap((node) => {
      const isRenaming = editing?.path === node.path && editing?.type === 'rename'
      const isCreatingHere =
        editing?.path === node.path && (editing?.type === 'create' || editing?.type === 'create-file')

      const rows: React.ReactNode[] = [
        <TreeNodeRow
          key={node.path}
          node={node}
          depth={depth}
          onToggle={handleToggle}
          onContextMenu={handleContextMenu}
          onOpenFile={handleOpenFile}
          renaming={isRenaming}
          onFinishRename={handleFinishEdit}
          onCancelEdit={handleCancelEdit}
        />,
      ]

      // 新建文件夹/文件的内联输入框（紧跟父节点之后）
      if (isCreatingHere) {
        const isFile = editing?.type === 'create-file'
        rows.push(
          <div
            key={`${node.path}--create`}
            className="flex items-center gap-1 px-2 py-0.5"
            style={{ paddingLeft: `${12 + (depth + 1) * 14}px` }}
          >
            {isFile ? (
              <File size={14} className="text-mady-accent shrink-0" />
            ) : (
              <Folder size={14} className="text-mady-accent shrink-0" />
            )}
            <input
              ref={createInputRef}
              placeholder={isFile ? '文件名称（如 notes.md）' : '文件夹名称'}
              defaultValue=""
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  handleFinishEdit(node.path, e.currentTarget.value)
                } else if (e.key === 'Escape') {
                  handleCancelEdit()
                }
              }}
              onBlur={(e) => {
                const val = e.target.value.trim()
                if (!val) {
                  setEditing(null)
                }
              }}
              className="flex-1 min-w-0 bg-mady-bg-primary border border-mady-accent rounded px-1 py-0.5 text-mady-ui text-mady-text-primary outline-none"
            />
          </div>,
        )
      }

      // 展开的子节点
      if (node.isDir && node.expanded && node.children.length > 0) {
        rows.push(...renderNodes(node.children, depth + 1))
      }

      return rows
    })
  }

  // ── Render ────────────────────────────────────────

  return (
    <div className="select-none">
      {/* 项目选择器 */}
      <div className="px-3 py-2 border-b border-mady-separator">
        <div className="flex items-center justify-between mb-1.5">
          <span className="text-mady-caption font-medium text-mady-text-secondary uppercase tracking-wider">
            当前项目
          </span>
          <div className="flex items-center gap-0.5">
            <button
              onClick={selectProjectFolder}
              className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
              title="打开现有文件夹作为项目"
            >
              <FolderOpen size={13} />
            </button>
            <button
              onClick={() => {
                setShowNewProject(true)
                setNewProjectName('')
              }}
              className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
              title="新建项目文件夹"
            >
              <FolderPlus size={13} />
            </button>
            <button
              onClick={loadProjects}
              className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
              title="刷新"
            >
              <RefreshCw size={12} className={projectLoading ? 'animate-spin' : ''} />
            </button>
          </div>
        </div>

        {projectError && (
          <div className="text-mady-caption text-mady-danger mb-1.5">
            {projectError}
          </div>
        )}

        {projectCurrent ? (
          <div className="flex items-center gap-1.5 text-mady-ui text-mady-text-primary" title={projectCurrent.path}>
            <Folder size={14} className="text-mady-accent shrink-0" />
            <span className="truncate flex-1">{projectCurrent.alias || projectCurrent.path}</span>
          </div>
        ) : (
          <div className="text-mady-caption text-mady-text-tertiary">
            未选择项目
          </div>
        )}

        {/* 新建项目输入框 */}
        {showNewProject && (
          <div className="mt-1.5 flex items-center gap-1">
            <input
              ref={newProjectInputRef}
              type="text"
              value={newProjectName}
              onChange={(e) => setNewProjectName(e.target.value)}
              placeholder="项目名称"
              className="flex-1 min-w-0 bg-mady-bg-primary border border-mady-accent rounded px-2 py-0.5 text-mady-ui text-mady-text-primary outline-none"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newProjectName.trim()) {
                  createProjectFolder(newProjectName.trim())
                } else if (e.key === 'Escape') {
                  setShowNewProject(false)
                }
              }}
              onBlur={() => {
                if (!newProjectName.trim()) setShowNewProject(false)
              }}
            />
            <button
              onClick={() => createProjectFolder(newProjectName.trim())}
              disabled={!newProjectName.trim()}
              className="px-2 py-0.5 rounded text-mady-caption bg-mady-accent text-white disabled:opacity-50"
            >
              创建
            </button>
          </div>
        )}

        {/* 最近项目列表 */}
        {projectProjects.length > 0 && (
          <div className="mt-2 pt-2 border-t border-mady-separator/50">
            <div className="text-mady-caption text-mady-text-tertiary mb-1">
              最近项目
            </div>
            <div className="space-y-0.5 max-h-32 overflow-y-auto">
              {projectProjects.map((p: ProjectInfo) => (
                <button
                  key={p.id}
                  onClick={() => switchProject(p.id)}
                  className={`w-full flex items-center gap-1.5 px-2 py-1 rounded text-left text-mady-ui transition-colors ${
                    p.id === projectCurrent?.id
                      ? 'bg-mady-accent-soft text-mady-accent'
                      : 'text-mady-text-secondary hover:bg-mady-bg-primary hover:text-mady-text-primary'
                  }`}
                  title={p.path}
                >
                  <Folder size={12} className="shrink-0" />
                  <span className="truncate flex-1">{p.alias || p.path}</span>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* 文件树头部 + 工具栏 */}
      <div className="px-3 py-2 border-b border-mady-separator flex items-center justify-between">
        <span className="text-mady-caption font-medium text-mady-text-secondary uppercase tracking-wider">
          项目文件
        </span>
        <div className="flex items-center gap-0.5">
          <button
            onClick={() => setEditing({ path: '', type: 'create-file' })}
            className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
            title="新建文件"
          >
            <FilePlus size={13} />
          </button>
          <button
            onClick={() => setEditing({ path: '', type: 'create' })}
            className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
            title="新建文件夹"
          >
            <FolderPlus size={13} />
          </button>
          <button
            onClick={loadRoot}
            className="p-1 rounded hover:bg-mady-bg-primary text-mady-text-tertiary transition-colors"
            title="刷新"
          >
            <RefreshCw size={12} />
          </button>
        </div>
      </div>

      {/* 根级新建输入框 */}
      {editing && editing.path === '' && (editing.type === 'create' || editing.type === 'create-file') && (
        <div className="flex items-center gap-1 px-2 py-1 border-b border-mady-separator/50" style={{ paddingLeft: '12px' }}>
          {editing.type === 'create-file' ? (
            <File size={14} className="text-mady-accent shrink-0" />
          ) : (
            <Folder size={14} className="text-mady-accent shrink-0" />
          )}
          <input
            ref={createInputRef}
            placeholder={editing.type === 'create-file' ? '文件名称（如 notes.md）' : '文件夹名称'}
            defaultValue=""
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleFinishEdit('', e.currentTarget.value)
              } else if (e.key === 'Escape') {
                handleCancelEdit()
              }
            }}
            onBlur={(e) => {
              if (!e.target.value.trim()) setEditing(null)
            }}
            className="flex-1 min-w-0 bg-mady-bg-primary border border-mady-accent rounded px-1 py-0.5 text-mady-ui text-mady-text-primary outline-none"
          />
        </div>
      )}

      {/* 树区域 */}
      <div className="overflow-y-auto py-1" style={{ maxHeight: 'calc(100vh - 280px)' }}>
        {rootLoading ? (
          <div className="flex items-center justify-center gap-2 py-8 text-mady-text-tertiary text-mady-caption">
            <Loader2 size={14} className="animate-spin" />
            加载中…
          </div>
        ) : rootError ? (
          <div className="flex flex-col items-center gap-2 py-8 px-4 text-center">
            <AlertCircle size={16} className="text-mady-warning" />
            <span className="text-mady-caption text-mady-text-tertiary">
              加载失败: {rootError}
            </span>
            <button
              onClick={() => window.location.reload()}
              className="text-mady-caption text-mady-accent hover:text-mady-accent-hover transition-colors"
            >
              重试
            </button>
          </div>
        ) : tree.length === 0 ? (
          <div className="py-8 text-center text-mady-caption text-mady-text-tertiary">
            项目为空
          </div>
        ) : (
          <div>{renderNodes(tree, 0)}</div>
        )}
      </div>

      {/* 右键菜单 */}
      {contextMenu && (
        <div
          className="fixed z-50 min-w-36 py-1 bg-mady-bg-primary border border-mady-separator rounded-lg shadow-lg"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onClick={(e) => e.stopPropagation()}
        >
          {contextMenu.node.isDir && (
            <>
              <button
                onClick={handleCreateFile}
                className="w-full flex items-center gap-2 px-3 py-1.5 text-mady-ui text-mady-text-primary hover:bg-mady-accent-soft transition-colors text-left"
              >
                <FilePlus size={12} className="text-mady-text-tertiary" />
                新建文件
              </button>
              <button
                onClick={handleCreateFolder}
                className="w-full flex items-center gap-2 px-3 py-1.5 text-mady-ui text-mady-text-primary hover:bg-mady-accent-soft transition-colors text-left"
              >
                <Plus size={12} className="text-mady-text-tertiary" />
                新建文件夹
              </button>
            </>
          )}
          <button
            onClick={handleRenameFolder}
            className="w-full flex items-center gap-2 px-3 py-1.5 text-mady-ui text-mady-text-primary hover:bg-mady-accent-soft transition-colors text-left"
          >
            <Pencil size={12} className="text-mady-text-tertiary" />
            重命名
          </button>
          <button
            onClick={handleDelete}
            className="w-full flex items-center gap-2 px-3 py-1.5 text-mady-ui text-mady-danger hover:bg-mady-danger/10 transition-colors text-left"
          >
            <Trash2 size={12} />
            删除
          </button>
        </div>
      )}
    </div>
  )
}
