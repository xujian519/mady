/**
 * Wails Runtime 封装层。
 *
 * Wails 的 Go ↔ JS binding 通过 `wailsjs/runtime/runtime` 模块提供。
 * 该模块由 `wails generate module` 命令自动生成，随仓库提交（frontend/wailsjs/）。
 *
 * 本文件提供：
 * 1. 类型定义（与 wailsjs 返回类型一致的外部声明）
 * 2. 静态导入（F-I4）：生成物总是存在于仓库，静态导入避免「动态 import
 *    异步加载 vs 首个 useEffect 订阅」的竞态——早期订阅不再整体丢事件。
 *    纯浏览器环境（e2e）由 isWailsHost() 拦截，不触发真实绑定。
 */

// wailsjs 由 Wails CLI 生成，随仓库提交
import { EventsOn, EventsEmit } from '../../wailsjs/runtime/runtime'

// 重新导出类型供其他模块使用
export type { EventsOn, EventsEmit }

/**
 * 是否运行在 Wails 宿主中。
 * 生成代码内部直接访问 window.runtime / window.go，
 * 纯浏览器（pnpm dev / e2e）中该对象不存在，必须回避真实绑定。
 */
function isWailsHost(): boolean {
  return typeof window !== 'undefined' && !!window.runtime
}

/**
 * 订阅 Wails 事件。在 Wails 宿主中绑定 EventsOn，纯浏览器环境 noop。
 *
 * 载荷类型由调用方给定（`listenToWailsEvent<AguiEventPayload>(...)`），
 * 避免用 `any[]` 抹平各事件之间的载荷差异。
 */
export function listenToWailsEvent<T = unknown>(
  eventName: string,
  callback: (payload: T) => void,
): () => void {
  if (isWailsHost()) {
    return EventsOn(eventName, callback)
  }
  return () => {}
}
