# 03 — 设计：Mady 桌面端

- **功能名**：desktop
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **设计日期**：2026-07-27
- **状态**：待人工审阅
- **依赖规格**：[02-spec.md](./02-spec.md)

---

## 1. 技术选型

### 1.1 桌面框架对比（决策依据）

| 维度 | Wails v2 | Tauri v2 | Electron | Fyne / Gio |
|------|:---:|:---:|:---:|:---:|
| 后端语言 | **Go 原生** | Rust | Node.js | Go 原生 |
| 与现有 Go 集成 | ⭐⭐⭐⭐⭐ 直接 import | ⭐⭐ sidecar | ⭐⭐ sidecar | ⭐⭐⭐⭐⭐ 直接 import |
| 富文本/PDF 渲染 | ⭐⭐⭐⭐⭐ WebView | ⭐⭐⭐⭐⭐ WebView | ⭐⭐⭐⭐⭐ Chromium | ⭐⭐ 弱 |
| 打包体积 | ~15-25MB | ~5-10MB | ~80-150MB | ~20-30MB |
| 内存占用 | 低-中 | 低 | 高 | 低 |
| 引入新编译链 | **无** | Rust 工具链 | Node 全家桶 | 无 |
| 契合"克制"哲学 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |
| A2UI 适配 | ⭐⭐⭐⭐⭐ Web 渲染 | ⭐⭐⭐⭐⭐ Web 渲染 | ⭐⭐⭐⭐⭐ Web 渲染 | ⭐⭐ 需自实现 |

**决策：Wails v2**。

理由：

1. **零生态扩张**——后端仍是 Go，可直接 `import server/ a2ui/ agui/ agentcore/`，无 sidecar、无 IPC 序列化层
2. **契合项目哲学**——"克制、中庸、去繁就简"，不引入 Rust/Node 编译链，保持 `go build` 一键产出
3. **A2UI 承接完美**——WebView 原生渲染声明式 UI，前端只写一个 A2UI → React 渲染器
4. **体积可控**——15-25MB，与现有 TUI 二进制量级相当

**为何不选 Tauri**：体积更小（5-10MB）但需把 `mady serve` 作为 sidecar，多一层 IPC，且引入 Rust 工具链与现有 Go-only 工作流冲突。

**为何不选 Fyne/Gio**：富文本和 PDF 渲染太弱，专利文档场景（权利要求书/说明书）会非常吃力，且放弃 A2UI 的 Web 渲染优势。

**为何不选 Electron**：体积 5-10 倍于 Wails/Tauri，与"克制"哲学严重冲突。

**版本锁定**：选用 **Wails v2.12.x**。该版本修复了 clipboard、WKWebView stability、Linux panics 等已知问题，且与 React 18 / Vite 5.4.x 的兼容性经过社区验证。Wails v3 仍在 alpha，本期不采用。

### 1.2 前端框架对比（"最美观"维度）

| 维度 | React 18 | Svelte 5 | Vue 3.5 |
|------|:---:|:---:|:---:|
| 美观天花板 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| 顶级美观产品参考 | Linear/Raycast/Arc/Vercel/Stripe/Notion | 极少 | GitLab/Nova |
| 动画天花板 | ⭐⭐⭐⭐ Motion | ⭐⭐⭐⭐⭐ 内置 | ⭐⭐⭐ |
| 顶级组件库 | shadcn/ui / Geist | Skeleton v3 | Nuxt UI / PrimeVue |
| A2UI 渲染器实现成本 | 中 | 低（响应式天然契合） | 中 |
| 生态/求助容易度 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |

**决策：React 18 + shadcn/ui + Tailwind v4 + Motion**。

理由：

1. **Linear / Raycast / Arc / Vercel 美学的同款技术栈**——当今"专业精致"美学标杆产品几乎清一色用这套，Mady 作为专业工具要的就是这种"克制、可信、精致"气质
2. **shadcn/ui "复制粘贴"哲学**契合 Mady 克制定位——组件代码进仓库而非依赖，可逐像素定制，与 TUI 主题系统（`tui/theme/`）打通
3. **A2UI 渲染器实现最成熟**——React 是声明式 UI 协议的事实标准宿主，社区参考实现最多
4. **Tailwind v4 + Motion 动画天花板足够**——细微入场过渡、列表展开、流式 token 淡入，是"高级感"关键

### 1.3 状态管理与数据层

| 层 | 选型 | 理由 |
|----|------|------|
| 服务端状态 | **TanStack Query** | 缓存/ListThreads/GetThread 天然契合，重试/失效内置 |
| 客户端状态 | **Zustand** | 轻量（<1KB），无 Redux 样板，契合 SSE 数据流 reducer 模式 |
| SSE 事件流 | **Wails Events**（非 EventSource） | 绕过 WebView 跨域与缓冲，同进程更稳 |

**为何不用 Redux**：专利桌面工具状态不复杂，Redux 样板成本 > 收益。

**为何不用 Jotai/Recoil**：原子化状态对 A2UI 的 surface 树状结构不友好。

### 1.4 构建与工具链版本

| 项 | 选型 | 锁定版本 | 说明 |
|----|------|----------|------|
| 前端构建 | Vite | **5.4.x** | Wails v2 对 Vite 7+ dev server 曾有兼容问题（issue #4620），5.4.x 为当前最稳选择 |
| 前端框架 | React | **18.3.x** | Wails v2 React-TS 模板验证版本；暂不升级 React 19，避免绑定生成工具不兼容 |
| 样式 | Tailwind CSS | **4.2.x** | 使用 `@tailwindcss/vite` 插件；`@theme` 指令定义 design tokens |
| 组件库 | shadcn/ui | latest | 复制粘贴进仓库，可逐像素定制 |
| 动画 | Motion（framer-motion） | latest | 流式 token 淡入、列表过渡 |
| 状态 | Zustand + TanStack Query | v5 | 客户端 + 服务端状态分层 |
| 包管理 | pnpm | latest | 磁盘节省，monorepo 友好 |
| Go 构建 | 标准 `go build` | Go 1.26 | Wails CLI 触发；若构建失败可临时降级到 1.24 验证 |
| 桌面框架 | Wails v2 | **2.12.x** | 见 §1.1 版本锁定说明 |

> **注意**：`desktop/frontend/package.json` 应显式声明 `vite: "^5.4.0"`，避免 Wails CLI 默认模板拉取到不兼容的 Vite 6/7。

---

## 2. 架构与模块边界

### 2.1 多模块工作区扩展

`desktop/` 成为 `go.work` 的第 4 个模块：

```
go.work
├── .          # 根模块（agentcore/server/a2ui/agui/...）
├── ./tools    # 工具子模块
├── ./tui      # TUI 子模块
└── ./desktop  # 🆕 桌面端模块
```

`desktop/go.mod`：

```go
module github.com/xujian519/mady/desktop

go 1.26

require (
    github.com/xujian519/mady v0.0.0-00010101000000-000000000000
    github.com/wailsapp/wails/v2 v2.x.x
)

replace github.com/xujian519/mady => ../
```

### 2.2 依赖方向（单向，符合项目分层）

```
desktop/  ──imports──▶  server/  agentcore/  a2ui/  agui/
                              │
                              ▼
                    （现有分层不变）
```

**红线**：

- `desktop/` **不得** import `tui/`——两者是并列的 UI 通道，不互相依赖
- `desktop/` **不得** import `tools/` 内部包——工具能力通过 agentcore 扩展机制间接消费
- 现有模块**不得**反向 import `desktop/`（否则破坏分层）

### 2.3 Wails 应用骨架

```go
// desktop/main.go
package main

import (
    "context"
    "embed"
    _ "github.com/joho/godotenv/autoload"
    "github.com/wailsapp/wails/v2"
    "github.com/wailsapp/wails/v2/pkg/options"
    "github.com/wailsapp/wails/v2/pkg/options/assetserver"
    "github.com/wailsapp/wails/v2/pkg/options/mac"
    "github.com/xujian519/mady/pkg/framework"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    ctx := context.Background()
    fc, err := framework.Setup(ctx, framework.Options{Mode: framework.Sync})
    if err != nil { panic(err) }

    app, err := NewApp(fc.BaseConfig)
    if err != nil { panic(err) }

    err = wails.Run(&options.App{
        Title:     "Mady",
        Width:     1200, Height: 800,
        MinWidth:  900, MinHeight: 600,
        AssetServer: &assetserver.Options{Assets: assets},
        OnStartup:  app.startup,
        OnShutdown: app.shutdown,
        Bind: []interface{}{app},
        Mac: &mac.Options{
            TitleBar: mac.TitleBarHiddenInsetUnified(),
            // About / Preferences 菜单项后续接入
        },
    })
    if err != nil { panic(err) }
}
```

### 2.4 事件透传机制（关键设计）

桌面端不走 HTTP SSE，而是把 `agentcore.Event` 经 `agui.Converter` 转为 AGUI 事件后，通过 Wails Runtime emit 到前端。事件回调直接来自 **agent 事件总线**（chat 执行时注册在 `agent.OnAll` 上），而非 `server.eventBus`。

```go
// desktop/app.go（事件透传核心）

type App struct {
    ctx    context.Context
    server *server.Server
    runs   sync.Map // runId -> context.CancelFunc
}

// mapAguiEventToWailsName 将 AGUI 事件映射为前端订阅的 Wails 事件名。
// 标准事件：agui: + kebab-case(EventType)
// 自定义事件：agui: + kebab-case(CustomEvent.Name)
func mapAguiEventToWailsName(ev any) string {
    switch e := ev.(type) {
    case agui.RunStartedEvent:
        return "agui:agent-start"
    case agui.TextMessageContentEvent:
        return "agui:message-delta"
    case agui.ThinkingTextMessageContentEvent:
        return "agui:thinking-delta"
    case agui.ToolCallStartEvent:
        return "agui:tool-call-start"
    case agui.ToolCallEndEvent:
        return "agui:tool-call-end"
    case agui.CustomEvent:
        return "agui:" + toKebabCase(e.Name) // handoff_start, a2ui, approval_prompt, ...
    case agui.RunErrorEvent:
        return "agui:error"
    // ... 其余标准事件同理
    default:
        return "agui:" + toKebabCase(extractEventType(ev))
    }
}

// emitAguiEvents 把单个 AGUI 事件或一组 AGUI 事件 emit 到前端。
func (a *App) emitAguiEvents(ctx context.Context, events ...any) {
    for _, ev := range events {
        name := mapAguiEventToWailsName(ev)
        runtime.EventsEmit(ctx, name, ev)
    }
}

// Chat 内部使用 server.Server.Chat，并通过 onEvent 回调接收 agent 事件。
func (a *App) Chat(req ChatRequest) (string, error) {
    runID := generateRunID()
    ctx, cancel := context.WithCancel(a.ctx)
    a.runs.Store(runID, cancel)

    output, err := a.server.Chat(ctx, toServerChatRequest(req), func(e agentcore.Event) {
        aguiEvents := agui.Convert(e)
        a.emitAguiEvents(a.ctx, aguiEvents...)
    })

    // 无论成功失败，都需要向前端发送 agui:done，释放输入框
    done := map[string]any{
        "runId":   runID,
        "threadId": req.ThreadKey,
        "output":  output,
    }
    if err != nil {
        done["error"] = err.Error()
    }
    runtime.EventsEmit(a.ctx, "agui:done", done)
    return runID, err
}
```

**关键说明**：

1. **事件源是 agent 总线**：`server.Server.Chat` 内部会注册 `agent.OnAll` 回调并把事件传出来；仅订阅 `server.OnAll()` 会收不到 chat 消息。
2. **AGUI → Wails 事件名映射**：`agui.Converter` 输出的是标准 `agui.EventType`（如 `RUN_STARTED`、`TEXT_MESSAGE_CONTENT`）或 `CUSTOM` 类型，需要显式映射为 `agui:*` 事件名。
3. **一次 agentcore.Event 可能产生多个 AGUI 事件**：如 `AgentEndEvent` 会生成 `closeAll + RUN_FINISHED`，需循环 `runtime.EventsEmit`。
4. **`agui:done` 自行 emit**：`agui.Converter` 不产出 done 事件，由 `App.Chat` 在 run 结束时显式发送。
5. **取消机制**：`App.Cancel(runId)` 调用保存的 `cancel()` 函数，通过 context cancel 终止 `server.Chat`。

**为何不走 HTTP SSE**：

1. WebView 内 `EventSource` 跨域/混合内容限制多
2. WebView 厂商对 SSE 缓冲策略不一（macOS WKWebView 对 `text/event-stream` 有已知 buffering 问题）
3. Wails 同进程 IPC 比 HTTP loopback 更快、更可靠

### 2.5 前端启动与事件消费

```ts
// frontend/src/agui-bridge/client.ts
import { EventsOn } from '../wailsjs/runtime/runtime'

export function subscribeAguiEvents(store: ChatStore) {
  EventsOn('agui:message-delta', (p: MessageDelta) => {
    store.appendToken(p.delta)
  })
  EventsOn('agui:a2ui', (p: A2UIEnvelope) => {
    store.a2ui.applyEnvelope(p)
  })
  EventsOn('agui:approval-prompt', (p: ApprovalPrompt) => {
    store.showApproval(p)
  })
  EventsOn('agui:done', () => store.finishTurn())
  // ... 其余事件类型
}
```

---

## 3. 页面架构（与 Open Design 原型对应）

Open Design 已产出 4 个高保真 HTML 页面，作为 React 实现的**视觉与交互基准**。本节把原型中的关键结构映射到桌面端实现模块。

### 3.1 文件地图

| 页面 | 原型文件 | React 入口 | 说明 |
|------|----------|-----------|------|
| 导航页 | `index.html` | `App.tsx` 开发期路由 | 页面 Hub，链接到各视图 |
| 聊天工作台 | `chat-workspace.html` | `ChatView.tsx` | 核心三栏布局 + 分屏文档预览 |
| 新会话空状态 | `chat-empty.html` | `EmptyStateView.tsx` | 欢迎 + 4 个快速入口 |
| 设置 | `settings.html` | `SettingsView.tsx` | 外观 / Provider / 知识库 / 关于 |
| 知识库管理 | `knowledge.html`（规划） | `KnowledgeView.tsx` | 阶段 3 扩展 |
| 专利模板库 | `templates.html`（规划） | `TemplatesView.tsx` | 阶段 3 扩展 |

### 3.2 聊天工作台布局

原型 `chat-workspace.html` 采用**三栏 + 可选分屏文档**结构，最终实现同样分层：

```
┌──────────────────────────────────────────────────────────────────┐
│ Titlebar（红绿灯 / 标题 / 视图切换 / 侧栏&面板开关）                    │
├──────────┬───────────────────────────────┬───────────────────────┤
│ Sidebar  │        Chat Main              │   Context Panel       │
│ 项目树    │  Agent intro + 消息流          │   参考资料            │
│ 搜索/新建 │  ToolCard / ConclusionCard    │   已上传文档/法条/模板   │
│ 设置/状态 │  ChatInput（悬浮 Composer）    │                       │
├──────────┴───────────────────────────────┴───────────────────────┤
│ StatusBar（Provider / 知识库 / 版本 / 用量）                         │
└──────────────────────────────────────────────────────────────────┘
```

**关键区域与实现组件映射**：

| 原型 CSS 类 | 实现组件 | 数据来源 |
|------------|---------|---------|
| `.titlebar` | `TitleBar.tsx` | 静态 / Wails 窗口控制 |
| `.sidebar` | `Sidebar.tsx` | `ListThreads()` + 本地项目状态 |
| `.project-folder` | `ProjectTree.tsx` | `domains.ProjectRecord` / CWD 检测 |
| `.project-child.conversation` | `ThreadItem.tsx` | 会话 API |
| `.project-child.document` | `DocumentItem.tsx` | 文件索引 / fileindex.Extension |
| `.chat-messages` | `MessageList.tsx` | AGUI 事件流 |
| `.chat-message-row.agent` | `AgentMessageBubble.tsx` | `agui:message-delta` 等 |
| `.chat-message-row.user` | `UserMessageBubble.tsx` | 本地乐观更新 |
| `.tool-card` | `ToolCard.tsx` | `agui:tool-call-*` |
| `.conclusion-card` | `ConclusionCard.tsx` | Agent 结构化输出 |
| `.confidence-bar` | `ConfidenceBar.tsx` | 结论置信度 |
| `.chat-input-area` | `Composer.tsx` | 本地 state + `App.Chat()` |
| `.context-panel` | `ContextPanel.tsx` | 会话上下文 / 检索历史 |
| `.doc-viewer` | `DocumentViewer.tsx` | 文件索引；HTML 内嵌 / PDF 外部打开（Q7） |
| `.statusbar` | `StatusBar.tsx` | `Health()` |

### 3.3 项目树模型

桌面端项目树采用**可读写**的虚拟文件树，基于 CWD / 案件上下文，同时允许用户在项目树中创建/重命名文件夹（阶段 3 暂不支持删除）：

- **顶层**：当前案件/项目（`domains.ProjectRecord` 或 CWD 瞬态上下文）
- **会话节点**：`ThreadItem`，来自 `ListThreads()`
- **文档节点**：当前 CWD / 案件目录下的文件，来自 `fileindex.Extension`
- **操作入口**：新建会话、新建文件夹、重命名文件夹
- **沙箱约束**：文件操作通过 `tools/path.go` 的 `resolvePathSandboxed` 路径校验，禁止越狱到项目目录之外

**为何可读写而非只读**：专利代理师工作流中经常需要在案件目录下新建子文件夹组织文件（如按“OA 答复”“对比文件”“检索报告”分类），强制只读会破坏已有工作习惯。

### 3.4 空状态与快速入口

原型 `chat-empty.html` 提供 4 个快速入口：

| 入口 | 触发命令 | 跳转 |
|------|---------|------|
| AI 辅助撰写 | `message="帮我撰写权利要求书"` | `chat-workspace` |
| OA 答复辅助 | `message="答复审查意见"` | `chat-workspace` |
| 专利检索 | `message="检索现有技术"` | `chat-workspace` |
| 自由对话 | 空输入 | `chat-workspace` |

实现时把这些入口的提示词写入 `frontend/src/lib/quick-starts.ts`，保持文案可配置且不硬编码业务逻辑。

### 3.5 设置面板

原型 `settings.html` 分区与最终实现一致：

| 分区 | 设置项 | 持久化 |
|------|--------|--------|
| 外观 | 主题模式 / Vibrancy | `~/.mady/desktop-settings.json` |
| AI 服务 | 默认 Provider / 模型 | 复用 `pkg/agentconfig` + 桌面 settings |
| 知识库 | 状态 / 重新索引 / 自动上下文 | 复用 `pkg/agentconfig` |
| 关于 | 版本 / 许可 / 隐私 | 静态 |

---

## 4. A2UI 渲染器架构（核心资产详解）

### 4.1 渲染器分层

参考开源 A2UI 渲染器（CopilotKit、a2ui-react-runtime、a2ui-blazor）的分层经验，Mady 前端 A2UI 渲染器划分为四层：

```
desktop/frontend/src/a2ui-renderer/
├── catalog.ts          # CatalogRegistry + BasicCatalog 定义（对齐 a2ui/catalog.go）
├── store.ts            # SurfaceStore（处理 envelope、data model）
├── dynamic.ts          # Dynamic / Bind / FunctionCall 解析
├── datamodel.ts        # JSON Pointer get/set/remove（RFC 6901）
├── renderer.tsx        # 组件树 → React 元素
├── registry.tsx        # ComponentType → ReactComponent 映射
├── theme.ts            # A2UI theme properties → Tailwind class
├── validate.ts         # 开发期结构校验
├── components/         # 18 个 Basic 组件实现
└── functions/          # 15 个 A2UI 函数实现
```

### 4.2 双 SurfaceStore 对齐

Go 端 `a2ui.SurfaceStore` 已实现；前端实现**等价**的 TS 版，**字段语义必须 1:1 对齐**（避免漂移）：

```ts
// frontend/src/a2ui-renderer/store.ts
interface SurfaceState {
  id: string
  catalogId: string
  theme: Record<string, unknown>
  components: Map<string, Component>      // id → component（邻接表）
  sendDataModel: boolean
  dataModel: unknown                      // JSON Pointer 寻址根
  // rootId 是派生字段：从 components.get('root') 获取
}

class SurfaceStore {
  surfaces: Map<string, SurfaceState>
  applyEnvelope(env: Envelope): void      // createSurface/updateComponents/updateDataModel/deleteSurface
  getSurface(id: string): SurfaceState | undefined
  rootOf(surfaceId: string): Component | undefined
  clientDataModel(): ClientDataModelPayload // 聚合所有 sendDataModel=true 的 surfaces
}
```

**与 Go 端对齐要点**：

- Go 端 `Surface` 没有显式 `rootId` 字段，而是通过 `Root()` 方法从 `Components["root"]` 获取；前端保持一致，`rootId` 不作为 state 字段存储。
- Go 端 `SurfaceStore.ClientDataModel()` 返回所有 `sendDataModel=true` 的 surfaces 的聚合 payload；前端同样返回聚合对象，而不是按 surfaceId 单独返回。
- `applyEnvelope` 处理的四种 envelope kind 与 Go 端完全一致：`createSurface`、`updateComponents`、`updateDataModel`、`deleteSurface`。

### 4.3 组件渲染分派与注册表

```tsx
// frontend/src/a2ui-renderer/registry.tsx
const componentRegistry: Record<string, ComponentType<any>> = {
  Text: TextComponent,
  Button: ButtonComponent,
  Card: CardComponent,
  // ... 其余 15 个 Basic 组件
}

export function renderComponent(comp: Component, store: SurfaceStore): ReactNode {
  const Impl = componentRegistry[comp.type]
  if (!Impl) {
    console.warn(`[a2ui] unknown component type: ${comp.type}`)
    return <UnknownComponent comp={comp} />
  }
  return <Impl comp={comp} store={store} />
}

// frontend/src/a2ui-renderer/renderer.tsx
function renderTree(surfaceId: string, rootId: string, store: SurfaceStore): ReactNode {
  const srf = store.getSurface(surfaceId)
  if (!srf) return null
  const comp = srf.components.get(rootId)
  if (!comp) return null
  const childIds = resolveChildIds(comp, srf.catalogId)
  return React.cloneElement(
    renderComponent(comp, store) as ReactElement,
    {},
    childIds.map(id => renderTree(surfaceId, id, store))
  )
}
```

**自定义 catalog / 业务组件边界**：

- `a2ui-renderer/components/` 只放 A2UI BasicCatalog 的 18 个标准组件；
- 专利领域业务组件（`ConclusionCard`、`ConfidenceBar`、`ApprovalCard`、`ToolCard`）放在 `frontend/src/components/`，由 AGUI 事件直接驱动，不走 A2UI 渲染器。

### 4.4 动态值解析（memoize 优化）

A2UI `Dynamic` 的 wire 格式必须与 Go 端 `a2ui/dynamic.go` 保持一致：

| 形态 | Go 端 JSON | 前端解析 |
|------|-----------|---------|
| Literal | 直接值 | 直接使用 |
| Bind | `{"path": "/user/name"}` | `jsonPointerGet(dataModel, path)` |
| FunctionCall | `{"call": "formatDate", "args": {"key": value}}` | 查函数表执行；`args` 为命名参数对象 |

```ts
// frontend/src/a2ui-renderer/dynamic.ts
const bindCache = new WeakMap<object, Map<string, unknown>>()

// 识别顺序与 Go 端 UnmarshalJSON 完全一致：call → 仅 path → literal
function classifyDynamic(value: any): { kind: 'literal' | 'bind' | 'call'; data: any } {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    if ('call' in value) return { kind: 'call', data: value }
    if (Object.keys(value).length === 1 && 'path' in value) {
      return { kind: 'bind', data: value }
    }
  }
  return { kind: 'literal', data: value }
}

export function resolveDynamic(d: any, dataModel: unknown): unknown {
  const { kind, data } = classifyDynamic(d)
  if (kind === 'call') return callFunction(data.call, data.args, dataModel)
  if (kind === 'bind') return resolveBind(dataModel, data.path)
  return data
}

export function resolveBind(dm: unknown, pointer: string): unknown {
  if (dm === null || typeof dm !== 'object') return undefined
  let cache = bindCache.get(dm)
  if (!cache) { cache = new Map(); bindCache.set(dm, cache) }
  if (cache.has(pointer)) return cache.get(pointer)
  const v = jsonPointerGet(dm, pointer)
  cache.set(pointer, v)
  return v
}
```

**不可变更新要求**：`SurfaceStore.applyDataModel` 必须产生新的 `dataModel` 对象（使用 `immer` 或结构化 clone 局部更新），否则 `WeakMap` 缓存会失效，导致 Bind 不刷新。

---

## 5. 美学设计：Apple HIG 原生专业工具

### 5.1 设计语言定位

**关键词：原生 · 克制 · 专业 · 值得信赖**

Mady 桌面端不是"网页套壳"，而是 macOS 原生专业工具。美学上优先遵循 **Apple Human Interface Guidelines**，让用户从第一眼就感到熟悉和可信。在 HIG 框架内，注入 Mady 的品牌识别（图标中的紫/橙双色）。

| 反例（不做） | 正例（要做） |
|--------------|--------------|
| 花哨渐变、霓虹色 | 语义化中性层级 + 单一强调色 |
| 大量阴影/玻璃拟态 | Vibrancy 材质 + 明暗对比构建层次 |
| 弹跳/夸张动画 | ≤ 200ms 的缓和微交互 |
| 信息密度过高 | 留白充足，信息分组清晰 |
| 绝对化措辞（"100%"/"必定"） | 附置信度标注 |
| 橙色大面积铺陈 | 橙色仅作为品牌点缀，极低频使用 |

### 5.2 参考产品对标

| 产品 | 借鉴点 |
|------|--------|
| **Apple Mail / Notes** | 原生 sidebar + content 双栏、vibrancy、字体层级 |
| **Xcode** | 专业工具的信息密度、面板组织、状态指示 |
| **Linear** | 克制气质、命令面板（⌘K）、键盘优先 |
| **Raycast** | 卡片化信息展示、扩展生态观感 |
| **Arc Browser** | 侧栏会话组织 |
| **ChatGPT / Claude** | 流式 token 淡入动画 |

### 5.3 配色策略（Apple HIG 语义化）

完整令牌见 [02-spec.md §5.2](./02-spec.md#52-颜色令牌light--dark)。本章节说明设计意图。

#### 5.3.1 三层色彩纪律

1. **背景靠层级说话**：`systemBackground` → `secondarySystemBackground` → `tertiarySystemBackground` 自然区分内容/卡片/输入框
2. **文字靠透明度说话**：`label` → `secondaryLabel` → `tertiaryLabel` 表达主次弱
3. **强调色极其克制**：主 accent 用 `systemIndigo`（呼应图标深紫）；橙色仅用于品牌点缀和最高置信度提示

#### 5.3.2 品牌色与图标的关系

YunPat-Ai 图标由 **深紫（Indigo）** 和 **橙色** 构成：

- **Indigo 作为主 accent**：占绝对主导（90% 以上强调场景）。它既有专业可信度，又与图标视觉锚点一致。
- **橙色作为点缀**：仅用于 (a) 置信度 90%+ 的结论徽标、(b) 品牌 moments（启动/关于页）、(c) "新增" badge。绝不用作按钮主色或大面积背景。

> 这种"紫主橙辅"的比例与 Apple 自己的产品类似：Mail 用蓝色 accent，但品牌橙色只出现在通知 badge 中。

#### 5.3.3 深色模式一致性

- 所有颜色使用语义 token，自动映射到 HIG 的 light/dark 值
- 深色模式下不"反色"，而是按 Apple 规范整体压低亮度（背景近黑、文字纯白但层级通过透明度保持）
- 专利文档区（衬线文字）在深色模式下使用略暖的白色，减少刺眼感

### 5.4 排版规范

完整令牌见 [02-spec.md §5.4](./02-spec.md#54-字体排版)。

| 场景 | 字体 | 字号 | 字重 | 行高 |
|------|------|------|------|------|
| UI 主文字 | Inter / SF Pro / system-ui | 13px | 400 | 1.4 |
| UI 小文字 | Inter / SF Pro | 11px | 400 | 1.3 |
| 聊天消息正文 | Inter / SF Pro | 14px | 400 | 1.5 |
| 专利文档正文（权利要求/说明书） | Georgia / Songti SC（衬线） | 15px | 400 | 1.6 |
| 代码块 | JetBrains Mono / SF Mono | 13px | 400 | 1.5 |
| 标题 H1-H3 | Inter / SF Pro | 22/17/15px | 600 | 1.2-1.3 |

**排版原则**：
- macOS 原生应用以 13px 为基准，而非 Web 常用的 14-16px
- 专利文档使用衬线体提升正式感；UI 元素使用无衬线保证清晰度
- 行宽限制在 65-75 字符，避免长行阅读疲劳

### 5.5 材质与层次（Vibrancy）

遵循 Apple HIG 的 **Depth** 原则：

| 区域 | 处理方式 | HIG 参考 |
|------|----------|----------|
| 侧栏 | Vibrancy 材质，半透明，可看到桌面壁纸轻微透出 | `NSVisualEffectView.Material.sidebar` |
| 标题栏/工具栏 | Vibrancy 材质 | `NSVisualEffectView.Material.headerView` |
| 主内容区 | 纯色 `systemBackground`，保证阅读专注 |
| 卡片/输入框 | `secondarySystemBackground` 或 `tertiarySystemBackground` |
| 浮层面板 | Vibrancy + 投影 | `NSVisualEffectView.Material.popover` |

在 WebView 中通过 `backdrop-filter: blur(20px) saturate(180%)` 近似实现。减弱动态效果模式下降级为纯色。

### 5.6 图标系统

- **App Icon**：复用 YunPat-Ai 图标（深紫 + 橙色抽象图形），已复制到 `desktop/build/appicon.png`
- **UI 图标**：优先使用 **SF Symbols**；无法使用时用 Lucide React 回退
- **图标风格**：outline，线宽一致（1.5px 视觉），避免 filled 与 outline 混用
- **工具栏图标**：16×16pt @1x / 32×32pt @2x

### 5.7 动画规范（Motion）

| 场景 | 动画 | 时长 | 缓动 |
|------|------|------|------|
| 流式 token 淡入 | opacity 0→1 | 120ms | ease-out |
| 列表项入场 | opacity + translateY(4px→0) | 160ms | ease-out |
| 卡片展开/收起 | height + opacity | 200ms | ease-in-out |
| ToolCard 状态切换 | border/background 过渡 | 120ms | ease-out |
| 侧栏会话切换 | opacity | 100ms | ease-out |
| 模态弹窗 | scale 0.96→1 + opacity | 180ms | cubic-bezier(0.16, 1, 0.3, 1) |
| 按钮悬停 | background 过渡 | 80ms | ease-out |

**红线**：

- 单次动画 **不超过 250ms**（macOS 原生感）
- **尊重 `prefers-reduced-motion`**：减弱动态效果模式下，所有位移动画取消，仅保留透明度变化；透明度动画也减半时长
- 流式输出期间**不阻塞输入框**（用户可随时打断）
- 不使用弹跳、弹簧、弹性 overshoot（避免轻浮感）

### 5.8 布局与间距

遵循 Apple HIG 的 **8pt 网格 + 16pt 基线**：

| 场景 | 间距 |
|------|------|
| 窗口内边距 | 0（内容顶边），列表项 12-16px |
| 卡片内边距 | 16px |
| 表单字段间距 | 12px |
| 按钮内部 padding | 6px 12px（小按钮） / 8px 16px（标准按钮） |
| 侧栏宽度 | 240-280px，可拖拽调整（最小 200px） |
| 消息气泡最大宽度 | 760px，居中 |

### 5.9 控件与交互

- **按钮**：6px 圆角，主按钮用 `--mady-accent` 填充，次按钮用 `secondarySystemBackground` + 1px border
- **输入框**：6px 圆角，聚焦时 2px `--mady-accent` ring（与 macOS focus ring 一致）
- **开关/复选**：优先使用 shadcn/ui 的 Apple HIG 风格变体；若不可行，自定义为 21×12pt 胶囊开关
- **滚动条**：窄滚动条（8px），自动隐藏，hover 时显示（macOS 默认行为）
- **工具提示**：300ms 延迟出现，6px 圆角，vibrancy 材质

---

## 6. 安全考量

### 6.1 敏感路径影响分析

本次新增**不修改任何安全敏感路径**，但需记录接触点：

| 敏感路径 | 接触方式 | 影响 |
|----------|----------|------|
| `tools/path.go`（沙箱） | 间接（通过 agentcore） | 无变更 |
| `agentcore/handoff.go`（白名单） | 间接（消费事件） | 无变更；前端**必须**对 handoff 事件静默 |
| `agentcore/permission/` | 间接（审批走现有 ApprovalGate） | 无变更 |
| `guardrails/guardian/` | 间接（消费熔断事件） | 无变更 |

**新增安全考量**：

| 风险 | 缓解 |
|------|------|
| WebView 内执行 Agent 下发的 JS | **A2UI 协议本身不执行任意代码**（声明式），渲染器只渲染组件树，不 eval |
| Agent 下发恶意 `openUrl` | 渲染器拦截，仅允许 http(s) 协议；非白名单协议（file/javascript:）拒绝 |
| API Key 泄露到前端日志 | 后端日志层屏蔽；前端 console 默认 prod 模式 silent |
| WebView 调试端口暴露 | 仅 `MADY_DESKTOP_DEV=true` 时开调试端口；release 模式禁用 |
| Wails Binding 方法暴露过多能力 | 仅暴露 §2.1 列出的方法，**不暴露**文件系统/进程/exec |

### 6.2 Invisible Handoff 前端契约

`domains/unified.go` 的统一 Agent 通过 `transfer_to_patent`/`transfer_to_legal` 委派任务。前端**必须遵守**：

1. 收到 `agui:handoff-start`/`handoff-end` 事件 → **不渲染任何 UI**
2. 不向用户显示 `transfer_to_*` 工具调用卡片
3. 工具调用卡片列表过滤掉 type 以 `transfer_to_` 开头的项

> 这是安全契约，违反会导致用户看到内部交接细节，破坏统一 Agent 体验。

### 6.3 措辞规范

前端所有面向用户文案（错误提示/toast/空状态/按钮）遵循 `docs/tone-style-guide.md`：

- 不使用绝对化表述（"绝对/一定/百分百" → "通常/大概率"）
- 结论性表述附带置信度
- 拒绝类文案提供替代帮助
- 不在 UI 文案中提及"中观/佛教"出处

---

## 7. 性能考量

| 关注点 | 策略 |
|--------|------|
| 流式渲染卡顿 | token 批量化（每 16ms 一帧合并）；React 18 `useTransition` 降级长列表；`MessageBubble` 使用 `React.memo` 避免历史消息重渲染 |
| 长对话布局抖动 | 给 message bubble 加 `content-visibility: auto` 和 `contain: layout paint`，减少 off-screen 消息的布局计算 |
| A2UI 大组件树重渲染 | `React.memo` + `useMemo`；SurfaceStore 不可变更新触发精确 re-render；组件按 `id` 稳定 key |
| WebView 启动白屏 | embed 一个最小 HTML 骨架（loading spinner）先渲染，JS 加载后 hydrate |
| 内存（长会话） | 虚拟列表（react-window）渲染历史消息；超过阈值触发上下文压缩提示 |
| macOS 能效 | 窗口失焦时降低动画帧率；空闲时停 SSE 心跳 |

---

## 8. 跨平台预留（阶段 4）

阶段 1-3 仅 macOS，但代码结构预留：

```go
// desktop/main.go（条件编译预留）
//go:build darwin
// +build darwin

package main
// macOS 特定初始化（TitleBar、Dock menu）

// desktop/main_windows.go（阶段 4 填充）
//go:build windows
// +build windows

// desktop/main_linux.go（阶段 4 填充）
//go:build linux
// +build linux
```

Makefile 阶段 4 目标：

```makefile
desktop-dmg:      # macOS（阶段 3）
desktop-msi:      # Windows（阶段 4，需 Windows 构建机/CI runner）
desktop-appimage: # Linux（阶段 4）
```

---

## 9. 备选与降级

| 场景 | 主方案 | 降级 |
|------|--------|------|
| Wails v2 与 Go 1.26 兼容性问题 | Wails v2 | 回退 Go 1.24（如 Wails 尚未声明 1.26 兼容） |
| shadcn/ui 组件无法满足 A2UI 语义 | shadcn/ui | 自建对应组件（仍走 Tailwind） |
| Motion 与 React 18 StrictMode 冲突 | Motion | 降级到 CSS transition |
| WKWebView 渲染差异 | 系统 WebView | 阶段 5 评估强制 Chromium（代价：体积+80MB） |
| A2UI 渲染器工作量超预期 | 18 组件全做 | 阶段 2 只做 P0（9 个），P1/P2 后置 |

---

## 10. 下一步

人工审阅本设计后，进入 [04-tasks.md](./04-tasks.md)（任务拆解）。
