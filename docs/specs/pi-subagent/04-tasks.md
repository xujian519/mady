# 04 — 任务拆解：基于 sky-valley/pi 的动态子会话（辅助智能体）

- **功能名**：pi-subagent
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **拆解日期**：2026-08-13
- **状态**：待 Sign-off（未开始）
- **依赖设计**：[03-design.md](./03-design.md)

> 每个任务标注：**涉及文件范围**、**验收**、**风险等级**、**审查要求**。
> 遵循 AGENTS.md「单次改动 3-5 文件」「小炸弹不是大炸弹」原则，任务粒度对应一次提交。
> 前置条件：01-proposal / 02-spec / 03-design 人工 Sign-off；Owner 完成 02-spec §4 开放问题决策。

---

## 阶段 1：MVP（本期范围）

**阶段目标**：`spawn_agent` 工具 + explore/verify 只读预设，pi 会话嵌入 tools 子模块，AC-1..7 全过。

### T1.1 — tools 子模块引入 pi 依赖并核验传递依赖树

- **文件**：`tools/go.mod`、`tools/go.sum`；新增 `tools/piagent/doc.go`
- **内容**：
  - `cd tools && go get github.com/sky-valley/pi@v0.84.17`
  - `go mod graph | grep sky-valley` 核验传递依赖仅 `golang.org/x/image`、`golang.org/x/text`（或其他，如实记录）
  - `tools/piagent/doc.go`：包说明（用途、安全不变量、预设语义对照 Sati）
- **验收**：`cd tools && go build ./...` 通过；依赖树清单记录在 PR 描述
- **风险**：低（纯依赖引入）| **审查**：L1

### T1.2 — 桥接层：Mady Tool → pi AgentTool 转换

- **文件**：`tools/piagent/bridge.go`、`tools/piagent/bridge_test.go`（新增，~250 行）
- **内容**：
  - `ToAgentTool(t *agentcore.Tool) (agent.AgentTool, error)`：Name/Description/Parameters（JSON Schema → `ai.Schema`）转换
  - `Execute` 包装：permission → 沙箱 → 只读判定 → Mady 工具实现（依赖注入工具执行函数，避免直接 import tools 造成循环依赖）
  - Schema 不兼容字段降级：跳过 + WARN（03-design §1.4）
- **验收**：正/反向转换单测通过；不兼容 schema 返回明确错误；V-4/V-5 权限与沙箱断言通过
- **风险**：中（schema 兼容面）| **审查**：L2

### T1.3 — 预设定义（explore/verify/plan/general-purpose）

- **文件**：`tools/piagent/presets.go`、`tools/piagent/presets_test.go`（新增，~200 行）
- **内容**：
  - `Preset` 结构：`Name` / `AllowedTools`（Mady 工具名白名单）/ `IsReadOnly` / `SystemPromptSuffix` / `DefaultModel` / `DefaultThinking`
  - 只读判定辅助：`isWriteTool(name)`（写类工具清单 + domain 判定，复用 `tool_domains.go` 语义）
  - 白名单集合运算 `ResolveAllowed(preset, tools, exclude)`（03-design §4.1）
- **验收**：预设语义单测（explore 无写工具；白名单集合运算边界用例）；V-2 断言通过
- **风险**：低 | **审查**：L1

### T1.4 — `spawn_agent` 工具实现与注册

- **文件**：`tools/piagent/session.go`（新增）、`tools/tools.go`（注册，敏感路径）
- **内容**：
  - `session.go`：`RunSpawn(ctx, params)` → 解析参数 → `coding.NewSession(SessionOptions{Model, APIKey, ToolNames, ExcludeTools, CustomTools, ThinkingLevel, Compaction, TimeoutMs})` → `Session.Run` → 报告聚合（03-design §4.3）
  - `tools/tools.go`：`spawn_agent` 加入 `BuildTools`（tools 扩展机制），参数 schema 按 02-spec §2.2
  - 嵌套禁用：子会话注册表不含 `spawn_agent`
- **验收**：工具在 `mady tui` 可被父 Agent 调用（手动冒烟）；V-7 降级（无 Key 明确报错）V-8 超时通过
- **风险**：高（触及 `tools/tools.go` 门控）| **审查**：L3（敏感路径，人工审阅）

### T1.5 — faux-provider 集成测试（AC-1..7）

- **文件**：`tools/piagent/integration_test.go`（新增，~300 行）
- **内容**：
  - 用 pi `ai/providers/faux`（确定性测试替身）跑真实 `Session.Run`：
    - AC-1：explore 派发返回结构化报告
    - AC-2：只读预设调用写工具被拒（断言无副作用 + 错误结果）
    - AC-3：`tools`/`exclude_tools` 白名单生效
    - AC-4：子会话运行后父 Agent `State().Messages` 不变（上下文隔离）
    - AC-6：`RunResult.Usage` 非零
  - 安全用例：沙箱越界返回错误；Deny 域拦截
- **验收**：AC-1..7 对应断言全绿；`cd tools && go test -race ./piagent/...` 通过
- **风险**：中 | **审查**：L2

### T1.6 — 文档同步 + AI changelog

- **文件**：`README.md`（工具表补 `spawn_agent`）、`docs/specs/README.md`（索引表）、AI changelog（脚本追加）
- **内容**：
  - README 工具表：`spawn_agent — 派发受限辅助智能体（pi 子会话）`
  - specs 索引：`pi-subagent` 行（阶段：实现中）
  - `go run scripts/changelog/main.go --type=feat --scope=tools --title="..." --body="..."`（按 AGENTS.md）
- **验收**：三处文档一致；changelog 索引更新
- **风险**：低（纯文档）| **审查**：L1

---

## 阶段 2：扩展（后续迭代，不在本期）

| 任务 | 内容 | 前置 |
|------|------|------|
| T2.1 | `plan`/`general-purpose` 预设上线 + `spawn_agent.model` 走 `ResolveModel` + 多 provider Key 解析 | 阶段 1 全绿 |
| T2.2 | `thinking` 参数 ↔ `agent.ThinkingLevel` 映射 + 按模型 clamp | T2.1 |
| T2.3 | pi 内置 coding 工具白名单评估（bash 沙箱对策） | Owner 决策 |

---

## 阶段 3：生产化（后续迭代，不在本期）

| 任务 | 内容 | 前置 |
|------|------|------|
| T3.1 | pi `agent.Listener` → Mady EventBus/agentadapter 事件桥接（TUI 可见子会话活动） | 阶段 2 |
| T3.2 | 三级护栏/evidence 账本挂到子会话工具调用 | T3.1 |
| T3.3 | 子会话 token 预算继承（父预算拆分）+ JSONL 会话留存开关 | T3.2 |

---

## 验收清单（Sign-off 用）

- [ ] 01-proposal / 02-spec / 03-design 人工审阅通过，02-spec §4 开放问题已决策
- [ ] T1.1..T1.6 全部完成，`make verify` 全绿（lint + build + race 测试覆盖根模块与 tools 子模块）
- [ ] AC-1..7 全部可复现（集成测试 + 手动冒烟）
- [ ] `tools/tools.go` 改动经 L3 人工审阅并记录
- [ ] README / specs 索引 / AI changelog 三处一致
