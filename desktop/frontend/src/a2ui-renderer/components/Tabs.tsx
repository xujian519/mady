/**
 * A2UI Tabs 组件 — 标签页。
 *
 * Props:
 *   tabs: Array<{ child: string; label: string }> — 标签定义
 *
 * childRefsOf 从 NestedChildFields 解析 child 引用。
 * 此处只渲染标签头；子组件由 renderer 的 tree walker 递归渲染。
 */

import { useState } from 'react'
import type { A2UIComponent } from '../registry'

interface TabDef {
  child?: string
  label?: string
}

export const TabsComponent: A2UIComponent = ({ component, children }) => {
  const tabs = (component.props.tabs as TabDef[] | undefined) ?? []
  const [activeIdx, setActiveIdx] = useState(0)

  // children 是平行排列的，只渲染活跃标签的 children
  const activeChild = children
    ? Array.isArray(children)
      ? children[activeIdx]
      : children
    : null

  return (
    <div className="flex flex-col">
      <div className="flex gap-0 border-b border-mady-separator">
        {tabs.map((tab, idx) => (
          <button
            key={idx}
            className={`px-3 py-1.5 text-mady-ui font-medium transition-colors duration-80
              ${idx === activeIdx
                ? 'text-mady-accent border-b-2 border-mady-accent -mb-px'
                : 'text-mady-text-secondary hover:text-mady-text-primary'
              }`}
            onClick={() => setActiveIdx(idx)}
          >
            {tab.label ?? `Tab ${idx + 1}`}
          </button>
        ))}
      </div>
      <div className="pt-3">
        {activeChild}
      </div>
    </div>
  )
}
