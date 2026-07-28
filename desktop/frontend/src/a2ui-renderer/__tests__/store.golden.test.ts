/**
 * store.ts 黄金对照测试。
 *
 * 移植自 Go 端 a2ui/a2ui_test.go：
 *   - TestSurfaceStoreLifecycle — update 先于 create 报错、重复 create 报错、
 *     delete unknown 为 no-op
 *   - TestClientDataModelCollection — 只聚合 sendDataModel=true 的 surface
 *   - TestUpdateDataModelRemoveVsSet — value 省略=删除、"value":null=显式设值
 *   - TestChildListMarshaling — 静态数组 vs {path, componentId} 模板
 *
 * 其中 TestUpdateDataModelRemoveVsSet 是 G3 修复过的 valueSet 漂移点的回归护栏。
 */

import { describe, it, expect } from 'vitest'
import {
  SurfaceStore,
  SurfaceExistsError,
  SurfaceNotFoundError,
  componentFromFlat,
  parseChildList,
  envelopeKind,
  envelopeSurfaceID,
  A2UI_VERSION,
} from '../store'

const CATALOG = 'https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json'

function createEnv(surfaceId: string, sendDataModel?: boolean) {
  return {
    version: A2UI_VERSION,
    createSurface: { surfaceId, catalogId: CATALOG, sendDataModel },
  }
}

// ── TestSurfaceStoreLifecycle（Go）───────────────

describe('SurfaceStore 生命周期（TestSurfaceStoreLifecycle）', () => {
  it('update 先于 create 报 SurfaceNotFoundError', () => {
    const store = new SurfaceStore()
    expect(() =>
      store.apply({ updateComponents: { surfaceId: 's1', components: [] } }),
    ).toThrow(SurfaceNotFoundError)
    expect(() =>
      store.apply({ updateDataModel: { surfaceId: 's1', path: '/a', value: 1 } }),
    ).toThrow(SurfaceNotFoundError)
  })

  it('重复 createSurface 报 SurfaceExistsError', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    expect(() => store.apply(createEnv('s1'))).toThrow(SurfaceExistsError)
  })

  it('createSurface 缺 surfaceId 报错', () => {
    const store = new SurfaceStore()
    expect(() =>
      store.apply({ createSurface: { surfaceId: '', catalogId: CATALOG } }),
    ).toThrow(/surfaceId/)
  })

  it('deleteSurface 未知 surface 为 no-op', () => {
    const store = new SurfaceStore()
    expect(() =>
      store.apply({ deleteSurface: { surfaceId: 'nope' } }),
    ).not.toThrow()
  })

  it('完整流程：create → updateComponents → delete', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    store.apply({
      updateComponents: {
        surfaceId: 's1',
        components: [
          // 线格式："component" 键携带类型（对齐 Go Component.UnmarshalJSON）
          { id: 'root', component: 'Column', children: ['t1'] },
          { id: 't1', component: 'Text', text: 'hello' },
        ] as never,
      },
    })
    const srf = store.surface('s1')!
    expect(srf.components.get('root')!.type).toBe('Column')
    expect(srf.components.get('t1')!.props.text).toBe('hello')

    store.apply({ deleteSurface: { surfaceId: 's1' } })
    expect(store.surface('s1')).toBeUndefined()
  })
})

// ── TestClientDataModelCollection（Go）───────────

describe('clientDataModel（TestClientDataModelCollection）', () => {
  it('只聚合 sendDataModel=true 的 surface', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('send', true))
    store.apply(createEnv('nosend', false))
    store.apply(createEnv('default')) // 省略 sendDataModel → false
    store.apply({
      updateDataModel: { surfaceId: 'send', path: '/x', value: 1 },
    })
    store.apply({
      updateDataModel: { surfaceId: 'nosend', path: '/y', value: 2 },
    })

    const payload = store.clientDataModel()
    expect(Object.keys(payload.surfaces)).toEqual(['send'])
    expect(payload.surfaces.send).toEqual({ x: 1 })
  })
})

// ── TestUpdateDataModelRemoveVsSet（Go）──────────

describe('updateDataModel remove vs set（TestUpdateDataModelRemoveVsSet）', () => {
  it('value 键省略 = 删除（Go wire 语义：键存在性判定）', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    store.apply({ updateDataModel: { surfaceId: 's1', path: '/a', value: 1 } })
    expect(store.surface('s1')!.dataModel).toEqual({ a: 1 })

    // 省略 value 键 → remove
    store.apply({ updateDataModel: { surfaceId: 's1', path: '/a' } })
    expect(store.surface('s1')!.dataModel).toEqual({})
  })

  it('"value":null = 显式设为 null（不是删除）', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    store.apply({ updateDataModel: { surfaceId: 's1', path: '/a', value: null } })
    const dm = store.surface('s1')!.dataModel as Record<string, unknown>
    expect('a' in dm).toBe(true)
    expect(dm.a).toBeNull()
  })

  it('valueSet 兼容字段：valueSet=false 且省略 value 时为删除', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    store.apply({ updateDataModel: { surfaceId: 's1', path: '/a', value: 1 } })
    store.apply({
      updateDataModel: { surfaceId: 's1', path: '/a', valueSet: false },
    })
    expect(store.surface('s1')!.dataModel).toEqual({})
  })

  it('path 省略视为根：省略 value 时整根置 null', () => {
    const store = new SurfaceStore()
    store.apply(createEnv('s1'))
    store.apply({ updateDataModel: { surfaceId: 's1', path: '/a', value: 1 } })
    store.apply({ updateDataModel: { surfaceId: 's1' } })
    expect(store.surface('s1')!.dataModel).toBeNull()
  })
})

// ── componentFromFlat / parseChildList ───────────

describe('componentFromFlat', () => {
  it('"component" 键映射为 type，id 保留，其余进 props', () => {
    const c = componentFromFlat({
      id: 't1',
      component: 'Text',
      text: 'hi',
      style: { bold: true },
    })
    expect(c.id).toBe('t1')
    expect(c.type).toBe('Text')
    expect(c.props).toEqual({ text: 'hi', style: { bold: true } })
  })
})

describe('parseChildList（TestChildListMarshaling）', () => {
  it('字符串数组 → 静态列表', () => {
    expect(parseChildList(['a', 'b'])).toEqual({ static: ['a', 'b'] })
  })

  it('{path, componentId} → 模板', () => {
    expect(parseChildList({ path: '/items', componentId: 'item' })).toEqual({
      template: { path: '/items', componentId: 'item' },
    })
  })

  it('其他值 → undefined', () => {
    expect(parseChildList(42)).toBeUndefined()
    expect(parseChildList({ path: '/items' })).toBeUndefined()
    expect(parseChildList(null)).toBeUndefined()
  })
})

// ── envelopeKind / envelopeSurfaceID ─────────────

describe('信封识别', () => {
  it('四种消息体', () => {
    expect(envelopeKind({ createSurface: { surfaceId: 's', catalogId: 'c' } })).toBe('createSurface')
    expect(envelopeKind({ updateComponents: { surfaceId: 's', components: [] } })).toBe('updateComponents')
    expect(envelopeKind({ updateDataModel: { surfaceId: 's' } })).toBe('updateDataModel')
    expect(envelopeKind({ deleteSurface: { surfaceId: 's' } })).toBe('deleteSurface')
    expect(envelopeKind({})).toBe('')
  })

  it('envelopeSurfaceID 提取目标 surface', () => {
    expect(envelopeSurfaceID({ deleteSurface: { surfaceId: 's9' } })).toBe('s9')
    expect(envelopeSurfaceID({})).toBe('')
  })
})
