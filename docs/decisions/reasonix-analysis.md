# Reasonix 高价值特性引入分析报告

> 对照 DeepSeek-Reasonix-main-v2 与 Mady 项目的深度分析，识别值得引入的高价值设计、模块和工具。

---

## 一、项目对比总览

| 维度 | Reasonix | Mady |
|------|----------|------|
| **定位** | 终端编程 Agent（DeepSeek 原生） | 专利/法律专业领域 Agent 平台 |
| **语言** | Go（单模块） | Go（go.work 多模块） |
| **核心设计** | 缓存优先 + 配置/插件驱动 | 分层架构 + 领域扩展 + 心理引擎 |
| **安全机制** | AI 审查(Guardian) + 权限门控 + 沙箱 | 关键词护栏 + Handoff 白名单 |
| **记忆系统** | Memory Compiler v5（策略学习 + 因果图） | 基础 Store/Extract/Retrieve |
| **上下文管理** | 四级压缩管线（snip→prune→summary→force） | 单级摘要压缩 |
| **工具安全** | Evidence Ledger + Tool Contract + Preview/Diff | 无 |
| **回溯能力** | 文件级快照 + 会话回卷 | 会话级 Checkpoint（无文件状态） |

---

## 二、高价值特性分析（按优先级排序）

### P0 — 立即引入（安全/正确性关键）

#### 1. 🔒 Evidence Ledger（工具调用证据账本）

**Reasonix 实现：** `internal/evidence/evidence.go` (~1050 行)

**核心思路：**
每个工具调用记录为 `Receipt`，内存中维护当前 turn 的完整调用链。支持：
- 命令匹配验证（`HasSuccessfulCommand`）
- 路径覆盖验证（`HasSuccessfulWrite`, `HasSuccessfulReadOrWrite`）
- Todo 步骤匹配（`MatchStep`, `IncompleteLatestTodos`）
- 跨 turn 路径溯源（`PathsProvenInSession`）
- 写操作/命令检测（`HasWriteOrCommandSince`）

**Mady 缺失：** 完全没有工具调用追踪。Agent 执行专利分析、法律文书生成时，无法验证"模型声称的步骤是否真正执行了对应工具"。

**引入价值：** 专利/法律场景下，工具调用的可审计性是刚需。例如"专利检索是否真正执行了""文书文件是否已写入"都可通过 Ledger 验证。

**建议落地方案：**
```
agentcore/evidence/
├── ledger.go       # Receipt 存储 + 查询
├── receipt.go      # 工具调用记录结构
├── match.go        # 命令/路径匹配逻辑
└── context.go      # context.Context 注入
```
作为 `LifecycleHook` 注入 `BeforeToolCall`/`AfterToolCall`，自动记录。

---

#### 2. 🛡️ Guardian AI 安全审查（子 Agent 审查工具调用）

**Reasonix 实现：** `internal/guardian/guardian.go` (~554 行)

**核心思路：**
独立的 AI 子 Agent，使用专用安全策略 Prompt 审查每次工具调用：
- **复用单会话**（prefix cache 友好）：每次审查只增量发送 delta 证据
- **熔断器**：连续 3 次拒绝或 50 次窗口内 10 次拒绝 → 中断
- **Fail-closed**：审查失败 = 拒绝
- **结构化判定**：`{risk_level, user_authorization, outcome, rationale}`

**Mady 对比：** 当前护栏（`guardrails/levels.go`）基于关键词匹配，无法理解上下文语义。例如"删除专利文件"不会触发关键词护栏，但 Guardian 可以从对话上下文判断风险。

**引入价值：** 在专利/法律领域，Guardian 可在执行高风险操作前（如发送邮件、提交官方文件、删除案件文件）进行 AI 级安全审查，是关键词护栏的语义升级。

**建议落地方案：**
- 作为 `Extension` 注入 `BeforeToolCall` 钩子
- 复用 Mady 已有的 Provider 接口创建审查子 Agent
- 策略 Prompt 针对专利/法律领域定制（检测：真实当事人信息、不可逆法律操作、未经授权的文件提交等）
- 与现有三级护栏（Light/Standard/Strict）叠加：Strict 级别自动启用 Guardian

---

#### 3. 📸 文件级 Checkpoint + Rewind（快照回溯）

**Reasonix 实现：** `internal/checkpoint/checkpoint.go` (~200+ 行)

**核心思路：**
- 每个 user turn 开始时，对即将被编辑的文件拍摄快照
- 快照存储在 `.ckpt/` 目录（git-free，独立于用户 git）
- 支持工作区文件回卷 + 会话对话回卷
- 只追踪编辑工具的变更（bash 副作用不追踪）

**Mady 对比：** 当前 `checkpoint.go` 只有会话状态快照（`StateSnapshot`），没有文件内容快照。Agent 编辑专利文书后无法回退到编辑前状态。

**引入价值：** 专利代理人使用 Agent 修改权利要求、说明书后，如果修改方向错误，可以一键回退到任意 turn 的文件状态。这是法律文书编辑的刚需安全网。

**建议落地方案：**
```
agentcore/filecheckpoint/
├── store.go        # FileSnap 存储
├── snapshot.go     # 文件快照捕获
└── restore.go      # 回卷恢复
```
集成到 `tools/edit.go`、`tools/write_file.go` 的 `BeforeToolCall` 钩子。

---

### P1 — 短期引入（显著提升能力）

#### 4. 🧠 Memory Compiler v5（策略学习型记忆编译器）

**Reasonix 实现：** `internal/memorycompiler/runtime.go` (~2000+ 行)

**核心思路：**
超越简单的"存取"记忆，实现**执行学习闭环**：
- **策略图谱**：记录每种策略的成功率/失败率/样本数
- **ε-greedy 探索**：自适应探索率（3%-12%），稳定时多探索，不稳定时收敛
- **因果边**：记忆引用 → 决策 → 结果的因果关系追踪
- **漂移检测**：语义漂移（semantic drift）检测和告警
- **记忆质量分级**：HIGH_SIGNAL / MEDIUM_SIGNAL / NOISE / CORRUPTED
- **置信度衰减**：基于时间的置信度衰减（每周 5%）
- **编译器变异**：基于执行反馈的自改进规则
- **IR 验证**：计划 IR 与实际执行的偏差检测

**Mady 对比：** 当前 `memory/manager.go` 是基础的 Store + Extract + Retrieve，没有学习能力——不会从成功/失败中积累策略偏好。

**引入价值：** 在专利撰写、审查意见答复等场景中，Agent 可以学习"哪种答复策略成功率更高""哪些检索关键词组合更有效"，逐步提升专业能力。

**建议落地方案（分阶段）：**
- **Phase 1**：引入策略记录和成功率统计（Strategy + SuccessRate）
- **Phase 2**：引入记忆质量分级和置信度衰减
- **Phase 3**：引入 ε-greedy 策略选择和探索机制
- **Phase 4**：引入漂移检测和因果分析

适配为专利/法律领域策略（如"三步法答复""权利要求拆分""检索式构建"等）。

---

#### 5. 🔄 四级上下文压缩管线

**Reasonix 实现：** `internal/agent/compact.go` (~500+ 行)

**核心思路：**
四级渐进式压缩，最大化 prefix cache 命中率：
1. **Soft Notice** (0.5)：仅报告上下文增长，不修改
2. **Tool-Result Snip** (0.6)：旧工具输出截断为头尾摘要
3. **Prune** (0.8)：旧工具结果替换为占位符
4. **Force Fold** (0.9)：强制摘要压缩

关键设计：
- **用户 turn 原文保留**：短用户消息永不摘要
- **已有摘要不二次摘要**：避免漂移
- **结构化摘要**：Standing facts / Goal / Decisions / Files / Commands / Errors / Next step
- **JSONL 归档**：被压缩的原始内容归档保存

**Mady 对比：** 当前 `compaction.go` 是单级摘要压缩，没有渐进式管线。所有工具输出要么保留要么摘要，没有中间态。

**引入价值：** 专利分析对话往往很长（多轮检索 + 分析 + 答复），渐进式压缩可以在不丢失关键上下文的前提下显著延长对话寿命。

**建议落地方案：**
- 扩展现有 `compaction.go` 增加 snip 和 prune 阶段
- 在 `context_engine.go` 中添加分级触发逻辑
- 保留现有的中文摘要 Prompt（已经很完善），增加工具结果截断逻辑

---

#### 6. 📋 Plan Mode（计划模式 — 只读研究 → 用户审批 → 执行）

**Reasonix 实现：** `internal/planmode/policy.go` (~609 行)

**核心思路：**
- 计划阶段只允许只读工具 + `ask` + `todo_write`
- 工具三级信任：内置 > 第一方 MCP > 不受信任的 MCP
- Bash 命令安全分类：自动识别只读命令 vs 写命令
- 计划结构化：分阶段（phases）→ 子步骤（sub-steps）
- 用户审批后才进入执行阶段

**Mady 对比：** 已有 `/plan` 计划模式（最近提交 `d8d0ff9`），但没有工具级别的安全门控。

**引入价值：** 在专利撰写、法律分析等高 stakes 场景，用户需要先看到完整计划再批准执行。Plan Mode 的工具安全分类可以防止"计划阶段意外执行了写操作"。

**建议落地方案：**
- 扩展现有 `/plan` 模式，增加 `PlanSafety` 接口
- 为 `tools/` 下的工具添加 `ReadOnly()` 和 `PlanModeSafe()` 方法
- 实现 Bash 命令只读分类器（参考 `internal/shellsafe/`）
- 计划审批后开启"approved-plan execution window"

---

#### 7. 🔐 Permission System（细粒度权限门控）

**Reasonix 实现：** `internal/permission/` (~500+ 行)

**核心思路：**
- 每次工具调用独立决策：Allow / Ask / Deny
- 规则语法：`Tool(specifier)` 匹配（如 `Bash(go test:*)`、`Edit(docs/**)`）
- 优先级：deny > ask > allow > fallback
- 三种姿态模式：ask（需批准）、auto（自动批准写工具）、yolo（全部自动）
- 非交互模式：Ask → Allow（保持自主性）
- Deny 硬阻断：工具永不执行，模型收到 "blocked" 结果

**Mady 对比：** 没有细粒度权限系统。工具调用的安全完全依赖 Handoff 白名单和护栏关键词。

**引入价值：** 专利/法律场景中，某些操作（如发送邮件、提交官方文件、删除案件文档）需要用户确认。权限系统可以在工具调用级别实现"需要确认"的语义。

---

### P2 — 中期引入（架构增强）

#### 8. 🤖 Two-Model Coordinator（双模型协作）

**Reasonix 实现：** `internal/agent/coordinator.go` (~300+ 行)

**核心思路：**
- **Planner**（低频）：独立会话，只读工具，产出结构化计划
- **Executor**（高频）：独立会话，完整工具，执行计划
- 两个会话永不混合 → 各自 prefix cache 独立保持稳定
- Planner 可以请求用户审批 (`[planner_requires_approval]`)
- Planner 可以提出用户决策问题 (`<planner-ask>`)

**Mady 对比：** 有 `ReasoningRouter` 做复杂度分级，但没有双模型分离。高质量推理模型（如 plan 模式切换的 GLM）和执行模型共享同一会话。

**引入价值：** 专利分析中，检索策略规划（需要深度推理）和文书执行（需要高效执行）可以用不同模型分离，既保证质量又控制成本。且两个会话独立缓存，避免互相污染。

**建议落地方案：**
- 扩展 `ReasoningRouter` → 支持 `Coordinator` 模式
- Planner 使用高质量模型（如 GLM-5.1），Executor 使用快速模型
- 适配为专利场景：Planner 规划检索策略/答复框架，Executor 执行检索/撰写

---

#### 9. 📊 Tool Contract + Schema Canonicalization（工具契约管理）

**Reasonix 实现：** `internal/tool/contract.go` + `docs/TOOL_CONTRACT.md`

**核心思路：**
- 所有内置工具的 Schema 被文档化，作为回归测试的"契约"
- Schema 在注册时自动规范化（canonicalize）
- 测试比对文档化 Schema 与实际代码生成的一致性
- 新工具不能绕过审查（reconcile test 强制分类）

**Mady 缺失：** 工具 Schema 没有契约化管理，Schema 变更无回归保护。

**引入价值：** Mady 有 20+ 工具，Schema 变更可能导致模型调用失败。契约化可以保证工具接口稳定性。

---

#### 10. 🔀 Parallel Tasks（并行子 Agent 调度）

**Reasonix 实现：** `internal/agent/parallel_tasks.go` (~200+ 行)

**核心思路：**
- 并发分发多个只读子 Agent 任务
- 每个子任务有独立事件流、独立工具白名单
- 结果汇总后返回
- 单任务时拒绝（引导使用 `task` 工具）

**Mady 对比：** 有 `domains/agent_pool.go`，但没有并行任务调度工具。

**引入价值：** 专利分析中常需要并行检索多个技术特征（如独立权利要求的各技术特征分别检索），并行子 Agent 可以显著提升效率。

---

#### 11. 🔍 AutoResearch Protocol（自动研究协议）

**Reasonix 实现：** `internal/autoresearch/` (~400+ 行)

**核心思路：**
- 长周期研究任务的状态机管理
- 任务契约：Context / Request / Output / Constraints / Pause policy
- 成功标准追踪（SuccessCriterion + Evidence）
- 方向追踪：stale detection（重复方向检测）、pivot counting
- Heartbeat 监控
- 项目本地状态存储（不污染系统 Prompt）

**Mady 对比：** 有 `workflows/` 但没有自动研究的协议层。

**引入价值：** 专利无效宣告检索是典型的长周期研究任务（多轮检索 → 分析 → 策略调整），AutoResearch 可以管理整个流程的状态和进度。

---

### P3 — 长期引入（差异化竞争力）

#### 12. 💬 IM Bot Integration（即时通讯机器人集成）

**Reasonix 实现：** `internal/bot/` (Feishu / WeChat / QQ)

**核心思路：**
- 多 IM 平台统一接入（飞书、企业微信、QQ）
- 从 IM 发起审批、YOLO 执行、斜杠命令
- 桌面应用 ↔ Bot 桥接
- 配对认证（pairing）

**引入价值：** 专利代理人/律师在外出时可通过飞书/微信与 Agent 交互，审批操作、查看分析结果。

---

#### 13. 🖥️ Desktop Application（Wails 桌面应用）

**Reasonix 实现：** `desktop/` (Wails + React/TS 前端)

**核心思路：**
- Wails 桌面应用封装（macOS/Windows/Linux）
- 前端 React + TypeScript
- 多 Tab 会话管理
- 系统托盘、自动更新、崩溃恢复
- 心跳看门狗

**引入价值：** 为 Mady 提供桌面应用形态，降低非技术用户（专利代理人、律师）的使用门槛。

---

#### 14. 🌐 Cache-First Architecture（缓存优先架构）

**Reasonix 核心设计哲学：**
- System prompt 前缀跨 turn 保持字节稳定 → 最大化 prefix cache 命中
- 记忆/技能索引只放名称+描述（body 按需加载）
- 压缩是唯一的"缓存重置点"
- 环境摘要注入稳定前缀
- 双模型使用独立会话保持各自缓存

**引入价值：** Mady 在长对话（专利分析常达 50+ 轮）中，缓存命中率直接影响响应速度和成本。

---

## 三、引入优先级矩阵

```
高价值
  │
  │   ┌─────────────────────────────────┐
  │   │ Evidence Ledger (P0)            │ ← 安全审计刚需
  │   │ Guardian AI 审查 (P0)           │ ← 语义安全升级
P0│   │ 文件 Checkpoint (P0)            │ ← 文书编辑安全网
  │   └─────────────────────────────────┘
  │   ┌─────────────────────────────────┐
  │   │ Memory Compiler (P1)            │ ← 策略学习能力
  │   │ 四级压缩管线 (P1)               │ ← 长对话支持
  │   │ Plan Mode 门控 (P1)             │ ← 计划安全执行
P1│   │ Permission System (P1)          │ ← 细粒度权限
  │   └─────────────────────────────────┘
  │   ┌─────────────────────────────────┐
  │   │ 双模型协作 (P2)                 │ ← 推理/执行分离
  │   │ Tool Contract (P2)              │ ← 接口稳定性
P2│   │ Parallel Tasks (P2)             │ ← 并行效率
  │   │ AutoResearch (P2)               │ ← 长周期任务
  │   └─────────────────────────────────┘
  │   ┌─────────────────────────────────┐
P3│   │ IM Bot 集成 (P3)                │ ← 移动办公
  │   │ Desktop App (P3)                │ ← 桌面形态
  │   │ Cache-First (P3)                │ ← 性能优化
  │   └─────────────────────────────────┘
  │
  └────────────────────────────────────── 低价值
     低难度                           高难度
```

---

## 四、具体实施建议

### 第一批（1-2 周）

| 特性 | 涉及文件 | 工作量 |
|------|---------|--------|
| Evidence Ledger | 新建 `agentcore/evidence/` | ~500 行 |
| 文件 Checkpoint | 新建 `agentcore/filecheckpoint/` | ~300 行 |
| Tool ReadOnly 接口 | 修改 `agentcore/tool.go` + `tools/*.go` | ~200 行 |

### 第二批（2-4 周）

| 特性 | 涉及文件 | 工作量 |
|------|---------|--------|
| Guardian AI 审查 | 新建 `guardrails/guardian/` | ~800 行 |
| 四级压缩管线 | 扩展 `agentcore/compaction.go` | ~400 行 |
| Plan Mode 门控 | 扩展 `agentcore/` + 新建 `agentcore/planmode/` | ~600 行 |

### 第三批（4-8 周）

| 特性 | 涉及文件 | 工作量 |
|------|---------|--------|
| Permission System | 新建 `agentcore/permission/` | ~800 行 |
| Memory Compiler v5 | 扩展 `memory/` | ~2000 行 |
| 双模型协作 | 扩展 `agentcore/reasoning_router.go` | ~500 行 |

---

## 五、不建议引入的部分

| 特性 | 原因 |
|------|------|
| 沙箱系统 (Seatbelt/bubblewrap) | Mady 不是编程 Agent，文件操作范围可控 |
| Slash 命令系统 | Mady TUI 已有自己的命令系统 |
| `.mcp.json` 兼容 | Mady MCP 客户端已独立实现 |
| Billing 模块 | 非核心功能，按需实现 |
| 系统代理 (sysproxy) | 非核心功能 |
| Windows 沙箱 | 不适用 |

---

## 六、Reasonix 的关键设计启示

### 1. "Cache-First" 应成为架构原则
Reasonix 的所有设计决策都围绕"prefix cache 稳定性"展开。Mady 应在架构文档中明确：
> **系统 Prompt 前缀（基础提示 + 工具 Schema + 记忆索引）必须跨 turn 保持字节稳定。任何需要变更前缀的操作（压缩、记忆更新）必须是低频的、刻意的"缓存重置点"。**

### 2. "Fail-Closed" 应成为安全默认值
Reasonix 的 Guardian、Plan Mode、Permission 都遵循 fail-closed 原则——不确定时拒绝，而不是放行。Mady 在专利/法律领域更应如此。

### 3. "Separation of Concerns" 应贯穿安全层
Reasonix 将以下安全关注点完全分离：
- Plan Mode（执行阶段门控）
- Permission（工具调用门控）
- Tool Approval Posture（用户姿态）
- Guardian（AI 语义审查）
- Sandbox（OS 级隔离）

Mady 当前将这些混在 `guardrails/` 中，应考虑分层。

### 4. "Evidence-Based Execution" 应成为验证范式
Reasonix 的 Evidence Ledger 不信任模型的自我报告——它通过工具调用收据验证模型声称的每个步骤。这种"用证据验证声明"的范式在专利/法律领域尤为重要。

---

*本报告基于对 Reasonix v2 全部 `internal/` 模块的代码级分析。*
