/**
 * A2UIOverlay — A2UI surface 渲染容器。
 *
 * 遍历 useA2UIStore 中所有活跃 surface，用 A2UISurface 渲染组件树，
 * 并将组件交互（onAction）接线到后端 sendAction，形成
 * 按钮点击 → Wails binding → agent A2UI handler → surface 更新的完整闭环。
 *
 * 挂载于 App 根（与全局 store 生命周期一致）；模块级 handleAction 保持
 * 引用稳定，避免破坏 A2UISurface 内部 useMemo 导致每 token 全树重渲染。
 * deleteSurface 信封到达后 surface 从 store 快照移除，本组件自动卸载。
 */

import React from 'react'
import { useA2UIStore } from '@/a2ui-renderer/a2ui-store'
import { A2UISurface } from '@/a2ui-renderer/renderer'
import { sendAction } from '@/lib/backend'

/**
 * 模块级 onAction：不捕获任何渲染期状态，引用恒稳定。
 * surfaceId 由 A2UI 渲染器回传（agent 创建 surface 时的 id），
 * 与后端 surface_<threadID> 契约一致（server.SendAction 按前缀提取 threadID）。
 */
function handleAction(
  surfaceId: string,
  sourceId: string,
  eventName: string,
  context?: Record<string, unknown>,
): void {
  sendAction(surfaceId, {
    name: eventName,
    surfaceId,
    sourceComponentId: sourceId,
    timestamp: new Date().toISOString(),
    context: context ?? {},
  }).catch((err: Error) => {
    // 投递失败必须有反馈：静默丢弃会让用户以为审批已提交。
    // 生产环境（Wails binding）失败时打日志；后续可升级为 surface 内错误提示。
    console.error(`[a2ui] sendAction failed for surface ${surfaceId}:`, err)
  })
}

export const A2UIOverlay: React.FC = () => {
  const surfaces = useA2UIStore((s) => s.surfaces)
  const functions = useA2UIStore((s) => s.functions)
  const store = useA2UIStore((s) => s._store)

  const surfaceIds = Object.keys(surfaces)
  if (surfaceIds.length === 0) return null

  return (
    <div className="pointer-events-none fixed inset-0 z-50">
      {surfaceIds.map((id) => (
        // inline-block 收缩到内容宽度：避免静态块级 wrapper 占满
        // 全宽截获顶部点击（如标题栏按钮）。
        <div key={id} className="pointer-events-auto inline-block align-top">
          <A2UISurface store={store} surfaceId={id} functions={functions} onAction={handleAction} />
        </div>
      ))}
    </div>
  )
}
