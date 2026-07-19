import { SSEStreamConsumer, parseSSEBody, SSERawEvent } from "./parser.js";
import { AgentCapabilities, AgentEvent, RunAgentInput } from "./types.js";

export interface AgentClientOptions {
  /** Mady server base URL (e.g. "http://localhost:8080") */
  baseUrl: string;
  /**
   * 可选的 AbortSignal 用于取消进行中的请求。
   * 调用 run() 时可传入单独的 signal 覆盖此全局配置。
   */
  signal?: AbortSignal;
}

/**
 * Mady AGUI 客户端。
 *
 * 封装了 SSE 事件流的消费逻辑，提供类型安全的 Agent 交互接口。
 *
 * @example
 * ```typescript
 * const client = new AgentClient({ baseUrl: "http://localhost:8080" });
 *
 * // 消费事件流
 * client.on("TEXT_MESSAGE_CONTENT", (event) => {
 *   process.stdout.write(event.delta);
 * });
 * client.on("RUN_FINISHED", (event) => {
 *   console.log("\n✅ 完成");
 * });
 *
 * // 执行 Agent
 * await client.run({
 *   messages: [{ role: "user", content: "你好" }],
 * });
 * ```
 */
export class AgentClient {
  private options: AgentClientOptions;
  private consumer: SSEStreamConsumer;

  constructor(options: AgentClientOptions) {
    this.options = { ...options };
    this.consumer = new SSEStreamConsumer();
  }

  // ── Event Registration ───────────────────────────

  /** 注册事件监听器 */
  on(type: string, handler: (event: AgentEvent) => void): this {
    this.consumer.on(type, handler);
    return this;
  }

  /** 注册全局事件监听器（接收所有事件） */
  onAny(handler: (event: AgentEvent) => void): this {
    this.consumer.on("*", handler);
    return this;
  }

  /** 移除事件监听器 */
  off(type: string, handler: (event: AgentEvent) => void): this {
    this.consumer.off(type, handler);
    return this;
  }

  // ── API Methods ──────────────────────────────────

  /**
   * 获取 Agent 能力声明。
   * GET /agui/events → AgentCapabilities
   */
  async getCapabilities(): Promise<AgentCapabilities> {
    const url = `${this.options.baseUrl}/agui/events`;
    const res = await fetch(url);
    if (!res.ok) {
      throw new Error(
        `AGUI capabilities request failed: ${res.status} ${res.statusText}`,
      );
    }
    return res.json() as Promise<AgentCapabilities>;
  }

  /**
   * 执行 Agent。
   *
   * 以流式模式消费 SSE 事件。已注册的监听器在事件到达时被逐事件调用。
   * 如果 options.signal 或 signal 参数被触发，请求被中止。
   *
   * @param input - RunAgentInput
   * @param signal - 可选的 AbortSignal，覆盖构造时的全局配置
   */
  async run(
    input: RunAgentInput,
    signal?: AbortSignal,
  ): Promise<void> {
    const abortSignal = signal || this.options.signal;
    const url = `${this.options.baseUrl}/agui/events`;

    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      signal: abortSignal,
    });

    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(
        `AGUI run request failed: ${res.status} ${res.statusText}\n${text}`,
      );
    }

    if (!res.body) {
      throw new Error("AGUI response has no body (streaming not supported)");
    }

    await this.consumer.read(res.body.getReader(), abortSignal);
  }

  /**
   * 以非流式（一次性）方式执行 Agent。
   * 适用于测试或不需要实时事件的场景。
   * 将完整响应体解析后返回事件列表。
   */
  async runOnce(input: RunAgentInput): Promise<SSERawEvent[]> {
    const url = `${this.options.baseUrl}/agui/events`;
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });

    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(
        `AGUI run request failed: ${res.status} ${res.statusText}\n${text}`,
      );
    }

    const body = await res.text();
    return parseSSEBody(body);
  }
}
