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

export interface HealthInfo {
  provider: string
  model: string
  version: string
  uptime: string
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

// ── Health ────────────────────────────────────────

/**
 * 健康检查。
 */
export async function health(): Promise<HealthInfo> {
  return callBinding<HealthInfo>('main/App', 'Health')
}
