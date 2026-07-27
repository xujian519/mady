/**
 * Wails Runtime — 由 `wails generate module` 自动生成。
 *
 * 当前文件为开发期占位桩，使 `pnpm build` / `pnpm typecheck` 通过。
 * 首次运行 `wails generate module` 后，本目录将被真实实现覆盖。
 *
 * 注意：Wails CLI 生成的文件为 JavaScript，此处提供 TypeScript 桩
 * 仅用于编译期通过。生成后以 JS 版本为准。
 */

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function EventsOn(_eventName: string, _callback: (...args: any[]) => void): () => void {
  return () => {}
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function EventsEmit(_eventName: string, _data?: any) {
  // noop
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function EventsOff(_eventName: string, ..._additionalEventNames: string[]) {
  // noop
}

export function LogInfo(message: string) {
  console.info('[wails]', message)
}

export function LogError(message: string) {
  console.error('[wails]', message)
}
