/**
 * A2UI Text 组件 — 渲染文本内容。
 *
 * Props:
 *   content: string | Dynamic — 文本内容
 *   variant: "body" | "heading" | "caption" — 变体
 *
 * 与 Go a2ui.TextProps 对齐。
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const TextComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const content = resolveDynamic(component.props.content, context.surface.dataModel, context.functions)
    const variant = component.props.variant as string | undefined
    return { content: content ?? '', variant }
  }, [component.props, context.surface.dataModel, context.functions])

  const className = useMemo(() => {
    switch (resolved.variant) {
      case 'heading': return 'text-mady-text-primary font-semibold'
      case 'caption': return 'text-mady-text-tertiary text-mady-caption'
      default: return 'text-mady-text-primary text-mady-body'
    }
  }, [resolved.variant])

  const content = resolved.content

  // null/undefined → 不渲染；number/boolean/string → React 原生处理
  if (content == null) return null

  // resolveDynamic 返回 unknown，安全转换到 ReactNode
  const node: React.ReactNode =
    typeof content === 'string' || typeof content === 'number' || typeof content === 'boolean'
      ? content
      : String(content)

  return <span className={className}>{node}</span>
}
