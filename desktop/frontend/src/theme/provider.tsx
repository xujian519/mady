/**
 * ThemeProvider — 主题上下文提供者。
 *
 * 支持三种模式：
 *   - light: 强制亮色
 *   - dark: 强制暗色
 *   - system: 跟随系统偏好（通过 matchMedia 监听）
 *
 * 用法：
 * ```tsx
 * <ThemeProvider>
 *   <App />
 * </ThemeProvider>
 * ```
 */

import React, { useEffect, useMemo, useState, useCallback } from 'react'
import { ThemeContext, type ThemeMode } from './tokens'

// ── localStorage Key ──────────────────────────────

const STORAGE_KEY = 'mady-theme-mode'

// ── 工具函数 ──────────────────────────────────────

/** 从 localStorage 读取保存的主题模式，无则返回 system。 */
function loadMode(): ThemeMode {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch {
    // localStorage 不可用（SSR/隐私模式）
  }
  return 'system'
}

/** 保存主题模式到 localStorage。 */
function saveMode(mode: ThemeMode): void {
  try {
    localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    // 静默失败
  }
}

/** 查询系统深色模式偏好。 */
function querySystemDark(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

/** 根据 mode 和系统偏好解析实际主题。 */
function resolveMode(mode: ThemeMode): boolean {
  if (mode === 'dark') return true
  if (mode === 'light') return false
  return querySystemDark()
}

// ── Provider ──────────────────────────────────────

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => loadMode())

  // 保存到 localStorage
  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m)
    saveMode(m)
  }, [])

  // 解析后的暗色标志
  const [isDark, setIsDark] = useState(() => resolveMode(mode))

  useEffect(() => {
    // 当前解析结果
    const update = () => setIsDark(resolveMode(mode))

    // 如果是 system 模式，监听系统偏好变化
    if (mode === 'system') {
      const mq = window.matchMedia('(prefers-color-scheme: dark)')
      mq.addEventListener('change', update)
      update()
      return () => mq.removeEventListener('change', update)
    }

    update()
  }, [mode])

  // 将 resolved 主题写入 <html> 的 class 和 data-theme 属性
  useEffect(() => {
    const root = document.documentElement
    if (isDark) {
      root.classList.add('dark')
      root.setAttribute('data-theme', 'dark')
    } else {
      root.classList.remove('dark')
      root.setAttribute('data-theme', 'light')
    }
  }, [isDark])

  const value = useMemo(
    () => ({
      mode,
      setMode,
      resolved: isDark ? 'dark' as const : 'light' as const,
      isDark,
    }),
    [mode, setMode, isDark],
  )

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  )
}
