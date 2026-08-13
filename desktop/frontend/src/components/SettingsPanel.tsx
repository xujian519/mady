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
import { getAISettings, setAISettings, checkUpdate, health } from '@/lib/backend'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import type { UpdateInfo } from '@/lib/backend'
import { AnimatePresence, motion } from 'framer-motion'
import { X, Sun, Moon, Monitor, Server, Cpu, Check, AlertCircle, RefreshCw, ExternalLink } from 'lucide-react'
import { McpServersSettings } from './McpServersSettings'
import { ModelSettings } from './ModelSettings'
import { ThemeGallery } from './ThemeGallery'
import { ModalShell } from './ModalShell'

interface SettingsPanelProps {
  onClose: () => void
}

/** Toast 通知。 */
interface Toast {
  kind: 'success' | 'error'
  message: string
}

export const SettingsPanel: React.FC<SettingsPanelProps> = ({ onClose }) => {
  // 关于区版本号：优先展示 health().version（后端 ldflags 注入），未取到则回退占位。
  const [appVersion, setAppVersion] = React.useState<string | null>(null)
  React.useEffect(() => {
    let cancelled = false
    health()
      .then((h) => {
        if (!cancelled && h.version) setAppVersion(h.version)
      })
      .catch(() => {
        /* 后端不可用时保持回退值 */
      })
    return () => {
      cancelled = true
    }
  }, [])

  // F-I16：字段级 selector，避免温度滑条/面板刷新时整树重订阅
  const settingsProvider = useSettingsStore((s) => s.provider)
  const settingsModel = useSettingsStore((s) => s.model)
  const themeMode = useSettingsStore((s) => s.themeMode)
  const updateSettings = useSettingsStore((s) => s.update)
  const { setMode } = useTheme()

  // AI 服务：本地编辑态（保存前不影响后端）
  const [provider, setProvider] = React.useState(settingsProvider)
  const [model, setModel] = React.useState(settingsModel)
  const [saving, setSaving] = React.useState(false)
  const [toast, setToast] = React.useState<Toast | null>(null)
  const [checking, setChecking] = React.useState(false)
  const [updateInfo, setUpdateInfo] = React.useState<UpdateInfo | null>(null)

  // 挂载时从后端读取当前生效的 Provider/Model（真相源在后端）
  React.useEffect(() => {
    getAISettings()
      .then((s) => {
        setProvider(s.provider)
        setModel(s.model)
        updateSettings({ provider: s.provider, model: s.model })
      })
      .catch(() => {
        // 后端未就绪（初始化中）：保留本地编辑态，用户仍可保存触发重试
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Toast 自动消失
  React.useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(null), 3000)
    return () => clearTimeout(timer)
  }, [toast])

  const aiDirty =
    (provider !== '' && provider !== settingsProvider) ||
    (model !== '' && model !== settingsModel)

  const handleSaveAI = async () => {
    if (saving || !aiDirty) return
    setSaving(true)
    const effective = {
      provider: provider || settingsProvider,
      model: model || settingsModel,
    }
    try {
      await setAISettings(effective)
      updateSettings(effective)
      setProvider(effective.provider)
      setModel(effective.model)
      setToast({ kind: 'success', message: '已保存，切换将在下一轮新会话中生效' })
    } catch (err) {
      setToast({
        kind: 'error',
        message: err instanceof Error ? err.message : '保存失败，请稍后重试',
      })
    } finally {
      setSaving(false)
    }
  }

  const themeOptions: { value: ThemeMode; label: string; icon: React.ReactNode }[] = [
    { value: 'light', label: '亮色', icon: <Sun size={14} /> },
    { value: 'dark', label: '深色', icon: <Moon size={14} /> },
    { value: 'system', label: '跟随系统', icon: <Monitor size={14} /> },
  ]

  const handleThemeChange = (mode: ThemeMode) => {
    setMode(mode)
    updateSettings({ themeMode: mode })
  }

  // 检查更新（R-P1-6：真实检测 GitHub Releases desktop-v*；发现新版本时展示下载入口）
  const handleCheckUpdate = async () => {
    if (checking) return
    setChecking(true)
    setUpdateInfo(null)
    try {
      const info = await checkUpdate()
      if (info.hasUpdate && info.downloadUrl) {
        setUpdateInfo(info)
      } else {
        setToast({ kind: 'success', message: info.message })
      }
    } catch (err) {
      setToast({
        kind: 'error',
        message: err instanceof Error ? err.message : '检查更新失败，请稍后重试',
      })
    } finally {
      setChecking(false)
    }
  }

  // 打开下载页（M-DSK-SEC-003：仅放行 http/https，交给系统浏览器）
  const handleOpenDownload = () => {
    if (!updateInfo?.downloadUrl) return
    let url: URL
    try {
      url = new URL(updateInfo.downloadUrl)
    } catch {
      setToast({ kind: 'error', message: '下载地址无效' })
      return
    }
    if (url.protocol !== 'http:' && url.protocol !== 'https:') {
      setToast({ kind: 'error', message: '下载地址无效' })
      return
    }
    BrowserOpenURL(url.toString())
  }

  return (
    <ModalShell onClose={onClose} ariaLabel="设置">
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
                  value={provider}
                  onChange={(e) => setProvider(e.target.value)}
                  placeholder="如 deepseek, kimi, zhipu"
                  className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20 transition-all duration-150"
                />
              </div>

              <div>
                <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1">
                  <Cpu size={11} />
                  默认模型
                </label>
                <input
                  type="text"
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="如 deepseek-v4-flash, kimi-k2.6"
                  className="w-full rounded-lg px-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20 transition-all duration-150"
                />
              </div>

              <div className="flex items-center justify-between">
                <p className="text-mady-caption text-mady-text-tertiary">
                  切换仅对新建会话生效，已有会话保持原有模型
                </p>
                <button
                  onClick={handleSaveAI}
                  disabled={!aiDirty || saving}
                  className={`
                    px-3 py-1.5 rounded-lg text-mady-small transition-colors
                    ${aiDirty && !saving
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

          {/* 模型与推理（C08） */}
          <div className="border-t border-mady-separator pt-4">
            <ModelSettings />
          </div>

          {/* MCP 服务器（C06） */}
          <div className="border-t border-mady-separator pt-4">
            <McpServersSettings />
          </div>

          {/* 关于 */}
          <section>
            <div className="flex items-center gap-2 mb-3">
              <span className="w-3.5 h-3.5 rounded-full bg-mady-accent flex items-center justify-center text-[8px] text-white font-bold">M</span>
              <h3 className="text-mady-ui font-medium">关于</h3>
            </div>
            <div className="bg-mady-bg-secondary rounded-lg p-3 space-y-1 text-mady-small text-mady-text-secondary">
              <div className="flex justify-between">
                <span>版本</span>
                <span className="text-mady-text-primary">{appVersion ?? '0.1.0'}</span>
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
              onClick={handleCheckUpdate}
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
                    onClick={handleOpenDownload}
                    className="inline-flex shrink-0 items-center gap-1 rounded-md bg-mady-accent px-2.5 py-1 text-mady-small font-medium text-mady-accent-foreground hover:opacity-90 transition-opacity"
                  >
                    <ExternalLink size={12} />
                    打开下载页
                  </button>
                </div>
              </div>
            )}
          </section>
        </div>
      </div>

      {/* Toast 通知（T3.6：切换结果反馈） */}
      <AnimatePresence>
        {toast && (
          <motion.div
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 8 }}
            transition={{ duration: 0.16, ease: 'easeOut' }}
            className={`
              fixed bottom-6 left-1/2 -translate-x-1/2 z-[60]
              flex items-center gap-2 px-4 py-2.5 rounded-lg shadow-lg backdrop-blur-xl
              text-mady-small border
              ${toast.kind === 'success'
                ? 'bg-mady-bg-material text-mady-text-primary border-mady-separator'
                : 'bg-mady-bg-material text-mady-danger border-mady-danger/30'
              }
            `}
          >
            {toast.kind === 'success' ? (
              <Check size={14} className="text-mady-success" />
            ) : (
              <AlertCircle size={14} className="text-mady-danger" />
            )}
            {toast.message}
          </motion.div>
        )}
      </AnimatePresence>
    </ModalShell>
  )
}
