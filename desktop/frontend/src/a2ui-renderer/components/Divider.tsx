/**
 * A2UI Divider 组件 — 分割线。
 *
 * Props:
 *   orientation: "horizontal" | "vertical"
 *   label: string | Dynamic — 可选标签
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const DividerComponent: A2UIComponent = ({ component, context }) => {
  const orientation = (component.props.orientation as string) ?? 'horizontal'
  const label = useMemo(
    () => resolveDynamic(component.props.label, context.surface.dataModel, context.functions) as string | undefined,
    [component.props.label, context.surface.dataModel, context.functions],
  )

  if (orientation === 'vertical') {
    return (
      <div className="w-px bg-mady-separator self-stretch mx-2" />
    )
  }

  if (label) {
    return (
      <div className="flex items-center gap-3">
        <div className="flex-1 h-px bg-mady-separator" />
        <span className="text-mady-text-tertiary text-mady-caption shrink-0">{label}</span>
        <div className="flex-1 h-px bg-mady-separator" />
      </div>
    )
  }

  return <div className="w-full h-px bg-mady-separator my-2" />
}
