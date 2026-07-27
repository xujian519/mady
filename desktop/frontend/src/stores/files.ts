/**
 * files store — 文件查看器状态。
 *
 * 管理当前打开的文件、加载状态与脏标记。
 * ProjectTree 点击文件 → openFile；FileViewerOverlay 消费本 store。
 */

import { create } from 'zustand'
import { readFile, writeFile, type FileContent } from '@/lib/backend'

interface FilesState {
  /** 当前打开的文件内容；null 表示查看器关闭。 */
  current: FileContent | null
  loading: boolean
  /** 保存进行中。 */
  saving: boolean
  error: string | null
  /** 编辑后的草稿内容（仅 text/md）。 */
  draft: string | null
  /** 打开文件并加载内容。 */
  openFile: (relPath: string) => Promise<void>
  /** 关闭查看器（有未保存修改时由调用方先确认）。 */
  closeFile: () => void
  /** 更新草稿内容。 */
  setDraft: (text: string) => void
  /** 保存成功后重置草稿。 */
  clearDraft: () => void
  /** 保存草稿到磁盘；成功返回 true。 */
  saveFile: () => Promise<boolean>
}

export const useFilesStore = create<FilesState>((set, get) => ({
  current: null,
  loading: false,
  saving: false,
  error: null,
  draft: null,

  openFile: async (relPath: string) => {
    set({ loading: true, error: null, draft: null })
    try {
      const content = await readFile(relPath)
      set({ current: content, loading: false })
    } catch (err: unknown) {
      set({
        current: null,
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      })
    }
  },

  closeFile: () => set({ current: null, error: null, draft: null }),

  setDraft: (text: string) => set({ draft: text }),

  clearDraft: () => set({ draft: null }),

  saveFile: async () => {
    const { current, draft } = get()
    if (!current || draft === null) return true
    if (current.kind !== 'text' && current.kind !== 'md') return false

    set({ saving: true })
    try {
      await writeFile(current.path, draft)
      set({
        saving: false,
        draft: null,
        current: { ...current, text: draft, size: new TextEncoder().encode(draft).length },
      })
      return true
    } catch (err: unknown) {
      set({
        saving: false,
        error: err instanceof Error ? err.message : String(err),
      })
      return false
    }
  },
}))
