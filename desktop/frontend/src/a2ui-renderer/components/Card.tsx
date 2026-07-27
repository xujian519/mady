/**
 * A2UI Card 组件 — 卡片容器。
 *
 * Props:
 *   title: string | Dynamic — 卡片标题（可选）
 *   variant: "default" | "elevated" | "outlined"
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const CardComponent: A2UIComponent = ({ component, context, children }) => {
  const resolved = useMemo(() => {
    const title = resolveDynamic(component.props.title, context.surface.dataModel, context.functions)
    const variant = (component.props.variant as string) ?? 'default'
    return { title: title as string | undefined, variant }
  }, [component.props.title, component.props.variant, context.surface.dataModel, context.functions])

  const variantClass = {
    default: 'bg-mady-bg-secondary border border-mady-border',
    elevated: 'bg-mady-bg-primary shadow-sm border border-mady-border',
    outlined: 'bg-transparent border border-mady-border',
  }[resolved.variant] ?? 'bg-mady-bg-secondary border border-mady-border'

  return (
    <div className={`rounded-lg overflow-hidden ${variantClass}`}>
      {resolved.title && (
        <div className="px-4 pt-3 pb-1 text-mady-text-secondary text-mady-ui font-medium">
          {resolved.title}
        </div>
      )}
      <div className="p-4">
        {children}
      </div>
    </div>
  )
}
