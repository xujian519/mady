/**
 * A2UI DateTimeInput 组件 — 日期/时间输入。
 *
 * Props:
 *   label: string | Dynamic — 标签
 *   value: string | Dynamic — ISO 日期字符串
 *   type: "date" | "time" | "datetime-local"
 *   disabled: boolean | Dynamic
 */

import { useCallback, useMemo, useState } from 'react'
import type React from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const DateTimeInputComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions) as string | undefined
    const value = resolveDynamic(component.props.value, context.surface.dataModel, context.functions) as string | undefined
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    const type = (component.props.type as string) ?? 'date'
    return { label, value: value ?? '', disabled, type }
  }, [component.props, context.surface.dataModel, context.functions])

  const [localValue, setLocalValue] = useState(resolved.value)

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newVal = e.target.value
      setLocalValue(newVal)
      if (context.onAction) {
        context.onAction(context.surface.id, component.id, 'inputChanged', {
          value: newVal,
        })
      }
    },
    [context, component.id],
  )

  return (
    <div className="flex flex-col gap-1">
      {resolved.label && (
        <label className="text-mady-text-secondary text-mady-ui">{resolved.label}</label>
      )}
      <input
        type={resolved.type}
        value={localValue}
        disabled={resolved.disabled}
        onChange={handleChange}
        className="px-3 py-1.5 rounded-md border border-mady-border bg-mady-bg-primary
          text-mady-text-primary text-mady-body
          focus:outline-none focus:ring-2 focus:ring-mady-accent-glow focus:border-mady-accent
          disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-80"
        data-a2ui-id={component.id}
      />
    </div>
  )
}
