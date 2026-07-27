/**
 * A2UI 动态值解析器。
 *
 * 实现 Dynamic 值的三种形态识别与解析（对齐 Go a2ui/dynamic.go）：
 *   1. Literal（字面量）—— 直接使用
 *   2. Bind（JSON Pointer 绑定）—— `{"path": "/user/name"}`
 *   3. FunctionCall（函数调用）—— `{"call": "formatDate", "args": {...}}`
 *
 * 识别顺序与 Go 端 UnmarshalJSON 完全一致：call → 仅 path → literal。
 */

import { getData } from './datamodel'

// ── 类型 ───────────────────────────────────────────

export type DynamicKind = 'literal' | 'bind' | 'call'

export interface ClassifiedDynamic {
  kind: DynamicKind
  data: unknown
}

// ── 运行时识别 ────────────────────────────────────

/**
 * 识别 Dynamic 值的形态。
 *
 * 与 Go 端 UnmarshalJSON 的逻辑顺序一致：
 *   1. 对象且含 "call" 键 → FunctionCall
 *   2. 对象且只有 "path" 键 → Bind
 *   3. 其余 → Literal
 */
export function classifyDynamic(value: unknown): ClassifiedDynamic {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const obj = value as Record<string, unknown>
    if ('call' in obj) {
      return { kind: 'call', data: value }
    }
    if (Object.keys(obj).length === 1 && 'path' in obj) {
      return { kind: 'bind', data: value }
    }
  }
  return { kind: 'literal', data: value }
}

// ── 绑定缓存 ───────────────────────────────────────

const _bindCache = new WeakMap<object, Map<string, unknown>>()

function _cachedGet(dm: unknown, pointer: string): unknown {
  if (dm === null || typeof dm !== 'object') return undefined
  let cache = _bindCache.get(dm as object)
  if (!cache) {
    cache = new Map()
    _bindCache.set(dm as object, cache)
  }
  if (cache.has(pointer)) return cache.get(pointer)
  const [v] = getData(dm, pointer)
  cache.set(pointer, v)
  return v
}

/**
 * 清除 dataModel 上所有绑定缓存。
 * 在 SurfaceStore.applyDataModel 后调用。
 */
export function clearBindCache(dm: unknown): void {
  if (dm !== null && typeof dm === 'object') {
    _bindCache.delete(dm as object)
  }
}

// ── 解析 ──────────────────────────────────────────

/**
 * 解析 Dynamic 值为实际值。
 *
 * @param d Dynamic 值（JSON 解码后的值）
 * @param dataModel 当前 data model
 * @param functions 函数注册表（可选，缺省时 call 返回 undefined）
 */
export function resolveDynamic(
  d: unknown,
  dataModel: unknown,
  functions?: Record<string, (args: Record<string, unknown>) => unknown>,
): unknown {
  const { kind, data } = classifyDynamic(d)
  switch (kind) {
    case 'call': {
      const fc = data as { call: string; args?: Record<string, unknown> }
      return callFunction(fc.call, fc.args ?? {}, dataModel, functions)
    }
    case 'bind': {
      const b = data as { path: string }
      return resolveBind(dataModel, b.path)
    }
    default:
      return data
  }
}

/**
 * 解析 Bind 值。
 * 从 dataModel 按 JSON Pointer 读取，带 WeakMap 缓存。
 */
export function resolveBind(dm: unknown, pointer: string): unknown {
  return _cachedGet(dm, pointer)
}

/**
 * 执行 FunctionCall。
 *
 * @param name 函数名
 * @param args 命名参数对象
 * @param dataModel 当前 data model（传递给函数，供其按需绑定）
 * @param functions 函数注册表
 */
export function callFunction(
  name: string,
  args: Record<string, unknown>,
  _dataModel: unknown,
  functions?: Record<string, (args: Record<string, unknown>) => unknown>,
): unknown {
  if (functions && name in functions) {
    return functions[name](args)
  }
  console.warn(`[a2ui] unknown function: ${name}`)
  return undefined
}
