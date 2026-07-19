import { AgentEvent, EventType } from "./types.js";

/**
 * 解析 SSE 文本行，提取 event 类型和 data JSON。
 *
 * SSE 格式:
 *   event: EVENT_TYPE\n
 *   data: {"json": "payload"}\n
 *   \n
 */
export interface SSERawEvent {
  event: string;
  data: string;
}

/**
 * 将一块 SSE 响应体按空行分隔解析为原始事件列表。
 * 适用于一次性解析（非流式），如读取完整的测试响应。
 */
export function parseSSEBody(body: string): SSERawEvent[] {
  const events: SSERawEvent[] = [];
  const blocks = body.split("\n\n");

  for (const block of blocks) {
    if (!block.trim()) continue;
    const lines = block.split("\n");
    let event = "";
    let data = "";

    for (const line of lines) {
      if (line.startsWith("event: ")) {
        event = line.slice(7);
      } else if (line.startsWith("data: ")) {
        data = line.slice(6);
      }
    }

    if (event) {
      events.push({ event, data });
    }
  }

  return events;
}

/**
 * 将 SSERawEvent 反序列化为强类型的 AgentEvent。
 *
 * 类型信息来自 SSE 的 event: 行（优先）或 data JSON 中的 type 字段。
 * 所有 AGUI 事件类型共享相同的反序列化逻辑——JSON.parse +
 * type 覆盖——因为 Go 端输出已经保证了字段结构的正确性，
 * 无需按类型做不同的转换分派。
 */
export function deserializeEvent(raw: SSERawEvent): AgentEvent {
  const parsed = JSON.parse(raw.data);
  const type = raw.event || parsed.type || EventType.CUSTOM;
  return { ...parsed, type } as AgentEvent;
}

export type SSEEventHandler = (event: AgentEvent) => void;

/**
 * SSE 流式消费者。
 * 从 ReadableStream<Uint8Array> 中持续读取并解析 SSE 事件。
 *
 * 用法:
 *   const consumer = new SSEStreamConsumer();
 *   consumer.on("TEXT_MESSAGE_CONTENT", (event) => console.log(event.delta));
 *   await consumer.read(response.body!.getReader());
 */
export class SSEStreamConsumer {
  private handlers = new Map<string, SSEEventHandler[]>();

  /** 注册指定事件类型的监听器 */
  on(type: string, handler: SSEEventHandler): void {
    const list = this.handlers.get(type) || [];
    list.push(handler);
    this.handlers.set(type, list);
  }

  /** 移除指定事件类型的监听器 */
  off(type: string, handler: SSEEventHandler): void {
    const list = this.handlers.get(type) || [];
    this.handlers.set(
      type,
      list.filter((h) => h !== handler),
    );
  }

  /** 从 ReadableStreamDefaultReader 读取并消费 SSE 事件 */
  async read(
    reader: ReadableStreamDefaultReader<Uint8Array>,
    signal?: AbortSignal,
  ): Promise<void> {
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      if (signal?.aborted) break;

      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split("\n\n");
      buffer = parts.pop() || "";

      for (const part of parts) {
        if (!part.trim()) continue;
        const lines = part.split("\n");
        let eventType = "";
        let jsonData = "";

        for (const line of lines) {
          if (line.startsWith("event: ")) eventType = line.slice(7);
          if (line.startsWith("data: ")) jsonData = line.slice(6);
        }

        if (eventType && jsonData) {
          const agentEvent = deserializeEvent({
            event: eventType,
            data: jsonData,
          });
          this.dispatch(agentEvent);
        }
      }
    }
  }

  private dispatch(event: AgentEvent): void {
    // 类型化监听器
    const list = this.handlers.get(event.type) || [];
    for (const handler of list) {
      try {
        handler(event);
      } catch (err) {
        console.error(`AGUI handler error for ${event.type}:`, err);
      }
    }

    // 全局监听器（通配符 *）
    const allList = this.handlers.get("*") || [];
    for (const handler of allList) {
      try {
        handler(event);
      } catch (err) {
        console.error(`AGUI * handler error for ${event.type}:`, err);
      }
    }
  }
}
