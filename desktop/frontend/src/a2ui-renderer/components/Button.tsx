/**
 * A2UI Button 组件 — 按钮。
 *
 * Props:
 *   label: string | Dynamic — 按钮文字
 *   variant: "primary" | "secondary" | "ghost" | "danger"
 *   disabled: boolean | Dynamic
 *   action: Action — 点击后触发的动作
 *
 * 动作分派: action.event → dispatch 到 onAction 回调
 *          action.functionCall → 本地执行
 */

import { useCallback, useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'
import type { Action } from '../store'

export const ButtonComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions)
    const variant = (component.props.variant as string) ?? 'secondary'
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    return { label: String(label ?? ''), variant, disabled }
  }, [component.props, context.surface.dataModel, context.functions])

  const action = component.props.action as Action | undefined

  const handleClick = useCallback(() => {
    if (resolved.disabled || !action) return

    if (action.event && context.onAction) {
      context.onAction(
        context.surface.id,
        component.id,
        action.event.name,
        action.event.context,
      )
    }

    if (action.functionCall && context.functions) {
      const fn = context.functions[action.functionCall.call]
      if (fn) {
        fn(action.functionCall.args ?? {})
      } else {
        console.warn(`[a2ui] Button action function not found: ${action.functionCall.call}`)
      }
    }
  }, [resolved.disabled, action, context, component.id])

  const variantClass = {
    primary: 'bg-mady-accent text-white hover:bg-mady-accent-hover active:bg-mady-accent',
    secondary: 'bg-mady-bg-secondary text-mady-text-primary border border-mady-border hover:bg-mady-bg-tertiary',
    ghost: 'text-mady-text-primary hover:bg-mady-bg-secondary',
    danger: 'bg-mady-danger text-white hover:opacity-90',
  }[resolved.variant] ?? 'bg-mady-bg-secondary text-mady-text-primary border border-mady-border'

  return (
    <button
      className={`px-3 py-1.5 rounded-md text-mady-ui font-medium transition-colors duration-80
        ${variantClass} ${resolved.disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
      disabled={resolved.disabled}
      onClick={handleClick}
      data-a2ui-id={component.id}
    >
      {resolved.label}
    </button>
  )
}
