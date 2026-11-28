/**
 * 设置面板 · 左侧导航。
 *
 * 从 SettingsPanel.tsx（M65）拆出：分区清单（图标 + 文案）与激活态样式。
 * 纯展示组件——激活分区与切换回调仍由 SettingsPanel 持有。
 */

import React from 'react'
import { Sun, Server, Brain, Info, Plug } from 'lucide-react'

/** 全屏设置页左侧导航分区 ID（规范 §3.3 SettingsNav）。 */
export type SettingsSectionId = 'appearance' | 'ai' | 'model' | 'mcp' | 'about'

interface SettingsNavProps {
  /** 当前激活分区。 */
  activeSection: SettingsSectionId
  /** 分区切换回调。 */
  onSelect: (id: SettingsSectionId) => void
}

/** 左侧导航分区（规范 §3.3：SettingsNav 200px、导航项 32px、图标 + 13px 文字）。 */
const SECTIONS: { id: SettingsSectionId; label: string; icon: React.ReactNode }[] = [
  { id: 'appearance', label: '外观', icon: <Sun size={14} /> },
  { id: 'ai', label: 'AI 服务', icon: <Server size={14} /> },
  { id: 'model', label: '模型与推理', icon: <Brain size={14} /> },
  { id: 'mcp', label: 'MCP 服务器', icon: <Plug size={14} /> },
  { id: 'about', label: '关于', icon: <Info size={14} /> },
]

/** 左侧导航：200px 固定宽度，分区按钮带激活态。 */
export const SettingsNav: React.FC<SettingsNavProps> = ({ activeSection, onSelect }) => {
  return (
    <nav className="w-[var(--mady-settings-nav-width)] shrink-0 border-r border-mady-separator p-2 space-y-0.5 overflow-y-auto">
      {SECTIONS.map((sec) => (
        <button
          key={sec.id}
          onClick={() => onSelect(sec.id)}
          aria-current={activeSection === sec.id ? 'page' : undefined}
          className={`w-full flex items-center gap-2 px-3 h-8 rounded-md text-mady-ui transition-colors duration-100 ${
            activeSection === sec.id
              ? 'bg-mady-accent-soft text-mady-accent font-medium'
              : 'text-mady-text-secondary hover:bg-mady-bg-hover hover:text-mady-text-primary'
          }`}
        >
          {sec.icon}
          {sec.label}
        </button>
      ))}
    </nav>
  )
}
