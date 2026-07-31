/**
 * A2UI 组件树渲染器。
 *
 * 将 SurfaceStore 中的 surface + 组件树映射为 React 元素树。
 * 递归遍历组件邻接表，按注册表分派渲染。
 *
 * 用法：
 * ```tsx
 * <A2UISurface surfaceId="main" store={surfaceStore} />
 * ```
 */

import React, { useMemo } from 'react'
import type { SurfaceStore } from './store'
import { RootComponentID } from './store'
import { getComponent } from './registry'
import { childRefsOf } from './child-refs'
import { resolveDynamic } from './dynamic'
import type { RenderContext } from './registry'
import { useA2UIStore } from './a2ui-store'

// ── Props ─────────────────────────────────────────

export interface A2UIRendererProps {
  /** SurfaceStore 实例。 */
  store: SurfaceStore
  /** 要渲染的 surface ID。 */
  surfaceId: string
  /** 函数注册表。 */
  functions?: Record<string, (args: Record<string, unknown>) => unknown>
  /** 组件交互回调。 */
  onAction?: RenderContext['onAction']
}

// ── 渲染器 ────────────────────────────────────────

/**
 * 渲染单个 A2UI surface 的完整组件树。
 */
export const A2UISurface: React.FC<A2UIRendererProps> = ({
  store,
  surfaceId,
  functions,
  onAction,
}) => {
  // F-B2：surface 状态从 zustand 快照订阅——applyEnvelope 后受影响的
  // surfaces[surfaceId] 是「新引用」（见 a2ui-store.applyEnvelope 的浅拷贝），
  // context 随之重建，React.memo 层不再被引用相等性 bail out。
  // 不再直接读 store.surface(surfaceId)：底层 SurfaceStore 原地 mutate，
  // 旧实现下 surface 引用永不变化，dataModel/组件更新后 UI 冻结。
  const surface = useA2UIStore((s) => s.surfaces[surfaceId])

  const context: RenderContext = useMemo(
    () => ({
      surface: surface!,
      functions,
      onAction,
    }),
    [surface, functions, onAction],
  )

  if (!context.surface) return null

  return (
    <A2UINode
      componentId={RootComponentID}
      context={context}
      store={store}
    />
  )
}

// ── 内部节点 ──────────────────────────────────────

interface A2UINodeProps {
  componentId: string
  context: RenderContext
  store: SurfaceStore
}

/** 递归渲染单个节点及其子节点。 */
const A2UINode: React.FC<A2UINodeProps> = React.memo(({ componentId, context, store }) => {
  const comp = context.surface.components.get(componentId)

  // 所有 hooks 必须在条件返回之前（rules-of-hooks）。
  const children = useMemo(() => {
    if (!comp) return []
    const refs = childRefsOf(comp, context.surface.catalogId)
    return refs.map((childId) => (
      <A2UINode
        key={childId}
        componentId={childId}
        context={context}
        store={store}
      />
    ))
  }, [comp, context])

  if (!comp) {
    console.warn(`[a2ui] component not found: ${componentId}`)
    return null
  }

  const Component = getComponent(comp.type)

  return (
    <Component component={comp} context={context}>
      {children}
    </Component>
  )
})

// ── 辅助导出 ──────────────────────────────────────

/**
 * 解析组件 props 中的 Dynamic 值。
 * 所有 A2UI 组件可使用此工具处理其 props。
 */
export function resolveProps(
  props: Record<string, unknown>,
  dataModel: unknown,
  functions?: Record<string, (args: Record<string, unknown>) => unknown>,
): Record<string, unknown> {
  const resolved: Record<string, unknown> = {}
  for (const [key, val] of Object.entries(props)) {
    resolved[key] = resolveDynamic(val, dataModel, functions)
  }
  return resolved
}
