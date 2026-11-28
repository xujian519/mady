/**
 * 设置面板 · 关于分区。
 *
 * 从 SettingsPanel.tsx（M62）拆出：版本三行信息 + 检查更新 + 新版本下载引导。
 * 纯展示组件——请求、Toast 与校验逻辑仍由 SettingsPanel 持有。
 */

import React from 'react'
import { RefreshCw, ExternalLink } from 'lucide-react'
import type { UpdateInfo } from '@/lib/backend'

interface AboutSectionProps {
  /** 后端返回的版本号，未取到时为 null（展示回退值）。 */
  version: string | null
  /** 检查到的新版本信息；无更新时为 null。 */
  updateInfo: UpdateInfo | null
  /** 是否正在检查更新。 */
  checking: boolean
  /** 「检查更新」回调。 */
  onCheckUpdate: () => void
  /** 「打开下载页」回调（URL 校验在调用方完成）。 */
  onOpenDownload: () => void
}

/** 关于分区：版本信息、检查更新与下载引导。 */
export const AboutSection: React.FC<AboutSectionProps> = ({
  version,
  updateInfo,
  checking,
  onCheckUpdate,
  onOpenDownload,
}) => {
  return (
    <section>
      <div className="flex items-center gap-2 mb-3">
        <span className="w-3.5 h-3.5 rounded-full bg-mady-accent flex items-center justify-center text-[8px] text-white font-bold">M</span>
        <h3 className="text-mady-ui font-medium">关于</h3>
      </div>
      <div className="bg-mady-bg-secondary rounded-lg p-3 space-y-1 text-mady-small text-mady-text-secondary">
        <div className="flex justify-between">
          <span>版本</span>
          <span className="text-mady-text-primary">{version ?? '0.1.0'}</span>
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
      <button
        onClick={onCheckUpdate}
        disabled={checking}
        className="mt-2 inline-flex items-center gap-1.5 text-mady-small text-mady-text-secondary hover:text-mady-accent disabled:opacity-60 transition-colors"
      >
        <RefreshCw size={12} className={checking ? 'animate-spin' : ''} />
        {checking ? '检查中…' : '检查更新'}
      </button>

      {/* 发现新版本（R-P1-6：手动下载引导，公证前不自替换） */}
      {updateInfo?.hasUpdate && updateInfo.downloadUrl && (
        <div className="mt-3 rounded-lg border border-mady-accent/30 bg-mady-bg-secondary px-3 py-2.5">
          <div className="flex items-center justify-between gap-3">
            <div className="text-mady-small text-mady-text-primary">
              发现新版本 v{updateInfo.latestVersion}（当前 v{updateInfo.currentVersion}）
            </div>
            <button
              onClick={onOpenDownload}
              className="inline-flex shrink-0 items-center gap-1 rounded-md bg-mady-accent px-2.5 py-1 text-mady-small font-medium text-white hover:opacity-90 transition-opacity"
            >
              <ExternalLink size={12} />
              打开下载页
            </button>
          </div>
        </div>
      )}
    </section>
  )
}
