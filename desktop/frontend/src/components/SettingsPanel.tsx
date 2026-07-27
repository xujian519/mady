/**
 * SettingsPanel — 设置面板（覆盖层）。
 *
 * 分区：
 * - 外观：主题模式切换
 * - AI 服务：默认 Provider / 模型
 * - 关于：版本 / 许可
 */

import React from 'react'
import { useSettingsStore } from '@/stores/settings'
import { useTheme } from '@/theme/tokens'
import type { ThemeMode } from '@/theme/tokens'
import { X, Sun, Moon, Monitor, Server, Cpu } from 'lucide-react'

interface SettingsPanelProps {
  onClose: () => void
}

export const SettingsPanel: React.FC<SettingsPanelProps> = ({ onClose }) => {
  const settings = useSettingsStore()
  const { setMode } = useTheme()

  const themeOptions: { value: ThemeMode; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: '亮色', icon: <Sun size={14} /> },
    { value: 'dark', label: '深色', icon: <Moon size={14} /> },
    { value: 'system', label: '跟随系统', icon: <Monitor size={14} /> },
  ]

  const handleThemeChange = (mode: ThemeMode) => {
    setMode(mode)
    settings.update({ themeMode: mode })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm">
      <div className="w-[480px] max-h-[80vh] bg-mady-bg-primary rounded-2xl border border-mady-separator shadow-xl overflow-y-auto">
        {/* 头部 */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-mady-separator">
          <h2 className="text-mady-heading font-semibold">设置</h2>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-mady-bg-secondary text-mady-text-secondary"
          >
            <X size={16} />
          </button>
        </div>

        <div className="p-5 space-y-6">
          {/* 外观 */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <Sun size={14} className="text-mady-accent" />
              <h3 className="text-mady-ui font-medium">外观</h3>
            </div>
            <div className="flex gap-2">
              {themeOptions.map((opt) => (
                <button
                  key={opt.value}
                  onClick={() => handleThemeChange(opt.value)}
                  className={`
                    flex items-center gap-2 px-3 py-2 rounded-lg text-mady-ui transition-colors
                    ${settings.themeMode === opt.value
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
          </section>

          {/* AI 服务 */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <Server size={14} className="text-mady-accent" />
              <h3 className="text-mady-ui font-medium">AI 服务</h3>
            </div>

            <div className="space-y-3">
              <div>
                <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1">
                  <Cpu size={11} />
                  默认 Provider
                </label>
                <input
                  type="text"
                  value={settings.provider}
                  onChange={(e) => settings.update({ provider: e.target.value })}
                  placeholder="如 openai, anthropic"
                  className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent"
                />
              </div>

              <div>
                <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1">
                  <Cpu size={11} />
                  默认模型
                </label>
                <input
                  type="text"
                  value={settings.model}
                  onChange={(e) => settings.update({ model: e.target.value })}
                  placeholder="如 gpt-4o, claude-3-opus"
                  className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent"
                />
              </div>

              <p className="text-mady-caption text-mady-text-tertiary">
                Provider/Model 切换将在下一轮对话中生效
              </p>
            </div>
          </section>

          {/* 关于 */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <span className="w-3.5 h-3.5 rounded-full bg-mady-accent flex items-center justify-center text-[8px] text-white font-bold">M</span>
              <h3 className="text-mady-ui font-medium">关于</h3>
            </div>
            <div className="bg-mady-bg-secondary rounded-lg p-3 space-y-1 text-mady-small text-mady-text-secondary">
              <div className="flex justify-between">
                <span>版本</span>
                <span className="text-mady-text-primary">0.1.0</span>
              </div>
              <div className="flex justify-between">
                <span>构建</span>
                <span className="text-mady-text-primary font-mono">desktop</span>
              </div>
              <div className="flex justify-between">
                <span>A2UI 协议</span>
                <span className="text-mady-text-primary font-mono">v0.9.1</span>
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
