/**
 * 设置 Zustand Store — 持久化到 localStorage（UI 缓存）。
 *
 * 存储：
 * - 主题模式（light / dark / system）
 * - Provider 选择
 * - 模型选择
 * - 推理设置（modelId / reasoningEffort / contextWindow / temperature）
 *
 * 注意：Provider/Model 的真相源是后端（~/.mady/desktop-settings.json，
 * 通过 SetAISettings binding 写入）。本 store 仅作为 UI 缓存与即时回显，
 * 挂载时由 SettingsPanel 从 GetAISettings 同步。
 *
 * 推理设置暂为纯前端状态（无后端端点），通过 localStorage 持久化。
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
  /** 选中的模型 ID（ModelSettings 用）。 */
  modelId: string
  /** 推理力度。 */
  reasoningEffort: 'low' | 'medium' | 'high' | 'max'
  /** 上下文窗口尺寸。 */
  contextWindow: string
  /** 温度。 */
  temperature: number
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
  modelId: 'deepseek-v4',
  reasoningEffort: 'medium',
  contextWindow: '128K',
  temperature: 0.7,
}

export const useSettingsStore = create<SettingsStore>()(
  persist(
    (set) => ({
      ...DEFAULTS,

      update: (partial) => {
        set(partial)
      },

      reset: () => set(DEFAULTS),
    }),
    {
      name: 'mady-settings',
    },
  ),
)
