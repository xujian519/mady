/**
 * Mady 主题令牌系统。
 *
 * 提供 TypeScript 类型化的 CSS 变量引用，以及 useTheme hook。
 * CSS 变量本身定义在 styles/globals.css（Tailwind v4 @theme 指令）。
 *
 * 用法：
 * ```tsx
 * import { useTheme, type ThemeMode } from '@/theme/tokens'
 *
 * function Component() {
 *   const { mode, setMode, resolved } = useTheme()
 *   return <div>当前模式: {mode}（实际: {resolved}）</div>
 * }
 * ```
 */

import { createContext, useContext } from 'react'

// ── 主题模式 ──────────────────────────────────────

/** 用户选择的模式。 */
export type ThemeMode = 'light' | 'dark' | 'system'

/** 解析后的实际模式（始终是 light 或 dark）。 */
export type ResolvedTheme = 'light' | 'dark'

// ── 主题上下文 ────────────────────────────────────

export interface ThemeContextValue {
  /** 用户选择的模式。 */
  mode: ThemeMode
  /** 设置主题模式。 */
  setMode: (mode: ThemeMode) => void
  /** 解析后的实际模式（跟随系统时自动更新）。 */
  resolved: ResolvedTheme
  /** 是否为暗色模式（便捷判断）。 */
  isDark: boolean
}

export const ThemeContext = createContext<ThemeContextValue>({
  mode: 'system',
  setMode: () => {},
  resolved: 'light',
  isDark: false,
})

/** 获取当前主题上下文。 */
export function useTheme(): ThemeContextValue {
  return useContext(ThemeContext)
}

// ── CSS 变量名称（供程序化访问） ─────────────────────

export const CSS_VARS = {
  bgPrimary: '--color-mady-bg-primary',
  bgSecondary: '--color-mady-bg-secondary',
  bgTertiary: '--color-mady-bg-tertiary',
  textPrimary: '--color-mady-text-primary',
  textSecondary: '--color-mady-text-secondary',
  textTertiary: '--color-mady-text-tertiary',
  textLink: '--color-mady-text-link',
  accent: '--color-mady-accent',
  accentHover: '--color-mady-accent-hover',
  accentSoft: '--color-mady-accent-soft',
  accentGlow: '--color-mady-accent-glow',
  accentSecondary: '--color-mady-accent-secondary',
  danger: '--color-mady-danger',
  success: '--color-mady-success',
  warning: '--color-mady-warning',
  info: '--color-mady-info',
  separator: '--color-mady-separator',
  border: '--color-mady-border',
} as const
