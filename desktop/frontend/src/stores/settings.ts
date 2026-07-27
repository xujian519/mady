/**
 * 设置 Zustand Store — 持久化到 localStorage。
 *
 * 存储：
 * - 主题模式（light / dark / system）
 * - Provider 选择
 * - 模型选择
 *
 * Provider/Model 切换写入全局配置（复用 pkg/agentconfig），
 * 仅新会话生效。切换时弹 Toast 提示。
 */

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { type ThemeMode, type ThemePackId } from '@/theme/tokens'

// ── Types ─────────────────────────────────────────

export interface SettingsState {
  /** 主题模式。 */
  themeMode: ThemeMode
  /** 主题包 ID。 */
  themePack: ThemePackId
  /** 布局模式。 */
  layout: LayoutMode
  /** 默认 Provider。 */
  provider: string
  /** 默认模型。 */
  model: string
}

interface SettingsActions {
  /** 更新设置。 */
  update: (partial: Partial<SettingsState>) => void
  /** 重置为默认值。 */
  reset: () => void
}

/** 布局模式。 */
export type LayoutMode = 'standard' | 'focus'

export type SettingsStore = SettingsState & SettingsActions

const DEFAULTS: SettingsState = {
  themeMode: 'system',
  themePack: 'professional',
  layout: 'standard',
  provider: '',
  model: '',
}

export const useSettingsStore = create<SettingsStore>()(
  persist(
    (set) => ({
      ...DEFAULTS,

      update: (partial) => {
        set(partial)

        // Provider/Model 切换时提示（在生产环境可改为 Toast）
        if (partial.provider || partial.model) {
          console.info('[settings] Provider/Model 变更将在下一轮对话中生效')
        }
      },

      reset: () => set(DEFAULTS),
    }),
    {
      name: 'mady-settings',
    },
  ),
)
