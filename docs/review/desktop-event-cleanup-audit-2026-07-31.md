# 桌面端 Wails 事件监听 Cleanup 契约审计（W4-T3）

- **日期**：2026-07-31
- **范围**：`desktop/frontend/src/` 下 Wails 事件订阅相关代码
- **背景**：Wails v2 事件监听未清理会导致组件重挂载后回调重复执行、内存累积（官方 issue #3796/#4683）；在事件 handler 内部调用取消函数会失效（issue #4393）。`desktop/frontend/src/lib/wails.ts` 的 `listenToWailsEvent(eventName, callback)` 已封装「返回取消函数」模式。

## 结论

**全部 4 个事件订阅调用点均已遵循「useEffect 内订阅 → 持有取消函数 → cleanup 中调用」标准契约，本次审计无需任何代码修复。**

- 未发现「在事件 handler 内部调用取消函数」的违规模式
- 未发现应用代码直接使用 `EventsOn` / `EventsEmit` / `EventsOff` 绕过 `listenToWailsEvent` 封装层（仅 `wailsjs/` 自动生成目录与 `src/lib/wails.ts` 封装层内部出现）
- 无闭包捕获过期 state 的问题：handler 依赖的 state 均已通过 ref / useCallback / 模块级纯函数处理

## 审计表

| 调用点 | 事件 | 在 useEffect 内 | 持有并返回取消函数 | cleanup 中调用 | handler 内调用取消 | 状态 | 修复动作 |
|---|---|---|---|---|---|---|---|
| `src/components/SplashScreen.tsx:34` | `mady:init-progress` | ✅（deps `[handleDone]`） | ✅ `unsubProgress` | ✅ `return () => { unsubProgress(); ... }` | 否 | 合规 | 无需修复 |
| `src/components/SplashScreen.tsx:39` | `mady:init-done` | ✅（同 useEffect） | ✅ `unsubDone` | ✅ 同上 | 否 | 合规 | 无需修复 |
| `src/agui-bridge/client.ts:49`（`subscribeAguiEvents()` 内部，17 个 `agui:*` 事件） | `agui:${name}` | 间接：经 `subscribeAguiEvents()` 聚合返回取消函数，由调用方在 useEffect 内持有 | ✅ `unsubscribers` 数组聚合 | ✅（`unsubscribers.forEach(fn => fn())`） | 否 | 合规 | 无需修复 |
| `src/app/App.tsx:66`（`subscribeAguiEvents()` 调用点） | —（聚合订阅） | ✅（空 deps useEffect） | ✅ `unsubscribe` | ✅ `return () => { unsubscribe() }` | 否 | 合规 | 无需修复 |

## 细节说明

- **SplashScreen**（`src/components/SplashScreen.tsx:30-53`）：两个 `listenToWailsEvent` 均在同一 useEffect 内订阅，cleanup 依次调用 `unsubProgress()` / `unsubDone()` 并 `clearTimeout`（15 秒兜底定时器）。`handleDone` 经 `useCallback` 稳定引用，内部用 `doneRef` / `fadingOutRef` 规避闭包过期，deps 为 `[handleDone]`。
- **agui-bridge**（`src/agui-bridge/client.ts:47-58`）：`subscribeAguiEvents()` 将 17 个事件订阅的取消函数聚合成一个返回函数，属于合理的「批量订阅」封装，无泄漏。
- **App.tsx**（`src/app/App.tsx:63-71`）：聚合订阅的 useEffect 空 deps，cleanup 正确调用 `unsubscribe()`。handler 链路只依赖模块级纯函数 `aguiReducer` 与 Zustand `getState`，无 state 捕获问题。
- 取消机制依赖 Wails `EventsOn` 返回的取消函数（内部调用 `EventsOff`），应用代码无需直接接触 `EventsOff`。

## 观察项（超出本次 cleanup 范围，未修改）

1. **`mady:init-error` 未订阅**：Go 侧 `desktop/app.go:117` 在 provider 构建失败时 emit `mady:init-error`，前端 SplashScreen 未订阅该事件，仅依赖先行的进度文案（"引擎初始化失败: ..."）与 15 秒兜底自动关闭。这属于「订阅缺失」而非「cleanup 缺失」，若需处理建议另立任务评估（前端可在 init-error 时展示失败态 UI）。
2. `emitWailsEvent`（`src/lib/wails.ts:62`）当前无任何调用点，属预留 API。
3. App.tsx 中的 `mady:set-theme-pack` 与 `beforeunload` 为 DOM 事件（非 Wails 事件），均已配对 `removeEventListener`，一并确认无泄漏。

## 验证结果

| 命令 | 结果 |
|---|---|
| `cd desktop/frontend && pnpm typecheck` | ✅ 通过（`tsc --noEmit`） |
| `cd desktop/frontend && pnpm test` | ✅ 通过（8 files / 100 tests） |
| `cd desktop && go build ./...` | ✅ 通过（未改动 Go 侧，作 sanity check 执行） |

## 改动文件清单

| 文件 | 改动 |
|---|---|
| `docs/review/desktop-event-cleanup-audit-2026-07-31.md` | 新增：本审计报告 |
| （其余） | 无——全部调用点已合规，零代码改动 |
