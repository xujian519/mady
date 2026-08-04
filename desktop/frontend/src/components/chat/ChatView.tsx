/**
 * ChatView — 主聊天视图（三栏布局），支持虚拟化消息列表。
 *
 * 布局结构：
 * ┌─────────────────────────────────────────────────────┐
 * │ TitleBar（全宽标题栏，含交通灯安全区）              │
 * ├──────────┬──────────────────────────┬─────────────┤
 * │ Sidebar  │ Chat Main                │ Context Panel│
 * │ 会话/项目 │  消息流 / Composer        │ 文档预览      │
 * └──────────┴──────────────────────────┴─────────────┘
 * │ StatusBar                                           │
 * └─────────────────────────────────────────────────────┘
 *
 * 虚拟化策略：
 * - 已完成的所有 past-messages 参与虚拟化
 * - 当前轮次的流式内容（output/thinking/toolCalls）放在虚拟列表下方
 * - 智能自动滚动：仅在用户接近底部时跟随
 */

import React, { useRef, useEffect, useMemo, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useShallow } from 'zustand/react/shallow'
import { useChatStore, initialState } from '@/stores/chat'
import { useTabsStore } from '@/stores/tabs'
import { getThread } from '@/lib/backend'
import { TabBar } from '../TabBar'
import { useSettingsStore, type LayoutMode } from '@/stores/settings'
import type { Message, ToolCall, CompactionNotice, RetryNotice } from '@/stores/chat'
import { ReasoningBlock } from '../ReasoningBlock'
import { UsageStrip } from '../UsageStrip'
import { ContextWindowRing } from '../ContextWindowRing'
import { Sidebar } from '../Sidebar'
import { MessageBubble } from './MessageBubble'
import { DecisionSurface } from '../DecisionSurface'
import { ContextIndicator } from '../ContextIndicator'
import { StatusBar } from '../StatusBar'
import { ToolCard } from '../ToolCard'
import { useTheme } from '@/theme/tokens'
import { exportSession } from '@/lib/sessionExport'
import { listenToWailsEvent } from '@/lib/wails'
import { TodoDock } from '../TodoDock'
import { DocumentViewer, type DocViewerFile } from '../DocumentViewer'
import { FileViewerOverlay } from '../fileviewer/FileViewerOverlay'
import { SettingsPanel } from '../SettingsPanel'
import { KnowledgeView } from '../KnowledgeView'

/**
 * 将 Go 侧历史消息（agentcore.Message：role=user/assistant、无 timestamp 字段）
 * 映射为前端 Message（role 归一 + 时间戳缺省为 0），避免渲染出 NaN:NaN 与角色错位（M-14）。
 */
function mapHistoryMessage(m: Record<string, unknown>): Message {
  const id = typeof m.id === 'string' && m.id ? m.id : `hist-${Math.random().toString(36).slice(2)}`
  return {
    id,
    role: m.role === 'user' ? 'user' : 'agent',
    content: typeof m.content === 'string' ? m.content : '',
    timestamp: typeof m.timestamp === 'number' && Number.isFinite(m.timestamp) ? m.timestamp : 0,
    thinking: typeof m.thinking === 'string' ? m.thinking : undefined,
  }
}
import { TemplatesView } from '../TemplatesView'
import { SkillsView } from '../SkillsView'
import { McpView } from '../McpView'
import { CommandPalette } from './CommandPalette'
import { buildCommands } from '@/stores/commands'
import { Sparkles, PanelRightOpen, Database, FileText, Server, Zap, Loader2, RefreshCw, Scissors, ChevronDown, ChevronRight } from 'lucide-react'

// ── 虚拟列表项类型 ────────────────────────────────

/** 步骤名美化：turn_N → 推理第 N 步。 */
function formatStepName(step: string): string {
  const m = /^turn_(\d+)$/.exec(step)
  if (m) return `推理与工具调用进行中`
  return step
}

/** Token 数量格式化：12500 → 12.5k。 */
function formatTokens(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

type TranscriptItem =
  | { kind: 'message'; message: Message; index: number }
  // 阶段 2.4：回合组头（可折叠）。startIndex 为回合首条消息在 messages 中的索引，
  // roundId 为该回合首条消息 id，作为折叠状态的身份标识（索引漂移时状态不串位）。
  | { kind: 'round-header'; roundId: string; startIndex: number; count: number; collapsed: boolean }
  | { kind: 'streaming-output'; output: string }
  | { kind: 'streaming-thinking'; thinking: string }
  | { kind: 'tool-calls'; toolCalls: ToolCall[] }
  | { kind: 'step-indicator'; step: string; count: number }
  | { kind: 'retry-notice'; notice: RetryNotice }
  | { kind: 'compaction-notice'; notice: CompactionNotice }
  | { kind: 'error'; error: string }

/** 根据消息内容估算行数，用于动态高度预估。 */
function estimateLines(text: string): number {
  if (!text) return 1
  // 按 \n + 按 80 字符折行
  const newlines = text.split('\n').length
  const wrapLines = Math.ceil(text.length / 80)
  return Math.max(newlines, wrapLines)
}

/** 单项预估高度（px），用于虚拟化器的 estimateSize。 */
function itemHeight(item: TranscriptItem): number {
  switch (item.kind) {
    case 'round-header':
      return 32
    case 'message':
      // 头像 32px + 气泡 padding ~48px + 时间戳 ~20px + 内容
      return 80 + estimateLines(item.message.content) * 22
    case 'streaming-output':
      return 60 + estimateLines(item.output) * 22
    case 'streaming-thinking':
      return Math.min(200, 40 + estimateLines(item.thinking) * 22)
    case 'tool-calls':
      return item.toolCalls.length * 80 + 24
    case 'step-indicator':
    case 'retry-notice':
    case 'compaction-notice':
      return 36
    case 'error':
      return 60
  }
}

/** 是否有活动内容（非空列表）。 */
function hasActiveContent(
  messages: Message[], output: string, thinking: string,
  toolCalls: ToolCall[], error: string | null,
): boolean {
  return messages.length > 0 || output.length > 0 || thinking.length > 0 || toolCalls.length > 0 || !!error
}

// ── 智能自动滚动 Hook ─────────────────────────────

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function useAutoScroll(
  virtualizer: any,
  enabled: boolean,
  items: TranscriptItem[],
  scrollContainerRef: React.RefObject<HTMLDivElement | null>,
): void {
  const prevCountRef = useRef(items.length)
  const prevOutputLenRef = useRef(0)
  // 用户是否「贴底」：上翻阅读历史时暂停自动滚动，回到底部附近才恢复（F-I14）
  const stickToBottomRef = useRef(true)

  useEffect(() => {
    const el = scrollContainerRef.current
    if (!el) return
    const onScroll = () => {
      const { scrollTop, scrollHeight, clientHeight } = el
      stickToBottomRef.current = scrollHeight - scrollTop - clientHeight < 120
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [scrollContainerRef])

  useEffect(() => {
    if (!enabled || items.length === 0) return

    // 流式文本增量（item 数不变）也触发跟随：汇总 streaming item 的文本长度
    let outputLen = 0
    for (const it of items) {
      if (it.kind === 'streaming-output') outputLen += it.output.length
      if (it.kind === 'streaming-thinking') outputLen += it.thinking.length
    }
    const countChanged = items.length !== prevCountRef.current
    const outputChanged = outputLen !== prevOutputLenRef.current
    prevCountRef.current = items.length
    prevOutputLenRef.current = outputLen

    if (!stickToBottomRef.current) return // 用户上翻时保持不动
    if (!countChanged && !outputChanged) return

    virtualizer.scrollToIndex(items.length - 1, { align: 'end', behavior: 'auto' })
  }, [items, enabled, virtualizer])
}

// ── 主要组件 ──────────────────────────────────────

export const ChatView: React.FC = () => {
  // 高频变更
  const { messages, output, toolCalls } = useChatStore(
    useShallow((s) => ({ messages: s.messages, output: s.output, toolCalls: s.toolCalls })),
  )
  // 低频变更
  const error = useChatStore((s) => s.error)
  const running = useChatStore((s) => s.running)
  const thinking = useChatStore((s) => s.thinking)
  const threadId = useChatStore((s) => s.threadId)
  const currentStep = useChatStore((s) => s.currentStep)
  const stepCount = useChatStore((s) => s.stepCount)
  const compaction = useChatStore((s) => s.compaction)
  const retryNotice = useChatStore((s) => s.retryNotice)
  const layout = useSettingsStore((s) => s.layout as LayoutMode)

  const isFocusMode = layout === 'focus'
  // 阶段 2.1c：会话标签（TabBar 数据源）
  const tabsActiveId = useTabsStore((s) => s.activeTabId)
  const loadTabs = useTabsStore((s) => s.loadTabs)
  // 侧栏显隐与持久化设置联动（W4-T13 布局持久化：重启恢复折叠状态）
  const sidebarCollapsed = useSettingsStore((s) => s.sidebarCollapsed)
  const [showSidebar, setShowSidebar] = useState(
    () => !useSettingsStore.getState().sidebarCollapsed && !isFocusMode,
  )

  // 折叠状态变化时同步本地显隐（收起按钮在 Sidebar 内触发）
  useEffect(() => {
    if (sidebarCollapsed && showSidebar && !isFocusMode) {
      setShowSidebar(false)
    }
  }, [sidebarCollapsed, showSidebar, isFocusMode])

  // 窄窗口（<900px）自动折叠侧栏（W4-T8，对齐 HIG Sidebars：空间紧张时自动隐藏导航）
  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth < 900 && !isFocusMode) {
        useSettingsStore.setState({ sidebarCollapsed: true })
      }
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [isFocusMode])
  const [showDocViewer, setShowDocViewer] = useState(false)
  const [docFile, setDocFile] = useState<DocViewerFile | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [showKnowledge, setShowKnowledge] = useState(false)
  const [showTemplates, setShowTemplates] = useState(false)
  const [showSkills, setShowSkills] = useState(false)
  const [showMcp, setShowMcp] = useState(false)
  const [showCommandPalette, setShowCommandPalette] = useState(false)

  const scrollContainerRef = useRef<HTMLDivElement>(null)

  // 主题三态切换（F-I1）：命令面板与 SettingsPanel 走同一路径（ThemeProvider）
  const { setMode } = useTheme()

  // /settings 斜杠命令（F-I2）：Composer dispatch mady:open-settings 事件，
  // 由本组件（设置面板所有者）监听并打开。
  useEffect(() => {
    const handleOpenSettings = () => setShowSettings(true)
    window.addEventListener('mady:open-settings', handleOpenSettings)
    return () => window.removeEventListener('mady:open-settings', handleOpenSettings)
  }, [])

  // 原生菜单「设置」（⌘,）→ Wails 事件 app:open-settings（desktop/menu.go 触发）
  useEffect(() => {
    return listenToWailsEvent('app:open-settings', () => setShowSettings(true))
  }, [])

  // 阶段 2.1c：挂载时加载会话标签（TabBar 数据源；后端保证至少 1 个默认标签）
  useEffect(() => {
    void loadTabs()
  }, [loadTabs])

  // 阶段 2.1c：激活标签变化 → 同步聊天上下文（threadId + 历史加载 / 新会话清空）
  useEffect(() => {
    const activeTabId = useTabsStore.getState().activeTabId
    if (!activeTabId) return
    const tab = useTabsStore.getState().tabs.find((t) => t.id === activeTabId)
    if (!tab) return
    const targetThreadId = tab.threadId ?? ''
    const current = useChatStore.getState()
    if (current.threadId === targetThreadId) return // 已在目标会话，避免重复加载
    useChatStore.setState({ threadId: targetThreadId })
    if (!targetThreadId) {
      // 新标签（未关联会话）：清空聊天状态
      useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
      return
    }
    // 已有关联会话：加载历史（完成后校验仍在同一会话，防竞态）
    void getThread(targetThreadId)
      .then((snap) => {
        if (useChatStore.getState().threadId !== targetThreadId) return
        const history = (snap?.messages ?? []).map((m) => mapHistoryMessage(m as Record<string, unknown>))
        useChatStore.setState({ messages: history })
      })
      .catch(() => {
        /* 后端未就绪时静默 */
      })
  }, [tabsActiveId])

  // ⌘K 快捷键切换命令面板
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setShowCommandPalette((v) => !v)
        return
      }
      // ⌘B 切换侧栏折叠（W4-T8；仅在非输入元素时响应，避免与编辑区粗体冲突）
      if ((e.metaKey || e.ctrlKey) && e.key === 'b') {
        const target = e.target as HTMLElement | null
        const tag = target?.tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || target?.isContentEditable) return
        e.preventDefault()
        useSettingsStore.setState((s) => ({ sidebarCollapsed: !s.sidebarCollapsed }))
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [])

  const handleNewChat = () => {
    useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
  }

  const handleSettings = () => {
    setShowSettings(true)
  }

  const showContent = hasActiveContent(messages, output, thinking, toolCalls, error)

  // 阶段 2.4：折叠的回合（键为回合首条消息 id；折叠时仅显示组头）
  const [collapsedRounds, setCollapsedRounds] = useState<Set<string>>(new Set())
  const toggleRound = (roundId: string) => {
    setCollapsedRounds((prev) => {
      const next = new Set(prev)
      if (next.has(roundId)) next.delete(roundId)
      else next.add(roundId)
      return next
    })
  }

  // ── 构建虚拟化条目 ──────────────────────────────

  const items = useMemo<TranscriptItem[]>(() => {
    const result: TranscriptItem[] = []

    // 回合分组：user 消息（或首条消息）开启新回合；
    // 组内消息（agent 回复等）跟随到下一个 user 消息之前。
    const roundStartOf: number[] = new Array(messages.length)
    {
      let cur = -1
      for (let i = 0; i < messages.length; i++) {
        if (i === 0 || messages[i].role === 'user') cur = i
        roundStartOf[i] = cur
      }
    }
    // 每个回合的消息条数（一次遍历统计，避免逐回合 filter 的 O(n²)）
    const roundCount = new Map<number, number>()
    for (const s of roundStartOf) {
      roundCount.set(s, (roundCount.get(s) ?? 0) + 1)
    }
    // 每个回合组头只输出一次
    const emittedHeaders = new Set<number>()
    for (let i = 0; i < messages.length; i++) {
      const start = roundStartOf[i]
      const roundId = messages[start]?.id ?? ''
      if (!emittedHeaders.has(start)) {
        emittedHeaders.add(start)
        result.push({
          kind: 'round-header',
          roundId,
          startIndex: start,
          count: roundCount.get(start) ?? 0,
          collapsed: roundId !== '' && collapsedRounds.has(roundId),
        })
      }
      if (roundId !== '' && collapsedRounds.has(roundId)) continue // 折叠：跳过组内消息
      result.push({ kind: 'message', message: messages[i], index: i })
    }

    // 当前轮的流式 Agent 内容
    if (output) {
      result.push({ kind: 'streaming-output', output })
    }
    if (thinking && running) {
      result.push({ kind: 'streaming-thinking', thinking })
    }
    if (toolCalls.length > 0) {
      result.push({ kind: 'tool-calls', toolCalls })
    }
    // 运行状态指示：步骤进度 / 自动重试 / 上下文压缩
    if (running && currentStep) {
      result.push({ kind: 'step-indicator', step: currentStep, count: stepCount })
    }
    if (running && retryNotice) {
      result.push({ kind: 'retry-notice', notice: retryNotice })
    }
    if (compaction) {
      result.push({ kind: 'compaction-notice', notice: compaction })
    }
    if (error) {
      result.push({ kind: 'error', error })
    }

    return result
  }, [messages, output, thinking, running, toolCalls, error, currentStep, stepCount, compaction, retryNotice, collapsedRounds])

  // ── 虚拟化器 ────────────────────────────────────

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollContainerRef.current,
    estimateSize: (index) => itemHeight(items[index]),
    overscan: 8,
    paddingEnd: 8,
  })

  // 智能自动滚动
  useAutoScroll(virtualizer, running || !!output || !!thinking, items, scrollContainerRef)

  // ── 渲染单项 ────────────────────────────────────

  const renderItem = (item: TranscriptItem) => {
    switch (item.kind) {
      case 'round-header': {
        const first = messages[item.startIndex]
        const time = first?.timestamp
          ? new Date(first.timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
          : ''
        return (
          <div key={`round-${item.roundId}`} className="px-4 pt-2">
            <button
              onClick={() => toggleRound(item.roundId)}
              className="w-full flex items-center gap-2 text-mady-caption text-mady-text-tertiary hover:text-mady-text-secondary transition-colors duration-150 group"
              title={item.collapsed ? '展开该回合' : '折叠该回合'}
            >
              <span className="h-px flex-1 bg-mady-separator group-hover:bg-mady-border" />
              {item.collapsed ? <ChevronRight size={11} /> : <ChevronDown size={11} />}
              <span>
                回合 · {item.count} 条消息{time ? ` · ${time}` : ''}
                {item.collapsed && '（已折叠）'}
              </span>
              <span className="h-px flex-1 bg-mady-separator group-hover:bg-mady-border" />
            </button>
          </div>
        )
      }
      case 'message':
        return <MessageBubble key={item.message.id} message={item.message} />
      case 'streaming-output':
        return (
          <MessageBubble
            key="streaming"
            message={{
              id: 'streaming',
              role: 'agent',
              content: item.output,
              timestamp: Date.now(),
            }}
            isStreaming={running}
          />
        )
      case 'streaming-thinking':
        return (
          <ReasoningBlock
            key="thinking"
            content={item.thinking}
          />
        )
      case 'tool-calls':
        return (
          <div key="tool-calls" className="px-4 py-2 space-y-2">
            {item.toolCalls.map((tc) => (
              <ToolCard key={tc.id} toolCall={tc} />
            ))}
          </div>
        )
      case 'step-indicator':
        return (
          <div key="step-indicator" className="px-4 py-1.5">
            <div className="flex items-center gap-2 text-mady-caption text-mady-text-secondary">
              <Loader2 size={12} className="animate-spin text-mady-accent" />
              <span>
                {formatStepName(item.step)}
                {item.count > 1 && ` · 第 ${item.count} 步`}
              </span>
            </div>
          </div>
        )
      case 'retry-notice':
        return (
          <div key="retry-notice" className="px-4 py-1.5">
            <div className="flex items-center gap-2 text-mady-caption text-mady-warning">
              <RefreshCw size={12} className="animate-spin" />
              <span>
                请求失败，{Math.round(item.notice.delayMs / 1000)}s 后自动重试
                （第 {item.notice.attempt}/{item.notice.maxRetries} 次）…
              </span>
            </div>
          </div>
        )
      case 'compaction-notice':
        return (
          <div key="compaction-notice" className="px-4 py-1.5">
            <div className="flex items-center gap-2 text-mady-caption text-mady-text-tertiary">
              <Scissors size={12} />
              {item.notice.active ? (
                <span>
                  正在压缩上下文
                  {item.notice.tokensBefore
                    ? `（约 ${formatTokens(item.notice.tokensBefore)} tokens）`
                    : ''}
                  …
                </span>
              ) : (
                <span>
                  上下文已压缩
                  {item.notice.tokensBefore != null && item.notice.tokensAfter != null
                    ? `：${formatTokens(item.notice.tokensBefore)} → ${formatTokens(item.notice.tokensAfter)} tokens`
                    : ''}
                  {item.notice.messagesCut ? `，裁剪 ${item.notice.messagesCut} 条历史消息` : ''}
                </span>
              )}
            </div>
          </div>
        )
      case 'error':
        return (
          <div key="error" className="px-4 py-2">
            <div className="max-w-[75%] rounded-xl px-4 py-3 bg-mady-danger/10 border border-mady-danger/20 text-mady-danger text-mady-body">
              {item.error}
            </div>
          </div>
        )
    }
  }

  // ── 渲染 ────────────────────────────────────────

  return (
    <div className={`h-screen w-screen flex flex-col bg-mady-bg-primary text-mady-text-primary select-none ${isFocusMode ? 'layout-focus' : ''}`}>
      {/* 全宽标题栏：对齐 macOS 交通灯，内容左侧缩进避免重叠 */}
      <header className="titlebar-drag-region mac-titlebar-safe h-[var(--mady-titlebar-height)] flex items-center justify-between px-4 border-b border-mady-separator mady-material">
        <div className="flex items-center gap-2.5">
          {!showSidebar && !isFocusMode && (
            <button
              onClick={() => {
                setShowSidebar(true)
                useSettingsStore.setState({ sidebarCollapsed: false })
              }}
              className="p-1 rounded-md hover:bg-mady-bg-secondary text-mady-text-secondary transition-colors"
              title="显示侧栏"
            >
              <PanelRightOpen size={15} />
            </button>
          )}
          {/* 品牌标识 */}
          <div className="flex items-center gap-1.5">
            <div
              className="w-5 h-5 rounded-md flex items-center justify-center"
              style={{ background: 'linear-gradient(135deg, var(--color-mady-accent) 0%, var(--color-mady-accent-tertiary) 100%)' }}
            >
              <span className="text-white text-[9px] font-bold">M</span>
            </div>
            <h1 className="text-mady-ui font-semibold text-mady-text-primary">Mady</h1>
          </div>

          {/* 阶段 2.1c：会话标签栏（多标签并行会话） */}
          <div className="flex items-center">
            <span className="text-mady-text-quaternary mr-1.5">/</span>
            <TabBar />
          </div>
          {threadId && (
            <>
              <span className="text-mady-text-quaternary">/</span>
              <span className="text-mady-caption text-mady-text-tertiary">
                会话
              </span>
            </>
          )}
        </div>

        {/* UsageStrip + ContextWindowRing：用量条（C09）/ 环形指示器（阶段 2.3） */}
        <UsageStrip />
        <ContextWindowRing />

        {!isFocusMode && (
          <div className="flex items-center gap-0.5">
            {/* 功能视图组 */}
            <button
              onClick={() => setShowSkills(true)}
              className="p-1.5 rounded-md text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary transition-all duration-150"
              title="技能"
            >
              <Zap size={14} />
            </button>
            <button
              onClick={() => setShowMcp(true)}
              className="p-1.5 rounded-md text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary transition-all duration-150"
              title="MCP 服务器"
            >
              <Server size={14} />
            </button>
            <button
              onClick={() => setShowKnowledge(true)}
              className="p-1.5 rounded-md text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary transition-all duration-150"
              title="知识库"
            >
              <Database size={14} />
            </button>
            <button
              onClick={() => setShowTemplates(true)}
              className="p-1.5 rounded-md text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary transition-all duration-150"
              title="模板库"
            >
              <FileText size={14} />
            </button>
            {/* 分组分隔线 */}
            <div className="w-px h-4 bg-mady-separator mx-1" />
            {/* 面板切换组 */}
            <button
              onClick={() => setShowDocViewer(!showDocViewer)}
              className={`p-1.5 rounded-md text-mady-ui transition-all duration-150 ${
                showDocViewer
                  ? 'bg-mady-accent-soft text-mady-accent'
                  : 'text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary'
              }`}
              title="文档预览"
            >
              <PanelRightOpen size={14} />
            </button>
          </div>
        )}
      </header>

      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        {showSidebar && !isFocusMode && (
          <Sidebar onNewChat={handleNewChat} onSettings={handleSettings} />
        )}

        {/* Chat Main */}
        <main className="flex-1 flex flex-col min-w-0">
          {/* 消息列表（虚拟化） */}
          <div ref={scrollContainerRef} className="flex-1 overflow-y-auto">
            {!showContent ? (
              /* 空状态 */
              <div className="h-full flex items-center justify-center">
                <div className="text-center max-w-md px-6 relative">
                  {/* 背景装饰光晕 */}
                  <div
                    className="absolute inset-0 flex items-center justify-center pointer-events-none"
                  >
                    <div
                      className="w-48 h-48 rounded-full opacity-8 blur-3xl"
                      style={{ background: 'var(--color-mady-accent)' }}
                    />
                  </div>

                  {/* 图标组合 */}
                  <div className="relative z-10 mb-6 flex items-center justify-center">
                    <div
                      className="w-16 h-16 rounded-2xl flex items-center justify-center shadow-lg"
                      style={{
                        background: 'linear-gradient(135deg, var(--color-mady-accent) 0%, var(--color-mady-accent-tertiary) 100%)',
                        boxShadow: '0 8px 24px var(--color-mady-accent-glow)',
                      }}
                    >
                      <Sparkles size={28} className="text-white" />
                    </div>
                  </div>

                  <h2 className="text-mady-h1 font-semibold mb-2 relative z-10">开始新对话</h2>
                  <p className="text-mady-text-secondary text-mady-body mb-8 relative z-10">
                    与 Mady 助手交流，获取专利分析与法律问题解答
                  </p>

                  {/* 快捷引导 */}
                  <div className="flex flex-wrap gap-2 justify-center relative z-10">
                    {['专利新颖性分析', '权利要求撰写', 'OA 答复策略'].map((hint) => (
                      <button
                        key={hint}
                        onClick={() => {
                          useChatStore.getState().sendMessage(hint)
                        }}
                        className="px-3 py-1.5 rounded-lg text-mady-small text-mady-text-secondary bg-mady-bg-secondary border border-mady-border hover:border-mady-accent/30 hover:text-mady-accent transition-all duration-150"
                      >
                        {hint}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className="max-w-3xl mx-auto py-4 relative" style={{ height: virtualizer.getTotalSize() }}>
                {virtualizer.getVirtualItems().map((vItem) => {
                  const item = items[vItem.index]
                  const outerKey = item.kind === 'message'
                    ? (item as Extract<TranscriptItem, { kind: 'message' }>).message.id
                    : item.kind === 'round-header'
                      ? `round-${(item as Extract<TranscriptItem, { kind: 'round-header' }>).roundId || (item as Extract<TranscriptItem, { kind: 'round-header' }>).startIndex}`
                      : item.kind
                  return (
                    <div
                      key={outerKey}
                      ref={virtualizer.measureElement}
                      data-index={vItem.index}
                      className="absolute left-0 right-0"
                      style={{
                        transform: `translateY(${vItem.start}px)`,
                      }}
                    >
                      {renderItem(item)}
                    </div>
                  )
                })}
              </div>
            )}
          </div>

          {/* Context Indicator */}
          <ContextIndicator />

          {/* Decision Surface Footer */}
          <DecisionSurface />
        </main>

        {/* Document Viewer / Context Panel */}
        {showDocViewer && (
          <DocumentViewer
            file={docFile}
            onClose={() => {
              setShowDocViewer(false)
              setDocFile(null)
            }}
          />
        )}

        {/* 文件查看器浮层（PilotDeck 对齐） */}
        <FileViewerOverlay />

        {/* KnowledgeView 覆盖层 */}
        {showKnowledge && (
          <KnowledgeView onClose={() => setShowKnowledge(false)} />
        )}

        {/* TemplatesView 覆盖层 */}
        {showTemplates && (
          <TemplatesView onClose={() => setShowTemplates(false)} />
        )}

        {/* SkillsView 覆盖层 */}
        {showSkills && (
          <SkillsView onClose={() => setShowSkills(false)} />
        )}

        {/* McpView 覆盖层 */}
        {showMcp && (
          <McpView onClose={() => setShowMcp(false)} />
        )}

        {/* Settings 覆盖层 */}
        {showSettings && (
          <SettingsPanel onClose={() => setShowSettings(false)} />
        )}

        {/* CommandPalette — ⌘K 命令面板 */}
        <CommandPalette
          open={showCommandPalette}
          onClose={() => setShowCommandPalette(false)}
          commands={buildCommands({
            toggleSettings: () => setShowSettings(true),
            toggleSidebar: () => setShowSidebar((v) => !v),
            setTheme: (mode) => {
              // F-I1：经 ThemeProvider 切换（写 .dark class + localStorage），
              // 不再直接改 DOM 属性（data-theme 是 provider 的输出而非输入）。
              setMode(mode)
            },
            clearChat: () => {
              useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
            },
            exportChat: () => {
              // F-I3：真实导出为 Markdown 并触发下载
              const { messages } = useChatStore.getState()
              const md = exportSession(messages)
              const blob = new Blob([md], { type: 'text/markdown;charset=utf-8' })
              const url = URL.createObjectURL(blob)
              const link = document.createElement('a')
              link.href = url
              link.download = `mady-session-${Date.now()}.md`
              link.click()
              URL.revokeObjectURL(url)
            },
            toggleFocusMode: () => {
              // M-15：toggle 应双向——专注模式 ↔ 标准布局（原实现只进不出）
              const settings = useSettingsStore.getState()
              const next = settings.layout === 'focus' ? 'standard' : 'focus'
              settings.update({ layout: next as LayoutMode })
              setShowSidebar(next !== 'focus' && !useSettingsStore.getState().sidebarCollapsed)
            },
            openTemplate: (_name: string) => {
              setShowTemplates(true)
            },
          })}
        />
      </div>

      {/* TodoDock — 底部待办坞 */}
      <TodoDock />

      {/* StatusBar */}
      <StatusBar />
    </div>
  )
}
