/**
 * UsageStrip — 用量条（对齐设计规范第9.1.3章）。
 *
 * 规范定义：
 *   位置：AgentHeader 居中偏右
 *   高度：20px，胶囊形状
 *   数值格式：{已用}/{总额}
 *   颜色级别：<50% 灰色，50-80% 警告黄，80-95% 橙色，>95% 红色
 */

import React from 'react'
import { useChatStore } from '@/stores/chat'
import { DollarSign } from 'lucide-react'

export const UsageStrip: React.FC = () => {
  const contextUsagePercent = useChatStore((s) => s.contextUsagePercent)
  const contextTotalTokens = useChatStore((s) => s.contextTotalTokens)
  const contextWindow = useChatStore((s) => s.contextWindow)

  if (contextUsagePercent == null || contextWindow == null) return null

  // 用量级别颜色
  const levelColor =
    contextUsagePercent > 95
      ? 'text-mady-danger'
      : contextUsagePercent > 80
        ? 'text-mady-warning/80' // 橙色感
        : contextUsagePercent > 50
          ? 'text-mady-warning'
          : 'text-mady-text-secondary'

  return (
    <div className="flex items-center gap-1 h-5 px-2 rounded-full bg-mady-bg-hover text-mady-caption font-mono font-medium select-none">
      <DollarSign size={10} className={levelColor} />
      <span className={levelColor}>
        {contextTotalTokens != null
          ? `${(contextTotalTokens / 1000).toFixed(1)}k`
          : '—'}
        /{(contextWindow / 1000).toFixed(0)}k
      </span>
    </div>
  )
}
