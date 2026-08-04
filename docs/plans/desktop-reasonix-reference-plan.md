# Mady 桌面端仿照 Reasonix 桌面端调整方案

> 状态：**已实施**（阶段 1-4 主体于 2026-08-02 commit `b6a14df` 落地；2026-08-04 全面审阅对齐，
> 剩余缺口见文末「G. 实施状态核对表」）
> 参考对象：`/Users/xujian/projects/参考架构/DeepSeek-Reasonix-main-v2/desktop/`
> 结论：参考项目有成熟桌面端（Wails v2 + React 19），Mady 桌面端同为 Wails v2 + React 18，
> 架构同构，可按四个方向有选择吸收。**不建议整体照搬** —— Reasonix 是通用 agent IDE，
> Mady 是专利/法律领域工作台（A2UI/AGUI/领域能力 UI 是 Mady 自己的特色，应保留）。

---

## 0. 两项目桌面端架构对照

| 维度 | Reasonix（参考） | Mady（现状） |
|------|------------------|--------------|
| 外壳 | Wails v2，独立嵌套 Go module | Wails v2，独立嵌套 Go module（同构 ✓） |
| 绑定 | 单一 `App` struct（100+ 方法，成对 `无参版/ForTab 版`） | 单一 `App` struct（27 方法） |
| 事件 | `runtime.EventsEmit`（`agent:event`）+ 事件 Sink | `runtime.EventsEmit`（`agui:*`/`mady:init-*`）+ AGUI converter |
| 前端 | React 19 + TS + Vite + pnpm + zustand | React 18 + TS + Vite + pnpm + zustand（同构 ✓） |
| 内核对接 | `internal/boot.Build → control.Controller` 直绑 | `server.Server + bootstrap.Context + agentcore` 直绑 |
| 前端规模 | 50+ 大型组件（Composer 181KB / Settings 294KB / ProjectTree 98KB） | 约 30 个中小组件 |
| 特色 | 多 Tab、远程 SSH 工作台、Bot 网关、心跳 | A2UI 渲染器、AGUI 事件链路、领域能力 UI |

---

## A. UI 布局与视觉风格

### A.1 Reasonix 的布局与组件体系

- **整体骨架**：`AppChrome`（顶部 `TabBar` 多标签）+ 侧边栏（历史/上下文/子代理/记忆/能力等可切换面板）+ 主区 `Transcript` + 底部 `Composer` + `StatusBar` + `⌘K CommandPalette`。
- **多标签 TabBar**：会话级多标签并行，标签间切换/关闭/新建，启动时 `restoreOrBuildTabs()` 恢复标签。
- **核心组件**（带行号引用来自调研）：
  - `Composer.tsx`（181KB）：@提及、斜杠菜单、目标开关、粘贴图片、会话草稿、↑/↓ 历史、自动补全、推理努力切换。
  - `Transcript.tsx`（68KB）：回合分组/折叠、工具卡、审批、选区菜单、turn 级复制。
  - `HistoryPanel.tsx`：会话历史搜索/恢复/回收站。
  - `ContextPanel.tsx` + `ContextWindowRing.tsx`：上下文用量环形进度 + 分项 breakdown（读文件/变更文件/用量）。
  - `WorkspacePanel.tsx`（85KB）+ `ProjectTree.tsx`（98KB）：文件树 + Git 变更/分支/历史。
  - `SettingsPanel.tsx`（294KB）、`CapabilitiesPanel.tsx`（132KB）、`SubagentsPanel.tsx`、`MemoryPanel.tsx`（62KB）、`StatusBar.tsx`、`CommandPalette.tsx`。
- **交互细节**：`@tanstack/react-virtual` 虚拟列表、gsap 入场动画、文件拖放（`runtime.OnFileDrop`）、approval 模态、`useController.ts`（150KB 统一控制器：提交/取消/引导/审批/流式状态机）。
- **设计语言**：`styles.css` 约 900KB（组件级 token 体系 + z-index 契约检查脚本）、三语 i18n（zh/en/zh-TW 各 170KB）。

### A.2 Mady 现状

- 布局：全宽 TitleBar + `Sidebar`（会话/项目/文件三 tab，文件 tab 为占位）+ `ChatView`（消息列表已虚拟化）+ `DocumentViewer` + `A2UIOverlay`；覆盖层含设置/知识库/模板/技能/MCP/文件查看器/`⌘K`。
- 无多标签、无独立历史面板、无上下文用量环形可视化、无记忆/子代理面板。
- 动画用 framer-motion；主题 4 套 token。

### A.3 差距与落地建议（按价值排序）

| 仿照点 | 价值 | 成本 | 落地建议 |
|--------|------|------|----------|
| 多标签 TabBar | 高（多案件并行） | 中 | 先做**会话级标签**（前端多 thread 并行渲染切换，Go 侧不动）；完整 `WorkspaceTab` 状态机 + 启动恢复见阶段 4 |
| 历史面板（搜索/恢复/回收站） | 高 | 中 | 复用现有会话列表，加搜索 + 软删除 + 恢复 |
| 上下文用量环形可视化 | 中 | 低 | AGUI `context-usage` 事件已到位，前端补 ContextWindowRing 组件即可 |
| Transcript 回合分组/折叠 | 中 | 中 | 改造现有 `MessageBubble`/消息列表渲染 |
| Composer 增强（草稿/粘贴图/↑↓ 历史） | 高 | 中 | 会话草稿本地持久化；粘贴图片走现有 `SavePastedImage` 思路 |
| 命令面板升级 | 低 | 低 | 已有 `⌘K`，补充斜杠/技能条目即可 |
| 三语 i18n | 中 | 中 | 已有 `pkg/i18n`（zh-CN/en-US），前端补齐 en-US |

---

## B. Go 后端功能模块

### B.1 Reasonix 的绑定层设计

- **模式**：绑定方法成对 `Method()` / `MethodForTab(tabID, …)`，作用于当前激活 Tab 或指定 Tab；轮询快照（`History` 分页 / `Meta` / `ContextUsage` / `Jobs`）。
- **会话层**（`sessions.go`）：自定义标题 sidecar `.titles.json`、展示层 `.display.json`、回收站 `.trash`/`.trash-meta.json`、恢复副本 + GC。
- **标签/工作区层**（`tabs.go` 268KB + `workspace.go`）：`WorkspaceTab`/`TabMeta`/`ProjectNode`/`TopicMeta`；`desktop-workspaces.json`（最近 12 个）+ 启动 `chdir` 到可写目录；`restoreOrBuildTabs` 启动恢复。
- **会话 runtime 状态机**（`session_runtime.go`）：每会话 runtime 注册表，`starting/ready/lease_blocked/failed/closing` 相位。
- **Git 变更面板**（`workspace_changes.go`）：`WorkspaceChanges`/`GitBranches`/`GitCheckout`/`WorkspaceGitHistory`。
- **记忆/目标**：`Memory/Remember/Forget/SaveDoc`；`SetGoal/ClearGoal/ResumeGoal`；`AutoResearch*`。
- **MCP 全生命周期**：`InstallMCP/Add/Update/Remove/Reconnect/SetEnabled/SetTier/AuthorizeAndConnect`（对照 Mady 仅只读 `ListMcpServers`）。

### B.2 Mady 现状

- 27 个绑定：`Chat`/`Cancel`/`SendAction`/线程 CRUD/文件 CRUD/设置/技能/MCP 只读/模板/知识库状态/项目/`Health`/`CheckUpdate`（占位）/`SaveWindowState`。
- 单 agent runtime：`server.Server` + `bootstrap.Context`，`runs sync.Map` 管理并发回合。

### B.3 差距与落地建议（只吸收与专利/法律场景相关的）

| 仿照点 | 价值 | 落地建议 |
|--------|------|----------|
| 会话自定义标题 + 回收站 | 高 | Go 侧加 `.titles.json` sidecar + 软删除/恢复/彻底清除；前端历史面板配合 |
| 会话 runtime 状态机 | 中 | 现为 `runs sync.Map`，补相位枚举 + 启动恢复标记 |
| 启动恢复上次线程/项目 | 高 | 已有 `applyLastProject`，补「上次线程」恢复 |
| 上下文用量持久化快照 | 中 | `context-usage` 事件落盘，供历史回看 |
| MCP 生命周期管理 | 中 | **涉安全红线**（`mcp/config_trust.go`），需独立评审后推进 |
| 记忆面板（`memory/` 三层系统已有） | 中 | 新增 `Memory/Remember/Forget` 绑定 + 前端面板 |
| 长会话历史分页 | 中 | 现有 `GetThread` 全量返回，补分页 |

**不吸收**：远程 SSH 工作台、Bot 网关、心跳定时任务（与 Mady 领域无关，避免包袱）。

---

## C. 工程能力

### C.1 Reasonix 的工程设施

- **自动更新全链路**：manifest（minisign 验签）→ 下载 → 平台安装（Win NSIS / Linux tar+deb+Polkit / mac 手动跳转）→ 回滚；`updater:progress` 事件 + `UpdateBanner`；`desktop-v<semver>` 独立 tag + `release-desktop.yml` 原生 runner 构建。
- **崩溃恢复**：fatal 输出捕获 → pending 上报（重度脱敏）→ 启动 `StartupTracker` 判定 safe-mode → `RecoveryGC` 清理。
- **窗口/托盘/单实例**：窗口几何 + ZoomFactor 恢复、`StartHidden` + domReady 显示、macOS 原生菜单、托盘菜单、单实例锁（Mady 已有 ✓）。
- **杂项**：外部 opener、自动保存、hang 看门狗、WebKit 兼容策略、遥测/指标。

### C.2 Mady 现状与缺口

| 能力 | 状态 | 缺口 |
|------|------|------|
| 自动更新 | 占位 | `CheckUpdate` 恒返回最新，`desktopVersion` 硬编码未 ldflags 注入；已有评估文档 `desktop-autoupdate-assessment.md` / `desktop-notarization-assessment.md` 可直接推进 |
| 窗口状态 | 部分 | 只存宽高，**X/Y 为死字段**未持久化 |
| 系统托盘 | ✓ | 已有（fork systray + 长任务通知） |
| 单实例 | ✓ | 已有 `SingleInstanceLock` + focus 回调 |
| 崩溃恢复 | ✗ | 无 fatal 捕获、无 pending 上报、无 safe-mode 启动 |
| 原生菜单 | ✗ | macOS 菜单缺失（设置入口） |
| 平台 | macOS-only | `main_unsupported.go` 明确 Win/Linux 未实现 |

### C.3 落地建议（按风险从低到高）

1. **版本注入 + 真实 CheckUpdate**：`-ldflags -X` 注入 `desktopVersion`，实现真实版本比较（低风险、立即可做）。
2. **窗口 X/Y 持久化**：补 `window_state.go` 死字段（5 分钟小步）。
3. **macOS 原生菜单**：设置/关于/退出入口（参考 `menu.go` 的 `app:open-settings` 事件模式）。
4. **自动更新链路**：按已有评估文档分步推进（manifest 验签 → mac 路径）。
5. **崩溃捕获 + 启动恢复**：fatal 输出重定向 + 启动标记 + safe-mode（需谨慎设计，避免误伤）。
6. **跨平台**（Windows/Linux）：**独立立项**，拆 darwin build tag、WebView2/WebKitGTK 适配，属大工程。

---

## D. 主题系统

### D.1 Reasonix 的主题体系

- **主题包**：V2 格式（受控不可执行皮肤），`theme.json` + `background.webp` 背景图；8 套官方主题内嵌 `themes/official/`。
- **Go 侧**：`theme_store.go` 持久化 / `theme_official.go` 内嵌 / `theme_import.go` zip 导入 / `theme_assets.go` HTTP 中间件（`/__reasonix_theme_asset/`）服务背景图 / `theme_contrast.go` 对比度校验 / `theme_image.go` 尺寸校验。
- **绑定**：`ListThemePacks`/`ActivateThemePack`/`ActivateBaseStyle`/`Import`/`Export`/`PickThemeBackground`。
- **前端**：`ThemeGallery`/`ThemeLibrary`/`ThemePreviewSurface`、`lib/themePack.ts`、`lib/themeExperience.ts`。

### D.2 Mady 现状

- `tokens.ts` + `packs.ts`（4 套：professional 靛蓝默认 / focus-blue / paper-warm / slate，各含 light+dark）+ `provider.tsx`（localStorage 持久化、运行时注入 CSS 变量、防 FOUC）。
- 无背景图、无导入导出、无画廊/预览、无对比度校验。

### D.3 差距与落地建议

| 仿照点 | 落地建议 |
|--------|----------|
| 主题包结构扩展 | `packs.ts` 加背景图字段 + 可选强调色/侧栏 token（低风险，纯前端） |
| 背景图资产服务 | 参考 `theme_assets.go` 中间件模式，在 Mady `AssetServer` 链上加 `/__mady_theme_asset/` 或直接走 `AssetServer` 静态路由 |
| 主题画廊/预览 | 新组件 `ThemeGallery`（4 套现有主题先入画廊） |
| 导入/导出 | zip 打包/解包（Go 侧 `theme_import.go` 模式），中优先级 |
| 对比度校验 | WCAG 检查（`theme_contrast.go` 模式），低优先级 |

---

## E. 阶段路线图（每阶段小步、可回退）

> 遵循 AGENTS.md「单次改动 3-5 个文件」约束；每阶段独立可交付、可回退，不阻塞其他阶段。

- **阶段 1 — 工程基础**（低风险，无 UI 大改）
  1. ldflags 注入版本号 + `CheckUpdate` 真实化（基于已有评估文档）
  2. 窗口 X/Y 持久化
  3. macOS 原生菜单（设置入口）
  4. 会话自定义标题 + 回收站（Go 侧）

- **阶段 2 — UI 布局升级**（最大用户感知）
  1. 会话级多标签 TabBar —— **架构级重构**（见下「2.1 调研结论」）
  2. 历史面板（搜索/恢复/回收站联动）
  3. 上下文用量环形可视化（ContextWindowRing）
  4. Transcript 回合分组/折叠
  5. Composer 增强（草稿/粘贴图片/↑↓ 历史）

  **2.1 调研结论（2026-08-02）**：多标签非小改动。前端 chat store 现状是
  单实例运行态（`chatSlice` 全局一份 `messages/output/thinking/toolCalls/…`，
  `stores/slices/chatSlice.ts:70-136`），无 per-thread 状态容器；agui-bridge
  reducer 直接写全局 store（`reducer.ts:37-100`）。会话级并行多标签需要：
  - 方案 A（前端分片）：`chatSlice` 改为 `Record<threadId, ChatRunState>` +
    `activeThreadId`，所有 action 按 active tab 分片写入；影响面覆盖
    ChatView/MessageBubble/Composer/ToolCard/DecisionSurface/UsageStrip/
    ContextIndicator/AgentFooter/StatusBar/Sidebar/ThreadItem/TodoDock/
    ApprovalCard/sessionExport/App/reducer 等 15+ 文件，属架构重构；
    且阶段 4 完整状态机（Go 侧 `WorkspaceTab`）落地时前端分片可能返工。
  - 方案 B（Go 侧状态机提前）：把阶段 4 的 Go tabs 状态机提前到 2.1，
    前端只做 TabBar 展示 + 按 tabID 参数调用绑定；与最终目标一致，无返工，
    但工程量大（新增 tab 状态结构、绑定方法对 `ForTab` 化、启动恢复），
    且当前后端 `server.Server` 无 tab 概念，需先设计。
  - 决策待用户确认（2026-08-02 会议记录于 F 节）。

- **阶段 3 — 主题系统**
  1. 主题包结构扩展（背景图 + 更多 token）
  2. ThemeGallery 画廊 + 预览
  3. 主题导入/导出（zip）

- **阶段 4 — 进阶能力**（可选，逐项评估）
  1. 完整多标签状态机（Go 侧 `WorkspaceTab` + 启动恢复）
  2. 记忆面板、子代理面板、能力面板
  3. MCP 生命周期管理（涉安全红线，专项评审）
  4. 崩溃捕获 + safe-mode 启动恢复
  5. 跨平台（Windows/Linux）——独立立项

---

## F. 决策记录（已确认 2026-08）

1. **多标签层次**：完整状态机（Go 侧 `WorkspaceTab` + 启动恢复，阶段 4）；阶段 2 先以会话级标签过渡。
2. **视觉风格取向**：Mady 现有主题语言渐进增强，不整体换皮。
3. **平台目标**：仅 macOS（保持 darwin tag，跨平台不立项）。
4. **阶段推进**：按阶段 1→4 顺序全做。
5. **threadId 双真相源（2026-08-04 审阅对齐）**：侧栏会话列表与标签绑定会话采用**标签联动**——
   侧栏点击会话时，若已存在绑定该会话的标签则激活它，否则新建标签并绑定到该会话
   （新增 Go 绑定 `BindTabToThread`）。侧栏不再独立维护「当前会话」，聊天上下文唯一由标签驱动，
   消除「消息落 A 会话、UI 高亮 B 会话」的撕裂。
6. **wailsjs 生成物提交策略（2026-08-04 审阅对齐）**：提交当前差异，保持提交物 = 本地
   `wails dev` 生成物；`wails dev` 后若 wailsjs 有变化随代码一并提交（Wails 官方惯例）。
7. **会话重命名 UI（2026-08-04 审阅对齐）**：`RenameThread` 绑定前端接入（ThreadItem 两段式
   编辑），补阶段 1.4「自定义标题」最后一块。

---

## G. 实施状态核对表（2026-08-04 全面审阅后更新）

> 逐项对应上文 A-D 各节的落地建议；✅=已实现，⚠️=部分实现/有缺陷，❌=未实现（多为有意保留或低优先）。

| 计划项 | 状态 | 说明 |
|--------|------|------|
| 版本号 ldflags 注入（C.3.1） | ✅ | Makefile 注入 `main.desktopVersion` |
| CheckUpdate 真实化（C.3.1） | ⚠️ | 占位实现恒返回「已是最新」；真实通道依赖公证（见 autoupdate-assessment），中间态文案待改 |
| 窗口 X/Y 持久化（C.2/C.3.2） | ⚠️ | `beforeClose` 已保存 X/Y，但前端 `SaveWindowState` 双写入者覆盖，正常退出位置丢失（2026-08-04 审阅发现，修复计划批次 A3） |
| macOS 原生菜单（C.3.3） | ✅ | `menu.go` 设置/退出已就位；「关于」项待补（低） |
| 会话自定义标题 + 回收站（B.3/阶段 1.4） | ⚠️ | Go 侧完整（RenameThread/Trash/Restore/Purge）；前端重命名 UI 待接入（决策 7） |
| 启动恢复上次线程/项目（B.3） | ✅ | 项目 `applyLastProject` + 标签持久化 `desktop-tabs.json` |
| 上下文用量环形可视化（A.3） | ✅ | `ContextWindowRing.tsx`（颜色分级缺 80% 档，低） |
| 多标签 TabBar（A.3/阶段 2.1） | ✅ | Go 侧 tab 状态机 + `ChatInTab` + 前端 TabBar；threadId 采用标签联动（决策 5） |
| 历史面板/回收站（A.3） | ✅ | 侧栏搜索 + `TrashPanel` 软删除/恢复/彻底清除 |
| Composer 增强（A.3） | ⚠️ | 草稿/↑↓历史/斜杠菜单/长粘贴检测 ✅；**粘贴图片未做**（低优先） |
| 三语 i18n（A.3） | ❌ | 产品决策：发布前只做 zh-CN 单语言（见 desktop-i18n-assessment） |
| 上下文用量持久化快照（B.3） | ❌ | 未实现（低优先） |
| 长会话历史分页（B.3） | ❌ | `GetThread` 全量返回（低优先） |
| MCP 生命周期管理（B.3） | ❌ | 涉安全红线（`mcp/config_trust.go`），未立项 |
| 记忆面板（B.3/阶段 4） | ⚠️ | `MemoryPanel.tsx` 已实现；字段名 snake/camel 不匹配 bug（2026-08-04 审阅发现，修复计划批次 A2） |
| 主题包结构扩展 + 画廊（D.3） | ⚠️ | 渐变 background + `ThemeGallery` 导入导出 ✅；背景图资产服务/zip 打包/对比度校验未做（低优先） |
| 自动更新链路（C.3.4） | ❌ | 依赖公证落地后分步实施 |
| 崩溃捕获 + safe-mode（C.3.5） | ❌ | 未立项（需谨慎设计） |
| 跨平台 Win/Linux（C.3.6） | ❌ | 独立立项，不做（决策 3） |

---

## 附：参考文件

- Reasonix 桌面端：`/Users/xujian/projects/参考架构/DeepSeek-Reasonix-main-v2/desktop/`（README.md 架构图、wails.json、app.go/main.go、tabs.go、theme_*.go、updater*.go、crash_*.go、frontend/src/）
- Mady 既有评估文档：`docs/plans/desktop-autoupdate-assessment.md`、`docs/plans/desktop-notarization-assessment.md`、`docs/plans/desktop-next-development-plan.md`
- Mady 桌面端规范：`docs/specs/desktop/02-spec.md`（Wails Events 替代 SSE 的决策）
