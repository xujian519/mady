/**
 * project store — 桌面端项目（案件）状态管理。
 *
 * 与 domains.ProjectRegistry 共享项目历史：
 * - 列出已注册项目
 * - 新建项目文件夹
 * - 打开现有文件夹作为项目
 * - 切换当前项目（触发 Wails 窗口重载）
 */

import { create } from 'zustand'
import {
  listProjects,
  getCurrentProject,
  createProjectFolder as backendCreateProjectFolder,
  selectProjectFolder as backendSelectProjectFolder,
  switchProject as backendSwitchProject,
  type ProjectInfo as ProjectInfoBackend,
} from '@/lib/backend'

export type ProjectInfo = ProjectInfoBackend

interface ProjectState {
  /** 当前生效项目。 */
  current: ProjectInfo | null
  /** 已注册项目列表。 */
  projects: ProjectInfo[]
  /** 是否正在加载项目列表。 */
  loading: boolean
  /** 错误信息。 */
  error: string | null
  /** 加载当前项目与项目列表。 */
  load: () => Promise<void>
  /** 新建项目文件夹。 */
  createProjectFolder: (name: string) => Promise<void>
  /** 打开现有文件夹作为项目。 */
  selectProjectFolder: () => Promise<void>
  /** 切换到指定项目。 */
  switchProject: (projectID: string) => Promise<void>
}

export const useProjectStore = create<ProjectState>((set) => ({
  current: null,
  projects: [],
  loading: false,
  error: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const [current, projects] = await Promise.all([
        getCurrentProject().catch(() => null),
        listProjects().catch(() => []),
      ])
      set({ current, projects, loading: false })
    } catch (err: unknown) {
      set({
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      })
    }
  },

  createProjectFolder: async (name: string) => {
    set({ loading: true, error: null })
    try {
      await backendCreateProjectFolder(name)
      // 成功后会触发 Wails 窗口重载，页面刷新后自动 load
    } catch (err: unknown) {
      set({
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      })
    }
  },

  selectProjectFolder: async () => {
    set({ loading: true, error: null })
    try {
      await backendSelectProjectFolder()
    } catch (err: unknown) {
      set({
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      })
    }
  },

  switchProject: async (projectID: string) => {
    set({ loading: true, error: null })
    try {
      await backendSwitchProject(projectID)
    } catch (err: unknown) {
      set({
        loading: false,
        error: err instanceof Error ? err.message : String(err),
      })
    }
  },
}))
