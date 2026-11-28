/**
 * Wails 宿主与 e2e 测试钩子在 `window` 上的全局声明（P2-7 收口）。
 *
 * 此前 `window.runtime` / `window.go` / `window.__mady` 三处分散用 `any` 断言，
 * 这里集中声明：既去掉 any 扩散，也让「是否运行在 Wails 宿主」的判断有类型可依。
 */
import type { Envelope, SurfaceState } from '@/a2ui-renderer/store'

export {}

declare global {
  interface Window {
    /** Wails 运行时（EventsOn / EventsEmit 等），纯浏览器预览环境不存在。 */
    runtime?: Record<string, unknown>
    /** Wails Go binding 根对象：`window.go.<modulePath>.<method>`。 */
    go?: Record<string, unknown>
    /** e2e 测试钩子，仅在 ENABLE_TEST_API 时注入（见 app/App.tsx）。 */
    __mady?: MadyTestHooks
  }
}

/** e2e 测试钩子：绕过 Wails Events，直接驱动 reducer 或读取 store 状态。 */
export interface MadyTestHooks {
  a2ui: {
    applyEnvelope: (envelope: Envelope) => void
    getSurface: (id: string) => SurfaceState | undefined
  }
  agui: {
    dispatch: (name: string, payload: unknown) => void
    getChatState: () => Record<string, unknown>
  }
}
