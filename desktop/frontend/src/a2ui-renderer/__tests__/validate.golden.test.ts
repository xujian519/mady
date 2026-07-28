/**
 * validate.ts 黄金对照测试。
 *
 * 移植自 Go 端 a2ui/a2ui_test.go：
 *   - TestValidateEnvelope — createSurface 双空=2 错、未知组件类型=1 错、合法=0 错
 *   - TestValidateSurfaceTree — 缺 root / dangling ref / 合法
 *   - TestValidateDetectsCycle — circular reference
 */

import { describe, it, expect } from 'vitest'
import { validateEnvelope, validateSurfaceTree } from '../validate'
import { basicCatalog } from '../catalog'
import type { SurfaceState, Component } from '../store'

const CATALOG = 'https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json'

function makeSurface(components: Component[]): SurfaceState {
  return {
    id: 's1',
    catalogId: CATALOG,
    theme: {},
    sendDataModel: false,
    components: new Map(components.map((c) => [c.id, c])),
    dataModel: {},
  }
}

// ── TestValidateEnvelope（Go）───────────────────

describe('validateEnvelope（TestValidateEnvelope）', () => {
  it('空信封 = 1 错（no recognized message body）', () => {
    const errs = validateEnvelope({})
    expect(errs).toHaveLength(1)
    expect(errs[0].message).toContain('no recognized message body')
  })

  it('createSurface 双空 = 2 错', () => {
    const errs = validateEnvelope({
      createSurface: { surfaceId: '', catalogId: '' },
    })
    expect(errs).toHaveLength(2)
  })

  it('未知组件类型 = 1 错，含 "unknown component"', () => {
    const errs = validateEnvelope(
      {
        updateComponents: {
          surfaceId: 's1',
          components: [{ id: 'c1', type: 'NotAComponent', props: {} }],
        },
      },
      basicCatalog(),
    )
    expect(errs).toHaveLength(1)
    expect(errs[0].message).toContain('unknown component')
  })

  it('合法信封 = 0 错', () => {
    expect(
      validateEnvelope({
        createSurface: { surfaceId: 's1', catalogId: CATALOG },
      }),
    ).toHaveLength(0)
    expect(
      validateEnvelope(
        {
          updateComponents: {
            surfaceId: 's1',
            components: [{ id: 'c1', type: 'Text', props: {} }],
          },
        },
        basicCatalog(),
      ),
    ).toHaveLength(0)
  })

  it('组件缺 id / type 各报 1 错', () => {
    const errs = validateEnvelope({
      updateComponents: {
        surfaceId: 's1',
        components: [{ id: '', type: '', props: {} }],
      },
    })
    expect(errs).toHaveLength(2)
  })
})

// ── TestValidateSurfaceTree（Go）────────────────

describe('validateSurfaceTree（TestValidateSurfaceTree）', () => {
  it('缺 root 组件 → 含 no "root" 错误', () => {
    const srf = makeSurface([{ id: 't1', type: 'Text', props: {} }])
    const errs = validateSurfaceTree(srf)
    expect(errs.some((e) => e.message.includes('no "root"'))).toBe(true)
  })

  it('dangling ref → 含 "undefined component"', () => {
    const srf = makeSurface([
      { id: 'root', type: 'Column', props: { children: ['ghost'] } },
    ])
    const errs = validateSurfaceTree(srf)
    expect(errs.some((e) => e.message.includes('undefined component'))).toBe(true)
  })

  it('合法树 = 0 错', () => {
    const srf = makeSurface([
      { id: 'root', type: 'Column', props: { children: ['t1'] } },
      { id: 't1', type: 'Text', props: { text: 'hi' } },
    ])
    expect(validateSurfaceTree(srf, basicCatalog())).toHaveLength(0)
  })

  it('未知组件类型（带 catalog）→ 含 "unknown component"', () => {
    const srf = makeSurface([{ id: 'root', type: 'Nope', props: {} }])
    const errs = validateSurfaceTree(srf, basicCatalog())
    expect(errs.some((e) => e.message.includes('unknown component'))).toBe(true)
  })
})

// ── TestValidateDetectsCycle（Go）───────────────

describe('validateSurfaceTree 循环检测（TestValidateDetectsCycle）', () => {
  it('直接自引用 → 含 "circular reference"', () => {
    const srf = makeSurface([
      { id: 'root', type: 'Column', props: { children: ['root'] } },
    ])
    const errs = validateSurfaceTree(srf)
    expect(errs.some((e) => e.message.includes('circular reference'))).toBe(true)
  })

  it('间接环 a → b → a → 含 "circular reference"', () => {
    const srf = makeSurface([
      { id: 'root', type: 'Column', props: { children: ['a'] } },
      { id: 'a', type: 'Card', props: { child: 'b' } },
      { id: 'b', type: 'Card', props: { child: 'a' } },
    ])
    const errs = validateSurfaceTree(srf)
    expect(errs.some((e) => e.message.includes('circular reference'))).toBe(true)
  })
})
