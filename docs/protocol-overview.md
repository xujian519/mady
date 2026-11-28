# 协议面总览与契约变更流程（M70）

> 定位：`docs/code-review-plan.md` 阶段 9（协议面专项）收尾卡产物。
>
> 本文件回答三个问题：**协议面有哪几层**、**哪些东西是契约**、**改契约要走什么流程**。
> 逐层的完整线协议参考见 [`protocol-reference.md`](protocol-reference.md)；
> 契约取证与覆盖矩阵见 [`code-health/protocol-contract-baseline.md`](code-health/protocol-contract-baseline.md)。

## 1. 协议面地图

七层里六层是有线协议的（有帧、有错误码），`bootstrap` 是启动装配层，没有线协议，
但装配顺序构成「协议服务端可用」的隐式契约，故一并纳入。

| 层 | 传输 | 对外角色 | 契约权威源 | Fixture |
| --- | --- | --- | --- | --- |
| `a2a/` | HTTP JSON-RPC + WebSocket + SSE | Agent-to-Agent 互操作 | `a2a/types.go`、`a2a/server.go`、`a2a/server_jsonrpc.go` | `a2a/contract_test.go` |
| `a2ui/` | 内嵌于 AGUI/A2A/JSONL | 声明式 UI 信封 | `a2ui/message.go`、`a2ui/doc.go`、`a2ui/stream.go` | `a2ui/contract_test.go` |
| `agui/` | HTTP + SSE | 前端事件流 | `agui/types.go`、`agui/handler.go`、`agui/converter.go` | `agui/contract_test.go` |
| `acp/` | stdio 行分隔 JSON-RPC | 编辑器接入（Zed 等） | `acp/protocol.go`、`acp/server_handlers.go` | `acp/contract_test.go` |
| `server/` | HTTP + SSE | 桌面端 / 第三方 HTTP API | `server/server.go`、`server/middleware.go`、`server/stream_events.go` | `server/contract_test.go` |
| `mcp/` | stdio / http / sse | 外部 MCP 服务端互操作 | `mcp/client.go`、`mcp/discovery_client.go`、`mcp/events.go` | `mcp/contract_test.go` |
| `bootstrap/` | —（进程内装配） | 启动顺序与能力注入 | `bootstrap/setup_steps.go`、`bootstrap/init_*.go`、`bootstrap/plantask_bridge.go` | `bootstrap/plantask_bridge_test.go` |

## 2. 契约六类

任何「外部客户端会观察到、且不能一夜之间同步升级」的东西都算契约：

1. **帧结构** — JSON-RPC 信封、SSE 帧、WebSocket 消息、stdio 行帧
2. **JSON 字段名** — 结构体 `json` 标签，含 `omitempty` 语义（缺省即无此字段）
3. **事件类型** — AGUI 23 个事件名、`agentcore.EventType` 字符串、ACP 通知方法名、MCP 事件名
4. **错误码** — JSON-RPC 标准码 + A2A / ACP 自定义码及其文案
5. **工具 `inputSchema`** — 暴露给 LLM 或 MCP 客户端的工具描述（改动会破坏 replay fixture）
6. **instrumentation** — 指标名、请求/响应头、流 schema 名

不属于契约（阶段 9 允许改动）：私有函数名、文件划分、内部数据结构、注释、日志文案
（面向用户文案另受 `docs/tone-style-guide.md` 约束）。

## 3. 契约变更流程

> 这是协议契约变更的**唯一权威流程**（「家」）。M67 fixture 是机器验证，本文件是理由记录。
> 三件套缺一不可（见 AGENTS.md 规范元规则）。

### 3.1 判定：这次改动是不是契约变更？

| 现象 | 判定 |
| --- | --- |
| 改常量值 / 改 `json` 标签 / 改事件名 / 改错误码或文案 | **契约变更** |
| 增删端点、路由、RPC 方法名 | **契约变更** |
| 改工具 `inputSchema` 任何字段 | **契约变更** |
| 提取函数、拆文件、改私有名、补注释、加测试 | 非契约变更（可直接走重构卡） |
| 只改日志级别、内部错误包装（不改变对外文案） | 非契约变更 |

### 3.2 流程（契约变更必须全部走完）

1. **立卡**：单独开卡，标明「协议契约变更」，不得混入重构 / 补测 / 清理提交
2. **判断破坏性**：客户端不升级会坏 → 破坏性；反之按兼容演进（新增可选字段）
3. **同一提交内同步三处**：
   - 代码（契约权威源常量 / 结构体标签）
   - **M67 fixture**（对应 `*/contract_test.go` 的 golden 值；不允许「先改代码、后补 fixture」）
   - 本仓文档：`docs/protocol-reference.md` 对应层小节
4. **人工 review**：协议面属敏感变更，禁止未经人工审阅直接合入；
   破坏性变更须在提交正文与 changelog 中写明影响面（哪些客户端、如何迁移）
5. **记录**：`scripts/changelog/main.go` 追加条目，`--scope=protocol`
6. **验证**：`go test -race ./...` + `make verify`（fixture 必须全绿）

### 3.3 禁止事项

- 禁止在重构卡里「顺手」改帧结构、字段名、事件名、错误码、`inputSchema`
- 禁止只改 fixture 不改文档，或只改文档不改 fixture（两边会漂移）
- 禁止用 `omitempty` 增删字段来「保持兼容」——空值语义本身就是契约的一部分
- 禁止把 A2A 的「SSE 无 `event:` 行」与 AGUI/server 的「SSE 有 `event:` 行」互相「统一」

### 3.4 已知的、按实现固化的两处偏差

M67-2 取证时发现基线描述与实测不一致，现按实现固化，改动任一侧都属契约变更：

1. `a2a`：未启用 `federationRegistry` 时不挂载 `/federation/agents`，
   请求会落到 `/` 的 JSON-RPC 入口并返回 `-32600`（而不是 404）
2. `acp`：`NewServer` 会为 nil `AuthProvider` 注入 `noopAuthProvider`（认证必然失败），
   因此 `-32001` 的可观察文案是 `Authentication failed`；`Authentication not configured`
   分支在 `NewServer` 路径上不可达

## 4. 分层索引

| 主题 | 位置 |
| --- | --- |
| A2A 端点 / 方法 / 枚举 / 错误码 / SSE 帧 | [`protocol-reference.md` §1](protocol-reference.md) |
| A2UI 信封 / 版本 / 绑定 / JSONL | [`protocol-reference.md` §2](protocol-reference.md) |
| AGUI 23 事件 / ID 规则 / SSE 帧 / HTTP 语义 | [`protocol-reference.md` §3](protocol-reference.md) |
| ACP 方法 / 通知 / 错误码 / stdio 帧 | [`protocol-reference.md` §4](protocol-reference.md) |
| Server 路由表 / 版本化头 / 流 schema / 事件名 | [`protocol-reference.md` §5](protocol-reference.md) |
| MCP RPC / 传输 / 事件 / 信任边界 | [`protocol-reference.md` §6](protocol-reference.md) |
| Bootstrap 装配契约 | [`protocol-reference.md` §7](protocol-reference.md) |
| fixture 与关键测试名对照 | [`protocol-reference.md` §8](protocol-reference.md) |
| 取证命令与覆盖率矩阵 | [`code-health/protocol-contract-baseline.md`](code-health/protocol-contract-baseline.md) |

## 5. 变更记录

| 日期 | 变更 | 卡 |
| --- | --- | --- |
| 2026-11-28 | 建立七层契约索引、覆盖矩阵与取证命令 | M66 |
| 2026-11-28 | AGUI / A2UI golden fixture 落地 | M67-1 |
| 2026-11-28 | A2A / ACP / server / MCP fixture 落地（固化 §3.4 两处偏差） | M67-2 |
| 2026-11-28 | `agui/converter.go` 按事件族拆分（帧结构零改动） | M68 |
| 2026-11-28 | `mcp` / `bootstrap` 弱覆盖补测 | M69 |
| 2026-11-28 | 本总览与逐层参考文档、变更流程建立 | M70 |
