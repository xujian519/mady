/**
 * datamodel.ts 黄金对照测试。
 *
 * 移植自 Go 端 a2ui/a2ui_test.go：
 *   - TestDataModelPointerEngine — set 嵌套 / 整根替换 / "-" append / remove key
 *   - TestPointerEscaping — ~0 / ~1 转义与 JoinPointer
 *
 * 断言与 Go 语义 1:1，任何 Go↔TS 行为漂移都应在此暴露。
 */

import { describe, it, expect } from 'vitest'
import {
  parsePointer,
  joinPointer,
  getData,
  applyUpdate,
} from '../datamodel'

// ── parsePointer ─────────────────────────────────

describe('parsePointer', () => {
  it('空字符串与 "/" 都是根指针', () => {
    expect(parsePointer('')).toEqual([])
    expect(parsePointer('/')).toEqual([])
  })

  it('拆分多级路径', () => {
    expect(parsePointer('/user/name')).toEqual(['user', 'name'])
    expect(parsePointer('/a/b/c')).toEqual(['a', 'b', 'c'])
  })
})

// ── TestPointerEscaping（Go）────────────────────

describe('指针转义（TestPointerEscaping）', () => {
  it('~0 解码为 ~，~1 解码为 /', () => {
    expect(parsePointer('/a~0b/c~1d')).toEqual(['a~b', 'c/d'])
  })

  it('joinPointer 自动转义 ~ 和 /', () => {
    expect(joinPointer('a~b', 'c/d')).toBe('/a~0b/c~1d')
  })

  it('joinPointer 空 token 列表返回根 "/"', () => {
    expect(joinPointer()).toBe('/')
  })

  it('转义往返一致', () => {
    const tokens = ['a~b', 'c/d', 'plain']
    expect(parsePointer(joinPointer(...tokens))).toEqual(tokens)
  })
})

// ── getData ─────────────────────────────────────

describe('getData', () => {
  const model = {
    user: { name: 'Ada', tags: ['x', 'y'] },
    count: 3,
  }

  it('读取嵌套值', () => {
    expect(getData(model, '/user/name')).toEqual(['Ada', true])
  })

  it('读取数组元素', () => {
    expect(getData(model, '/user/tags/1')).toEqual(['y', true])
  })

  it('根指针返回整个模型', () => {
    expect(getData(model, '')).toEqual([model, true])
  })

  it('不存在的 key 返回 found=false', () => {
    expect(getData(model, '/user/age')).toEqual([undefined, false])
  })

  it('数组越界返回 found=false', () => {
    expect(getData(model, '/user/tags/9')).toEqual([undefined, false])
  })

  it('路径穿过非对象返回 found=false', () => {
    expect(getData(model, '/count/deeper')).toEqual([undefined, false])
  })
})

// ── TestDataModelPointerEngine（Go）──────────────

describe('applyUpdate（TestDataModelPointerEngine）', () => {
  it('set 嵌套路径（自动创建中间对象）', () => {
    const next = applyUpdate({}, '/user/name', 'Ada', true) as Record<string, unknown>
    expect(next).toEqual({ user: { name: 'Ada' } })
  })

  it('整根替换（空路径）', () => {
    const next = applyUpdate({ a: 1 }, '', { b: 2 }, true)
    expect(next).toEqual({ b: 2 })
  })

  it('整根删除（空路径 + hasValue=false）返回 null', () => {
    expect(applyUpdate({ a: 1 }, '', undefined, false)).toBeNull()
  })

  it('"-" 追加到数组末尾', () => {
    const next = applyUpdate({ list: [1, 2] }, '/list/-', 3, true) as { list: unknown[] }
    expect(next.list).toEqual([1, 2, 3])
  })

  it('按索引 set 数组元素', () => {
    const next = applyUpdate({ list: [1, 2] }, '/list/0', 9, true) as { list: unknown[] }
    expect(next.list).toEqual([9, 2])
  })

  it('remove 对象 key（delete）', () => {
    const next = applyUpdate({ a: 1, b: 2 }, '/a', undefined, false) as Record<string, unknown>
    expect(next).toEqual({ b: 2 })
    expect('a' in next).toBe(false)
  })

  it('remove 数组元素置为 undefined 且不缩短数组', () => {
    const next = applyUpdate({ list: [1, 2] }, '/list/0', undefined, false) as { list: unknown[] }
    expect(next.list.length).toBe(2)
    expect(next.list[0]).toBeUndefined()
    expect(next.list[1]).toBe(2)
  })

  it('不可变更新：原模型不被修改', () => {
    const model = { user: { name: 'Ada' } }
    applyUpdate(model, '/user/name', 'Grace', true)
    expect(model.user.name).toBe('Ada')
  })

  it('model 为 null 时 set 视为从空对象开始', () => {
    const next = applyUpdate(null, '/a', 1, true)
    expect(next).toEqual({ a: 1 })
  })
})
