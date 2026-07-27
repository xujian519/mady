/**
 * A2UI SurfaceStore + 线格式类型（对齐 Go a2ui/surface.go + a2ui/message.go）。
 *
 * SurfaceStore 管理所有活跃的 A2UI surface，处理四种 envelope 操作：
 *   createSurface / updateComponents / updateDataModel / deleteSurface。
 *
 * 前端收到 `agui:a2ui` 事件后，解析其 payload 为 Envelope，
 * 调用 `apply(envelope)` 更新渲染状态。
 */

import { applyUpdate } from './datamodel'
import { clearBindCache } from './dynamic'
import type { CatalogRegistry } from './catalog'

// ═══════════════════════════════════════════════════
// 线格式类型（Wire Types）
// ═══════════════════════════════════════════════════

// ── A2UI 协议版本 ──────────────────────────────────

export const A2UI_VERSION = 'v0.9.1'

// ── Envelope 类型 ──────────────────────────────────

/** 信封类型枚举。 */
export type MessageKind =
  | 'createSurface'
  | 'updateComponents'
  | 'updateDataModel'
  | 'deleteSurface'
  | ''

/** 服务端 → 客户端信封。 */
export interface Envelope {
  version?: string
  createSurface?: CreateSurface
  updateComponents?: UpdateComponents
  updateDataModel?: UpdateDataModel
  deleteSurface?: DeleteSurface
}

/** 识别信封携带的消息体类型。 */
export function envelopeKind(env: Envelope): MessageKind {
  if (env.createSurface) return 'createSurface'
  if (env.updateComponents) return 'updateComponents'
  if (env.updateDataModel) return 'updateDataModel'
  if (env.deleteSurface) return 'deleteSurface'
  return ''
}

/** 返回信封目标 surface ID。 */
export function envelopeSurfaceID(env: Envelope): string {
  switch (envelopeKind(env)) {
    case 'createSurface': return env.createSurface!.surfaceId
    case 'updateComponents': return env.updateComponents!.surfaceId
    case 'updateDataModel': return env.updateDataModel!.surfaceId
    case 'deleteSurface': return env.deleteSurface!.surfaceId
    default: return ''
  }
}

/** CreateSurface 消息体。 */
export interface CreateSurface {
  surfaceId: string
  catalogId: string
  theme?: Record<string, unknown>
  sendDataModel?: boolean
}

/** UpdateComponents 消息体。 */
export interface UpdateComponents {
  surfaceId: string
  components: Component[]
}

/** UpdateDataModel 消息体。 */
export interface UpdateDataModel {
  surfaceId: string
  path?: string
  value?: unknown
  /** 标记 value 是否在 JSON 中出现过（区分 null vs 省略）。 */
  valueSet?: boolean
}

/** DeleteSurface 消息体。 */
export interface DeleteSurface {
  surfaceId: string
}

// ── Component 类型 ─────────────────────────────────

/**
 * A2UI Component。
 *
 * 与 Go 端一致：id 和 type 是保留字段，其余都在 props 中。
 */
export interface Component {
  id: string
  type: string
  props: Record<string, unknown>
}

/** 从扁平 JSON 对象构建 Component。 */
export function componentFromFlat(obj: Record<string, unknown>): Component {
  const comp: Component = { id: '', type: '', props: {} }
  for (const [k, v] of Object.entries(obj)) {
    switch (k) {
      case 'id':
        comp.id = String(v ?? '')
        break
      case 'component':
        comp.type = String(v ?? '')
        break
      default:
        comp.props[k] = v
    }
  }
  return comp
}

// ── ChildList / 子组件引用 ─────────────────────────

/** 子组件列表：静态 ID 数组或模板。 */
export interface ChildList {
  static?: string[]
  template?: ChildTemplate
}

/** 子组件模板：为绑定数组的每个元素实例化一个子组件。 */
export interface ChildTemplate {
  path: string
  componentId: string
}

/** 解析 props 中的 ChildList 值。 */
export function parseChildList(v: unknown): ChildList | undefined {
  if (Array.isArray(v)) {
    return { static: v.filter((e): e is string => typeof e === 'string') }
  }
  if (v && typeof v === 'object' && !Array.isArray(v)) {
    const obj = v as Record<string, unknown>
    if (typeof obj.path === 'string' && typeof obj.componentId === 'string') {
      return { template: { path: obj.path, componentId: obj.componentId } }
    }
  }
  return undefined
}

// ── Action 类型 ────────────────────────────────────

/** 组件交互动作定义。 */
export interface Action {
  event?: ActionEvent
  functionCall?: FunctionCall
}

export interface ActionEvent {
  name: string
  context?: Record<string, unknown>
}

// ── FunctionCall ───────────────────────────────────

export interface FunctionCall {
  call: string
  args?: Record<string, unknown>
}

// ── Check ──────────────────────────────────────────

export interface Check {
  call?: string
  args?: Record<string, unknown>
  condition?: FunctionCall
  message?: string
}

// ── 客户端消息 ─────────────────────────────────────

export interface ClientAction {
  name: string
  surfaceId: string
  sourceComponentId: string
  timestamp: string
  context: Record<string, unknown>
}

export interface ClientError {
  code: string
  surfaceId?: string
  path?: string
  message: string
}

export interface ClientDataModelPayload {
  surfaces: Record<string, unknown>
}

// ═══════════════════════════════════════════════════
// SurfaceStore
// ═══════════════════════════════════════════════════

export const RootComponentID = 'root'

/** Surface 状态。 */
export interface SurfaceState {
  id: string
  catalogId: string
  theme: Record<string, unknown>
  sendDataModel: boolean
  components: Map<string, Component>
  dataModel: unknown
}

/** 错误类型。 */
export class SurfaceExistsError extends Error {
  constructor(surfaceId: string) {
    super(`a2ui: surface already exists: ${surfaceId}`)
    this.name = 'SurfaceExistsError'
  }
}

export class SurfaceNotFoundError extends Error {
  constructor(surfaceId: string) {
    super(`a2ui: surface not found: ${surfaceId}`)
    this.name = 'SurfaceNotFoundError'
  }
}

/** 返回 surface 的根组件（ID 为 "root"）。 */
export function surfaceRoot(srf: SurfaceState): Component | undefined {
  return srf.components.get(RootComponentID)
}

/**
 * SurfaceStore — 管理所有活跃 A2UI surface。
 *
 * 与 Go 端 SurfaceStore 语义对齐（a2ui/surface.go）。
 */
export class SurfaceStore {
  private _surfaces = new Map<string, SurfaceState>()

  /** 当前所有 surface 的快照。 */
  get surfaces(): ReadonlyMap<string, SurfaceState> {
    return this._surfaces
  }

  /** 获取指定 surface。 */
  surface(id: string): SurfaceState | undefined {
    return this._surfaces.get(id)
  }

  /**
   * 应用一个服务端信封，更新 surface 状态。
   * 返回自身以支持链式调用。
   */
  apply(env: Envelope, _registry?: CatalogRegistry): this {
    switch (envelopeKind(env)) {
      case 'createSurface':
        this._applyCreate(env.createSurface!)
        break
      case 'updateComponents':
        this._applyComponents(env.updateComponents!)
        break
      case 'updateDataModel':
        this._applyDataModel(env.updateDataModel!)
        break
      case 'deleteSurface':
        this._applyDelete(env.deleteSurface!)
        break
    }
    return this
  }

  /**
   * 收集所有 sendDataModel=true 的 surface 数据模型。
   * 客户端的 ClientMessage 应携带此 payload。
   */
  clientDataModel(): ClientDataModelPayload {
    const surfaces: Record<string, unknown> = {}
    for (const [id, srf] of this._surfaces) {
      if (srf.sendDataModel) {
        surfaces[id] = srf.dataModel
      }
    }
    return { surfaces }
  }

  // ── 内部 handler ──────────────────────────────────

  private _applyCreate(m: CreateSurface): void {
    if (!m.surfaceId) {
      throw new Error('a2ui: createSurface requires surfaceId')
    }
    if (this._surfaces.has(m.surfaceId)) {
      throw new SurfaceExistsError(m.surfaceId)
    }
    this._surfaces.set(m.surfaceId, {
      id: m.surfaceId,
      catalogId: m.catalogId,
      theme: m.theme ?? {},
      sendDataModel: m.sendDataModel ?? false,
      components: new Map(),
      dataModel: {},
    })
  }

  private _applyComponents(m: UpdateComponents): void {
    if (!m.surfaceId) {
      throw new Error('a2ui: updateComponents requires surfaceId')
    }
    const srf = this._surfaces.get(m.surfaceId)
    if (!srf) throw new SurfaceNotFoundError(m.surfaceId)
    for (const raw of m.components) {
      // 原始 JSON 中 "component" 键对应 TypeScript 的 type，
      // 需通过 componentFromFlat 转换（对齐 Go Component.UnmarshalJSON）
      const c = componentFromFlat(raw as unknown as Record<string, unknown>)
      srf.components.set(c.id, c)
    }
  }

  private _applyDataModel(m: UpdateDataModel): void {
    if (!m.surfaceId) {
      throw new Error('a2ui: updateDataModel requires surfaceId')
    }
    const srf = this._surfaces.get(m.surfaceId)
    if (!srf) throw new SurfaceNotFoundError(m.surfaceId)
    const path = m.path ?? ''
    const hasValue = m.valueSet ?? false
    srf.dataModel = applyUpdate(srf.dataModel, path, m.value, hasValue)
    // 清除绑定缓存，确保下一次 resolveBind 读取最新 data model
    clearBindCache(srf.dataModel)
  }

  private _applyDelete(m: DeleteSurface): void {
    this._surfaces.delete(m.surfaceId)
  }
}
