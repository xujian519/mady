/**
 * functions/ 函数注册表测试。
 *
 * 覆盖 BasicCatalog 14 个内置函数的重点路径：
 *   format 5 个 — formatString / formatNumber / formatCurrency / formatDate / pluralize
 *   validation 9 个 — required / regex / length / numeric / email / and / or / not / openUrl
 *
 * openUrl 是安全敏感函数：仅放行 http/https，其余协议必须拦截。
 */

import { describe, it, expect, vi } from 'vitest'
import { functionRegistry } from '../functions'

const EXPECTED = [
  'formatString', 'formatNumber', 'formatCurrency', 'formatDate', 'pluralize',
  'required', 'regex', 'length', 'numeric', 'email',
  'and', 'or', 'not', 'openUrl',
]

describe('functionRegistry', () => {
  it('恰好注册 14 个 BasicCatalog 函数', () => {
    expect(Object.keys(functionRegistry).sort()).toEqual(EXPECTED.sort())
  })
})

describe('format 函数', () => {
  it('formatString 用 {n} 占位替换', () => {
    expect(
      functionRegistry.formatString({ template: '{0} world', arg0: 'hello' }),
    ).toBe('hello world')
    expect(functionRegistry.formatString({})).toBe('')
  })

  it('formatNumber 支持 decimals 与千分位', () => {
    expect(functionRegistry.formatNumber({ value: 3.14159, decimals: 2 })).toBe('3.14')
    expect(functionRegistry.formatNumber({})).toBe('')
  })

  it('formatCurrency 返回非空字符串', () => {
    const out = functionRegistry.formatCurrency({ value: 1234.5, currency: 'CNY' })
    expect(typeof out).toBe('string')
    expect((out as string).length).toBeGreaterThan(0)
    expect(functionRegistry.formatCurrency({})).toBe('')
  })

  it('formatDate 按格式占位符输出', () => {
    expect(
      functionRegistry.formatDate({ value: '2024-03-05T10:20:30', format: 'YYYY-MM-DD' }),
    ).toBe('2024-03-05')
    expect(
      functionRegistry.formatDate({ value: '2024-03-05T10:20:30', format: 'HH:mm:ss' }),
    ).toBe('10:20:30')
    // 非法日期原样返回
    expect(functionRegistry.formatDate({ value: 'not-a-date' })).toBe('not-a-date')
    expect(functionRegistry.formatDate({})).toBe('')
  })

  it('pluralize 按 count 选 one/other', () => {
    expect(functionRegistry.pluralize({ count: 1, one: 'item', other: 'items' })).toBe('item')
    expect(functionRegistry.pluralize({ count: 5, one: 'item', other: 'items' })).toBe('items')
    expect(functionRegistry.pluralize({})).toBe('')
  })
})

describe('validation 函数', () => {
  it('required：null/undefined/空白字符串为 false', () => {
    expect(functionRegistry.required({ value: null })).toBe(false)
    expect(functionRegistry.required({})).toBe(false)
    expect(functionRegistry.required({ value: '  ' })).toBe(false)
    expect(functionRegistry.required({ value: 'x' })).toBe(true)
    expect(functionRegistry.required({ value: 0 })).toBe(true)
  })

  it('regex：匹配与非法模式', () => {
    expect(functionRegistry.regex({ value: 'abc123', pattern: '\\d+' })).toBe(true)
    expect(functionRegistry.regex({ value: 'abc', pattern: '^\\d+$' })).toBe(false)
    expect(functionRegistry.regex({ value: 'x', pattern: '([' })).toBe(false)
  })

  it('length：min/max 边界', () => {
    expect(functionRegistry.length({ value: 'abc', min: 2, max: 3 })).toBe(true)
    expect(functionRegistry.length({ value: 'a', min: 2 })).toBe(false)
    expect(functionRegistry.length({ value: 'abcd', max: 3 })).toBe(false)
  })

  it('numeric：数字与数字字符串', () => {
    expect(functionRegistry.numeric({ value: 42 })).toBe(true)
    expect(functionRegistry.numeric({ value: '3.14' })).toBe(true)
    expect(functionRegistry.numeric({ value: 'abc' })).toBe(false)
    expect(functionRegistry.numeric({ value: '' })).toBe(false)
  })

  it('email：基础格式', () => {
    expect(functionRegistry.email({ value: 'a@b.com' })).toBe(true)
    expect(functionRegistry.email({ value: 'not-an-email' })).toBe(false)
  })

  it('and / or / not 逻辑组合', () => {
    expect(functionRegistry.and({ values: [true, true] })).toBe(true)
    expect(functionRegistry.and({ values: [true, false] })).toBe(false)
    expect(functionRegistry.or({ values: [false, true] })).toBe(true)
    expect(functionRegistry.or({ values: [false, false] })).toBe(false)
    expect(functionRegistry.not({ value: true })).toBe(false)
    expect(functionRegistry.not({ value: 0 })).toBe(true)
  })
})

describe('openUrl 协议拦截（安全敏感）', () => {
  it('放行 http/https', () => {
    expect(functionRegistry.openUrl({ url: 'https://example.com' })).toBe('https://example.com')
    expect(functionRegistry.openUrl({ url: 'http://example.com/path?q=1' })).toBe(
      'http://example.com/path?q=1',
    )
  })

  it('拦截 javascript: / file: 等非 http 协议', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(functionRegistry.openUrl({ url: 'javascript:alert(1)' })).toBeUndefined()
    expect(functionRegistry.openUrl({ url: 'file:///etc/passwd' })).toBeUndefined()
    expect(functionRegistry.openUrl({ url: 'data:text/html,<script>1</script>' })).toBeUndefined()
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })

  it('非法 URL 与缺参返回 undefined', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    expect(functionRegistry.openUrl({ url: 'not a url' })).toBeUndefined()
    expect(functionRegistry.openUrl({})).toBeUndefined()
    warn.mockRestore()
  })
})
