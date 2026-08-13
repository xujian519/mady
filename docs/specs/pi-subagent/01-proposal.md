# 01 — 提案：基于 sky-valley/pi 的动态子会话（辅助智能体）

- **功能名**：pi-subagent
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **提案日期**：2026-08-13
- **状态**：待人工 Sign-off
- **参考架构**：Sati（opencode 系）子代理体系 `src/agent/sub/`

---

## 1. 背景

### 1.1 现状：Mady 缺少"动态子会话"能力

Mady 现有的子任务机制全部是**同一上下文内**或**构建期静态配置**的，缺少运行期按需派生的**独立子会话**：

| 机制 | 位置 | 上下文 | 派发方式 |
|------|------|--------|----------|
| Handoff（transfer_to_patent/legal） | `agentcore/handoff.go` + `domains/unified.go` | 子 Agent 独立会话 | **构建期固定目标**，运行期不可按需派生 |
| Orchestration（OrchestrationManifest） | `agentcore/orchestration*.go` | **同一上下文**多步执行 | 预编排清单，非动态派发 |
| Worker（并行工具执行） | `agentcore/worker/` | **同一上下文**并行工具 | 无独立系统提示词/上下文裁剪 |
| TaskList（结构化任务） | `agentcore/tasklist/` | 任务清单 | 无子会话 |

结果：父 Agent 无法运行期派生一个"只读探查"或"定向核验"的子会话——这类能力在 Sati 中是核心特性（`agent` 工具 + `builtinSubagentTypes`：general-purpose / explore / plan / verify）。

### 1.2 参照：Sati 的子代理架构

Sati（opencode 系）的辅助智能体 = 同一运行时内的动态子会话（`src/agent/sub/`）：

| 特性 | Sati 实现 |
|------|-----------|
| 派发 | 父 Agent 的 `agent` 工具运行期调用，`subagent_type` 选预设 |
| 预设 | `builtinSubagentTypes.ts`：general-purpose / explore / plan / verify |
| 工具裁剪 | `allowedTools` 白名单 + `visibleDomains`/`hiddenDomains` 域裁剪 + `omitTools` |
| 只读强制 | `isReadOnly`：只读预设直接拒绝破坏性工具 |
| 上下文 | `contextInheritance` 继承父消息 + 独立子上下文；可省略 `<project-instructions>`/`<git-status>` |
| 推理强度 | `effort` 覆盖（S12） |
| 回传 | 强制结构化报告（Scope / Result / Key files / Files changed / Issues，<4KB） |

### 1.3 候选：sky-valley/pi（纯 Go 智能体框架）

`github.com/sky-valley/pi`（v0.84.17）是 [earendil-works/pi](https://github.com/earendil-works/pi) 的纯 Go 移植：

| 包 | 内容 | 与 Mady 的契合点 |
|----|------|------------------|
| `ai` | 统一多 provider LLM API（faux/Anthropic/OpenAI Chat+Responses/Google Gemini）+ 999 模型目录 + JSON-Schema 工具校验 + 成本统计 | 补 Mady 原生 Anthropic/Google 接入 |
| `agent` | `AgentLoop` + 状态化 `Agent`（工具调用、hooks、steering、顺序/并行工具执行）+ `AgentTool` 接口 | 子会话运行时 |
| `coding` | 编码 Agent（7 内置工具 read/write/edit/bash/ls/find/grep）+ 系统提示词构建（自动折叠 AGENTS.md/CLAUDE.md + Agent Skills）+ 会话运行器 + JSONL 持久化 + compaction | 现成编码循环，父上下文隔离 |

契合度核验（已确认）：
- **语言/版本**：纯 Go，`go 1.26` 与 Mady 完全一致，无 Node 运行时
- **依赖面**：仅 `golang.org/x/image` + `golang.org/x/text`，对 Mady「核心依赖极少」约定友好
- **规模**：agent + coding 两包约 22.8K 行，库级嵌入成本可控
- **SDK 形态**：`coding.NewSession` + `Subscribe` 流式事件 + `RunResult` 结构化回传，可直接作为子会话运行时

### 1.4 为什么现在做

1. 专利/法律分析常有"先探查再结论"的工作模式（探索代码库/文档 → 定向核验 → 汇报），动态子会话是通用能力，收益面广
2. Mady 是 Go 项目，pi 同为 Go 且版本一致，**库嵌入零异构成本**（对比 Sati 需进程隔离的方案）
3. Mady 已有 `tool_domains.go`（FilterToolNames/ToolHasDomain）与 pi 的 `ToolNames` 白名单天然同构，桥接成本低

---

## 2. 目标

### 2.1 总目标

在 Mady 工具层新增 `spawn_agent` 工具，父 Agent（mady-agent）运行期按需派发 **pi 驱动的独立子会话**（explore / verify / plan / general-purpose 预设），实现 Sati 式的动态子代理能力：独立上下文、工具白名单/域裁剪、只读强制、结构化报告回传、权限与沙箱继承。

### 2.2 阶段目标（本期覆盖阶段 1）

| 阶段 | 目标 | 一句话验收 |
|------|------|-----------|
| **阶段 1：MVP** | `spawn_agent` 工具 + explore/verify 只读预设，pi 会话嵌入 Mady 工具层，`tool_domains` → `ToolNames` 桥接 | 父 Agent 可派发只读子会话探查文件/核验结论，返回结构化报告，写工具被拒 |
| **阶段 2：扩展** | plan/general-purpose 预设 + 多模型选择（`ResolveModel`）+ `ThinkingLevel` 映射 | 子会话可按任务选模型与推理档位 |
| **阶段 3：生产化** | 三级护栏/evidence/预算继承 + pi 事件流对接 EventBus/agentadapter | 子会话工具调用走 Mady permission 与沙箱，事件在 TUI 可见 |

> 阶段 2、3 **不在本期**，作为后续迭代候选，但设计中预留接口。

### 2.3 非目标（本期不做）

- **不替换** Mady agentcore 主循环（pi 仅作子会话运行时，父 Agent 仍走 `UnifiedAgentConfig`）
- **不引入** pi 作为 provider 层替代（`provider/` + chatcompat 维持现状，pi provider 仅用于子会话）
- **不做** 进程外/子进程部署（纯库嵌入）
- **不暴露** pi 的 coding 7 内置工具给父 Agent（仅存在于子会话白名单内）
- **不实现** Sati 的 `visibleDomains`/`hiddenDomains` 域裁剪语义（Mady 已有 `tool_domains.go`，直接用其机制）

---

## 3. 成功标准

### 3.1 功能验收

| 编号 | 标准 | 验证方式 |
|------|------|----------|
| AC-1 | `mady tui` 中父 Agent 可调用 `spawn_agent` 派发 explore 子会话完成只读探查，返回结构化报告（Scope/Result/Key files/Files changed/Issues） | 集成测试 |
| AC-2 | explore/verify 子会话调用写类工具（write/edit/delete/bash 写命令）被拒绝并说明原因 | 集成测试 |
| AC-3 | 子会话工具白名单可配置：`spawn_agent` 参数 `tools` 或预设决定可见工具 | 单元测试 |
| AC-4 | 父上下文不被子会话的工具结果污染（子会话上下文独立） | 集成测试断言父消息数不变 |
| AC-5 | 子会话工具调用继承 Mady permission（Allow/Ask/Deny）与 WorkingDir 沙箱，不绕过红线 | 安全测试 |
| AC-6 | 子会话返回 token 用量（Usage），可计入父会话观测 | 断言 RunResult.Usage 非零 |
| AC-7 | pi 未配置 API Key / provider 不可用时，`spawn_agent` 返回明确错误而非 panic | 单元测试 |

### 3.2 质量验收

- `go build ./...` / `go vet ./...` / `go test -race ./...` 全绿（含 tools 子模块）
- `golangci-lint run` 零 issue
- 新增代码符合分层架构（领域层不 import 基础设施实现，见 ADR-0001）
- 不引入硬编码密钥；API Key 走既有环境变量/agentconfig 通道
- 不破坏现有 Handoff / orchestration / worker / tasklist 行为（回归）

### 3.3 回归红线

- `agentcore/` 公开 API 不做破坏性变更（只新增）
- 敏感路径 `tools/tools.go`（工具能力门控）改动需人工审阅（L3）
- pi 依赖版本锁 pin（go.mod + go.sum），升级走单独 PR
- 不引入 cwd 相对路径默认值（遵循 AGENTS.md 资源定位约定）

---

## 4. 关键约束

1. **纯库嵌入**：pi 以 Go module 依赖引入，不做子进程包装
2. **子会话不绕过安全层**：所有子会话工具调用经 Mady permission + 沙箱包装后执行
3. **父循环不动**：agentcore 主循环、UnifiedAgentConfig、Handoff 均不改动
4. **依赖克制**：pi 引入带来的传递依赖（x/image、x/text）需评估 vendor 策略
5. **敏感路径**：`tools/tools.go`、`agentcore/permission/` 受影响时需人工审阅（L3）

---

## 5. 决策摘要（详见 03-design.md）

| 决策点 | 选择 | 备选 | 理由 |
|--------|------|------|------|
| 子会话运行时 | pi `coding.NewSession`（库嵌入） | Mady 自建子会话 / 子进程 | 现成编码循环 + 多 provider + compaction，零异构成本 |
| 派发方式 | 新工具 `spawn_agent`（tools 层注册） | Handoff 扩展 | 对齐 Sati `agent` 工具语义；不触碰 Handoff 安全红线 |
| 工具门控 | pi `ToolNames`/`ExcludeTools` + Mady `tool_domains.FilterToolNames` | 仅预设硬编码 | 双保险：预设白名单 + Mady 域机制 |
| 只读强制 | 桥接层包装：只读预设拒绝写类工具 | 依赖 pi isReadOnly 语义 | Mady 侧强制，防 pi 内置工具绕过 |
| 模型来源 | pi `ResolveModel` + 既有 agentconfig 环境变量 | 新增配置面 | 阶段 1 仅默认模型，阶段 2 开放 |
| 事件流 | pi `Subscribe` → Mady EventBus（阶段 3） | 忽略（阶段 1 黑盒） | 阶段 1 收敛范围，预留接口 |

---

## 6. 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| pi 依赖面膨胀（x/image、x/text 及潜在间接依赖） | 中 | 锁 pin 版本；vendor 前评估传递依赖树；如膨胀过大退回子进程方案 |
| pi `coding` 内置工具行为与 Mady 工具不一致（如 bash 沙箱） | 中 | 子会话白名单默认仅注入 Mady 工具桥接；内置工具按需启用 |
| 双 agent 循环语义重叠（Mady agentcore vs pi loop） | 中 | 明确职责边界：父循环管编排/护栏/权限，子循环管子任务内部工具迭代 |
| 子会话绕过 Mady 沙箱 | 高 | 桥接层强制包装：所有 AgentTool 经 permission + WorkingDir 校验后才执行（AC-5） |
| pi 事件模型与 Mady EventBus 不匹配 | 低 | 阶段 1 不透传事件；阶段 3 用适配器映射 `agent.Listener` |
| 双份 token 成本（父 + 子） | 中 | 子会话启用 pi `DefaultCompactionSettings`；阶段 3 接入预算继承 |

---

## 7. 下一步

人工 Sign-off 本提案后，进入 `02-spec.md`（详细规格）。规格中标记 `[NEEDS CLARIFICATION]` 的点需 Owner 决策后方可进入实现。
