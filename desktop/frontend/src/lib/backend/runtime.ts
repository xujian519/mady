/**
 * 后端服务层。
 *
 * 封装对 Wails Go Binding 的调用，提供类型安全的 API。
 * 生产环境优先使用 Wails 注入的 `window.go` 全局对象；
 * 开发期/浏览器回退到动态导入生成的 `wailsjs` 模块。
 *
 * `window.go` 的全局声明集中在 `src/types/wails.d.ts`（P2-7）。
 */

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
    let target: unknown = window.go
    for (const part of parts) {
      if (target === null || typeof target !== 'object') {
        target = undefined
        break
      }
      target = (target as Record<string, unknown>)[part]
    }
    if (target !== null && typeof target === 'object') {
      const fn = (target as Record<string, unknown>)[method]
      if (typeof fn === 'function') {
        return (fn as (...args: unknown[]) => Promise<unknown>).bind(target)
      }
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
export async function callBinding<T>(module: string, method: string, ...args: unknown[]): Promise<T> {
  const fn = await resolveBinding(module, method)
  return (await fn(...args)) as T
}

// ── Types ─────────────────────────────────────────

export interface HealthInfo {
  provider: string
  model: string
  version: string
  uptime: string
}

/** UpdateInfo — 更新检查结果（对应 Go UpdateInfo，R-P1-6 真实检测）。 */
export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  downloadUrl?: string
  message: string
}

/** 已注册项目摘要。 */
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
