/**
 * 子组件引用解析。
 *
 * 从 Component.props 中提取所有子组件 ID（对齐 Go a2ui/surface.go childRefs）。
 * 使用 basicCatalog 的 ComponentDef 确定哪些 props 是结构引用。
 */

import type { Component } from './store'
import { BasicCatalogID, basicCatalog, type ComponentDef } from './catalog'

const _basic = basicCatalog()

/** 从 props 中提取子组件 ID。 */
export function childRefsOf(comp: Component, catalogID?: string): string[] {
  const catId = catalogID ?? BasicCatalogID
  const def = catId === BasicCatalogID ? _basic.components[comp.type] : undefined
  return _extractRefs(comp.props, def)
}

function _extractRefs(props: Record<string, unknown>, def?: ComponentDef): string[] {
  const refs: string[] = []

  const singleFields = def?.childFields ?? ['child']
  const listFields = def?.childListFields ?? ['children']
  const nestedFields = def?.nestedChildFields ?? {}

  for (const f of singleFields) {
    const id = props[f]
    if (typeof id === 'string' && id !== '') {
      refs.push(id)
    }
  }

  for (const f of listFields) {
    const v = props[f]
    if (Array.isArray(v)) {
      for (const e of v) {
        if (typeof e === 'string' && e !== '') {
          refs.push(e)
        }
      }
    } else if (v && typeof v === 'object') {
      const obj = v as Record<string, unknown>
      if (typeof obj.componentId === 'string') {
        refs.push(obj.componentId)
      }
    }
  }

  for (const [prop, key] of Object.entries(nestedFields)) {
    const arr = props[prop]
    if (Array.isArray(arr)) {
      for (const item of arr) {
        if (item && typeof item === 'object') {
          const id = (item as Record<string, unknown>)[key]
          if (typeof id === 'string' && id !== '') {
            refs.push(id)
          }
        }
      }
    }
  }

  return refs
}
