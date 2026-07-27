/**
 * StatusBar — 底部状态栏。
 *
 * 显示：
 * - Provider / 模型名称
 * - 知识库状态
 * - 应用版本
 * - Agent 运行状态
 */

import React, { useEffect, useState } from 'react'
import { useChatStore } from '@/stores/chat'
import { health } from '@/lib/backend'
import { Circle, Brain, Server, Package, Loader2 } from 'lucide-react'
import type { HealthInfo } from '@/lib/backend'

export const StatusBar: React.FC = () => {
  const running = useChatStore((s) => s.running)
  const ready = useChatStore((s) => s.ready)
  const [info, setInfo] = useState<HealthInfo | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!ready) return
    setLoading(true)
    health()
      .then(setInfo)
      .catch(() => {}) // 开发期/CI 静默忽略
      .finally(() => setLoading(false))
  }, [ready])

  return (
    <footer className="h-7 flex items-center justify-between px-3 bg-mady-bg-secondary border-t border-mady-separator text-mady-caption text-mady-text-secondary select-none">
      {/* 左侧：运行状态 + Provider */}
      <div className="flex items-center gap-3">
        <span className="flex items-center gap-1.5">
          <Circle
            size={7}
            className={running ? 'text-mady-success' : ready ? 'text-mady-text-tertiary' : 'text-mady-warning'}
            fill="currentColor"
          />
          <span>{running ? '运行中' : ready ? '就绪' : '初始化…'}</span>
        </span>

        {loading && (
          <Loader2 size={10} className="animate-spin text-mady-text-tertiary" />
        )}

        {info && (
          <>
            <span className="text-mady-separator">|</span>
            <span className="flex items-center gap-1">
              <Server size={10} />
              {info.provider ?? '—'}
              {info.model && <span className="text-mady-text-tertiary">/ {info.model}</span>}
            </span>
          </>
        )}
      </div>

      {/* 右侧：知识库 + 版本 */}
      <div className="flex items-center gap-3">
        {info && (
          <>
            <span className="flex items-center gap-1">
              <Brain size={10} />
              知识库
            </span>
            <span className="text-mady-separator">|</span>
          </>
        )}
        <span className="flex items-center gap-1">
          <Package size={10} />
          v{info?.version ?? '0.1.0'}
        </span>
      </div>
    </footer>
  )
}
