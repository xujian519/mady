# 03 — 设计：基于 sky-valley/pi 的动态子会话（辅助智能体）

- **功能名**：pi-subagent
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **设计日期**：2026-08-13
- **状态**：待人工审阅
- **依赖规格**：[02-spec.md](./02-spec.md)

---

## 1. 技术选型

### 1.1 子会话运行时：pi `coding.Session`（库嵌入）

| 方案 | 评估 |
|------|------|
| **A. pi `coding.NewSession`（选）** | 现成编码循环 + 系统提示词构建（AGENTS.md/CLAUDE.md 折叠）+ compaction + JSONL；SDK 形态（`Run`/`RunResult`/`Subscribe`）专为嵌入设计 |
| B. 仅用 pi `agent` 包自建循环 | 重复 pi coding 已有能力，失去内置工具/compaction |
| C. Mady agentcore 自建子会话 | 需新增独立上下文、工具裁剪、只读强制、compaction 全套，成本远高于引入 pi |
| D. 子进程调用 pi CLI | 进程生命周期/资源管理成本高，且与「纯库嵌入」约束冲突 |

**为何选 A**：Mady 与 pi 同 Go 同版本（1.26），库嵌入零异构成本；pi 的 `SessionOptions` 提供 `ToolNames`/`ExcludeTools`/`NoTools`/`CustomTools` 四重工具门控，与 Sati 子代理的 `allowedTools`/`omitTools` 语义直接对应。

### 1.2 派发方式：tools 层新工具 `spawn_agent`

| 方案 | 评估 |
|------|------|
| **A. 新工具 `spawn_agent`（选）** | 对齐 Sati `agent` 工具；tools 扩展机制成熟（`tools.NewExtension` + `RegisterTools`）；不动 Handoff 安全红线 |
| B. 扩展 Handoff（transfer_to_*） | 触碰 `agentcore/handoff.go` 安全红线，且 Handoff 语义是领域委派而非任意子会话 |
| C. LifecycleHook 注入 | 违背「父循环不动」约束，事件驱动不适合同步派发 |

### 1.3 工具桥接与安全包装

**核心不变量：子会话内任何工具执行前必须过 Mady 安全层。**

```
子会话模型请求 → pi 内部校验 → 调用桥接 AgentTool.Execute
                                     │
                                     ▼
                    ① permission 校验（Allow/Ask/Deny，agentcore/permission）
                    ② WorkingDir 沙箱（tools/path.go resolvePathSandboxed）
                    ③ 只读预设拒绝写类工具（按 domain/名称判定）
                    ④ 执行 Mady 工具实现 → AgentToolResult
```

- **为什么包装在桥接层**：pi 内置工具（bash 等）直接暴露会绕过 Mady 沙箱与 permission。阶段 1 子会话白名单**只注入桥接后的 Mady 工具**，pi 内置 coding 工具不进入白名单（除非 Owner 在阶段 2 明确放开）。
- **只读判定**：预设级 `isReadOnly` 标志；写类工具清单复用 `tool_domains.go` 的 domain 机制（写类工具打 write domain 或名称黑名单 `write/edit/delete/*_write`）。

### 1.4 Schema 转换（Mady Tool.Parameters → pi ai.Schema）

- Mady `agentcore.Tool.Parameters` 为 JSON Schema（`tool_gen_schema.go` 生成）。
- pi `ai.Schema` 为 JSON Schema 草案 2020-12 子集（`ai/tools.go`）。需在实现阶段核验字段覆盖（$ref/anyOf 支持度）。
- **[NEEDS CLARIFICATION: 若 Mady 现有工具 schema 含 pi 不支持的字段（如 $ref），降级策略为：子会话工具注册时跳过不支持的工具并 WARN（不阻塞派发）]**

### 1.5 模型与密钥

- 阶段 1：默认模型 = Mady agentconfig 当前默认 Provider/Model（沿用 `pkg/agentconfig` 解析通道），API Key 走既有环境变量。
- 阶段 2：`spawn_agent.model` 参数走 pi `ResolveModel(spec)`（`anthropic/claude-sonnet-4-5` 格式），Key 由 pi `ai.GetEnvApiKey` 解析（ANTHROPIC_API_KEY/OPENAI_API_KEY/GEMINI_API_KEY）。
- `ThinkingLevel` 映射：`spawn_agent.thinking` 枚举 ↔ pi `agent.ThinkingLevel`（off→xhigh），pi 侧按模型 clamp。

### 1.6 超时与取消

- 子会话运行受 `context.WithTimeout(ctx, SPAWN_AGENT_TIMEOUT)`（默认 120s）。
- 父会话取消（用户中断/Agent 终止）通过 ctx 级联到 pi `Session.Run`（pi 全程 `context.Context`，天然支持）。

### 1.7 包归属

| 新增内容 | 位置 | 说明 |
|----------|------|------|
| 桥接层（转换 + 安全包装） | `tools/piagent/`（tools 子模块） | bridge.go / presets.go / session.go |
| 工具注册 | `tools/tools.go` | `spawn_agent` 加入 `BuildTools` |
| 预设定义 | `tools/piagent/presets.go` | 对齐 Sati builtinSubagentTypes 语义 |

> tools 子模块独立 go.mod：pi 依赖落在 `tools/go.mod`，不污染根模块依赖面（同时回答 02-spec Q1：若走 tools 子模块，根模块 go.mod 零新增）。

---

## 2. 架构

### 2.1 时序图（阶段 1 MVP）

```
父 Agent (mady-agent)                    tools/piagent 桥接层                pi coding.Session
        │ 调用 spawn_agent                     │                                │
        │─────────────────────────────────────>│                                │
        │                                      │ ① 解析预设(白名单+提示词后缀)   │
        │                                      │ ② 白名单 = (预设∪tools)−exclude │
        │                                      │    经 tool_domains 校验         │
        │                                      │ ③ NewSession(SessionOptions{   │
        │                                      │     ToolNames, ExcludeTools,    │
        │                                      │     CustomTools: 桥接工具,      │
        │                                      │     ThinkingLevel, Compaction})│
        │                                      │───────────────────────────────>│
        │                                      │           Run(ctx, directive)  │
        │                                      │<── (工具调用循环, 每步过安全层)─┤
        │                                      │   RunResult{Text,ToolCalls,    │
        │                                      │     Usage,StopReason}          │
        │<── 结构化报告(success/report/usage)──│                                │
```

### 2.2 包依赖图

```
tools (子模块) ──> agentcore（接口层 agentcore.Extension / agentcore.Tool）
   │
   └── piagent
         ├──> github.com/sky-valley/pi/coding
         ├──> github.com/sky-valley/pi/agent
         ├──> github.com/sky-valley/pi/ai
         └──> tools（既有工具实现：read_file/grep/glob/... 的执行函数）
```

### 2.3 数据流（子会话工具调用）

```
模型 → pi 校验参数 → AgentTool.Execute(ctx, id, params, onUpdate)
  → ① permission.Decide()      [Deny → 返回权限拒绝结果]
  → ② WorkingDir 沙箱校验      [越界 → 返回沙箱错误结果]
  → ③ 只读预设写工具判定        [拒绝 → 返回只读错误结果]
  → ④ 执行 Mady 工具实现 → AgentToolResult{Content, Terminate}
```

---

## 3. 安全考量

| 面 | 设计 | 对照 |
|----|------|------|
| 沙箱 | 子会话工具全部经 `tools/path.go` 解析；pi 内置 bash 不注入白名单 | Sati 无此层，Mady 独有 |
| 权限 | permission Allow/Ask/Deny 在桥接层执行，Deny 结果回传模型（不暴露内部异常） | 对齐 Mady 全局门控 |
| 只读 | 预设级 isReadOnly + 写工具清单双重判定，拒绝发生在执行前 | 对齐 Sati isReadOnly |
| 嵌套 | 子会话注册表不含 `spawn_agent` | 对齐 Sati 禁止嵌套派发 |
| 密钥 | 复用 agentconfig/环境变量通道，不新增硬编码 | 对齐 SECURITY.md |
| 供应链 | pi pin v0.84.17；依赖树（x/image、x/text）在 PR 中列出 | 对齐「依赖克制」 |
| 敏感路径 | `tools/tools.go` 注册改动 → L3 人工审阅；不动 `agentcore/handoff.go`/`permission/` 语义 | 对齐 AGENTS.md |

---

## 4. 关键算法

### 4.1 白名单集合运算

```
allowed(preset, tools, exclude) =
    (presetTools ∪ tools) ∩ registeredMadyTools − exclude − {spawn_agent}
    再经 tool_domains.FilterToolNames 校验（非法域剔除并 WARN）
```

### 4.2 只读判定

```
isWriteTool(name) = domainOf(name) ∈ WRITE_DOMAINS ∨ name ∈ WRITE_NAME_SET
只读预设执行前：isWriteTool → 返回错误结果 {content: "read-only subagent: tool X is not allowed"}
```

### 4.3 报告聚合

```
RunResult → report:
  scope/result    ← RunResult.Text（子会话按强制格式输出，桥接层透传）
  key_files       ← 从 ToolCalls 中 read_file/grep/glob 参数提取（阶段 1 启发式）
  files_changed   ← 从写工具调用提取（只读预设恒为空）
  issues          ← RunResult.ErrorMessage / StopReason=error 时填充
  usage           ← RunResult.Usage（input/output/cost）
```

---

## 5. 测试策略

| 层级 | 内容 |
|------|------|
| 单元 | 白名单集合运算；Schema 转换正/反向；只读拒绝矩阵；超时取消 |
| 集成（tools 子模块） | 用 pi `faux` provider（确定性测试替身，`ai/providers/faux`）跑真实 `Session.Run`，断言 AC-1/2/3/4/6 |
| 安全 | 沙箱越界返回错误；Deny 域拦截；只读预设写工具拒绝 |
| 回归 | `make verify` 全绿；既有 Handoff/orchestration/worker 测试不回归 |

---

## 6. 实施顺序（详见 04-tasks.md）

1. **T1** 依赖引入 + 传递依赖核验（tools 子模块 go.mod）
2. **T2** 桥接层（Schema 转换 + 安全包装 + 只读判定）
3. **T3** `spawn_agent` 工具 + 预设注册
4. **T4** faux-provider 集成测试（AC-1..7）
5. **T5** 文档（README 工具表 + specs 索引 + AI changelog）

---

## 7. 风险回顾（增量）

| 风险 | 缓解 |
|------|------|
| pi `ai.Schema` 与 Mady schema 不兼容 | §1.4 降级策略：跳过不支持工具并 WARN |
| pi 内置工具行为不可控（bash 沙箱缺失） | 阶段 1 白名单仅桥接 Mady 工具 |
| `faux` provider 覆盖面不足 | 仅用于确定性测试；真实验收走手动冒烟 |
