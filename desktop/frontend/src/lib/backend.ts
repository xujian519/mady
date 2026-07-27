/**
 * 后端服务层。
 *
 * 封装对 Wails Go Binding 的调用，提供类型安全的 API。
 * 开发期（wailsjs 不可用）回退到 mock 实现。
 */

/**
 * 动态导入 Wails Go binding 并执行方法。
 * 开发期返回 mock 数据。
 */
async function callBinding<T>(module: string, method: string, ...args: unknown[]): Promise<T> {
  try {
    const mod = await import(/* @vite-ignore */ `../../wailsjs/go/${module}`)
    if (typeof mod[method] === 'function') {
      return (await mod[method](...args)) as T
    }
  } catch {
    // wailsjs 未生成：开发期/CI 环境
  }
  throw new Error(`Wails binding unavailable: ${module}.${method}`)
}

// ── Types ─────────────────────────────────────────

export interface ThreadSummary {
  key: string
  title: string
  updatedAt: string
  messageN: number
}

/** 文件系统条目（来自 ListDirectory binding）。 */
export interface FileEntry {
  name: string
  isDir: boolean
  size: number
  modTime: number
}

export interface HealthInfo {
  provider: string
  model: string
  version: string
  uptime: string
}

/** A2UI ClientAction — 用户在 A2UI surface 上触发的交互。 */
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
 * 删除会话。
 */
export async function deleteThread(key: string): Promise<void> {
  return callBinding<void>('main/App', 'DeleteThread', key)
}

// ── File System ────────────────────────────────────

/**
 * 列出指定相对路径下的文件/文件夹条目。
 * relPath 为空字符串时列出项目根目录。
 */
export async function listDirectory(relPath?: string): Promise<FileEntry[]> {
  return callBinding<FileEntry[]>('main/App', 'ListDirectory', relPath ?? '')
}

/**
 * 在 parentPath 下创建名为 folderName 的新文件夹。
 * 返回创建后的完整路径。
 */
export async function createFolder(parentPath: string, folderName: string): Promise<string> {
  return callBinding<string>('main/App', 'CreateFolder', parentPath, folderName)
}

/**
 * 将 oldPath 重命名为 newName（仅文件名部分）。
 */
export async function renameFolder(oldPath: string, newName: string): Promise<void> {
  return callBinding<void>('main/App', 'RenameFolder', oldPath, newName)
}

// ── Health ────────────────────────────────────────

/**
 * 健康检查。
 */
export async function health(): Promise<HealthInfo> {
  return callBinding<HealthInfo>('main/App', 'Health')
}
