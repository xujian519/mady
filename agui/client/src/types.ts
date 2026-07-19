// ── Event Types ────────────────────────────────────
// 与 Go agui/types.go 保持同步

export const EventType = {
  RUN_STARTED: "RUN_STARTED",
  RUN_FINISHED: "RUN_FINISHED",
  RUN_ERROR: "RUN_ERROR",
  STEP_STARTED: "STEP_STARTED",
  STEP_FINISHED: "STEP_FINISHED",
  TEXT_MESSAGE_START: "TEXT_MESSAGE_START",
  TEXT_MESSAGE_CONTENT: "TEXT_MESSAGE_CONTENT",
  TEXT_MESSAGE_END: "TEXT_MESSAGE_END",
  THINKING_START: "THINKING_START",
  THINKING_TEXT_MESSAGE_START: "THINKING_TEXT_MESSAGE_START",
  THINKING_TEXT_MESSAGE_CONTENT: "THINKING_TEXT_MESSAGE_CONTENT",
  THINKING_TEXT_MESSAGE_END: "THINKING_TEXT_MESSAGE_END",
  THINKING_END: "THINKING_END",
  TOOL_CALL_START: "TOOL_CALL_START",
  TOOL_CALL_ARGS: "TOOL_CALL_ARGS",
  TOOL_CALL_END: "TOOL_CALL_END",
  TOOL_CALL_RESULT: "TOOL_CALL_RESULT",
  STATE_SNAPSHOT: "STATE_SNAPSHOT",
  STATE_DELTA: "STATE_DELTA",
  MESSAGES_SNAPSHOT: "MESSAGES_SNAPSHOT",
  CUSTOM: "CUSTOM",
  RAW: "RAW",
} as const;

export type EventType = (typeof EventType)[keyof typeof EventType];

// ── Base ───────────────────────────────────────────

export interface BaseEvent {
  type: EventType;
  timestamp?: number;
  rawEvent?: unknown;
}

// ── Lifecycle Events ───────────────────────────────

export interface RunStartedEvent extends BaseEvent {
  type: typeof EventType.RUN_STARTED;
  threadId: string;
  runId: string;
  parentRunId?: string;
}

export interface RunFinishedOutcome {
  type: "success" | "interrupt" | string;
  interrupts?: Interrupt[];
}

export interface Interrupt {
  id: string;
  reason: string;
  message?: string;
  toolCallId?: string;
  responseSchema?: unknown;
  expiresAt?: string;
  metadata?: Record<string, unknown>;
}

export interface RunFinishedEvent extends BaseEvent {
  type: typeof EventType.RUN_FINISHED;
  threadId: string;
  runId: string;
  result?: unknown;
  outcome?: RunFinishedOutcome;
}

export interface RunErrorEvent extends BaseEvent {
  type: typeof EventType.RUN_ERROR;
  threadId: string;
  runId: string;
  message: string;
  code?: string;
}

export interface StepStartedEvent extends BaseEvent {
  type: typeof EventType.STEP_STARTED;
  stepName: string;
}

export interface StepFinishedEvent extends BaseEvent {
  type: typeof EventType.STEP_FINISHED;
  stepName: string;
}

// ── Text Message Events ────────────────────────────

export interface TextMessageStartEvent extends BaseEvent {
  type: typeof EventType.TEXT_MESSAGE_START;
  messageId: string;
  role: string;
}

export interface TextMessageContentEvent extends BaseEvent {
  type: typeof EventType.TEXT_MESSAGE_CONTENT;
  messageId: string;
  delta: string;
}

export interface TextMessageEndEvent extends BaseEvent {
  type: typeof EventType.TEXT_MESSAGE_END;
  messageId: string;
}

// ── Thinking Events ────────────────────────────────

export interface ThinkingStartEvent extends BaseEvent {
  type: typeof EventType.THINKING_START;
  thinkingId: string;
  title?: string;
}

export interface ThinkingTextMessageStartEvent extends BaseEvent {
  type: typeof EventType.THINKING_TEXT_MESSAGE_START;
  thinkingId: string;
  messageId: string;
}

export interface ThinkingTextMessageContentEvent extends BaseEvent {
  type: typeof EventType.THINKING_TEXT_MESSAGE_CONTENT;
  thinkingId: string;
  messageId: string;
  delta: string;
}

export interface ThinkingTextMessageEndEvent extends BaseEvent {
  type: typeof EventType.THINKING_TEXT_MESSAGE_END;
  thinkingId: string;
  messageId: string;
}

export interface ThinkingEndEvent extends BaseEvent {
  type: typeof EventType.THINKING_END;
  thinkingId: string;
}

// ── Tool Call Events ───────────────────────────────

export interface ToolCallStartEvent extends BaseEvent {
  type: typeof EventType.TOOL_CALL_START;
  toolCallId: string;
  toolCallName: string;
  parentMessageId?: string;
}

export interface ToolCallArgsEvent extends BaseEvent {
  type: typeof EventType.TOOL_CALL_ARGS;
  toolCallId: string;
  delta: string;
}

export interface ToolCallEndEvent extends BaseEvent {
  type: typeof EventType.TOOL_CALL_END;
  toolCallId: string;
}

export interface ToolCallResultEvent extends BaseEvent {
  type: typeof EventType.TOOL_CALL_RESULT;
  messageId: string;
  toolCallId: string;
  content: string;
  role?: string;
}

// ── State Events ───────────────────────────────────

export interface StateSnapshotEvent extends BaseEvent {
  type: typeof EventType.STATE_SNAPSHOT;
  snapshot: unknown;
}

export interface StateDeltaEvent extends BaseEvent {
  type: typeof EventType.STATE_DELTA;
  delta: JsonPatchOp[];
}

export interface JsonPatchOp {
  op: string;
  path: string;
  value?: unknown;
}

export interface MessagesSnapshotEvent extends BaseEvent {
  type: typeof EventType.MESSAGES_SNAPSHOT;
  messages: Message[];
}

// ── Custom Events ──────────────────────────────────

export interface CustomEvent extends BaseEvent {
  type: typeof EventType.CUSTOM;
  name: string;
  value?: unknown;
}

// ── Union Type ─────────────────────────────────────

export type AgentEvent =
  | RunStartedEvent
  | RunFinishedEvent
  | RunErrorEvent
  | StepStartedEvent
  | StepFinishedEvent
  | TextMessageStartEvent
  | TextMessageContentEvent
  | TextMessageEndEvent
  | ThinkingStartEvent
  | ThinkingTextMessageStartEvent
  | ThinkingTextMessageContentEvent
  | ThinkingTextMessageEndEvent
  | ThinkingEndEvent
  | ToolCallStartEvent
  | ToolCallArgsEvent
  | ToolCallEndEvent
  | ToolCallResultEvent
  | StateSnapshotEvent
  | StateDeltaEvent
  | MessagesSnapshotEvent
  | CustomEvent;

// ── Request Types ──────────────────────────────────

export type MessageRole = "user" | "assistant" | "system" | "tool" | "developer";

export interface Message {
  id?: string;
  role: MessageRole;
  content?: string;
  name?: string;
  toolCalls?: ToolCall[];
  toolCallId?: string;
  error?: string;
  encryptedValue?: string;
}

export interface ToolCall {
  id: string;
  type: string;
  function: ToolCallFunc;
}

export interface ToolCallFunc {
  name: string;
  arguments: string;
}

export interface ToolDef {
  name: string;
  description?: string;
  parameters?: unknown;
}

export interface ContextEntry {
  description?: string;
  value?: string;
}

export interface ResumeEntry {
  interruptId: string;
  status: string;
  payload?: unknown;
}

export interface RunAgentInput {
  threadId?: string;
  runId?: string;
  parentRunId?: string;
  messages?: Message[];
  tools?: ToolDef[];
  context?: ContextEntry[];
  state?: unknown;
  forwardedProps?: unknown;
  resume?: ResumeEntry[];
}

// ── Capabilities ───────────────────────────────────

export interface AgentCapabilities {
  identity?: IdentityCapabilities;
  transport?: TransportCapabilities;
  tools?: ToolsCapabilities;
  output?: OutputCapabilities;
  state?: StateCapabilities;
  multiAgent?: MultiAgentCapabilities;
  reasoning?: ReasoningCapabilities;
  multimodal?: MultimodalCapabilities;
  execution?: ExecutionCapabilities;
  humanInTheLoop?: HumanInTheLoopCapabilities;
  custom?: Record<string, unknown>;
}

export interface IdentityCapabilities {
  name?: string;
  type?: string;
  description?: string;
  version?: string;
  provider?: string;
  documentationUrl?: string;
  metadata?: Record<string, unknown>;
}

export interface TransportCapabilities {
  streaming?: boolean;
  websocket?: boolean;
  httpBinary?: boolean;
  pushNotifications?: boolean;
  resumable?: boolean;
}

export interface ToolsCapabilities {
  supported?: boolean;
  items?: ToolDef[];
  parallelCalls?: boolean;
  clientProvided?: boolean;
}

export interface OutputCapabilities {
  structuredOutput?: boolean;
  supportedMimeTypes?: string[];
}

export interface StateCapabilities {
  snapshots?: boolean;
  deltas?: boolean;
  memory?: boolean;
  persistentState?: boolean;
}

export interface MultiAgentCapabilities {
  supported?: boolean;
  delegation?: boolean;
  handoffs?: boolean;
  subAgents?: SubAgentDescriptor[];
}

export interface SubAgentDescriptor {
  name: string;
  description?: string;
}

export interface ReasoningCapabilities {
  supported?: boolean;
  streaming?: boolean;
  encrypted?: boolean;
}

export interface MultimodalCapabilities {
  input?: MultimodalInputCapabilities;
  output?: MultimodalOutputCapabilities;
}

export interface MultimodalInputCapabilities {
  image?: boolean;
  audio?: boolean;
  video?: boolean;
  pdf?: boolean;
  file?: boolean;
}

export interface MultimodalOutputCapabilities {
  image?: boolean;
  audio?: boolean;
}

export interface ExecutionCapabilities {
  codeExecution?: boolean;
  sandboxed?: boolean;
  maxIterations?: number;
  maxExecutionTime?: number;
}

export interface HumanInTheLoopCapabilities {
  supported?: boolean;
  approvals?: boolean;
  interventions?: boolean;
  feedback?: boolean;
  interrupts?: boolean;
  approveWithEdits?: boolean;
}
