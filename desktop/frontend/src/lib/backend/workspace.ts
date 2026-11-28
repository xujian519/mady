/**
 * 工作区相关的后端调用：文件树、技能、MCP、模板、知识库与项目。
 */

import { callBinding } from './runtime'

export interface FileEntry {
  name: string
  isDir: boolean
  size: number
  modTime: number
}

/** 后端健康信息：当前 provider/model、版本与运行时长。 */
export interface ProjectInfo {
  id: string
  alias: string
  path: string
  status: string
  lastAccessed: string
}

/** A2UI ClientAction — 用户在 A2UI surface 上触发的交互。 */
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
