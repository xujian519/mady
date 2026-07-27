/**
 * ChatView — 主聊天视图（三栏布局）。
 *
 * 布局结构：
 * ┌──────────────────────────────────────────────┐
 * │ Sidebar  │  Chat Main         │ Context Panel │
 * │ 会话列表  │  消息流             │ 文档预览      │
 * │          │  ToolCard          │              │
 * │          │  Composer          │              │
 * └──────────────────────────────────────────────┘
 * │ StatusBar                                     │
 * └──────────────────────────────────────────────┘
 */

import React, { useRef, useEffect, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useChatStore, initialState } from '@/stores/chat'
import { Sidebar } from './Sidebar'
import { MessageBubble } from './MessageBubble'
import { Composer } from './Composer'
import { StatusBar } from './StatusBar'
import { ToolCard } from './ToolCard'
import { ApprovalCard } from './ApprovalCard'
import { DocumentViewer, type DocViewerFile } from './DocumentViewer'
import { SettingsPanel } from './SettingsPanel'
import { KnowledgeView } from './KnowledgeView'
import { TemplatesView } from './TemplatesView'
import { Sparkles, PanelRightOpen, Brain, Database, FileText } from 'lucide-react'

export const ChatView: React.FC = () => {
  // 高频变更：用 useShallow 避免无关状态变化触发重渲染
  const { messages, output, toolCalls } = useChatStore(
    useShallow((s) => ({ messages: s.messages, output: s.output, toolCalls: s.toolCalls })),
  )
  // 低频变更
  const error = useChatStore((s) => s.error)
  const running = useChatStore((s) => s.running)
  const thinking = useChatStore((s) => s.thinking)
  const approvalPrompt = useChatStore((s) => s.approvalPrompt)
  const threadId = useChatStore((s) => s.threadId)

  const [showSidebar, setShowSidebar] = useState(true)
  const [showDocViewer, setShowDocViewer] = useState(false)
  const [docFile, setDocFile] = useState<DocViewerFile | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [showKnowledge, setShowKnowledge] = useState(false)
  const [showTemplates, setShowTemplates] = useState(false)

  const messagesEndRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)

  // 智能自动滚动：仅在用户接近底部时跟随
  useEffect(() => {
    const el = scrollContainerRef.current
    if (!el) return
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 120
    if (nearBottom) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, output, toolCalls, thinking])

  const handleNewChat = () => {
    useChatStore.setState({ ...initialState, ready: true, threads: useChatStore.getState().threads })
  }

  const handleSettings = () => {
    setShowSettings(true)
  }

  const hasMessages = messages.length > 0 || output || toolCalls.length > 0

  return (
    <div className="h-screen w-screen flex flex-col bg-mady-bg-primary text-mady-text-primary select-none">
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar */}
        {showSidebar && (
          <Sidebar onNewChat={handleNewChat} onSettings={handleSettings} />
        )}

        {/* Chat Main */}
        <main className="flex-1 flex flex-col min-w-0">
          {/* 标题栏（红绿灯区 + 视图切换） */}
          <header className="titlebar-drag-region h-10 flex items-center justify-between px-4 border-b border-mady-separator bg-mady-bg-secondary/50 backdrop-blur-sm">
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
          </header>

          {/* 消息列表 */}
          <div ref={scrollContainerRef} className="flex-1 overflow-y-auto">
            {!hasMessages ? (
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
              <div className="max-w-3xl mx-auto py-4">
                {/* 历史消息 */}
                {messages.map((msg) => (
                  <MessageBubble key={msg.id} message={msg} />
                ))}

                {/* 当前轮 Agent 流式输出 */}
                {output && (
                  <MessageBubble
                    message={{
                      id: 'streaming',
                      role: 'agent',
                      content: output,
                      timestamp: Date.now(),
                    }}
                    isStreaming={running}
                  />
                )}

                {/* 流式 thinking 实时展示 */}
                {thinking && running && (
                  <div className="px-4 py-2 max-w-[75%]">
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
                        {thinking}
                      </div>
                    </details>
                  </div>
                )}

                {/* ToolCard 列表 */}
                {toolCalls.length > 0 && (
                  <div className="px-4 py-2 space-y-2">
                    {toolCalls.map((tc) => (
                      <ToolCard key={tc.id} toolCall={tc} />
                    ))}
                  </div>
                )}

                {/* ApprovalCard */}
                {approvalPrompt && (
                  <div className="px-4 py-2">
                    <ApprovalCard prompt={approvalPrompt} />
                  </div>
                )}

                {/* 错误提示 */}
                {error && (
                  <div className="px-4 py-2">
                    <div className="max-w-[75%] rounded-xl px-4 py-3 bg-mady-danger/10 border border-mady-danger/20 text-mady-danger text-mady-body">
                      {error}
                    </div>
                  </div>
                )}

                <div ref={messagesEndRef} />
              </div>
            )}
          </div>

          {/* Composer */}
          <Composer />
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
