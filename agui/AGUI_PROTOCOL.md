# AGUI 协议规范

> Agent GUI Event Protocol v1 — 将 Agent 运行时事件通过 SSE 实时推送给前端 UI。

## 概述

AGUI（Agent GUI Events）是 Mady 框架定义的 Agent→UI 事件协议。它通过 **Server-Sent Events (SSE)** 将 Agent 执行过程中的每一个动作——思考、输出文本、调用工具、出错、Agent 交接——实时推送给前端消费端。

### 协议分层

```
┌─────────────────────────────────────────────────┐
│                  前端 UI (Web/桌面)                 │
│  EventSource / fetch 消费 SSE 事件流               │
├─────────────────── HTTP ─────────────────────────┤
│  POST /agui/events   → 提交请求，接收 SSE 事件流     │
│  GET  /agui/events   → 获取 Agent 能力声明          │
├─────────────────── agui.Handler ─────────────────┤
│  Converter:  agentcore.Event → agui.Event         │
├─────────────────────────────────────────────────┤
│              Agent 运行时 (agentcore)              │
│  执行 → 发射事件 → 转换 → 推送 SSE                  │
└─────────────────────────────────────────────────┘
```

### SSE 格式

每条事件由两行组成，以空行分隔：

```
event: <EVENT_TYPE>
data: <JSON_PAYLOAD>

```

Content-Type: `text/event-stream`

---

## 端点

### `POST /agui/events`

提交 Agent 执行请求，返回 SSE 事件流。

**请求体：** `RunAgentInput` JSON（见下一节）

**响应：** `text/event-stream`，事件流在 Agent 执行完毕后自动结束。

### `GET /agui/events`

返回 Agent 的能力声明（`AgentCapabilities` JSON），告知前端该 Agent 支持哪些功能（流式输出、工具调用、多 Agent 交接、多模态等）。

**响应：** `application/json`

---

## 请求体：RunAgentInput

```json
{
  "threadId": "thread-1",
  "runId": "run-42",
  "parentRunId": "run-41",
  "messages": [
    {
      "id": "msg-1",
      "role": "user",
      "content": "查询专利 CN123456"
    }
  ],
  "tools": [
    {
      "name": "patent_search",
      "description": "搜索中国专利数据库",
      "parameters": { "type": "object", "properties": { ... } }
    }
  ],
  "context": [
    { "description": "用户所在领域", "value": "专利法" }
  ],
  "state": { "phase": "analysis" },
  "resume": [
    {
      "interruptId": "int-1",
      "status": "resolved",
      "payload": { "approved": true }
    }
  ]
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `threadId` | string | 否 | 会话 ID。为空时自动生成。与 Store 配合可实现会话持久化。 |
| `runId` | string | 否 | 执行 ID。为空时自动生成。 |
| `parentRunId` | string | 否 | 父执行 ID。多轮/嵌套执行时标识执行树关系。 |
| `messages` | Message[] | 是* | 消息列表。最后一条 role=user 的消息作为输入。 |
| `tools` | ToolDef[] | 否 | 工具定义列表，覆盖或补充 Agent 内置工具。 |
| `context` | ContextEntry[] | 否 | 上下文条目，注入 Agent 的上下文窗口。 |
| `state` | any | 否 | 初始状态快照，通过 `STATE_SNAPSHOT` 事件回传给前端。 |
| `resume` | ResumeEntry[] | 否 | 中断恢复条目，用于从 `interrupt` 状态继续执行。 |

> *`messages` 至少需要包含一条 `role=user` 的消息，否则返回 `RUN_ERROR`。

### Message

```json
{
  "id": "msg-1",
  "role": "user",
  "content": "你好",
  "name": "工具名（仅 tool 消息）",
  "toolCalls": [
    {
      "id": "call-1",
      "type": "function",
      "function": { "name": "search", "arguments": "{\"q\":\"test\"}" }
    }
  ],
  "toolCallId": "call-1（仅 tool 结果消息）",
  "error": "错误信息（可选）",
  "encryptedValue": "加密值（可选）"
}
```

`role` 取值：`user` / `assistant` / `system` / `tool` / `developer`

### ToolDef

```json
{
  "name": "tool_name",
  "description": "工具描述",
  "parameters": { "type": "object", "properties": { ... } }
}
```

### ContextEntry

```json
{ "description": "上下文描述", "value": "上下文值" }
```

### ResumeEntry

```json
{
  "interruptId": "int-1",
  "status": "resolved",
  "payload": { "approved": true }
}
```

`status` 取值：`resolved` / `canceled` / `escalated`

---

## 事件类型

### 生命周期事件

#### RUN_STARTED

Agent 执行开始。

```json
{
  "type": "RUN_STARTED",
  "timestamp": 1710000000000.0,
  "threadId": "thread-1",
  "runId": "run-42",
  "parentRunId": "run-41"
}
```

#### RUN_FINISHED

Agent 执行完成。

```json
{
  "type": "RUN_FINISHED",
  "timestamp": 1710000001000.0,
  "threadId": "thread-1",
  "runId": "run-42",
  "result": "最终输出文本",
  "outcome": {
    "type": "success",
    "interrupts": []
  }
}
```

`outcome.type` 取值：
- `success` — 正常完成
- `interrupt` — 因用户审批等中断而暂停

#### RUN_ERROR

Agent 执行出错。

```json
{
  "type": "RUN_ERROR",
  "timestamp": 1710000001000.0,
  "threadId": "thread-1",
  "runId": "run-42",
  "message": "错误描述",
  "code": "ERROR_CODE"
}
```

#### STEP_STARTED

LLM 调用回合开始。

```json
{
  "type": "STEP_STARTED",
  "timestamp": 1710000000500.0,
  "stepName": "turn_1"
}
```

每轮 LLM 调用（包括工具调用后的后续轮次）都会产生一次 `STEP_STARTED`。

#### STEP_FINISHED

LLM 调用回合结束。

```json
{
  "type": "STEP_FINISHED",
  "timestamp": 1710000000950.0,
  "stepName": "turn_1"
}
```

---

### 文本消息事件

文本消息的流式输出分为三段式：`START` → `CONTENT` (零或多个) → `END`。

#### TEXT_MESSAGE_START

```json
{
  "type": "TEXT_MESSAGE_START",
  "timestamp": 1710000000600.0,
  "messageId": "msg_1",
  "role": "assistant"
}
```

#### TEXT_MESSAGE_CONTENT

```json
{
  "type": "TEXT_MESSAGE_CONTENT",
  "timestamp": 1710000000650.0,
  "messageId": "msg_1",
  "delta": "你好，"
}
```

连续多个 `TEXT_MESSAGE_CONTENT` 事件的 `messageId` 相同，`delta` 内容需拼接。

#### TEXT_MESSAGE_END

```json
{
  "type": "TEXT_MESSAGE_END",
  "timestamp": 1710000000800.0,
  "messageId": "msg_1"
}
```

---

### 推理事件

Agent 的思维链/推理过程（通常在前端以特殊样式展示，如灰色背景）。

#### THINKING_START

```json
{
  "type": "THINKING_START",
  "timestamp": 1710000000550.0,
  "thinkingId": "thinking_1",
  "title": "推理标题"
}
```

#### THINKING_TEXT_MESSAGE_START

```json
{
  "type": "THINKING_TEXT_MESSAGE_START",
  "timestamp": 1710000000560.0,
  "thinkingId": "thinking_1",
  "messageId": "msg_2"
}
```

#### THINKING_TEXT_MESSAGE_CONTENT

```json
{
  "type": "THINKING_TEXT_MESSAGE_CONTENT",
  "timestamp": 1710000000570.0,
  "thinkingId": "thinking_1",
  "messageId": "msg_2",
  "delta": "让我分析这个专利的权利要求..."
}
```

#### THINKING_TEXT_MESSAGE_END

```json
{
  "type": "THINKING_TEXT_MESSAGE_END",
  "timestamp": 1710000000590.0,
  "thinkingId": "thinking_1",
  "messageId": "msg_2"
}
```

#### THINKING_END

```json
{
  "type": "THINKING_END",
  "timestamp": 1710000000595.0,
  "thinkingId": "thinking_1"
}
```

---

### 工具调用事件

#### TOOL_CALL_START

```json
{
  "type": "TOOL_CALL_START",
  "timestamp": 1710000000700.0,
  "toolCallId": "call_1",
  "toolCallName": "patent_search",
  "parentMessageId": "msg_1"
}
```

#### TOOL_CALL_ARGS

```json
{
  "type": "TOOL_CALL_ARGS",
  "timestamp": 1710000000710.0,
  "toolCallId": "call_1",
  "delta": "{\"q\":\"CN123456\"}"
}
```

#### TOOL_CALL_END

```json
{
  "type": "TOOL_CALL_END",
  "timestamp": 1710000000800.0,
  "toolCallId": "call_1"
}
```

#### TOOL_CALL_RESULT

```json
{
  "type": "TOOL_CALL_RESULT",
  "timestamp": 1710000000900.0,
  "messageId": "tool_result_call_1",
  "toolCallId": "call_1",
  "content": "检索结果: 中国专利 CN123456...",
  "role": "tool"
}
```

---

### 状态快照事件

#### STATE_SNAPSHOT

```json
{
  "type": "STATE_SNAPSHOT",
  "timestamp": 1710000001000.0,
  "snapshot": { "messages": [...], "status": "running", "turn": 3 }
}
```

#### STATE_DELTA

```json
{
  "type": "STATE_DELTA",
  "timestamp": 1710000001000.0,
  "delta": [
    { "op": "replace", "path": "/status", "value": "finished" },
    { "op": "add", "path": "/messages/-", "value": { ... } }
  ]
}
```

`delta` 使用 JSON Patch (RFC 6902) 格式。

#### MESSAGES_SNAPSHOT

```json
{
  "type": "MESSAGES_SNAPSHOT",
  "timestamp": 1710000000000.0,
  "messages": [
    { "id": "msg-1", "role": "user", "content": "你好" },
    { "id": "msg-2", "role": "assistant", "content": "你好！有什么可以帮助你的？" }
  ]
}
```

当请求指定了 `threadId` 且 Store 中有已保存的会话历史时，Agent 会在执行开始时推送 `MESSAGES_SNAPSHOT` 事件。

---

### 自定义事件

#### CUSTOM

通用自定义事件，用于扩展协议无法覆盖的事件类型。

```json
{
  "type": "CUSTOM",
  "timestamp": 1710000000950.0,
  "name": "handoff_start",
  "value": {
    "source_agent": "router",
    "target_agent": "patent_agent",
    "mode": "transfer",
    "context": "用户询问专利审查意见"
  }
}
```

已知的自定义事件名称：

| name | 说明 |
|------|------|
| `handoff_start` | Agent 交接开始。value 含 `source_agent`、`target_agent`、`mode` |
| `handoff_end` | Agent 交接完成。value 含 `target_agent`、`output`、`duration_ms` |
| `compaction_start` | 上下文压缩开始。value 含 `tokens_before`、`context_window` |
| `compaction_end` | 上下文压缩完成。value 含 `tokens_before`、`tokens_after`、`messages_cut`、`duration_ms` |
| `auto_retry` | LLM 调用自动重试。value 含 `attempt`、`max_retries`、`delay_ms` |
| `a2ui` | A2UI 声明式 UI 信令。value 为 A2UI `Envelope` 结构。 |

#### RAW

当无法识别内部事件类型时的回退事件，透传原始事件数据。

---

## 典型事件序列

### 纯文本对话

```
RUN_STARTED
  └─ STEP_STARTED (turn_1)
       ├─ TEXT_MESSAGE_START (msg_1)
       ├─ TEXT_MESSAGE_CONTENT (msg_1, delta: "你好")
       ├─ TEXT_MESSAGE_CONTENT (msg_1, delta: "！我是Mady")
       ├─ TEXT_MESSAGE_END (msg_1)
       └─ STEP_FINISHED (turn_1)
RUN_FINISHED (success)
```

### 带推理过程

```
RUN_STARTED
  └─ STEP_STARTED (turn_1)
       ├─ THINKING_START (thinking_1)
       │   ├─ THINKING_TEXT_MESSAGE_CONTENT (thinking_1, "让我思考...")
       │   └─ THINKING_TEXT_MESSAGE_CONTENT (thinking_1, "经过分析...")
       ├─ THINKING_END (thinking_1)
       ├─ TEXT_MESSAGE_START (msg_2)
       ├─ TEXT_MESSAGE_CONTENT (msg_2, "答案是这样的")
       ├─ TEXT_MESSAGE_END (msg_2)
       └─ STEP_FINISHED (turn_1)
RUN_FINISHED (success)
```

### 带工具调用（多轮）

```
RUN_STARTED
  └─ STEP_STARTED (turn_1)
       ├─ TOOL_CALL_START (call_1, search)
       ├─ TOOL_CALL_ARGS (call_1, {"q":"..."})
       ├─ TOOL_CALL_END (call_1)
       ├─ TOOL_CALL_RESULT (call_1, "搜索结果...")
       └─ STEP_FINISHED (turn_1)
  └─ STEP_STARTED (turn_2)
       ├─ TEXT_MESSAGE_START (msg_3)
       ├─ TEXT_MESSAGE_CONTENT (msg_3, "根据检索结果...")
       ├─ TEXT_MESSAGE_END (msg_3)
       └─ STEP_FINISHED (turn_2)
RUN_FINISHED (success)
```

### 带 Human-in-the-Loop 中断

```
RUN_STARTED
  └─ STEP_STARTED (turn_1)
       └─ TOOL_CALL_START (call_1, send_email)
            └─ TOOL_CALL_RESULT (call_1, "需要用户审批")
       └─ STEP_FINISHED (turn_1)
RUN_FINISHED (interrupt, interrupts=[{id:"int-1", reason:"approval", ...}])
```

前端收到 `RUN_FINISHED` 且 `outcome.type=interrupt` 时，应暂停等待用户操作。用户操作后通过 `resume` 字段提交恢复请求。

---

## 消费示例

### curl

```bash
# 纯文本对话
curl -N -X POST http://localhost:8080/agui/events \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "你好，请介绍一下你自己"}]
  }'
```

```bash
# 带会话恢复
curl -N -X POST http://localhost:8080/agui/events \
  -H "Content-Type: application/json" \
  -d '{
    "threadId": "thread-42",
    "messages": [{"role": "user", "content": "继续分析"}],
    "resume": [{"interruptId": "int-1", "status": "resolved"}]
  }'
```

### JavaScript (EventSource)

```javascript
// 注意：POST 请求不能用 EventSource API（它只支持 GET）。
// 需要使用 fetch + ReadableStream 来消费 SSE
async function runAgent(baseUrl, input) {
  const response = await fetch(`${baseUrl}/agui/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';  // 保留不完整的行

    let currentEvent = '';
    let currentData = '';

    for (const line of lines) {
      if (line.startsWith('event: ')) {
        currentEvent = line.slice(7);
      } else if (line.startsWith('data: ')) {
        currentData = line.slice(6);
      } else if (line === '') {
        // 空行 = 事件结束
        if (currentEvent && currentData) {
          handleEvent(currentEvent, JSON.parse(currentData));
        }
        currentEvent = '';
        currentData = '';
      }
    }
  }
}

function handleEvent(type, data) {
  switch (type) {
    case 'RUN_STARTED':
      console.log(`[${data.runId}] 开始执行`);
      break;
    case 'THINKING_START':
      console.log('🤔 Agent 正在思考...');
      break;
    case 'THINKING_TEXT_MESSAGE_CONTENT':
      process.stdout.write(data.delta);  // 推理内容增量
      break;
    case 'TEXT_MESSAGE_START':
      // 新消息开始
      break;
    case 'TEXT_MESSAGE_CONTENT':
      process.stdout.write(data.delta);  // 打字机效果
      break;
    case 'TEXT_MESSAGE_END':
      process.stdout.write('\n');
      break;
    case 'TOOL_CALL_START':
      console.log(`🔧 调用工具: ${data.toolCallName}`);
      break;
    case 'TOOL_CALL_RESULT':
      console.log(`✅ 工具结果: ${data.content.slice(0, 100)}...`);
      break;
    case 'RUN_FINISHED':
      if (data.outcome?.type === 'interrupt') {
        console.log('⏸️ 需要用户审批');
      } else {
        console.log('✅ 执行完成');
      }
      break;
    case 'RUN_ERROR':
      console.error(`❌ 错误: ${data.message}`);
      break;
  }
}

// 使用示例
runAgent('http://localhost:8080', {
  messages: [{ role: 'user', content: '查询专利 CN123456' }],
});
```

### TypeScript (使用 EventTarget 封装)

```typescript
interface AgentClientOptions {
  baseUrl: string;
}

type EventHandler = (event: Record<string, unknown>) => void;

class AgentClient extends EventTarget {
  private baseUrl: string;

  constructor(options: AgentClientOptions) {
    super();
    this.baseUrl = options.baseUrl.replace(/\/$/, '');
  }

  async getCapabilities(): Promise<Record<string, unknown>> {
    const res = await fetch(`${this.baseUrl}/agui/events`);
    return res.json();
  }

  async run(input: Record<string, unknown>): Promise<void> {
    const response = await fetch(`${this.baseUrl}/agui/events`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${await response.text()}`);
    }

    const reader = response.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split('\n\n');
      buffer = parts.pop() || '';

      for (const part of parts) {
        const lines = part.split('\n');
        let eventType = '';
        let eventData = '';

        for (const line of lines) {
          if (line.startsWith('event: ')) eventType = line.slice(7);
          if (line.startsWith('data: ')) eventData = line.slice(6);
        }

        if (eventType && eventData) {
          this.dispatchEvent(new CustomEvent(eventType, {
            detail: JSON.parse(eventData),
          }));
        }
      }
    }
  }
}

// 使用
const client = new AgentClient({ baseUrl: 'http://localhost:8080' });

client.addEventListener('TEXT_MESSAGE_CONTENT', (e: CustomEvent) => {
  console.log(e.detail.delta);
});

client.addEventListener('RUN_FINISHED', (e: CustomEvent) => {
  console.log('完成:', e.detail.outcome);
});

await client.run({
  threadId: 'thread-1',
  messages: [{ role: 'user', content: '你好' }],
});
```

---

## 与 A2UI 的关系

AGUI 和 A2UI 是两个互补的协议，各自关注不同层面的 UI 交互：

| 协议 | 全称 | 关注点 | 传输方式 |
|------|------|--------|---------|
| **AGUI** | Agent GUI Events | **事件流** — Agent 执行过程中发生了什么（思考、说话、调工具） | SSE 事件流 |
| **A2UI** | Agent-to-UI | **组件声明** — 前端应该渲染什么 UI 组件（表单、表格、按钮） | JSON Lines / A2A DataPart |

**两者的关系：**
- AGUI 是"传输层"，负责将事件从后端推送到前端
- A2UI 是"渲染层"，描述前端应展示的 UI 组件结构
- A2UI 信令通过 AGUI 的 `CUSTOM` 事件（`name: "a2ui"`）传输

示例：Agent 返回一个交互式表单：

```
SSE 事件流:
  RUN_STARTED
  STEP_STARTED
  CUSTOM (name: "a2ui", value: { surfaceId: "form-1", components: [...] })
  RUN_FINISHED
```

前端收到 `a2ui` 自定义事件后，解析其中的 A2UI 组件声明并渲染。

---

## 错误处理

### Agent 配置错误

当 Agent 未配置 LLM Provider 时，返回：

```
event: RUN_ERROR
data: {"type":"RUN_ERROR","message":"no provider configured","code":""}
```

### 缺少用户消息

```
event: RUN_ERROR
data: {"type":"RUN_ERROR","message":"no user message provided","code":""}
```

### 无效请求体

HTTP 400 Bad Request:

```json
{ "error": "invalid request body" }
```

### 流式传输不支持

HTTP 500 Internal Server Error:

```json
{ "error": "streaming not supported" }
```

---

## 能力声明 (AgentCapabilities)

`GET /agui/events` 返回的 Agent 能力声明结构：

```json
{
  "identity": {
    "name": "mady-patent-agent",
    "type": "mady",
    "description": "你是一名专利代理师...",
    "version": "",
    "provider": "",
    "documentationUrl": ""
  },
  "transport": {
    "streaming": true,
    "websocket": false,
    "httpBinary": false,
    "pushNotifications": false,
    "resumable": false
  },
  "tools": {
    "supported": true,
    "items": [
      { "name": "patent_search", "description": "...", "parameters": {} }
    ],
    "parallelCalls": false,
    "clientProvided": false
  },
  "state": {
    "snapshots": true,
    "deltas": false,
    "memory": false,
    "persistentState": true
  },
  "multiAgent": {
    "supported": true,
    "delegation": true,
    "handoffs": true,
    "subAgents": [
      { "name": "patent", "description": "专利分析" }
    ]
  },
  "reasoning": {
    "supported": true,
    "streaming": true,
    "encrypted": false
  },
  "humanInTheLoop": {
    "supported": true,
    "approvals": true,
    "interrupts": true
  },
  "execution": {
    "maxIterations": 20,
    "maxExecutionTime": 0
  }
}
```

---

## 设计原则

1. **流式优先** — 所有事件都设计为可流式传输，支持打字机效果
2. **幂等消费** — 每个事件携带完整上下文（messageId、thinkingId），客户端可安全重连
3. **增量更新** — TEXT_CONTENT 和 TOOL_CALL_ARGS 使用增量 delta，减少每帧数据传输
4. **可扩展** — CUSTOM 事件为协议扩展提供无限空间，无需修改核心事件类型
5. **前后端解耦** — AGUI 只定义事件格式和传输方式，前端渲染逻辑完全由消费端决定
