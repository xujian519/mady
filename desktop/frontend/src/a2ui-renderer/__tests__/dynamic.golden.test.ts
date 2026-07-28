/**
 * dynamic.ts 黄金对照测试。
 *
 * 移植自 Go 端 a2ui/a2ui_test.go：
 *   - TestDynamicMarshaling — literal → "hi"、bind → {"path":...}、
 *     call → {"call":...,"args":{...}} 三种形态识别
 *
 * Go 端测 JSON 序列化形态，TS 端测运行时分类（classifyDynamic），
 * 识别顺序与 Go UnmarshalJSON 完全一致：call → 仅 path 单键 → literal。
 */

import { describe, it, expect, vi } from 'vitest'
import {
  classifyDynamic,
  resolveDynamic,
  resolveBind,
  callFunction,
  clearBindCache,
} from '../dynamic'

// ── TestDynamicMarshaling（Go）──────────────────

describe('classifyDynamic（TestDynamicMarshaling）', () => {
  it('字面量字符串 → literal', () => {
    expect(classifyDynamic('hi')).toEqual({ kind: 'literal', data: 'hi' })
  })

  it('{"path":...} 单键对象 → bind', () => {
    const d = { path: '/user/name' }
    expect(classifyDynamic(d)).toEqual({ kind: 'bind', data: d })
  })

  it('{"call":...,"args":{...}} → call', () => {
    const d = { call: 'formatDate', args: { value: '2024-01-01' } }
    expect(classifyDynamic(d)).toEqual({ kind: 'call', data: d })
  })

  it('call 优先于 path（同时含 call 和 path → call）', () => {
    const d = { call: 'f', path: '/x' }
    expect(classifyDynamic(d).kind).toBe('call')
  })

  it('path 带额外键 → literal（不是 bind）', () => {
    const d = { path: '/x', extra: 1 }
    expect(classifyDynamic(d).kind).toBe('literal')
  })

  it('数组不判 bind/call → literal', () => {
    expect(classifyDynamic([1, 2]).kind).toBe('literal')
    expect(classifyDynamic([]).kind).toBe('literal')
  })

  it('null 与原始类型 → literal', () => {
    expect(classifyDynamic(null).kind).toBe('literal')
    expect(classifyDynamic(42).kind).toBe('literal')
    expect(classifyDynamic(true).kind).toBe('literal')
  })
})

// ── resolveDynamic / resolveBind ────────────────

describe('resolveDynamic', () => {
  const dm = { user: { name: 'Ada' }, n: 2 }

  it('bind 从 dataModel 取值', () => {
    expect(resolveDynamic({ path: '/user/name' }, dm)).toBe('Ada')
  })

  it('bind 未命中返回 undefined', () => {
    expect(resolveDynamic({ path: '/missing' }, dm)).toBeUndefined()
  })

  it('literal 原样返回', () => {
    expect(resolveDynamic('hi', dm)).toBe('hi')
  })

  it('call 经函数注册表执行', () => {
    const functions = {
      double: (args: Record<string, unknown>) => (args.v as number) * 2,
    }
    expect(resolveDynamic({ call: 'double', args: { v: 3 } }, dm, functions)).toBe(6)
  })

  it('call 未注册函数：warn + undefined', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(resolveDynamic({ call: 'nope' }, dm, {})).toBeUndefined()
    expect(warn).toHaveBeenCalledOnce()
    warn.mockRestore()
  })

  it('call 无注册表时返回 undefined', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(resolveDynamic({ call: 'anyFn' }, dm)).toBeUndefined()
    warn.mockRestore()
  })
})

describe('resolveBind 缓存', () => {
  it('同一 dataModel 重复读取命中缓存，结果一致', () => {
    const dm = { a: { b: 1 } }
    expect(resolveBind(dm, '/a/b')).toBe(1)
    expect(resolveBind(dm, '/a/b')).toBe(1)
  })

  it('clearBindCache 后读取新值（dataModel 对象已替换）', () => {
    const dm1 = { a: 1 }
    const dm2 = { a: 2 }
    expect(resolveBind(dm1, '/a')).toBe(1)
    // SurfaceStore.applyDataModel 不可变更新后引用已变，WeakMap 自然隔离
    expect(resolveBind(dm2, '/a')).toBe(2)
    clearBindCache(dm1)
    expect(resolveBind(dm1, '/a')).toBe(1)
  })
})

// ── callFunction ────────────────────────────────

describe('callFunction', () => {
  it('未注册函数 warn 并返回 undefined', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(callFunction('ghost', {}, null, {})).toBeUndefined()
    expect(warn).toHaveBeenCalledWith('[a2ui] unknown function: ghost')
    warn.mockRestore()
  })

  it('已注册函数收到 args', () => {
    const functions = { echo: (args: Record<string, unknown>) => args }
    expect(callFunction('echo', { x: 1 }, null, functions)).toEqual({ x: 1 })
  })
})
