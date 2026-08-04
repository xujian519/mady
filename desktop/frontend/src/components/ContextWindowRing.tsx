/**
 * ContextWindowRing — 上下文窗口用量环形指示器（阶段 2.3）。
 *
 * 仿照 Reasonix 桌面端 ContextWindowRing：SVG 圆环展示上下文窗口占用比例，
 * 颜色分级与 UsageStrip 一致（<50% 灰 / 50-80% 黄 / 80-95% 橙 / >95% 红），
 * 悬停 tooltip 展示精确用量（{已用}/{窗口}）。
 */

import React from 'react'
import { useChatStore } from '@/stores/chat'

interface Props {
  /** 环形外径（px），默认 14（总尺寸约 32px 含描边）。 */
  radius?: number
}

/** 用量级别 → CSS 变量颜色（与 UsageStrip 分级一致）。 */
function levelColor(percent: number): string {
  if (percent > 95) return 'var(--color-mady-danger)'
  if (percent > 50) return 'var(--color-mady-warning)'
  return 'var(--color-mady-text-tertiary)'
}

export const ContextWindowRing: React.FC<Props> = ({ radius = 14 }) => {
  const percent = useChatStore((s) => s.contextUsagePercent)
  const totalTokens = useChatStore((s) => s.contextTotalTokens)
  const window = useChatStore((s) => s.contextWindow)

  if (percent == null || window == null) return null

  const stroke = 3
  const size = (radius + stroke) * 2
  const c = 2 * Math.PI * radius
  const clamped = Math.min(100, Math.max(0, percent))
  const offset = c * (1 - clamped / 100)
  const color = levelColor(percent)

  const usedK = totalTokens != null ? (totalTokens / 1000).toFixed(1) : '—'
  const windowK = (window / 1000).toFixed(0)

  return (
    <div
      className="flex items-center select-none"
      title={`上下文用量 ${usedK}k / ${windowK}k（${percent.toFixed(0)}%）`}
      aria-label={`上下文窗口已用 ${percent.toFixed(0)}%`}
    >
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} className="-rotate-90">
        {/* 底环 */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="var(--color-mady-bg-hover)"
          strokeWidth={stroke}
        />
        {/* 进度环 */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={offset}
          style={{ transition: 'stroke-dashoffset 0.3s ease, stroke 0.3s ease' }}
        />
      </svg>
      <span
        className="ml-1 font-mono text-mady-caption font-medium"
        style={{ color }}
      >
        {percent.toFixed(0)}%
      </span>
    </div>
  )
}
