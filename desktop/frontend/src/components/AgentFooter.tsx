/**
 * AgentFooter — Agent 面板底部栏（对齐设计规范第9.5章）。
 *
 * 规范定义：高度 32px，位于 Composer 与 StatusBar 之间。
 *   左侧: ConnectionStatus（状态圆点 6px + "就绪"/"已断开"）
 *   右侧: ModelBadge（胶囊徽章：provider · model · reasoning）
 *
 * 注意：connecting 状态暂不可达（chat store 无"连接中"态），
 * 待后端添加连接生命周期事件后恢复。
 */

import React from 'react'
import { useChatStore } from '@/stores/chat'
import { Circle } from 'lucide-react'

type ConnState = 'connected' | 'disconnected'

const CONN_CONFIG: Record<ConnState, { dotColor: string; label: string; labelColor: string }> = {
  connected: { dotColor: 'text-mady-connection-connected', label: '就绪', labelColor: 'text-mady-text-tertiary' },
  disconnected: { dotColor: 'text-mady-connection-disconnected', label: '已断开', labelColor: 'text-mady-danger' },
} as const

export const AgentFooter: React.FC = () => {
  const running = useChatStore((s) => s.running)
  const ready = useChatStore((s) => s.ready)

  const connState: ConnState = ready || running ? 'connected' : 'disconnected'
  const conn = CONN_CONFIG[connState]

  return (
    <footer className="h-8 flex items-center justify-between px-3 bg-mady-bg-secondary border-t border-mady-separator text-mady-caption select-none">
      {/* 左侧：连接状态 */}
      <div className="flex items-center gap-1.5">
        <Circle size={6} className={conn.dotColor} fill="currentColor" />
        <span className={conn.labelColor}>{conn.label}</span>
      </div>

      {/* 右侧：模型徽章 */}
      <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-mady-bg-hover text-mady-caption text-mady-text-secondary">
        <span className="font-medium">Mady</span>
        <span className="text-mady-text-tertiary">·</span>
        <span>{running ? '运行中' : '就绪'}</span>
      </div>
    </footer>
  )
}
