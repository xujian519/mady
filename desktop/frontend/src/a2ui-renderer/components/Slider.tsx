/**
 * A2UI Slider 组件 — 滑块。
 *
 * Props:
 *   label: string | Dynamic — 标签
 *   min: number — 最小值
 *   max: number — 最大值
 *   step: number — 步长
 *   value: number | Dynamic — 当前值
 *   disabled: boolean | Dynamic
 */

import { useCallback, useMemo, useState } from 'react'
import type React from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const SliderComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions) as string | undefined
    const value = resolveDynamic(component.props.value, context.surface.dataModel, context.functions) as number | undefined
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    const min = (component.props.min as number) ?? 0
    const max = (component.props.max as number) ?? 100
    const step = (component.props.step as number) ?? 1
    return { label, value: value ?? min, disabled, min, max, step }
  }, [component.props, context.surface.dataModel, context.functions])

  const [localValue, setLocalValue] = useState(resolved.value)

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newVal = Number(e.target.value)
      setLocalValue(newVal)
      if (context.onAction) {
        context.onAction(context.surface.id, component.id, 'inputChanged', {
          value: newVal,
        })
      }
    },
    [context.onAction, context.surface.id, component.id],
  )

  return (
    <div className="flex flex-col gap-1">
      {resolved.label && (
        <div className="flex items-center justify-between">
          <label className="text-mady-text-secondary text-mady-ui">{resolved.label}</label>
          <span className="text-mady-text-tertiary text-mady-caption">{localValue}</span>
        </div>
      )}
      <input
        type="range"
        min={resolved.min}
        max={resolved.max}
        step={resolved.step}
        value={localValue}
        disabled={resolved.disabled}
        onChange={handleChange}
        className="w-full h-1.5 rounded-full appearance-none cursor-pointer
          bg-mady-bg-secondary accent-mady-accent
          disabled:opacity-50 disabled:cursor-not-allowed"
        data-a2ui-id={component.id}
      />
    </div>
  )
}
