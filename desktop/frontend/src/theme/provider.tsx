/**
 * ThemeProvider — 主题上下文提供者。
 *
 * 支持三种模式 + 主题包：
 *   - 模式: light / dark / system（跟随系统）
 *   - 主题包: professional / focus-blue / paper-warm / slate
 *
 * 主题包通过动态 `<style id="mady-theme-pack">` 注入 CSS 变量覆盖。
 */

import React, { useEffect, useMemo, useState, useCallback, useRef } from 'react'
import { ThemeContext, type ThemeMode, type ThemePackId } from './tokens'
import { getThemePack, buildThemePackCSS } from './packs'

// ── localStorage Keys ─────────────────────────────

const MODE_KEY = 'mady-theme-mode'
const PACK_KEY = 'mady-theme-pack'

// ── 工具函数 ──────────────────────────────────────

function loadMode(): ThemeMode {
  try {
    const raw = localStorage.getItem(MODE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch { /* ignore */ }
  return 'system'
}

function saveMode(mode: ThemeMode): void {
  try { localStorage.setItem(MODE_KEY, mode) } catch { /* ignore */ }
}

function loadPack(): ThemePackId {
  try {
    const raw = localStorage.getItem(PACK_KEY)
    if (raw === 'professional' || raw === 'focus-blue' || raw === 'paper-warm' || raw === 'slate') {
      return raw as ThemePackId
    }
  } catch { /* ignore */ }
  return 'professional'
}

function savePack(pack: ThemePackId): void {
  try { localStorage.setItem(PACK_KEY, pack) } catch { /* ignore */ }
}

function querySystemDark(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolveMode(mode: ThemeMode): boolean {
  if (mode === 'dark') return true
  if (mode === 'light') return false
  return querySystemDark()
}

// ── CSS 变量注入 ──────────────────────────────────

/**
 * 在 `<head>` 中维护一个 `<style id="mady-theme-pack">` 元素，
 * 用于运行时覆盖 Tailwind CSS 变量。
 */
function applyThemePackCSS(packId: ThemePackId, isDark: boolean): void {
  const pack = getThemePack(packId)
  const css = `:root {\n${buildThemePackCSS(pack, isDark)}\n}`

  let style = document.getElementById('mady-theme-pack') as HTMLStyleElement | null
  if (!style) {
    style = document.createElement('style')
    style.id = 'mady-theme-pack'
    document.head.appendChild(style)
  }
  style.textContent = css
}

// ── Provider ──────────────────────────────────────

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(() => loadMode())
  const [packId, setPackId] = useState<ThemePackId>(() => loadPack())

  // 保存到 localStorage
  const setMode = useCallback((m: ThemeMode) => {
    setModeState(m)
    saveMode(m)
  }, [])

  const setThemePack = useCallback((p: ThemePackId) => {
    setPackId(p)
    savePack(p)
  }, [])

  // 暗色标志解析
  const [isDark, setIsDark] = useState(() => resolveMode(mode))

  useEffect(() => {
    const update = () => setIsDark(resolveMode(mode))
    if (mode === 'system') {
      const mq = window.matchMedia('(prefers-color-scheme: dark)')
      mq.addEventListener('change', update)
      update()
      return () => mq.removeEventListener('change', update)
    }
    update()
  }, [mode])

  // 写入 <html> class + data-theme
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

  // 应用主题包 CSS 变量覆盖
  const prevPackRef = useRef(packId)
  const prevDarkRef = useRef(isDark)
  useEffect(() => {
    if (packId === prevPackRef.current && isDark === prevDarkRef.current) return
    prevPackRef.current = packId
    prevDarkRef.current = isDark
    applyThemePackCSS(packId, isDark)
  }, [packId, isDark])

  // 首次挂载时始终应用（可能和上次会话不同）
  useEffect(() => {
    applyThemePackCSS(packId, isDark)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const value = useMemo(
    () => ({
      mode,
      setMode,
      resolved: isDark ? 'dark' as const : 'light' as const,
      isDark,
      themePack: packId,
      setThemePack,
    }),
    [mode, setMode, isDark, packId, setThemePack],
  )

  return (
    <ThemeContext.Provider value={value}>
      {children}
    </ThemeContext.Provider>
  )
}
