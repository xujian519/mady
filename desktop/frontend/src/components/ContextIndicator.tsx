/**
 * ContextIndicator — 上下文窗口使用率指示器。
 *
 * 在 Composer 上方以进度条和百分比数字展示当前上下文使用情况。
 * 颜色阈值：
 *   < 50%  绿色
 *   50-80% 黄色
 *   > 80%  红色（接近溢出）
 */

import React from 'react'
import { useChatStore } from '@/stores/chat'
import { Brain } from 'lucide-react'

/** 获取进度条颜色 CSS 类。 */
function usageColor(percent: number): string {
  if (percent >= 80) return 'bg-red-500'
  if (percent >= 50) return 'bg-amber-500'
  return 'bg-emerald-500'
}

/** 获取文本颜色 CSS 类。 */
function usageTextColor(percent: number): string {
  if (percent >= 80) return 'text-red-500'
  if (percent >= 50) return 'text-amber-500'
  return 'text-emerald-500'
}

export const ContextIndicator: React.FC = () => {
  const usagePercent = useChatStore((s) => s.contextUsagePercent)
  const totalTokens = useChatStore((s) => s.contextTotalTokens)
  const contextWindow = useChatStore((s) => s.contextWindow)

  // 没有数据时不渲染
  if (usagePercent == null || totalTokens == null || contextWindow == null) {
    return null
  }

  const pct = Math.min(usagePercent, 100)
  const barColor = usageColor(pct)
  const textColor = usageTextColor(pct)
  const totalLabel = formatToken(totalTokens)
  const limitLabel = formatToken(contextWindow)

  return (
    <div className="px-4 pt-2 pb-1 border-t border-mady-separator/50">
      <div className="max-w-3xl mx-auto flex items-center gap-2">
        {/* 图标 */}
        <Brain size={12} className="text-mady-text-tertiary shrink-0" />

        {/* 进度条 */}
        <div className="flex-1 h-1.5 rounded-full bg-mady-bg-secondary overflow-hidden">
          <div
            className={`h-full rounded-full transition-all duration-300 ${barColor}`}
            style={{ width: `${pct}%` }}
          />
        </div>

        {/* 百分比和数值 */}
        <span className={`text-mady-caption font-mono shrink-0 ${textColor}`}>
          {pct.toFixed(0)}%
        </span>
        <span className="text-mady-caption text-mady-text-tertiary font-mono shrink-0">
          {totalLabel} / {limitLabel}
        </span>
      </div>
    </div>
  )
}

/** 格式化 token 数为人类可读格式。 */
function formatToken(n: number): string {
  if (n >= 1_000_000) {
    return `${(n / 1_000_000).toFixed(1)}M`
  }
  if (n >= 1_000) {
    return `${(n / 1_000).toFixed(0)}K`
  }
  return String(n)
}
