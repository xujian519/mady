/**
 * ThemePacks — 主题包定义。
 *
 * 每个主题包定义一组 CSS 变量覆盖值（亮色和暗色），
 * 用于切换 Mady 的整体视觉风格。
 *
 * 当前包含 4 套专业风格主题包：
 *   professional — Mady 品牌靛蓝（默认）
 *   focus-blue   — 专注蓝
 *   paper-warm   — 暖纸色
 *   slate        — 石板灰
 */

// ── 类型 ──────────────────────────────────────────

export interface ThemePackVars {
  bgPrimary: string
  bgSecondary: string
  bgTertiary: string
  textPrimary: string
  textSecondary: string
  textTertiary: string
  accent: string
  accentHover: string
  accentSoft: string
  accentGlow: string
  accentSecondary: string
  separator: string
  border: string
}

export interface ThemePack {
  /** 唯一标识符。 */
  id: string
  /** 显示名称。 */
  label: string
  /** 简短描述。 */
  desc: string
  /** 亮色模式变量。 */
  light: ThemePackVars
  /** 暗色模式变量。 */
  dark: ThemePackVars
}

// ── 默认值（professional — 品牌靛蓝） ─────────────

const PROFESSIONAL: ThemePack = {
  id: 'professional',
  label: '专业',
  desc: 'Mady 品牌靛蓝，默认配色（像素级对齐设计规范）',
  light: {
    bgPrimary: '#F5F2EE',
    bgSecondary: '#FAF8F5',
    bgTertiary: '#FFFFFF',
    textPrimary: '#1A1814',
    textSecondary: '#6B6560',
    textTertiary: '#A39E98',
    accent: '#5856d6',
    accentHover: '#4b4ac4',
    accentSoft: 'rgba(88, 86, 214, 0.08)',
    accentGlow: 'rgba(88, 86, 214, 0.18)',
    accentSecondary: '#f56600',
    separator: 'rgba(0, 0, 0, 0.08)',
    border: 'rgba(0, 0, 0, 0.08)',
  },
  dark: {
    bgPrimary: '#1C1A18',
    bgSecondary: '#1A1816',
    bgTertiary: '#252320',
    textPrimary: '#F0EDE8',
    textSecondary: '#8A8580',
    textTertiary: '#6B6560',
    accent: '#4b4ac4',
    accentHover: '#6e6cf0',
    accentSoft: 'rgba(94, 92, 230, 0.12)',
    accentGlow: 'rgba(94, 92, 230, 0.22)',
    accentSecondary: '#f56600',
    separator: 'rgba(255, 255, 255, 0.08)',
    border: 'rgba(255, 255, 255, 0.08)',
  },
}

const FOCUS_BLUE: ThemePack = {
  id: 'focus-blue',
  label: '专注蓝',
  desc: '蓝色主调，适合深度阅读与撰写',
  light: {
    bgPrimary: '#ffffff',
    bgSecondary: '#f0f4ff',
    bgTertiary: '#ffffff',
    textPrimary: '#000000',
    textSecondary: 'rgba(0, 0, 0, 0.55)',
    textTertiary: 'rgba(0, 0, 0, 0.25)',
    accent: '#007aff',
    accentHover: '#0062cc',
    accentSoft: 'rgba(0, 122, 255, 0.08)',
    accentGlow: 'rgba(0, 122, 255, 0.18)',
    accentSecondary: '#5856d6',
    separator: 'rgba(0, 0, 0, 0.10)',
    border: 'rgba(0, 0, 0, 0.10)',
  },
  dark: {
    bgPrimary: '#0b0b0f',
    bgSecondary: '#16161f',
    bgTertiary: '#1e1e2e',
    textPrimary: '#ffffff',
    textSecondary: 'rgba(255, 255, 255, 0.55)',
    textTertiary: 'rgba(255, 255, 255, 0.25)',
    accent: '#0a84ff',
    accentHover: '#409cff',
    accentSoft: 'rgba(10, 132, 255, 0.12)',
    accentGlow: 'rgba(10, 132, 255, 0.22)',
    accentSecondary: '#5e5ce6',
    separator: 'rgba(255, 255, 255, 0.10)',
    border: 'rgba(255, 255, 255, 0.10)',
  },
}

const PAPER_WARM: ThemePack = {
  id: 'paper-warm',
  label: '暖纸',
  desc: '暖色调，舒适阅读体验',
  light: {
    bgPrimary: '#faf8f5',
    bgSecondary: '#f0ece4',
    bgTertiary: '#fffdf9',
    textPrimary: '#2c2416',
    textSecondary: 'rgba(44, 36, 22, 0.55)',
    textTertiary: 'rgba(44, 36, 22, 0.25)',
    accent: '#b45309',
    accentHover: '#92400e',
    accentSoft: 'rgba(180, 83, 9, 0.08)',
    accentGlow: 'rgba(180, 83, 9, 0.18)',
    accentSecondary: '#5856d6',
    separator: 'rgba(44, 36, 22, 0.10)',
    border: 'rgba(44, 36, 22, 0.12)',
  },
  dark: {
    bgPrimary: '#1c1917',
    bgSecondary: '#292524',
    bgTertiary: '#3f3a36',
    textPrimary: '#f5f0eb',
    textSecondary: 'rgba(245, 240, 235, 0.55)',
    textTertiary: 'rgba(245, 240, 235, 0.25)',
    accent: '#d97706',
    accentHover: '#f59e0b',
    accentSoft: 'rgba(217, 119, 6, 0.12)',
    accentGlow: 'rgba(217, 119, 6, 0.22)',
    accentSecondary: '#5e5ce6',
    separator: 'rgba(245, 240, 235, 0.10)',
    border: 'rgba(245, 240, 235, 0.10)',
  },
}

const SLATE: ThemePack = {
  id: 'slate',
  label: '石板',
  desc: '低对比度中性色，极简风格',
  light: {
    bgPrimary: '#f8fafc',
    bgSecondary: '#f1f5f9',
    bgTertiary: '#ffffff',
    textPrimary: '#0f172a',
    textSecondary: 'rgba(15, 23, 42, 0.55)',
    textTertiary: 'rgba(15, 23, 42, 0.25)',
    accent: '#475569',
    accentHover: '#334155',
    accentSoft: 'rgba(71, 85, 105, 0.08)',
    accentGlow: 'rgba(71, 85, 105, 0.18)',
    accentSecondary: '#6366f1',
    separator: 'rgba(15, 23, 42, 0.08)',
    border: 'rgba(15, 23, 42, 0.10)',
  },
  dark: {
    bgPrimary: '#0f172a',
    bgSecondary: '#1e293b',
    bgTertiary: '#334155',
    textPrimary: '#f1f5f9',
    textSecondary: 'rgba(241, 245, 249, 0.55)',
    textTertiary: 'rgba(241, 245, 249, 0.25)',
    accent: '#94a3b8',
    accentHover: '#cbd5e1',
    accentSoft: 'rgba(148, 163, 184, 0.12)',
    accentGlow: 'rgba(148, 163, 184, 0.22)',
    accentSecondary: '#818cf8',
    separator: 'rgba(241, 245, 249, 0.08)',
    border: 'rgba(241, 245, 249, 0.10)',
  },
}

// ── 注册表 ────────────────────────────────────────

export const THEME_PACKS: ThemePack[] = [
  PROFESSIONAL,
  FOCUS_BLUE,
  PAPER_WARM,
  SLATE,
]

/** 按 ID 查找主题包。 */
export function getThemePack(id: string): ThemePack {
  return THEME_PACKS.find((p) => p.id === id) ?? PROFESSIONAL
}

// ── CSS 变量注入 ──────────────────────────────────

/** 将主题包变量转换为 CSS 自定义属性字符串。 */
export function buildThemePackCSS(pack: ThemePack, isDark: boolean): string {
  const vars = isDark ? pack.dark : pack.light
  return [
    `--color-mady-bg-primary: ${vars.bgPrimary};`,
    `--color-mady-bg-secondary: ${vars.bgSecondary};`,
    `--color-mady-bg-tertiary: ${vars.bgTertiary};`,
    `--color-mady-text-primary: ${vars.textPrimary};`,
    `--color-mady-text-secondary: ${vars.textSecondary};`,
    `--color-mady-text-tertiary: ${vars.textTertiary};`,
    `--color-mady-accent: ${vars.accent};`,
    `--color-mady-accent-hover: ${vars.accentHover};`,
    `--color-mady-accent-soft: ${vars.accentSoft};`,
    `--color-mady-accent-glow: ${vars.accentGlow};`,
    `--color-mady-accent-secondary: ${vars.accentSecondary};`,
    `--color-mady-separator: ${vars.separator};`,
    `--color-mady-border: ${vars.border};`,
  ].join('\n')
}
