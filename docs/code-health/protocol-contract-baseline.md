# 协议面契约基线（M66）

> 定位：`docs/code-review-plan.md` 阶段 9（协议面专项）第一张卡的产物，**取证卡，零代码改动**。
>
> 本文件回答一个问题：七层协议面里，哪些东西是「契约」——一旦改动就会破坏已发布的客户端、
> 破坏 LLM replay fixture、或破坏跨进程互操作？把它们逐条列出来，作为 M67（golden fixture）
> 与 M70（协议文档）的输入。
>
> 契约的「家」始终是代码里的常量与结构体标签，本文件只是索引；M67 才提供机器验证。

## 1. 口径与红线

### 1.1 覆盖范围

| 层 | 源文件 | 测试文件 | 总行数 | 角色 |
| --- | ---: | ---: | ---: | --- |
| `a2a/` | 26 | 12 | 9,766 | Agent-to-Agent：HTTP JSON-RPC + WebSocket + SSE 流 |
| `a2ui/` | 14 | 6 | 3,813 | 声明式 UI 信封（surface/component/dataModel） |
| `agui/` | 6 | 3 | 3,126 | Agent-User Interaction：23 类事件 + SSE |
| `acp/` | 16 | 6 | 4,599 | Agent Client Protocol：stdio 行分隔 JSON-RPC |
| `server/` | 16 | 10 | 7,611 | HTTP/SSE API 面（含 AGUI 挂载与版本化中间件） |
| `mcp/` | 25 | 5 | 8,000 | MCP 客户端：stdio / http / sse 三种传输 |
| `bootstrap/` | 16 | 5 | 3,527 | 启动装配（含协议服务端所需依赖的注入顺序） |
| **合计** | **119** | **42** | **40,442** | |

### 1.2 契约面六类

1. **帧结构**：JSON-RPC 信封、SSE 帧、WebSocket 消息、JSONL 行
2. **JSON 字段名**：结构体 `json` 标签（含 `omitempty` 语义）
3. **事件类型**：AGUI 事件名、agentcore `EventType` 字符串、ACP 通知方法名
4. **错误码**：JSON-RPC 标准码 + 各层自定义码
5. **工具 `inputSchema`**：MCP / AGUI 暴露给 LLM 的工具描述（改动会破坏 replay fixture）
6. **instrumentation**：指标名、请求头、tracing 开关

### 1.3 阶段红线

- 阶段 9 **只允许内部重构**：提取函数、改名私有符号、拆文件、补注释、补测试
- 上述六类**一律零改动**；确需变更须按 M70 的变更流程单独立卡，并同步更新 M67 fixture
- 提交类型限定 `refactor` / `test` / `docs` / `chore`，不得混入 `feat` / `fix`

## 2. 测试覆盖矩阵（实测）

命令：`go test -cover ./a2a/... ./a2ui/... ./agui/... ./acp/... ./server/... ./mcp/... ./bootstrap/...`

| 层 | 语句覆盖率 | 测试文件 | 判定 |
| --- | ---: | ---: | --- |
| `a2ui` | 99.6% | 6 | 充分 |
| `agui` | 78.5% | 3 | 较好（converter 有 878 行专测；`handleRun` 仅在 e2e 覆盖） |
| `mcp` | 72.1% | 5 | 可接受，但 25 源仅 5 测，模块内分布不均 |
| `acp` | 58.3% | 6 | 偏弱：`session/*` 有 1,446 行专测，`server_*` 处理器覆盖不足 |
| `server` | 57.2% | 10 | 偏弱：`disclosure` / `enablement` 事件面缺直测 |
| `a2a` | 50.2% | 12 | 偏弱：`ws_*` 与 `push` / `handoff` 覆盖最低 |
| `bootstrap` | **3.4%** | 1 | **严重薄弱**：9 个源文件仅 `setup_test.go` 触达 3.4% |
| `bootstrap/agentconfig` | 46.2% | 3 | 偏弱 |

> 说明：语句覆盖率只说明「代码被执行过」，不说明「契约被断言过」。例如 AGUI 的 23 个事件类型
> 大部分有构造测试，但没有一个测试锁定「完整事件序列 + 字段名」；这正是 M67 的缺口。

**薄弱点排序（M69 依据）**：`bootstrap`（3.4%）> `a2a`（50.2%）> `server`（57.2%）> `acp`（58.3%）。
本阶段 M69 只处理 `mcp` + `bootstrap`，`a2a`/`server`/`acp` 中与契约直接相关的部分由 M67 的 fixture 覆盖。

## 3. 逐层契约

### 3.1 `a2a/`

**权威源**：`a2a/types.go`（枚举与错误码）、`a2a/server.go`（端点）、`a2a/server_jsonrpc.go`（方法分发、SSE）、`a2a/ws.go`（WS 分发）。

传输端点：

| 端点 | 方法 | 用途 |
| --- | --- | --- |
| `/.well-known/agent.json` | GET | Agent Card |
| `/` | POST | JSON-RPC 2.0 单入口 |
| `/health` | GET | 健康检查 |
| `/ws` | GET（Upgrade） | WebSocket 双向通道 |
| `/federation/agents` | GET | 联邦 Agent 列表（可选启用） |

JSON-RPC 方法（8 个）：`tasks/send`、`tasks/sendSubscribe`、`tasks/get`、`tasks/cancel`、
`tasks/query`、`tasks/pushNotification/set`、`tasks/pushNotification/get`、`tasks/resubscribe`。

WebSocket 方法（8 个）：上述 7 个 + `tasks/subscribe`（`sendSubscribe` 在 WS 下为 `subscribe`）。

数据枚举：

- `TaskState`：`submitted` / `working` / `input-required` / `completed` / `failed` / `canceled`
- `Role`：`user` / `agent`
- `PartType`：`text` / `file` / `data`

错误码：

| 码 | 常量 | 语义 |
| ---: | --- | --- |
| -32700 | `JSONRPCParseError` | 解析失败 |
| -32600 | `JSONRPCInvalidRequest` | 非法请求 |
| -32601 | `JSONRPCMethodNotFound` | 方法不存在 |
| -32602 | `JSONRPCInvalidParams` | 参数非法 |
| -32603 | `JSONRPCInternalError` | 内部错误 |
| -32001 | `A2AErrorTaskNotFound` | 任务不存在 |
| -32002 | `A2AErrorTaskNotCancelable` | 任务不可取消 |
| -32003 | `A2AErrorPushNotSupported` | 不支持推送 |
| -32004 | `A2AErrorUnsupportedOperation` | 不支持的操作 |
| -32005 | `A2AErrorContentTypeNotSupported` | 内容类型不支持 |
| -32006 | （`ratelimit.go` 内联） | 触发限流 |

SSE 帧（`tasks/sendSubscribe`、`tasks/resubscribe`）：

```
Content-Type: text/event-stream
id: <eventID>\ndata: <TaskUpdateEvent JSON>\n\n     # eventID>0 时带 id 行
data: <TaskUpdateEvent JSON>\n\n                  # eventID==0 时无 id 行
: <comment>\n\n                                  # 保活注释
```

注意：A2A 的 SSE **不使用 `event:` 行**，事件类型在 JSON 载荷内部。这与 AGUI/server 的帧格式不同，
是 M67 fixture 必须分别锁定的点。

### 3.2 `a2ui/`

**权威源**：`a2ui/message.go`（信封）、`a2ui/doc.go`（版本）、`a2ui/stream.go`（JSONL）、`a2ui/binding_*.go`（三处绑定）。

- 协议版本：`v0.9.1`（`a2ui.Version`）
- 基础组件目录 ID：`https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json`
- 信封：四个消息体**互斥其一**

| 字段 | 类型 | 关键子字段 |
| --- | --- | --- |
| `version` | string（`omitempty`，缺省填 `v0.9.1`） | — |
| `createSurface` | object | `surfaceId`、`catalogId`、`theme?`、`sendDataModel?` |
| `updateComponents` | object | `surfaceId`、`components[]`（每项含 `id`、`component`） |
| `updateDataModel` | object | `surfaceId`、`path`（JSON Pointer）、`value` |
| `deleteSurface` | object | `surfaceId` |

传输绑定（三条，均不得改动）：

1. **JSONL**：`Encoder`/`Decoder`，一行一个紧凑 JSON 对象（`stream.go`）
2. **AGUI CUSTOM 通道**：`agui.EventCustom` + `name="a2ui"` + `value=<Envelope>`（`binding_agui.go`）
3. **A2A / agentcore**：`binding_a2a.go`、`binding_agentcore.go` 把信封包进对应事件载荷

### 3.3 `agui/`

**权威源**：`agui/types.go`（事件类型与结构体）、`agui/handler.go`（SSE 写出）、`agui/converter.go`（事件映射）。

事件基座：`BaseEvent{type, timestamp}`，`timestamp` 为**毫秒**浮点（`tsNano = UnixNano/1e6`）。

23 个事件类型（`EventType` 字符串即 SSE `event:` 名）：

| 族 | 事件 |
| --- | --- |
| 运行 | `RUN_STARTED`、`RUN_FINISHED`、`RUN_ERROR` |
| 步骤 | `STEP_STARTED`、`STEP_FINISHED` |
| 文本消息 | `TEXT_MESSAGE_START`、`TEXT_MESSAGE_CONTENT`、`TEXT_MESSAGE_END` |
| 思考 | `THINKING_START`、`THINKING_TEXT_MESSAGE_START`、`THINKING_TEXT_MESSAGE_CONTENT`、`THINKING_TEXT_MESSAGE_END`、`THINKING_END` |
| 工具调用 | `TOOL_CALL_START`、`TOOL_CALL_ARGS`、`TOOL_CALL_END`、`TOOL_CALL_RESULT` |
| 状态 | `STATE_SNAPSHOT`、`STATE_DELTA`、`MESSAGES_SNAPSHOT` |
| 通用 | `CUSTOM`、`RAW` |
| 用量 | `CONTEXT_USAGE` |

消息 ID 生成规则（客户端会据此做流式拼接，属契约）：

- 文本消息：`msg_<递增序号>`
- 思考块：`thinking_<递增序号>`

SSE 帧：

```
event: <EVENT_TYPE>\ndata: <JSON>\n\n
event: <EVENT_TYPE>\ndata: {}\n\n        # 序列化失败时的降级帧
```

HTTP：

- `POST` → 执行一轮（SSE 响应）
- `GET` → 返回能力描述 `CapabilitiesFromConfig(cfg)`
- 其他方法 → `405 {"error":"method not allowed"}`
- 在 `server/` 下挂载路径为 `/agui/{path}`，**不参与 `/api/v1` 版本化**

### 3.4 `acp/`

**权威源**：`acp/protocol.go`（帧与字段）、`acp/server_handlers.go`（方法分发）、`acp/server_response.go`（帧写出）。

- 协议版本：`ProtocolVersion = 1`
- 传输：stdio，**每帧一行 JSON**（`fmt.Fprintf(w, "%s\n", data)`）

JSON-RPC 方法（11 个）：`initialize`、`authenticate`、`session/new`、`session/load`、`session/resume`、
`session/fork`、`session/list`、`session/prompt`、`session/cancel`、`session/set_mode`、`session/set_model`。

通知与反向请求：

- Agent → Client 通知：`session/update`（`SessionNotification` 载荷）
- Agent → Client 请求：`session/request_permission`

错误码：

| 码 | 语义 |
| ---: | --- |
| -32603 | 内部错误 |
| -32601 | 方法不存在 |
| -32602 | 参数非法 / 客户端能力被拒 |
| -32000 | 需要认证 / 认证失败 |
| -32001 | 未配置认证提供方 |
| -32002 | 会话不存在 |
| -32003 | 会话属性更新失败 |

提示内容块类型（`session/prompt` 载荷内）：`text`、`resource`、`resource_link`、`image`、`audio`。

### 3.5 `server/`

**权威源**：`server/server.go`（路由注册）、`server/middleware.go`（版本化/日志头）、`server/stream_events.go`（SSE 载荷）。

路由（`/v1/*` 原生 + `/api/*` 经 `registerAPIRoute` 同时注册 `/api/v1/*`）：

| 路径 | 方法 | 说明 |
| --- | --- | --- |
| `/health`、`/ready` | GET | 健康/就绪 |
| `/api/chat` → `/api/v1/chat` | POST | 对话（SSE 或非流式） |
| `/api/skills`、`/api/skills/diagnostics`、`/api/skills/events`、`/api/skills/status` | GET | 技能查询 |
| `/api/skills/reload` | POST | 重载技能 |
| `/api/threads` | POST / GET | 线程创建/列表 |
| `/api/threads/{key}` | GET / DELETE | 线程元数据/删除 |
| `/api/threads/{key}/config` | GET / PUT | 线程配置 |
| `/api/threads/{key}/thinking` | GET / PUT | 思考配置 |
| `/api/threads/{key}/branch` | POST | 分支 |
| `/api/states`、`/api/states/{key}` | GET / DELETE | 状态查询与删除 |
| `/v1/disclosure/analyze`（及 `/{task_id}`、`/{task_id}/stream`、`/{task_id}/review`） | POST / GET | 公开分析 |
| `/v1/evidence/judge`、`/judge/batch`、`/judge/{task_id}`、`/burden/{scenario}`、`/standard/{standard}`、`/conflict` | POST / GET | 证据判断 |
| `/agui/{path}` | POST / GET | AGUI 事件流 |
| `/metrics` | GET | 当 recorder 实现 `MetricsHandler()` 时注册 |

SSE 帧（`/api/chat`、`/api/skills/events`、disclosure 流）：

```
event: <agentcore.EventType>\ndata: <streamEventPayload JSON>\n\n
event: done\ndata: <StreamDoneEvent JSON>\n\n
: connected\n\n                                   # disclosure 流首帧注释
```

事件名来源：`agentcore.EventType` 字符串（`agent_start`/`agent_end`/`turn_start`/`turn_end`/
`message_delta`/`tool_call_start`/`tool_call_end`/`handoff_*`/`compaction_*`/`auto_retry`/
`agent_interrupt`/`approval_prompt`/`task_created`/`task_updated`/`a2ui` 等）
＋ `plantask_status_changed` / `plantask_feedback_added` / `plantask_interrupted`
＋ `deliver/registered`
＋ `skills_snapshot`
＋ MCP 五个：`mcp_capabilities_updated` / `mcp_tools_refreshed` / `mcp_transport_error` / `mcp_reconnect` / `mcp_refresh`。

> 这批字符串同时是「服务端 SSE 事件名」与「agentcore 事件类型」，**一处改动会同时影响两侧**，
> 是 M67 最需要锁定的跨层契约。

响应头契约（`server/middleware.go`）：`X-Request-ID`（透传或生成 16 字节 hex 回写）、
`X-API-Version: v1`（新路径）、`X-API-Deprecated: true` + `X-API-Deprecated-Date`（旧路径）。

### 3.6 `mcp/`

**权威源**：`mcp/client.go`、`mcp/discovery_client.go`（方法名）、`mcp/config_discovery.go`（配置来源）、`mcp/events.go`（事件）。

客户端 RPC 名（11 个，含 1 个通知）：`initialize`、`notifications/initialized`、`tools/list`、`tools/call`、
`resources/list`、`resources/read`、`resources/templates/list`、`resources/subscribe`、
`resources/unsubscribe`、`prompts/list`、`prompts/get`。

传输类型（`Transport` 字段取值）：`stdio`、`http`、`sse`。

配置来源与信任边界：

- 发现顺序见 `config_discovery.go`（项目 `.mcp.json`、用户级配置等）
- 命令执行信任：`config_trust.go` 以 SHA-256 记录已信任配置（**敏感路径**）
- stdio 子进程：`process_unix.go` / `process_windows.go` 平台分支

MCP 事件（5 个，经 server SSE 透出）：见 3.5 末段。

### 3.7 `bootstrap/`

`bootstrap/` **不是线协议层**，它没有帧、没有错误码；纳入本专项的原因是其装配顺序构成了
「协议服务端可用」的隐式契约：tracing 注入、知识库加载、推理模板注册、plantask 桥接。
顺序错误不会编译失败，只会运行期降级。

已知装配契约（改动须人工复核）：

- `bootstrap/setup.go` — `MADY_TRACING=stdout` 时安装 stdout tracer（instrumentation 入口）
- `bootstrap/init_reasoning.go` — `InitReasoningAndTemplates`（登记为长函数，启动顺序敏感）
- `bootstrap/init_knowledge.go` — `LoadWikiStore`（登记为长函数）
- `bootstrap/setup_steps.go` — 分步装配主流程
- `bootstrap/plantask_bridge.go` — 任务面板桥接

## 4. instrumentation 清单

| 位置 | 形态 | 契约点 |
| --- | --- | --- |
| `server/metrics.go` | `expvar` 指标 | `http_requests_total`、`http_request_errors_total`、`http_active_connections`、`http_request_duration_seconds_bucket_{0_05,0_1,0_25,0_5,1,2_5,5,10,30}` |
| `server/middleware.go` | 请求头 | `X-Request-ID`、`X-API-Version`、`X-API-Deprecated`、`X-API-Deprecated-Date` |
| `server/server.go` | 开关 | `MetricsRecorder` 接口；实现 `MetricsHandler()` 才注册 `GET /metrics` |
| `bootstrap/setup.go` | 环境变量 | `MADY_TRACING`（仅支持 `stdout`，未知值仅告警） |

结论：**`a2a` / `a2ui` / `agui` / `acp` / `mcp` 五层没有自带的 tracing/metrics**，可观测性统一由
`agentcore` 事件 + `server` 的 SSE 透出。这是有意为之（分层单向依赖），阶段 9 不得在协议层私接指标。

## 5. 后续卡片映射（本基线如何被使用）

| 卡 | 用法 |
| --- | --- |
| **M67** | 把 §3 的枚举、方法集、错误码、帧格式固化为 golden fixture：AGUI 23 事件序列、A2A 8 方法 + 11 错误码、A2UI 四信封 + JSONL、ACP 11 方法 + 7 错误码、server 路由表 + SSE 事件名、MCP 11 RPC 名；任何契约漂移测试即红 |
| **M68** | 按 §3.3 重拆 `agui/converter.go` 的 `Convert`（P2 登记项，194 行），拆完必须让 M67 的 AGUI fixture 原样通过 |
| **M69** | 按 §2 薄弱点补测 `mcp/`（握手/重连/信任）与 `bootstrap/`（装配顺序），只加测试 |
| **M70** | 把 §3 扩写为按层 `docs/protocol-*.md`，并定义契约变更流程（改帧 → 改 fixture → 改文档 → 人工 review） |

## 6. 取证命令（可复现）

```bash
# 规模
for d in a2a a2ui agui acp server mcp bootstrap; do
  git ls-files "$d/**/*.go" "$d/*.go" | sort -u | xargs wc -l | tail -1
done

# 覆盖率
go test -cover ./a2a/... ./a2ui/... ./agui/... ./acp/... ./server/... ./mcp/... ./bootstrap/...

# 契约枚举
grep -n 'EventType = "' agui/types.go            # 23 个 AGUI 事件
grep -n 'case "' acp/server_handlers.go         # 11 个 ACP 方法
grep -n 'case "' a2a/server_jsonrpc.go          # 8 个 A2A 方法
grep -n 'JSONRPC.*= -3\|A2AError' a2a/types.go  # A2A 错误码
grep -n 'mux.Handle\|registerAPIRoute' server/server.go
```
