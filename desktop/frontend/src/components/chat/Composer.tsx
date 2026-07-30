/**
 * Composer — 聊天输入组件（Reasonix 增强版）。
 *
 * 功能：
 * - 多行文本输入（Enter 发送，Shift+Enter 换行）
 * - 斜杠命令菜单（/ 触发，模糊搜索 + 分类分组）
 * - UI 命令前端拦截：/clear, /settings, /theme, /help
 * - 领域命令发送到 Agent：/patent, /novelty, /oa 等
 * - 空输入禁用 / 运行时禁用
 * - 自动聚焦 / 失焦
 * - IME 组合感知（CJK 输入法组合期不触发快捷键）
 * - 长粘贴检测
 */

import React, { useState, useRef, useEffect, useCallback } from 'react'
import { useChatStore, initialState } from '@/stores/chat'
import { useSettingsStore } from '@/stores/settings'
import { Send, Sparkles } from 'lucide-react'
import { SlashCommandMenu, type SlashCommand } from './SlashCommandMenu'
import { exportSession, downloadSession, generateExportFilename } from '@/lib/sessionExport'

// ── 常量 ──────────────────────────────────────────

/** 长粘贴检测阈值（字符数） */
const LONG_PASTE_THRESHOLD = 2000
/** 长粘贴检测阈值（行数） */
const LONG_PASTE_LINES = 20
/** 输入框最大高度 */
const MAX_INPUT_HEIGHT = 200

// ── 组件 ──────────────────────────────────────────

export const Composer: React.FC = () => {
  const [text, setText] = useState('')
  const [showCommands, setShowCommands] = useState(false)
  const [slashQuery, setSlashQuery] = useState('')
  const [isComposing, setIsComposing] = useState(false)
  const [longPasteDetected, setLongPasteDetected] = useState(false)
  const running = useChatStore((s) => s.running)
  const sendMessage = useChatStore((s) => s.sendMessage)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // 自动聚焦
  useEffect(() => {
    if (!running && inputRef.current) {
      inputRef.current.focus()
    }
  }, [running])

  // ── 斜杠命令状态管理 ────────────────────────────

  // 当输入的文本变化时检测斜杠命令状态
  useEffect(() => {
    if (text.startsWith('/') && !text.includes(' ')) {
      setShowCommands(true)
      setSlashQuery(text.slice(1))
    } else if (text.startsWith('/') && text.includes(' ') && !showCommands) {
      // 已经有参数，不显示命令菜单
      setShowCommands(false)
    } else if (!text.startsWith('/')) {
      setShowCommands(false)
    }
    // 当用户继续输入但菜单打开时，更新查询
    if (showCommands) {
      setSlashQuery(text.startsWith('/') ? text.slice(1) : '')
    }
  }, [text, showCommands])

  // ── 命令处理 ────────────────────────────────────

  const handleCommandSelect = useCallback((cmd: SlashCommand) => {
    setShowCommands(false)

    if (cmd.local) {
      // UI 命令 — 前端直接处理
      switch (cmd.name) {
        case 'clear':
          useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
          break
        case 'settings':
          // 触发设置面板（由父组件接收）
          window.dispatchEvent(new CustomEvent('mady:open-settings'))
          break
        case 'theme':
          setText(`/theme `)
          inputRef.current?.focus()
          return
        case 'help':
          break
      }
      setText('')
    } else {
      // 领域命令 — 插入文本 + 发送文本到 Agent
      setText(`/${cmd.name} `)
      inputRef.current?.focus()
    }

    inputRef.current?.focus()
  }, [])

  const handleCloseCommands = useCallback(() => {
    setShowCommands(false)
  }, [])

  // ── 本地命令检测 ────────────────────────────────

  /** 检测并执行本地 UI 命令。返回 true 表示已处理。 */
  const tryLocalCommand = (input: string): boolean => {
    // /theme <name>
    const themeMatch = input.match(/^\/theme\s+(.+)$/)
    if (themeMatch) {
      const name = themeMatch[1].toLowerCase()
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

  // ── 发送 ────────────────────────────────────────

  const handleSend = () => {
    const trimmed = text.trim()
    if (!trimmed || running) return

    // 如果斜杠命令菜单打开且用户按 Enter，不要发送（菜单会拦截 Enter）
    if (showCommands) return

    // 检测本地 UI 命令
    if (tryLocalCommand(trimmed)) {
      setText('')
      return
    }

    setText('')
    setLongPasteDetected(false)
    sendMessage(trimmed)
  }

  // ── 键盘事件 ────────────────────────────────────

  const handleKeyDown = (e: React.KeyboardEvent) => {
    // IME 组合期间不处理快捷键
    if (isComposing) return

    if (e.key === 'Enter' && !e.shiftKey) {
      // 菜单打开时，Enter 由菜单处理
      if (showCommands) return
      e.preventDefault()
      handleSend()
    }

    // Esc 关闭命令菜单
    if (e.key === 'Escape' && showCommands) {
      e.preventDefault()
      setShowCommands(false)
      return
    }

    // 命令菜单打开时，上/下键由菜单处理
    if ((e.key === 'ArrowUp' || e.key === 'ArrowDown') && showCommands) {
      e.preventDefault()
      return
    }
  }

  // ── 输入事件 ────────────────────────────────────

  const handleInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value
    setText(value)

    // 自动调整高度
    const el = e.target
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, MAX_INPUT_HEIGHT)}px`

    // 长粘贴检测
    if (value.length > LONG_PASTE_THRESHOLD) {
      const lines = value.split('\n').length
      setLongPasteDetected(lines > LONG_PASTE_LINES)
    } else {
      setLongPasteDetected(false)
    }
  }

  // ── 粘贴事件（简化长粘贴） ──────────────────────

  const handlePaste = (e: React.ClipboardEvent) => {
    const pasted = e.clipboardData.getData('text')
    if (pasted.length > LONG_PASTE_THRESHOLD || pasted.split('\n').length > LONG_PASTE_LINES) {
      // 不阻止粘贴，但标记为长粘贴（UI 展示）
      setTimeout(() => {
        setLongPasteDetected(true)
      }, 0)
    }
  }

  const canSend = text.trim().length > 0 && !running

  return (
    <div className="border-t border-mady-separator bg-mady-bg-primary px-4 py-3">
      <div className="max-w-3xl mx-auto">
        {/* 斜杠命令菜单 */}
        {showCommands && (
          <SlashCommandMenu
            query={slashQuery}
            onSelect={handleCommandSelect}
            onClose={handleCloseCommands}
          />
        )}

        {/* 长粘贴提示 */}
        {longPasteDetected && (
          <div className="mb-2 px-3 py-1.5 rounded-lg bg-mady-warning/10 border border-mady-warning/20 text-mady-caption text-mady-warning">
            检测到大量文本粘贴，内容将被精简显示
          </div>
        )}

        {/* 输入区域 */}
        <div className="flex items-end gap-2">
          <div className="flex-1 relative group">
            <textarea
              ref={inputRef}
              value={text}
              onChange={handleInput}
              onKeyDown={handleKeyDown}
              onPaste={handlePaste}
              onCompositionStart={() => setIsComposing(true)}
              onCompositionEnd={() => setIsComposing(false)}
              placeholder={running ? '等待回复…' : '输入消息…（/ 打开命令菜单，Enter 发送，Shift+Enter 换行）'}
              disabled={running}
              rows={1}
              className={`
                w-full resize-none rounded-xl px-4 py-2.5 pr-10
                bg-mady-bg-secondary border border-mady-border
                text-mady-body text-mady-text-primary
                placeholder-mady-text-tertiary
                outline-none
                transition-all duration-200
                focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20
                focus:bg-mady-bg-primary
                disabled:opacity-50
                group-hover:border-mady-text-quaternary
              `}
            />
          </div>

          <button
            onClick={handleSend}
            disabled={!canSend}
            className={`
              shrink-0 w-9 h-9 rounded-xl flex items-center justify-center
              transition-all duration-200
              ${canSend
                ? 'bg-mady-accent text-white hover:bg-mady-accent-hover shadow-md hover:shadow-lg hover:scale-105 active:scale-95'
                : 'bg-mady-bg-secondary text-mady-text-tertiary border border-mady-border'
              }
              disabled:cursor-not-allowed
            `}
            title="发送"
          >
            {running ? (
              <Sparkles size={15} className="animate-pulse" />
            ) : (
              <Send size={15} />
            )}
          </button>
        </div>
      </div>
    </div>
  )
}
