# @mady/agui-client

Mady AGUI (Agent GUI Events) 协议的 TypeScript 客户端 SDK。

通过 SSE（Server-Sent Events）实时消费 Agent 运行时事件——推理过程、流式文本、工具调用、状态变化等。

## 安装

```bash
npm install @mady/agui-client
```

## 快速开始

```typescript
import { AgentClient } from "@mady/agui-client";

const client = new AgentClient({
  baseUrl: "http://localhost:8080",
});

// 注册事件监听器
client.on("TEXT_MESSAGE_CONTENT", (event) => {
  // 打字机效果：逐字输出
  process.stdout.write(event.delta);
});

client.on("THINKING_START", () => {
  process.stdout.write("\n🤔 ");
});

client.on("TOOL_CALL_START", (event) => {
  console.log(`\n🔧 调用工具: ${event.toolCallName}`);
});

client.on("RUN_FINISHED", (event) => {
  if (event.outcome?.type === "interrupt") {
    console.log("\n⏸️ 需要用户审批");
  } else {
    console.log("\n✅ 执行完成");
  }
});

client.on("RUN_ERROR", (event) => {
  console.error(`\n❌ 错误: ${event.message}`);
});

// 执行 Agent
await client.run({
  threadId: "my-session-1",
  messages: [{ role: "user", content: "查询专利 CN123456" }],
});
```

## API

### `AgentClient`

#### `constructor(options: AgentClientOptions)`

| 参数 | 类型 | 说明 |
|------|------|------|
| `baseUrl` | `string` | Mady 服务器地址，如 `http://localhost:8080` |
| `signal` | `AbortSignal` | (可选) 全局中止信号 |

#### `on(type: string, handler): this`

注册事件监听器。`type` 可以是：
- 具体事件类型：`"TEXT_MESSAGE_CONTENT"`, `"TOOL_CALL_START"` 等
- 通配符 `"*"`：接收所有事件

```typescript
client.on("TEXT_MESSAGE_CONTENT", (event) => {
  console.log(event.delta);
});

client.on("*", (event) => {
  // 所有事件都会经过这里
});
```

#### `onAny(handler): this`

注册全局监听器，等价于 `on("*", handler)`。

#### `async getCapabilities(): Promise<AgentCapabilities>`

获取 Agent 的能力声明。对应 `GET /agui/events`。

```typescript
const caps = await client.getCapabilities();
console.log(caps.tools?.items); // 可用工具列表
console.log(caps.multiAgent?.subAgents); // 子 Agent 列表
```

#### `async run(input: RunAgentInput, signal?: AbortSignal): Promise<void>`

执行 Agent。以流式模式消费 SSE 事件，已注册的监听器被逐事件调用。

```typescript
await client.run({
  messages: [{ role: "user", content: "你好" }],
  threadId: "thread-1",
});
```

#### `async runOnce(input: RunAgentInput): Promise<SSERawEvent[]>`

以非流式（一次性）方式执行 Agent，返回完整的事件列表。适用于测试或批处理场景。

```typescript
const events = await client.runOnce({
  messages: [{ role: "user", content: "hello" }],
});
console.log(events.map((e) => e.event));
// ["RUN_STARTED", "STEP_STARTED", "TEXT_MESSAGE_START", ...]
```

### 类型定义

完整的 TypeScript 类型定义在 `types.ts` 中，包括：

- **事件类型**: `AgentEvent` 联合类型（包含所有 18 种具体事件）
- **请求类型**: `RunAgentInput`, `Message`, `ToolDef`, `ResumeEntry` 等
- **能力声明**: `AgentCapabilities` 及其子类型

所有类型与 Go 后端 `agui/types.go` 保持同步。

## 事件参考

详见 [AGUI_PROTOCOL.md](../AGUI_PROTOCOL.md) 协议规范文档。

## 开发

```bash
# 构建
npm run build

# 清理
npm run clean
```
