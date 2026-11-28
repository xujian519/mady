/**
 * 设置面板 · 外观分区。
 *
 * 从 SettingsPanel.tsx（M62）拆出：明暗模式三选一与主题画廊，
 * 纯展示组件——状态与持久化仍由 SettingsPanel 持有。
 */

import React from 'react'
import { Sun, Moon, Monitor } from 'lucide-react'
import { ThemeGallery } from '../ThemeGallery'
import type { ThemeMode } from '@/theme/tokens'

interface AppearanceSectionProps {
  /** 当前明暗模式。 */
  themeMode: ThemeMode
  /** 明暗模式切换回调。 */
  onThemeChange: (mode: ThemeMode) => void
}

/** 外观分区：明暗模式按钮组 + 主题画廊。 */
export const AppearanceSection: React.FC<AppearanceSectionProps> = ({ themeMode, onThemeChange }) => {
  const themeOptions: { value: ThemeMode; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: '亮色', icon: <Sun size={14} /> },
    { value: 'dark', label: '深色', icon: <Moon size={14} /> },
    { value: 'system', label: '跟随系统', icon: <Monitor size={14} /> },
  ]

  return (
    <section>
      <div className="flex items-center gap-2 mb-3">
        <Sun size={14} className="text-mady-accent" />
        <h3 className="text-mady-ui font-medium">外观</h3>
      </div>
      <div className="flex gap-2">
        {themeOptions.map((opt) => (
          <button
            key={opt.value}
            onClick={() => onThemeChange(opt.value)}
            className={`
              flex items-center gap-2 px-3 py-2 rounded-lg text-mady-ui transition-colors
              ${themeMode === opt.value
                ? 'bg-mady-accent text-white'
                : 'bg-mady-bg-secondary text-mady-text-secondary hover:bg-mady-border'
              }
            `}
          >
            {opt.icon}
            {opt.label}
          </button>
        ))}
      </div>

      {/* 阶段 3.2：主题画廊（主题包切换） */}
      <div className="mt-3">
        <ThemeGallery />
      </div>
    </section>
  )
}
