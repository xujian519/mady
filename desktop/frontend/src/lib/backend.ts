/**
 * 后端服务层。
 *
 * 封装对 Wails Go Binding 的调用，提供类型安全的 API。
 * 生产环境优先使用 Wails 注入的 `window.go` 全局对象；
 * 开发期/浏览器回退到动态导入生成的 `wailsjs` 模块。
 */

declare global {
  interface Window {
    /** Wails 运行时注入的 Go binding 根对象。 */
    go?: Record<string, any>
  }
}

/** 是否在真实的 Wails 宿主中运行。 */
function isWailsHost(): boolean {
  return typeof window !== 'undefined' && !!window.go
}

/**
 * 解析 Wails Go binding 方法。
 * 优先从 `window.go.<module>.<method>` 读取；
 * 若不存在（开发期/浏览器预览），回退到动态导入 `wailsjs/go/<module>`。
 */
async function resolveBinding(
  module: string,
  method: string,
): Promise<(...args: unknown[]) => Promise<unknown>> {
  if (isWailsHost()) {
    const parts = module.split('/')
    let target: any = window.go
    for (const part of parts) {
      target = target?.[part]
    }
    if (typeof target?.[method] === 'function') {
      return target[method].bind(target)
    }
  }

  // 开发期/非 Wails 环境：回退到生成的 wailsjs 模块
  try {
    const mod = await import(/* @vite-ignore */ `../../wailsjs/go/${module}`)
    if (typeof mod[method] === 'function') {
      return mod[method]
    }
  } catch {
    // wailsjs 未生成
  }
  throw new Error(`Wails binding unavailable: ${module}.${method}`)
}

/**
 * 调用 Wails Go binding 方法。
 */
async function callBinding<T>(module: string, method: string, ...args: unknown[]): Promise<T> {
  const fn = await resolveBinding(module, method)
  return (await fn(...args)) as T
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

/** UpdateInfo — 更新检查结果（W4-T12 占位契约，对应 Go UpdateInfo）。 */
export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  message: string
}

/** 已注册项目摘要。 */
export interface ProjectInfo {
  id: string
  alias: string
  path: string
  status: string
  lastAccessed: string
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

/** 文件内容种类：文本 / Markdown / 图片 / PDF。 */
export type FileKind = 'text' | 'md' | 'image' | 'pdf'

/** 文件内容（ReadFile 返回）。 */
export interface FileContent {
  name: string
  path: string
  kind: FileKind
  /** kind=text/md 时的 UTF-8 内容。 */
  text?: string
  /** kind=image/pdf 时的 base64 内容。 */
  data?: string
  mime?: string
  size: number
}

/**
 * 读取项目沙箱内的文件内容。
 * 文本/Markdown 返回 text；图片/PDF 返回 base64 data。
 */
export async function readFile(relPath: string): Promise<FileContent> {
  return callBinding<FileContent>('main/App', 'ReadFile', relPath)
}

/**
 * 将文本内容写入项目沙箱内的文件（仅 text/md 类可写）。
 */
export async function writeFile(relPath: string, content: string): Promise<void> {
  return callBinding<void>('main/App', 'WriteFile', relPath, content)
}

// ── Skills / MCP（T5.6 / T5.7） ─────────────────────

/** 技能概要。 */
export interface SkillEntry {
  name: string
  description: string
  /** SKILL.md 相对项目根的路径。 */
  path: string
}

/** 扫描项目 skills/ 目录。 */
export async function listSkills(): Promise<SkillEntry[]> {
  return callBinding<SkillEntry[]>('main/App', 'ListSkills')
}

/** MCP 服务器概要（只读，env 仅含键名）。 */
export interface McpServerEntry {
  name: string
  type: string
  command?: string
  args?: string[]
  url?: string
  envKeys?: string[]
  source: string
}

/** 列出已配置的 MCP 服务器（只读）。 */
export async function listMcpServers(): Promise<McpServerEntry[]> {
  return callBinding<McpServerEntry[]>('main/App', 'ListMcpServers')
}

/**
 * 删除项目沙箱内的文件或空目录。
 */
export async function deleteEntry(relPath: string): Promise<void> {
  return callBinding<void>('main/App', 'DeleteEntry', relPath)
}

// ── Health ────────────────────────────────────────

/**
 * 健康检查。
 */
export async function health(): Promise<HealthInfo> {
  return callBinding<HealthInfo>('main/App', 'Health')
}

/** 检查是否有可用更新（W4-T12 占位：当前返回「已是最新版本」）。 */
export async function checkUpdate(): Promise<UpdateInfo> {
  return callBinding<UpdateInfo>('main/App', 'CheckUpdate')
}

// ── AI Settings（Q9：全局切换 + 新会话生效） ──────────

/** AI 服务设置（Provider/Model），持久化于 ~/.mady/desktop-settings.json。 */
export interface AISettings {
  provider: string
  model: string
}

/**
 * 读取当前生效的 Provider/Model。
 */
export async function getAISettings(): Promise<AISettings> {
  return callBinding<AISettings>('main/App', 'GetAISettings')
}

/**
 * 切换全局 Provider/Model。
 * 仅对后续新建会话生效；已有会话保持原有模型。
 * Provider 切换失败（API Key 缺失等）时抛出错误，后端状态不变。
 */
export async function setAISettings(settings: AISettings): Promise<void> {
  return callBinding<void>('main/App', 'SetAISettings', settings)
}

// ── 模型列表 ────────────────────────────────────────

/** 可用模型信息。 */
export interface ModelInfo {
  id: string
  name: string
  provider: string
  contextWindow: number
}

/**
 * 获取当前可用的模型列表。
 * 替代之前的 mock 数据。
 */
export async function listModels(): Promise<ModelInfo[]> {
  return callBinding<ModelInfo[]>('main/App', 'ListModels')
}

// ── 文档模板库 ──────────────────────────────────────

/** 文档模板条目。 */
export interface DocTemplateEntry {
  name: string
  category: string
  categoryLabel: string
  description: string
  content: string
}

/** 扫描 doc-templates/ 目录，返回所有模板。 */
export async function listDocTemplates(): Promise<DocTemplateEntry[]> {
  return callBinding<DocTemplateEntry[]>('main/App', 'ListDocTemplates')
}

// ── 知识库管理 ──────────────────────────────────────

/** 知识库状态概览。 */
export interface KnowledgeStatus {
  docCount: number
  indexSizeMB: number
  lastUpdated: string
  sourceDirs: string[]
  isIndexing: boolean
}

/** 获取知识库状态概览。 */
export async function getKnowledgeStatus(): Promise<KnowledgeStatus> {
  return callBinding<KnowledgeStatus>('main/App', 'GetKnowledgeStatus')
}

/** 知识库嵌入/Rerank 模型设置（apiKey 为掩码，空表示未配置）。 */
export interface KnowledgeModelSettings {
  baseURL: string
  apiKey: string
  embedModel: string
  rerankModel: string
  rerankEnabled: boolean
}

/** 本地 oMLX 推理服务状态（嵌入/Rerank 依赖）。 */
export interface OmlxServiceStatus {
  running: boolean
  installed: boolean
  message: string
}

/** 获取保存的知识库模型设置（未保存时返回默认值，apiKey 仅掩码）。 */
export async function getKnowledgeModelSettings(): Promise<KnowledgeModelSettings> {
  return callBinding<KnowledgeModelSettings>('main/App', 'GetKnowledgeModelSettings')
}

/** 保存知识库模型设置（apiKey 传掩码保持原值，传空串清空）。保存后重启应用生效。 */
export async function setKnowledgeModelSettings(s: KnowledgeModelSettings): Promise<void> {
  return callBinding<void>('main/App', 'SetKnowledgeModelSettings', s)
}

/** 检测本地 oMLX 推理服务运行状态。 */
export async function getOmlxServiceStatus(): Promise<OmlxServiceStatus> {
  return callBinding<OmlxServiceStatus>('main/App', 'GetOmlxServiceStatus')
}

// ── 项目管理 ──────────────────────────────────────

/** 列出已注册的项目。 */
export async function listProjects(): Promise<ProjectInfo[]> {
  return callBinding<ProjectInfo[]>('main/App', 'ListProjects')
}

/** 获取当前生效的项目。 */
export async function getCurrentProject(): Promise<ProjectInfo | null> {
  return callBinding<ProjectInfo>('main/App', 'GetCurrentProject')
}

/** 打开系统文件夹选择对话框，将选中目录注册为项目并切换。 */
export async function selectProjectFolder(): Promise<ProjectInfo> {
  return callBinding<ProjectInfo>('main/App', 'SelectProjectFolder')
}

/** 在 workspace/projects 下新建文件夹并注册为项目。 */
export async function createProjectFolder(name: string): Promise<ProjectInfo> {
  return callBinding<ProjectInfo>('main/App', 'CreateProjectFolder', name)
}

/** 切换到指定 ID 的项目。 */
export async function switchProject(projectID: string): Promise<void> {
  return callBinding<void>('main/App', 'SwitchProject', projectID)
}
