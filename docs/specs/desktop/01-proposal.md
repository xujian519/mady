# 01 — 提案：Mady 桌面端（Wails + React）

- **功能名**：desktop
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **提案日期**：2026-07-27
- **状态**：待人工 Sign-off

---

## 1. 背景

### 1.1 现状：协议与后端已就位，缺一个渲染出口

Mady 当前的 UI 通道只有 TUI（`tui/`，8 层 Elm 架构终端 UI）。但**面向桌面端的"协议 + 后端"基础设施早已完整落地**，且 AGUI 协议规范明确画出了"前端 UI (Web/桌面)"的占位：

| 能力 | 现状 | 对桌面端的意义 |
|------|------|---------------|
| **A2UI v0.9.1** | `a2ui/` 完整实现，声明式 UI 协议，框架无关 | Agent 直接描述 UI，前端只做渲染器，**业务逻辑零重复** |
| **AGUI SSE** | `agui/` 完整实现，`text/event-stream` 流式事件 | 思考/工具调用/交接/审批等事件已就绪，桌面端直接订阅 |
| **HTTP/SSE Server** | `mady serve`，`server.Handler()` 返回标准 `http.Handler` | 桌面壳可内嵌而非 sidecar，无 IPC 序列化开销 |
| **纯 Go SQLite** | `modernc.org/sqlite`，**零 CGO** | 可打包成单二进制，跨平台编译无痛 |
| **TUI 组件范式** | 35 个组件（editor/approval_card/markdown/conclusion_card/confidence_bar 等） | 交互范式与组件清单可直接迁移到 Web 渲染 |
| **embed 机制** | manifest/模板已用 `go:embed` | 前端静态资源可同样 embed 进二进制 |

**关键缺口**：没有 Web 渲染层（A2UI → React 的渲染器）和桌面壳（窗口/菜单/系统集成）。

### 1.2 为什么现在做

1. **协议资产沉淀已到临界点**——A2UI v0.9.1 + AGUI 已稳定，再不消费就开始折旧
2. **专业领域的可信度需要"看得见的精致"**——专利代理人/律师对工具的第一印象决定采用；TUI 对非技术专业用户门槛过高
3. **Wails v2 已成熟**——Go 原生绑定 + 系统 WebView，契合 Mady"克制、中庸、去繁就简"的哲学，不引入 Rust/Node 编译链
4. **A2UI 渲染器是核心资产**——写一次，未来 PWA/Web/移动端（如选 Tauri Mobile）都能复用
5. **视觉原型已就绪**——Open Design 已产出 4 个高保真 HTML 页面（`index.html` / `chat-workspace.html` / `chat-empty.html` / `settings.html`），为 React 实现提供了可直接参照的布局、令牌和交互范式

### 1.2.1 现有原型资产

> 原型位置：`/Users/xujian/Library/Application Support/Open Design/namespaces/release-stable/data/projects/6d6c72d0-99e3-498a-96d5-18c1e7cfdcdf`

| 页面 | 文件 | 状态 | 作用 |
|------|------|------|------|
| 导航启动页 | `index.html` | ✅ 已完成 | 开发期页面 Hub，演示卡片化导航 |
| 聊天工作台 | `chat-workspace.html` | ✅ 已完成 | 核心三栏布局：项目树、聊天、参考资料 + 分屏文档预览 |
| 新会话空状态 | `chat-empty.html` | ✅ 已完成 | 欢迎页 + 4 个快速入口 + 快捷键提示 |
| 设置 | `settings.html` | ✅ 已完成 | 外观 / Provider / 知识库 / 关于 |
| 知识库管理 | `knowledge.html` | ⬜ 待构建 | 见 [04-tasks.md](./04-tasks.md) 阶段 3 扩展 |
| 专利模板库 | `templates.html` | ⬜ 待构建 | 见 [04-tasks.md](./04-tasks.md) 阶段 3 扩展 |

这些原型不是最终交付物，而是**视觉与交互参考**：
- 布局和组件层级直接迁移到 React（`frontend/src/components/`）
- `:root` 设计令牌与 [02-spec.md §5](./02-spec.md#5-主题与设计令牌) 对齐，并在实现时收敛到 Tailwind v4 `@theme`
- 每个页面内联了 CSS/JS，实现阶段需拆分为组件 + store + Wails bridge

### 1.3 技术选型决策摘要

四个核心决策点已与 Owner 对齐（2026-07-27）：

| # | 决策点 | 选择 | 理由 |
|---|--------|------|------|
| 1 | 后端集成方式 | **Wails 内嵌**（直接 import server/agentcore） | 零 IPC，零 sidecar，单进程单二进制 |
| 2 | UI 渲染策略 | **A2UI → React 声明式渲染器** | 复用协议资产，Agent 改 UI 无需前端改代码 |
| 3 | 前端框架 | **React 18 + shadcn/ui + Tailwind v4 + Motion** | Linear/Vercel/Arc 同款美学，专业精致 |
| 4 | 目标平台 | **macOS 先行**，Windows/Linux 预留 | Wails 跨平台编译，后续 `.msi`/`.AppImage` 可低成本扩展 |

> 候选方案对比（Tauri/Electron/Fyne）详见 [03-design.md](./03-design.md) §1。

---

## 2. 目标

### 2.1 总目标

构建 `mady-desktop` 桌面应用：Wails v2 单进程内嵌现有 Go 后端，前端用 React + shadcn/ui 渲染 A2UI 声明式界面并消费 AGUI SSE 事件流，macOS 优先产出可分发安装包。

### 2.2 阶段目标（本期覆盖阶段 1-3）

| 阶段 | 目标 | 一句话验收 |
|------|------|-----------|
| **阶段 1：骨架打通** | Wails 模块 + React 骨架，内嵌 server，SSE 流到前端可见 | 桌面窗口里能跑通一轮 chat，流式 token 实时显示 |
| **阶段 2：A2UI 渲染器**（核心资产） | 实现 A2UI v0.9.1 → React 的完整渲染器 | Agent 声明的任意 surface 可渲染可交互 |
| **阶段 3：组件迁移 + 打包** | 迁移 TUI 关键组件（editor/approval/markdown/conclusion），产出 macOS `.app` + `.dmg` | 三类专业卡片可用；`make desktop-dmg` 一键产出安装包 |

> 阶段 4（Windows/Linux 打包）与阶段 5（自动更新/签名/公证）**不在本期**，但设计预留接口。

### 2.3 非目标（本期不做）

- 移动端（iOS/Android）——Wails 不支持，未来若需要迁 Tauri Mobile
- 多窗口/标签页——本期单窗口单会话
- 直接交付 Open Design 的静态 HTML 原型——原型仅作视觉/交互参考，生产代码使用 React + shadcn/ui 重新实现
- 离线 LLM 推理——仍走现有 provider（远程 API / 本地 oMLX）
- 内置 PDF 预览器——首期用系统 WebView 原生渲染 HTML，PDF 由系统打开
- 应用商店上架（Mac App Store）——本期仅开发者签名直链分发

---

## 3. 成功标准

### 3.1 功能验收

| 编号 | 标准 | 验证方式 |
|------|------|----------|
| AC-1 | `make desktop-run` 启动桌面窗口，聊天输入→流式输出可见 | 手动 + 冒烟测试 |
| AC-2 | 前端通过 Wails Events 收到 `agui:message-delta`，token 逐字淡入显示 | 手动 + e2e |
| AC-3 | Agent 发出 A2UI `createSurface` envelope，前端渲染出对应组件树 | 集成测试（覆盖 BasicCatalog 全部组件类型） |
| AC-4 | A2UI `updateDataModel` 触发前端响应式更新，`Bind("/user/name")` 实时刷新 | 单元测试 + 手动 |
| AC-5 | 审批卡片（`agui:approval-prompt` 事件）渲染"批准/拒绝"按钮，点击回传 `action` envelope | e2e |
| AC-6 | `make desktop-dmg` 产出 `Mady-x.x.x.dmg`，拖拽安装后可独立运行 | 手动 |
| AC-7 | 桌面端与 `mady serve`（Web 模式）共用同一渲染器，行为一致 | 对比测试 |
| AC-8 | Open Design 4 个原型页面在 React 实现中完整复现（布局/令牌/交互） | 视觉回归 + 手动 |

### 3.2 质量验收

- 新增 `desktop/` 模块 `go build ./...` / `go vet ./...` / `go test ./...` 全绿
- 前端 `pnpm typecheck` / `pnpm lint` / `pnpm test` 全绿
- 不引入硬编码密钥；API Key 走环境变量（沿用现有约定）
- 不破坏根模块 / `tools` / `tui` 三个现有模块的构建
- 新增代码符合分层架构：`desktop/` 仅 import `server`/`agentcore`/`a2ui`/`agui`，不反向依赖

### 3.3 回归红线

- 现有 `server.Handler()`、`a2ui.*`、`agui.*` 公开 API **不做破坏性变更**（只新增）
- TUI 入口（`mady tui`）行为保持不变——桌面端是**新增通道**，不替代 TUI
- 不修改安全敏感路径（`tools/path.go` 沙箱、`agentcore/handoff.go` 白名单等）——桌面端只消费现有接口

---

## 4. 关键约束

1. **单进程单二进制**：Wails 内嵌，不 spawn 子进程；前端静态资源 `go:embed` 进 Go 二进制
2. **零 CGO 新增**：Wails v2 macOS 走系统 WebView（WKWebView），不引入 CGO；保持 `modernc.org/sqlite` 的纯 Go 路线
3. **任意目录可用**：桌面端遵循 `MADY_HOME` 统一路径解析约定（AGENTS.md 资源定位 gotcha），不新增 cwd 相对路径
4. **协议优先**：所有 Agent → UI 通信走 A2UI/AGUI，**禁止前端直接调 agentcore 内部方法**——前端只通过 SSE + REST 消费
5. **主题对齐**：前端主题层与 `tui/theme/` 设计语言对齐（同一色板/语义令牌），保证品牌一致性
6. **macOS 先行**：CI 仅跑 macOS 构建；Windows/Linux 代码路径保留但暂不阻塞合并

---

## 5. 决策摘要（详见 03-design.md）

| 决策点 | 选择 | 备选 | 理由 |
|--------|------|------|------|
| 桌面框架 | **Wails v2.12.x** | Tauri v2 / Electron / Fyne / Wails v3 | Go 原生，零生态扩张；v2.12.x 修复 WebView stability 等已知问题 |
| 后端集成 | **内嵌 import**（非 sidecar） | spawn `mady serve` | 单进程，无 IPC 序列化 |
| 前端框架 | **React 18.3 + TS** | Svelte 5 / Vue 3 / React 19 | shadcn 生态成熟；React 19 待 Wails 模板验证后再升级 |
| 组件库 | **shadcn/ui** | Mantine / Geist | 复制粘贴可定制，与 TUI 主题对齐 |
| 样式 | **Tailwind CSS v4.2.x** | CSS Modules / styled-components | 原子化，与 shadcn 默认搭配 |
| 动画 | **Motion**（framer） | @formkit/auto-animate | 流式 token 淡入、列表过渡 |
| 状态管理 | **Zustand + TanStack Query** | Redux / Jotai | 轻量，契合 SSE 数据流 |
| 构建 | **Vite 5.4.x** | webpack / Vite 6/7 | Wails v2 对 Vite 7+ dev server 曾有兼容问题，5.4.x 最稳 |
| SSE 桥 | **Wails Events**（非 fetch SSE） | EventSource | 绕过 WebView 跨域，更稳 |
| 模块归属 | **新增 `./desktop` 第 4 工作区模块** | 放根目录 | 隔离前端依赖，符合多模块约定 |
| 框架装配 | **提取 `pkg/framework`** | 直接 import `cmd/mady` | `cmd/mady` 是 main 包且未导出，无法被 desktop import |

---

## 6. 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| Wails v2 vs v3 选型（v3 仍在 alpha） | 中 | **选 v2.12.x 稳定版**；v3 API 稳定后再评估迁移 |
| `server` 无桌面端非 HTTP chat 入口 | **高** | 新增 `Server.Chat(ctx, req, onEvent)`；T1.3b 优先完成 |
| `cmd/mady/framework.go` 不可被 desktop import | **高** | 提取到 `pkg/framework.Setup`；T1.3 优先完成 |
| A2UI `Dynamic` wire 格式与 spec 漂移 | **高** | 开工前修正 spec，前端按 Go 端 `path`/`call`/literal 解析 |
| AGUI → Wails 事件名映射遗漏 | 中 | 集中维护映射表并加单测 |
| A2UI 渲染器工作量被低估（v0.9.1 组件较多） | 中 | 阶段 2 拆细：先 BasicCatalog 核心组件，自定义 catalog 后置 |
| Vite 版本与 Wails v2 dev server 兼容性 | 中 | 锁定 Vite 5.4.x；T1.4 验证 HMR |
| WKWebView 与 Chrome 行为差异（CSS/JS 兼容） | 中 | 避免 Bleeding-edge CSS；用 Tailwind 默认值；CI 加 web 模式回归 |
| macOS 签名/公证需 Apple Developer 账号 | 中 | 阶段 3 先出未签名版（ad-hoc）；公证作为阶段 5 |
| SSE 在 WebView 内可能被缓冲 | 低 | 用 Wails Events 透传而非原生 EventSource |
| 前端依赖体积膨胀影响二进制大小 | 低 | `pnpm` 去 dev 依赖；embed 前 `vite build` tree-shake |
| Wails 与 Go 1.26 兼容性 | 中 | Wails v2 支持 Go 1.21+；阶段 1 先验证 |

---

## 7. 下一步

人工 Sign-off 本提案后，进入 [02-spec.md](./02-spec.md)（详细规格）。四个核心决策点（Wails 内嵌 / A2UI 渲染器 / React+shadcn/ui / macOS 先行）及六项执行细节（品牌色 / Bundle ID / 图标 / 自动更新 / 托盘 / Wails 版本）已全部澄清，详见 [02-spec.md §8](./02-spec.md#8-已澄清问题汇总)。
