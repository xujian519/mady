/**
 * Composer 的本地 UI 命令拦截。
 *
 * 从 Composer.tsx（M61）拆出：这些命令（主题/布局/设置/导出/清空）
 * 全部在前端处理，不发送到 Agent。输入未命中时返回 false，交由调用方发送。
 */

import { useChatStore, initialState } from '@/stores/chat'
import { useSettingsStore } from '@/stores/settings'
import { exportSession, downloadSession, generateExportFilename } from '@/lib/sessionExport'
import type { ThemeMode } from '@/theme/tokens'

/**
 * 检测并执行本地 UI 命令。
 *
 * @param input 用户输入原文
 * @param setMode 明暗模式切换（来自 useTheme，便于调用方注入）
 * @returns true 表示已就地处理，调用方不应再发送
 */
export function tryLocalCommand(input: string, setMode: (mode: ThemeMode) => void): boolean {
  // /theme <light|dark|system|packName>
  const themeMatch = input.match(/^\/theme\s+(.+)$/)
  if (themeMatch) {
    const name = themeMatch[1].toLowerCase()
    // M-16：light/dark/system 是明暗模式切换（SlashCommandMenu usage 语义），
    // 其余值按主题包名匹配（professional/focus-blue/paper-warm/slate）。
    if (name === 'light' || name === 'dark' || name === 'system') {
      setMode(name)
      return true
    }
    // 通过 CustomEvent 通信给 App 层（SettingsPanel 监听）
    window.dispatchEvent(new CustomEvent('mady:set-theme-pack', { detail: name }))
    return true
  }
  // /layout <mode>
  const layoutMatch = input.match(/^\/layout\s+(.+)$/)
  if (layoutMatch) {
    const mode = layoutMatch[1].toLowerCase()
    if (mode === 'focus' || mode === 'standard') {
      useSettingsStore.getState().update({ layout: mode as 'standard' | 'focus' })
    }
    return true
  }
  // /settings
  if (input === '/settings') {
    window.dispatchEvent(new CustomEvent('mady:open-settings'))
    return true
  }
  // /export [format]
  const exportMatch = input.match(/^\/export\s+(.+)$/) || (input === '/export' ? ['', 'markdown'] : null)
  if (exportMatch) {
    const fmt = exportMatch[1] === 'json' ? 'json' as const : 'markdown' as const
    const messages = useChatStore.getState().messages
    if (messages.length === 0) return true
    const content = exportSession(messages, { format: fmt, title: '会话导出' })
    const filename = generateExportFilename(fmt)
    downloadSession(content, filename, fmt)
    return true
  }
  // /clear
  if (input === '/clear' || input === '/new') {
    useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
    return true
  }
  return false
}
