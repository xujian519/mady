/**
 * A2UI CheckBox 组件 — 复选框。
 *
 * Props:
 *   label: string | Dynamic — 标签文本
 *   checked: boolean | Dynamic — 选中状态
 *   disabled: boolean | Dynamic
 */

import { useCallback, useMemo, useState } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const CheckBoxComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions) as string | undefined
    const checked = !!resolveDynamic(component.props.checked, context.surface.dataModel, context.functions)
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    return { label, checked, disabled }
  }, [component.props, context.surface.dataModel, context.functions])

  const [localChecked, setLocalChecked] = useState(resolved.checked)

  const handleChange = useCallback(() => {
    const newVal = !localChecked
    setLocalChecked(newVal)
    if (context.onAction) {
      context.onAction(context.surface.id, component.id, 'inputChanged', {
        value: newVal,
      })
    }
  }, [localChecked, context.onAction, context.surface.id, component.id])

  return (
    <label className="flex items-center gap-2 cursor-pointer select-none">
      <input
        type="checkbox"
        checked={localChecked}
        disabled={resolved.disabled}
        onChange={handleChange}
        className="w-4 h-4 rounded border-mady-border text-mady-accent
          focus:ring-2 focus:ring-mady-accent-glow
          disabled:opacity-50 disabled:cursor-not-allowed"
        data-a2ui-id={component.id}
      />
      {resolved.label && (
        <span className="text-mady-text-primary text-mady-body">{resolved.label}</span>
      )}
    </label>
  )
}
