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
import { useTheme } from '@/theme/tokens'

// ── 常量 ──────────────────────────────────────────

/** 长粘贴检测阈值（字符数） */
const LONG_PASTE_THRESHOLD = 2000
/** 长粘贴检测阈值（行数） */
const LONG_PASTE_LINES = 20
/** 输入框最大高度 */
const MAX_INPUT_HEIGHT = 200
/** 发送历史最大条数（阶段 2.5，↑↓ 导航） */
const MAX_SEND_HISTORY = 50

// ── 会话草稿 / 发送历史（阶段 2.5） ───────────────

/** 草稿 localStorage key（按会话隔离）。 */
function draftKey(threadId: string | null): string {
  return `mady-composer-draft-${threadId || 'default'}`
}

/** 发送历史 localStorage key（按会话隔离）。 */
function historyKey(threadId: string | null): string {
  return `mady-composer-history-${threadId || 'default'}`
}

/** 读取发送历史（旧 → 新，Index 0 为最早）。 */
function loadSendHistory(threadId: string | null): string[] {
  try {
    const raw = localStorage.getItem(historyKey(threadId))
    const parsed = raw ? (JSON.parse(raw) as unknown) : []
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

/** 写入发送历史（新消息在前，去重相邻，上限 MAX_SEND_HISTORY）。 */
function saveSendHistory(threadId: string | null, history: string[]): void {
  try {
    localStorage.setItem(historyKey(threadId), JSON.stringify(history.slice(0, MAX_SEND_HISTORY)))
  } catch {
    // localStorage 不可用（隐私模式等）时静默
  }
}

// ── 组件 ──────────────────────────────────────────

export const Composer: React.FC = () => {
  const [text, setText] = useState('')
  const [showCommands, setShowCommands] = useState(false)
  const [slashQuery, setSlashQuery] = useState('')
  const [isComposing, setIsComposing] = useState(false)
  const [longPasteDetected, setLongPasteDetected] = useState(false)
  const running = useChatStore((s) => s.running)
  const threadId = useChatStore((s) => s.threadId)
  const sendMessage = useChatStore((s) => s.sendMessage)
  const { setMode } = useTheme()
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // ── 会话草稿 + 发送历史（阶段 2.5）：切换会话时恢复/保存 ──

  const sendHistoryRef = useRef<string[]>(loadSendHistory(threadId))
  const [historyIdx, setHistoryIdx] = useState(-1)
  const initTextRef = useRef('')

  useEffect(() => {
    // 切换会话：恢复该会话上次未发送的草稿 + 该会话的发送历史
    let saved = ''
    try {
      saved = localStorage.getItem(draftKey(threadId)) ?? ''
    } catch {
      // localStorage 不可用时忽略
    }
    setText(saved)
    setHistoryIdx(-1)
    initTextRef.current = ''
    sendHistoryRef.current = loadSendHistory(threadId)
  }, [threadId])

  useEffect(() => {
    // 输入变化时实时保存草稿（空串也存，覆盖旧草稿）
    try {
      localStorage.setItem(draftKey(threadId), text)
    } catch {
      // 忽略
    }
  }, [text, threadId])

  // ── 发送历史（阶段 2.5）：↑ 回看已发送，↓ 前进 ──

  const navigateHistory = (dir: 1 | -1) => {
    const hist = sendHistoryRef.current
    if (hist.length === 0) return

    if (historyIdx === -1 && dir === 1) {
      // ↑ 首次：记住当前输入，回退到最新一条已发送
      initTextRef.current = text
      setHistoryIdx(0)
      setText(hist[hist.length - 1])
      return
    }
    const next = historyIdx + dir
    if (next >= 0 && next < hist.length) {
      setHistoryIdx(next)
      setText(hist[hist.length - 1 - next])
    } else if (next < 0) {
      // ↓ 越过最新 → 恢复进入历史前的输入
      setHistoryIdx(-1)
      setText(initTextRef.current)
    }
  }

  const recordSent = (input: string) => {
    const hist = sendHistoryRef.current
    const deduped = hist[hist.length - 1] === input ? hist : [...hist, input]
    sendHistoryRef.current = deduped.slice(-MAX_SEND_HISTORY)
    saveSendHistory(threadId, sendHistoryRef.current)
    setHistoryIdx(-1)
    initTextRef.current = ''
  }

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
    recordSent(trimmed)
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

    // 阶段 2.5：输入区 ↑/↓ 导航发送历史（单行/空输入时触发）
    if (e.key === 'ArrowUp' && !e.shiftKey) {
      const el = e.currentTarget as HTMLTextAreaElement
      if (el.selectionStart === 0 && el.selectionEnd === el.value.length) {
        e.preventDefault()
        navigateHistory(1)
      }
    } else if (e.key === 'ArrowDown' && !e.shiftKey) {
      const el = e.currentTarget as HTMLTextAreaElement
      if (el.selectionStart === el.value.length && el.selectionEnd === el.value.length) {
        e.preventDefault()
        navigateHistory(-1)
      }
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
