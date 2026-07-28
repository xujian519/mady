# BCIP 桌面端：性能架构、无感接入与 Codex 像素级对齐策略

**日期**: 2026-05-30
**状态**: 架构决策（仅文档，不含实现）
**读者**: 产品、设计、桌面端与 app-server 开发

---

## 0. 执行摘要

| 问题 | 结论 |
|------|------|
| 当前 `apps/desktop` 是否是性能最佳方案？ | **不是。** WebSocket 直连 + 双进程管理 + 不完整 JSON-RPC 握手，既非官方推荐路径，也增加延迟与故障面。 |
| 客户已装 CLI/TUI，桌面如何无感接入？ | **单一事实来源 = 本机 `config.toml` + 单一 app-server 进程**；桌面只做「发现 / 附着 / 展示」，不复制 MCP/技能配置。 |
| MCP、技能等如何像 Codex 一样好配？ | **全部走 app-server v2**（`config/*`、`mcpServer*`、`skills/*`、`plugin/*`），UI 是配置面的编辑器，不是第二套配置系统。 |
| 「像素级复刻 Codex」指什么？ | **对话与配置体验**对齐 Codex 官方客户端（线程/回合/条目、审批、MCP 状态、设置写入）；**专利工作区**（文件树、预览、阶段指示器）可作为 BCIP 差异化壳层，不应与 Codex 主会话 UX 抢同一信息架构。 |

**推荐目标架构（本地部署）**：

```
桌面壳 (Tauri + React)
  ├─ 专利工作区 UI（文件 / 预览 / 阶段 / Todo）── Tauri FS + 可选 fs/watch
  └─ Codex parity 客户端（线程 UI + 设置 + MCP/技能）── JSON-RPC
         └─ Tauri 仅负责：附着 app-server（unix socket 或 stdio 代理）
                └─ bcip app-server（与 CLI/TUI 共用 CODEX_HOME / config.toml）
```

---

## 1. 当前方案为何不是「性能最佳」

### 1.1 现状（`apps/desktop`）

- 前端 `bcipClient.ts` 使用 **WebSocket** `ws://127.0.0.1:9002`。
- Tauri 侧存在 **两套** app-server 生命周期：`AppServerManager`（setup 自动启动）与 `commands/system.rs` 静态子进程（`invoke` 启动）。
- 协议不完整：缺少 `initialized` 通知；`turn/start` 未带 `threadId` / `input`；未实现 `thread/start`。
- 中心区大量 **mock 数据**（检索/对比/审查/起草），与后端专利流水线未绑定。
- 内嵌 `bcip tui`（xterm + PTY）作为能力兜底，**不应**与 app-server 主路径并行承担「主对话」职责。

### 1.2 官方传输层立场（`codex-rs/app-server/README.md`）

| 传输 | 适用 | 性能 / 稳定性 |
|------|------|----------------|
| **stdio**（默认） | 编辑器/IDE 子进程、代理 | 最低开销；无 HTTP 升级 |
| **unix socket** + WS 帧 | 本地控制面（`app-server-control.sock`） | 本机推荐；VS Code 类客户端 |
| **WebSocket `ws://IP:PORT`** | 实验性 | 文档明确 **experimental / unsupported**，不宜生产默认 |

因此：**继续以 WS 为唯一通道不是性能最佳，也不是与 Codex 生态一致的选择。**

### 1.3 性能对比（本地单机，定性）

| 维度 | 当前 WS + 自启子进程 | 推荐：unix socket / stdio 代理 + 单例 app-server |
|------|----------------------|--------------------------------------------------|
| 连接建立 | TCP + WS 握手 | 本机 socket 或管道，少一跳 |
| 进程数 | 易重复启动 2 份 server | 发现已有实例则附着 |
| 配置热更新 | 易与磁盘 config 脱节 | `config/mcpServer/reload` 与 TUI/CLI 一致 |
| 客户端维护成本 | 手写残缺协议 | `bcip app-server generate-ts` 与 Codex 同源 |

---

## 2. 推荐架构：三层分离

### 2.1 Layer A — 桌面壳（Tauri）

**职责（应保留）**

- 无边框窗口、文件对话框、本地 FS 读写、PTY 终端（可选）、系统通知。
- 可选：**捆绑 `bcip` sidecar**（安装包内二进制），用于「零前置安装」客户。

**不应承担**

- 自实现 MCP 协议、技能解析、模型路由（均属 app-server / core）。

### 2.2 Layer B — Codex Parity 客户端（React 模块）

**职责（应对齐 Codex / `codex_vscode`）**

- JSON-RPC 2.0 全生命周期：`initialize` → `initialized` → `thread/start` → `turn/start` → 订阅 `item/*`、`turn/*` 通知。
- 会话列表：`thread/list`、`thread/resume`、`thread/read`。
- 用户审批 UI：`commandExecution/requestApproval`、`fileChange/requestApproval`、`mcpServer/elicitation/request` 等（见 app-server README Approvals）。
- 设置与集成：
  - `config/read`、`config/value/write`、`config/batchWrite`
  - `mcpServerStatus/list`、`config/mcpServer/reload`、`mcpServer/oauth/login`
  - `skills/list`、`skills/config/write`、`skills/changed`
  - `model/list`、`permissionProfile/list`（按需）
  - 插件/marketplace（experimental 区，与 Codex 一致标注）

**实现约束**

- TypeScript 类型：**必须**由 `bcip app-server generate-ts` 生成，禁止长期手写 RPC 载荷。
- `clientInfo.name`：申请稳定 id（文档建议企业客户端登记）；过渡期可用 `bcip-desktop`，与 `codex_vscode` 区分。

### 2.3 Layer C — 专利工作区（BCIP 差异化）

**职责**

- 项目目录（`.bcip/project.json`）、专利文书预览/编辑、工作流阶段指示器、Todo（对接 agent 输出或 `item/*`）。
- `thread/start` 的 **`cwd`** 必须绑定当前打开的项目根路径，保证技能/MCP/规则按项目解析。

**与 Codex 的关系**

- 这是 **壳层增强**，不是第二套 Agent；所有「智能」仍通过 Layer B 发往 app-server。

---

## 3. 客户本地已装 CLI/TUI：无感接入设计

### 3.1 原则

1. **不二次安装配置**：桌面读写与 TUI 相同的 `config.toml`（路径解析需与 core `find_codex_home()` 一致，而非仅 `~/.config/bcip`）。
2. **不二次启动 server（默认）**：优先连接已有 app-server；仅在无实例时启动。
3. **CLI 与 TUI 等价**：二者共用 home；桌面是第三种 **UI 皮肤**，不是第四种后端。

### 3.2 启动时状态机（产品 + 工程）

```
启动桌面应用
    │
    ▼
检测 bcip 二进制 ──未安装──► 引导：捆绑 sidecar 安装 / 文档链接 / 仅文件模式
    │
    已安装
    ▼
解析 CODEX_HOME 与 config.toml
    │
    ▼
探测 app-server 是否已监听
    ├─ unix socket 可用 ──► proxy 附着（推荐）
    ├─ 用户已在终端运行 `bcip app-server` ──► 附着现有进程
    └─ 无实例 ──► 启动单例（daemon 或子进程，二选一策略见 3.3）
    │
    ▼
initialize + initialized
    │
    ▼
thread/start(cwd = 当前项目) 或 thread/resume(最近线程)
    │
    ▼
UI 就绪（无「请先手动起 server」）
```

**用户可见文案**：仅当附着/启动失败时提示；成功时 StatusBar 显示「已连接 · 与终端共用配置」。

### 3.3 app-server 单例策略（择一为主，避免双实现）

| 策略 | 场景 | 说明 |
|------|------|------|
| **A. 附着 unix socket** | 用户常用 TUI/IDE、或 `app-server daemon` | 与 Codex 桌面一致；Tauri spawn `bcip app-server proxy`，stdin/stdout 与 UI 桥接 |
| **B. 内嵌启动** | 首次纯 GUI 用户 | Tauri 启动 `bcip app-server --listen unix://`（或 stdio），**禁止**再有一套 `static APP_SERVER_CHILD` |
| **C. 远程/SSH** | 企业部署 | `app-server-daemon` bootstrap（experimental），桌面作远程控制客户端 |

**禁止**：`AppServerManager` 与 `system.rs` 各管一份子进程（当前缺陷，会导致端口占用、配置不同步）。

### 3.4 TUI 用户路径

- **不需要**客户「先开 TUI 再开桌面」。
- 若客户习惯 TUI：桌面与 TUI 同时连接**同一** app-server 实例时，以 **threadId** 区分会话；UI 显示「其他客户端打开的线程」可选（`thread/list`）。
- 内嵌 xterm `bcip tui` 降级为 **高级用户入口**，默认隐藏或收起到「打开终端模式」。

### 3.5 捆绑 vs 外置 CLI

| 模式 | 优点 | 缺点 |
|------|------|------|
| **外置**（客户自行 `brew install` / 安装包） | 与 TUI 100% 同二进制；升级独立 | 首启需检测 |
| **Sidecar 捆绑** | 真·无感 | 包体大；须与 channel 版本同步；仍应读写用户 home 下 config |

**建议**：发行版 **捆绑 sidecar + 检测外置更新版本**，优先用较新者；配置始终写用户 home，不写 app bundle 内。

---

## 4. MCP / 技能 / 插件：与 Codex 同源的配置模型

### 4.1 单一配置源

所有扩展能力定义在 **`$CODEX_HOME/config.toml`**（及项目级覆盖），桌面 **禁止**维护平行数据库。

| 能力 | app-server 入口 | 桌面 UI 应提供 |
|------|-----------------|----------------|
| MCP 服务器列表 | `mcpServerStatus/list` | 列表 + 状态（starting/ready/failed）+ OAuth 按钮 |
| 编辑 MCP | 写 `mcp_servers.*` via `config/value/write` 或批量 `config/batchWrite` | 表单 / JSON 编辑器 + 保存后 `config/mcpServer/reload` |
| MCP OAuth | `mcpServer/oauth/login` + 通知 `mcpServer/oauthLogin/completed` | 系统浏览器打开 + 回调状态 |
| MCP 工具调用中 | `item` type `mcpToolCall` | 与 Codex 相同的进行中/完成/失败卡片 |
| MCP elicitation | `mcpServer/elicitation/request` | 模态表单 |
| 技能列表 | `skills/list`（按 cwd） | 启用/禁用 → `skills/config/write` |
| 技能热更新 | `skills/changed` | 自动刷新列表 |
| 插件市场 | `plugin/list` / `plugin/install`（experimental） | 与 Codex 相同「实验性」标签 |
| 模型 | `model/list` | 设置页下拉，禁止硬编码 DeepSeek/Claude 列表 |
| 审批策略 | `thread/start` / `turn/start` 参数 + config | 与 Codex 沙箱/permissions 文案一致 |

### 4.2 专利技能（BCIP）

- 仓库内 `codex-patent-skills`、`.codex/skills` 等仍通过 **同一** `skills/list` 暴露。
- 桌面「专利工作流」按钮 = 向当前 thread 发送 **带 skill 引用的用户输入** 或 slash 命令，而非本地 mock 页面。

### 4.3 配置 UX 对标 Codex（功能清单）

设计稿需覆盖（可与 Codex 设置页逐项 diff）：

- [ ] 模型与 reasoning effort / service tier
- [ ] Approval policy / sandbox / permissions profile
- [ ] MCP servers（增删改、重载、OAuth）
- [ ] Skills（列表、启用、路径）
- [ ] Plugins / marketplaces（experimental）
- [ ] Memories / features flags（experimental）
- [ ] `configRequirements/read` 只读展示（企业托管约束）
- [ ] 键盘快捷键（可映射 `desktop.*` config 键）

---

## 5. 「像素级复刻 Codex」的范围定义

### 5.1 必须复刻（P0）

- 线程侧边栏 + 消息时间线结构（user / agent / tool / reasoning 分区）。
- 流式 `item/agentMessage/delta` 渲染与中断（`turn/interrupt`）。
- 工具调用、命令执行、补丁应用的 **条目卡片**样式与状态机。
- 审批弹窗/interrupt 流程（含 MCP elicitation）。
- 连接/重连/未初始化/过载（`-32001`）提示文案与重试策略。
- 设置页与 `config.toml` 字段一一对应（camelCase 仅存在于 RPC 层）。

### 5.2 允许差异化（P1）

- 三栏布局、专利阶段指示器、PDF/Markdown 工作区、项目树。
- 品牌（云熙 / BCIP）、配色可在 **token 层**替换，但 **组件拓扑**（间距、密度、面板比例）建议对齐 Codex 以便用户迁移。

### 5.3 不应复刻（避免范围爆炸）

- 不要在桌面内再实现一套 TUI 字符画布。
- 不要用 mock 专利数据替代真实 `turn/item` 流（演示模式除外，需明确标注）。

### 5.4 参考实现来源

- 协议：`codex-rs/app-server/README.md`
- 类型：`bcip app-server generate-ts`
- 客户端标识与握手：README 中 `codex_vscode` 示例
- 集成范例：`docs/plans/INTEGRATION.md`（需按本文 transport 章节修正 WS 默认）

---

## 6. 与现有 `2026-05-30-desktop-redesign.md` 的关系

| 原 redesign 项 | 本策略调整 |
|----------------|------------|
| 删除中心 ChatView，仅 RightPanel 对话 | **同意**；RightPanel 应升级为 **Codex parity 会话面板**（非简化 WS 客户端） |
| API 抽象层 tauri/mock | **同意**；增加 **`appServer.ts`**（或 Rust proxy）作为第三实现 |
| 工作流阶段 + 文件驱动 | **保留**为 BCIP 壳层，通过 `cwd` + 文件 watch 与 thread 联动 |
| Markdown 分屏 / PDF 标注 | **保留**为 P1 专利差异化，不阻塞 Codex parity |

---

## 7. 实施路线（建议）

| 阶段 | 目标 | 产出 |
|------|------|------|
| **P0** | 传输与单例 | 去掉 WS 默认；unix/stdio proxy；合并 app-server 管理；完整握手 + thread/turn |
| **P0** | Codex 会话 UI | 基于 generate-ts 的会话组件；审批 + MCP 状态 |
| **P1** | 设置 = config | MCP/技能/模型设置页全部 RPC 驱动 |
| **P1** | 无感接入 | sidecar + 检测；启动状态机；失败引导 |
| **P2** | 专利壳层 | 阶段指示器与 item 通知联动；Todo 与 agent 同步 |
| **P2** | 像素打磨 | 与 Codex 截图 diff；无障碍与快捷键 |

**详细任务分解、文件映射、排期与 Issue 模板**：见 [`2026-05-30-desktop-implementation-plan.md`](2026-05-30-desktop-implementation-plan.md)。

---

## 8. 风险与决策记录

| 风险 | 缓解 |
|------|------|
| WS 在生产环境不可用 | P0 切换 unix/stdio |
| 双 app-server 进程 | 删除重复管理，只保留 attach-or-spawn |
| 配置路径不一致（`~/.config/bcip` vs CODEX_HOME） | Rust 统一调用 core 路径解析 |
| 「像素级」范围失控 | 本文 5.1/5.2/5.3 边界 |
| 插件 API experimental | UI 与 Codex 相同标注，不承诺稳定 |

---

## 9. 验收标准（无感 + Codex parity）

1. 用户已配置 TUI 的 MCP/技能，**不修改任何文件**打开桌面，对话可用且 `mcpServerStatus/list` 与 TUI 一致。
2. 用户仅安装桌面捆绑包，**首次启动** 60 秒内完成 thread 创建并发送首条消息。
3. 设置页修改 MCP 后，`config/mcpServer/reload` + TUI 侧下一 turn 可见新工具。
4. 协议测试：通过 `codex-app-server-protocol` 同类握手用例（initialize/initialized/thread/turn）。
5. UI：核心会话/审批/MCP 状态与 Codex 对照表 **≥ 90%** 组件级一致（设计走查）。

---

**文档结束**
