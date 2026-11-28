/**
 * 设置面板 · AI 服务分区。
 *
 * 从 SettingsPanel.tsx（M64）拆出：默认 Provider / 默认模型的本地编辑态展示与保存按钮。
 * 纯展示组件——编辑态、脏判定、保存请求与 Toast 仍由 SettingsPanel 持有。
 */

import React from 'react'
import { Server, Cpu } from 'lucide-react'

interface AIServiceSectionProps {
  /** 本地编辑的 Provider（保存前不影响后端）。 */
  provider: string
  /** 本地编辑的默认模型。 */
  model: string
  /** 是否正在保存。 */
  saving: boolean
  /** 编辑态与已生效配置是否存在差异。 */
  dirty: boolean
  /** Provider 输入回调。 */
  onProviderChange: (value: string) => void
  /** 模型输入回调。 */
  onModelChange: (value: string) => void
  /** 保存回调。 */
  onSave: () => void
}

/** AI 服务分区：Provider / 模型输入与保存按钮。 */
export const AIServiceSection: React.FC<AIServiceSectionProps> = ({
  provider,
  model,
  saving,
  dirty,
  onProviderChange,
  onModelChange,
  onSave,
}) => {
  return (
    <section>
      <div className="flex items-center gap-2 mb-3">
        <Server size={14} className="text-mady-accent" />
        <h3 className="text-mady-ui font-medium">AI 服务</h3>
      </div>

      <div className="space-y-3">
        <div>
          <label htmlFor="settings-provider" className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1">
            <Cpu size={11} />
            默认 Provider
          </label>
          <input
            id="settings-provider"
            type="text"
            value={provider}
            onChange={(e) => onProviderChange(e.target.value)}
            placeholder="如 deepseek, kimi, zhipu"
            className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20 transition-all duration-150"
          />
        </div>

        <div>
          <label htmlFor="settings-model" className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1">
            <Cpu size={11} />
            默认模型
          </label>
          <input
            id="settings-model"
            type="text"
            value={model}
            onChange={(e) => onModelChange(e.target.value)}
            placeholder="如 deepseek-v4-flash, kimi-k2.6"
            className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20 transition-all duration-150"
          />
        </div>

        <div className="flex items-center justify-between">
          <p className="text-mady-caption text-mady-text-tertiary">
            切换仅对新建会话生效，已有会话保持原有模型
          </p>
          <button
            onClick={onSave}
            disabled={!dirty || saving}
            className={`
              px-3 py-1.5 rounded-lg text-mady-small transition-colors
              ${dirty && !saving
                ? 'bg-mady-accent text-white hover:bg-mady-accent-hover'
                : 'bg-mady-bg-secondary text-mady-text-tertiary cursor-not-allowed'
              }
            `}
          >
            {saving ? '保存中…' : '保存'}
          </button>
        </div>
      </div>
    </section>
  )
}
