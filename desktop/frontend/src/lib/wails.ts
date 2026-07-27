/**
 * Wails Runtime 封装层。
 *
 * Wails 的 Go ↔ JS binding 通过 `wailsjs/runtime/runtime` 模块提供。
 * 该模块由 `wails generate module` 命令自动生成，开发环境中可能不存在。
 *
 * 本文件提供：
 * 1. 类型定义（与 wailsjs 返回类型一致的外部声明）
 * 2. 条件导入：生产环境加载真实的 wailsjs 模块，不存在时使用空 shell
 *    保证 `pnpm build` / `pnpm typecheck` 不受 wailsjs 缺失影响。
 */

// wailsjs 由 Wails CLI 生成，开发期使用占位桩
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import type { EventsOn, EventsEmit } from '../../wailsjs/runtime/runtime'

// 重新导出类型供其他模块使用
export type { EventsOn, EventsEmit }

let eventsOn: typeof EventsOn | null = null
let eventsEmit: typeof EventsEmit | null = null

// 动态加载 wailsjs 模块，Web 浏览器回退为空函数
async function loadWailsRuntime() {
  try {
    const mod = await import('../../wailsjs/runtime/runtime')
    eventsOn = mod.EventsOn
    eventsEmit = mod.EventsEmit
  } catch {
    // wailsjs 未生成（开发期 / pnpm build），使用 noop 占位
    console.warn('[mady] wailsjs runtime not found — Wails binding unavailable')
    eventsOn = null as any
    eventsEmit = null as any
  }
}

// 启动时尝试加载
loadWailsRuntime()

/**
 * 订阅 Wails 事件。在生产环境绑定 EventsOn，开发期 noop。
 */
export function listenToWailsEvent(eventName: string, callback: (...args: any[]) => void): () => void {
  if (eventsOn) {
    eventsOn(eventName, callback)
    return () => {
      // Wails 的 EventsOn 返回 unregister 函数
      // 此处保留接口；实际清除依赖 Wails 的 EventsOff
    }
  }
  return () => {}
}

/**
 * 通过 Wails Events 向后端发送事件。在生产环境绑定 EventsEmit，开发期 noop。
 */
export function emitWailsEvent(eventName: string, data?: any) {
  if (eventsEmit) {
    eventsEmit(eventName, data)
  }
}
