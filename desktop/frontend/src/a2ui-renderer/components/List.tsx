/**
 * A2UI List 组件 — 渲染子组件列表。
 *
 * Props:
 *   children: ChildList — 静态 ID 列表或模板
 *
 * 注意：List 的 children 属性是 ChildList 类型，
 * renderer.tsx 的 tree walker 已处理静态 children 的迭代。
 * 模板模式需在此组件内处理（TODO: 阶段 3 实现 data-binding template）。
 */

import type { A2UIComponent } from '../registry'

export const ListComponent: A2UIComponent = ({ children }) => {
  return (
    <div className="flex flex-col gap-2">
      {children}
    </div>
  )
}
