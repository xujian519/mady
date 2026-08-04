/**
 * A2UI ChoicePicker 组件 — 选项选择器。
 *
 * Props:
 *   label: string | Dynamic — 标签
 *   options: Array<{ value: string; label: string }> — 选项列表
 *   value: string | Dynamic — 当前选中值
 *   disabled: boolean | Dynamic
 */

import { useCallback, useMemo, useState } from 'react'
import type React from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const ChoicePickerComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions) as string | undefined
    const value = resolveDynamic(component.props.value, context.surface.dataModel, context.functions) as string | undefined
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    const options = (component.props.options as Array<{ value: string; label: string }>) ?? []
    return { label, value: value ?? '', disabled, options }
  }, [component.props, context.surface.dataModel, context.functions])

  const [localValue, setLocalValue] = useState(resolved.value)

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
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
      <select
        value={localValue}
        disabled={resolved.disabled}
        onChange={handleChange}
        className="px-3 py-1.5 rounded-md border border-mady-border bg-mady-bg-primary
          text-mady-text-primary text-mady-body
          focus:outline-none focus:ring-2 focus:ring-mady-accent-glow focus:border-mady-accent
          disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-80"
        data-a2ui-id={component.id}
      >
        {resolved.options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  )
}
