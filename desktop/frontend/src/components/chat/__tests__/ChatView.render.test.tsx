/**
 * ChatView 组件级特征测试（M57 护栏）。
 *
 * 补充 M54/M56 纯函数测试覆盖不到的组合行为：空态引导、消息渲染、
 * 流式输出与错误提示。ChatView 的重依赖（pdf.js、虚拟化、侧栏与各类面板）
 * 全部 mock 成轻量替身，断言只看 ChatView 自身的组装结果。
 *
 * @vitest-environment jsdom
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, cleanup } from '@testing-library/react'
import type { Message } from '@/stores/chat'

vi.mock('pdfjs-dist', () => ({ GlobalWorkerOptions: {}, getDocument: vi.fn() }))

// 虚拟化器：jsdom 无布局尺寸，直接让全部条目可见。
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 100,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, i) => ({ index: i, start: i * 100, key: i })),
    measureElement: () => {},
    scrollToIndex: () => {},
  }),
}))

vi.mock('@/lib/backend', () => ({
  getThread: vi.fn(async () => []),
  listDirectory: vi.fn(async () => []),
  sendMessage: vi.fn(),
  stopChat: vi.fn(),
  listProjects: vi.fn(async () => []),
  getCurrentProject: vi.fn(async () => null),
  createProjectFolder: vi.fn(),
  selectProjectFolder: vi.fn(),
  switchProject: vi.fn(),
  createFolder: vi.fn(),
  renameFolder: vi.fn(),
  deleteEntry: vi.fn(),
  writeFile: vi.fn(),
}))

vi.mock('@/lib/wails', () => ({
  listenToWailsEvent: vi.fn(() => () => {}),
}))

vi.mock('@/lib/sessionExport', () => ({ exportSession: vi.fn() }))

// 轻量替身：只保留可断言的可见内容。
vi.mock('@/components/chat/MessageBubble', () => ({
  MessageBubble: ({ message, isStreaming }: { message: Message; isStreaming?: boolean }) => (
    <div data-testid={isStreaming ? 'streaming-bubble' : 'message-bubble'}>{message.content}</div>
  ),
}))
vi.mock('@/components/TabBar', () => ({ TabBar: () => <div data-testid="tab-bar" /> }))
vi.mock('@/components/Sidebar', () => ({ Sidebar: () => <div data-testid="sidebar" /> }))
vi.mock('@/components/ReasoningBlock', () => ({ ReasoningBlock: () => null }))
vi.mock('@/components/UsageStrip', () => ({ UsageStrip: () => null }))
vi.mock('@/components/ContextWindowRing', () => ({ ContextWindowRing: () => null }))
vi.mock('@/components/DecisionSurface', () => ({ DecisionSurface: () => null }))
vi.mock('@/components/ContextIndicator', () => ({ ContextIndicator: () => null }))
vi.mock('@/components/StatusBar', () => ({ StatusBar: () => null }))
vi.mock('@/components/ToolCard', () => ({ ToolCard: () => null }))
vi.mock('@/components/TodoDock', () => ({ TodoDock: () => null }))
vi.mock('@/components/DocumentViewer', () => ({ DocumentViewer: () => null }))
vi.mock('@/components/fileviewer/FileViewerOverlay', () => ({ FileViewerOverlay: () => null }))
vi.mock('@/components/SettingsPanel', () => ({ SettingsPanel: () => null }))
vi.mock('@/components/KnowledgeView', () => ({ KnowledgeView: () => null }))
vi.mock('@/components/TemplatesView', () => ({ TemplatesView: () => null }))
vi.mock('@/components/SkillsView', () => ({ SkillsView: () => null }))
vi.mock('@/components/McpView', () => ({ McpView: () => null }))
vi.mock('@/components/chat/CommandPalette', () => ({ CommandPalette: () => null }))

import { ChatView } from '@/components/chat/ChatView'
import { useChatStore } from '@/stores/chat'

/** 重置聊天 store 到初始状态，避免用例间串扰。 */
function resetChatStore() {
  useChatStore.setState(useChatStore.getInitialState(), true)
}

function userMessage(content: string): Message {
  return { id: 'm1', role: 'user', content, timestamp: 0 }
}

beforeEach(() => {
  cleanup()
  resetChatStore()
})

afterEach(() => {
  cleanup()
  resetChatStore()
})

describe('ChatView', () => {
  it('无消息时展示空态引导', () => {
    render(<ChatView />)
    expect(screen.getByText('开始新对话')).toBeInTheDocument()
    expect(screen.getByText('专利新颖性分析')).toBeInTheDocument()
    expect(screen.getByText('OA 答复策略')).toBeInTheDocument()
    expect(screen.queryByTestId('message-bubble')).toBeNull()
  })

  it('有消息时渲染消息气泡且不显示空态', () => {
    useChatStore.setState({ messages: [userMessage('我想分析新颖性')] })

    render(<ChatView />)
    expect(screen.getByTestId('message-bubble')).toHaveTextContent('我想分析新颖性')
    expect(screen.queryByText('开始新对话')).toBeNull()
  })

  it('流式输出以 isStreaming 渲染为独立气泡', () => {
    useChatStore.setState({ output: '正在检索…', running: true })

    render(<ChatView />)
    expect(screen.getByTestId('streaming-bubble')).toHaveTextContent('正在检索…')
    expect(screen.queryByText('开始新对话')).toBeNull()
  })

  it('错误状态直接展示错误文本', () => {
    useChatStore.setState({ error: '模型调用失败' })

    render(<ChatView />)
    expect(screen.getByText('模型调用失败')).toBeInTheDocument()
  })
})
