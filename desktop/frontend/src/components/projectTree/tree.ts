/**
 * 项目树的纯逻辑：节点模型、reducer 与排序。
 *
 * 从 ProjectTree.tsx（M55）拆出：不含 React 依赖，便于单测与复用。
 */

import type { FileEntry } from '@/lib/backend'

/** 树节点（内部表示）。 */
export interface TreeNode {
  path: string
  name: string
  isDir: boolean
  expanded: boolean
  loading: boolean
  error: string | null
  children: TreeNode[]
}

export type TreeAction =
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

export function fileEntryToNode(e: FileEntry, parentPath: string): TreeNode {
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
export function findNode(nodes: TreeNode[], path: string): TreeNode | null {
  for (const n of nodes) {
    if (n.path === path) return n
    if (n.children.length > 0) {
      const found = findNode(n.children, path)
      if (found) return found
    }
  }
  return null
}

export function treeReducer(state: TreeNode[], action: TreeAction): TreeNode[] {
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
export function sortEntries(entries: FileEntry[]): FileEntry[] {
  return [...entries].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}
