/**
 * ThemeGallery — 主题画廊（阶段 3.2）。
 *
 * 卡片网格展示全部主题包：预览背景渐变 + 主色样本 + 名称/描述，
 * 点击切换主题包（写入 localStorage，经 ThemeProvider 注入 CSS 变量）。
 */

import React from 'react'
import { THEME_PACKS } from '@/theme/packs'
import { useTheme, type ThemeMode, type ThemePackId } from '@/theme/tokens'
import { Check, Download, Upload } from 'lucide-react'

/** 主题导出文件格式（3.3 简化版：JSON 配置，不打包背景图资产）。 */
interface ThemeExportFile {
  app: 'mady'
  kind: 'theme'
  version: 1
  mode: 'light' | 'dark' | 'system'
  pack: string
}

export const ThemeGallery: React.FC = () => {
  const { themePack, setThemePack, isDark, mode, setMode } = useTheme()
  const fileInputRef = React.useRef<HTMLInputElement>(null)

  // ── 导出当前主题配置（JSON 下载） ────────────────

  const handleExport = () => {
    const payload: ThemeExportFile = { app: 'mady', kind: 'theme', version: 1, mode, pack: themePack }
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'mady-theme.json'
    a.click()
    URL.revokeObjectURL(url)
  }

  // ── 导入主题配置（校验后应用） ────────────────────

  const handleImportFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // 允许重复选择同一文件
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      try {
        const parsed = JSON.parse(String(reader.result)) as Partial<ThemeExportFile>
        if (parsed.app !== 'mady' || parsed.kind !== 'theme' || !parsed.pack || !parsed.mode) {
          throw new Error('invalid theme file')
        }
        const known = THEME_PACKS.some((p) => p.id === parsed.pack)
        if (!known || !['light', 'dark', 'system'].includes(parsed.mode)) {
          throw new Error('unknown theme pack or mode')
        }
        setThemePack(parsed.pack as ThemePackId)
        setMode(parsed.mode as ThemeMode)
      } catch {
        // 非法文件：静默忽略（保持当前主题）
      }
    }
    reader.readAsText(file)
  }

  return (
    <div>
      <div className="grid grid-cols-2 gap-2">
        {THEME_PACKS.map((pack) => {
          const vars = isDark ? pack.dark : pack.light
          const active = themePack === pack.id
          return (
            <button
              key={pack.id}
              onClick={() => setThemePack(pack.id as ThemePackId)}
              aria-pressed={active}
              className={`
                text-left rounded-xl border p-2 transition-all duration-150
                ${active
                  ? 'border-mady-accent ring-2 ring-mady-accent/20'
                  : 'border-mady-border hover:border-mady-text-quaternary'}
              `}
            >
              {/* 预览表面：背景渐变 + 主色样本 */}
              <div
                className="relative h-14 rounded-lg mb-2 overflow-hidden"
                style={{ background: vars.background }}
              >
                {/* 模拟窗口内容：文本行 + 强调色块 */}
                <div className="absolute inset-0 p-2 space-y-1 opacity-90">
                  <div className="h-1.5 w-3/5 rounded-full" style={{ background: vars.textPrimary, opacity: 0.35 }} />
                  <div className="h-1.5 w-4/5 rounded-full" style={{ background: vars.textPrimary, opacity: 0.15 }} />
                  <div className="h-1.5 w-2/5 rounded-full" style={{ background: vars.accent }} />
                </div>
                {active && (
                  <span className="absolute top-1 right-1 p-0.5 rounded-full bg-mady-accent text-white">
                    <Check size={10} />
                  </span>
                )}
              </div>
              <p className="text-mady-ui font-medium text-mady-text-primary">{pack.label}</p>
              <p className="text-mady-caption text-mady-text-tertiary truncate">{pack.desc}</p>
            </button>
          )
        })}
      </div>

      {/* 3.3：导入 / 导出 */}
      <div className="flex items-center gap-2 mt-2">
        <button
          onClick={handleExport}
          title="导出当前主题配置"
          className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-mady-caption text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors"
        >
          <Download size={12} />
          导出
        </button>
        <button
          onClick={() => fileInputRef.current?.click()}
          title="导入主题配置（JSON）"
          className="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-mady-caption text-mady-text-secondary hover:text-mady-text-primary hover:bg-mady-bg-hover transition-colors"
        >
          <Upload size={12} />
          导入
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          onChange={handleImportFile}
        />
      </div>
    </div>
  )
}
