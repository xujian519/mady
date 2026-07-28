/**
 * SlashCommandMenu — 斜杠命令菜单。
 *
 * 用户在 Composer 输入 "/" 时触发，支持：
 * - UI 命令（前端拦截，不发送到 Agent）
 * - 领域命令（发送到 Agent 处理）
 * - 模糊搜索筛选
 *
 * 参考 TUI slash_registry.go 的命令分类体系。
 */

import React, { useRef, useEffect, useMemo, useState } from 'react'

// ── 命令定义 ──────────────────────────────────────

export interface SlashCommand {
  /** 命令名称（不含 "/"） */
  name: string
  /** 描述 */
  desc: string
  /** 分类: "ui" | "domain" | "session" | "mode" */
  category: string
  /** 在前端直接处理，不发送到 Agent */
  local: boolean
  /** 处理函数（local=true 时必填） */
  handler?: () => void
  /** 参数提示 */
  usage?: string
}

// ── 内建命令注册 ──────────────────────────────────

const BUILTIN_COMMANDS: SlashCommand[] = [
  // UI 命令（前端拦截）
  { name: 'clear',       desc: '清空当前对话',       category: 'session', local: true },
  { name: 'settings',    desc: '打开设置面板',       category: 'ui',      local: true },
  { name: 'theme',       desc: '切换主题风格',       category: 'ui',      local: true, usage: '/theme <light|dark>' },
  { name: 'layout',      desc: '切换布局模式',       category: 'ui',      local: true, usage: '/layout <standard|focus>' },

  // 领域命令（发送到 Agent）
  { name: 'patent',      desc: '专利分析帮助',       category: 'domain',  local: false },
  { name: 'novelty',     desc: '新颖性/创造性分析',   category: 'domain',  local: false, usage: '/novelty <发明描述>' },
  { name: 'oa',          desc: '审查意见答复起草',     category: 'domain',  local: false, usage: '/oa <通知书文本>' },
  { name: 'invalidation',desc: '专利无效宣告分析',    category: 'domain',  local: false, usage: '/invalidation <权利要求>' },
  { name: 'infringement',desc: '专利侵权比对分析',    category: 'domain',  local: false, usage: '/infringement <权利要求> | <方案>' },
  { name: 'reexamination',desc: '驳回复审请求书起草', category: 'domain',  local: false, usage: '/reexamination <驳回决定>' },
  { name: 'disclosure',  desc: '技术交底书分析',      category: 'domain',  local: false },
  { name: 'draft',       desc: '专利文档撰写',        category: 'domain',  local: false },

  // 模式命令
  { name: 'plan',        desc: '切换计划模式',       category: 'mode',    local: false, usage: '/plan [on|off]' },

  // 会话命令
  { name: 'export',      desc: '导出对话为 Markdown', category: 'session', local: false },
  { name: 'help',        desc: '显示帮助信息',       category: 'ui',      local: true },
]

// ── 分类元数据 ────────────────────────────────────

const CATEGORY_LABELS: Record<string, string> = {
  ui:      '界面操作',
  domain:  '专利分析',
  mode:    '推理模式',
  session: '会话管理',
}

const CATEGORY_COLORS: Record<string, string> = {
  ui:      'text-mady-info',
  domain:  'text-mady-success',
  mode:    'text-mady-accent',
  session: 'text-mady-warning',
}

// ── 组件 ──────────────────────────────────────────

interface SlashCommandMenuProps {
  /** 当前输入的 "/" 后的文本 */
  query: string
  /** 选中后回调 */
  onSelect: (cmd: SlashCommand) => void
  /** 关闭菜单 */
  onClose: () => void
}

export const SlashCommandMenu: React.FC<SlashCommandMenuProps> = ({
  query,
  onSelect,
  onClose,
}) => {
  const listRef = useRef<HTMLDivElement>(null)
  const [selectedIndex, setSelectedIndex] = useState(0)

  // 过滤命令
  const filtered = useMemo(() => {
    const q = query.toLowerCase()
    if (!q) return BUILTIN_COMMANDS
    return BUILTIN_COMMANDS.filter(
      (c) => c.name.toLowerCase().includes(q) || c.desc.toLowerCase().includes(q),
    )
  }, [query])

  // 当前选中索引调整
  useEffect(() => {
    setSelectedIndex(0)
  }, [query])

  // 键盘导航
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      switch (e.key) {
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
          if (filtered[selectedIndex]) {
            onSelect(filtered[selectedIndex])
          }
          break
        case 'Escape':
          e.preventDefault()
          onClose()
          break
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [filtered, selectedIndex, onSelect, onClose])

  // 滚动可见
  useEffect(() => {
    const el = listRef.current?.children[selectedIndex] as HTMLElement | undefined
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  if (filtered.length === 0) return null

  // 按分类分组
  const grouped = useMemo(() => {
    const map = new Map<string, SlashCommand[]>()
    for (const cmd of filtered) {
      const group = map.get(cmd.category) ?? []
      group.push(cmd)
      map.set(cmd.category, group)
    }
    return Array.from(map.entries())
  }, [filtered])

  // 构建扁平索引用于 selectedIndex 导航
  const flatList = useMemo(() => filtered, [filtered])

  return (
    <div className="mb-2 rounded-xl border border-mady-border mady-material shadow-mady-popover overflow-hidden max-h-72">
      <div ref={listRef} className="overflow-y-auto py-1">
        {grouped.map(([category, cmds]) => (
          <div key={category}>
            {/* 分类标题 */}
            <div className="px-3 py-1 text-mady-caption text-mady-text-tertiary uppercase tracking-wider">
              {CATEGORY_LABELS[category] ?? category}
            </div>
            {cmds.map((cmd) => {
              const idx = flatList.indexOf(cmd)
              const isSelected = idx === selectedIndex
              return (
                <button
                  key={cmd.name}
                  onClick={() => onSelect(cmd)}
                  onMouseEnter={() => setSelectedIndex(idx)}
                  className={`
                    w-full flex items-center gap-3 px-3 py-1.5 text-left transition-colors
                    ${isSelected ? 'bg-mady-accent/10' : 'hover:bg-mady-bg-primary'}
                  `}
                >
                  {/* 命令名 */}
                  <span className="font-mono text-mady-body text-mady-accent shrink-0">
                    /{cmd.name}
                  </span>
                  {/* 描述 */}
                  <span className="text-mady-body text-mady-text-secondary truncate flex-1">
                    {cmd.desc}
                  </span>
                  {/* 分类标签 */}
                  <span className={`text-mady-caption shrink-0 ${CATEGORY_COLORS[cmd.category] ?? ''}`}>
                    {cmd.local ? '本地' : 'Agent'}
                  </span>
                  {/* 斜杠快捷键提示 */}
                  <kbd className="hidden sm:inline text-mady-caption text-mady-text-tertiary border border-mady-border rounded px-1">
                    ↵
                  </kbd>
                </button>
              )
            })}
          </div>
        ))}
      </div>
    </div>
  )
}
