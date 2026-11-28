/**
 * 会话与标签页相关的后端调用：聊天、会话管理、标签绑定、记忆。
 */

import { callBinding } from './runtime'

export interface ThreadSummary {
  key: string
  title: string
  updatedAt: string
  messageN: number
}

/** 文件系统条目（来自 ListDirectory binding）。 */
export interface ClientAction {
  name: string
  surfaceId: string
  sourceComponentId: string
  timestamp: string
  context?: Record<string, unknown>
}

// ── Thread ────────────────────────────────────────

/** 会话快照（来自 server.GetThread）。 */
export interface ThreadSnapshot {
  // 按需补充字段；当前为初步类型，后续随 T3.1 ChatView 实现完善
  id: string
  messages: unknown[]
}

// ── Chat ──────────────────────────────────────────

export interface ChatRequest {
  message: string
  thread_id?: string
  model?: string
  skills?: string[]
}

/**
 * 发起一轮对话，返回 runId。
 * 流式事件通过 AGUI bridge（Wails Events）实时推送。
 */
export async function chat(req: ChatRequest): Promise<string> {
  return callBinding<string>('main/App', 'Chat', req)
}

/**
 * 取消指定 runId 的对话。
 */
export async function cancelChat(runId: string): Promise<void> {
  return callBinding<void>('main/App', 'Cancel', runId)
}

/**
 * 发送 A2UI 客户端动作（按钮点击、表单提交等）到 agent。
 * surfaceId 标识 action 来源的 A2UI surface。
 */
export async function sendAction(surfaceId: string, action: ClientAction): Promise<void> {
  return callBinding<void>('main/App', 'SendAction', surfaceId, action)
}

// ── Threads ───────────────────────────────────────

/**
 * 列出所有会话。
 */
export async function listThreads(): Promise<ThreadSummary[]> {
  return callBinding<ThreadSummary[]>('main/App', 'ListThreads')
}

/**
 * 获取会话详情。
 */
export async function getThread(key: string): Promise<ThreadSnapshot> {
  return callBinding<ThreadSnapshot>('main/App', 'GetThread', key)
}

/**
 * 重命名会话（自定义标题，阶段 1.4）。
 * 写入会话元数据；listThreads 返回的 title 会携带新标题。
 */
export async function renameThread(key: string, name: string): Promise<void> {
  return callBinding<void>('main/App', 'RenameThread', key, name)
}

/**
 * 将会话移入回收站（软删除，阶段 1.4）。
 */
export async function trashThread(key: string): Promise<void> {
  return callBinding<void>('main/App', 'TrashThread', key)
}

/**
 * 从回收站恢复会话到主列表。
 */
export async function restoreThread(key: string): Promise<void> {
  return callBinding<void>('main/App', 'RestoreThread', key)
}

/**
 * 列出回收站中的会话（按更新时间倒序）。
 */
export async function listTrashedThreads(): Promise<ThreadSummary[]> {
  return callBinding<ThreadSummary[]>('main/App', 'ListTrashedThreads')
}

/**
 * 从回收站彻底删除会话（不可恢复）。
 */
export async function purgeThread(key: string): Promise<void> {
  return callBinding<void>('main/App', 'PurgeThread', key)
}

// ── Tabs（阶段 2.1：Go 侧标签状态机） ──────────────

/** 会话标签（对应 Go 侧 main.Tab）。 */
export interface DesktopTab {
  id: string
  threadId?: string
  title: string
  createdAt: string
  activeAt: string
}

/**
 * 列出全部会话标签（TabBar 渲染用）。
 */
export async function listTabs(): Promise<DesktopTab[]> {
  return callBinding<DesktopTab[]>('main/App', 'ListTabs')
}

/**
 * 返回当前激活标签 ID。
 */
export async function activeTabId(): Promise<string> {
  return callBinding<string>('main/App', 'ActiveTabID')
}

/**
 * 新建会话标签并激活。
 */
export async function createTab(): Promise<DesktopTab> {
  return callBinding<DesktopTab>('main/App', 'CreateTab')
}

/**
 * 关闭指定标签（最后一个标签不可关闭）。
 */
export async function closeTab(id: string): Promise<void> {
  return callBinding<void>('main/App', 'CloseTab', id)
}

/**
 * 激活指定标签。
 */
export async function activateTab(id: string): Promise<void> {
  return callBinding<void>('main/App', 'ActivateTab', id)
}

/**
 * 将标签绑定到既有会话并更新标题（2026-08-04 决策 5：标签联动）。
 * 侧栏点击会话时调用：已存在绑定该会话的标签则激活，否则新建标签并绑定，
 * 消除「侧栏当前会话」与「标签绑定会话」的双真相源撕裂。
 */
export async function bindThreadToSession(tabId: string, threadId: string, title?: string): Promise<void> {
  return callBinding<void>('main/App', 'BindThreadToSession', tabId, threadId, title ?? '')
}

/**
 * 保存剪贴板粘贴的图片（data URL）到项目 attachments/ 目录。
 * 返回相对项目根的路径（供消息 Markdown 引用，如 attachments/pasted-xxx.png）。
 */
export async function savePastedImage(dataURL: string): Promise<string> {
  return callBinding<string>('main/App', 'SavePastedImage', dataURL)
}

/**
 * 向指定标签发起对话（阶段 2.1b：会话绑定按 tab 分派）。
 * 标签未关联会话时后端自动创建并写回。
 */
export async function chatInTab(
  tabId: string,
  req: { message: string; thread_id?: string },
): Promise<string> {
  return callBinding<string>('main/App', 'ChatInTab', tabId, req)
}

// ── Memory（阶段 4：记忆面板） ────────────────────

/** 记忆条目（对应 Go 侧 memory.MemoryEntry；字段名与 wailsjs 生成模型一致，snake_case）。 */
export interface MemoryEntry {
  id: string
  scope: Record<string, unknown>
  layer: string
  content: string
  importance: number
  created_at: string
  updated_at: string
  tier?: string
}

/** 语义检索结果（对应 Go 侧 memory.ScoredMemory）。 */
export interface ScoredMemory {
  entry: MemoryEntry
  semantic: number
  recency: number
  importance: number
  composite: number
  rank: number
}

/**
 * 列出全部三层记忆（user/session/long_term，按更新时间倒序）。
 */
export async function listMemories(limit = 100): Promise<MemoryEntry[]> {
  return callBinding<MemoryEntry[]>('main/App', 'ListMemories', limit)
}

/**
 * 手动写入一条长期记忆。
 */
export async function rememberMemory(content: string): Promise<string> {
  return callBinding<string>('main/App', 'RememberMemory', content)
}

/**
 * 按 ID 删除记忆。
 */
export async function forgetMemory(id: string): Promise<void> {
  return callBinding<void>('main/App', 'ForgetMemory', id)
}

/**
 * 语义检索记忆（记忆面板搜索框）。
 */
export async function recallMemories(query: string, limit = 20): Promise<ScoredMemory[]> {
  return callBinding<ScoredMemory[]>('main/App', 'RecallMemories', query, limit)
}

// ── File System ────────────────────────────────────

/**
 * 列出指定相对路径下的文件/文件夹条目。
 * relPath 为空字符串时列出项目根目录。
 */
