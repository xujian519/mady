# 桌面端下一阶段开发计划

> **制定日期**：2026-07-29 | **修订**：2026-07-31（新增第四波「规范对齐与质量门禁」，依据 [mady-desktop-standards.md](../mady-desktop-standards.md) §14 差距清单）
> **依据**：缺口分析（G1-G7）、[04-tasks.md](../specs/desktop/04-tasks.md)、[desktop-design-development-basis.md](../specs/desktop/desktop-design-development-basis.md)、[mady-desktop-standards.md](../mady-desktop-standards.md)
> **四波推进**：第一波「闭环真实化」→ 第二波「领域差异化」→ 第三波「视觉收尾与发布」→ 第四波「规范对齐与质量门禁」

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
| 第四波 | W4-T1 | P0-1 | 组件测试环境补齐（vitest jsdom + jest-dom） | `vitest.component.config.ts`, `package.json` | 低 | L1 |
| 第四波 | W4-T2 | P0-2 | 构建产物入库治理（.gitignore + git rm） | `desktop/.gitignore`, `git` | 低 | L1 |
| 第四波 | W4-T3 | P0-3 | 事件监听 cleanup 契约审计 | `agui-bridge/client.ts`, `App.tsx` | 低 | L2 |
| 第四波 | W4-T4 | P1-1 | wailsjs 类型漂移校验（CI 契约测试） | `scripts/`, CI | 低 | L1 |
| 第四波 | W4-T5 | P1-2 | Zustand store 按 slices 切分 | `stores/chat.ts` 等 | 中 | L2 |
| 第四波 | W4-T6 | P1-5 | TanStack Query 接管只读列表 | `stores/`, `components/` | 中 | L2 |
| 第四波 | W4-T7 | P1-3 | 暗色模式三态切换（@custom-variant dark） | `globals.css`, `theme/provider.tsx` | 中 | L2 |
| 第四波 | W4-T8 | P1-6 | HIG 视觉走查（可折叠侧栏/toolbar 对齐） | `Sidebar.tsx`, 各组件 CSS | 中 | L2 |
| 第四波 | W4-T9 | P1-4 | 前端 i18n 评估（react-i18next 对齐 pkg/i18n） | `frontend/`, 文档 | 低 | L1 |
| 第四波 | W4-T10 | P2-3 | CI 对比度审计（WCAG AA） | `scripts/`, CI | 低 | L1 |
| 第四波 | W4-T11 | P2-5 | 托盘 + 长任务完成通知 | `main.go`, `app.go` | 中 | L2 |
| 第四波 | W4-T12 | P2-6 | 自动更新预留与评估 | `app.go`, 文档 | 低 | L1 |
| 第四波 | W4-T13 | P2-7 | 布局/面板比例持久化 | `window_state.go`, `stores/` | 低 | L1 |

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

## 第四波：规范对齐与质量门禁（工程收尾，2026-07-31 新增）

> **依据**：[mady-desktop-standards.md](../mady-desktop-standards.md) §14 差距清单（P0-1/P0-2/P0-3、P1-1~P1-6、P2-3/P2-5/P2-6/P2-7）。
> **说明**：P2-1（⌘K 命令面板）与 P2-2（TodoDock）已由 W2-T1/W2-T2 覆盖，P2-4（Windows 适配）已由 W3-T4 覆盖，此处不重复建任务；第四波聚焦规范识别的**新差距**。
> **建议节奏**：P0 项（W4-T1~T3）是质量门禁，可在第三波期间并行完成，不必等前三波全部结束。
>
> **✅ 执行状态（2026-07-31）**：W4-T1~T13 全部完成。W4-T2 核实为误报（.gitignore 已覆盖）；W4-T3 审计零违规；W4-T9/W4-T12 产出评估文档；其余任务代码落地。验证：前端 typecheck + 100 测试 + build 全过，desktop `go build`/`go vet`/`go test` 全过。遗留：对比度脚本发现的 5 类真实问题待视觉走查修复（见 W4-T10 小节），W4-T4 的 CI 接入步骤与 W4-T10 的 CI 集成待与既有 CI 一并配置。

### W4-T1 — 组件测试环境补齐（P0-1）

**缺口** P0-1（M-DSK-TST-002）：`vitest.config.ts` 为 `environment: 'node'`，`*.test.tsx` 组件测试（jsdom + `@testing-library/jest-dom` matchers）未被覆盖；现有 `src/components/__tests__/*.test.tsx` 与 `src/a2ui-renderer/__tests__/*.test.tsx` 实际未跑。

**当前状态追踪**：

- `package.json` 已有 `@testing-library/react` / `@testing-library/jest-dom` / `@testing-library/user-event` / `jsdom` 依赖 ✅
- `vitest.config.ts` 仅 include `src/**/*.test.ts` / `*.test.tsx`，但 environment=node、无 jest-dom setup ❌

**设计方案**：新建独立组件测试配置（不动现有纯函数配置），两种方案择一：

```ts
// 方案 A：vitest.component.config.ts（推荐，配置隔离最清晰）
import { defineConfig } from 'vitest/config'
export default defineConfig({
  resolve: { alias: { '@': new URL('./src', import.meta.url).pathname } },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.tsx'],
    setupFiles: ['./src/test/setup.ts'], // 引入 '@testing-library/jest-dom'
  },
})
```

```ts
// 方案 B：vitest.config.ts 用 projects 分流（单一配置，复杂度略高）
// test.projects = [{ environment: 'node', include: ['src/**/*.test.ts'] },
//                  { environment: 'jsdom', include: ['src/**/*.test.tsx'], setupFiles: [...] }]
```

- `package.json` 脚本改为 `"test": "vitest run && vitest run -c vitest.component.config.ts"`（或 projects 分流后单命令）
- 注意：升级 Vitest 大版本前验证其 Vite 版本要求（当前 vite 5.4 / vitest 3.2.7 组合已在用，勿贸然升级）

**验收标准**：

1. `pnpm test` 实际执行 `*.test.tsx` 组件测试（此前为静默跳过）
2. 现有组件测试（`TodoDock.test.tsx`、`CommandPalette.test.tsx`、`toolcard.test.ts` 等）全绿
3. 新增组件必须带组件测试（M-DSK-TST-001 门禁）

### W4-T2 — 构建产物入库治理（P0-2，已核实为误报）

> **2026-07-31 核实结论**：**非差距，无需修复**。`desktop/` 构建产物已被正确忽略：
> - `desktop/Mady` → 根 `.gitignore:104`（`desktop/Mady`）
> - `desktop/desktop.exe` → 根 `.gitignore:3`（`*.exe`）
> - `desktop/build/bin` → 根 `.gitignore:16`（`build/`）
> - `frontend/node_modules` / `frontend/dist` → `desktop/.gitignore` 第 6-7 行
>
> `git status --porcelain desktop/` 验证无二进制跟踪。本节保留为**防回归检查**（可选）：CI 中加 `git status --porcelain` 静默检查，出现未忽略的构建产物即失败。

**缺口** P0-2（M-DSK-WLS-003）：`desktop/` 下存在已构建产物 `Mady` / `desktop.exe`（二进制），疑似被 git 跟踪，导致仓库膨胀与跨机器二进制不一致。

**设计方案**：

1. ~~检查 `desktop/.gitignore` 现状~~ ✅ 已核实：根 `.gitignore` + `desktop/.gitignore` 共同覆盖全部产物
2. ~~`git rm --cached`~~ ✅ 已核实：二进制未被跟踪，无需操作
3. ~~`.gitignore` 补齐~~ ✅ 已覆盖
4. CI 增加静态检查：`git status --porcelain` 不允许出现未忽略的构建产物（**可选防回归**，与 W4-T4 的 CI 步骤合并实施）

**验收标准**：

1. `git status` 无 `Mady` / `desktop.exe` 二进制 ✅ 已满足
2. `.gitignore` 覆盖 `build/bin` / `frontend/dist` / `node_modules` ✅ 已满足
3. CI 产物检查步骤存在（防回归，随 W4-T4 一起加）

### W4-T3 — 事件监听 cleanup 契约审计（P0-3）

**缺口** P0-3（M-DSK-WLS-010）：Wails v2 事件监听未清理会导致组件重挂载后回调重复执行、内存累积（issue #3796/#4683）。`src/lib/wails.ts` 已封装「返回取消函数」模式，但需审计所有调用方是否持有并调用。

**审计清单**：

- [ ] `agui-bridge/client.ts` — 每个 `listenToWailsEvent` 调用点的 useEffect cleanup
- [ ] `app/App.tsx` — `mady:init-*` 与 AGUI 事件订阅的清理
- [ ] `components/` 中所有直接订阅 Wails 事件的组件（`grep -rn "listenToWailsEvent" src/`）
- [ ] 确认不在事件 handler 内部调用取消函数（M-DSK-WLS-011，issue #4393）

**验收标准**：

1. 审计报告列出全部调用点及 cleanup 状态（✅/❌）
2. 缺失处修复：useEffect 返回取消函数
3. 新增订阅代码必须遵循「useEffect + cleanup」模式（Code Review 检查项）

### W4-T4 — wailsjs 类型漂移校验（P1-1）

**缺口** P1-1（M-DSK-TST-005）：`backend.ts` 包装类型与 `wailsjs` 生成类型可能漂移（Go 侧改字段后前端静默失配）。

**设计方案**：

- 方案 A（轻量，推荐先做）：CI 步骤 `wails generate module` 后 `git diff --exit-code frontend/wailsjs/`——生成物与仓库不一致即失败，防止忘提交生成物
- 方案 B（增强）：新增契约测试，断言 `backend.ts` 包装函数签名与 `wailsjs/go/main/App.d.ts` 生成签名一致（`typeof` 派生对比）
- 两种方案都依赖 CI 安装 Wails CLI（macOS runner）

**验收标准**：

1. CI 契约步骤存在且可拦截漂移
2. `tsc --noEmit` 全绿（已有门禁保持）

### W4-T5 — Zustand store 按 slices 切分（P1-2）

**缺口** P1-2（M-DSK-ST-005）：`stores/chat.ts` 为单文件大 store（~30 字段 + 全部 actions），继续膨胀将难以维护。

**设计方案**（参照 [Zustand Slices 模式](https://zustand.docs.pmnd.rs/guides/slices-pattern)）：

```ts
// stores/chat.ts — 组合入口
interface AppState extends ChatSlice, ThreadsSlice, CommandsSlice, SettingsSlice {}
export const useAppStore = create<AppState>()((...args) => ({
  ...createChatSlice(...args),
  ...createThreadsSlice(...args),
  ...createCommandsSlice(...args),
  ...createSettingsSlice(...args),
}))
```

- 切分边界：`chatSlice`（消息流/输入态/流式状态）、`threadsSlice`（会话列表）、`commandsSlice`（⌘K 命令注册表，与 W2-T2 衔接）、`settingsSlice`（主题/面板开关）
- 组件订阅用 selector + `useShallow`（复合值），避免无关字段引发整树重渲染（M-DSK-ST-004）
- 行为不变量：重构期间 `stores/` 对外导出保持兼容或一次性全量迁移

**验收标准**：

1. `chat.ts` 拆分为 slices 文件，组合 store 对外行为不变
2. 现有测试（store 相关）全绿；`-race`/StrictMode 无异常
3. 组件订阅审计：使用 selector，复合值用 `useShallow`

### W4-T6 — TanStack Query 接管只读列表（P1-5）

**缺口** P1-5（M-DSK-ST-002）：`ListProjects` / `ListThreads` / `ListModels` / `ListMcpServers` / `GetKnowledgeStatus` 等只读后端数据散落在组件 Effect + Zustand 中，无缓存与失效机制。

**设计方案**（参照 [TanStack Query 官方指南](https://tanstack.com/query/latest/docs/framework/react/guides/queries)）：

- 新建 `src/queries/` 目录，每个只读列表一个 query hook（`useProjects()` / `useThreads()` / `useModels()` / `useMcpServers()` / `useKnowledgeStatus()`）
- queryKey 唯一分层：`['projects']` / `['threads']` / `['models']` / `['mcp']` / `['knowledge']`
- 写操作（如 `SetAISettings` 后刷新模型）用 `useMutation` + `invalidateQueries`（M-DSK-ST-003）
- 流式会话状态（`stores/chat.ts`）**不迁移**，保持 Zustand（M-DSK-ST-001 分工原则）
- 移除组件内手写数据拉取 Effect

**验收标准**：

1. 只读列表全部走 Query，组件无手写拉取 Effect
2. mutation 后列表自动失效刷新（如切换模型后模型列表/状态更新）
3. 加载态/错误态/重试按 Query 三态渲染

### W4-T7 — 暗色模式三态切换（P1-3，需产品决策）

**缺口** P1-3（M-DSK-TW-003）：`02-spec.md` §5.6 规划「跟随系统 / 浅色 / 深色」三档，当前 `globals.css` 用 `@media (prefers-color-scheme: dark)` 只跟随系统，两处不一致。

**待决策**（[NEEDS CLARIFICATION]）：

- 三档切换是否本期交付？（设置面板已有 `theme/provider.tsx` 的 `mode: 'light' | 'dark' | 'system'` 状态，但 CSS 令牌层未接入 class 策略）

**迁移方案**（决策为「是」时）：

```css
/* globals.css — Tailwind v4 class 策略（一行切换 dark: variant 语义） */
@custom-variant dark (&:where(.dark, .dark *));
/* 现有 @media (prefers-color-scheme: dark) 覆盖迁移为 .dark 下的变量覆盖 */
```

- 主题初始化：head 内联脚本读 localStorage + `matchMedia('(prefers-color-scheme: dark)')` 防 FOUC（shadcn 官方 Vite 示例模式）
- 手动切换即写 `document.documentElement.classList` + 持久化
- `theme/provider.tsx` 的 `resolved` 计算同步更新（跟随系统时监听 matchMedia change）
- 高对比模式（`prefers-contrast: more`）保持媒体查询不变

**验收标准**：

1. 三档切换生效且重启后保持
2. 切换无闪烁（FOUC）
3. 深浅色下 `prefers-contrast` / `prefers-reduced-motion` 行为不回退

### W4-T8 — HIG 视觉走查（可折叠侧栏 / toolbar 对齐，P1-6）

**缺口** P1-6（M-DSK-VIS-002/005）：侧栏固定 260px 无折叠模式；工具栏项样式未按 HIG「无 bezel、单 primary」走查。

**设计方案**：

1. **侧栏可折叠**：`Sidebar.tsx` 增加收起态（48px 图标列，对应 `--mady-sidebar-width` / 规范 `sidebar-collapsed: 48px`）；窄窗口（<900px）自动折叠；折叠/恢复双入口（按钮 + ⌘B 快捷键）
2. **toolbar 对齐**：工具栏项默认透明无描边、hover 才有底；每屏只设一个 accent 主按钮置于右侧；分组 ≤3
3. **TitleBar 检查**：`TitleBarHiddenInset` 下交通灯不与侧栏图标/按钮重叠（`padding-left` 预留约 80px）
4. 其余 HIG 项对照 `mady-desktop-standards.md` §6 走查（关键操作不放底部、窗口标题语义化等）

**验收标准**：

1. 侧栏可折叠、窄窗口自动折叠、⌘B 恢复
2. 工具栏无 bezel、单 primary、分组 ≤3
3. 交通灯不重叠 UI（走查截图）

### W4-T9 — 前端 i18n 评估（P1-4）

**缺口** P1-4（案例参考 §13.1）：后端已有 `pkg/i18n`（zh-CN/en-US），前端 UI 文案未接入翻译框架；专利/法律术语的翻译一致性对专业产品尤其重要（参考 tiny-rdm 12 语言、WailBrew 11 语言）。

**评估范围**（本期为评估 + 可行性原型，不做全量翻译）：

- 方案：`react-i18next` + JSON 资源，术语表与 `pkg/i18n` 对齐（共享一份术语源）
- 评估点：① 文案抽取范围（组件内硬编码字符串清单）② 与后端 i18n 的术语一致性机制 ③ 切换入口与默认语言（zh-CN）

**验收标准**：

1. 评估报告输出（范围/方案/工作量）
2. 原型验证：至少一个关键页面（如设置面板）可切换 zh-CN/en-US

### W4-T10 — CI 对比度审计（P2-3）

**缺口** P2-3（M-DSK-TST-008）：`--color-mady-*` 令牌对文字/背景的 WCAG 对比度无自动化验证（与 TUI 规范 M-TUI-TST-005 对齐）。

**设计方案**：

- 脚本 `scripts/check-color-contrast.mjs`（或 .ts）：解析 `globals.css` 的 `@theme` 令牌（light/dark 两套），对「文字色 × 背景色」组合计算对比度，低于 4.5:1（小文本）/ 3:1（大文本或加粗）输出失败
- 组合矩阵：`text-primary/secondary/tertiary × bg-primary/secondary/tertiary/sidebar` + 语义色（danger/warning/success/info）× 常见背景
- 纳入 CI（`make desktop-test` 或独立 job）

**验收标准**：

1. 脚本可解析令牌并输出对比度报告
2. CI 中对比度不达标即失败（当前令牌应全过或列出豁免清单）

### W4-T11 — 托盘 + 长任务完成通知（P2-5）

**缺口** P2-5（M-DSK-PKG-005）：长分析任务（专利检索/证据判断，分钟级）无系统通知；窗口最小化后用户无法感知完成。

**设计方案**：

- macOS 托盘：Wails v2 无内置托盘 API，评估 `getlantern/systray` 或原生菜单栏集成（P2 阶段可先用最小实现：最小化到托盘 + Dock 通知）
- 长任务完成通知：`agui:done` / `agui:error` 事件到达且窗口非激活时发系统通知（`runtime` 或原生 `NSUserNotification` / macOS 10.14+ `UNUserNotificationCenter`）
- 托盘图标复用 `build/appicon.png`

**验收标准**：

1. 长任务完成且窗口失焦时弹出系统通知
2. 最小化到托盘不退出（可选，若评估可行）
3. 通知点击可聚焦窗口

### W4-T12 — 自动更新预留与评估（P2-6）

**缺口** P2-6（M-DSK-PKG-003）：Wails 生态无官方 autoupdate；参考 RWKV-Runner（内置更新 + 保留用户配置）与 jcp（前端 `updateService.ts`）。

**评估范围**：

- 版本接口复用 `Health().Version`（commitHash + buildTime）
- 评估方案：Sparkle（macOS 原生，需签名）、`go-update` 类自实现（HTTP 拉取 + 校验 + 替换二进制）、或 GitHub Releases + 手动下载引导
- 预留：`app.go` 增加 `CheckUpdate()` 绑定方法占位 + 设置面板「检查更新」入口（本期可不实现真实更新）

**验收标准**：

1. 评估报告输出（三方案对比 + 推荐）
2. `CheckUpdate()` 绑定占位存在（返回「当前版本」即可）

### W4-T13 — 布局/面板比例持久化（P2-7）

**缺口** P2-7（案例参考 §13.3-6）：多面板（侧栏/聊天/文件查看器/Agent 面板）的 split 比例与面板开关状态未持久化；`window_state.go` 已持久化窗口几何。

**设计方案**：

- 面板状态并入 `window_state.go` 或 `desktop-settings.json`：`{ sidebarCollapsed, sidebarWidth, agentPanelWidth, fileViewerOpen }`
- 前端 `stores/settings.ts`（W4-T5 slices 切分后的 settingsSlice）持久化开关状态，split 宽度经 `SaveWindowState` 扩展或新增 `SaveLayout` binding
- 启动时恢复（参考 jcp「布局持久化——自动保存窗口和面板布局，下次启动自动恢复」）

**验收标准**：

1. 调整面板比例/开关后重启，布局恢复
2. 多窗口场景不串台（按窗口 ID 或全局一份，本期全局一份即可）

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
  │
  ├── 第四波（规范对齐）──
  │
W4-T1 (组件测试环境) ← W1-T4 之后（前端测试门禁完整）
W4-T2 (产物治理)          独立，可随时完成
W4-T3 (事件 cleanup 审计)  独立
W4-T4 (wailsjs 契约校验) ← CI 就绪后
W4-T5 (Zustand slices)     独立（约定与 W2-T2 的 commands 共用接口）
W4-T6 (TanStack Query)   ← W1-T3 完成后（ListModels 动态化）
W4-T7 (暗色三态)          待产品决策
W4-T8 (HIG 走查)         ← W1-T3 + W3-T1 完成后进行（与 W3-T2 合并走查）
W4-T9 (i18n 评估)         独立
W4-T10 (对比度审计)        独立
W4-T11 (托盘/通知)         独立
W4-T12 (自动更新评估)      独立
W4-T13 (布局持久化)       ← W4-T5 之后（settingsSlice 承接）
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
| `pnpm test` 实际执行 `*.test.tsx` 组件测试且全绿 | `pnpm test` | W4-T1 |
| `git status` 无 `Mady` / `desktop.exe` 二进制，CI 产物检查存在 | `git status --porcelain` | W4-T2 |
| 事件监听 cleanup 审计报告输出，缺失处已修复 | 审计报告 + grep | W4-T3 |
| CI 契约步骤存在（wails generate 后无 diff / 类型对比） | CI 检查 | W4-T4 |
| chat.ts 拆分 slices，组件订阅用 selector + useShallow | 代码审查 | W4-T5 |
| 只读列表全部走 TanStack Query，mutation 后自动失效刷新 | 组件测试 + 手动 | W4-T6 |
| 暗色三态切换生效、无 FOUC、重启保持 | 手动测试 | W4-T7 |
| 侧栏可折叠、toolbar 无 bezel、交通灯不重叠 | 视觉走查截图 | W4-T8 |
| i18n 评估报告输出 + 至少一个页面可切换语言 | 文档记录 + 原型 | W4-T9 |
| 对比度审计脚本进 CI，不达标即失败 | CI 检查 | W4-T10 |
| 长任务完成且窗口失焦时弹出系统通知 | 手动测试 | W4-T11 |
| 自动更新评估报告输出，`CheckUpdate()` 占位存在 | 文档记录 | W4-T12 |
| 调整面板比例/开关后重启布局恢复 | 手动测试 | W4-T13 |

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
| 第四波 | W4-T1 组件测试环境 | 0.5 | 独立 |
| 第四波 | W4-T2 产物治理 | 0.3 | 独立 |
| 第四波 | W4-T3 cleanup 审计 | 0.5 | 独立 |
| 第四波 | W4-T4 契约校验 | 0.3 | 独立 |
| 第四波 | W4-T5 Zustand slices | 1-2 | 可并行 |
| 第四波 | W4-T6 TanStack Query | 1-1.5 | 依赖 W1-T3 |
| 第四波 | W4-T7 暗色三态 | 1-2 | 待决策 |
| 第四波 | W4-T8 HIG 走查 | 1-2 | 依赖 W1-T3 + W3-T1 |
| 第四波 | W4-T9 i18n 评估 | 0.5-1 | 独立 |
| 第四波 | W4-T10 对比度审计 | 0.3-0.5 | 独立 |
| 第四波 | W4-T11 托盘/通知 | 1-2 | 独立 |
| 第四波 | W4-T12 自动更新评估 | 0.5-1 | 独立 |
| 第四波 | W4-T13 布局持久化 | 0.5 | 依赖 W4-T5 |
| **第四波合计** | | **8.4-14.1** | |

**总计**：19.9-30.6 人天（约 4-6 周全职开发；第四波 P0 项可随第三波并行推进）

---

## 实施建议

1. **先攻 W1-T1**：这是 G1 缺口的关键路径，涉及 `agentcore` 层改动（审查等级 L3）。应优先投入资源并安排人工审阅。

2. **A2UIPromise 模式的安全边界**：`consumePendingA2UIActions` 只在 agent 正在运行的轮次中被调用。如果 agent 已完成（StatusFinished），action 不会被消费——这是正确的行为，因为审批在 agent 暂停时发送才有意义。

3. **W2 并行开发**：W2-T1/W2-T2 彼此独立，可以分配给不同开发者。只需约定统一的 Zustand store 接口。

4. **W3-T2 的时机**：视觉走查需要在 W1-T3（模型列表动态化）和 W3-T1（P2 Token）完成后进行，否则走查结果会被后续修改推翻。

5. **每次提交 3-5 个文件**：遵循 AGENTS.md 的"小炸弹"原则，CI（`make verify`）全过后再推进下一步。

6. **A2UI 事件 vs A2UIPromise**：本计划采用 Promise 而非在 EventBus 上做 request-response，原因是 EventBus 是 fire-and-forget 的广播机制，不适合"写一次读一次"的语义。Promise 模式在每个 agent 实例上只分配一次，零开销直到被使用。

7. **第四波 P0 项尽早并行**：W4-T1（组件测试环境）与 W4-T2（产物治理）是质量门禁，建议在第三波期间就完成——它们不受前三波依赖，且能让后续 W4-T5/T6/T8 的改动一开始就有组件测试兜底。

8. **W4-T7 需产品决策先行**：暗色模式三态切换（`@custom-variant dark` class 策略迁移）会改动全局 CSS 令牌层，方案存在 [NEEDS CLARIFICATION]（三档是否本期交付）。决策前不要动 `globals.css` 的暗色覆盖结构。

9. **W4-T8 与 W3-T2 合并走查**：HIG 视觉走查与既有「按走查表逐项 ⚠️→✅」目标一致，统一在 W1-T3 + W3-T1 完成后执行一次，避免重复走查推翻结果。

10. **W4 各任务遵循「小炸弹」**：每次提交 3-5 个文件；W4-T5（slices 切分）与 W4-T6（Query 接管）改动面较大，按 store/组件分批提交，每批 `make desktop-test` 全过再推进。
