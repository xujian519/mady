# BCIP 桌面端 Codex 对齐 — 详细落地计划

> [!WARNING]
> **DEPRECATED（归档）**：本文档为 BCIP 时代（2026-05-30）历史设计，
> 基于已废弃的 **Tauri 方案**。当前桌面端实现以
> `docs/specs/desktop/` 根目录下的 01-proposal/02-spec/03-design/04-tasks/05-pilotdeck-alignment
> 为准（Wails v2 + React）。仅保留作为设计历史参考。


**日期**: 2026-05-30
**状态**: 工程执行计划
**代码根目录**: `apps/desktop/`
**UI 参考实现**: `/Users/xujian/Downloads/Kimi_Agent_直接复刻Codex桌面/app`（迁入目标，非独立产品）

---

## 0. 文档关系

| 文档 | 用途 |
|------|------|
| [desktop-redesign.md](2026-05-30-desktop-redesign.md) | 壳层 IA 摘要 |
| [codex-parity-strategy.md](2026-05-30-desktop-codex-parity-strategy.md) | **架构决策**（必读） |
| [desktop-design-spec.md](2026-05-30-desktop-design-spec.md) | 设计交付 / Figma / 走查 C01–C12 |
| [codex-desktop-pixel-perfect-design-spec.md](../codex-desktop-pixel-perfect-design-spec.md) | 像素 Token、Windows 变体 |
| [IMPLEMENTATION_PHASES.md](IMPLEMENTATION_PHASES.md) | 历史分阶段（Tauri/预览）；**勿按 WS 主路径执行** |
| **本文** | 排期、文件映射、验收、Issue 拆分 |

---

## 1. 现状差距（2026-05-30 基线）

### 1.1 已有

- Tauri v2 + React 19 + Tailwind + shadcn（`apps/desktop`）
- 语义色 Token 与 [`desktop-design-spec.md`](2026-05-30-desktop-design-spec.md) 对齐（`src/index.css`）
- 文件树、多格式预览（PDF/MD/DOCX/图片）、Todo、阶段指示器骨架
- ~~残缺 WS 客户端 `bcipClient.ts`~~（已删除，统一 `appServerClient.ts`）

### 1.2 缺口

| 领域 | 缺口 |
|------|------|
| 传输 | WS 实验路径；`AppServerManager` 与 `commands/system.rs` 双轨启进程 |
| 协议 | 无完整 `initialized`；`turn/start` 缺 `threadId`/`input`；无 `thread/start` |
| Agent UI | ~~`RightPanel`~~ → 已用模块化 `components/agent/*` |
| 设置 | 无 MCP、技能、审批沙箱、插件页 |
| 浮层 | 无 `ApprovalDialog` / MCP elicitation / OAuth 等待 / CommandPalette |
| 专利壳层 | 未连接/mock 用演示视图；已连接用 `AgentWorkPane` 绑消息流 |
| 类型 | 未接入 `bcip app-server generate-ts` |

### 1.3 参考原型可迁移资产

参考目录 `Kimi_Agent_直接复刻Codex桌面/app/src/components/`：

- `AppShell`, `CenterWorkspace`, `AgentPanel`, `agent/*`, `settings/*`, `overlays/*`
- `hooks/useAppStore.tsx`（**仅开发期布局**；生产须接 RPC）

---

## 2. 目标架构

```
Tauri（FS、对话框、可选 PTY、app-server proxy 单路径）
  └─ React
       ├─ Layer B: Codex parity（Agent + Settings + Boot + Overlays）→ JSON-RPC
       └─ Layer C: 专利壳（树、预览、阶段、Todo）→ FS + cwd 绑定 thread
            └─ bcip app-server（与 CLI/TUI 共用 CODEX_HOME / config.toml）
```

**硬约束**

1. 不维护第二套 MCP/技能配置。
2. 中心区域不得出现第二套聊天气泡。
3. RPC 类型来自 `bcip app-server generate-ts`。
4. 生产默认禁止 `ws://127.0.0.1:9002` 直连。

---

## 3. 目标目录结构（`apps/desktop/src`）

```
components/
  shell/       DesktopShell, TitleBar, StatusBar, ResizeHandle
  sidebar/     LeftSidebar, FileTree
  center/      CenterPanel, AgentWorkPane, 专利 mock 视图（门控）
  agent/       AgentPanel, MessageTimeline, Composer, …
  settings/    SettingsLayout + codex pages
  overlays/    ApprovalDialog, McpElicitationModal, …
store/         buildInitialState, appReducer, AppStoreContext
hooks/
  useAppStore.tsx         # re-export
  useAppServerSession.ts
lib/
  appServerClient.ts, devMock.ts
generated/
  app-server/             # generate-ts 输出
pages/
  MainApp.tsx             # DesktopShell + boot + overlays
```

**根目录 npm 转发**（`BCIP/package.json`）：`desktop:ci`、`desktop:smoke`、`tauri:dev`、`prepare-sidecar`。

---

## 4. 分阶段任务

### 阶段 0 — 对齐（0.5–1 人天）

- [ ] 团队确认 P0/P1/P2 边界（策略文档 §5）
- [ ] 参考原型本地 `npm run dev` + 走查表 C01–C12 截图存档
- [ ] 看板创建里程碑 M1–M5（§8）
- [ ] `INTEGRATION.md` 已加传输层废弃说明（见文首）

**退出**: 无人按 `IMPLEMENTATION_PHASES` 阶段 5 单独推进 WS。

---

### 阶段 1 — P0 传输与 app-server 单例（3–5 人天）

| ID | 任务 | 文件/模块 | 状态 |
|----|------|-----------|------|
| 1.1 | 合并 attach-or-spawn，删除重复子进程管理 | `app_server_manager.rs`, `commands/system.rs`, `lib.rs` | ✅ stdio 单例；已删 WS:9002 自启 |
| 1.2 | unix socket / `app-server proxy`（附着已有 daemon） | `app_server_manager.rs` | ✅ socket 存在时 `proxy`，否则 stdio |
| 1.3 | 前端经 Tauri invoke 收发 JSON-RPC | `lib/appServerClient.ts`, `commands/app_server.rs` | ✅ |
| 1.4 | `initialize` → `initialized` | `appServerClient.handshake()` | ✅ |
| 1.5 | `thread/start`, `turn/start` 等 | `useAppServerSession.ts` | ✅ 首版（Codex 预览 AgentPanel） |
| 1.6 | 通知：`item/*`, `turn/*` | `threadItemMapper.ts` + 通知分发 | ✅ delta/started/completed/turn |
| 1.7 | `bcip app-server generate-ts` | `src/generated/app-server/` | ✅ 已生成 v2 类型 |
| 1.8 | `CODEX_HOME` 路径与 core 一致 | `src-tauri/src/config.rs` | ✅ spawn 注入 `CODEX_HOME` |

**验收**

- TUI 已配 MCP/技能，桌面不改文件即可对话且 `mcpServerStatus/list` 一致。
- 无「请手动运行 app-server --port」类文案。
- Smoke：握手 + `thread/start` + `turn/start` 一条链路。✅ `scripts/smoke-app-server-rpc.py`（默认跳过 `turn/start`，设 `BCIP_SMOKE_SKIP_TURN=0` 可测完整回合）

---

### 阶段 2 — P0 UI 壳迁移（4–6 人天）

从参考目录迁入组件；可与 1.7 并行（mock 会话）。

| ID | 迁入/改写 | 替换 |
|----|-----------|------|
| 2.1 | `AppShell`, `ResizeHandle` | `Layout`, `ResizablePanel` 部分 |
| 2.2 | `TitleBar`, `StatusBar` | 现有同名组件 |
| 2.3 | `LeftSidebar` + 现有 `FileTree` | `sidebar/LeftSidebar` |
| 2.4 | `CenterPanel` + `AgentWorkPane` | 已连接显示 Agent 输出；mock 视图仅未连接 / `VITE_DEV_MOCK=1` |
| 2.5 | `agent/*`, `AgentPanel` | ✅ 已删除 `RightPanel.tsx`、`bcipClient.ts` |
| 2.6 | `GlobalOverlays` + 审批 RPC | ✅ `ApprovalDialog` 接 `item/*/requestApproval` |
| 2.6 | `overlays/*` | 新建 |
| 2.7 | 设置 overlay（参考 `App.tsx`） | 可选保留 `/settings` 深链 |
| 2.8 | 侧栏联动宽 260/48/400，中心 min 400px | 设计 spec §8.1 |

**验收**: `tauri dev` 三栏与参考原型视觉一致；C01–C03、C09–C10 静态 UI 通过设计走查。

---

### 阶段 3 — P0 Agent 数据绑定（5–7 人天）

**依赖阶段 1**

| ID | 任务 | RPC / 通知 |
|----|------|------------|
| 3.1 | `MessageTimeline` 按 `item` 类型渲染 | `item/*` | ✅ plan/MCP/shell/patch 卡片 |
| 3.2 | 真流式 delta，移除假打字动画 | `item/agentMessage/delta` | ✅ |
| 3.3 | `ThreadListDrawer` | `thread/list`, `thread/resume` | ✅ |
| 3.4 | `Composer` + slash palette（能力由服务端决定） | `turn/start` | ✅ 专利 slash → 阶段+提示词 |
| 3.5 | `ApprovalDialog` | `commandExecution/requestApproval`, `fileChange/requestApproval` |
| 3.6 | `McpElicitationModal`, `OAuthWaitingSheet` | elicitation / oauth 通知 | ✅ |
| 3.7 | `AgentFooter` 重连、过载文案 | 策略 §5.1 | ✅ error 通知 + 重试 |
| 3.8 | `thread/start` 的 `cwd` = 当前项目根 | 与 `.bcip` / `useProjects` | ✅ |

**验收**: 真实对话 + 至少一次命令审批；C04–C05、C11–C12。

---

### 阶段 4 — P1 设置 = config（4–5 人天）✅

**依赖阶段 1**（2026-05-30 桌面端已全部接线）

| 页面 | RPC |
|------|-----|
| GeneralSettings | `config/read`, `config/value/write` | ✅ 首版 |
| ModelSettings | `model/list` + config（禁止硬编码模型表） | ✅ |
| ApprovalSandboxSettings | permissions / sandbox config | ✅ approval_policy / sandbox_mode |
| McpServersSettings | `mcpServerStatus/list`, `config/mcpServer/reload`, `mcpServer/oauth/login` | ✅ 首版 |
| SkillsSettings | `skills/list`, `skills/config/write`, `skills/changed` | ✅ |
| PluginsSettings | experimental `plugin/*` | ✅ list/install/uninstall |
| AppearanceSettings | `desktop.*` + store 主题 | ✅ |
| ShortcutsSettings | `desktop.shortcuts` | ✅ |
| AboutDiagnostics | CODEX_HOME、bcip、app-server 诊断 | ✅ |

**验收**: 设置修改 MCP → reload → TUI 下一 turn 可见新工具。

---

### 阶段 5 — P1 无感接入（3–4 人天）✅

| UI 状态 | 逻辑 |
|---------|------|
| Boot/Splash | 检测 `bcip` | ✅ `useDesktopBoot` + `DesktopBootOverlay` |
| Boot/NoCli | sidecar / 仅文件模式 | ✅ 无 bcip 时可进文件模式 |
| Boot/Connecting | attach proxy（禁止暴露端口） | ✅ |
| Boot/Fault | 日志 + 重试 | ✅ |
| 成功 | Toast「已与终端配置同步」 | ✅ sonner |

| ID | 工程 |
|----|------|
| 5.1 | sidecar + 与外置二进制版本择优 | ✅ `bcip_binary.rs` |
| 5.2 | attach 优先于 spawn | ✅ |
| 5.3 | 首启 60s 内 thread + 首条消息 | ✅ `warmupAppServerSession`（Boot + cwd 变更） |

---

### 阶段 6 — P2 专利壳层（5–8 人天）✅（2026-05-30 收尾）

**依赖阶段 3**

| ID | 任务 | 走查 |
|----|------|------|
| 6.1 | `StageIndicator` 与 agent/用户操作同步 | P01 | ✅ 中心/标题栏共用 `state.stages`；已连接时 `AgentWorkPane` 展示 Agent 输出 |
| 6.2 | `TodoDock` 对接 agent 或结构化 item | P02 | ✅ 中心待办接 store + 折叠 |
| 6.3 | `FilePreviewRouter` 统一路由 | spec §8.2 | ✅ |
| 6.4 | 项目树 ↔ `thread/start.cwd` | P04 | ✅ cwd + `SET_CURRENT_FILE` |
| 6.5 | PDF 标注 toolbar | P03 | ✅ `PdfAnnotationToolbar` + `.bcip/annotations` |
| 6.6 | 终端 overlay 默认收起 | 非主对话路径 | ✅ 默认不展开 |

---

### 阶段 7 — P2 像素与 Windows（3–5 人天）✅（工程项完成，7.6 设计走查仍可选）

| ID | 任务 | 状态 |
|----|------|------|
| 7.1 | 全局快捷键 §8.2（⌘, / ⌘N / ⌘⇧P / ⌘B / ⌘J / Esc） | ✅ `useGlobalKeyboardShortcuts` |
| 7.2 | 命令面板 / 新建线程 / 聚焦输入联动 | ✅ `desktopEvents` + CommandPalette |
| 7.3 | 中心 min 400px 自动收起右栏 | ✅ `useResponsiveShellLayout` |
| 7.4 | 断点 900/1200（窄屏 Agent 浮层、收起线程列表） | ✅ |
| 7.5 | Windows TitleBar / Segoe | ✅ `platform-windows` + TitleBar |
| 7.6 | Page 10 走查 C01–C12 ≥ 90% | ✅ 工程对齐 + Playwright E2E（`e2e/`）；设计并排截图仍可选 |
| 7.7 | 无障碍 §29 全量 | ✅ Composer/命令面板 aria；全量走查仍可选 |
| 7.8 | StatusBar 用量 RPC | ✅ `account/rateLimits` + `thread/tokenUsage` |

---

## 5. 文件迁移对照表

| 参考（Kimi `app/src`） | 目标（`apps/desktop/src`） |
|------------------------|----------------------------|
| `components/AppShell.tsx` | `components/shell/AppShell.tsx` |
| `components/agent/*` | `components/agent/*` |
| `components/settings/*` | `components/settings/*` |
| `components/overlays/*` | `components/overlays/*` |
| `components/CenterWorkspace.tsx` | `components/center/CenterWorkspace.tsx` |
| `hooks/useAppStore.tsx` | 拆为 `useLayoutStore` + `useAppServerSession` |
| `data/mock*.ts` | 仅 `DEV_MOCK=1` 或 Storybook |
| — | ~~删除 `bcipClient` / `RightPanel`~~ | ✅ 已完成 |

---

## 6. 推荐排期

| 顺序 | 阶段 | 人天 | 并行 |
|------|------|------|------|
| 1 | 0 对齐 | 0.5–1 | — |
| 2 | 1 传输 | 3–5 | — |
| 3 | 2 UI 壳 | 4–6 | 1 后半（mock） |
| 4 | 3 Agent | 5–7 | 依赖 1 |
| 5 | 4 设置 | 4–5 | 与 5 并行 |
| 6 | 5 无感 | 3–4 | 与 4 并行 |
| 7 | 6 专利 | 5–8 | 依赖 3 |
| 8 | 7 走查 | 3–5 | 依赖 2–6 |

**合计约 28–41 人天**（1 全栈 + 0.5 设计走查）。

---

## 7. 里程碑

| 里程碑 | 标准 |
|--------|------|
| **M1 可连接** | 单例 app-server；完整握手；无 WS 生产默认 |
| **M2 可对话** | thread/turn + 流式 item；审批可用 |
| **M3 可配置** | MCP/技能/模型 RPC；与 TUI 一致 |
| **M4 可交付** | 无感首启；专利区演示横幅 / 已连接占位视图；`npm run smoke`（含 RPC）；sidecar 见 `src-tauri/binaries/` |
| **M5 可发行** | macOS 包；Windows P2 |

---

## 8. GitHub Issue 拆分模板

复制到 Issue 时替换 `阶段 X` / `ID x.y`：

```markdown
## 目标
（从本文 §4 对应行粘贴）

## 依赖
- 阻塞: #___
- 文档: docs/plans/2026-05-30-desktop-implementation-plan.md §4 阶段 N

## 任务
- [ ] …

## 验收
- [ ] …

## 非目标
- 不修改 codex-rs 协议（除非单独开 Issue）
```

建议 Epic 标签：`desktop-p0-transport` | `desktop-p0-ui` | `desktop-p1-settings` | `desktop-p2-patent`

---

## 9. 风险

| 风险 | 缓解 |
|------|------|
| mock 误进生产 | PR：禁止未 gate 的 `mockData`；E2E 打真实 server |
| RightPanel 大文件冲突 | feature flag 一周；先增 `agent/` 再删 |
| generate-ts 漂移 | ✅ 主 `ci.yml` 路径门控 desktop 任务 + `npm run check:generate-ts` |
| 文档双轨 | 仅认策略 + 本文 + design-spec |

---

## 10. 本周行动项

1. [ ] Epic + M1 Issue（阶段 1.1–1.4）
2. [x] 从参考目录复制 `agent/`、`overlays/`、`shell/`、`settings/codex/` 到 `apps/desktop`（见 §11）
3. [x] 运行 `bcip app-server generate-ts`；主 `ci.yml` desktop 任务 + `npm run check:generate-ts` 防漂移
4. [ ] 设计走查：参考原型 vs `tauri dev` 当前 UI 截图 diff

---

## 11. 已迁入代码（2026-05-30）

| 路径 | 说明 |
|------|------|
| `apps/desktop/src/components/shell/` | DesktopShell、TitleBar、StatusBar（已删预览用 CenterWorkspace/AppShell） |
| `apps/desktop/src/components/agent/` | Agent 面板与子组件 |
| `apps/desktop/src/components/overlays/` | 审批、MCP、OAuth、命令面板 |
| `apps/desktop/src/components/settings/codex/` | Codex 对标设置页（mock） |
| `apps/desktop/src/store/` + `hooks/useAppStore.tsx` | 全局 store（mock 门控见 `lib/devMock.ts`） |
| `apps/desktop/src/pages/CodexShellPreview.tsx` | 预览入口 |

**本地预览**

```bash
# 仓库根目录
npm run desktop:ci
npm run tauri:dev

# 或仅浏览器 mock（默认 VITE_DEV_MOCK=1）
cd apps/desktop && npm run dev
```

`#/preview/codex-shell` 与 `/` 均挂载 `MainApp`（`DesktopShell`）。生产 Tauri 构建勿设置 `VITE_DEV_MOCK`。

---

**文档结束**
