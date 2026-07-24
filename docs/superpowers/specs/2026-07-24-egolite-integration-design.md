# EgoLite Browser 集成设计

> 状态：待审批 | 日期：2026-07-24

## 目标

将 ego-browser（Ego Lite 内置的 AI Agent 浏览器自动化工具）作为一等公民集成到 Mady 的浏览器工具体系中，
涵盖 **Runtime 层**（第 7 个浏览器后端）和 **Workflow 层**（任务空间隔离 + 控制权交接）。

## 动机

ego-browser 提供三个 Mady 现有浏览器工具不具备的差异化能力：

1. **任务空间隔离** — 每个任务获得独立的浏览上下文，共享 Ego Lite 登录态但互不干扰
2. **控制权交接（Handoff）** — Agent 可将浏览器交给用户（验证码/登录），完成后取回继续
3. **富交互能力** — 坐标点击、拖拽、智能滚动加载（`scrollToBottomUntil`）

## 架构

```
┌──────────────────────────────────────────────────────────────┐
│                     Mady Agent                               │
│                                                              │
│  ┌──────────────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │ browser      │  │ ego_lite_      │  │ ego_lite_       │  │
│  │ (新增        │  │ handoff        │  │ task_spaces     │  │
│  │  BackendEgo  │  │ (新工具)        │  │ (新工具)         │  │
│  │  Lite)       │  │                │  │                 │  │
│  └──────┬───────┘  └───────┬────────┘  └────────┬────────┘  │
│         │                  │                    │            │
│  ┌──────┴──────────────────┴────────────────────┴──────────┐ │
│  │              EgoLiteManager (持久子进程)                  │ │
│  │   · stdin/stdout 行分隔 JSON 协议                         │ │
│  │   · 自动重连、命令队列（互斥锁串行化）                      │ │
│  └──────────────────────┬───────────────────────────────────┘ │
└─────────────────────────┼────────────────────────────────────┘
                          │ stdin → JSON command
                          │ stdout → JSON result
                   ┌──────┴──────────┐
                   │ ego_lite_       │   go:embed
                   │ bridge.js       │   嵌入
                   │                 │
                   │ 长生命周期       │
                   │ Node.js 进程     │
                   └──────┬──────────┘
                          │ ego-browser helpers
                   ┌──────┴──────────┐
                   │   Ego Lite      │
                   │   Browser App   │
                   │   (daemon)      │
                   └─────────────────┘
```

**三个核心组件：**

| 组件 | 文件 | 职责 |
|------|------|------|
| Bridge 脚本 | `tools/ego_lite_bridge.js` | stdin 读 JSON 命令，执行 ego-browser helper，stdout 写 JSON 结果 |
| EgoLiteManager | `tools/ego_lite_manager.go` | Go 侧子进程管理、健康监控、自动重连、命令队列 |
| EgoLiteExtension | `tools/ego_lite.go` | 实现 agentcore.Extension，注册 browser backend + handoff/task_spaces 工具 |

## Bridge 协议

Go 侧和 Node.js 桥进程之间通过 stdin/stdout 走**行分隔 JSON** 协议（每行一个完整 JSON）。

```jsonc
// Request (Go → Node.js)
{"id": "1", "method": "snapshotText", "params": {"scope": "full_page"}}

// Success Response (Node.js → Go)
{"id": "1", "ok": true, "result": "root\n  heading \"...\""}

// Error Response (Node.js → Go)
{"id": "1", "ok": false, "error": "task space not found"}
```

**支持的方法（16 个）：**

| method | params | 返回值 | 用途 |
|--------|--------|--------|------|
| `initTaskSpace` | `{name}` | `{taskId, id}` | 创建/恢复任务空间 |
| `navigate` | `{url, timeout?}` | snapshotText 字符串 | 打开 URL + 等待加载 |
| `snapshotText` | `{scope?}` | 语义树字符串 | 获取页面可访问性树 |
| `captureScreenshot` | `{}` | base64 PNG | 截图 |
| `click` | `{ref, label?}` | snapshotText | 点击元素 |
| `typeText` | `{ref, text}` | 文本 | 输入文字 |
| `scroll` | `{dy?}` | snapshotText | 滚动 |
| `pressKey` | `{key}` | 文本 | 按键 |
| `evaluateJS` | `{expression}` | JSON 值 | 执行 JS |
| `pageInfo` | `{}` | `{url, title, w, h}` | 页面元信息 |
| `handoffTaskSpace` | `{}` | `{done, skipped?}` | 交出控制权 |
| `takeOverTaskSpace` | `{}` | `{}` | 取回控制权 |
| `listTaskSpaces` | `{}` | `[{id, name, ownership, tabCount}]` | 列出所有任务空间 |
| `listTabs` | `{}` | `[{id, url, title}]` | 列出当前空间标签页 |
| `closeTab` | `{targetId?}` | `{}` | 关闭标签页 |
| `completeTaskSpace` | `{keep}` | `{done, skipped?}` | 关闭当前任务空间 |
| `ping` | `{}` | `"pong"` | 健康检查 |

## Runtime 层 — BackendEgoLite

Mady 现有 browser 工具通过 `browserActionHandlers` map 分发 14 种 action。EgoLite 作为第 7 个后端插入现有分支。

### 设计原则

- 不破坏现有结构，仅在 handler 的 `switch session.backendType` 中增加 `case BackendEgoLite`
- 适配 7 个需要实现的 action，其余 action 返回 "not supported"
- Ref 映射沿用 ego-browser 原生 `@N` ref（与 Camofox 模式一致）

### 需要适配的 handler

| action | bridge method | 说明 |
|--------|--------------|------|
| navigate | `navigate` | 涵盖 snapshot + pageInfo |
| snapshot | `snapshotText` | ego-browser 自带 ref 映射 |
| click | `click` | bridge 回发 snapshot |
| type | `typeText` | 返回确认文本 |
| scroll | `scroll` | 默认 dy=500 |
| press | `pressKey` | 按键名直传 |
| screenshot | `captureScreenshot` | base64 解码后保存 |
| evaluate | `evaluateJS` | 返回 JSON 值 |

其余 6 个 action（back/dialog/vision/console/cdp/get_images）EgoLite 后端返回 "not supported"。

### BrowserSession 改动

```go
// browser_session.go — 新增
const BackendEgoLite BrowserBackendType = "egolite"

type BrowserSession struct {
    // ... 现有字段 ...
    egoLiteManager *EgoLiteManager  // egolite 后端专用
}
```

### BrowserToolConfig 改动

```go
// browser_tool.go — 新增字段
type BrowserToolConfig struct {
    // ... 现有字段 ...
    EgoLiteEnabled  bool   // 启用 EgoLite 后端
    EgoLiteTaskName string // 任务空间名称，默认 "mady-agent"
}
```

## Workflow 层 — Handoff + Task Spaces

### ego_lite_handoff 工具

**设计动机：** Agent 在遇到登录、验证码、双因素认证时，将浏览器交给用户，完成后取回继续。

**操作：**

| action | 说明 |
|--------|------|
| `handoff` | 交出控制权给用户，附 message 说明 |
| `takeover` | 取回控制权，自动获取 snapshot 了解当前状态 |
| `status` | 仅查询控制权状态，不动控制权 |

**参数：**

```json
{
  "action": "handoff",       // handoff | takeover | status
  "message": "请登录后点击继续"  // handoff 时的用户说明
}
```

**流程：**

```
handoff → Bridge handoffTaskSpace() → 返回确认 → Agent 告知用户等待
        → 用户在 Ego Lite GUI 中操作
        → 用户说"继续"
takeover → Bridge takeOverTaskSpace() → Bridge snapshotText() → 返回当前页面状态
```

**特殊处理：**
- `takeover` 后自动执行 snapshot，让 Agent 立即知道用户在浏览器中做了什么
- `handoff` message 展示给用户（通过 Agent 回复）
- 如果 handoff 时任务空间是 user-owned 的，Bridge 返回 `{done: false, skipped: "user-owned"}`，工具返回友好提示

**ReadOnly:** false（handoff/takeover 改变浏览器控制权）

### ego_lite_task_spaces 工具

**设计动机：** 并行处理多个独立浏览任务时，每个任务有隔离的浏览器上下文。

**操作：**

| action | 说明 |
|--------|------|
| `list` | 列出所有任务空间及其标签页 |
| `create` | 创建新的任务空间 |
| `switch` | 切换到指定空间（后续 browser 操作在空间中执行） |
| `close` | 关闭任务空间 |

**参数：**

```json
{
  "action": "switch",   // list | create | switch | close
  "name": "patent-cn",  // 任务空间名称
  "keep": false         // 关闭时保留标签页
}
```

**多任务空间实现：** 单进程多任务空间方案。在一个 ego-browser 子进程中通过 `useOrCreateTaskSpace(name)` 切换当前任务空间，Manager 维护 `currentTask` 状态。无需为每个空间启动新进程。

**ReadOnly:** false（创建/关闭改变浏览器状态）

### EgoLiteExtension 组装

```go
// tools/ego_lite.go — 新文件

type EgoLiteConfig struct {
    Enabled   bool
    TaskName  string
    Headless  bool
}

type EgoLiteExtension struct {
    mgr *EgoLiteManager
    cfg EgoLiteConfig
}

// 实现 agentcore.Extension + BrowserBackendProvider
func (e *EgoLiteExtension) Name() string           { return "ego-lite" }
func (e *EgoLiteExtension) Tools() []*agentcore.Tool { ... }
func (e *EgoLiteExtension) BrowserBackend() BrowserBackendType { return BackendEgoLite }
func (e *EgoLiteExtension) Dispose() error          { return e.mgr.Close() }
```

## EgoLiteManager 设计

### 核心职责

- 管理 `ego-browser nodejs` 子进程生命周期
- 通过 stdin/stdout JSON 协议与 bridge 通信
- 命令串行化（单进程不能并发操作同一标签页）
- 自动重连（进程崩溃后重启并恢复任务空间）

### 关键设计

```go
type EgoLiteManager struct {
    mu          sync.Mutex       // 保护 Send() 串行访问
    cmd         *exec.Cmd        // ego-browser 子进程
    stdin       io.WriteCloser   // 写入 JSON 命令
    stdout      *bufio.Scanner   // 读取 JSON 响应
    pending     map[string]chan jsonResult
    counter     atomic.Int64
    restartMu   sync.Mutex       // 防止并发重启
    currentTask string           // 当前选中的任务空间
    taskSpaces  map[string]TaskSpaceInfo
    ctx         context.Context
    cancel      context.CancelFunc
}

func (m *EgoLiteManager) Send(ctx context.Context, method string, params map[string]any) (any, error)
func (m *EgoLiteManager) Close() error
```

**延迟启动：** 首次 Send() 调用时才启动子进程。

**重启流程：**
1. readLoop goroutine 检测 EOF → 触发重启锁
2. Send() 等待重启完成后重试命令
3. 重启后重新执行 `initTaskSpace` 恢复 currentTask 状态

**生命周期：** 创建于 BrowserManager 初始化时，一个 Mady 会话一个 Manager 实例，Close 时发送 `completeTaskSpace` 并 kill 子进程。

## 文件清单

| 文件 | 类型 | 职责 |
|------|------|------|
| `tools/ego_lite_bridge.js` | 新建 | Go embed 的 bridge 脚本 |
| `tools/ego_lite_manager.go` | 新建 | 子进程管理 + JSON 通信 |
| `tools/ego_lite_manager_test.go` | 新建 | Manager 单元测试 |
| `tools/ego_lite.go` | 新建 | Extension 组装 + 工具定义 |
| `tools/ego_lite_test.go` | 新建 | 工具层单元测试 |
| `tools/browser_tool.go` | 修改 | 新增 EgoLiteEnabled/TaskName 配置 |
| `tools/browser_tool_navigate.go` | 修改 | 新增 BackendEgoLite 分支 |
| `tools/browser_tool_interact.go` | 修改 | 新增 BackendEgoLite 分支 |
| `tools/browser_session.go` | 修改 | 新增 BackendEgoLite 常量 + Session 字段 |

## 错误处理策略

| 场景 | 处理方式 |
|------|----------|
| `ego-browser` 命令未安装 | `NewEgoLiteManager()` 返回错误，提示安装 ego-lite |
| 子进程启动失败 | 重试 3 次（间隔 500ms），失败返回错误 |
| 子进程运行时崩溃 | 自动重启 + 恢复 currentTask，当前命令返回重连错误 |
| Bridge 返回 `ok: false` | 将 error 字符串包装为 Go error 返回 |
| Send() 超时 | ctx 超时 → 返回 context.DeadlineExceeded |
| 重复 Close() | 幂等，second call 无操作 |
| Handoff 到 user-owned 空间 | 返回 `{done: false, skipped: "user-owned"}` → 工具提示用户 |

## 测试策略

### 单元测试（不需真实 Ego Lite）

- `ego_lite_manager_test.go`：Mock bridge 脚本，测试命令序列化/响应匹配/重启逻辑
- `ego_lite_test.go`：测试工具参数校验、错误处理分支

### 集成测试（需要 ego-browser 环境）

- `tools/ego_lite_integration_test.go`：真实连接 ego-browser，覆盖 navigate/snapshot/click/handoff 核心流程
- 集成测试用 `testing.Short()` 跳过模式，CI 中作为可选步骤

## 未来扩展

- 智能滚动 `scrollToBottomUntil` 作为 browser action 参数（而非独立工具）
- 坐标点击和拖拽作为 browser action 参数
- 支持 `--ego-server-name` flag 连接到特定的 Ego Lite 实例
