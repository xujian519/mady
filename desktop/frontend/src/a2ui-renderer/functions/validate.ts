/**
 * A2UI 校验与逻辑函数集。
 *
 * 15 个内置函数中的 10 个：
 *   required / regex / length / numeric / email
 *   and / or / not
 *   openUrl
 *
 * 所有函数以 Record<string, (args: Record<string, unknown>) => unknown> 导出。
 */

/** 必填校验。value 不能为 null/undefined/空字符串。 */
function _required(args: Record<string, unknown>): unknown {
  const value = args.value
  if (value === null || value === undefined) return false
  if (typeof value === 'string' && value.trim() === '') return false
  return true
}

/** 正则匹配。 */
function _regex(args: Record<string, unknown>): unknown {
  const value = args.value as string | undefined
  const pattern = args.pattern as string | undefined
  if (value === undefined || pattern === undefined) return false
  try {
    return new RegExp(pattern).test(value)
  } catch {
    return false
  }
}

/** 字符串长度校验（含 min/max）。 */
function _length(args: Record<string, unknown>): unknown {
  const value = args.value as string | undefined
  const min = args.min as number | undefined
  const max = args.max as number | undefined
  if (value === undefined) return false
  const len = value.length
  if (min !== undefined && len < min) return false
  if (max !== undefined && len > max) return false
  return true
}

/** 数字类型校验。 */
function _numeric(args: Record<string, unknown>): unknown {
  const value = args.value
  if (value === null || value === undefined) return false
  if (typeof value === 'number') return true
  if (typeof value === 'string') return !isNaN(Number(value)) && value.trim() !== ''
  return false
}

/** Email 格式校验（基础正则）。 */
function _email(args: Record<string, unknown>): unknown {
  const value = args.value as string | undefined
  if (value === undefined) return false
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)
}

// ── 逻辑函数 ─────────────────────────────────────

function _and(args: Record<string, unknown>): unknown {
  // "values" 是条件数组
  const values = args.values as unknown[] | undefined
  if (!Array.isArray(values)) return false
  return values.every(Boolean)
}

function _or(args: Record<string, unknown>): unknown {
  const values = args.values as unknown[] | undefined
  if (!Array.isArray(values)) return false
  return values.some(Boolean)
}

function _not(args: Record<string, unknown>): unknown {
  const value = args.value
  return !value
}

// ── 副作用 ───────────────────────────────────────

/**
 * openUrl — 打开 URL。
 * 安全约束：仅允许 http/https 协议。
 * 实际调用由 renderer 拦截后通过 Wails runtime.EventsEmit 或 window.open 执行。
 * 本函数仅做协议校验并返回 URL。
 */
function _openUrl(args: Record<string, unknown>): unknown {
  const url = args.url as string | undefined
  if (!url) return undefined
  try {
    const parsed = new URL(url)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      console.warn(`[a2ui] openUrl blocked for protocol: ${parsed.protocol}`)
      return undefined
    }
    return url
  } catch {
    console.warn(`[a2ui] openUrl invalid URL: ${url}`)
    return undefined
  }
}

/** 校验与逻辑函数注册表。 */
export const validationFunctions: Record<string, (args: Record<string, unknown>) => unknown> = {
  required: _required,
  regex: _regex,
  length: _length,
  numeric: _numeric,
  email: _email,
  and: _and,
  or: _or,
  not: _not,
  openUrl: _openUrl,
}
