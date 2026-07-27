/**
 * A2UI Row 组件 — 水平排列子组件。
 *
 * Props:
 *   gap: number — 子组件间距（默认 8）
 *   alignment: "start" | "center" | "end" | "stretch"
 */

import type { A2UIComponent } from '../registry'

export const RowComponent: A2UIComponent = ({ component, children }) => {
  const gap = (component.props.gap as number) ?? 8
  const alignment = (component.props.alignment as string) ?? 'start'

  const alignClass = {
    start: 'items-start',
    center: 'items-center',
    end: 'items-end',
    stretch: 'items-stretch',
  }[alignment] ?? 'items-start'

  return (
    <div
      className={`flex flex-row ${alignClass}`}
      style={{ gap: `${gap}px` }}
    >
      {children}
    </div>
  )
}
