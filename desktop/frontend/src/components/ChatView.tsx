/**
 * ChatView — 主聊天视图（三栏布局），支持虚拟化消息列表。
 *
 * 布局结构：
 * ┌──────────────────────────────────────────────┐
 * │ Sidebar  │  Chat Main         │ Context Panel │
 * │ 会话列表  │  消息流（虚拟化）   │ 文档预览      │
 * │          │  ToolCard          │              │
 * │          │  Composer          │              │
 * └──────────────────────────────────────────────┘
 * │ StatusBar                                     │
 * └──────────────────────────────────────────────┘
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
import type { Message, ToolCall } from '@/stores/chat'
import { Sidebar } from './Sidebar'
import { MessageBubble } from './MessageBubble'
import { DecisionSurface } from './DecisionSurface'
import { ContextIndicator } from './ContextIndicator'
import { StatusBar } from './StatusBar'
import { ToolCard } from './ToolCard'
import { DocumentViewer, type DocViewerFile } from './DocumentViewer'
import { SettingsPanel } from './SettingsPanel'
import { KnowledgeView } from './KnowledgeView'
import { TemplatesView } from './TemplatesView'
import { Sparkles, PanelRightOpen, Brain, Database, FileText } from 'lucide-react'

// ── 虚拟列表项类型 ────────────────────────────────

type TranscriptItem =
  | { kind: 'message'; message: Message; index: number }
  | { kind: 'streaming-output'; output: string }
  | { kind: 'streaming-thinking'; thinking: string }
  | { kind: 'tool-calls'; toolCalls: ToolCall[] }
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
  const layout = useSettingsStore((s) => s.layout as LayoutMode)

  const isFocusMode = layout === 'focus'
  const [showSidebar, setShowSidebar] = useState(!isFocusMode)
  const [showDocViewer, setShowDocViewer] = useState(false)
  const [docFile, setDocFile] = useState<DocViewerFile | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [showKnowledge, setShowKnowledge] = useState(false)
  const [showTemplates, setShowTemplates] = useState(false)

  const scrollContainerRef = useRef<HTMLDivElement>(null)

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
    if (error) {
      result.push({ kind: 'error', error })
    }

    return result
  }, [messages, output, thinking, running, toolCalls, error])

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
          <div key="thinking" className="px-4 py-2 max-w-[75%]">
            <details className="group" open>
              <summary className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary cursor-pointer hover:text-mady-text-primary transition-colors select-none">
                <Brain size={12} className="text-mady-accent" />
                <span>思考中…</span>
                <span className="flex gap-0.5 ml-1">
                  <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '0ms' }} />
                  <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '150ms' }} />
                  <span className="w-1 h-1 rounded-full bg-mady-accent animate-bounce" style={{ animationDelay: '300ms' }} />
                </span>
              </summary>
              <div className="mt-2 text-mady-small text-mady-text-secondary bg-mady-bg-secondary rounded-lg p-3 whitespace-pre-wrap">
                {item.thinking}
              </div>
            </details>
          </div>
        )
      case 'tool-calls':
        return (
          <div key="tool-calls" className="px-4 py-2 space-y-2">
            {item.toolCalls.map((tc) => (
              <ToolCard key={tc.id} toolCall={tc} />
            ))}
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
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        {showSidebar && !isFocusMode && (
          <Sidebar onNewChat={handleNewChat} onSettings={handleSettings} />
        )}

        {/* Chat Main */}
        <main className="flex-1 flex flex-col min-w-0">
          {/* 标题栏（红绿灯区 + 视图切换） */}
          <header className={`titlebar-drag-region h-10 flex items-center justify-between px-4 border-b border-mady-separator bg-mady-bg-secondary/50 backdrop-blur-sm ${isFocusMode ? 'justify-center' : ''}`}>
            <div className="flex items-center gap-3">
              {!showSidebar && (
                <button
                  onClick={() => setShowSidebar(true)}
                  className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
                  title="显示侧栏"
                >
                  <PanelRightOpen size={15} />
                </button>
              )}
              <h1 className="text-mady-ui font-medium text-mady-text-primary">Mady</h1>
              {threadId && (
                <span className="text-mady-caption text-mady-text-tertiary">
                  会话
                </span>
              )}
            </div>
            {!isFocusMode && (
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setShowKnowledge(true)}
                  className="p-1.5 rounded text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary transition-colors"
                  title="知识库"
                >
                  <Database size={14} />
                </button>
                <button
                  onClick={() => setShowTemplates(true)}
                  className="p-1.5 rounded text-mady-ui text-mady-text-secondary hover:bg-mady-bg-secondary transition-colors"
                  title="模板库"
                >
                  <FileText size={14} />
                </button>
                <button
                  onClick={() => setShowDocViewer(!showDocViewer)}
                  className={`p-1.5 rounded text-mady-ui transition-colors ${
                    showDocViewer
                      ? 'bg-mady-accent-soft text-mady-accent'
                      : 'text-mady-text-secondary hover:bg-mady-bg-secondary'
                  }`}
                  title="文档预览"
                >
                  <PanelRightOpen size={14} />
                </button>
              </div>
            )}
          </header>

          {/* 消息列表（虚拟化） */}
          <div ref={scrollContainerRef} className="flex-1 overflow-y-auto">
            {!showContent ? (
              /* 空状态 */
              <div className="h-full flex items-center justify-center">
                <div className="text-center max-w-md px-6">
                  <div className="w-12 h-12 rounded-2xl bg-mady-accent-soft flex items-center justify-center mx-auto mb-4">
                    <Sparkles size={24} className="text-mady-accent" />
                  </div>
                  <h2 className="text-mady-heading font-semibold mb-2">开始新对话</h2>
                  <p className="text-mady-text-secondary text-mady-body mb-6">
                    与 Mady 助手交流，获取专利分析与法律问题解答
                  </p>
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

        {/* KnowledgeView 覆盖层 */}
        {showKnowledge && (
          <KnowledgeView onClose={() => setShowKnowledge(false)} />
        )}

        {/* TemplatesView 覆盖层 */}
        {showTemplates && (
          <TemplatesView onClose={() => setShowTemplates(false)} />
        )}

        {/* Settings 覆盖层 */}
        {showSettings && (
          <SettingsPanel onClose={() => setShowSettings(false)} />
        )}
      </div>

      {/* StatusBar */}
      <StatusBar />
    </div>
  )
}
