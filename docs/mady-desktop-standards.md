# Mady 桌面端开发规范 v1.0（社区对齐版）

> 综合 **Apple HIG（Human Interface Guidelines）**、**Wails v2 官方文档与源码**、**React / TypeScript / Tailwind CSS v4 / Zustand / TanStack Query 官方规范**、**shadcn/ui 设计原则**，以及 **Wails 生态成功开源案例**（jcp / tiny-rdm / RWKV-Runner / GoNavi 等）调研后的桌面端开发规范。
>
> 本文档是 `docs/specs/desktop/`（01-proposal / 02-spec / 03-design / 04-tasks）、`desktop-design-development-basis.md`（视觉 Token 适配）、`desktop-next-development-plan.md`（缺口计划）的**补充与对齐规范**——把官方社区标准映射到 Mady 桌面端的具体实现，识别差距并给出可操作的开发指引。规则编号 `M-DSK-*`，与 TUI 规范 `M-TUI-*` 并列。
>
> 版本：v1.0 | 日期：2026-07-31 | 调研基准：2026-07-31（GitHub API / Wails v2.13.0 源码 / Apple HIG 官方数据端点）

---

## 目录

1. [设计哲学对照](#1-设计哲学对照)
2. [技术栈与官方规范基线](#2-技术栈与官方规范基线)
3. [Wails 工程规范](#3-wails-工程规范)
4. [前端工程规范（React / TS / Tailwind）](#4-前端工程规范react--ts--tailwind)
5. [状态管理分工（Zustand vs TanStack Query）](#5-状态管理分工zustand-vs-tanstack-query)
6. [视觉设计规范（Apple HIG 映射）](#6-视觉设计规范apple-hig-映射)
7. [交互设计规范](#7-交互设计规范)
8. [无障碍映射](#8-无障碍映射)
9. [性能规范](#9-性能规范)
10. [安全规范](#10-安全规范)
11. [测试与质量门禁](#11-测试与质量门禁)
12. [打包与发布](#12-打包与发布)
13. [成功案例参考](#13-成功案例参考)
14. [差距分析与改进路线图](#14-差距分析与改进路线图)
15. [附录：Sources](#15-附录sources)

---

## 1. 设计哲学对照

### 1.1 三条核心信条 vs Mady 实现

| 社区信条 | 出处 | Mady 对应实现 | 状态 |
|---------|------|--------------|------|
| **原生感优先**：窗口外观与行为以系统为认知基线，不复刻系统窗口控件 | Apple HIG「Windows」 | Wails `mac.TitleBarHiddenInset()` 保留系统交通灯，业务 chrome（侧栏/工具栏）画在 Web 内容层；`main.go` 只做装配 | ✅ 已实现 |
| **语义化优先**：颜色全部走语义令牌，自动适配深浅色/高对比，禁止硬编码色值 | Apple HIG「Color」+ shadcn Theming | `globals.css` 的 `@theme` 语义令牌体系（41 个 `--color-mady-*` 色令牌 + 圆角/阴影/字号/时长/缓动/布局令牌，共 77 个基础令牌、含深浅色双份共 126 处声明）+ `theme/tokens.ts` 的 `CSS_VARS` 集中表 | ✅ 已实现 |
| **克制与渐进披露**：对话框仅用于「少见+不可撤销+破坏性」操作；组件信息按需展开 | Apple HIG「Alerts」+ shadcn Open Code | ApprovalCard（审批）、ToolCard 渐进展开、错误 toast 非模态 | ✅ 已实现 |

### 1.2 渲染模式选择

社区推荐 **保留模式（组件树 + 状态驱动）**，Mady 桌面端为 React 声明式渲染，天然保留模式。关键纪律（详见 §4）：

- 数据单向流动（props 向下、回调向上）
- 流式内容通过 **事件驱动增量更新**（`agui-bridge/reducer.ts`），而非整块重渲染
- 长列表虚拟化（`@tanstack/react-virtual`）

### 1.3 三层质量基线

| 层级 | 社区参考 | Mady 基线 |
|------|---------|-----------|
| 正确性 | Wails Binding 契约 + React 官方状态纪律 | `wailsjs` 生成类型参与编译 + `tsc --noEmit` 门禁 |
| 体验 | Apple HIG（视觉/交互/无障碍） | Token 走查表 + `prefers-*` 媒体查询（reduced-motion / contrast 已实现） |
| 工程 | shadcn 语义令牌 + Playwright Best Practices | `make desktop-test` / `desktop-test-e2e` |

---

## 2. 技术栈与官方规范基线

| 技术栈组件 | 当前版本 | 官方规范源 | 状态 |
|-----------|---------|-----------|------|
| Wails v2 | v2.13.0（go.mod） | [wails.io/docs](https://wails.io/docs/) + [wailsapp/wails 源码](https://github.com/wailsapp/wails) | ✅ 锁定 v2 |
| Go | 1.26（go.work） | Effective Go | ✅ |
| React | 18.3.1 | [react.dev/learn](https://react.dev/learn)（18.3 语义一致，19 专属 API 不采用） | ✅ 暂不升级 19 |
| TypeScript | 5.6 | [typescriptlang.org/tsconfig](https://www.typescriptlang.org/tsconfig) | ✅ strict |
| Vite | 5.4.x | [vite.dev](https://vite.dev) | ✅ 锁定 5.4（Wails v2 对 Vite 7+ 有兼容问题，issue #4620） |
| Tailwind CSS | 4.2 | [tailwindcss.com/docs](https://tailwindcss.com/docs)（v4 `@theme`） | ✅ |
| Zustand | 5.0 | [zustand.docs.pmnd.rs](https://zustand.docs.pmnd.rs) | ✅ |
| TanStack Query / Virtual | 5.60 / 3.14 | [tanstack.com](https://tanstack.com) | ✅ Query 待规模化使用 |
| CodeMirror 6 | 6.x | [codemirror.net/docs](https://codemirror.net/docs/) | ✅ 文件查看器 |
| framer-motion | 11 | [motion.dev](https://motion.dev) | ✅ `MotionConfig reducedMotion="user"` |
| 测试 | Vitest 3.2 / Playwright 1.62 | [vitest.dev](https://vitest.dev) / [playwright.dev/docs/best-practices](https://playwright.dev/docs/best-practices) | ⚠️ 组件测试环境缺口（见 §11） |
| 视觉基线 | — | [Apple HIG](https://developer.apple.com/design/human-interface-guidelines/) | ✅ macOS 主平台 |

> **版本锁定原则（M-DSK-WLS-009）**：桌面端依赖版本变更前，先验证与 Wails v2 的兼容性（Vite dev server、TS 绑定生成、WebView2/WKWebView 行为），避免在兼容性上返工。参考 `review-2026-07-27.md` §6.2 的版本结论。

---

## 3. Wails 工程规范

> 来源：Wails 官方文档源码（`wailsapp/wails/website/docs`）与 v2.13.0 本地模块源码核实。Mady `desktop/` 目录已遵循官方标准布局（`main.go` + `app.go` + `frontend/` + `build/` + `wails.json` + `go.mod`）。

### 3.1 项目结构与配置

**规则 M-DSK-WLS-001 (MUST)** — 保持官方布局约定：`main.go` 只做 `wails.Run()` 装配，业务逻辑在 `app.go` 及 `app_*.go` 分文件（当前 `app_files.go` / `app_mcp.go` / `app_settings.go` / `app_skills.go` 已遵循）。

**规则 M-DSK-WLS-002 (MUST)** — `wails.json` 中 `frontend:install` / `frontend:build` / `frontend:dev:serverUrl` 与 `frontend/dist` 的 `go:embed` 保持一致；新增前端依赖后运行 `pnpm install` 并更新 `package.json.md5`。

**规则 M-DSK-WLS-003 (MUST)** — 构建产物不得入库：`.gitignore` 至少忽略 `build/bin`、`node_modules`、`frontend/dist`。**当前差距**：`desktop/` 下存在已构建产物 `Mady` / `desktop.exe`（二进制），需确认其 git 忽略状态（见 §14 P0-2）。

**规则 M-DSK-WLS-004 (SHOULD)** — 生产构建使用 `-trimpath` 剥离源码路径；`-platform darwin/universal` 出双架构（Makefile `desktop-dmg` 已配置）；UPX 与 Apple Silicon 不兼容，不使用。

### 3.2 Binding / IPC 规范

**规则 M-DSK-WLS-005 (MUST)** — 只暴露公开方法（大写开头）；参数/返回值必须 JSON 可序列化（Wails 经 JSON 编解码）；`startup`/`shutdown` 生命周期钩子不对外暴露。

**规则 M-DSK-WLS-006 (MUST)** — 绑定方法的返回 `error` 会令前端 Promise reject：业务方法须返回有语义的错误（`fmt.Errorf("...")` 含上下文），前端 `backend.ts` 统一包装为可读错误提示。

**规则 M-DSK-WLS-007 (MUST)** — `frontend/wailsjs/` 为 CLI 自动生成物（文件头标注 DO NOT EDIT），**禁止手改**；生成类型参与编译（tsconfig `include` 已含 `wailsjs`）。新增/修改 Go 绑定方法后，需重新生成（`wails generate module`）并在 `backend.ts` 服务层补语义化封装。

**规则 M-DSK-WLS-008 (MUST)** — 前端**不直接**触碰 `window.go`（除 `src/lib/wails.ts` 的宿主检测与 `backend.ts` 服务层外）：所有 binding 调用收敛到 `src/lib/backend.ts`，组件层只依赖 `backend.ts` 的类型化 API。此模式便于测试（浏览器环境回退）与未来 Web 版复用。

**规则 M-DSK-WLS-009 (SHOULD)** — 绑定方法不支持可变参数直传（需去掉 spread 传数组）；TS 生成类型偶发不准时用 `ts_type` struct tag 修正，不手改生成文件。

### 3.3 事件系统规范

**规则 M-DSK-WLS-010 (MUST)** — 事件监听必须在 React `useEffect` cleanup 中取消：`listenToWailsEvent(...)` 返回的取消函数必须被持有并在 cleanup 调用（Wails issue #3796/#4683：未清理导致重挂载后回调重复执行、内存累积）。`wails.ts` 已封装「返回取消函数」模式，需审计所有调用方均履行此契约。

**规则 M-DSK-WLS-011 (MUST)** — **不在事件 handler 内部调用取消函数**（Wails v2 通知时复制监听器列表，handler 内 `EventsOff` 会失效，issue #4393）。

**规则 M-DSK-WLS-012 (MUST)** — 事件 payload 保持轻量。Wails v2 事件经 `ExecJS` 整体推送前端，每秒千次推送实测内存持续上涨（issue #1217 / #4587）。流式 token 事件**必须批处理**（如 16ms 合并），禁止逐 token `EventsEmit`。

**规则 M-DSK-WLS-013 (SHOULD)** — 事件名统一 `namespace:event` 冒号命名空间：聊天流用 `agui:*`（已实现），初始化用 `mady:init-*`（已实现）。新事件命名先查表，避免与现有事件冲突。

**规则 M-DSK-WLS-014 (SHOULD)** — 初始化阶段时序：后端 `EventsEmit` 早于前端注册会丢事件（issue #4355）。启动进度类事件在 `OnDomReady` 后注册订阅；必要时后端先保存状态、前端注册后再补发快照。

**规则 M-DSK-WLS-015 (SHOULD)** — macOS 锁屏/后台后 `wails://wails` 进程内存可能显著增长（issue #2772）：避免常驻大块 DOM、控制页面复杂度；长空闲时考虑降低流式订阅密度。

### 3.4 Wails 侧安全（详见 §10）

**规则 M-DSK-WLS-016 (MUST)** — **Wails v2 不内置 CSP**（v2.13.0 源码全量 grep 无 CSP 处理）：CSP 由应用自行注入。当前 `frontend/index.html` 已含 `<meta http-equiv="Content-Security-Policy" content="default-src 'self'; script-src 'self'; ...">`（✅），dev 模式由 `vite.config.ts` 的 `devCspPlugin` 单独放宽 `'unsafe-inline'`（仅 dev 生效，✅ 正确做法）。

---

## 4. 前端工程规范（React / TS / Tailwind）

> 来源：react.dev 官方文档（五步构建法 / Effect 纪律 / memo 纪律）、TypeScript tsconfig 参考、Tailwind v4 官方文档、shadcn/ui 设计原则。所有引用的 React API 在 18.3 行为一致。

### 4.1 React 组件与状态规范

**规则 M-DSK-RCT-001 (MUST)** — 数据向下、事件向上：组件通过 props 接收数据、通过 `onXxxChange` 回调反向更新；状态「提升到最近共同父组件」，形成单一事实来源。

**规则 M-DSK-RCT-002 (MUST)** — 渲染期派生不写 Effect：过滤/派生值在组件顶层直接计算；Effect 只用于同步外部系统（事件订阅、非 React 组件）。当前 `agui-bridge/reducer.ts` 的事件→store 归约即属合法 Effect 用途，但需在 cleanup 清理（§3.3）。

**规则 M-DSK-RCT-003 (MUST)** — 用户事件逻辑放事件处理器，不放 Effect（Effect 运行时无法感知用户动作）；避免 Effect 链。

**规则 M-DSK-RCT-004 (SHOULD)** — useMemo/useCallback 仅作性能优化（官方原文：「只把它当作性能优化」）：三类受益场景为①计算明显慢且依赖少变 ②传给 `memo` 组件的 props ③作为其他 Hook 依赖。不无脑 memo；用组件组合（children 传参）替代大部分手工 memo。

**规则 M-DSK-RCT-005 (MUST)** — 列表 `key` 用数据自带稳定 ID，禁止数组 index 与 `Math.random()`。

**规则 M-DSK-RCT-006 (SHOULD)** — 流式消息等非紧急大更新可用 `startTransition` / `useDeferredValue` 保持 UI 响应；**不能**用于控制文本输入（输入框必须同步）。

**规则 M-DSK-RCT-007 (SHOULD)** — 长列表用 `@tanstack/react-virtual`（依赖已装）；消息时间线是典型场景。

### 4.2 TypeScript 规范

**规则 M-DSK-TS-001 (MUST)** — 保持 `strict: true`（已启用，含 strictNullChecks / noImplicitAny / noUnusedLocals / noUnusedParameters / noFallthroughCasesInSwitch）。

**规则 M-DSK-TS-002 (MUST)** — `wailsjs` 生成类型参与编译（tsconfig `include` 已覆盖），通过 `backend.ts` 语义化封装把生成的 `arg1` 参数包装为具名参数；`src/lib/wails.ts` 用 `typeof EventsOn` 从生成类型推导签名。

**规则 M-DSK-TS-003 (SHOULD)** — `any` 仅限 Wails 宿主边界（`window.go` / `window.runtime` 检测与占位桩）；业务代码新增 `any` 须在 Code Review 说明理由。

### 4.3 Tailwind v4 设计令牌规范

**规则 M-DSK-TW-001 (MUST)** — 颜色/圆角/阴影/时长/缓动/字号全部消费 `globals.css` `@theme` 令牌（`--color-mady-*` / `--radius-*` / `--shadow-mady-*` / `--duration-*` / `--ease-*`），**禁止在组件内硬编码裸色值/像素值**（对照 `desktop-design-development-basis.md` §5.1 开发流程第 3 条）。Code Review 检查 `#` / `rgba(` 直写。

**规则 M-DSK-TW-002 (SHOULD)** — 令牌职责划分：需要生成工具类（`bg-*` / `text-*`）的用 `@theme`；纯 CSS 变量（不需工具类）用 `:root` 定义；令牌引用其他变量时用 `@theme inline`（否则 var() 解析位置会出错）。当前 `globals.css` 大量 var() 引用场景应关注此点。

**规则 M-DSK-TW-003 (决策项)** — 暗色模式策略：当前 `@media (prefers-color-scheme: dark)` 覆盖令牌，只跟随系统。若设置面板提供 light/dark/system 三态切换（02-spec.md §5.6 已规划三档），需迁移为 Tailwind `@custom-variant dark (&:where(.dark, .dark *))` + 类驱动（localStorage + matchMedia，防 FOUC 需内联 head 脚本）。**现状与规划的差异点，见 §14 P1-3。**

**规则 M-DSK-TW-004 (SHOULD)** — 条件 className 组合统一用 `cn()`（clsx + tailwind-merge）。**当前差距**：依赖已在 `package.json`（clsx / tailwind-merge / class-variance-authority），但 `cn()` 工具函数尚未建立（`src/` 下无使用记录）——应新建 `src/lib/utils.ts` 提供 `cn()`（shadcn 生态标准实现），组件变体用 `cva` 声明 variant 映射；`@apply` 克制使用。

### 4.4 组件库设计原则（shadcn/ui 对齐）

**规则 M-DSK-RCT-008 (SHOULD)** — 若引入 shadcn/ui 风格组件，遵循其 Open Code 原则（copy-paste 进源码树、可定制）+ 语义 CSS 变量令牌（`--background` / `--foreground` 成对、`--radius` 单一基准派生圆角阶梯）。当前 A2UI 渲染器为自研组件（`a2ui-renderer/components/`），与 shadcn 原则不冲突——保持「业务组件 vs A2UI Basic 组件」边界（业务组件在 `components/`，A2UI 组件在 `a2ui-renderer/components/`，见 review §3.3）。

**规则 M-DSK-RCT-009 (MUST)** — A2UI 渲染器**禁用 `dangerouslySetInnerHTML` 与 `eval`**；Markdown 渲染使用白名单组件映射（`MarkdownRenderer.tsx`），不注入原始 HTML。

---

## 5. 状态管理分工（Zustand vs TanStack Query）

> 来源：TanStack Query 官方文档（server-state vs client-state）、Zustand 官方指南（Slices / selector / useShallow）。

**规则 M-DSK-ST-001 (MUST)** — 分工总原则：**TanStack Query 管 server-state**（异步数据缓存/同步），**Zustand 管 client-state**（UI 状态/流式会话）。二者互补非竞争（官方原文：「Does TanStack Query replace Redux/MobX/Zustand? — No」）。

**规则 M-DSK-ST-002 (SHOULD)** — 只读后端列表（`ListProjects` / `ListThreads` / `ListModels` / `ListMcpServers` / `GetKnowledgeStatus`）迁移到 TanStack Query：`queryKey` 唯一分层（`['projects']` / `['threads', id]`）、三态渲染（`isPending` → `isError` → 数据）、mutation 成功后 `invalidateQueries`。当前这些调用散落在 `backend.ts` + 组件 Effect 中，是 Query 的候选对象。

**规则 M-DSK-ST-003 (MUST)** — 写操作用 `useMutation`（四态 isIdle/isPending/isError/isSuccess），**禁止用 useQuery 做写操作**。

**规则 M-DSK-ST-004 (MUST)** — 流式会话状态（`stores/chat.ts`）保留 Zustand：订阅必须用 selector（`useStore(s => s.x)`，Object.is 比较决定重渲染）；订阅复合/派生值用 `useShallow`，避免无关字段更新引发整树重渲染。

**规则 M-DSK-ST-005 (SHOULD)** — `stores/chat.ts` 当前为单文件大 store（~30 字段），按 **Slices 模式**切分：`chatSlice` / `threadsSlice` / `commandsSlice` / `settingsSlice`，组合进一个 bound store，`persist` 等 middleware 只加在组合 store 上。

**规则 M-DSK-ST-006 (SHOULD)** — 瞬态 UI 状态（表单输入、hover、面板开合）**不要**盲目提升到全局 store，组件本地 state 优先。

---

## 6. 视觉设计规范（Apple HIG 映射）

> 来源：Apple HIG 官方主题页（Windows / Sidebars / Toolbars / Materials / Color / Dark Mode / Typography / SF Symbols / Layout，经 Apple 官方数据端点逐页核实）+ WebKit 源码 `-apple-*` CSS 关键字核实。

### 6.1 窗口与布局

**规则 M-DSK-VIS-001 (MUST)** — **不隐藏原生窗口控件/不复刻系统窗口外观**：`main.go` 使用 `mac.TitleBarHiddenInset()`（保留交通灯）是正确做法；业务 chrome（侧栏/工具栏/状态栏）画在 Web 内容层。调整 `TitleBarHiddenInset` 时确保交通灯不与 UI 元素重叠（review §3.6 已提示）。

**规则 M-DSK-VIS-002 (SHOULD)** — 侧栏可折叠、两档密度（行高/字号/图标随档缩放），窄窗口自动折叠为图标列（CSS media query / ResizeObserver）；折叠与恢复需双入口（折叠按钮 + 菜单命令）。当前 `Sidebar.tsx` 固定 260px，折叠模式未实现（desktop-design-development-basis §3.1 已标注 ⚠️）。

**规则 M-DSK-VIS-003 (SHOULD)** — 侧栏层级不超过两层；更深结构用内容区列表承接（专利案件→文档树超两层时，第二层放内容区）。当前 `ProjectTree` 需对照评估。

**规则 M-DSK-VIS-004 (MUST)** — **关键操作不放窗口底部**（HIG：用户常把窗口底部移出屏幕）：提交/生成类主操作放工具栏或右侧；底部只放次要状态信息（StatusBar 可保留）。

**规则 M-DSK-VIS-005 (SHOULD)** — 工具栏项**无 bezel**（默认透明，hover 才有底），每屏只设一个 primary（accent 色）主按钮，置于右侧；工具栏分组 ≤3 组，窄窗口次要项自动折叠进「…」溢出菜单。

**规则 M-DSK-VIS-006 (SHOULD)** — 窗口标题简洁（<15 字符）、不用 app 名、能区分多窗口内容（如「权利要求书 - 案件#1024」），随当前案件/文档更新。

**规则 M-DSK-VIS-007 (SHOULD)** — split view 分栏可拖拽、每栏有 min/max 尺寸（最小约 200px）、分隔条 1px 高对比可见；`window_state.go` 已持久化窗口几何，面板比例持久化可后置（§14 P2）。

### 6.2 材质与层次

**规则 M-DSK-VIS-008 (SHOULD)** — **材质（vibrancy）是系统层能力**：WebView 内 CSS `backdrop-filter` 仅是近似。需要侧栏/浮层半透明感时，正确路径是 Wails 原生层在 WebView 下叠 `NSVisualEffectView`（`Material.sidebar` / `.popover`），Web 端对应区域背景透明。当前 `mady-material` 类用 CSS 近似（`backdrop-filter: blur(20px) saturate(180%)`），可作为现阶段实现，后续评估原生叠层。

**规则 M-DSK-VIS-009 (SHOULD)** — inactive 窗口不使用 vibrancy（HIG：key 窗口交通灯彩色、inactive 变灰且无 vibrancy）：前端监听 `window.blur/focus` 弱化失焦状态下的选中高亮与材质。

**规则 M-DSK-VIS-010 (SHOULD)** — 深色模式调色板**不是浅色的简单反色**：当前 `globals.css` 已为 light/dark 各维护一套语义令牌值（✅），遵循「深浅两种外观都必须可用」；尽量让系统关键字（`-apple-*`）自动适配，减少手维护。

### 6.3 色彩与排版

**规则 M-DSK-VIS-011 (MUST)** — 颜色只消费语义令牌，禁止硬编码系统色值（HIG：文档色值仅供设计参考，随版本变化）。可用 WebKit 原生关键字：`-apple-system-label` / `-apple-system-background` / `-apple-system-control-accent` / `-apple-system-separator` 等（WebKit 源码 `CSSValueKeywords.in` 已核实）。当前 `--color-mady-*` 体系已对齐此原则，注意**成功色/链接色用品牌紫**（P0 已修复 ✅）。

**规则 M-DSK-VIS-012 (MUST)** — 对比度：≤17pt 文本 ≥4.5:1，≥18pt 或加粗 ≥3:1（WCAG AA 分档）。当前无自动化审计（§14 P2-5）。

**规则 M-DSK-VIS-013 (MUST)** — 排版基线：macOS 默认 13px body、最小 10px；HIG 文本档位表映射为 CSS token：

| HIG 档位 | 字号/行高 | Mady token |
|---------|-----------|-----------|
| Title 1 | 22/26 | `--font-size-mady-h1: 22px` |
| Title 2 | 17/22 | `--font-size-mady-h2: 16px`（P1 差异，待对齐 17px） |
| Title 3 | 15/20 | `--font-size-mady-heading` |
| Headline / Body | 13/16 | `--font-size-mady-ui: 13px` / `--font-size-mady-body: 14px`（对齐时评估） |
| Callout / Subheadline | 12/15 | — |
| Caption 1/2 | 10/13 | `--font-size-mady-caption: 11px` |

**规则 M-DSK-VIS-014 (MUST)** — 避免细字重：用 Regular(400) / Medium(500) / Semibold(600) / Bold(700)，禁用 100/200/300。

**规则 M-DSK-VIS-015 (SHOULD)** — 字体栈：`-apple-system, BlinkMacSystemFont, "PingFang SC", "Microsoft YaHei", sans-serif`（macOS 中文回退 PingFang SC）；代码块 `JetBrains Mono` + `SF Mono` 回退（当前 `--font-mono` 已含，✅）。

**规则 M-DSK-VIS-016 (SHOULD)** — 图标：UI 图标用 lucide-react（跨平台，已装）；工具栏用 outline 变体，选中态用 accent 色；品牌标识不用系统符号（lucide 可替代 SF Symbols，macOS 端不引入 SF Symbols 字体分发问题）。

### 6.4 主题包机制

**规则 M-DSK-VIS-017 (MUST)** — 4 套主题包（`theme/packs.ts`：professional / focus-blue / paper-warm / slate）保持「语义令牌恒定、仅色值不同」；新增主题只需在 packs.ts 增加一个包，不改组件（已实现 ✅）。

---

## 7. 交互设计规范

> 来源：Apple HIG（Keyboards / Menus / Context menus / Drag and drop / Undo and redo / Alerts / Pointing devices）。

### 7.1 快捷键

**规则 M-DSK-IX-001 (MUST)** — **不占用系统标准快捷键**：⌘A / ⌘C / ⌘V / ⌘X / ⌘Z / ⇧⌘Z / ⌘F / ⌘N / ⌘O / ⌘S / ⌘P / ⌘W / ⌘Q / ⌘, / ⌘? / Esc / ⌘.。标准动作（复制/粘贴/查找/保存）触发标准行为，绝不改绑。

**规则 M-DSK-IX-002 (SHOULD)** — 自定义快捷键修饰键层级：**Command 首选、Shift 次级、Option 仅低频/专业命令、避免 Control**；多修饰键顺序 Control-Option-Shift-Command。如「生成权利要求」用 ⌥⌘G 类组合。

**规则 M-DSK-IX-003 (SHOULD)** — 快捷键集中注册：`stores/commands.ts` 命令注册表（W2-T2 已规划 ⌘K 面板），macOS/Windows 差异（⌘ vs Ctrl）在注册表内平台判断，组件不散落硬编码 keydown。

**规则 M-DSK-IX-004 (MUST)** — 每个核心动作**键盘可达**（Tab 导航 + 快捷键 + 菜单命令三途径至少一）；对话框焦点陷阱可 Esc 退出。

### 7.2 菜单与右键菜单

**规则 M-DSK-IX-005 (SHOULD)** — 右键菜单（contextual menu）：只放与当前上下文最相关的命令、数量少、**隐藏不可用项**（Cut/Copy/Paste 例外可置灰）、子菜单限一层、分隔组 ≤3、**不显示快捷键**；所有右键命令在主界面/菜单栏必可达（禁止「隐藏菜单成为命令唯一入口」）。

**规则 M-DSK-IX-006 (SHOULD)** — 菜单项标签用动词短语、简洁（「生成分析报告」「导入案件」），不可用项置灰，动作需更多信息时加省略号「…」。

### 7.3 拖放与撤销

**规则 M-DSK-IX-007 (SHOULD)** — 拖放语义：同容器=移动、跨容器=复制、跨应用恒为复制；⌥ 键强制复制（检测 `event.altKey`）；拖起即显半透明拖影、目标区高亮、无效目标 `cursor: not-allowed`；拖放操作提供菜单替代方式。

**规则 M-DSK-IX-008 (SHOULD)** — ⌘Z / ⇧⌘Z 全局 undo/redo：文件操作（重命名/删除/写入）入 undo 栈；CodeMirror 编辑器区已有内置 undo；撤销后滚动到受影响位置。

### 7.4 对话框

**规则 M-DSK-IX-009 (MUST)** — 确认对话框仅用于「少见 + 不可撤销 + 破坏性」操作（删除案件、覆盖已存在草稿）；启动不弹窗；错误提示用非模态 toast。

**规则 M-DSK-IX-010 (MUST)** — 对话框按钮规范：动词短语（「删除」而非「OK」）；取消恒为「Cancel」；破坏性按钮红色 destructive 样式且**不做默认按钮**；Esc / ⌘. 取消；取消默认聚焦。

**规则 M-DSK-IX-011 (SHOULD)** — 全局 ⌘K 命令面板（W2-T2 已规划）：宽度 560px、搜索框 52px、毛玻璃背景、命令项 36px、分类标题 11px caption（对照 `desktop-design-development-basis.md` §3.4）。

---

## 8. 无障碍映射

> 来源：Apple HIG（Accessibility / VoiceOver / Motion）+ WAI-ARIA APG。

**规则 M-DSK-A11Y-001 (MUST)** — 对比度 ≥4.5:1（小文本）/ ≥3:1（大文本或加粗）；高对比模式 `@media (prefers-contrast: more)` 加深边框/分割线（`globals.css` 已实现 ✅）。

**规则 M-DSK-A11Y-002 (MUST)** — 最小控件尺寸 28×28pt（图标按钮可点击区外扩到 28px）；控件间距带边框 12px / 无边框 24px。

**规则 M-DSK-A11Y-003 (MUST)** — 语义 HTML + ARIA：唯一 `title`、heading 层级准确（每面板一个 h1 语义，业务文档块用 h2/h3）、图片 `alt`、装饰图标 `aria-hidden`、动态内容 `aria-live`、对话框 `role="dialog"` + `aria-modal`。按钮用 `aria-pressed`、菜单按钮 `aria-haspopup`。

**规则 M-DSK-A11Y-004 (MUST)** — 尊重减弱动态效果：`@media (prefers-reduced-motion: reduce)` 禁用进场动画/自动滚动（`globals.css` ✅）+ framer-motion `MotionConfig reducedMotion="user"`（`main.tsx` ✅）；流式打字不用逐字动画。

**规则 M-DSK-A11Y-005 (MUST)** — **不只靠颜色**传达状态：错误/成功/警告用「图标 + 文字 + 颜色」三重编码（TUI 规范 M-TUI-STA-001 的桌面端对应）。

**规则 M-DSK-A11Y-006 (SHOULD)** — 焦点管理：可见焦点环（`--focus-ring` 已定义）、Tab 顺序合理、对话框焦点陷阱可 Esc 退出、失焦选中态用 `unemphasized` 样式。

**规则 M-DSK-A11Y-007 (SHOULD)** — 文本可放大至少 200% 不截断：布局用可伸缩单位 / `clamp()`，避免固定高度截断。

> macOS 无 Dynamic Type（HIG 明确），大字号诉求靠浏览器缩放/分辨率，前端做到布局不破即可。

---

## 9. 性能规范

> 来源：Wails 官方 Options/CLI 参考 + 社区案例（RWKV-Runner / Image-Studio / tiny-rdm）+ React 官方性能纪律。

### 9.1 流式渲染

**规则 M-DSK-PRF-001 (MUST)** — 流式 token **批处理渲染**：每 16ms 合并一批 token 再 setState，避免每字触发整轮 re-render；尾部「已完成增量 + 待续」状态分离。

**规则 M-DSK-PRF-002 (SHOULD)** — 长对话消息时间线用 `@tanstack/react-virtual` 虚拟化；`MessageBubble` 用 `memo` 隔离（仅当前流式消息 re-render）。

**规则 M-DSK-PRF-003 (SHOULD)** — CSS containment：消息气泡加 `content-visibility: auto`（社区实测可减 40-60% 渲染时间）。

**规则 M-DSK-PRF-004 (MUST)** — 事件 payload 轻量 + 批处理（呼应 M-DSK-WLS-012）：Wails v2 事件经 ExecJS 全量推送，流式事件不得逐 token 高频 emit。

### 9.2 构建与体积

**规则 M-DSK-PRF-005 (MUST)** — 构建产物 manualChunks 拆分（`vite.config.ts` 已实现：pdfjs / codemirror / react / ui 独立 chunk ✅）；主 entry 目标 <500KB。

**规则 M-DSK-PRF-006 (SHOULD)** — `go:embed` 全量内嵌 `frontend/dist`：静态资源（`pdfjs-dist/cmaps` 100+ 文件、字体）直接决定二进制体积。定期检查 dist 体积；cmaps 已按需复制（`copyCmapsPlugin` ✅）。

### 9.3 长任务可靠性

**规则 M-DSK-PRF-007 (SHOULD)** — 长分析任务（专利检索/证据判断，分钟级）借鉴 Image-Studio 的抗断连设计：后端 SSE 心跳/重连/游标恢复，WebSocket 作为降级通道；前端对「已完成增量 + 尾部待续」分段渲染。当前桌面端走 Wails Events（同进程，无网络断连问题），此条针对未来 Web 版/远程模式。

**规则 M-DSK-PRF-008 (SHOULD)** — 停止按钮用 AbortController 语义：取消后端流并释放资源（`cancelChat(runId)` 已实现，前端需保证调用）。

---

## 10. 安全规范

> 来源：Wails 官方（Options / Dynamic Assets / Signing / Community Templates）+ review-2026-07-27 §3.6 + 项目安全红线（AGENTS.md）。

**规则 M-DSK-SEC-001 (MUST)** — CSP 由应用注入（Wails v2 不内置）：生产 `script-src 'self'`（index.html 已配置 ✅）；dev 放宽仅限 dev（`devCspPlugin` ✅）；新增内联脚本/样式需先评估 CSP 影响。

**规则 M-DSK-SEC-002 (MUST)** — 绑定最小暴露：不暴露文件系统/exec 级 binding；文件操作（ReadFile/WriteFile/DeleteEntry/ListDirectory/CreateFolder/RenameFolder）**全部经 `tools/path.go` 沙箱路径校验**（安全红线路径，改动须人工审阅）；`DeleteEntry` 仅允许空目录或文件、非递归删除。

**规则 M-DSK-SEC-003 (MUST)** — A2UI 渲染器禁用 `dangerouslySetInnerHTML` 与 `eval`（M-DSK-RCT-009 重申）；`openUrl` 函数仅允许 `http://` / `https://` 协议（渲染器必须拦截其他协议）；A2UI envelope 做结构校验，防 oversized payload 渲染卡顿。

**规则 M-DSK-SEC-004 (MUST)** — API Key / 凭证不落前端：AI 设置经 `SetAISettings` 走后端持久化（`~/.mady/desktop-settings.json`）；`ListMcpServers` 的 env 仅暴露键名（已实现 ✅）。

**规则 M-DSK-SEC-005 (SHOULD)** — `BindingsAllowedOrigins` 限制 JS↔Go binding 合法来源（三平台统一校验）。

**规则 M-DSK-SEC-006 (SHOULD)** — production 下 `EnableDefaultContextMenu` 关闭（官方警告其非安全措施，但会泄漏「下载图片/保存网页」能力）；需要时提供自定义右键菜单（M-DSK-IX-005）。

**规则 M-DSK-SEC-007 (SHOULD)** — `EnableFraudulentWebsiteDetection` 保持默认 false（开启会把 URL 等应用信息发到 Apple/Microsoft 云扫描，隐私敏感）。

**规则 M-DSK-SEC-008 (SHOULD)** — 签名凭据走 CI Secrets（GitHub Secrets 存 base64 证书 + 密码），**禁止落仓库**；`Makefile desktop-notarize` 用环境变量展开（已符合 ✅）。

**规则 M-DSK-SEC-009 (SHOULD)** — `AssetsHandler` 暴露文件系统是官方明示风险：当前未使用动态资源服务，若引入须严格管理访问路径；注意 vite v5 + wails v2 动态资源不兼容（issue #3240）。

**规则 M-DSK-SEC-010 (SHOULD)** — Invisible Handoff 契约（安全红线）：前端对 `handoff-start/end` 事件静默、过滤 `transfer_to_*` 工具调用（02-spec.md §1.2 已定义，代码实现需保持）。

---

## 11. 测试与质量门禁

> 来源：Testing Library / Vitest / Playwright 官方 best practices + 本地测试配置核实。

### 11.1 分层测试矩阵

| 层 | 工具 | 覆盖目标 | 当前状态 |
|----|------|---------|---------|
| 纯逻辑/store | Vitest（node 环境） | SurfaceStore、datamodel、dynamic 解析、reducer | ✅ `vitest.config.ts` |
| 组件 | Vitest + jsdom + Testing Library | 组件交互、状态转换、主题切换 | ❌ **环境缺口**（见 M-DSK-TST-002） |
| e2e | Playwright | AC-1~AC-5 关键路径 | ✅ `playwright.config.ts`（chromium + webkit） |
| Go | `go test -race` | App 方法、生命周期、事件透传 | ✅ `desktop/` 模块 |

**规则 M-DSK-TST-001 (MUST)** — 三层测试齐备：新组件必须有组件测试（非仅纯函数测试）。

**规则 M-DSK-TST-002 (MUST)** — 补齐组件测试环境：当前 `vitest.config.ts` 为 `environment: 'node'`，`*.test.tsx` 组件测试（jsdom + `@testing-library/jest-dom` matchers）未被覆盖。方案：新建 `vitest.component.config.ts`（jsdom + setup 文件）或按文件后缀分流环境；升级 Vitest 前注意其 Vite ≥6 的版本要求（当前 Vite 5.4）。

**规则 M-DSK-TST-003 (MUST)** — Testing Library 查询优先级：`getByRole`（首选，带 name）> `getByLabelText` > `getByPlaceholderText` > `getByText` > `getByTestId`（最后手段）；异步断言用 `findBy` / `waitFor`；用户交互用 `user-event`（非 fireEvent）。

**规则 M-DSK-TST-004 (MUST)** — Playwright：locator 优先用户可见属性（`getByRole('button', { name })`），用 web-first assertions（`await expect(locator).toBeVisible()`），禁止手动 `isVisible()` 同步断言（当前配置已对齐 ✅）。

**规则 M-DSK-TST-005 (MUST)** — Wails 事件监听器测试须验证 cleanup（防泄漏回归）；Go↔TS 契约测试（backend.ts 包装类型 vs `wailsjs` 生成类型）在 CI 中校验漂移。

**规则 M-DSK-TST-006 (SHOULD)** — 前端测试纳入 `make desktop-test`（当前 Makefile 的 `desktop-test` 只跑 Go 测试；`desktop-test-e2e` 已存在但独立）；CI 中 `pnpm typecheck && pnpm test && pnpm build` 全过。

**规则 M-DSK-TST-007 (SHOULD)** — 新增组件 ≥70% 行覆盖，且必须测试：空数据 / 超长文本 / 最小宽度边界、主题切换后令牌生效、`-race`（Go 侧）与 React StrictMode 双渲染。

**规则 M-DSK-TST-008 (SHOULD)** — CI 增加对比度审计（遍历 `globals.css` 令牌对算 WCAG 对比度，≥4.5:1），与 TUI 规范 M-TUI-TST-005 对齐。

---

## 12. 打包与发布

> 来源：Wails Code Signing / NSIS / CLI 参考 + 社区案例（WailBrew / RWKV-Runner / tiny-rdm）+ `desktop-notarization-assessment.md`。

**规则 M-DSK-PKG-001 (MUST)** — macOS 签名 + 公证全链路：`codesign --timestamp --options=runtime` → `notarytool submit --wait` → `stapler staple` → `spctl --assess`（Makefile `desktop-notarize` 已配置 ✅）；`Info.plist` 的 Bundle ID 与签名配置一致（`com.mady.desktop`）；沙箱/网络权限在 `entitlements.plist` 声明。

**规则 M-DSK-PKG-002 (SHOULD)** — Windows 适配（P2 后置，构建不 panic 即可）：标题栏 32px / Segoe UI Variable 字体栈 / 10px 传统滚动条 / Ctrl 快捷键 / 最小尺寸 900×600（`main_windows.go` build tag）；WebView2 运行时用 `-webview2 download|embed|browser` 策略，管理员权限场景处理 `WebviewUserDataPath`（官方 Troubleshooting 三方案）。

**规则 M-DSK-PKG-003 (SHOULD)** — 自动更新预留：Wails 生态无官方 autoupdate，参考 RWKV-Runner（内置更新 + 保留用户配置）与 jcp（前端 `updateService.ts`）；早期预留更新检查通道与版本接口（`Health().Version` 已有）。

**规则 M-DSK-PKG-004 (SHOULD)** — 单实例锁：`SingleInstanceLock{ UniqueId, OnSecondInstanceLaunch }` 防多开（数据库/端口资源场景）。

**规则 M-DSK-PKG-005 (SHOULD)** — 托盘 + 通知（P2）：最小化到托盘 + 长任务完成系统通知（`window.Hide()` + 系统通知），让长分析不阻塞用户；托盘图标复用 AppIcon。

**规则 M-DSK-PKG-006 (SHOULD)** — 多平台 CI：参考 Wails 官方 workflow（matrix: ubuntu/windows/macos，Linux 需 `libgtk-3-dev` + `libwebkit2gtk-4.0-dev`，macOS 需 `CGO_LDFLAGS=-framework UniformTypeIdentifiers -mmacosx-version-min=10.13`，`GOWORK=off`）；macOS 工作流内置公证模板（参考 tiny-rdm 的 gon 注释模板）。

**规则 M-DSK-PKG-007 (SHOULD)** — 体积叙事：Wails 单二进制 15-25MB 是传播卖点（对照 GoNavi「~30MB vs Electron 数百 MB」）；README/发布页建立同类对比表。

---

## 13. 成功案例参考

> 调研日期 2026-07-31，stars 为 GitHub API 实测。完整清单见调研报告（Sources 中 jcp / tiny-rdm / RWKV-Runner 等链接）。

### 13.1 Wails 生态（技术栈直接对标）

| 项目 | stars | 技术栈 | 定位 | 可借鉴点 |
|------|-------|--------|------|---------|
| [tiny-rdm](https://github.com/tiny-craft/tiny-rdm) | 13.0k | Go + Vue 3 | Redis 桌面管理器（桌面+Web 双形态） | 后端 api/services/storage 分层、PubSub 实时推送、桌面/Web 同代码库 build tag 分流、12 语言 i18n、分平台 CI |
| [RWKV-Runner](https://github.com/josStorer/RWKV-Runner) | 6.4k | Go + React 18 + MobX | 本地大模型运行器 + AI 聊天客户端 | SSE 流式渲染（fetch-event-source）、内置自动更新、前端独立部署 WebUI、PDF 查看（pdfjs-dist） |
| [go-stock](https://github.com/ArvinLovegood/go-stock) | 7.1k | Go + Vue 3 | AI 股票分析（多 LLM 接入） | 多 Provider 统一适配层、本地数据存储、AI 流式输出 |
| [GoNavi](https://github.com/Syngnat/GoNavi) | 1.8k | Go + React 18 | 数据库客户端（AI + MCP first-class） | MCP 工具集成、AI 助手面板、Dockerfile 多形态部署、~30MB 体积对比叙事 |
| [jcp](https://github.com/run-bigpig/jcp) | 1.3k | Go 1.24 + React 18 + TS + Vite + Tailwind | AI 多 Agent 协作股票分析 | **与 Mady 相似度最高**：`internal/adk` 多模型适配 + MCP 管理器 + 工具注册表、Agent 会议室（工具调用展示 + 失败重试）、布局持久化、本地 JSON 配置 |
| [Image-Studio](https://github.com/RoseKhlifa/Image-Studio) | 430 | Go + React/TS | 图像生成/编辑客户端 | SSE/WebSocket 双传输抗长推理断连（524/504）、Cmd/Ctrl+Enter 快捷键 |
| [WailBrew](https://github.com/wickenico/WailBrew) | 2.7k | Go + React | macOS Homebrew GUI | 签名 + 公证 + brew cask 分发、11 语言 i18n、GitHub Actions 双工作流 |
| [ChatY](https://github.com/CiroLee/ChatY) | 31 | Go + Wails | GPT 桌面客户端 | 可控历史会话（省 token）、快捷键、本地 indexDB 存储 |

### 13.2 非 Wails AI 桌面应用（产品交互参考）

| 项目 | stars | 实际技术栈 | 参考价值 |
|------|-------|-----------|---------|
| [lobehub](https://github.com/lobehub/lobehub) | 81.0k | TypeScript（原 LobeChat） | 插件市场、助手市场、多模型路由 |
| [Cherry Studio](https://github.com/CherryHQ/cherry-studio) | 49.2k | Electron + React | 会话管理、智能体编排、知识库持久化 |
| [Jan](https://github.com/janhq/jan) | 43.8k | Tauri + llama.cpp | 本地优先叙事、模型管理 |
| [Chatbox](https://github.com/ChatboxAI/chatbox) | 41.2k | Electron + React/TS | 流式回复、快捷键、多 Provider、本地存储 |

### 13.3 对 Mady 的可借鉴模式（Top 8）

1. **工具调用状态机**（jcp）：把「工具注册 → 参数校验 → 调用 → 结果卡片 → 失败重试」做成事件驱动管线，ToolCard 渲染 pending/running/done/failed 四态，而非轮询。
2. **前后端 API 收敛**（tiny-rdm/jcp）：Wails binding 集中 `services` 层，组件不直触 `window.go`——Mady 的 `backend.ts` 已遵循。
3. **桌面 + Web 双形态**（tiny-rdm/RWKV-Runner）：binding 收口为薄适配器后，未来低成本产出 Web 演示版。
4. **可控上下文 + 滚动摘要**（ChatY/jcp）：会话按 token 预算裁剪/压缩历史，长会话滚动摘要写入会话头——直接影响桌面端长对话体验。
5. **集中配置管理器 + 原子读写**（jcp/SydneyQt）：版本化 JSON 结构体 + 迁移；注意 API Key 存储加固。
6. **布局/窗口状态持久化**（jcp + Wails WindowState）：split 比例与面板开关状态持久化。
7. **多 Provider 统一适配**（go-stock/jcp）：model_factory + 响应格式归一化沉淀为可测试独立包（Mady provider 层已类似）。
8. **快捷键矩阵**（Chatbox/Image-Studio）：统一注册 + 平台判断（⌘/Ctrl），可配置化。

---

## 14. 差距分析与改进路线图

### 14.1 差距汇总

| 优先级 | 缺失功能 | 规范依据 | 影响 | 状态 |
|--------|---------|---------|------|------|
| **P0-1** | 组件测试环境缺失（vitest 仅 node） | M-DSK-TST-002 | `*.test.tsx` 未跑、回归无法拦截 | ✅ 已修复（2026-07-31：vitest.component.config.ts + jest-dom/vitest） |
| **P0-2** | 构建产物入库（`Mady` / `desktop.exe` 二进制未忽略） | M-DSK-WLS-003 | 仓库膨胀、跨机器二进制不一致 | ✅ 已核实为误报（根 .gitignore + desktop/.gitignore 已覆盖，git status 无二进制） |
| **P0-3** | 事件监听 cleanup 契约审计 | M-DSK-WLS-010 | 组件重挂载回调重复/内存累积 | ✅ 已审计（2026-07-31：4 个订阅点全部合规，报告 docs/review/desktop-event-cleanup-audit-2026-07-31.md） |
| **P1-1** | `wailsjs` 类型漂移校验（CI） | M-DSK-TST-005 | 前后端契约漂移难发现 | ✅ 已实施（2026-07-31：scripts/check-wailsjs-contract.mjs，26 方法全匹配，可入 CI） |
| **P1-2** | Zustand store 按 slices 切分 | M-DSK-ST-005 | chat.ts 单文件膨胀 | ✅ 已实施（2026-07-31：stores/slices/ 三 slice 组合，对外 API 兼容） |
| **P1-3** | 暗色模式三态切换（prefers-color-scheme → class 策略） | M-DSK-TW-003 | 设置面板三档切换与现状不一致 | ✅ 已实施（2026-07-31：决策=完整实现；@custom-variant dark + .dark class + public/theme-init.js 防 FOUC） |
| **P1-4** | 前端 i18n（react-i18next 对齐 `pkg/i18n`） | 案例参考 §13.1 | 专利/法律术语翻译一致性 | ✅ 已决策（2026-07-31：评估完成见 docs/plans/desktop-i18n-assessment.md；**产品决策：发布前实施，届时只做 zh-CN 单语言版**，不引入 react-i18next 运行时） |
| **P1-5** | TanStack Query 接管只读列表 | M-DSK-ST-002 | 重复 Effect 拉取、无缓存失效 | ✅ 已实施（2026-07-31：src/queries/ 5 个 hook + 7 个组件迁移） |
| **P1-6** | 工具栏/侧栏 HIG 对齐走查（无 bezel、单 primary、可折叠侧栏） | M-DSK-VIS-002/005 | 视觉与原生感差距 | ✅ 已实施（2026-07-31：侧栏 48px 折叠态 + ⌘B + 窄窗口自动折叠；toolbar 无 bezel 已确认） |
| **P2-1** | 全局 ⌘K 命令面板 | M-DSK-IX-011 | 命令发现性 | ✅ 已存在（CommandPalette 已实现） |
| **P2-2** | TodoDock 底部待办坞 | 案例参考 + 02-spec | 任务可视化 | ✅ 已存在（TodoDock 已实现） |
| **P2-3** | CI 对比度审计 | M-DSK-TST-008 | 无障碍无法自动验证 | ✅ 已闭环（2026-07-31：scripts/check-color-contrast.mjs 修复 5 类真实问题 → 60 PASS / 0 FAIL / 20 EXEMPT，脚本已接入 ci.yml desktop job） |
| **P2-4** | Windows 完整适配（标题栏/字体/滚动条/快捷键） | M-DSK-PKG-002 | 跨平台体验 | ⚠️ 部分完成（前端平台适配层已就绪：main.tsx data-platform + globals.css win32 覆盖，GOOS=windows/linux 交叉编译通过；Go 侧 App 实现跨平台化与 CI matrix 待 W3-T4 排期） |
| **P2-5** | 托盘 + 长任务通知 | M-DSK-PKG-005 | 长任务不阻塞 | ✅ 已实施（2026-07-31：getlantern/systray 托盘 + osascript 系统通知，desktop/tray.go；**实机验证待做**：make desktop-run + 通知权限） |
| **P2-6** | 自动更新预留 | M-DSK-PKG-003 | 分发迭代成本 | ✅ 已实施占位（2026-07-31：评估见 docs/plans/desktop-autoupdate-assessment.md；CheckUpdate() 绑定 + 设置面板「检查更新」入口已落地，返回「已是最新版本」） |
| **P2-7** | 布局/面板比例持久化 | 案例参考 §13.3-6 | 多面板体验 | ✅ 已实施（2026-07-31：settings store 布局字段 + Sidebar/ChatView 折叠联动，localStorage 持久化） |
| **P2-8** | 初始化失败前端无感知（`mady:init-error` 未订阅） | W4-T3 观察项 | Provider 配置错误时用户看不到原因 | ✅ 已实施（2026-07-31：SplashScreen 订阅 init-error，错误态停住加载层 + 提示文案，兜底计时器失效） |
| **P2-9** | 单实例锁 | M-DSK-PKG-004 | 多开会话/数据库资源无保护 | ✅ 已实施（2026-07-31：wails.Run SingleInstanceLock `com.mady.desktop` + 第二实例聚焦窗口，focusMainWindow nil 防御） |
| **P2-10** | BindingsAllowedOrigins 未显式配置 | M-DSK-SEC-005 | JS↔Go binding 来源未收敛 | ✅ 已实施（2026-07-31：仅放行 http/https localhost） |
| **P2-11** | 体积叙事缺位 | M-DSK-PKG-007 | 传播卖点未利用 | ✅ 已实施（2026-07-31：README 桌面端章节体积对比表 + 平台状态） |
| **P2-12** | 全局 ⌘Z / ⇧⌘Z undo/redo | M-DSK-IX-008 | 文件操作无撤销 | ⏳ 待排期（文件重命名/删除/写入入 undo 栈，涉及 store 设计） |
| **P2-13** | 右键菜单覆盖不全（仅 ProjectTree） | M-DSK-IX-005 | 会话/知识面板无上下文菜单 | ⏳ 待排期（会话列表重命名/删除/导出 + 隐藏不可用项） |
| **P2-14** | 多平台 CI matrix（仅 macos-latest） | M-DSK-PKG-006 | Windows/Linux 构建无 CI 兜底 | ⏳ 待排期（随 W3-T4 Windows 适配一并接入） |
| **P2-15** | 消息气泡无 CSS containment | M-DSK-PRF-003 | 长对话渲染性能 | ⏳ 待排期（content-visibility: auto 需实测避免布局副作用） |

### 14.2 优先级定义

- **P0 (MUST)** — 阻塞正确性或工程质量门禁
- **P1 (SHOULD)** — 影响体验或开发效率
- **P2 (COULD)** — 未来增强

### 14.3 改进路线图（建议顺序）

| 阶段 | 内容 | 关联任务 | 状态 |
|------|------|---------|------|
| **Sprint A: 质量门禁** | P0-1 组件测试环境 + P0-2 构建产物治理 + P0-3 事件 cleanup 审计 + P1-1 契约漂移校验 | 1-2 天 | ✅ 已完成（2026-07-31） |
| **Sprint B: 状态与主题** | P1-2 Zustand slices + P1-5 TanStack Query 接管 + P1-3 暗色模式三态切换 | 3-5 天 | ✅ 已完成（2026-07-31） |
| **Sprint C: 交互与视觉** | P2-1 ⌘K 面板 + P2-2 TodoDock + P1-6 HIG 走查（可折叠侧栏/toolbar 对齐） | 5-7 天 | ✅ 已完成（2026-07-31，⌘K/TodoDock 此前已实现） |
| **Sprint D: 平台完善** | P2-4 Windows 适配 + P2-5 托盘通知 + P2-3 对比度审计 + P2-6 自动更新评估 | 3-5 天 | ⚠️ 部分完成（托盘/审计/评估已完成；Windows 适配在 W3-T4） |

> **2026-07-31 全面执行注记**：Sprint A-C 全部完成，Sprint D 中 P2-3/P2-5/P2-6 完成，P2-4（Windows）仍在 W3-T4 计划内。
>
> **2026-07-31 二次执行注记**（对比度修复 + 门禁闭环）：P2-3 对比度 5 类真实问题已修复（语义色令牌按 WCAG AA 校准：light danger #C62828 / warning #9C4F00 / info #0066E6，dark success #8A88E8 / info #2E9BFF / text-secondary #9A958F；text-tertiary 维持 Apple HIG tertiary 层级并纳入豁免，脚本 60 PASS / 0 FAIL）。两个审计脚本 + pnpm typecheck + 构建产物防回归已接入 ci.yml desktop job。新增识别差距 P2-8~P2-15（P2-8~P2-11 已实现，P2-12~P2-15 待排期）。i18n（P1-4）产品决策：**发布前实施，只做 zh-CN 单语言版**。
>
> **2026-07-31 审查修复注记**（code-reviewer 深度审查 + 全部修复）：5 BLOCKER + 25 IMPORTANT 全部落地——沙箱 symlink 逃逸（对齐 tools/path.go，含 4 个逃逸/回归测试）、A2UI 渲染冻结（快照浅拷贝 + zustand 订阅）、会话列表死数据（App 挂载 useThreads）、SlashCommandMenu 条件 hooks、thinking-fold 结构化、server/ctx 原子化与 settings 并发锁、e2e tag 隔离、wails.ts 静态导入、ModalShell 无障碍封装（5 覆盖层）、eslint 门禁（rules-of-hooks error）。遗留标注：托盘实机验证（P2-5）、Windows Go 侧跨平台化（P2-4）、openUrl/通知实机确认。

---

## 15. 附录：Sources

### 15.1 官方规范（一手来源）

**Apple HIG**（经官方数据端点逐页核实）：
- [Windows](https://developer.apple.com/design/human-interface-guidelines/windows) · [Sidebars](https://developer.apple.com/design/human-interface-guidelines/sidebars) · [Toolbars](https://developer.apple.com/design/human-interface-guidelines/toolbars) · [Panels](https://developer.apple.com/design/human-interface-guidelines/panels) · [Split views](https://developer.apple.com/design/human-interface-guidelines/split-views) · [Materials](https://developer.apple.com/design/human-interface-guidelines/materials) · [Color](https://developer.apple.com/design/human-interface-guidelines/color) · [Dark Mode](https://developer.apple.com/design/human-interface-guidelines/dark-mode) · [Typography](https://developer.apple.com/design/human-interface-guidelines/typography) · [SF Symbols](https://developer.apple.com/design/human-interface-guidelines/sf-symbols) · [Menus](https://developer.apple.com/design/human-interface-guidelines/menus) · [Context menus](https://developer.apple.com/design/human-interface-guidelines/context-menus) · [Drag and drop](https://developer.apple.com/design/human-interface-guidelines/drag-and-drop) · [Undo and redo](https://developer.apple.com/design/human-interface-guidelines/undo-and-redo) · [Alerts](https://developer.apple.com/design/human-interface-guidelines/alerts) · [Keyboards](https://developer.apple.com/design/human-interface-guidelines/keyboards) · [Accessibility](https://developer.apple.com/design/human-interface-guidelines/accessibility) · [VoiceOver](https://developer.apple.com/design/human-interface-guidelines/voiceover) · [Motion](https://developer.apple.com/design/human-interface-guidelines/motion) · [Web views](https://developer.apple.com/design/human-interface-guidelines/web-views) · [Layout](https://developer.apple.com/design/human-interface-guidelines/layout)
- [AppKit NSVisualEffectView.Material](https://developer.apple.com/documentation/appkit/nsvisualeffectview/material-swift.enum) · [WebKit CSSValueKeywords.in（`-apple-*` 关键字）](https://github.com/WebKit/WebKit/blob/main/Source/WebCore/css/CSSValueKeywords.in) · [MDN prefers-contrast](https://developer.mozilla.org/en-US/docs/Web/CSS/@media/prefers-contrast)

**Wails v2**（官方文档源码 + v2.13.0 本地源码核实）：
- [Wails 官方文档源仓库](https://github.com/wailsapp/wails/tree/master/website/docs) · [wailsapp/wails](https://github.com/wailsapp/wails) · [Options 参考](https://wails.io/docs/reference/options) · [Events 参考](https://wails.io/docs/reference/runtime/events) · [Code Signing 指南](https://wails.io/docs/guides/signing) · [Troubleshooting](https://wails.io/docs/guides/troubleshooting) · [NSIS Installer](https://wails.io/docs/guides/windows-installer) · [Obfuscated Builds](https://wails.io/docs/guides/obfuscated) · [Dynamic Assets](https://wails.io/docs/guides/dynamic-assets) · [awesome-wails](https://github.com/wailsapp/awesome-wails)
- 关键 issue：[#1217 事件内存泄漏](https://github.com/wailsapp/wails/issues/1217) · [#2772 WKWebView 内存](https://github.com/wailsapp/wails/issues/2772) · [#4587 v3 事件拉取模型](https://github.com/wailsapp/wails/issues/4587) · [#4393 handler 内取消失效](https://github.com/wailsapp/wails/issues/4393) · [#4683 监听器未清理](https://github.com/wailsapp/wails/issues/4683) · [#3240 Vite v5 动态资源](https://github.com/wailsapp/wails/issues/3240) · [#4620 Vite 7 兼容](https://github.com/wailsapp/wails/issues/4620)

**前端技术栈**：
- [React — Thinking in React](https://react.dev/learn/thinking-in-react) · [React — You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect) · [React — useMemo](https://react.dev/reference/react/useMemo) · [React — startTransition](https://react.dev/reference/react/startTransition) · [React — Rendering Lists](https://react.dev/learn/rendering-lists) · [React — DOM 通用属性](https://react.dev/reference/react-dom/components/common)
- [Tailwind CSS v4 — Theme Variables](https://tailwindcss.com/docs/theme) · [Tailwind CSS v4 — Dark Mode](https://tailwindcss.com/docs/dark-mode) · [Tailwind CSS v4 — Functions and Directives](https://tailwindcss.com/docs/functions-and-directives)
- [shadcn/ui — Introduction](https://ui.shadcn.com/docs) · [shadcn/ui — Theming](https://ui.shadcn.com/docs/theming) · [shadcn/ui — Dark Mode / Vite](https://ui.shadcn.com/docs/dark-mode/vite)
- [Zustand — Slices Pattern](https://zustand.docs.pmnd.rs/guides/slices-pattern) · [Zustand — useShallow](https://zustand.docs.pmnd.rs/guides/prevent-rerenders-with-use-shallow) · [Zustand — Testing](https://zustand.docs.pmnd.rs/guides/testing)
- [TanStack Query — Queries](https://tanstack.com/query/latest/docs/framework/react/guides/queries) · [TanStack Query — Mutations](https://tanstack.com/query/latest/docs/framework/react/guides/mutations) · [TanStack Query — Does it replace client state?](https://tanstack.com/query/latest/docs/framework/react/guides/does-this-replace-client-state)
- [Testing Library — About Queries](https://testing-library.com/docs/queries/about) · [Testing Library — Guiding Principles](https://testing-library.com/docs/guiding-principles) · [Playwright — Best Practices](https://playwright.dev/docs/best-practices) · [Vitest](https://vitest.dev/guide/) · [W3C WAI-ARIA APG — Button Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/button/) · [TypeScript — tsconfig](https://www.typescriptlang.org/tsconfig)

### 15.2 成功案例（开源社区）

**Wails 生态**：
- [wailsapp/wails](https://github.com/wailsapp/wails) · [tiny-craft/tiny-rdm](https://github.com/tiny-craft/tiny-rdm) · [josStorer/RWKV-Runner](https://github.com/josStorer/RWKV-Runner) · [ArvinLovegood/go-stock](https://github.com/ArvinLovegood/go-stock) · [GUI-for-Cores/GUI.for.SingBox](https://github.com/GUI-for-Cores/GUI.for.SingBox) · [wickenico/WailBrew](https://github.com/wickenico/WailBrew) · [Syngnat/GoNavi](https://github.com/Syngnat/GoNavi) · [run-bigpig/jcp](https://github.com/run-bigpig/jcp) · [juzeon/SydneyQt](https://github.com/juzeon/SydneyQt) · [RoseKhlifa/Image-Studio](https://github.com/RoseKhlifa/Image-Studio) · [CiroLee/ChatY](https://github.com/CiroLee/ChatY)

**非 Wails AI 桌面应用（产品参考）**：
- [lobehub/lobehub](https://github.com/lobehub/lobehub) · [CherryHQ/cherry-studio](https://github.com/CherryHQ/cherry-studio) · [janhq/jan](https://github.com/janhq/jan) · [ChatboxAI/chatbox](https://github.com/ChatboxAI/chatbox)

### 15.3 项目内关联文档

- [desktop-design-development-basis.md](specs/desktop/desktop-design-development-basis.md) — 视觉 Token 映射与走查表（本文档 §6 的 Token 依据）
- [02-spec.md](specs/desktop/02-spec.md) — 桌面端规格（事件映射/接口契约/主题）
- [desktop-next-development-plan.md](plans/desktop-next-development-plan.md) — 缺口计划（W1-W3，本文档 §14 路线图衔接）
- [review-2026-07-27.md](specs/desktop/review-2026-07-27.md) — 设计审阅（技术选型对比/开源实践）
- [mady-tui-standards.md](mady-tui-standards.md) — TUI 社区对齐规范（规则编号体系参照）

---

> 本文档是活文档，随桌面端架构演进持续更新。
> 最新同步：2026-07-31 | 配套文件：`docs/specs/desktop/`、`docs/plans/desktop-next-development-plan.md`
> 调研声明：所有外部规则均来自 2026-07-31 实际核实的官方文档/源码/issue；HIG 三处无明文项（侧栏像素宽度、Liquid Glass 对 Web 内容、WebView 原生感融合）已在调研中标注「未核实」，不构成规范断言。
