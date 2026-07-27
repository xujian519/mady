# 02 — 规格：Mady 桌面端

- **功能名**：desktop
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **规格日期**：2026-07-27
- **状态**：待人工审阅
- **依赖提案**：[01-proposal.md](./01-proposal.md)

---

## 1. 数据流与进程模型

### 1.1 进程模型（单进程内嵌）

```
┌─────────────────────────────────────────────────────────────┐
│                  mady-desktop（单进程）                        │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Go 后端（desktop/app.go）                            │  │
│  │   ├─ import server/ a2ui/ agui/ agentcore/           │  │
│  │   ├─ App struct（Wails binding 暴露给前端）            │  │
│  │   │   ├─ Chat(req) → 流式事件 via Wails Events        │  │
│  │   │   ├─ ListThreads() → []Thread                    │  │
│  │   │   ├─ GetThread(key) → Thread                     │  │
│  │   │   ├─ SendAction(surfaceId, action) → ack         │  │
│  │   │   └─ Health() → {provider, knowledge, version}   │  │
│  │   └─ 启动时：server.New(cfg) → 内嵌 *http.Server      │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │ Wails IPC（同进程，非 socket）    │
│  ┌───────────────────────▼──────────────────────────────┐  │
│  │  前端（desktop/frontend/，WebView 加载）              │  │
│  │   ├─ React 18 app                                     │  │
│  │   ├─ a2ui-renderer/（A2UI → React 组件树）            │  │
│  │   ├─ agui-bridge/（Wails Events → Zustand store）     │  │
│  │   └─ 主题层（与 tui/theme 对齐的 design tokens）      │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**关键约束**：前端**不**直接 import Go 代码，**不**直接调 agentcore 内部方法。所有通信走两条通道：

| 通道 | 方向 | 用途 |
|------|------|------|
| **Wails Binding**（Go 方法 → 前端 TS 调用） | 前端 → 后端 | 发起请求（Chat/ListThreads/SendAction） |
| **Wails Events**（后端 emit → 前端 on） | 后端 → 前端 | 流式事件透传（token/工具调用/A2UI envelope） |

### 1.2 AGUI → Wails Events 透传映射

桌面端不走 HTTP SSE，而是把 `agentcore.Event` 经 `agui.Converter` 转为 AGUI 事件后，通过 Wails Events emit 到前端（绕过 WebView 的 SSE 跨域/缓冲问题）。

`agui.Converter` 输出的是标准 AGUI 事件结构（内嵌 `BaseEvent{Type EventType}`），其 `Type` 字段为 `RUN_STARTED`、`TEXT_MESSAGE_CONTENT`、`TOOL_CALL_START` 等；自定义事件（handoff、a2ui、approval prompt）使用 `CUSTOM` 类型并通过 `Name` 字段区分。`desktop/app.go` 需要在两者之间做一层**显式映射**，最终产生前端订阅的 `agui:*` 事件名。

| AGUI 事件源 | AGUI `Type` / `Name` | Wails Event 名 | 前端处理 |
|------------|----------------------|----------------|----------|
| `AgentStartEvent` | `RUN_STARTED` | `agui:agent-start` | 显示 Agent 头像/名称 |
| `TextMessageStartEvent` | `TEXT_MESSAGE_START` | `agui:text-message-start` | 初始化消息气泡（role=assistant） |
| `MessageDeltaEvent`（内容） | `TEXT_MESSAGE_CONTENT` | `agui:message-delta` | 追加 token，Motion 淡入 |
| `TextMessageEndEvent` | `TEXT_MESSAGE_END` | `agui:text-message-end` | 消息结束标记，可触发总结 |
| `ThinkingStartEvent` | `THINKING_START` | `agui:thinking-start` | 展开思考区块 |
| `ThinkingTextMessageContentEvent` | `THINKING_TEXT_MESSAGE_CONTENT` | `agui:thinking-delta` | 渲染思考过程（可折叠） |
| `ThinkingEndEvent` | `THINKING_END` | `agui:thinking-end` | 收起思考区块 |
| `ToolCallStartEvent` | `TOOL_CALL_START` | `agui:tool-call-start` | 渲染 ToolCard（展开态） |
| `ToolCallArgsEvent` | `TOOL_CALL_ARGS` | `agui:tool-call-args` | 填充 ToolCard 参数 |
| `ToolCallEndEvent` | `TOOL_CALL_END` | `agui:tool-call-end` | ToolCard 收起/状态更新 |
| `ToolCallResultEvent` | `TOOL_CALL_RESULT` | `agui:tool-call-result` | 显示工具调用结果 |
| `HandoffStartEvent` | `CUSTOM` / `handoff_start` | `agui:handoff-start` | Invisible Handoff：**不渲染** |
| `HandoffEndEvent` | `CUSTOM` / `handoff_end` | `agui:handoff-end` | Invisible Handoff：**不渲染** |
| `A2UIEvent` | `CUSTOM` / `a2ui` | `agui:a2ui` | 投递给 A2UI SurfaceStore |
| `ApprovalPromptEvent` | `CUSTOM` / `approval_prompt` | `agui:approval-prompt` | 渲染 ApprovalCard |
| `TurnStartEvent` | `STEP_STARTED` | `agui:turn-start` | 进度指示器 |
| `TurnEndEvent` | `STEP_FINISHED` | `agui:turn-end` | 进度指示器收起 |
| `CompactionStartEvent` | `CUSTOM` / `compaction_start` | `agui:compaction-start` | 上下文压缩提示 |
| `CompactionEndEvent` | `CUSTOM` / `compaction_end` | `agui:compaction-end` | 收起压缩提示 |
| `AutoRetryEvent` | `CUSTOM` / `auto_retry` | `agui:auto-retry` | 显示重试提示 |
| `AgentEndEvent` / `AgentErrorEvent` | `RUN_FINISHED` / `RUN_ERROR` | 由 App 层额外 emit `agui:done` | 结束流，释放输入框 |
| `AgentErrorEvent` | `RUN_ERROR` | `agui:error` | 错误 toast + 重试按钮 |

**映射规则**：

1. 标准事件：`wailsName = "agui:" + kebabCase(string(agEvent.Type))`，例如 `TEXT_MESSAGE_CONTENT` → `agui:text-message-content`。为与现有 HTTP SSE 约定保持一致，表格中采用更短的别名（`agui:message-delta` 等），实现时允许别名表覆盖默认 kebab-case。
2. 自定义事件：`wailsName = "agui:" + kebabCase(agEvent.Name)`，例如 `handoff_start` → `agui:handoff-start`。
3. 一次 `agentcore.Event` 经 `agui.Convert` 可能产生多个 AGUI 事件（如 `AgentEndEvent` 产生 `closeAll + RUN_FINISHED`），App 层需循环 `runtime.EventsEmit`。
4. `agui:done` 不由 `agui.Converter` 直接产出，由桌面端 `App.Chat` 在 run 结束时显式 emit，Payload 中携带 `threadId` / `runId` / `output` / `error`。

> **Invisible Handoff 红线**：`handoff-start/end` 事件在前端**必须静默**，不向用户暴露 `transfer_to_*` 工具调用，遵循 `domains/unified.go` 的统一 Agent 契约。
>
> **实现提示**：映射函数建议放在 `desktop/app.go` 中集中维护并配套单元测试，避免与 `agui/handler.go` 的 `extractEventType` 重复实现时漂移。

---

## 2. 后端接口契约（desktop/app.go）

Wails 通过结构体方法暴露后端能力。前端 TS 由 `wails generate` 自动产出对应类型。

### 2.1 App 结构体方法

```go
// desktop/app.go
type App struct {
    ctx     context.Context
    server  *server.Server
    runs    sync.Map // runId -> context.CancelFunc
}

// New 构造 App，装配 server.Server（复用 pkg/framework.Setup 导出的配置）
func New(cfg agentcore.Config) (*App, error)

// Chat 发起一轮对话，返回 runId；流式事件通过 Wails Events 推送
// 前端拿到 runId 后订阅 wails 事件直至收到 agui:done
// 内部调用 server.Server.Chat（新增公开方法），事件回调直接来自 agent 事件总线
func (a *App) Chat(req ChatRequest) (runId string, err error)

// Cancel 取消指定 runId 的流
func (a *App) Cancel(runId string) error

// SendAction 回传用户在 A2UI surface 上触发的 action（按钮点击等）
// 内部转发到 server.Server.SendAction 或当前 agent 的 A2UI client message 通道
func (a *App) SendAction(surfaceId string, action a2ui.ClientAction) error

// ListThreads 列出持久化会话
func (a *App) ListThreads() ([]ThreadSummary, error)

// GetThread 读取会话消息
func (a *App) GetThread(key string) (*Thread, error)

// DeleteThread 删除会话
func (a *App) DeleteThread(key string) error

// Health 返回运行时健康信息（provider/知识库/版本）
func (a *App) Health() (HealthInfo, error)

// --- 生命周期（Wails 调用，非前端可见）---
func (a *App) startup(ctx context.Context)    // Wails OnStartup
func (a *App) shutdown(ctx context.Context)   // Wails OnShutdown
```

### 2.1.1 server 包需新增的公开方法

当前 `server` 包的 chat 入口是 HTTP 私有 handler（`handleChat`），线程管理也只有 HTTP handlers。为使桌面端能内嵌调用，需要在 `server` 包新增以下薄封装方法（不破坏现有 HTTP API）：

```go
// server/desktop.go（新增文件）

// Chat 在非 HTTP 场景下执行一轮对话，事件通过 onEvent 回调实时返回。
// 该方法是 desktop 内嵌调用的核心入口；内部逻辑与 handleStreamChat 保持一致
//（加载 agent、注册 agent.OnAll、保存状态、emit done），但绕过 HTTP ResponseWriter。
func (s *Server) Chat(ctx context.Context, req ChatRequest, onEvent func(agentcore.Event)) (output string, err error)

// Cancel 尝试取消指定 runId 正在进行的流。实现依赖 context cancel 或 run 级 cancel 机制。
func (s *Server) Cancel(runId string) error

// SendAction 将客户端 action 投递给对应 surface 所属的 agent / A2UI handler。
func (s *Server) SendAction(surfaceId string, action a2ui.ClientAction) error

// ListThreads / GetThread / DeleteThread 暴露线程管理能力。
func (s *Server) ListThreads(ctx context.Context) ([]session.Info, error)
func (s *Server) GetThread(ctx context.Context, key string) (*session.ThreadSnapshot, error)
func (s *Server) DeleteThread(ctx context.Context, key string) error

// Health 返回运行时健康信息。
func (s *Server) Health() HealthInfo
```

> **约束**：这些方法属于 `server` 包的**新增导出 API**，不修改现有 HTTP handler 行为；`desktop/` 只消费导出方法，不访问 `server` 内部字段。

### 2.2 请求/响应类型

```go
// ChatRequest 直接复用 server.ChatRequest，避免字段漂移。
// 前端发送：{ message, thread_id?, model?, response_format?, thinking?, skills? }
type ChatRequest = server.ChatRequest

// ThreadSummary 从 session.Info 转换而来，供侧栏渲染。
type ThreadSummary struct {
    Key       string    `json:"key"`
    Title     string    `json:"title"`
    UpdatedAt time.Time `json:"updatedAt"`
    MessageN  int       `json:"messageN"`
}

type HealthInfo struct {
    Provider       string `json:"provider"`        // 当前默认 Provider 名称
    Model          string `json:"model"`           // 当前默认模型
    KnowledgeReady bool   `json:"knowledgeReady"`   // knowledge.db 是否就绪
    Version        string `json:"version"`          // commitHash + buildTime
}
```

**与现有 `server.ChatRequest` 的对齐说明**（来源 `server/types.go`）：

| 字段 | 类型 | 必填 | 桌面端语义 |
|------|------|------|-----------|
| `Message` | `string` | ✅ | 用户输入 |
| `ThreadID` | `string` | 否 | 空则新建线程；桌面端首次新建后需回显 |
| `Model` | `string` | 否 | 覆盖默认模型，用于设置面板 |
| `ResponseFormat` | `*agentcore.ResponseFormat` | 否 | 结构化输出，暂不暴露 UI |
| `Thinking` | `*agentcore.ThinkingConfig` | 否 | 推理/思考链配置 |
| `Skills` | `[]string` | 否 | 技能白名单 |
| `Stream` | `bool` | — | 桌面端固定 `true`，由 `App.Chat` 内部忽略该字段 |

> **约束**：`desktop/` 包**不重新定义**领域类型，只做 `server.*` / `a2ui.*` / `session.*` 类型的薄封装（避免与 server 包语义漂移）。

---

## 3. A2UI 渲染器规格（核心资产）

### 3.1 渲染器职责

前端实现一个 **A2UI SurfaceStore**（TS 版，对应 Go 的 `a2ui.SurfaceStore`），负责：

1. 接收 `agui:a2ui` 事件中的 envelope
2. 维护 surface 邻接表 + data model（JSON Pointer RFC 6901）
3. 把组件树映射到 React 元素树
4. 处理 `Bind` / `FunctionCall` 动态值解析
5. 收集 Input 组件的客户端数据模型（`sendDataModel` 启用时）

### 3.2 BasicCatalog v0.9.1 组件映射表（18 个）

| A2UI 组件 | React 实现 | shadcn/ui 基础 | 子组件约定 | 优先级 |
|-----------|-----------|----------------|------------|--------|
| `Text` | `<Text>` 支持 markdown 子集 | — | — | P0（阶段 2） |
| `Image` | `<img>` + lazy | — | — | P1 |
| `Icon` | `<Icon>`（lucide-react 映射） | — | — | P0 |
| `Video` | `<video controls>` | — | — | P2 |
| `AudioPlayer` | `<audio controls>` | — | — | P2 |
| `Row` | `<div flex-row>` | — | `children` 为组件 ID 数组 | P0 |
| `Column` | `<div flex-col>` | — | `children` 为组件 ID 数组 | P0 |
| `List` | 遍历 ChildList 模板 | — | `children` 支持静态数组或 template | P0 |
| `Card` | shadcn `Card` | `Card`/`CardHeader`/`CardContent` | `child` 为单组件 ID | P0 |
| `Tabs` | shadcn `Tabs` | `Tabs`/`TabsList`/`TabsTrigger`/`TabsContent` | `tabs` 为 `{child}` 对象数组 | P1 |
| `Divider` | shadcn `Separator` | `Separator` | — | P0 |
| `Modal` | shadcn `Dialog` | `Dialog`/`DialogContent` | `child` / `entryPointChild` | P1 |
| `Button` | shadcn `Button` | `Button` | `child` 为单组件 ID | P0 |
| `CheckBox` | shadcn `Checkbox` | `Checkbox` | Input 绑定 | P1 |
| `TextField` | shadcn `Input` + 自定义 | `Input` | Input 绑定 | P0 |
| `DateTimeInput` | shadcn `Calendar` + `Popover` | `Calendar`/`Popover` | Input 绑定 | P2 |
| `ChoicePicker` | shadcn `RadioGroup` 或 `Select` | `RadioGroup`/`Select` | Input 绑定 | P1 |
| `Slider` | shadcn `Slider` | `Slider` | Input 绑定 | P2 |

> **子组件约定来源**：`a2ui/catalog.go` 的 `ComponentDef`（`ChildFields` / `ChildListFields` / `NestedChildFields`）。渲染器必须按 catalog 定义解析 props，避免把组件 ID 当成普通字符串渲染。

### 3.3 动态值解析规格

A2UI 的 `Dynamic` 值有三种形态，渲染器须全部支持。**注意：前端解析必须与 Go 端 `a2ui.Dynamic` 的实际 JSON wire 格式保持一致**（见 `a2ui/dynamic.go`）。

| 形态 | Go 端 JSON 形状 | 前端解析规则 |
|------|-----------------|-------------|
| Literal（字面量） | 直接值，例如 `"hello"` / `42` / `true` / `[]` / `{}` | 直接作为属性值使用 |
| Bind（JSON Pointer 绑定） | `{"path": "/user/name"}` | 从 `SurfaceStore.dataModel` 按 RFC 6901 读取；`path` 为唯一合法键，额外键视为非法 |
| FunctionCall（函数调用） | `{"call": "formatDate", "args": {"key": value, ...}}` | 查函数表执行；`args` 为**命名参数**（`map<string, any>`）；未注册函数返回 `undefined` 并 `console.warn` |

**识别顺序**：先探测对象是否含 `"call"` 键 → 再探测是否只含 `"path"` 键 → 其余情况按字面量处理。这与 Go 端 `Dynamic.UnmarshalJSON` 完全一致。

**前端 TS 类型定义**：

```ts
type Dynamic =
  | { path: string }                          // Bind
  | { call: string; args?: Record<string, any> } // FunctionCall
  | any                                       // Literal（任意 JSON 值）

// 运行时识别函数（与 Go 端 UnmarshalJSON 顺序一致）
function classifyDynamic(value: any): { kind: 'literal' | 'bind' | 'call'; data: any } {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    if ('call' in value) return { kind: 'call', data: value }
    if (Object.keys(value).length === 1 && 'path' in value) {
      return { kind: 'bind', data: value }
    }
  }
  return { kind: 'literal', data: value }
}
```

**已实现函数（15 个，对齐 `a2ui.BasicCatalog`）**：

- 校验类：`required` `regex` `length` `numeric` `email`
- 格式化：`formatString` `formatNumber` `formatCurrency` `formatDate` `pluralize`
- 副作用：`openUrl`（调 Wails 前端打开系统浏览器；渲染器必须拦截非 `http://` / `https://` 协议）
- 逻辑：`and` `or` `not`

> **重要**：`FunctionCall.Args` 是命名参数对象而非数组。例如 `formatDate` 的调用形如 `{"call": "formatDate", "args": {"value": "2026-07-27", "format": "YYYY-MM-DD"}}`。

### 3.4 验证规则（前端可选实现）

渲染器**必须**消费 envelope，但**可选**做结构验证（开发期）。若实现，对齐 Go 端 `a2ui.ValidateEnvelope` + `ValidateSurfaceTree`：

- 每个 surface 有且仅有一个 ID 为 `root` 的组件
- ChildList 引用的 ID 必须存在（dangling ref 检测）
- 组件引用图无环（cycle detection）
- 组件 type 在 catalog 中存在

> 开发期校验失败时 console.error 并渲染占位组件，**不阻塞流**。

---

## 4. 前端模块边界

```
desktop/frontend/
├── src/
│   ├── app/                    # React 应用入口
│   │   ├── App.tsx
│   │   └── routes/             # 后期多视图预留
│   ├── a2ui-renderer/          # 🏆 核心资产
│   │   ├── catalog.ts          # CatalogRegistry + BasicCatalog
│   │   ├── store.ts            # SurfaceStore（TS 版，对齐 a2ui.SurfaceStore）
│   │   ├── renderer.tsx        # 组件树 → React 元素
│   │   ├── registry.tsx        # ComponentType → ReactComponent 注册表
│   │   ├── dynamic.ts          # Bind/FunctionCall 解析（按 path/call/literal）
│   │   ├── datamodel.ts        # JSON Pointer get/set/remove
│   │   ├── theme.ts            # A2UI theme properties → Tailwind class
│   │   ├── validate.ts         # 开发期结构校验
│   │   ├── components/         # 18 个 Basic 组件实现
│   │   │   ├── Text.tsx
│   │   │   ├── Card.tsx
│   │   │   ├── Button.tsx
│   │   │   └── ...
│   │   └── functions/          # 15 个 A2UI 函数实现
│   │       ├── format.ts
│   │       └── validate.ts
│   ├── agui-bridge/            # SSE/Wails Events → store
│   │   ├── client.ts           # 订阅 wails 事件
│   │   └── reducer.ts          # 事件 → AppState
│   ├── components/             # 业务组件（非 A2UI）
│   │   ├── ChatView.tsx        # 主聊天视图
│   │   ├── MessageBubble.tsx
│   │   ├── ToolCard.tsx        # 工具调用卡片
│   │   ├── ApprovalCard.tsx    # 审批卡片
│   │   ├── ConclusionCard.tsx  # 结论卡片
│   │   ├── ConfidenceBar.tsx   # 置信度条
│   │   └── Sidebar.tsx         # 会话列表
│   ├── stores/                 # Zustand stores
│   │   ├── chat.ts             # 聊天状态
│   │   └── threads.ts          # 会话列表
│   ├── theme/                  # 主题层（对齐 tui/theme）
│   │   ├── tokens.ts           # design tokens
│   │   └── provider.tsx        # 主题 Provider
│   └── lib/
│       ├── wails.ts            # Wails runtime 封装
│       └── utils.ts            # cn() 等工具
├── package.json
├── vite.config.ts
├── tailwind.config.ts          # Tailwind v4
├── tsconfig.json
└── index.html
```

---

## 5. 主题与设计令牌

### 5.1 设计原则：遵循 Apple Human Interface Guidelines

桌面端是 macOS 原生应用，视觉层优先遵循 **Apple HIG**，保证用户第一眼感觉是"这是 Mac 上的原生专业工具"，而不是"又一个 Web 应用套壳"。同时保留 Mady 品牌识别（图标中的紫/橙双色）。

核心原则：

1. **语义化优先**：颜色全部映射到 Apple 的语义 token（`systemBackground`、`label`、`separator` 等），自动适配浅色/深色/高对比/减弱动态效果
2. **单一 Accent**：主强调色使用 `systemIndigo`，呼应图标的深紫色；橙色作为**品牌点缀色**极低频使用
3. **材质与层次**：侧栏/标题栏使用 vibrancy（`backdrop-filter` 模拟 `NSVisualEffectView`），用层级而非边框区分空间
4. **动态响应**：默认跟随系统外观；手动切换时即时生效，不重启应用

### 5.2 颜色令牌（Light / Dark）

所有令牌给出 **Apple HIG 语义名** + **具体 HEX/RGBA**（供 Tailwind / CSS 实现）。

#### 5.2.1 背景层级（Surfaces）

| Mady Token | Apple HIG Token | Light | Dark | 用途 |
|------------|-----------------|-------|------|------|
| `--mady-bg-primary` | `systemBackground` | `#FFFFFF` | `#000000` | 主内容区 |
| `--mady-bg-secondary` | `secondarySystemBackground` | `#F2F2F7` | `#1C1C1E` | 侧栏、输入框、卡片 |
| `--mady-bg-tertiary` | `tertiarySystemBackground` | `#FFFFFF` | `#2C2C2E` | 悬浮层、下拉面板 |
| `--mady-bg-grouped` | `systemGroupedBackground` | `#F2F2F7` | `#000000` | 分组列表背景 |
| `--mady-bg-grouped-secondary` | `secondarySystemGroupedBackground` | `#FFFFFF` | `#1C1C1E` | 分组列表中的卡片 |
| `--mady-bg-material` | `NSVisualEffectView` | `rgba(255,255,255,0.72)` | `rgba(30,30,30,0.72)` | vibrancy 材质 |

#### 5.2.2 文字层级（Labels）

| Mady Token | Apple HIG Token | Light | Dark | 用途 |
|------------|-----------------|-------|------|------|
| `--mady-text-primary` | `label` | `#000000` | `#FFFFFF` | 主标题、正文 |
| `--mady-text-secondary` | `secondaryLabel` | `rgba(0,0,0,0.55)` | `rgba(255,255,255,0.55)` | 次要说明 |
| `--mady-text-tertiary` | `tertiaryLabel` | `rgba(0,0,0,0.25)` | `rgba(255,255,255,0.25)` | 占位、禁用 |
| `--mady-text-quaternary` | `quaternaryLabel` | `rgba(0,0,0,0.10)` | `rgba(255,255,255,0.10)` | 分割线文字、水印 |
| `--mady-text-link` | `link` | `#007AFF` | `#0A84FF` | 链接（跟随 systemBlue） |

#### 5.2.3 强调色与品牌色

| Mady Token | Apple HIG Token / Custom | Light | Dark | 用途 |
|------------|--------------------------|-------|------|------|
| `--mady-accent` | `systemIndigo` | `#5856D6` | `#5E5CE6` | **主强调色**：主按钮、选中态、品牌锚点（呼应图标深紫） |
| `--mady-accent-hover` | `systemIndigo` (hovered) | `#4B4AC4` | `#6E6CF0` | 按钮悬停 |
| `--mady-accent-soft` | `systemIndigo` (tint) | `rgba(88,86,214,0.08)` | `rgba(94,92,230,0.12)` | 柔和强调表面（卡片顶部渐变、选中背景） |
| `--mady-accent-glow` | `systemIndigo` (glow) | `rgba(88,86,214,0.18)` | `rgba(94,92,230,0.22)` | focus ring / 输入框聚焦光晕 |
| `--mady-accent-secondary` | Custom（图标橙） | `#F56600` | `#FF7A1A` | **品牌点缀色**：极低频使用，如置信度峰值、关键 CTA |
| `--mady-accent-tertiary` | Custom（图标深紫） | `#2E1065` | `#4C1D95` | 品牌深色背景、加载/空状态插画 |

> 品牌橙色**限制使用场景**：仅用于 (a) 最高置信度结论的视觉提示、(b) 品牌 moments（启动/关于页图标）、(c) "新增" badge。常规 UI 不使用橙色。

#### 5.2.4 语义色（Semantic）

| Mady Token | Apple HIG Token | Light | Dark | 用途 |
|------------|-----------------|-------|------|------|
| `--mady-danger` | `systemRed` | `#FF3B30` | `#FF453A` | 错误、拒绝、删除 |
| `--mady-success` | `systemGreen` | `#34C759` | `#30D158` | 成功、批准、通过 |
| `--mady-warning` | `systemOrange` | `#FF9500` | `#FF9F0A` | 警告、待审、需关注 |
| `--mady-info` | `systemBlue` | `#007AFF` | `#0A84FF` | 信息提示、链接 |

#### 5.2.5 分割线与边框

| Mady Token | Apple HIG Token | Light | Dark | 用途 |
|------------|-----------------|-------|------|------|
| `--mady-separator` | `separator` | `rgba(0,0,0,0.10)` | `rgba(255,255,255,0.10)` | 默认分割线 |
| `--mady-separator-strong` | `opaqueSeparator` | `#C6C6C8` | `#38383A` | 强分割线（拖拽把手） |
| `--mady-border` | `separator` | `rgba(0,0,0,0.10)` | `rgba(255,255,255,0.10)` | 输入框/卡片边框 |

#### 5.2.6 布局与排版令牌

| Mady Token | Light | Dark | 用途 |
|------------|-------|------|------|
| `--mady-radius-sm` | `6px` | `6px` | 按钮、输入框 |
| `--mady-radius-md` | `8px` | `8px` | 卡片、会话项 |
| `--mady-radius-lg` | `10px` | `10px` | 窗口/浮层面板 |
| `--mady-sidebar-width` | `260px` | `260px` | 侧栏默认宽度，可拖拽 200~400px |
| `--mady-context-width` | `320px` | `320px` | 右侧面板默认宽度 |
| `--mady-titlebar-height` | `38px` | `38px` | macOS 标题栏高度 |
| `--mady-text-caption` | `11px` | `11px` | 时间戳、badge、区段标题 |
| `--mady-text-small` | `12px` | `12px` | 侧栏标签、表单提示 |
| `--mady-text-ui` | `13px` | `13px` | 主 UI 文字（macOS 基准） |
| `--mady-text-body` | `14px` | `14px` | 聊天正文、阅读内容 |
| `--mady-text-heading` | `17px` | `17px` | 区段标题（H2 等价） |
| `--mady-text-h1` | `22px` | `22px` | 页面标题（H1 等价） |

### 5.3 材质（Vibrancy）

在 WebView 中通过 CSS `backdrop-filter` 近似 Apple 原生材质：

```css
.mady-material {
  background: var(--mady-bg-material);
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
}
```

应用材质的区域：

| 区域 | 材质类型 | HIG 参考 |
|------|----------|----------|
| 侧栏 | `sidebar` | `NSVisualEffectView.Material.sidebar` |
| 标题栏/工具栏 | `headerView` | `NSVisualEffectView.Material.headerView` |
| 模态面板背景 | `popover` | `NSVisualEffectView.Material.popover` |
| 菜单/自动完成浮层 | `menu` | `NSVisualEffectView.Material.menu` |

> 降低动态效果（`prefers-reduced-motion` / 系统减弱动态效果）时，材质降级为纯色背景，取消 blur。

### 5.4 字体排版

| 用途 | 字体 | 字号 | 字重 | 行高 | HIG 参考 |
|------|------|------|------|------|----------|
| UI 主文字 | Inter / SF Pro / system-ui | 13px | 400 | 1.4 | `body` |
| UI 小文字 | Inter / SF Pro | 11px | 400 | 1.3 | `caption1` |
| 聊天消息正文 | Inter / SF Pro | 14px | 400 | 1.5 | — |
| 专利文档正文 | Georgia / `Songti SC` | 15px | 400 | 1.6 | — |
| 代码块 | JetBrains Mono / SF Mono | 13px | 400 | 1.5 | — |
| 标题 H1 | Inter / SF Pro | 22px | 600 | 1.2 | `title1` |
| 标题 H2 | Inter / SF Pro | 17px | 600 | 1.25 | `title2` |
| 标题 H3 | Inter / SF Pro | 15px | 600 | 1.3 | `title3` |

> SF Pro 不可直接分发，开发环境可用 `-apple-system` 回退，打包/截图使用 Inter 保证跨平台一致。

### 5.5 圆角与阴影

| 元素 | 圆角 | 阴影 | HIG 参考 |
|------|------|------|----------|
| 主窗口 | 10px（Wails 默认 macOS 圆角） | 系统窗口阴影 | — |
| 侧栏/主内容卡片 | 6-8px | 无 | `cornerRadius` |
| 按钮 | 6px（shadcn 默认） | 无 | — |
| 输入框 | 6px | 无 | — |
| 悬浮面板/模态 | 10px | 0 4px 24px rgba(0,0,0,0.15) | `popover` |
| 工具提示 | 6px | 0 2px 12px rgba(0,0,0,0.12) | `tooltip` |

> 阴影在深色模式下减半透明度，遵循 HIG "depth is communicated through luminance contrast" 原则。

### 5.6 深浅色模式

- **默认**：跟随系统 `prefers-color-scheme`
- **手动切换**：设置面板提供「跟随系统 / 浅色 / 深色」三档
- **持久化**：写入 `~/.mady/desktop-settings.json`
- **切换响应**：CSS 变量通过 `<html data-theme="light|dark">` 切换，无需重启
- **无障碍**：支持高对比模式（`prefers-contrast: more` 时边框、分割线加深）

### 5.7 图标

- **App Icon**：复用 `YunPat-Ai/AppIcon.appiconset`，已复制到 `desktop/build/appicon.png`（1024×1024）及完整 iconset
- **UI 图标**：使用 **SF Symbols**（Apple HIG 标准）或 **Lucide React** 作为跨平台回退
- **SF Symbols 使用原则**：
  - 工具栏/导航：填充轮廓（outline）风格
  - 状态指示：线宽 1.5-2pt
  - 避免用 Symbol 作为品牌标识（品牌用 App Icon）

### 5.8 与 TUI 的关系

桌面端**不直接复用** TUI 的 8-bit 色板，但在语义上保持一致：

| TUI 语义 | 桌面端映射 | 说明 |
|----------|-----------|------|
| `primary` | `--mady-accent` | 品牌强调 |
| `success` | `--mady-success` | 成功 |
| `warning` | `--mady-warning` | 警告 |
| `danger` | `--mady-danger` | 危险 |
| `muted` | `--mady-text-tertiary` | 弱化 |

> 未来可统一抽取 `packages/design-tokens` 共享给 TUI/Server/桌面端，本期桌面端独立实现以控制范围。

---

## 6. 验证规则

### 6.1 类型契约同步

- `wails generate module` 自动产出 `frontend/wailsjs/` 目录（Go 方法 → TS 声明）
- 前端 import 这些声明，编译期保证字段名/类型与后端一致
- **禁止手写 TS 声明覆盖生成的文件**（避免漂移）

### 6.2 测试矩阵

| 层 | 工具 | 覆盖目标 |
|----|------|---------|
| Go 单元测试 | `go test` | App 方法、生命周期、事件透传 |
| Go 集成测试 | `go test` | 启动 App → 发 Chat → 收到 agui:done |
| TS 单元测试 | Vitest | SurfaceStore、datamodel、dynamic 解析、18 组件渲染 |
| e2e | Playwright（WebView 模式） | AC-1~AC-5 关键路径 |
| 类型检查 | `tsc --noEmit` | 前后端类型契约 |

### 6.3 冒烟测试（CI 必过）

1. `desktop/` 模块 `go build ./...` 通过
2. `desktop/frontend/` `pnpm typecheck && pnpm build` 通过
3. `make desktop-run`（开发模式）能启动窗口
4. 注入 mock provider，发送一条消息，前端收到 `agui:message-delta` × N + `agui:done`

---

## 7. 配置项

### 7.1 环境变量（沿用现有 + 新增）

| 变量 | 用途 | 默认 |
|------|------|------|
| `PROVIDER` / `API_KEY` / `BASE_URL` | 沿用 `pkg/agentconfig` | — |
| `MADY_HOME` | 沿用 | `~/.mady` |
| `MADY_DESKTOP_PORT` | 开发模式 WebView 调试端口 | `34915`（Wails 默认） |
| `MADY_DESKTOP_DEV` | 开发模式（加载 localhost 而非 embed） | `false` |

### 7.2 macOS 打包配置（已决策）

| 项 | 决策值 | 说明 |
|----|--------|------|
| App Bundle ID | `com.mady.desktop` | 标准反向域名命名 |
| 应用名称（显示） | `Mady` | 菜单栏/Dock/About 显示 |
| 图标 | 复用 `YunPat-Ai/AppIcon.appiconset` | 已复制到 `desktop/build/appicon.png`（1024×1024）及完整 iconset；紫/橙抽象图形 |
| 签名证书 | 阶段 3 用 ad-hoc（`codesign -s -`） | 阶段 5 接入 Apple Developer ID |
| 公证 | 阶段 5 接 `notarytool` | 本期不做 |
| 自动更新 | 不做 | 阶段 5 再评估 Sparkle |
| 系统托盘 / Dock badge | 不做 | 阶段 3 不实现，代码路径预留 |

---

## 8. 已澄清问题汇总

| # | 问题 | 决策 | 影响 |
|---|------|------|------|
| Q1 | 品牌强调色是否沿用 TUI 色板 | **按 Apple HIG 重新设计**：主 accent 为 `systemIndigo`（呼应图标深紫），橙色作为品牌点缀色极低频使用 | 主题层 §5 |
| Q2 | macOS App Bundle ID / 应用名 | `com.mady.desktop` / `Mady` | 打包 |
| Q3 | 图标设计来源 | 复用 `YunPat-Ai/AppIcon.appiconset`；已复制到 `desktop/build/` | 打包、品牌 |
| Q4 | 是否支持自动更新（Sparkle 等） | **暂时不做**，阶段 5 再评估 | 阶段范围 |
| Q5 | 是否需要系统托盘 / Dock badge | **暂时不做**，阶段 3 预留 | UX、阶段范围 |
| Q6 | Wails 版本锁定策略 | 锁定 v2 最新稳定 tag；构建前验证 Go 1.26 兼容性，不兼容则临时降 go.mod 到 1.24 | 构建 |
| Q7 | 分屏文档预览 PDF 渲染方案 | **HTML 内嵌 + PDF 外部打开**：法条/模板等可控内容在 WebView 内渲染 HTML；正式 PDF 调用系统默认 PDF 应用（macOS Preview / QuickLook）；阶段 5 再评估 pdf.js 内嵌 | T3.1 DocumentViewer |
| Q8 | 项目树是否允许用户操作 | **可读写**：用户可在侧栏项目树中新建、重命名文件夹，但文件操作受沙箱边界约束（复用 `tools/path.go` 路径校验）；阶段 3 先实现文件/文件夹的创建与重命名，删除操作阶段 5 再做 | T3.2b ProjectTree |
| Q9 | Provider/Model 切换策略 | **全局切换 + 新会话生效**：写入 `pkg/agentconfig` 全局配置，已有会话保持原有模型；切换时弹 Toast 提示；不改造 session 存储 | T3.6 设置面板 |
| Q10 | Invisible Handoff 过滤 | **确认作为设计契约**：ToolCard 过滤 `transfer_to_*` 类型工具调用；AGUI bridge 对 `handoff-start/end` 事件不渲染；审查等级 L3 | T3.2 ToolCard / §1.2 |


---

## 9. 下一步

人工审阅本规格并澄清 §8 问题后，进入 [03-design.md](./03-design.md)（技术设计与架构）。
