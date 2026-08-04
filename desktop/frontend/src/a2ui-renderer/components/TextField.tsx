/**
 * A2UI TextField 组件 — 文本输入框。
 *
 * Props:
 *   label: string | Dynamic — 标签
 *   placeholder: string | Dynamic — 占位文本
 *   value: string — 当前值（数据绑定 path 通过 Input 约定实现）
 *   multiline: boolean — 多行模式
 *   disabled: boolean | Dynamic
 *
 * Input 组件的双向绑定：
 *   - 创建时 value 从 data model 的绑定路径读取
 *   - 用户输入后通过 onAction 回传 "inputChanged" 事件
 */

import { useCallback, useMemo, useState } from 'react'
import type React from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const TextFieldComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const label = resolveDynamic(component.props.label, context.surface.dataModel, context.functions)
    const placeholder = resolveDynamic(component.props.placeholder, context.surface.dataModel, context.functions)
    const multiline = !!component.props.multiline
    const disabled = !!resolveDynamic(component.props.disabled, context.surface.dataModel, context.functions)
    const initialValue = resolveDynamic(component.props.value ?? component.props.defaultValue, context.surface.dataModel, context.functions)
    return {
      label: label as string | undefined,
      placeholder: String(placeholder ?? ''),
      multiline,
      disabled,
      initialValue: String(initialValue ?? ''),
    }
  }, [component.props, context.surface.dataModel, context.functions])

  const [value, setValue] = useState(resolved.initialValue)
  const [syncedInitial, setSyncedInitial] = useState(resolved.initialValue)

  // 当 data model 更新时同步初始值
  // （React 官方 derived-state 模式：render 期间调用 setState，React 会丢弃本次渲染结果并重渲染；
  //   不用 ref 写入，避免 react-hooks/refs 警告）
  if (syncedInitial !== resolved.initialValue) {
    setSyncedInitial(resolved.initialValue)
    setValue(resolved.initialValue)
  }

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const newVal = e.target.value
      setValue(newVal)
      if (context.onAction) {
        context.onAction(context.surface.id, component.id, 'inputChanged', {
          value: newVal,
        })
      }
    },
    [context, component.id],
  )

  const className =
    'w-full px-3 py-1.5 rounded-md border border-mady-border bg-mady-bg-primary ' +
    'text-mady-text-primary text-mady-body placeholder:text-mady-text-tertiary ' +
    'focus:outline-none focus:ring-2 focus:ring-mady-accent-glow focus:border-mady-accent ' +
    'disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-80'

  if (resolved.multiline) {
    return (
      <div className="flex flex-col gap-1">
        {resolved.label && (
          <label className="text-mady-text-secondary text-mady-ui">{resolved.label}</label>
        )}
        <textarea
          className={`${className} resize-y min-h-[80px]`}
          placeholder={resolved.placeholder}
          value={value}
          disabled={resolved.disabled}
          onChange={handleChange}
          data-a2ui-id={component.id}
        />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      {resolved.label && (
        <label className="text-mady-text-secondary text-mady-ui">{resolved.label}</label>
      )}
      <input
        className={className}
        type="text"
        placeholder={resolved.placeholder}
        value={value}
        disabled={resolved.disabled}
        onChange={handleChange}
        data-a2ui-id={component.id}
      />
    </div>
  )
}
