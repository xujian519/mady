/**
 * ConfidenceBar — 置信度可视化组件。
 *
 * 展示结论的置信度水平：
 * - 高 (≥0.8): 绿色
 * - 中 (≥0.5): 橙色
 * - 低 (<0.5): 红色
 *
 * 用法：
 * ```tsx
 * <ConfidenceBar level={0.85} label="侵权可能性" />
 * ```
 */

import React from 'react'

// ── 阈值常量 ────────────────────────────────────

const HIGH_THRESHOLD = 0.8
const MEDIUM_THRESHOLD = 0.5

type ConfidenceLevel = 'high' | 'medium' | 'low'

const LEVEL_CONFIG: Record<ConfidenceLevel, { color: string; textColor: string; label: string }> = {
  high:   { color: 'bg-mady-success',  textColor: 'text-mady-success',  label: '高' },
  medium: { color: 'bg-mady-warning',  textColor: 'text-mady-warning',  label: '中' },
  low:    { color: 'bg-mady-danger',   textColor: 'text-mady-danger',   label: '低' },
}

function getConfidenceLevel(level: number): ConfidenceLevel {
  if (level >= HIGH_THRESHOLD) return 'high'
  if (level >= MEDIUM_THRESHOLD) return 'medium'
  return 'low'
}

// ── Props ─────────────────────────────────────────

interface ConfidenceBarProps {
  /** 置信度值 (0-1)。 */
  level: number
  /** 可选的标签文本。 */
  label?: string
  /** 可选的具体文字说明。 */
  description?: string
}

// ── Component ───────────────────────────────────

export const ConfidenceBar: React.FC<ConfidenceBarProps> = ({
  level,
  label,
  description,
}) => {
  const pct = Math.round(Math.max(0, Math.min(1, level)) * 100)
  const cfg = LEVEL_CONFIG[getConfidenceLevel(level)]

  return (
    <div className="space-y-1">
      {/* 标签行 */}
      <div className="flex items-center justify-between text-mady-caption">
        {label && <span className="text-mady-text-secondary">{label}</span>}
        <span className={`font-medium ${cfg.textColor}`}>
          {cfg.label} · {pct}%
        </span>
      </div>

      {/* 进度条 */}
      <div className="h-1.5 rounded-full bg-mady-bg-tertiary overflow-hidden">
        <div
          className={`h-full rounded-full transition-all duration-500 ease-out ${cfg.color}`}
          style={{ width: `${pct}%` }}
        />
      </div>

      {/* 描述 */}
      {description && (
        <p className="text-mady-caption text-mady-text-tertiary">{description}</p>
      )}
    </div>
  )
}
