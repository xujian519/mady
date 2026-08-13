/**
 * CommandPalette — ⌘K 全局命令面板。
 *
 * 规范 §12.4：560px 宽度、52px 搜索框、毛玻璃背景、模糊搜索。
 *
 * 聚合功能：
 *   - 导航（会话切换、侧栏切换）
 *   - 模板（使用模板）
 *   - 操作（设置、主题、清除上下文、导出）
 *   - 斜杠命令
 *
 * 快捷键：macOS Cmd+K / Windows Ctrl+K
 */

import React, { useRef, useEffect, useState, useMemo, useCallback } from 'react'
import { Search, Command } from 'lucide-react'
import type { PaletteCommand, CommandCategory } from '@/stores/commands'

// ── 分类元数据 ────────────────────────────────────

const CATEGORY_LABEL: Record<CommandCategory, string> = {
  navigation: '导航',
  template:   '文档模板',
  skill:      '技能',
  command:    '命令',
  action:     '操作',
}

const CATEGORY_ORDER: Record<CommandCategory, number> = {
  navigation: 1,
  command:    2,
  template:   3,
  action:     4,
  skill:      5,
}

// ── 组件 ──────────────────────────────────────────

export interface CommandPaletteProps {
  open: boolean
  onClose: () => void
  commands: PaletteCommand[]
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({ open, onClose, commands }) => {
  const inputRef = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)

  // 打开时聚焦搜索框、清空查询
  useEffect(() => {
    if (open) {
      setQuery('')
      setSelectedIndex(0)
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  // 过滤命令
  const filtered = useMemo(() => {
    if (!query.trim()) return commands
    const q = query.toLowerCase()
    return commands.filter((cmd) => {
      return (
        cmd.title.toLowerCase().includes(q) ||
        cmd.keywords.some((kw) => kw.toLowerCase().includes(q)) ||
        cmd.id.toLowerCase().includes(q)
      )
    })
  }, [query, commands])

  // 分组 + 排序
  const grouped = useMemo(() => {
    const groups = new Map<CommandCategory, PaletteCommand[]>()
    for (const cmd of filtered) {
      const list = groups.get(cmd.category) ?? []
      list.push(cmd)
      groups.set(cmd.category, list)
    }
    return [...groups.entries()].sort(
      (a, b) => (CATEGORY_ORDER[a[0]] ?? 99) - (CATEGORY_ORDER[b[0]] ?? 99),
    )
  }, [filtered])

  // 选中项随 filtered 变化回退
  useEffect(() => {
    if (selectedIndex >= filtered.length) {
      setSelectedIndex(Math.max(0, filtered.length - 1))
    }
  }, [filtered.length, selectedIndex])

  // 执行命令
  const executeSelected = useCallback(() => {
    const cmd = filtered[selectedIndex]
    if (cmd) {
      cmd.execute()
      onClose()
    }
  }, [filtered, selectedIndex, onClose])

  // 键盘导航
  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case 'Escape':
        e.preventDefault()
        onClose()
        break
      case 'ArrowDown':
        e.preventDefault()
        setSelectedIndex((i) => Math.min(i + 1, filtered.length - 1))
        break
      case 'ArrowUp':
        e.preventDefault()
        setSelectedIndex((i) => Math.max(i - 1, 0))
        break
      case 'Enter':
        e.preventDefault()
        executeSelected()
        break
    }
  }

  // 快捷键提示文本
  const shortcutHint = (cmd: PaletteCommand): string | null => {
    if (cmd.shortcut) return cmd.shortcut
    return null
  }

  if (!open) return null

  return (
    <>
      {/* 半透明遮罩 */}
      <div
        className="fixed inset-0 z-[90] bg-mady-bg-overlay transition-opacity duration-100"
        onClick={onClose}
      />

      {/* 面板 */}
      <div
        className="fixed top-[15%] left-1/2 -translate-x-1/2 z-[91] w-[560px] max-h-[400px] flex flex-col rounded-xl shadow-mady-modal border border-mady-border bg-mady-bg-tertiary/95 backdrop-blur-xl overflow-hidden"
        style={{ backdropFilter: 'blur(20px)' }}
      >
        {/* 搜索框 */}
        <div className="flex items-center gap-2 px-4 h-[52px] border-b border-mady-separator">
          <Search size={15} className="text-mady-text-tertiary shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => { setQuery(e.target.value); setSelectedIndex(0) }}
            onKeyDown={handleKeyDown}
            placeholder="搜索命令…"
            className="flex-1 bg-transparent text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none"
          />
          <kbd className="hidden sm:inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-mono bg-mady-bg-hover text-mady-text-tertiary border border-mady-border leading-none">
            <Command size={10} />K
          </kbd>
        </div>

        {/* 命令列表 */}
        <div className="flex-1 overflow-y-auto py-1">
          {grouped.length === 0 && (
            <div className="px-4 py-6 text-center text-mady-caption text-mady-text-tertiary">
              没有匹配的命令
            </div>
          )}

          {grouped.map(([category, cmds]) => (
            <div key={category}>
              <div className="px-4 py-1 text-mady-caption font-medium text-mady-text-tertiary tracking-wide uppercase">
                {CATEGORY_LABEL[category]}
              </div>
              {cmds.map((cmd) => {
                const idx = filtered.indexOf(cmd)
                return (
                  <button
                    key={cmd.id}
                    onClick={() => { cmd.execute(); onClose() }}
                    onMouseEnter={() => setSelectedIndex(idx)}
                    className={`
                      w-full flex items-center gap-3 px-4 h-9 text-left
                      transition-colors duration-75
                      ${idx === selectedIndex ? 'bg-mady-accent-soft' : 'hover:bg-mady-bg-hover'}
                    `}
                  >
                    <span className={`text-mady-ui ${idx === selectedIndex ? 'text-mady-accent' : 'text-mady-text-primary'}`}>
                      {cmd.title}
                    </span>
                    <span className="flex-1" />
                    {shortcutHint(cmd) && (
                      <kbd className="text-mady-caption font-mono text-mady-text-tertiary">
                        {shortcutHint(cmd)}
                      </kbd>
                    )}
                  </button>
                )
              })}
            </div>
          ))}
        </div>
      </div>
    </>
  )
}
