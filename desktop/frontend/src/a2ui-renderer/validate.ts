/**
 * A2UI 开发期结构校验。
 *
 * 对齐 Go 端 a2ui/validate.go：
 *   - ValidateEnvelope — 检查信封消息体结构
 *   - ValidateSurfaceTree — 检查组件树完整性（root 存在、无 dangling ref、无 cycle）
 *
 * 校验失败时 console.error 但不阻断渲染。
 */

import type { Envelope, SurfaceState } from './store'
import { envelopeKind, RootComponentID } from './store'
import type { Catalog } from './catalog'
import { childRefsOf } from './child-refs'

// ── 校验错误 ──────────────────────────────────────

export interface ValidationError {
  code: string
  surfaceId?: string
  path?: string
  message: string
}

function _err(surfaceId: string, path: string, message: string): ValidationError {
  return { code: 'VALIDATION_FAILED', surfaceId, path, message }
}

// ── Envelope 校验 ─────────────────────────────────

/**
 * 校验单个服务器信封的结构合法性。
 */
export function validateEnvelope(env: Envelope, cat?: Catalog): ValidationError[] {
  const errs: ValidationError[] = []

  const kind = envelopeKind(env)
  if (kind === '') {
    return [_err('', '', 'envelope contains no recognized message body')]
  }

  switch (kind) {
    case 'createSurface': {
      const m = env.createSurface!
      if (!m.surfaceId) {
        errs.push(_err('', '/createSurface/surfaceId', 'surfaceId is required'))
      }
      if (!m.catalogId) {
        errs.push(_err(m.surfaceId ?? '', '/createSurface/catalogId', 'catalogId is required'))
      }
      break
    }
    case 'updateComponents': {
      const m = env.updateComponents!
      if (!m.surfaceId) {
        errs.push(_err('', '/updateComponents/surfaceId', 'surfaceId is required'))
      }
      for (let i = 0; i < (m.components?.length ?? 0); i++) {
        const c = m.components[i]
        const base = `/updateComponents/components/${i}`
        if (!c.id) {
          errs.push(_err(m.surfaceId ?? '', base + '/id', 'component id is required'))
        }
        if (!c.type) {
          errs.push(_err(m.surfaceId ?? '', base + '/component', 'component type is required'))
        } else if (cat && !cat.components[c.type]) {
          errs.push(_err(m.surfaceId ?? '', base + '/component', `unknown component type "${c.type}"`))
        }
      }
      break
    }
    case 'updateDataModel': {
      const m = env.updateDataModel!
      if (!m.surfaceId) {
        errs.push(_err('', '/updateDataModel/surfaceId', 'surfaceId is required'))
      }
      break
    }
    case 'deleteSurface': {
      const m = env.deleteSurface!
      if (!m.surfaceId) {
        errs.push(_err('', '/deleteSurface/surfaceId', 'surfaceId is required'))
      }
      break
    }
  }

  return errs
}

// ── Surface Tree 校验 ─────────────────────────────

/**
 * 校验 surface 组件树的完整性。
 * 检查 root 存在、组件类型、dangling ref、cycle。
 */
export function validateSurfaceTree(srf: SurfaceState, cat?: Catalog): ValidationError[] {
  const errs: ValidationError[] = []

  // root 必须存在
  if (!srf.components.has(RootComponentID)) {
    errs.push(_err(srf.id, '', `surface has no "${RootComponentID}" component`))
  }

  // 检查每个组件
  for (const [id, comp] of srf.components) {
    if (cat && comp.type && !cat.components[comp.type]) {
      errs.push(_err(srf.id, '/' + id, `unknown component type "${comp.type}"`))
    }
    for (const ref of childRefsOf(comp, srf.catalogId)) {
      if (!srf.components.has(ref)) {
        errs.push(_err(srf.id, '/' + id, `references undefined component "${ref}"`))
      }
    }
  }

  // 循环检测（DFS）
  errs.push(..._detectCycles(srf))

  return errs
}

// ── 循环检测 ──────────────────────────────────────

enum Color { White, Gray, Black }

function _detectCycles(srf: SurfaceState): ValidationError[] {
  const color = new Map<string, Color>()
  const errs: ValidationError[] = []

  for (const id of srf.components.keys()) {
    color.set(id, Color.White)
  }

  const visit = (id: string) => {
    const comp = srf.components.get(id)
    if (!comp) return
    color.set(id, Color.Gray)
    for (const ref of childRefsOf(comp, srf.catalogId)) {
      switch (color.get(ref)) {
        case Color.Gray:
          errs.push(_err(srf.id, '/' + id, `circular reference involving "${ref}"`))
          break
        case Color.White:
          visit(ref)
          break
      }
    }
    color.set(id, Color.Black)
  }

  if (srf.components.has(RootComponentID)) {
    visit(RootComponentID)
  }

  // 也检查不在 root 子树中的组件
  for (const id of srf.components.keys()) {
    if (color.get(id) === Color.White) {
      visit(id)
    }
  }

  return errs
}
