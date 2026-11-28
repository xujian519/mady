# 协议面逐层参考（M70）

> 定位：`docs/code-review-plan.md` 阶段 9 收尾卡产物，面向**接入方**与**改动方**的线协议速查。
>
> 契约判定与变更流程见 [`protocol-overview.md`](protocol-overview.md)；
> 取证命令、覆盖率矩阵与逐层盘点过程见
> [`code-health/protocol-contract-baseline.md`](code-health/protocol-contract-baseline.md)。
> 本文件与 fixture（`*/contract_test.go`）是同一契约的两种表达：**文档给理由，fixture 给机器验证**。

## 1. `a2a/` — Agent-to-Agent

### 1.1 端点

| 端点 | 方法 | 用途 |
| --- | --- | --- |
| `/.well-known/agent.json` | GET | Agent Card（`name` 等能力描述） |
| `/` | POST | JSON-RPC 2.0 单入口 |
| `/health` | GET | 健康检查 |
| `/ws` | GET（Upgrade） | WebSocket 双向通道 |
| `/federation/agents` | GET | 联邦 Agent 列表（仅注册 `federationRegistry` 时挂载） |

未注册联邦时 `/federation/agents` 落到 `/` 的 JSON-RPC 入口并返回 `-32600`（见总览 §3.4）。

### 1.2 JSON-RPC 方法（HTTP 入口 8 个）

`tasks/send`、`tasks/sendSubscribe`、`tasks/get`、`tasks/cancel`、`tasks/query`、
`tasks/pushNotification/set`、`tasks/pushNotification/get`、`tasks/resubscribe`。

WebSocket 入口同样 8 个，但订阅方法名为 `tasks/subscribe`（不带 `send`）；
`tasks/sendSubscribe` 是 HTTP/SSE 专用。

入口错误行为：

| 场景 | 结果 |
| --- | --- |
| 非 POST 请求 | `-32600`（HTTP 200 + JSON-RPC 错误体） |
| 非 JSON `Content-Type` | `-32600`，message 含 `Content-Type` |
| 未知方法 | `-32601`，message 形如 `method "tasks/nope" not found` |

### 1.3 枚举

| 枚举 | 取值 |
| --- | --- |
| `TaskState` | `submitted` / `working` / `input-required` / `completed` / `failed` / `canceled` |
| `Role` | `user` / `agent` |
| `PartType` | `text` / `file` / `data` |

### 1.4 错误码

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

### 1.5 SSE 帧

A2A 的 SSE **不使用 `event:` 行**，事件类型在 JSON 载荷（`TaskUpdateEvent`）内部：

```
id: <eventID>\ndata: <JSON>\n\n     # eventID > 0
data: <JSON>\n\n                    # eventID == 0
: <comment>\n\n                     # 保活注释
```

与 AGUI / server 的帧格式不同，切勿「统一」。`TaskUpdateEvent` 字段为 `id`、`final` 等。

## 2. `a2ui/` — 声明式 UI 信封

| 项 | 值 |
| --- | --- |
| 协议版本 | `v0.9.1`（`a2ui.Version`） |
| 基础组件目录 | `https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json`（`a2ui.BasicCatalogID`） |

四个消息体**互斥其一**（同一条信封只出现一个）：

| 字段 | 关键子字段 |
| --- | --- |
| `version` | 缺省填 `v0.9.1`（`omitempty`） |
| `createSurface` | `surfaceId`、`catalogId`、`theme?`、`sendDataModel?` |
| `updateComponents` | `surfaceId`、`components[]`（每项含 `id`、`component`） |
| `updateDataModel` | `surfaceId`、`path`（JSON Pointer）、`value` |
| `deleteSurface` | `surfaceId` |

三种传输绑定（`binding_*.go`）：

1. **JSONL**（`stream.go`）：一行一个紧凑 JSON 对象
2. **AGUI CUSTOM 通道**：`EventCustom` + `name="a2ui"` + `value=<Envelope>`
3. **A2A / agentcore**：信封包进对应事件载荷

## 3. `agui/` — Agent-User Interaction

### 3.1 事件基座与 ID 规则

- `BaseEvent{type, timestamp}`，`timestamp` 为**毫秒**浮点（`UnixNano/1e6`）
- 文本消息 ID：`msg_<递增序号>`；思考块 ID：`thinking_<递增序号>`
  （客户端据此做流式拼接，属契约）

### 3.2 事件类型（23 个，`event:` 行直接取这些字符串）

| 族 | 事件 |
| --- | --- |
| 运行 | `RUN_STARTED`、`RUN_FINISHED`、`RUN_ERROR` |
| 步骤 | `STEP_STARTED`、`STEP_FINISHED` |
| 文本 | `TEXT_MESSAGE_START`、`TEXT_MESSAGE_CONTENT`、`TEXT_MESSAGE_END` |
| 思考 | `THINKING_START`、`THINKING_TEXT_MESSAGE_START`、`THINKING_TEXT_MESSAGE_CONTENT`、`THINKING_TEXT_MESSAGE_END`、`THINKING_END` |
| 工具 | `TOOL_CALL_START`、`TOOL_CALL_ARGS`、`TOOL_CALL_END`、`TOOL_CALL_RESULT` |
| 状态 | `STATE_SNAPSHOT`、`STATE_DELTA`、`MESSAGES_SNAPSHOT` |
| 通用 | `CUSTOM`、`RAW` |
| 用量 | `CONTEXT_USAGE` |

`agentcore` 事件到 AGUI 事件的映射在 `agui/converter.go`（M68 已按事件族拆分为
`convertXxx` 组装函数）；值形态与指针形态的事件都必须覆盖，少一支会改变事件序列。

### 3.3 帧与 HTTP 语义

```
event: <EVENT_TYPE>\ndata: <JSON>\n\n
event: <EVENT_TYPE>\ndata: {}\n\n        # 序列化失败时的降级帧
```

| 方法 | 行为 |
| --- | --- |
| GET | 返回能力描述（`CapabilitiesFromConfig`） |
| POST | 执行一轮，SSE 响应 |
| 其他 | `405` + `{"error":"method not allowed"}` |

在 `server/` 下挂载于 `/agui/{path}`，**不参与** `/api/v1` 版本化。

## 4. `acp/` — Agent Client Protocol

| 项 | 值 |
| --- | --- |
| 协议版本 | `ProtocolVersion = 1` |
| 传输 | stdio，**每帧一行 JSON**（`"%s\n"`，帧内不得出现裸换行） |

### 4.1 方法（11 个）

`initialize`、`authenticate`、`session/new`、`session/load`、`session/resume`、`session/fork`、
`session/list`、`session/prompt`、`session/cancel`、`session/set_mode`、`session/set_model`。

### 4.2 通知与反向请求

| 方向 | 方法 | 载荷 |
| --- | --- | --- |
| Agent → Client 通知 | `session/update` | `SessionNotification{sessionId, update{sessionUpdate, content}}` |
| Agent → Client 请求 | `session/request_permission` | 权限征询（`sessionId` + `path` 等） |

`session/update` 的 `sessionUpdate` 取值包含 `user_message_chunk`、`agent_message_chunk` 等；
通知帧**不带 `id` 字段**。

### 4.3 错误码

| 码 | 语义 | 可观察文案 |
| ---: | --- | --- |
| -32603 | 内部错误 | — |
| -32601 | 方法不存在 | `Method not found` |
| -32602 | 参数非法 / 客户端能力被拒 | `Invalid params` |
| -32000 | 需要认证 / 认证失败 | `Authentication required` |
| -32001 | 认证提供方拒绝 | `Authentication failed`（`NewServer` 注入 noop provider，见总览 §3.4） |
| -32002 | 会话不存在 | `Session not found` |
| -32003 | 会话属性更新失败 | — |

`session/prompt` 的内容块类型：`text`、`resource`、`resource_link`、`image`、`audio`。

## 5. `server/` — HTTP / SSE API

### 5.1 路由表

`/api/*` 经 `registerAPIRoute` 同时注册 `/api/v1/*` 镜像。

| 路径 | 方法 |
| --- | --- |
| `/health`、`/ready` | GET |
| `/api/chat` → `/api/v1/chat` | POST |
| `/api/skills`、`/api/skills/diagnostics`、`/api/skills/events`、`/api/skills/status` | GET |
| `/api/skills/reload` | POST |
| `/api/threads` | POST / GET |
| `/api/threads/{key}` | GET / DELETE |
| `/api/threads/{key}/config` | GET / PUT |
| `/api/threads/{key}/thinking` | GET / PUT |
| `/api/threads/{key}/branch` | POST |
| `/api/states`、`/api/states/{key}` | GET / DELETE |
| `/v1/disclosure/analyze`（及 `/{task_id}`、`/{task_id}/stream`、`/{task_id}/review`） | POST / GET |
| `/v1/evidence/judge`、`/judge/batch`、`/judge/{task_id}`、`/burden/{scenario}`、`/standard/{standard}`、`/conflict` | POST / GET |
| `/agui/{path}` | POST / GET |
| `/metrics` | GET（仅在 recorder 实现 `MetricsHandler()` 时注册） |

### 5.2 响应头契约

| 头 | 语义 |
| --- | --- |
| `X-Request-ID` | 透传或生成 16 字节 hex 回写 |
| `X-API-Version: v1` | 新路径（`/api/v1/*`） |
| `X-API-Deprecated: true` + `X-API-Deprecated-Date` | 旧路径（`/api/*`） |

非 `/api/` 前缀的路径不做版本镜像，也不加版本头。

### 5.3 SSE 帧与流 schema

```
event: <agentcore.EventType>\ndata: <streamEventPayload JSON>\n\n
event: done\ndata: <StreamDoneEvent JSON>\n\n
: connected\n\n                                   # disclosure 流首帧注释
```

`event:` 名来源：`agentcore.EventType` 字符串（`agent_start`、`agent_end`、`agent_error`、
`skill_loaded`、`skills_reloaded`、`turn_start`、`turn_end`、`message_delta`、`tool_call_start`、
`tool_call_end`、`handoff_start`、`handoff_end`、`compaction_start`、`compaction_end`、`auto_retry`、
`agent_interrupt`、`approval_prompt`、`task_created`、`task_updated`、`a2ui`、
`plantask_status_changed`、`plantask_feedback_added`、`plantask_interrupted`、`deliver/registered`）
＋ `skills_snapshot` ＋ MCP 5 个事件名（见 §6）。

流载荷 `schema` 常量（`server/stream_events.go`，8 个，属契约）：

| 常量 | 值 |
| --- | --- |
| `streamSchemaAgentEvent` | `agent.event.v1` |
| `streamSchemaMCPAbilitiesUpdated` | `mcp.capabilities_updated.v1` |
| `streamSchemaMCPToolsRefreshed` | `mcp.tools_refreshed.v1` |
| `streamSchemaMCPTransportError` | `mcp.transport_error.v1` |
| `streamSchemaMCPReconnect` | `mcp.reconnect.v1` |
| `streamSchemaMCPRefresh` | `mcp.refresh.v1` |
| `streamSchemaSkillsSnapshot` | `skills.snapshot.v1` |
| `streamSchemaChatDone` | `chat.done.v1` |

## 6. `mcp/` — MCP 客户端

### 6.1 RPC 方法（11 个，含 1 个通知）

`initialize`、`notifications/initialized`、`tools/list`、`tools/call`、`resources/list`、
`resources/read`、`resources/templates/list`、`resources/subscribe`、`resources/unsubscribe`、
`prompts/list`、`prompts/get`。

传输类型（`Transport` 取值）：`stdio`、`http`、`sse`。

### 6.2 事件（经 server SSE 透出）

| 常量 | 线值 |
| --- | --- |
| `EventMCPCapabilitiesUpdated` | `mcp_capabilities_updated` |
| `EventMCPToolsRefreshed` | `mcp_tools_refreshed` |
| `EventMCPTransportError` | `mcp_transport_error` |
| `EventMCPReconnect` | `mcp_reconnect` |
| `EventMCPRefresh` | `mcp_refresh` |

扩展的 `SnapshotEvents()` 产出 `CapabilitiesUpdatedEvent`，其中 `transport` 字段为
`stdio` 或 `http`，是前端区分传输类型的依据。

### 6.3 配置与信任边界

- 配置发现顺序见 `mcp/config_discovery.go`（项目 `.mcp.json`、用户级配置等）
- 命令执行信任：`mcp/config_trust.go` 以 SHA-256 记录已信任配置（**敏感路径**，
  改动须人工复核，见 AGENTS.md 安全敏感路径表）
- stdio 子进程按平台分文件：`process_unix.go` / `process_windows.go`

## 7. `bootstrap/` — 启动装配契约

`bootstrap/` 没有帧与错误码，但装配顺序决定协议服务端能否工作；顺序错误只会运行期降级，
不会编译失败，故列入协议面。

| 文件 | 装配契约 |
| --- | --- |
| `bootstrap/setup.go` | `MADY_TRACING=stdout` 时安装 stdout tracer（instrumentation 入口） |
| `bootstrap/init_knowledge.go` | `LoadWikiStore` 等知识库加载顺序 |
| `bootstrap/init_reasoning.go` | `InitReasoningAndTemplates` 推理模板注册（长函数，顺序敏感） |
| `bootstrap/setup_steps.go` | 分步装配主流程（manifest → skills → workspace → tools） |
| `bootstrap/plantask_bridge.go` | 任务面板桥接：`Replan` 内先同步任务、再写回 `Plan`，
顺序颠倒会导致旧步骤误复用旧任务 |

## 8. Fixture 索引

| Fixture | 锁定的契约 | 代表性用例 |
| --- | --- | --- |
| `a2a/contract_test.go` | 10 个错误码、3 组枚举、信封与 `TaskUpdateEvent` JSON、端点行为、HTTP/WS 各 8 方法、SSE 帧 | `TestContractA2AEndpoints`、`TestContractA2AWSMethodRegistry`、`TestContractA2ASSEFrame` |
| `a2ui/contract_test.go` | 版本号与 catalog ID、四信封逐字 JSON、JSONL 分帧、AGUI CUSTOM 绑定 | 见文件内 `TestContractA2UI*` |
| `agui/contract_test.go` | 23 事件名、22 类事件 golden JSON、`msg_`/`thinking_` ID 序列、SSE 帧 | `TestContractAGUIEventTypeRegistry`、`TestContractAGUIEventJSONFields` |
| `acp/contract_test.go` | `protocolVersion=1`、11 方法、5 个错误码文案、`session/update` 通知帧 | `TestContractACPMethodRegistry`、`TestContractACPSessionUpdateNotification` |
| `server/contract_test.go` | 8 个流 schema、版本化响应头、39 条路由、`/api/chat` SSE 帧 | `TestContractServerRouteTable`、`TestContractServerChatSSEFrames` |
| `mcp/contract_test.go` | 11 个 RPC 名（假服务端记录 `method`）、5 个事件名 | `TestContractMCPRPCMethods` |
| `mcp/extension_snapshot_test.go` | `SnapshotEvents` 的 `transport` 取值、5 事件 `EventKind`/`EventTime` | `TestExtensionSnapshotEvents` |
| `bootstrap/plantask_bridge_test.go` | `Replan` 闭环与任务同步顺序 | `TestPlantaskBridgeKeepsCompletedStep` |

运行全部契约护栏：

```bash
go test ./a2a/ ./a2ui/ ./agui/ ./acp/ ./server/ ./mcp/ ./bootstrap/ -run 'TestContract|TestExtensionSnapshotEvents|TestPlantaskBridge' -count=1
```
