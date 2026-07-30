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
import { useSettingsStore, type LayoutMode } from '@/stores/settings'
import type { Message, ToolCall, CompactionNotice, RetryNotice } from '@/stores/chat'
import { ReasoningBlock } from '../ReasoningBlock'
import { UsageStrip } from '../UsageStrip'
import { Sidebar } from '../Sidebar'
import { MessageBubble } from './MessageBubble'
import { DecisionSurface } from '../DecisionSurface'
import { ContextIndicator } from '../ContextIndicator'
import { StatusBar } from '../StatusBar'
import { ToolCard } from '../ToolCard'
import { TodoDock } from '../TodoDock'
import { DocumentViewer, type DocViewerFile } from '../DocumentViewer'
import { FileViewerOverlay } from '../fileviewer/FileViewerOverlay'
import { SettingsPanel } from '../SettingsPanel'
import { KnowledgeView } from '../KnowledgeView'
import { TemplatesView } from '../TemplatesView'
import { SkillsView } from '../SkillsView'
import { McpView } from '../McpView'
import { CommandPalette } from './CommandPalette'
import { buildCommands } from '@/stores/commands'
import { Sparkles, PanelRightOpen, Database, FileText, Server, Zap, Loader2, RefreshCw, Scissors } from 'lucide-react'

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
function useAutoScroll(virtualizer: any, enabled: boolean, items: TranscriptItem[]): void {
  const prevCountRef = useRef(items.length)

  useEffect(() => {
    if (!enabled || items.length === 0) return

    // 仅在新项追加或内容变化时触发
    if (items.length === prevCountRef.current) return
    prevCountRef.current = items.length

    // 自动滚到底部
    virtualizer.scrollToIndex(items.length - 1, { align: 'end', behavior: 'smooth' })
  }, [items.length, enabled, virtualizer])
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
  const [showSidebar, setShowSidebar] = useState(!isFocusMode)
  const [showDocViewer, setShowDocViewer] = useState(false)
  const [docFile, setDocFile] = useState<DocViewerFile | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [showKnowledge, setShowKnowledge] = useState(false)
  const [showTemplates, setShowTemplates] = useState(false)
  const [showSkills, setShowSkills] = useState(false)
  const [showMcp, setShowMcp] = useState(false)
  const [showCommandPalette, setShowCommandPalette] = useState(false)

  const scrollContainerRef = useRef<HTMLDivElement>(null)

  // ⌘K 快捷键切换命令面板
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setShowCommandPalette((v) => !v)
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

  // ── 构建虚拟化条目 ──────────────────────────────

  const items = useMemo<TranscriptItem[]>(() => {
    const result: TranscriptItem[] = []

    // 已完成的过往消息
    for (let i = 0; i < messages.length; i++) {
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
  }, [messages, output, thinking, running, toolCalls, error, currentStep, stepCount, compaction, retryNotice])

  // ── 虚拟化器 ────────────────────────────────────

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollContainerRef.current,
    estimateSize: (index) => itemHeight(items[index]),
    overscan: 8,
    paddingEnd: 8,
  })

  // 智能自动滚动
  useAutoScroll(virtualizer, running || !!output || !!thinking, items)

  // ── 渲染单项 ────────────────────────────────────

  const renderItem = (item: TranscriptItem) => {
    switch (item.kind) {
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
              onClick={() => setShowSidebar(true)}
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
          {threadId && (
            <>
              <span className="text-mady-text-quaternary">/</span>
              <span className="text-mady-caption text-mady-text-tertiary">
                会话
              </span>
            </>
          )}
        </div>

        {/* UsageStrip：用量条（C09） */}
        <UsageStrip />

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
                {virtualizer.getVirtualItems().map((vItem) => (
                  <div
                    key={items[vItem.index].kind === 'message'
                      ? (items[vItem.index] as Extract<TranscriptItem, { kind: 'message' }>).message.id
                      : items[vItem.index].kind}
                    ref={virtualizer.measureElement}
                    data-index={vItem.index}
                    className="absolute left-0 right-0"
                    style={{
                      transform: `translateY(${vItem.start}px)`,
                    }}
                  >
                    {renderItem(items[vItem.index])}
                  </div>
                ))}
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
              document.documentElement.setAttribute('data-theme', mode)
            },
            clearChat: () => {
              useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
            },
            exportChat: () => {
              // 由 MessageBubble 等组件实现的导出功能
            },
            toggleFocusMode: () => {
              useSettingsStore.getState().update({ layout: 'focus' as LayoutMode })
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
