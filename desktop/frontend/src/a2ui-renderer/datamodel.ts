/**
 * JSON Pointer (RFC 6901) 数据模型引擎。
 *
 * 对齐 Go 端 a2ui/datamodel.go 的语义：
 *   - ParsePointer → parsePointer
 *   - GetData → getData
 *   - ApplyUpdate → applyUpdate
 *   - arrayIndex → arrayIndex
 */

// ── 指针解析 ───────────────────────────────────────

const _unescapeToken = (t: string): string =>
  t.replace(/~1/g, '/').replace(/~0/g, '~')

const _escapeToken = (t: string): string =>
  t.replace(/~/g, '~0').replace(/\//g, '~1')

/**
 * 将 JSON Pointer 拆分为解码后的 token 数组。
 * 根指针 "" 或 "/" 返回空数组。
 */
export function parsePointer(path: string): string[] {
  if (path === '' || path === '/') return []
  const trimmed = path.startsWith('/') ? path.slice(1) : path
  return trimmed.split('/').map(_unescapeToken)
}

/**
 * 从解码后的 token 数组构建绝对 JSON Pointer。
 */
export function joinPointer(...tokens: string[]): string {
  if (tokens.length === 0) return '/'
  return '/' + tokens.map(_escapeToken).join('/')
}

// ── 数据读取 ───────────────────────────────────────

/**
 * 按 JSON Pointer 路径从数据模型中取值。
 * 返回 [value, found]。
 */
export function getData(model: unknown, path: string): [unknown, boolean] {
  let cur: unknown = model
  for (const tok of parsePointer(path)) {
    if (cur === null || typeof cur !== 'object') return [undefined, false]
    if (Array.isArray(cur)) {
      const [idx] = _arrayIndex(tok, cur.length)
      if (idx < 0 || idx >= cur.length) return [undefined, false]
      cur = cur[idx]
    } else {
      const m = cur as Record<string, unknown>
      if (!(tok in m)) return [undefined, false]
      cur = m[tok]
    }
  }
  return [cur, true]
}

// ── 数据更新 ───────────────────────────────────────

/**
 * 在数据模型上应用 updateDataModel 操作。
 * hasValue=true 时设值，false 时删除 key（数组元素置为 undefined）。
 * 返回新的数据模型根对象（不可变更新）。
 */
export function applyUpdate(
  model: unknown,
  path: string,
  value: unknown,
  hasValue: boolean,
): unknown {
  const tokens = parsePointer(path)
  if (tokens.length === 0) {
    return hasValue ? value : null
  }
  const root = model ?? ({} as Record<string, unknown>)
  return _applyTokens(structuredClone(root), tokens, value, hasValue)
}

function _applyTokens(
  node: unknown,
  tokens: string[],
  value: unknown,
  hasValue: boolean,
): unknown {
  const key = tokens[0]
  const last = tokens.length === 1

  if (Array.isArray(node)) {
    const [idx, isAppend] = _arrayIndex(key, node.length)
    if (last) {
      if (isAppend) {
        if (hasValue) node.push(value)
        return node
      }
      if (idx < 0 || idx >= node.length) return node
      if (hasValue) {
        node[idx] = value
      } else {
        node[idx] = undefined
      }
      return node
    }
    if (isAppend || idx < 0 || idx >= node.length) return node
    const child = node[idx] ?? ({} as Record<string, unknown>)
    node[idx] = _applyTokens(child, tokens.slice(1), value, hasValue)
    return node
  }

  const m = node as Record<string, unknown>
  if (last) {
    if (hasValue) {
      m[key] = value
    } else {
      delete m[key]
    }
    return m
  }
  const child = m[key] ?? ({} as Record<string, unknown>)
  m[key] = _applyTokens(child, tokens.slice(1), value, hasValue)
  return m
}

// ── 辅助 ───────────────────────────────────────────

/**
 * 解析数组索引 token。"-" 表示 append。
 * 返回 [index, isAppend]。
 */
function _arrayIndex(tok: string, length: number): [number, boolean] {
  if (tok === '-') return [length, true]
  const idx = parseInt(tok, 10)
  if (isNaN(idx)) return [-1, false]
  return [idx, false]
}
