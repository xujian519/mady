# 04 — 任务拆解：Mady 桌面端

- **功能名**：desktop
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **拆解日期**：2026-07-27
- **状态**：阶段 1-3 待开工 | 阶段 4 预留
- **依赖设计**：[03-design.md](./03-design.md)

> 每个任务标注：**涉及文件范围**、**验收**、**风险等级**、**审查要求**。
> 遵循 AGENTS.md「单次改动 3-5 文件」「小炸弹不是大炸弹」原则，任务粒度对应一次提交。
> 审查等级：L1（自动）｜L2（人审）｜L3（涉安全红线必人审）｜L4（架构变更必人审）

---

## 阶段 1：骨架打通（Wails + React + SSE 透传）

**阶段目标**：Wails 模块跑起来，内嵌 server，前端能收到 `agui:message-delta` 流式事件并显示。

### T1.1 — 初始化 `desktop/` Go 模块与 Wails 骨架

- **文件**（新增，3 个）：
  - `desktop/go.mod`（module + replace 指向根模块 + Wails v2 依赖）
  - `desktop/main.go`（Wails `Run` 入口，`embed` 前端 assets，macOS TitleBar 配置）
  - `go.work`（新增 `./desktop`）
- **内容**：
  - `go mod init github.com/xujian519/mady/desktop`
  - `replace github.com/xujian519/mady => ../`
  - `main.go` 用 [03-design.md §2.3](./03-design.md#23-wails-应用骨架) 的骨架（`//go:embed all:frontend/dist` + `wails.Run`）
  - macOS 优先（`//go:build darwin`），Win/Linux main 文件阶段 4 填充占位
  - Wails 锁定 **v2.12.x**；首次构建验证 Go 1.26 兼容性
- **验收**：`cd desktop && go build ./...` 通过；`go.work sync` 无报错
- **风险**：低 | **审查**：L2（新增模块）
- **依赖**：Q6 已决策——锁定 Wails v2.12.x；构建失败时临时降 `go.mod` 到 1.24 验证

### T1.2 — 实现 `App` 结构体与生命周期

- **文件**（新增，2 个）：
  - `desktop/app.go`（App struct + New + startup + shutdown）
  - `desktop/config.go`（配置加载，复用 `pkg/agentconfig`）
- **内容**：
  - `App` 持有 `*server.Server` + `context.Context` + run 取消映射表
  - `New(cfg agentcore.Config)`：调用 `server.New(cfg)` 装配（配置由 `pkg/framework.Setup` 提供，见 T1.3）
  - `startup(ctx)`：保存 ctx；**不**在此订阅 chat 事件（chat 事件来自 agent 私有总线，由 `Server.Chat` 的回调透出）
  - `shutdown(ctx)`：优雅关闭 server，cancel 所有未完成的 run，flush 未完成流
- **验收**：单元测试覆盖 startup/shutdown；`go vet` 通过
- **风险**：低 | **审查**：L2
- **关键约束**：**不修改** `cmd/mady/framework.go`，只消费 `pkg/framework` 导出函数；chat 事件源不是 `server.OnAll`

### T1.3 — 提取共享装配逻辑到 `pkg/framework`

- **文件**（修改 + 新增，≤5 个）：
  - 新增 `pkg/framework/setup.go`（导出 `Setup(ctx, Options)`，返回 `BaseConfig`/`MadyHome`/可选 `Deferred`）
  - 修改 `cmd/mady/framework.go`（将 `setupFrameworkContext` 主体迁移到 `pkg/framework`，保留兼容 shim）
  - 修改 `cmd/mady/server.go`、`cmd/mady/tui.go`（改为调用 `pkg/framework.Setup`）
  - `desktop/app.go`（消费 `pkg/framework.Setup`）
- **内容**：
  - 去掉 `os.Exit`，错误通过 `error` 返回
  - 去掉 CLI 专属行为（stderr 输出可保留为 slog，但不泄漏到 WebView）
  - 支持 `Options.Mode = Sync | Deferred`，桌面端首期用 `Sync`
- **验收**：`mady serve` / `mady tui` 行为不变（回归测试）；desktop 复用同一装配路径；`go vet ./pkg/framework` 通过
- **风险**：中（触及入口装配）| **审查**：L3
- **决策点**：必须执行；当前 `setupFrameworkContext` 位于 `package main` 且未导出，无法被 `desktop/` import

### T1.3b — `server` 包新增桌面端公开 API

- **文件**（新增 + 修改，2-3 个）：
  - 新增 `server/desktop.go`（`Server.Chat` / `Cancel` / `SendAction` / `ListThreads` / `GetThread` / `DeleteThread` / `Health`）
  - 修改 `server/chat.go`（可选：将 `handleStreamChat` 的核心逻辑抽取为内部可复用函数，供 `Server.Chat` 和 HTTP handler 共用）
- **内容**：
  - `Server.Chat(ctx, req, onEvent)`：非 HTTP chat 入口，内部注册 `agent.OnAll` 回调并通过 `onEvent` 实时返回事件
  - `Server.Cancel(runId)` / `SendAction(...)` / 线程管理 / Health
  - 所有方法不破坏现有 HTTP API，仅作为新增导出方法
- **验收**：
  - `go test ./server` 通过；HTTP chat 行为不变
  - mock provider → `Server.Chat` → `onEvent` 收到 `agentcore.MessageDeltaEvent`
- **风险**：高（核心入口）| **审查**：L3（触及 server 核心）
- **依赖**：无（可独立开发）

### T1.4 — 初始化 React + Vite + Tailwind v4 前端骨架

- **文件**（新增，~8 个，均在前端目录）：
  - `desktop/frontend/package.json`（React 18 + Vite + Tailwind v4 + TS + shadcn + Motion + Zustand + TanStack Query）
  - `desktop/frontend/vite.config.ts`
  - `desktop/frontend/tsconfig.json`
  - `desktop/frontend/tailwind.config.ts`（v4 配置，对齐 §02 主题令牌）
  - `desktop/frontend/index.html`（loading 骨架）
  - `desktop/frontend/src/main.tsx`
  - `desktop/frontend/src/app/App.tsx`
  - `desktop/frontend/src/lib/wails.ts`（Wails runtime 封装）
- **验收**：`cd desktop/frontend && pnpm install && pnpm build` 产出 `dist/`；`go build` 能 embed 该 dist
- **风险**：低 | **审查**：L1
- **依赖**：T1.1（go:embed 路径）

### T1.5 — 实现 Chat 方法与 AGUI → Wails 事件透传

- **文件**（修改 + 新增，4 个）：
  - `desktop/app.go`（新增 `Chat(req)` / `Cancel(runId)` / 事件映射 `mapAguiEventToWailsName` / `emitAguiEvents`）
  - `desktop/types.go`（ChatRequest/ThreadSummary/HealthInfo 等薄封装类型）
  - `desktop/app_test.go`（事件映射单元测试）
  - `desktop/frontend/src/agui-bridge/client.ts`（`EventsOn` 订阅 + 分发到 store）
- **内容**：
  - `Chat` 调用 `server.Server.Chat(ctx, req, onEvent)`，事件回调来自 agent 私有总线
  - `onEvent` 内部使用 `agui.Convert` + `mapAguiEventToWailsName` 循环 `runtime.EventsEmit`
  - `Chat` 结束或出错时显式 emit `agui:done`
  - `Cancel` 调用保存的 context cancel 函数
- **验收**：
  - AC-1（手动启动窗口跑一轮 chat）
  - AC-2（流式 token 显示）
  - 单测验证：mock `agentcore.Event` → `agui.Convert` → 正确 Wails 事件名
- **风险**：高（事件透传正确性，阶段 1 关键路径）| **审查**：L2
- **依赖**：T1.2、T1.3b、T1.4
- **关键约束**：不要依赖 `server.OnAll` 捕获 chat 事件；必须走 `Server.Chat` 的回调

### T1.6 — 会话/线程管理与健康检查方法

- **文件**（修改，2 个）：
  - `desktop/app.go`（新增 `ListThreads` / `GetThread` / `DeleteThread` / `Health` 方法，代理到 `server.Server.*`）
  - `desktop/frontend/src/stores/threads.ts`（Zustand store + TanStack Query 适配）
- **验收**：前端能列出/打开/删除会话；Health 显示 provider/knowledge 状态
- **风险**：低 | **审查**：L1
- **依赖**：T1.3b、T1.4

### T1.7 — Makefile 集成与冒烟测试

- **文件**（修改，2 个）：
  - `Makefile`（新增 `desktop-build` / `desktop-run` / `desktop-dev` / `desktop-test` 目标）
  - `desktop/app_test.go`（集成测试：mock provider → Chat → 收到 agui:done）
- **验收**：`make desktop-run` 启动窗口；`make desktop-test` 通过；冒烟测试 §02 §6.3 全过
- **风险**：低 | **审查**：L2

### T1.8 — 阶段 1 文档与 CHANGELOG

- **文件**（修改，2 个）：
  - `README.md`（新增"桌面端"章节，开发/构建说明）
  - `docs/decisions/AI_CHANGELOG.md`（追加 desktop 模块落地记录）
- **验收**：README 含开发流程；CHANGELOG 记录完整
- **风险**：低 | **审查**：L1

---

## 阶段 2：A2UI 渲染器（核心资产）

**阶段目标**：实现 A2UI v0.9.1 → React 完整渲染器，覆盖 BasicCatalog 全部 18 组件 + 15 函数。

### T2.1 — SurfaceStore + CatalogRegistry（TS 版）

- **文件**（新增，4 个）：
  - `desktop/frontend/src/a2ui-renderer/catalog.ts`（`CatalogRegistry` + `BasicCatalog` 定义，对齐 `a2ui/catalog.go`）
  - `desktop/frontend/src/a2ui-renderer/store.ts`（对齐 Go `a2ui.SurfaceStore`）
  - `desktop/frontend/src/a2ui-renderer/datamodel.ts`（JSON Pointer RFC 6901 get/set/remove）
  - `desktop/frontend/src/a2ui-renderer/store.test.ts`（Vitest 单元测试）
- **验收**：
  - `applyEnvelope(createSurface/updateComponents/updateDataModel/deleteSurface)` 行为与 Go 端一致
  - JSON Pointer 含 `-` append 语义
  - `ClientDataModel()` 聚合所有 `sendDataModel=true` 的 surfaces
- **风险**：中（语义对齐）| **审查**：L2
- **关键**：测试用例从 Go 端 `a2ui/surface_test.go` 移植，保证 1:1

### T2.2 — 动态值解析（Dynamic + FunctionCall）

- **文件**（新增，3 个）：
  - `desktop/frontend/src/a2ui-renderer/dynamic.ts`（Bind/FunctionCall 解析 + memoize；按 Go 端实际 wire 格式 `path`/`call` 解析）
  - `desktop/frontend/src/a2ui-renderer/functions/format.ts`（formatString/Number/Currency/Date/pluralize）
  - `desktop/frontend/src/a2ui-renderer/functions/validate.ts`（required/regex/length/numeric/email + and/or/not）
- **内容**：
  - 解析顺序：`call` → `path` → literal
  - `FunctionCall.Args` 是命名参数对象（`map<string, any>`），非数组
  - `openUrl` 走 Wails 前端打开系统浏览器（拦截非 http(s) 协议）
- **验收**：15 个函数全覆盖；`openUrl('javascript:...')` 被拦截；Dynamic 对照测试覆盖 `path`/`call`/literal
- **风险**：低 | **审查**：L2（openUrl 安全拦截）

### T2.3 — P0 组件渲染（9 个核心组件）

- **文件**（新增，~12 个）：
  - `desktop/frontend/src/a2ui-renderer/registry.tsx`（ComponentType → ReactComponent 注册表）
  - `desktop/frontend/src/a2ui-renderer/theme.ts`（A2UI theme properties → Tailwind class）
  - `desktop/frontend/src/a2ui-renderer/renderer.tsx`（组件分派 + 树渲染）
  - `desktop/frontend/src/a2ui-renderer/components/Text.tsx`
  - `desktop/frontend/src/a2ui-renderer/components/Icon.tsx`（lucide-react 映射）
  - `desktop/frontend/src/a2ui-renderer/components/Row.tsx`
  - `desktop/frontend/src/a2ui-renderer/components/Column.tsx`
  - `desktop/frontend/src/a2ui-renderer/components/List.tsx`（ChildList 模板遍历）
  - `desktop/frontend/src/a2ui-renderer/components/Card.tsx`（shadcn Card）
  - `desktop/frontend/src/a2ui-renderer/components/Divider.tsx`（shadcn Separator）
  - `desktop/frontend/src/a2ui-renderer/components/Button.tsx`（shadcn Button + action 回传）
  - `desktop/frontend/src/a2ui-renderer/components/TextField.tsx`（shadcn Input + 双向绑定）
- **验收**：AC-3（createSurface 渲染组件树）；AC-4（Bind 实时刷新）；组件 snapshot 测试
- **风险**：中 | **审查**：L2

### T2.4 — SendAction 回传闭环

- **文件**（修改 + 新增，3 个）：
  - `desktop/app.go`（`SendAction(surfaceId, action)` 方法 → 转发到 server）
  - `desktop/frontend/src/a2ui-renderer/components/Button.tsx`（点击 → `SendAction`）
  - `desktop/frontend/src/a2ui-renderer/store.ts`（`clientDataModel` 收集）
- **验收**：按钮点击触发后端 action；sendDataModel 启用时附 data model
- **风险**：中 | **审查**：L2

### T2.5 — P1 组件渲染（5 个）

- **文件**（新增，5 个）：
  - `components/Image.tsx` / `components/Tabs.tsx`（shadcn Tabs）/ `components/Modal.tsx`（shadcn Dialog）/ `components/CheckBox.tsx`（shadcn Checkbox）/ `components/ChoicePicker.tsx`（shadcn RadioGroup/Select）
- **验收**：snapshot 测试；交互测试
- **风险**：低 | **审查**：L1

### T2.6 — P2 组件渲染（4 个）

- **文件**（新增，4 个）：
  - `components/Video.tsx` / `components/AudioPlayer.tsx` / `components/DateTimeInput.tsx`（shadcn Calendar+Popover）/ `components/Slider.tsx`（shadcn Slider）
- **验收**：snapshot 测试
- **风险**：低 | **审查**：L1

### T2.7 — 开发期结构校验

- **文件**（新增，1 个）：
  - `desktop/frontend/src/a2ui-renderer/validate.ts`（对齐 `a2ui.ValidateEnvelope` + `ValidateSurfaceTree`）
- **验收**：dangling ref / cycle / 缺 root 检测；失败 console.error 不阻塞
- **风险**：低 | **审查**：L1

### T2.8 — A2UI 渲染器端到端测试

- **文件**（新增，2 个）：
  - `desktop/frontend/e2e/a2ui.spec.ts`（Playwright：注入 envelope → 截图对比）
  - `desktop/a2ui_e2e_test.go`（Go 端：Agent 发 surface → 前端渲染）
- **验收**：AC-3、AC-4、AC-5 全过
- **风险**：中 | **审查**：L2

---

## 阶段 3：组件迁移 + macOS 打包

**阶段目标**：迁移 TUI 关键业务组件，产出可分发 macOS `.app` + `.dmg`。

### T3.1 — ChatView 主视图与消息流

- **文件**（新增，~6 个）：
  - `desktop/frontend/src/components/ChatView.tsx`（消息列表 + 输入框 + 流式）
  - `desktop/frontend/src/components/MessageBubble.tsx`（含 Motion token 淡入）
  - `desktop/frontend/src/components/Sidebar.tsx`（会话列表 + 搜索）
  - `desktop/frontend/src/components/StatusBar.tsx`（底部状态栏，对应原型 `.statusbar`）
  - `desktop/frontend/src/components/DocumentViewer.tsx`（分屏文档预览，对应原型 `.doc-viewer`；HTML 内嵌渲染，PDF 调用系统默认应用 / QuickLook）
  - `desktop/frontend/src/stores/chat.ts`（聊天状态机）
- **验收**：
  - 聊天流式体验对齐 Claude/ChatGPT；空状态文案符合 tone-style-guide
  - 复现 Open Design 原型中的三栏布局、分屏文档、状态栏
- **风险**：低 | **审查**：L2（文案规范）

### T3.2 — ToolCard 工具调用卡片

- **文件**（新增，2 个）：
  - `desktop/frontend/src/components/ToolCard.tsx`（展开/收起 + 状态）
  - `desktop/frontend/src/components/ToolCard.test.tsx`
- **内容**：**过滤 `transfer_to_*` 类工具调用**（Invisible Handoff 红线）
- **验收**：handoff 工具调用不显示；常规工具调用展示参数/结果
- **风险**：中（契约）| **审查**：L3（涉 handoff 安全契约）

### T3.2b — ProjectTree 可读写项目树

- **文件**（新增 + 修改，4 个）：
  - `desktop/frontend/src/components/ProjectTree.tsx`（文件树 + 新建/重命名交互）
  - `desktop/frontend/src/components/ProjectTree.test.tsx`
  - `desktop/app.go`（新增 `CreateFolder` / `RenameFolder` 方法，代理到文件系统 + 沙箱校验）
  - `desktop/types.go`（文件操作响应类型）
- **内容**：
  - 文件树渲染：递归读取 CWD/案件目录结构（复用 `fileindex.Extension`）
  - 右键菜单：「新建文件夹」「重命名」
  - 新建文件夹：在选中节点下创建子目录（受 `tools/path.go` 沙箱约束）
  - 重命名：行内编辑模式，回车确认
  - 阶段 3 暂不支持删除文件夹
  - 文件操作错误（权限/沙箱越狱）→ Toast 提示
- **验收**：可在侧栏新建/重命名文件夹；越狱操作被沙箱拦截并提示；`go vet` 通过
- **风险**：中（文件系统操作边界）| **审查**：L2（涉及沙箱边界需人审）

### T3.3 — ApprovalCard 审批卡片

- **文件**（新增，2 个）：
  - `desktop/frontend/src/components/ApprovalCard.tsx`（批准/拒绝按钮 → SendAction）
  - `desktop/frontend/src/components/ApprovalCard.test.tsx`
- **验收**：AC-5（审批渲染 + 回传）；与 TUI `approval_card.go` 行为一致
- **风险**：中 | **审查**：L2

### T3.4 — ConclusionCard + ConfidenceBar

- **文件**（新增，3 个）：
  - `desktop/frontend/src/components/ConclusionCard.tsx`
  - `desktop/frontend/src/components/ConfidenceBar.tsx`
  - `desktop/frontend/src/components/MarkdownRenderer.tsx`（goldmark 兼容渲染）
- **验收**：结论卡片含置信度标注（tone-style-guide 合规）
- **风险**：低 | **审查**：L2（文案规范）

### T3.5 — 主题层与深浅色模式

- **文件**（新增 + 修改，3 个）：
  - `desktop/frontend/src/theme/tokens.ts`（design tokens，对齐 §02 §5.1）
  - `desktop/frontend/src/theme/provider.tsx`（ThemeProvider + 系统跟随）
  - `desktop/frontend/tailwind.config.ts`（接入 tokens）
- **验收**：深浅色切换；令牌与 Apple HIG 语义对齐
- **风险**：低 | **审查**：L1
- **依赖**：Q1 已决策——主 accent 为 `systemIndigo`，橙色为品牌点缀色

### T3.6 — 设置面板与持久化

- **文件**（新增，2 个）：
  - `desktop/frontend/src/components/SettingsPanel.tsx`（主题/Provider 切换）
  - `desktop/frontend/src/stores/settings.ts`（持久化到 `~/.mady/desktop-settings.json`）
- **内容**：
  - Provider/Model 切换：写入全局配置（复用 `pkg/agentconfig`），**仅新会话生效**
  - 切换时弹 Toast：「模型切换将在下一轮对话中生效」
  - 已有会话保持原有模型，不改造 session 存储
- **验收**：设置项持久化；切换 Provider 后新会话使用新模型，已有会话不变
- **风险**：低 | **审查**：L1

### T3.7 — macOS 打包（`.app` + `.dmg`）

- **文件**（修改 + 新增，3 个）：
  - `Makefile`（新增 `desktop-dmg` 目标，调 `wails build -platform darwin/universal`）
  - `desktop/build/Info.plist`（Bundle ID / 应用名 / 权限声明）
  - `desktop/build/appicon.png`（Q3 已决策：复用 YunPat-Ai AppIcon，1024×1024）
  - `desktop/build/AppIcon.appiconset/`（完整 iconset，macOS 标准尺寸）
- **验收**：AC-6（`make desktop-dmg` 产出可拖拽安装的 dmg）
- **风险**：中（签名/公证）| **审查**：L2
- **决策**：阶段 3 用 ad-hoc 签名（`codesign -s -`），公证留阶段 5

### T3.8 — 阶段 3 集成测试 + 文档

- **文件**（修改，2 个）：
  - `desktop/e2e_test.go`（端到端：启动 → chat → A2UI surface → 审批 → 关闭）
  - `README.md`（macOS 下载/安装说明）
- **验收**：AC-1~AC-7 全过；`make desktop-test` + `make desktop-dmg` 全绿
- **风险**：低 | **审查**：L2

### T3.9 — 知识库管理页面（`knowledge.html`）

- **文件**（新增，2 个）：
  - `desktop/frontend/src/components/KnowledgeView.tsx`
  - `desktop/frontend/src/stores/knowledge.ts`
- **内容**：复用 `settings.html` 中的知识库状态 + 索引进度，扩展为独立管理页：
  - 知识库概览卡片（文档数、索引大小、最后更新时间）
  - 来源管理（添加/删除本地文件夹、URL）
  - 索引范围选择（专利法 / 审查指南 / 判例 / 自定义）
  - 重新索引按钮 + 进度条
- **验收**：页面可查看知识库状态并触发重新索引；样式与设置页一致
- **风险**：低 | **审查**：L1
- **优先级**：P1（阶段 3 扩展，非 MVP 阻塞）

### T3.10 — 专利模板库页面（`templates.html`）

- **文件**（新增，2 个）：
  - `desktop/frontend/src/components/TemplatesView.tsx`
  - `desktop/frontend/src/stores/templates.ts`
- **内容**：
  - 模板分类标签（权利要求书 / 说明书 / 摘要 / OA 答复函 / PCT 申请）
  - 模板卡片网格（预览 + 标题 + 描述 + 使用按钮）
  - 模板详情侧板（完整预览 + 一键填充到聊天）
  - 自定义模板上传/编辑（阶段 3 可选）
- **验收**：可浏览模板并点击“使用”带入 `Composer`
- **风险**：低 | **审查**：L1
- **优先级**：P1（阶段 3 扩展，非 MVP 阻塞）

---

## 阶段 4：Windows / Linux 打包（预留，本期不开工）

> 代码路径在 T1.1 已用 build tag 预留。本阶段任务待 macOS 验证稳定后细化。

预计任务（粗略）：
- T4.1 Windows main 与打包（`.msi`，需 Windows CI runner）
- T4.2 Linux main 与打包（`.AppImage` + `.deb`）
- T4.3 跨平台回归测试矩阵

---

## 任务依赖图

```
T1.1 ─┬─▶ T1.4 ─┐
      │          │
T1.2 ─┤          ▼
      │     T1.5 ─▶ T1.6 ─▶ T1.7 ─▶ T1.8
T1.3 ─┘          ▲
                 │
T1.3b ───────────┘（T1.3b 可独立开发，T1.5 必须依赖它）

阶段 1 完成
    │
    ▼
T2.1 ─▶ T2.2 ─▶ T2.3 ─▶ T2.4
                       │
T2.5 / T2.6 / T2.7（可并行）
                       │
                       ▼
                     T2.8

阶段 2 完成
    │
    ▼
T3.1 ─▶ T3.2 ─▶ T3.2b ─▶ T3.3 ─▶ T3.4
          （T3.2b 可独立于 T3.3-T3.4 开发）
T3.5（可与 T3.1-T3.4 并行）
                │
                ▼
T3.6 ─▶ T3.7 ─▶ T3.8

阶段 3 完成 → 可发布 macOS MVP
```

**关键路径说明**：
- `T1.3`（提取 `pkg/framework`）和 `T1.3b`（server 新增公开 API）是阶段 1 的两大前置；两者可并行。
- `T1.5` 必须在 `T1.3b` 完成后才能验证事件透传，是阶段 1 的风险最高点。
- `T1.3b` 不应拖到 `T1.5` 才做，否则会发现 chat 事件无法被 `server.OnAll` 捕获。

---

## 验收检查清单（阶段合并）

| AC | 验收任务 | 状态 |
|----|----------|------|
| AC-1 | T1.5 / T1.7 | ☐ |
| AC-2 | T1.5 / T2.3 | ☐ |
| AC-3 | T2.3 / T2.8 | ☐ |
| AC-4 | T2.3 / T2.8 | ☐ |
| AC-5 | T2.4 / T3.3 | ☐ |
| AC-6 | T3.7 | ☐ |
| AC-7 | T2.8 / T3.8 | ☐ |
| AC-8 | T3.1 / T3.2b / T3.9 / T3.10 | ☐ |

---

## 风险登记册（实现期滚动更新）

| 风险 | 等级 | 触发条件 | 缓解 | 负责任务 |
|------|------|----------|------|----------|
| `server` 无桌面端 chat 入口 | **高** | T1.5 无法收到消息增量 | 新增 `Server.Chat(ctx, req, onEvent)`（T1.3b） | T1.3b |
| `cmd/mady/framework.go` 不可 import | **高** | T1.2/T1.3 无法复用装配逻辑 | 提取到 `pkg/framework.Setup`（T1.3） | T1.3 |
| A2UI `Dynamic` wire 格式漂移 | **高** | T2.2 解析失败 | 修正 spec 并按 Go 端 `path`/`call`/literal 实现 | T2.2 |
| AGUI → Wails 事件名映射遗漏 | 中 | T1.5 前端收不到事件 | 集中维护映射表 + 单测 | T1.5 |
| Wails v2 / Vite 7+ 兼容性 | 中 | T1.4 dev server 连不上 | 锁定 Vite 5.4.x + Wails v2.12.x | T1.1/T1.4 |
| Wails v2 与 Go 1.26 不兼容 | 中 | T1.1 构建失败 | 临时降 go.mod 到 1.24；跟踪 Wails 上游 | T1.1 |
| A2UI 渲染器工作量超估 | 中 | T2.3 进度滞后 | 降级到 P0 only，P1/P2 后置 | T2.3-T2.6 |
| WKWebView SSE 缓冲 | 低 | T1.5 流式卡顿 | 已用 Wails Events 规避 | T1.5 |
| macOS 签名公证阻塞发布 | 中 | T3.7 | ad-hoc 签名先行；公证阶段 5 | T3.7 |
| shadcn 组件不满足 A2UI 语义 | 低 | T2.3 | 自建对应组件 | T2.3 |
| 品牌色/图标未决 | 低 | T3.5 / T3.7 | 已决策复用 YunPat-Ai 图标 | T3.5/T3.7 |

---

## 下一步

1. Owner 确认 [01-proposal.md](./01-proposal.md) 中的 Human Owner 与 §5 决策摘要，并澄清 [02-spec.md §8](./02-spec.md#8-已澄清问题汇总) 中仍待确认的项
2. 人工 Sign-off 四份文档
3. 进入 T1.1 实现
