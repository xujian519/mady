# TUI 四周优化 Sprint 落地方案

> 日期：2026-07-21
> 状态：待实施
> 范围：`cmd/mady`、`tui/chat`、`tui/component` 及少量配套测试
> 目标：在不推翻现有 TUI 架构的前提下，先解决启动链路、状态一致性、错误可见性和审批体验问题。

---

## 1. 背景与问题定义

当前 `Mady` 的 TUI 模块具备较好的底层能力：自研事件循环、差量渲染、Overlay 机制、ChatApp 分层都已经成型。但专项质量审阅表明，现阶段的主要短板不在底层渲染引擎，而在上层装配与交互一致性：

- 启动链路仍有较重的同步装配，异步初始化 `Agent` 的收益被部分抵消。
- `JudgmentView`、`ChatApp`、`tuiSession` 之间状态源分散，审批态存在真实漂移风险。
- 持久化、审批留痕、运行失败等关键问题对用户不可见，容易形成“系统没反应”的感知。
- 审批流仍偏命令行式交互，尚未升级为结构化工作流。

本方案采用四周、四个 Sprint 的推进节奏，遵循“先稳再强、先可观察再提体验”的原则。

---

## 2. 总体目标

四周结束后，TUI 应达到以下目标：

- 首帧启动路径显著瘦身，非关键能力初始化不再阻塞进入主界面。
- 交互状态拥有单一真源，审批态、失败态、降级态不再被覆盖或丢失。
- 用户无需查看日志，即可感知持久化降级、审批留痕异常和运行失败。
- 审批流程从“聊天提示 + 手输命令”升级到更可用的结构化浮层交互。
- 关键链路拥有最小但有效的自动化测试护栏。

---

## 3. 执行原则

### 3.1 改动边界

- **优先改造**：`cmd/mady/tui*.go`、`tui/chat/*.go`、`tui/component/*.go`
- **谨慎触碰**：`tui/overlay.go`、`tui/agentadapter/adapter.go`
- **本轮不动**：`tui/core/`、`tui/layout/`、`tui/terminal/`、`tui/theme/` 的底层基础设施

### 3.2 任务粒度

- 单周尽量控制在 `3-5` 个核心文件的主改动面。
- 采用“主链路先收敛，配套测试随后补齐”的小步提交方式。
- 每周完成后都要能独立回归，不依赖后续 Sprint 才成立。

### 3.3 验证原则

- 每个 Sprint 至少补一组与本周改动直接相关的测试。
- 涉及并发/生命周期的修改，优先补 `go test -race` 可覆盖的回归场景。
- 任何降级逻辑都必须做到“日志可追踪 + UI 可感知”。

---

## 4. 四周路线图

```text
Week 1 ── Sprint 1：启动链路瘦身 + 存储/留痕降级显式化
Week 2 ── Sprint 2：显式状态机接管 + JudgmentView 状态收口
Week 3 ── Sprint 3：统一错误出口 + Agent 重建/恢复链路稳态化
Week 4 ── Sprint 4：审批 Overlay + 命令中心/系统态工作台增强
```

---

## 5. Sprint 1（第 1 周）

### 5.1 主题

启动链路瘦身与降级可见性。

### 5.2 目标

- 让 TUI 尽早出首帧。
- 让 `session`、`settings`、`approvals.db` 的失败和降级被用户明确感知。
- 为后续状态与交互改造打下稳定装配基础。

### 5.3 主要改动

#### A. 拆分启动装配阶段

将当前 `setupFrameworkContext()` 拆分为：

- **首帧必需阶段**：主题、基础配置、最小化 `ChatApp`、必要 slash registry
- **后台延迟阶段**：MCP discovery、部分 memory/reasoning 初始化、非关键知识能力装配

目标是让 `app.Start()` 更早发生，而不是等所有启动准备都结束后才进入渲染。

#### B. 建立统一的存储预检

为以下路径增加统一的写探针与错误分类：

- `sessions`
- `~/.mady/settings.json`
- `approvals.db`

预检结果需要同时落到：

- 日志
- 启动时的系统消息
- 状态栏或系统态摘要

#### C. 降级语义显式化

把现有“继续运行但静默降级”改成显式文案，例如：

- “会话持久化未启用，当前为仅内存模式”
- “审批仍可继续，但审批留痕未落盘”
- “后台能力加载失败，已进入降级运行”

### 5.4 文件落点

建议主改以下文件：

- `cmd/mady/tui.go`
- `cmd/mady/framework.go`
- `cmd/mady/tui_session.go`
- `cmd/mady/tui_session_config.go`
- `cmd/mady/server.go`（抽取可复用预检逻辑时）

### 5.5 测试与验证

新增或补充：

- `cmd/mady/tui_test.go`
- `cmd/mady/tui_session_test.go`

重点覆盖：

- 启动成功与后台初始化分离
- `session` 不可写时的降级提示
- `approval store` 打不开时的降级提示
- 非关键后台能力初始化失败时，TUI 仍可进入

### 5.6 验收标准

- 首帧前同步路径缩短，`app.Start()` 前不再包含整套重型能力装配。
- 只读目录或 SQLite 打开失败时，用户能在 TUI 中看到明确提示。
- `serve` 与 `tui` 的存储预检语义保持一致。

### 5.7 风险控制

- 避免一次性拆散全部 `frameworkContext`，优先抽“延迟初始化包装层”。
- 对启动链路的调整要保留现有 fallback 行为，避免影响非 TUI 入口。

---

## 6. Sprint 2（第 2 周）

### 6.1 主题

显式状态机接管与状态单一真源。

### 6.2 目标

- 让 `state.go` 从“文档化规范”升级为实际驱动器。
- 让 `JudgmentView` 不再通过散落字段猜测状态。
- 消除审批态、阻塞态、失败态被覆盖的风险。

### 6.3 主要改动

#### A. 用状态机接管关键事件流

把以下事件纳入统一状态迁移：

- `AgentStart`
- `MessageDelta`
- `ToolStart` / `ToolEnd`
- `ApprovalPrompt`
- `ApprovalDecision`
- `CompactionStart` / `CompactionEnd`
- `AgentEnd`
- `AgentError`

所有 UI 展示状态由 `Transition()` 的结果驱动，而不是在多个 handler 里直接写状态。

#### B. 重构 JudgmentView 同步逻辑

将 `updateJudgmentView()` 改为读取显式状态，而不是只根据：

- `Running`
- `StreamID`

来反推 `idle/running/streaming`。

新状态建议至少覆盖：

- `initializing`
- `idle`
- `streaming`
- `tool_running`
- `awaiting_review`
- `compacting`
- `degraded`
- `failed`

#### C. 收敛动作提示与扩展态

`JudgmentView` 的 expanded/collapsed 判断应完全跟随显式状态：

- `awaiting_review`
- `blocked`
- 必要时的 `degraded`

不要再出现“状态写成 awaiting_review，但刷新后回到 idle”的现象。

### 6.4 文件落点

建议主改以下文件：

- `tui/chat/state.go`
- `tui/chat/chat_app.go`
- `tui/chat/chat_app_stream.go`
- `tui/chat/chat_app_tool.go`
- `tui/chat/chat_app_layout.go`
- `tui/component/judgment_view.go`

### 6.5 测试与验证

新增或补充：

- `tui/chat/state_test.go`
- `tui/chat/chat_app_test.go`

重点覆盖：

- `Idle -> AwaitingReview -> Resume -> Streaming -> Idle`
- `ToolRunning -> AwaitingReview`
- `AgentError -> Failed`
- `Compacting` 的进入与退出

### 6.6 验收标准

- `JudgmentView` 状态只来自状态机。
- 审批态进入后不会被普通刷新覆盖回 `idle`。
- 系统态入口和动作提示与真实状态一致。

### 6.7 风险控制

- 第一阶段保留旧字段，但视为兼容字段，不再作为展示真源。
- 避免一次性重写整个 `ChatApp`，先让状态机接管显示和关键行为判断。

---

## 7. Sprint 3（第 3 周）

### 7.1 主题

统一错误出口与 Agent 运行链路稳态化。

### 7.2 目标

- 消灭“只打日志、用户无感知”的外围失败路径。
- 加强 `rebuildAgent()`、`Run()`、`Resume()`、`SaveState()` 的稳态性。
- 让用户知道系统当前失败在哪一层。

### 7.3 主要改动

#### A. 统一错误回显出口

为以下错误统一建立 UI 出口：

- `agent.Run()` 失败
- `agent.Resume()` 失败
- `agent.SaveState()` 失败
- `initializeAgentAsync()` 失败
- `rebuildAgent()` 失败
- 审批留痕失败

建议区分三类信息：

- **运行失败**：当前轮次无法继续
- **后处理失败**：运行结束但状态未保存
- **降级提醒**：功能仍可继续，但能力受限

#### B. 给 rebuildAgent 增加防护

为 `rebuildAgent()` 增加：

- `recover`
- 用户可见错误提示
- 重建前后状态提示

适用于 `/plan`、`/review`、`/case`、`/clear` 等可能触发重建的命令。

#### C. 统一恢复链路反馈

当 `/approve` 触发 `resumeIfInterrupted()` 时，要明确区分：

- 成功恢复执行
- 无中断态可恢复
- 恢复失败

避免用户只看到“似乎没有反应”。

### 7.4 文件落点

建议主改以下文件：

- `cmd/mady/tui_session_agent.go`
- `cmd/mady/tui_session.go`
- `cmd/mady/slash_registry.go`
- `tui/chat/chat_app.go`
- `tui/agentadapter/adapter.go`

### 7.5 测试与验证

新增或补充：

- `cmd/mady/tui_session_agent_test.go`
- `cmd/mady/tui_session_approval_test.go`

重点覆盖：

- `Run` 返回错误时聊天区回显
- `Resume` 返回错误时聊天区回显
- `SaveState` 失败时系统消息提示
- `rebuildAgent` panic 被 recover 并可见提示

### 7.6 验收标准

- 不再存在“仅日志可见、UI 不可见”的关键外围失败路径。
- `rebuildAgent()` 失败不会直接导致进程崩溃。
- `/approve` 相关恢复链路反馈清晰。

### 7.7 风险控制

- 避免把所有错误都变成红色致命提示，应按严重度分层输出。
- 对用户文案需遵循 `docs/tone-style-guide.md`，避免夸大或误导。

---

## 8. Sprint 4（第 4 周）

### 8.1 主题

审批体验升级与工作台增强。

### 8.2 目标

- 把审批流从“聊天流中的命令提示”升级为结构化 Overlay。
- 让用户可以从统一入口看到当前可执行动作和系统态摘要。
- 在不改为常驻多栏的前提下，增强工作台能力。

### 8.3 主要改动

#### A. 审批 Overlay

基于已有设计文档继续实施：

- `docs/design/tui-overlay-implementation-plan.md`
- `docs/design/tui-overlay-optimization-v0.1.md`

审批 Overlay 至少展示：

- 当前判断摘要
- 工具或操作名称
- 风险等级
- 参数/依据摘要
- 留痕状态
- 快捷键操作（继续/拒绝/关闭）

#### B. 命令中心增强

把命令中心从“slash 搜索”升级为“上下文动作入口”，增加：

- 当前状态下可执行动作
- 不可用原因
- 最近使用
- 快捷键提示

#### C. 系统态工作台雏形

增加只读系统态视图，展示：

- 当前 `AgentState`
- 活跃工具
- 是否等待审批
- 持久化状态
- 上下文窗口使用情况
- 最近 checkpoint 或运行摘要

### 8.4 文件落点

建议主改以下文件：

- `tui/component/review_gate.go`
- `tui/component/system_status.go`
- `tui/chat/chat_app.go`
- `tui/chat/chat_bridge.go`
- `cmd/mady/slash_registry.go`
- `cmd/mady/tui_session.go`

### 8.5 测试与验证

新增或补充：

- `tui/component/review_gate_test.go`
- `tui/component/system_status_test.go`
- `tui/chat/chat_app_test.go`

重点覆盖：

- Overlay 打开/关闭与焦点恢复
- 审批确认与拒绝快捷键
- 命令中心动作可见性
- 系统态面板数据绑定正确

### 8.6 验收标准

- 用户无需记忆 `/approve`、`/reject` 即可完成审批。
- 审批态、系统态和普通聊天消息在视觉上清晰区分。
- 命令中心能说明“现在能做什么”和“为什么不能做”。

### 8.7 风险控制

- 保持“聊天优先、浮层按需展开”，不要演变成常驻多栏主界面。
- 审批 Overlay 先做最小闭环，不在本轮引入复杂多级交互。

---

## 9. 每周交付物

### Week 1 交付物

- 启动链路拆分设计与首版实现
- 统一的存储预检/降级提示
- 启动链路 smoke test

### Week 2 交付物

- 状态机接管关键事件流
- `JudgmentView` 状态同步重构
- 状态机回归测试

### Week 3 交付物

- 统一错误回显出口
- `rebuildAgent()` 防护
- 运行/恢复/保存链路测试

### Week 4 交付物

- 审批 Overlay 最小闭环
- 命令中心增强
- 系统态工作台雏形

---

## 10. Definition of Done

四周方案完成时，应满足以下 DoD：

- 启动：
  - 首帧前同步装配被削薄
  - 非关键后台初始化失败不阻塞 TUI 进入
- 状态：
  - 显式状态机成为展示真源
  - 审批态、失败态、降级态不再漂移
- 可观察性：
  - 持久化/审批留痕/恢复/保存失败均有 UI 提示
  - 用户无需依赖日志理解当前问题
- 体验：
  - 审批流程具备结构化 Overlay
  - 命令中心和系统态具备最小工作台能力
- 验证：
  - 关键新增或变更路径具备测试护栏
  - `go test -race ./...` 不因本轮改动新增明显回归

---

## 11. 本轮不做

以下事项明确不在本四周方案内：

- 将现有 TUI 底层整体替换为其他框架
- 把主界面改为 `lazygit` 式常驻多栏布局
- 一次性重构 `ChatHistory` 整个子系统
- 提前引入插件体系或大规模主题系统重构

---

## 12. 推荐实施顺序

若资源充足，按四周顺序推进。

若资源有限，推荐优先级如下：

1. `Sprint 1`：启动链路瘦身 + 降级显式化
2. `Sprint 2`：状态机接管 + JudgmentView 收口
3. `Sprint 3`：统一错误出口
4. `Sprint 4`：审批 Overlay 与工作台增强

也就是说，即便只能完成前两周，本轮也应先把“启动更顺、状态可信、失败可见”做扎实，再考虑体验增强。

---

## 13. 关联文档

- `docs/design/tui-overlay-implementation-plan.md`
- `docs/design/tui-overlay-optimization-v0.1.md`
- `docs/review/phase6-p4-tui-entry.md`
- `docs/chat-assistant-architecture.md`
- `docs/tone-style-guide.md`
