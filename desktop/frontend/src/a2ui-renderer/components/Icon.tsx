/**
 * A2UI Icon 组件 — 渲染图标。
 *
 * Props:
 *   icon: string — lucide-react 图标名
 *   size: number — 图标尺寸（默认 16）
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'
import * as LucideIcons from 'lucide-react'

export const IconComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const iconName = resolveDynamic(component.props.icon, context.surface.dataModel, context.functions) as string | undefined
    const size = (resolveDynamic(component.props.size, context.surface.dataModel, context.functions) as number) ?? 16
    return { iconName: iconName ?? 'HelpCircle', size }
  }, [component.props.icon, component.props.size, context.surface.dataModel, context.functions])

  const IconComp = useMemo(() => {
    const name = resolved.iconName.charAt(0).toUpperCase() + resolved.iconName.slice(1)
    return (LucideIcons as unknown as Record<string, React.FC<{ size?: number; className?: string }>>)[name]
  }, [resolved.iconName])

  if (!IconComp) {
    return <span className="text-mady-text-tertiary">?{resolved.iconName}</span>
  }

  return <IconComp size={resolved.size} className="inline-block align-middle" />
}
