# 02 — 规格：基于 sky-valley/pi 的动态子会话（辅助智能体）

- **功能名**：pi-subagent
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **规格日期**：2026-08-13
- **状态**：待人工审阅
- **依赖提案**：[01-proposal.md](./01-proposal.md)

---

## 1. 术语

| 术语 | 含义 |
|------|------|
| 父 Agent | mady-agent（`UnifiedAgentConfig`），拥有 `spawn_agent` 工具 |
| 子会话 | pi `coding.Session` 实例，独立上下文、独立 token 预算 |
| 预设 | 子会话角色定义（explore/verify/plan/general-purpose），决定工具白名单与系统提示词后缀 |
| 桥接层 | `tools/piagent`（拟新增包）：Mady `agentcore.Tool` ↔ pi `agent.AgentTool` 双向转换 + 安全包装 |

---

## 2. 数据模型与接口

### 2.1 依赖引入

- `go get github.com/sky-valley/pi@v0.84.17`（锁 pin）
- 传递依赖核验：`golang.org/x/image`、`golang.org/x/text`（需 `go mod graph` 确认无其他新增）
- **[NEEDS CLARIFICATION: 是否接受 pi 传递依赖进入根模块 go.mod？若不接受，需评估子模块/子进程替代方案]**

### 2.2 `spawn_agent` 工具定义（tools 层新增）

**名称**：`spawn_agent`
**描述**：派发一个受限的辅助智能体（子会话）完成定向探查/核验/规划任务，返回结构化报告。

```jsonc
// 参数 schema（JSON Schema 草案 2020-12，经 agentcore 工具 schema 机制注册）
{
  "type": "object",
  "properties": {
    "subagent_type": {
      "type": "string",
      "enum": ["explore", "verify", "plan", "general-purpose"],
      "description": "子会话预设，决定工具白名单与系统提示词后缀"
    },
    "directive": {
      "type": "string",
      "description": "定向任务指令（父 Agent 生成，子会话的 System Prompt 主体）"
    },
    "tools": {
      "type": "array",
      "items": { "type": "string" },
      "description": "可选，追加到预设白名单的 Mady 工具名；缺省 = 仅预设白名单"
    },
    "exclude_tools": {
      "type": "array",
      "items": { "type": "string" },
      "description": "可选，从生效白名单中剔除的工具名"
    },
    "model": {
      "type": "string",
      "description": "可选，pi 模型规格（如 anthropic/claude-sonnet-4-5）；缺省 = 预设默认模型"
    },
    "thinking": {
      "type": "string",
      "enum": ["off", "minimal", "low", "medium", "high", "xhigh"],
      "description": "可选，子会话推理档位；缺省 = medium"
    },
    "max_tokens": { "type": "integer", "minimum": 256, "description": "可选，子会话单轮上限" }
  },
  "required": ["subagent_type", "directive"]
}
```

**返回**（结构化对象，非纯文本）：

```jsonc
{
  "success": true,
  "report": {
    "scope": "本次做了什么",
    "result": "发现/结论（Markdown）",
    "key_files": ["绝对路径"],
    "files_changed": ["改动文件+理由，或空"],
    "issues": ["注意点/阻塞项，或空"]
  },
  "usage": { "input_tokens": 0, "output_tokens": 0, "cost_usd": 0.0 },
  "stop_reason": "end_turn | max_tokens | tool_use | error",
  "error": ""  // success=false 时的原因
}
```

### 2.3 子会话预设（对齐 Sati `builtinSubagentTypes`）

| 预设 | 工具白名单（Mady 工具名） | 只读 | 系统提示词后缀要点 |
|------|---------------------------|------|--------------------|
| `explore` | 只读探查：read_file / grep / glob / find（Mady 工具层对应名）；禁写 | ✅ | 只读模式说明；优先专用搜索工具；不提议写/网络 |
| `verify` | 只读核验：read_file / grep / glob / bash（只读命令） | ✅ | 核验产物并报告问题；不修改文件 |
| `plan` | 只读规划：read_file / grep / glob；无 bash | ✅ | 产出逐步执行计划；不执行 |
| `general-purpose` | 父工具注册表全量（除嵌套 `spawn_agent`） | ❌ | 有权限工具全可用，但限于指令范围 |

**[NEEDS CLARIFICATION: 预设默认模型与默认 API Key 来源——是否沿用 Mady agentconfig 的默认 Provider/Model，还是 pi 侧独立解析（ANTHROPIC_API_KEY 等）？]**

### 2.4 工具桥接（Mady `agentcore.Tool` → pi `agent.AgentTool`）

转换规则（每字段映射）：

| Mady `agentcore.Tool` | pi `agent.AgentTool` | 说明 |
|----------------------|---------------------|------|
| `Name` / `Description` | `Name` / `Description` | 直通 |
| `Parameters`（JSON Schema） | `Parameters *ai.Schema` | Schema 结构转换（需核验 ai.Schema 字段兼容性） |
| 执行函数 | `Execute` | 包装：**先过 Mady permission + WorkingDir 沙箱校验**，再执行 Mady 工具实现 |
| — | `ExecutionMode` | 默认并行；写类工具强制顺序 |
| — | `PromptGuidelines` | 由工具 domain 元数据生成（若 Mady 工具带 domain） |

**双向不变量**：
1. 子会话内所有工具执行结果统一为 pi `AgentToolResult`（Content 为文本/图片列表）
2. 只读预设的桥接层拒绝写类工具（AC-2）：在执行前按工具 domain/名称判定，返回错误结果
3. 嵌套禁用：子会话工具注册表不包含 `spawn_agent`（对齐 Sati 禁止嵌套派发）

### 2.5 事件流（阶段 3 预留接口）

- pi `agent.Listener`（`Subscribe(func(ctx, AgentEvent) error)`）→ Mady `agentcore` EventBus
- 事件类型映射：`EvText` → `EventMessage`（增量）、`EvToolExecuting`/`EvToolResult` → 工具事件、`EvMessageEnd` → 消息完成
- 阶段 1 不接线，仅在桥接层暴露 `Subscribe` 透传供测试断言（AC-6 用量断言走 `RunResult.Usage`）

### 2.6 生命周期

```
父 Agent 调用 spawn_agent(subagent_type, directive)
  → 桥接层解析预设（工具白名单 + 系统提示词后缀）
  → 白名单 = (预设 ∪ tools) − exclude_tools，再经 tool_domains.FilterToolNames 校验
  → 构建 pi coding.NewSession(SessionOptions{Model, APIKey, ToolNames, ExcludeTools,
       CustomTools: 桥接后的 Mady 工具, ThinkingLevel, Compaction})
  → Session.Run(ctx, directive)  ← 超时控制（默认 120s，可配）
  → 汇总 RunResult → 结构化返回给父 Agent
  → 会话丢弃（不持久化；阶段 3 再评估 JSONL 留存）
```

---

## 3. 验证规则

### 3.1 功能验证（对应 01-proposal AC-1..7）

| 编号 | 规则 | 断言 |
|------|------|------|
| V-1 | explore 只读 | 调用 write/edit/delete/bash 写命令 → 返回错误结果，不执行副作用 |
| V-2 | 白名单生效 | `tools`/`exclude_tools` 合并结果与预设语义一致（集合运算断言） |
| V-3 | 上下文隔离 | 子会话运行后父 Agent `State().Messages` 长度不变 |
| V-4 | 权限继承 | 桥接层拦截 Deny 域工具并返回「权限拒绝」结果 |
| V-5 | 沙箱继承 | 桥接层用 Mady `tools/path.go` 解析路径，越界路径返回错误 |
| V-6 | 用量上报 | `RunResult.Usage` 非零且含 cost |
| V-7 | 降级 | pi provider 无 Key / 模型解析失败 → 返回明确错误（不 panic） |
| V-8 | 超时 | 子会话超时（ctx deadline）→ 返回超时错误，无泄漏 goroutine |

### 3.2 质量验证

- 根模块与 tools 子模块：`go build ./...` / `go vet ./...` / `go test -race ./...`
- `golangci-lint run` 零 issue
- 新增包测试覆盖：桥接转换（正/反向）、白名单集合运算、只读拒绝、超时、降级

### 3.3 回归红线

- `agentcore/` 公开 API 无破坏性变更
- `tools/tools.go` 改动（注册 `spawn_agent`）触发 L3 人工审阅
- 不引入 cwd 相对路径默认值
- pi 依赖 pin `v0.84.17`，升级另开 PR

---

## 4. 开放问题（需 Owner 决策）

| # | 问题 | 影响 | 建议默认 |
|---|------|------|----------|
| Q1 | pi 传递依赖进入根模块 go.mod？ | 依赖面 | 接受（x/image、x/text 均为 x/ 系，轻量） |
| Q2 | 预设默认模型/Key 来源 | 行为 | 阶段 1 用 Mady agentconfig 默认 Provider/Model + 其 API Key 通道 |
| Q3 | 子会话超时默认值 | 体验 | 120s，`SPAWN_AGENT_TIMEOUT` 环境变量可覆盖 |
| Q4 | 子会话是否持久化（JSONL） | 排障 | 阶段 1 不持久化；阶段 3 按需 |
| Q5 | `spawn_agent` 是否进入子会话工具（嵌套） | 安全 | 禁止嵌套（对齐 Sati） |

---

## 5. 下一步

Owner 对 §4 决策并 Sign-off 后，进入 `03-design.md`（技术选型与架构图）。
