/**
 * StatusBar — 底部状态栏（对齐设计规范）。
 *
 * 规范定义：高度 40px，三区布局。
 *   左区: ConnectionChip（状态圆点 + 文字）
 *   中区: UsageMeter / ModelChip（模型徽章）
 *   右区: 知识库状态 + 版本
 */

import React, { useEffect, useState } from 'react'
import { useChatStore } from '@/stores/chat'
import { useHealth } from '@/stores/threads'
import { Circle, Brain, Server, Package, Loader2 } from 'lucide-react'
import type { HealthInfo } from '@/lib/backend'

export const StatusBar: React.FC = () => {
  const running = useChatStore((s) => s.running)
  const ready = useChatStore((s) => s.ready)
  const healthQuery = useHealth(ready)
  const [info, setInfo] = useState<HealthInfo | null>(null)
  const [loading, setLoading] = useState(false)

  // 健康信息由 TanStack Query 接管（useHealth，缓存 60s），
  // 此处仅同步到本地 state 供渲染（开发期/CI 失败静默忽略）
  useEffect(() => {
    if (!ready) return
    setLoading(healthQuery.isFetching)
    if (healthQuery.data) {
      setInfo(healthQuery.data)
    }
  }, [ready, healthQuery.data, healthQuery.isFetching])

  // 连接状态
  const connColor = running
    ? 'text-mady-connection-connected'
    : ready
      ? 'text-mady-text-tertiary'
      : 'text-mady-connection-connecting'
  const connLabel = running ? '运行中' : ready ? '就绪' : '初始化…'

  return (
    <footer className="h-10 flex items-center justify-between px-4 bg-mady-bg-secondary border-t border-mady-separator text-mady-caption text-mady-text-secondary select-none">
      {/* 左区：连接状态芯片（ConnectionChip） */}
      <div className="flex items-center gap-3">
        <span className="flex items-center gap-1.5">
          <Circle size={8} className={connColor} fill="currentColor" />
          <span className="text-mady-small font-medium">{connLabel}</span>
        </span>
      </div>

      {/* 中区：模型芯片（ModelChip） */}
      <div className="flex items-center gap-3">
        {loading && (
          <Loader2 size={12} className="animate-spin text-mady-text-tertiary" />
        )}
        {info && (
          <span className="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-mady-bg-hover text-mady-caption text-mady-text-secondary">
            <Server size={11} />
            <span className="font-medium">{info.provider ?? '—'}</span>
            {info.model && (
              <>
                <span className="text-mady-text-tertiary">·</span>
                <span>{info.model}</span>
              </>
            )}
          </span>
        )}
      </div>

      {/* 右区：知识库 + 版本 */}
      <div className="flex items-center gap-3">
        {info && (
          <span className="flex items-center gap-1.5">
            <Brain size={11} />
            知识库
          </span>
        )}
        <span className="flex items-center gap-1.5">
          <Package size={11} />
          v{info?.version ?? '0.1.0'}
        </span>
      </div>
    </footer>
  )
}
