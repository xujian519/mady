# 桌面端下一阶段开发计划

> **制定日期**：2026-07-29
> **依据**：缺口分析（G1-G7）、[04-tasks.md](../specs/desktop/04-tasks.md)、[desktop-design-development-basis.md](../specs/desktop/desktop-design-development-basis.md)
> **三波推进**：第一波「闭环真实化」→ 第二波「领域差异化」→ 第三波「视觉收尾与发布」

---

## 总体任务清单

| 波次 | 任务 ID | 缺口 | 描述 | 涉及文件 | 风险 | 审查 |
|------|---------|------|------|---------|------|------|
| 第一波 | W1-T1 | G1 | A2UIEvent 入站处理器（A2UIPromise 模式） | `agentcore/agent.go`, `agentcore/agent_run.go`, `server/desktop.go` | **高** | L3 |
| 第一波 | W1-T2 | G1 | SendAction 前端→后端全链路验证 | `desktop/app_test.go`, `e2e/a2ui.spec.ts` | 中 | L2 |
| 第一波 | W1-T3 | G2 | Server.ListModels 与 mock 替换 | `server/desktop.go`, `ModelSettings.tsx`, `backend.ts` | 低 | L1 |
| 第一波 | W1-T4 | G6 | Playwright E2E 纳入 `make desktop-test` | `Makefile`, `playwright.config.ts` | 低 | L1 |
| 第一波 | W1-T5 | G6 | Vite manualChunks 拆包 | `vite.config.ts` | 低 | L1 |
| 第二波 | W2-T1 | G3(P02) | TodoDock — 底部待办坞 | 新增 `TodoDock.tsx` + reducer/stores + `app.go` | 中 | L2 |
| 第二波 | W2-T2 | G4 | CommandPalette — ⌘K 命令面板 | 新增 `CommandPalette.tsx` + `commands.ts` store | 中 | L2 |
| 第三波 | W3-T1 | G5 | P2 Token 全量补齐（含 C06 MCP 四状态色） | `globals.css` | 低 | L1 |
| 第三波 | W3-T2 | G5 | 视觉走查 — 按走查表逐项 ⚠️→✅ | 各组件 CSS | 低 | L2 |
| 第三波 | W3-T3 | G7 | macOS 公证评估与配置 | `Info.plist`, `Makefile` | 中 | L2 |
| 第三波 | W3-T4 | G7 | Windows 适配（标题栏/字体/滚动条/布局） | `main_windows.go`, `globals.css` | 中 | L2 |

---

## 第一波：闭环真实化（功能正确性，最高优先）

### W1-T1 — A2UIEvent 入站处理器（A2UIPromise 模式）

**缺口** G1：`server/desktop.go:SendAction()` 已将 ClientAction 投递到 agent 事件总线，但 agent 运行循环 **从未处理 EventA2UI**。这是"按钮点了没反应"的根本原因。

**当前状态追踪**：

- `server/desktop.go:SendAction()` → `entry.agent.Emit(NewA2UIEvent(payload))` ✅ 投递端已完成
- `agentcore/event.go:EventBus.dispatch()` 按 EventKind 分发给 handlers ✅ 事件总线可分发
- `agentcore/agent_run.go:runInnerLoop()` **未检查 A2UI 入站事件** ❌
- `domains/approval.go:AfterModelCall()` 通过 `agent.Interrupt()` 暂停，TUI 用 `agent.Resume()`/`agent.FollowUp()` 继续 ✅ TUI 已有 HITL 模式

**设计决策**：A2UI action 入站不能通过 LifecycleHook，因为 LifecycleHook 在 ModelCall/ToolExecution 等**触发点**执行，不是**事件轮询**。正确方案是引入 **A2UIPromise** 模式：

```
SendAction → agent.SetA2UIAction(action)  // 写入 Promise
                ↓
runPreTurn → consumePendingA2UIActions()  // 从 Promise 读取
                ↓
          action.name == "approve" → persist 审批消息 as follow-up
          action.name == "reject"  → persist 拒绝消息 as follow-up
          default                 → persist 通用 action 消息
```

#### 修改清单

**1. `agentcore/agent.go` — 新增 A2UIPromise 类型**

```go
// A2UIPromise 表示一个待处理的 A2UI action，由 SendAction 写入、
// consumePendingA2UIActions 消费。使用 sync.Mutex 保证 SendAction
// 和 runLoop 在不同 goroutine 间安全访问。
type A2UIPromise struct {
    mu       sync.Mutex
    action   *a2ui.ClientAction
    consumed bool
    done     chan struct{}
}

func NewA2UIPromise() *A2UIPromise {
    return &A2UIPromise{done: make(chan struct{})}
}

func (p *A2UIPromise) Set(action *a2ui.ClientAction) {
    p.mu.Lock()
    p.action = action
    p.mu.Unlock()
    select {
    case <-p.done:
    default:
        close(p.done)
    }
}

func (p *A2UIPromise) TryGet() *a2ui.ClientAction {
    p.mu.Lock()
    defer p.mu.Unlock()
    if p.consumed || p.action == nil {
        return nil
    }
    p.consumed = true
    return p.action
}
```

在 `Agent` struct 中新增 `a2uiPromise *A2UIPromise` 字段，`New()` 中不初始化（默认 nil，TUI 路径、测试路径不受影响）。新增导出方法：

```go
func (a *Agent) SetA2UIPromise(p *A2UIPromise) { a.a2uiPromise = p }
func (a *Agent) SetA2UIAction(action *a2ui.ClientAction) {
    if a.a2uiPromise != nil {
        a.a2uiPromise.Set(action)
    }
}
```

**2. `agentcore/agent_run.go` — 在 runPreTurn 中消费 A2UI action**

在 `runPreTurn` 函数末尾（`a.emit(&TurnStartEvent{...})` 之前）增加：

```go
// 消费待处理的 A2UI 入站事件（Promise 模式）
if err := a.consumePendingA2UIActions(ctx, turn); err != nil {
    return a.failLoop(ctx, fmt.Sprintf("turn:%d|a2ui", turn), "a2ui action consumption failed", err)
}
```

新增 `consumePendingA2UIActions` 方法，根据 action name 分发：

```go
func (a *Agent) consumePendingA2UIActions(ctx context.Context, turn int64) error {
    if a.a2uiPromise == nil {
        return nil
    }
    action := a.a2uiPromise.TryGet()
    if action == nil {
        return nil
    }

    var msg Message
    switch action.Name {
    case "approve":
        msg = Message{
            Role:    RoleUser,
            Content: fmt.Sprintf("审批已通过。额外信息: %s", extractContextStr(action.Context)),
        }
    case "reject":
        msg = Message{
            Role:    RoleUser,
            Content: fmt.Sprintf("审批已被拒绝。理由: %s", extractContextStr(action.Context)),
        }
    default:
        msg = Message{
            Role:    RoleUser,
            Content: fmt.Sprintf("用户操作: %s", action.Name),
        }
    }
    return a.persistMessage(ctx, msg)
}
```

**3. `server/desktop.go:SendAction` — 从 Emit 改为 SetA2UIAction**

```go
// 将 ClientAction 通过 A2UIPromise 投递给 agent。
// agent 在下一轮 runPreTurn 中消费此 action。
entry.agent.SetA2UIAction(action)
```

#### 安全考量

- `consumePendingA2UIActions` 只在 `runPreTurn` 中被调用，即 agent **正在运行**的轮次中
- 如果 agent 已完成（StatusFinished），`runPreTurn` 不会被调用，action 不会被消费——这是正确的行为
- 只消费一次（`consumed` 标志），防止同一 action 在多个轮次中被重复注入
- A2UIPromise 是 opt-in：仅桌面端调用 `SetA2UIPromise`，TUI 路径不受影响

#### 验收标准

1. 前端 ApprovalCard 点击"批准" → agent 在下一轮收到 follow-up 消息
2. 无待处理 action 时 `runInnerLoop` 行为不变（零开销分支）
3. `go test -race ./agentcore/...` 通过
4. `go test -race ./server/...` 通过

---

### W1-T2 — SendAction 前端→后端全链路验证

**依赖**：W1-T1

在 `desktop/app_test.go` 中新增集成测试：

1. 创建 `Server` + `Agent`（含 A2UIPromise）
2. 启动 `Chat`（发一条短消息后结束，不触发 ApprovalGate）
3. 调用 `server.SendAction("surface_test-1", approve action)`
4. 验证 agent 的 `a2uiPromise.TryGet()` 返回非 nil，`consumed` 为 false
5. 调用 `agent.Run()` 下一轮 → 验证 action 被消费（`consumed` == true）
6. 验证 agent 状态中新增了用户消息（审批结果）

在 `desktop/frontend/e2e/a2ui.spec.ts` 中新增 E2E 测试（可选，需 Wails 运行环境）：

1. 启动应用 → 等待 A2UI surface 渲染（含按钮组件）
2. 点击批准按钮 → 验证 `backend.sendAction` 被调用
3. 验证 agent 恢复执行

---

### W1-T3 — Server.ListModels 与 mock 替换

**缺口** G2：`ModelSettings.tsx` 第 11、34 行硬编码了 5 个 mock 模型（GPT-5.2/5.1、DeepSeek V4、Kimi K3、Claude 5）。

**当前状态**：

- `server/desktop.go:SwitchModel(provider, model, contextWindow)` ✅ 切换方法已存在
- `server/desktop.go:ListModels()` ❌ 无读取方法
- `pkg/agentconfig` ✅ 有配置聚合能力

#### 设计方案

**后端**：在 `server/desktop.go` 中新增：

```go
// ModelInfo 描述一个可用模型。
type ModelInfo struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    Provider      string `json:"provider"`
    ContextWindow int64  `json:"contextWindow"`
}

// ListModels 返回当前可用的模型列表。
// 从 agentconfig 静态配置聚合，若 provider 实现了 ModelLister 接口则动态获取。
func (s *Server) ListModels(ctx context.Context) ([]ModelInfo, error) {
    cfg := s.config.Get()
    // 从 agentconfig 的 Model/Provider 读取可用模型
    // 当前实现返回当前配置 + 一个预设列表
    // TODO: 待 agentconfig 完成 Provider 模型列表接口后动态化
    models := []ModelInfo{
        {ID: cfg.Model, Name: cfg.Model, Provider: fmt.Sprintf("%T", cfg.Provider), ContextWindow: cfg.ContextWindow},
    }
    return models, nil
}
```

**前端**：修改 `ModelSettings.tsx`：

1. 删除 `mockModels` 常量
2. 新增 `useEffect` 调用 `backend.listModels()`
3. 加载中显示加载骨架
4. 加载失败显示错误提示 + 回退到缓存数据（localStorage）

**Wails Binding**：在 `desktop/app.go` 中暴露 `ListModels` 方法，前端 `backend.ts` 新增 `listModels()`。

#### 验收标准

1. ModelSettings 面板的模型选择器不再使用硬编码数据
2. 启动后自动从后端加载模型列表
3. 加载过程显示 loading skeleton

---

### W1-T4 — Playwright E2E 纳入 make desktop-test

**缺口** G6：`desktop/frontend/e2e/a2ui.spec.ts`（4 个测试）和 `agui-events.spec.ts`（5 个测试）未被 Makefile 引用。

**设计方案**：

在 `Makefile` 中新增目标：

```makefile
# 桌面端 E2E 测试（需 Wails 应用在后台运行）
.PHONY: desktop-test-e2e
desktop-test-e2e:
	cd desktop/frontend && npx playwright test --config=playwright.config.ts

# 桌面端全部测试
.PHONY: desktop-test
desktop-test: desktop-test-unit desktop-test-e2e
```

检查 `playwright.config.ts` 的 `webServer` 配置——如果缺少 Wails dev server 的启动配置，则输出提示而不是自动启动（E2E 需要用户手动启动窗口后运行）。

---

### W1-T5 — Vite manualChunks 拆包

**缺口** G6：`pdfjs-dist`（~3MB）和 `codemirror`（~500KB）被打入主 chunk，首页加载 >500KB。

在 `vite.config.ts` 中配置：

```typescript
build: {
  rollupOptions: {
    output: {
      manualChunks: {
        pdfjs: ['pdfjs-dist'],
        codemirror: [
          '@codemirror/view', '@codemirror/state',
          '@codemirror/lang-json', '@codemirror/lang-markdown',
          '@codemirror/language', '@codemirror/commands',
        ],
      },
    },
  },
}
```

验收：主 entry chunk 减小 ≥300KB；pdfjs 和 codemirror 各自独立 chunk。

---

## 第二波：领域差异化（Mady 之所以是 Mady）

### ~~StageIndicator 专利工作流四阶段指示器~~（已移除）

> **移除理由**：实际专利工作流是非线性的（检索↔撰写↔答复经常交替），四个固定阶段无法反映真实复杂度，推行简化模型反而有误导性。任务进度通过 W2-T1 TodoDock 的任务列表自然呈现，无需额外抽象。

---

### W2-T1 — TodoDock 底部待办坞

**缺口** G3(P02)：`agentcore/tasklist/` 已有完整的任务管理系统（4 个工具 + MemoryStore/FileStore），但桌面端缺乏可视化展示。TUI 已有 `TodoPanel` 实现。

#### 当前资产

- `agentcore/tasklist/store.go` — Store 接口 + MemoryStore ✅
- `agentcore/tasklist/extension.go` — Extension 注入 4 个工具 ✅
- `agentcore/task_types.go` — Task 模型（Status/Priority/ActiveForm） ✅
- `agentcore/tasklist/filestore.go` — 文件系统持久化 ✅
- TUI `tui/chat/chat_app_todo.go` — TodoPanel 实现（可参考交互模式） ✅
- 前端桌面端 — 无对应组件 ❌

#### 数据流

1. Agent 调用 `task_create` / `task_update` / `task_list` 工具 → 写入 tasklist store
2. tasklist 的 `Extension` 作为 `EventSnapshotProvider` → 每次 turn 结束后发射快照
3. 快照通过 `agentcore.EventSnapshotEvent` → `agui-bridge/reducer` 接收
4. 或者：Agent 通过 A2UI `createSurface` 直接推送任务列表到前端

#### 设计方案

**位置**：ChatView 消息列表下方、StatusBar 上方，作为可折叠底部面板。

```tsx
// TodoDock.tsx
interface TodoDockProps {
  tasks: TaskItem[]
  expanded: boolean
  onToggle: () => void
}

interface TaskItem {
  id: string
  subject: string
  status: 'pending' | 'in_progress' | 'completed'
  priority: 'low' | 'normal' | 'high' | 'urgent'
  activeForm?: string
}
```

**交互**：

- 默认收起：底部单行摘要「3 个任务 · 2 进行中」
- 点击展开：列表视图，显示状态徽章 + 标题 + 进度文案
- 实时更新：任务状态变化时自动刷新（通过 AGUI 事件驱动）
- 空状态：无任务时隐藏 TodoDock

**后端支撑**：`desktop/app.go` 新增 `ListTasks(threadID)` 方法：

```go
func (a *App) ListTasks(threadID string) ([]*agentcore.Task, error) {
    // 通过 agent 的 tasklist store 读取当前会话的任务列表
    entry := a.getAgentEntry(threadID)
    if entry == nil {
        return nil, fmt.Errorf("no active session")
    }
    // 从 agent 的扩展中获取 tasklist store
    // 具体实现取决于 tasklist store 的访问方式
}
```

#### 文件清单

| 操作 | 文件 | 规模 |
|------|------|------|
| 新增 | `desktop/frontend/src/components/TodoDock.tsx` | ~150 行 |
| 新增 | `desktop/frontend/src/components/__tests__/TodoDock.test.tsx` | ~70 行 |
| 修改 | `desktop/frontend/src/stores/chat.ts` | +20 行（新增 taskList 状态） |
| 修改 | `desktop/frontend/src/agui-bridge/reducer.ts` | +25 行（监听 EventSnapshot 中的 task 数据） |
| 修改 | `desktop/app.go` | +30 行（新增 ListTasks 方法） |
| 修改 | `desktop/frontend/src/lib/backend.ts` | +5 行（新增 listTasks binding） |
| 修改 | `desktop/frontend/src/components/ChatView.tsx` | ~10 行（集成 TodoDock） |
| 修改 | `desktop/frontend/src/components/AgentFooter.tsx` | ~5 行（显示任务摘要计数） |

#### 视觉规范

- 折叠态高度：28px（一行摘要文字）
- 展开态最大高度：200px（超出滚动）
- 每任务行高：32px
- 优先级视觉：urgent=红圆点, high=橙圆点, normal=灰圆点, low=无
- 状态标记：pending=灰色边框, in_progress=品牌紫填充, completed=绿色+删除线文字

---

### W2-T2 — CommandPalette ⌘K 命令面板

**缺口** G4：规范 §12.4 定义命令面板未实现。已有 `SlashCommandMenu`（13 个 / 命令），但缺少全局 ⌘K 面板。

**状态对比**：

| 功能 | SlashCommandMenu | CommandPalette（新增） |
|------|-----------------|----------------------|
| 触发方式 | 输入 `/` | ⌘K 全局快捷键 |
| 范围 | 仅聊天输入框内命令 | 全应用命令、导航、模板、技能 |
| 搜索 | 无搜索 | 模糊搜索全部命令 |
| 快捷键提示 | 无 | 右侧显示快捷键 |

#### 命令注册表

在 `stores/commands.ts` 中定义集中注册表：

```typescript
type CommandCategory = 'navigation' | 'template' | 'skill' | 'command' | 'action'

interface Command {
  id: string
  title: string
  category: CommandCategory
  shortcut?: string       // 如 "Cmd+N"
  keywords: string[]      // 搜索关键词
  icon?: string           // lucide-react 图标名
  execute: () => void
}

const defaultCommands: Command[] = [
  // 导航
  { id: 'new-session', title: '新建会话', category: 'navigation', shortcut: 'Cmd+N', ... },
  { id: 'switch-sidebar', title: '切换侧栏', category: 'navigation', shortcut: 'Cmd+B', ... },
  { id: 'toggle-files', title: '文件面板', category: 'navigation', ... },
  // 模板
  { id: 'template-claims', title: '使用权利要求书模板', category: 'template', ... },
  { id: 'template-spec', title: '使用说明书模板', category: 'template', ... },
  // 技能
  { id: 'toggle-skill', title: '切换技能', category: 'skill', ... },
  // 斜杠命令（从 SlashCommandMenu 继承）
  { id: 'cmd-search', title: '搜索知识库', category: 'command', ... },
  { id: 'cmd-clear', title: '清除上下文', category: 'action', ... },
  // 操作
  { id: 'export-session', title: '导出会话', category: 'action', ... },
  { id: 'switch-model', title: '切换模型', category: 'action', ... },
  { id: 'toggle-theme', title: '切换深浅色', category: 'action', ... },
]
```

#### 组件设计

```tsx
// CommandPalette.tsx
// 全局快捷键: Cmd+K (macOS) / Ctrl+K (Win/Linux)
// 行为: 打开半透明覆盖层，聚焦搜索框
// 搜索: fuzzy match 命令的 title + keywords
// 选择: Enter 执行，Esc 关闭，↑↓ 导航

interface CommandPaletteProps {
  open: boolean
  onClose: () => void
}
```

**视觉规范**（参考 `desktop-design-development-basis.md` §3.4）：

- 宽度：560px（规范 §3.4 CommandPalette）
- 搜索框高度：52px
- 面板最大高度：400px（超出滚动）
- 背景：毛玻璃 `backdrop-filter: blur(20px)`，overlay 背景 `--bg-overlay`
- 命令项高度：36px
- 布局：图标 + 命令名 | 快捷键（右侧右对齐）
- 分类标题：11px caption，`--color-mady-text-tertiary`

#### 文件清单

| 操作 | 文件 | 规模 |
|------|------|------|
| 新增 | `desktop/frontend/src/components/CommandPalette.tsx` | ~200 行 |
| 新增 | `desktop/frontend/src/stores/commands.ts` | ~80 行 |
| 新增 | `desktop/frontend/src/components/__tests__/CommandPalette.test.tsx` | ~60 行 |
| 修改 | `desktop/frontend/src/app/App.tsx` | ~15 行（注册全局 ⌘K 快捷键） |
| 修改 | `desktop/frontend/src/components/ChatView.tsx` | ~5 行（渲染 CommandPalette，控制 open 状态） |

---

## 第三波：视觉收尾与发布

### W3-T1 — P2 Token 全量补齐（含 C06 MCP 四状态色）

**缺口** G5：`desktop-design-development-basis.md` 的 P2 待新增清单（8 个 Token）未实现。

在 `desktop/frontend/src/styles/globals.css` 的 `@theme` 块中新增：

```css
/* ═══════════════════════════════════════
   P2 Token — MCP 四状态色（C06 依赖）
   ═══════════════════════════════════════ */
--mcp-starting: #ff9500;
--mcp-ready: #5856d6;          /* 品牌紫 = accent/primary */
--mcp-failed: #ff3b30;
--mcp-cancelled: #A39E98;       /* = text/tertiary */

/* P2 — 连接状态色 */
--connection-connected: #5856d6;
--connection-connecting: #ff9500;
--connection-disconnected: #ff3b30;

/* P2 — 文字选择高亮背景 */
--selection-bg: rgba(88,86,214,0.25);
--focus-ring: 0 0 0 2px rgba(88,86,214,0.3);
```

在 `theme/tokens.ts` 中同步注册这些 Token 的 TypeScript 类型。

---

### W3-T2 — 视觉走查

**缺口** G5：按走查表逐项 ⚠️→✅。

按 `desktop-design-development-basis.md` §4 走查表逐项确认。走查方法：

1. 打开 macOS 桌面端应用
2. 对照 `desktop-design-development-basis.md` 中列出的 Token 值和组件参数
3. 截图记录，标注差异
4. 修复差异（修改 CSS 变量值或组件样式）

**重点走查项**：

| ID | 检查内容 | 预期值 | 当前风险点 |
|----|---------|--------|-----------|
| C01 | 线程列表选中竖线 | 3px `#0066FF` | 未确认是否使用 Codex 蓝 |
| C02 | UserBubble 圆角 | 12-4-12-12 | 当前可能为统一圆角 |
| C06 | MCP 设置页使用四状态色 | `--mcp-*` | 需要 W3-T1 完成 Token |
| C08 | 模型列表动态加载 | 无 mock | 依赖 W1-T3 |
| C09 | UsageStrip 位置 | AgentHeader 右侧 | 当前未实现 |

---

### W3-T3 — macOS 公证评估与配置

**缺口** G7：04-tasks.md T3.7 仅完成 ad-hoc 签名，未公证分发。

**评估内容**：

1. Apple Developer Program 账号状态确认（$99/年 开发者账号）
2. 公证依赖检查：
   - Xcode 16+ 的 `notarytool`（替代已废弃的 altool）
   - `Info.plist` 中 `com.apple.security.app-sandbox` 策略
3. `Makefile` 新增 `desktop-notarize` 目标：

```makefile
.PHONY: desktop-notarize
desktop-notarize:
	@echo "=== 公证 Mady.app ==="
	xcrun notarytool submit ./desktop/build/bin/Mady.app \
		--apple-id "$$APPLE_ID" \
		--team-id "$$TEAM_ID" \
		--password "$$APP_PASSWORD" \
		--wait
```

**降级方案**（无开发者账号时）：
- README 添加"首次运行"说明：`xattr -cr Mady.app` 解除隔离
- 输出提示引导用户操作

---

### W3-T4 — Windows 适配

**缺口** G7：04-tasks.md 阶段 4 预留，Windows 适配未开工。

参考 `desktop-design-development-basis.md` §6.1 平台差异：

| 项目 | Windows 值 | 实现方式 |
|------|-----------|---------|
| 标题栏 | 系统标题栏 + Caption Buttons | `desktop/main_windows.go`（build tag `windows`） |
| 字体栈 | Segoe UI Variable 优先 | CSS 中 `--font-sans` 增加 `"Segoe UI Variable", "Segoe UI"` |
| 标题栏高度 | 32px | `--mady-titlebar-height: 32px`（条件覆盖） |
| 滚动条 | 10px 传统式 | `::-webkit-scrollbar` 宽度调整 |
| 快捷键 | Ctrl 替代 Cmd | `stores/commands.ts` 中检测平台 |
| 最小尺寸 | 900×600px | `main_windows.go` 中 `wails.Run` 的 `MinWidth/MinHeight` |

**当前阶段目标**：仅确保 Windows 构建不 panic，功能降级可用（不阻塞）。完整适配可后置。

---

## 依赖关系图

```
W1-T1 ───→ W1-T2         第一波关键路径
  │
W1-T3 / W1-T4 / W1-T5    可独立并行
  │
  ├── 第二波 ──
  │
W2-T1 (TodoDock)           可独立（依赖 reducer）
W2-T2 (CommandPalette)     可独立
  │
  ├── 第三波 ──
  │
W3-T1 (P2 Token)           可随时完成
W3-T2 (视觉走查) ← W1-T3 + W3-T1 完成后进行
W3-T3 (公证)               独立评估
W3-T4 (Windows适配)        独立评估
```

---

## 验收检查清单

| 验收项 | 验证方式 | 关联任务 |
|--------|---------|---------|
| ApprovalCard 点击"批准"后 agent 恢复执行 | E2E 测试 + 手动 | W1-T1, W1-T2 |
| ModelSettings 从后端实际拉取模型列表 | 组件渲染测试 | W1-T3 |
| `make desktop-test` 包含 E2E | CI 检查 | W1-T4 |
| 构建产物主 entry <200KB | `du -sh dist/assets/` | W1-T5 |
| 底部 TodoDock 显示任务列表并可展开 | 手动测试 | W2-T1 |
| ⌘K 打开面板，模糊搜索可找到命令 | 手动测试 | W2-T2 |
| CSS 存在 `--mcp-*` `--connection-*` `--selection-bg` `--focus-ring` | 静态检查 | W3-T1 |
| C01-C12 走查表 ⚠️ 项 ≤3 个 | 视觉走查记录 | W3-T2 |
| 公证可行性评估报告输出 | 文档记录 | W3-T3 |
| Windows 构建不 panic（功能降级提示） | `GOOS=windows go build` | W3-T4 |

---

## 时间估算

| 波次 | 任务 | 估算（人天） | 并行度 |
|------|------|-------------|--------|
| 第一波 | W1-T1 A2UI 入站处理器 | 2-3 | 关键路径 |
| 第一波 | W1-T2 全链路验证 | 0.5 | 依赖 W1-T1 |
| 第一波 | W1-T3 ListModels | 0.5 | 独立 |
| 第一波 | W1-T4 E2E 纳入 | 0.3 | 独立 |
| 第一波 | W1-T5 拆包 | 0.3 | 独立 |
| **第一波合计** | | **3.5-4.5** | |
| 第二波 | W2-T1 TodoDock | 2-3 | 可并行 |
| 第二波 | W2-T2 CommandPalette | 2-3 | 可并行 |
| **第二波合计** | | **4-6** | |
| 第三波 | W3-T1 P2 Token | 0.3 | 可并行 |
| 第三波 | W3-T2 视觉走查 | 1-2 | 依赖 W1-T3 + W3-T1 |
| 第三波 | W3-T3 公证评估 | 0.5-1 | 独立 |
| 第三波 | W3-T4 Windows 适配 | 2-3 | 独立 |
| **第三波合计** | | **4-6** | |

**总计**：11.5-16.5 人天（约 2-3 周全职开发）

---

## 实施建议

1. **先攻 W1-T1**：这是 G1 缺口的关键路径，涉及 `agentcore` 层改动（审查等级 L3）。应优先投入资源并安排人工审阅。

2. **A2UIPromise 模式的安全边界**：`consumePendingA2UIActions` 只在 agent 正在运行的轮次中被调用。如果 agent 已完成（StatusFinished），action 不会被消费——这是正确的行为，因为审批在 agent 暂停时发送才有意义。

3. **W2 并行开发**：W2-T1/W2-T2 彼此独立，可以分配给不同开发者。只需约定统一的 Zustand store 接口。

4. **W3-T2 的时机**：视觉走查需要在 W1-T3（模型列表动态化）和 W3-T1（P2 Token）完成后进行，否则走查结果会被后续修改推翻。

5. **每次提交 3-5 个文件**：遵循 AGENTS.md 的"小炸弹"原则，CI（`make verify`）全过后再推进下一步。

6. **A2UI 事件 vs A2UIPromise**：本计划采用 Promise 而非在 EventBus 上做 request-response，原因是 EventBus 是 fire-and-forget 的广播机制，不适合"写一次读一次"的语义。Promise 模式在每个 agent 实例上只分配一次，零开销直到被使用。
