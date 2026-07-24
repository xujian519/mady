# PlanTask 引入计划

> 状态：草案 · 2026-07-24
> 原则：不能减少或降低现有功能；借鉴 eino `adk/middlewares/plantask` 优秀设计，按 Mady 架构原生方式实现

## 1. 背景与动机

### 1.1 现状缺口

Mady 在专利/法律场景下频繁面对**步骤数运行时才能确定**的复杂任务（OA 答复、无效宣告、侵权比对）。
现有机制各有侧重，但缺少"让 LLM 自管理结构化待办清单"这一能力：

| 现有机制 | 覆盖范围 | 缺口 |
|---------|---------|------|
| `disclosure`（11 节点 Pregel） | 编译期固定的图 | 无法适配动态步骤 |
| `specdrafting`（12 节点 Pregel） | 编译期固定的图 | 同上 |
| `planmode`（工具白名单门控） | 限制工具集 | 不追踪进度 |
| `ReasoningStrategy`（6 种策略注入） | 推理提示 | 不追踪执行 |
| `agentcore/task_tool.go` | 子 Agent 委派 | 不是任务清单 |

代码中已存在两个**预留但未实现**的接入点：
- `agentcore/planmode/policy.go:35` — `"todo": true`，将 todo 列为 always-allowed 工具，但全项目无 todo 工具实现
- `tui/component/todo_panel.go` — 完整的 TodoPanel 组件（272 行），含状态图标/优先级/键盘交互/主题，**从未被实例化**

### 1.2 eino plantask 的可借鉴设计

eino `adk/middlewares/plantask` 提供 4 个工具（TaskCreate/TaskGet/TaskUpdate/TaskList）+ 文件持久化 Backend，
核心设计亮点：

- **任务依赖图**：`blocks` / `blockedBy` 双向引用 + DFS 循环检测
- **状态机**：pending → in_progress → completed / deleted
- **Backend 抽象**：LsInfo/Read/Write/Delete 接口，可适配多种存储
- **highwatermark**：保证 ID 单调递增
- **全部完成自动清理**
- **双语提示词**：详细使用指南（何时用/何时不用）

### 1.3 不直接照搬的部分

| eino 设计 | 问题 | Mady 调整 |
|-----------|------|-----------|
| highwatermark 独立文件 | 多文件管理复杂 | 用 SQLite 自增主键或内存原子计数器 |
| 全部完成自动删除 | 专利法律需审计留痕 | 改为标记归档（`archived`），不物理删除 |
| 单一 `sync.Mutex` | 多 Agent 并发瓶颈 | per-session 锁 + 乐观检查 |
| 无优先级字段 | 专利撰写有天然优先级 | 增加 `priority` 字段 |
| 中间件注入方式 | Mady 用 Extension 机制 | 改为 `agentcore.Extension` |
| eino `Backend` 接口 | 面向文件系统 | Mady 用 `session/` 目录沙箱化 |

---

## 2. 目标

| 目标 | 衡量标准 |
|------|---------|
| LLM 能自管理结构化待办清单 | 4 个工具（create/get/update/list）可用 |
| 任务依赖关系可表达 | blocks/blockedBy + 循环检测 |
| 进度实时可视化 | TodoPanel 接入数据源，TUI 实时刷新 |
| planmode 集成 | `todo` 工具在计划模式下可用（create/update 受门控） |
| 事件总线集成 | 任务变更 emit 事件，TUI/SSE 可订阅 |
| 持久化与审计 | per-session 持久化，支持归档查询 |
| 零功能退化 | 现有所有测试通过，现有工具不受影响 |

---

## 3. 架构设计

### 3.1 分层定位

```
TUI 层：  tui/component/todo_panel.go（已有，需接线数据源）
              ↑ EventTaskCreated/Updated
接口层：  agentcore/event_types.go（新增 2 个事件类型）
              ↑ emit
核心层：  agentcore/tasklist/（新增包）
          ├── extension.go    — Extension 实现（ToolProvider）
          ├── store.go        — Store 接口 + 文件实现
          ├── task.go         — Task 数据模型
          ├── tool_create.go  — TaskCreate 工具
          ├── tool_get.go     — TaskGet 工具
          ├── tool_update.go  — TaskUpdate 工具
          └── tool_list.go    — TaskList 工具
              ↑ 注册到
装配层：  cmd/mady/framework.go（追加一行 Extension 注册）
```

### 3.2 核心数据模型

```go
// agentcore/tasklist/task.go

// Task 表示一个结构化待办事项。
type Task struct {
    ID          string         `json:"id"`           // 单调递增，"1"/"2"/...
    Subject     string         `json:"subject"`      // 祈使句标题
    Description string         `json:"description"`  // 详细描述
    Status      TaskStatus     `json:"status"`       // pending/in_progress/completed/archived
    Priority    TaskPriority   `json:"priority"`     // low/normal/high/urgent
    Blocks      []string       `json:"blocks"`       // 本任务阻塞的任务 ID 列表
    BlockedBy   []string       `json:"blocked_by"`   // 阻塞本任务的任务 ID 列表
    ActiveForm  string         `json:"active_form"`  // 进行中时 spinner 显示文案
    Owner       string         `json:"owner"`        // 认领该任务的 Agent 名称
    Metadata    map[string]any `json:"metadata"`     // 自由扩展（如领域标签）
    CreatedAt   time.Time      `json:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
}

type TaskStatus   string  // pending / in_progress / completed / archived
type TaskPriority string  // low / normal / high / urgent
```

相比 eino 的差异：
- `Status` 新增 `archived`（替代 deleted，保留审计留痕）
- 新增 `Priority`（专利撰写场景需要优先级排序）
- 新增 `CreatedAt` / `UpdatedAt`（审计需求）

### 3.3 Store 接口（持久化抽象）

```go
// agentcore/tasklist/store.go

// Store 是任务持久化的抽象接口。
// 实现可以是文件系统（FileStore）、内存（MemoryStore，用于测试）或 SQLite。
type Store interface {
    Create(ctx context.Context, t *Task) error
    Get(ctx context.Context, id string) (*Task, error)
    Update(ctx context.Context, t *Task) error
    List(ctx context.Context) ([]*Task, error)
    Delete(ctx context.Context, id string) error
    NextID(ctx context.Context) (string, error)
}
```

相比 eino 的 `Backend`（LsInfo/Read/Write/Delete 面向文件），Mady 的 Store 是**领域语义接口**（Create/Get/Update/List），实现细节不暴露给上层。FileStore 内部用 JSON 文件 + 原子写入。

### 3.4 Extension 实现

```go
// agentcore/tasklist/extension.go

type Extension struct {
    store    Store
    agent    *agentcore.Agent
    mu       sync.Mutex  // 保护 store 操作的串行化
}

// 实现 agentcore.Extension 接口
func (e *Extension) Name() string { return "tasklist" }
func (e *Extension) Init(_ context.Context, agent *agentcore.Agent) error { e.agent = agent; return nil }
func (e *Extension) Dispose() error { return nil }

// 实现 agentcore.ToolProvider 接口
func (e *Extension) Tools() []*agentcore.Tool {
    return []*agentcore.Tool{
        newCreateTool(e.store, &e.mu, e.agent),
        newGetTool(e.store, &e.mu),
        newUpdateTool(e.store, &e.mu, e.agent),
        newListTool(e.store, &e.mu),
    }
}

// 实现 agentcore.EventSnapshotProvider 接口（供新挂载的 TUI 获取当前状态）
func (e *Extension) SnapshotEvents() []agentcore.Event { ... }
```

注册方式与 evidence/planmode/filecheckpoint 完全一致——在 `cmd/mady/framework.go` 的 Extension 组装段追加一行。

### 3.5 事件类型

```go
// agentcore/event_types.go 新增

EventTaskCreated  EventType = "task_created"
EventTaskUpdated  EventType = "task_updated"

type TaskCreatedEvent struct {
    baseEvent
    Task *tasklist.Task `json:"task"`
}

type TaskUpdatedEvent struct {
    baseEvent
    Task     *tasklist.Task `json:"task"`
    OldStatus string         `json:"old_status"`
    NewStatus string         `json:"new_status"`
}
```

注意：为避免 `agentcore` → `agentcore/tasklist` → `agentcore` 循环导入，Task 类型定义在 `agentcore/tasklist` 包中，
事件类型定义在 `agentcore` 包中但引用 `tasklist.Task`（`agentcore/tasklist` 只 import `agentcore`，不反向）。

### 3.6 工具定义规范

遵循 Mady 现有 Tool 定义规范（参照 `agentcore/tool.go`）：

| 工具 | ReadOnly | 说明 |
|------|----------|------|
| TaskCreate | `false` | 有副作用（写存储），planmode 下受门控 |
| TaskUpdate | `false` | 有副作用，planmode 下受门控 |
| TaskGet | `true` | 只读，planmode 下始终允许 |
| TaskList | `true` | 只读，planmode 下始终允许 |

planmode 集成：`policy.go` 的 `alwaysAllowed` 中 `"todo": true` 需更新为 `"task_list": true, "task_get": true`，
TaskCreate/TaskUpdate 走标准 ReadOnly 门控（planmode 下被阻止）。

### 3.7 TUI 接线

TodoPanel 已有完整渲染逻辑，只需：
1. 在 `tui/chat/` 中实例化 TodoPanel
2. 订阅 `EventTaskCreated` / `EventTaskUpdated` 事件
3. 事件回调中调用 `dataProvider` 刷新 items
4. 将 TodoPanel 的 `TodoItem` 从 tasklist.Task 映射

```go
// 映射函数
func taskToTodoItem(t *tasklist.Task) component.TodoItem {
    return component.TodoItem{
        ID:       t.ID,
        Content:  t.Subject,
        Status:   string(t.Status),
        Priority: string(t.Priority),
    }
}
```

---

## 4. 实施阶段

### Phase 1：核心包（agentcore/tasklist/）— 3 天

| 文件 | 职责 | 行数估算 |
|------|------|---------|
| `task.go` | Task 数据模型 + 状态/优先级常量 | ~80 |
| `store.go` | Store 接口 + MemoryStore（测试用） | ~120 |
| `filestore.go` | FileStore 实现（JSON + 原子写入 + 沙箱化） | ~200 |
| `extension.go` | Extension 实现（ToolProvider + EventSnapshot） | ~80 |
| `tool_create.go` | TaskCreate 工具 | ~120 |
| `tool_get.go` | TaskGet 工具 | ~60 |
| `tool_update.go` | TaskUpdate 工具（含依赖图维护 + 循环检测） | ~200 |
| `tool_list.go` | TaskList 工具（按优先级+ID排序） | ~70 |

**验证标准**：`go test ./agentcore/tasklist/...` 全部通过，含依赖图循环检测、并发写入、ID 单调递增等测试。

### Phase 2：事件 + 装配 — 1 天

| 文件 | 改动 |
|------|------|
| `agentcore/event_types.go` | 新增 `EventTaskCreated` / `EventTaskUpdated` + 构造函数 |
| `cmd/mady/framework.go` | Extension 组装段追加 `tasklist.NewExtension(taskDir)` |
| `agentcore/planmode/policy.go` | `alwaysAllowed` 更新：移除 `"todo"`，新增 `"task_list"` / `"task_get"` |

**验证标准**：`go build ./...` 通过；启动 mady 后 Agent 拥有 4 个 task 工具。

### Phase 3：TUI 接线 — 2 天

| 文件 | 改动 |
|------|------|
| `tui/chat/chat_app.go` | 实例化 TodoPanel，订阅 task 事件 |
| `tui/chat/state.go` | 增加 TodoPanel 状态字段 |
| `tui/component/todo_panel.go` | 适配 Task → TodoItem 映射（如有字段差异） |

**验证标准**：TUI 中创建/更新任务后，TodoPanel 实时刷新。

### Phase 4：i18n + 提示词 — 1 天

| 文件 | 改动 |
|------|------|
| `pkg/i18n/catalog.go` 或对应 YAML | 新增 task 工具相关翻译键 |
| `agentcore/tasklist/prompts.go` | 双语工具描述（参照 eino 的详细使用指南） |

提示词需遵循 `docs/tone-style-guide.md`：
- 不使用绝对化表述
- 拒绝类文案提供替代性帮助
- 结论性表述附带置信度

### Phase 5：测试 + 文档 — 2 天

| 内容 | 范围 |
|------|------|
| 单元测试 | store.go（CRUD + 并发）、tool_update.go（循环检测）、extension.go（事件发射） |
| 集成测试 | 端到端：Agent.Run → LLM 调用 task 工具 → 事件 → TodoPanel 刷新 |
| `docs/decisions/AI_CHANGELOG.md` | 追加变更记录 |
| `docs/specs/plantask-introduction/` | 本计划文档归档 |

---

## 5. 关键设计决策

### 5.1 为什么用 Extension 而非中间件

Mady 没有 eino 的 `ChatModelAgentMiddleware` 机制，但有成熟的 `Extension` 体系（被 evidence/planmode/filecheckpoint/skill/memory/knowledge 使用）。
Extension 的 `ToolProvider` 接口天然适合注入工具，且 `EventSnapshotProvider` 支持新挂载的 TUI 获取当前状态。

### 5.2 为什么用独立 Store 接口而非直接文件操作

eino 的 `Backend` 接口是文件系统语义（LsInfo/Read/Write/Delete），上层工具直接操作文件路径。
Mady 的 Store 是领域语义（Create/Get/Update/List），实现可替换为 SQLite 或远程存储，且路径沙箱化封装在 Store 内部，不暴露给工具。

### 5.3 为什么 archived 替代 deleted

专利法律场景要求审计留痕（`SECURITY.md` + `AGENTS.md` 反复强调）。
eino 的物理删除会丢失任务历史。Mady 用 `archived` 状态标记，List 默认不返回 archived 任务，但可通过参数查询。

### 5.4 为什么增加 priority 字段

专利撰写有天然优先级：独立权利要求 > 从属权利要求 > 实施例。
eino 无优先级，任务按 ID 排序。Mady 增加 `priority` 字段，List 按 `priority DESC, ID ASC` 排序。

### 5.5 循环依赖处理

`agentcore/tasklist` 包 import `agentcore`（用于 Tool/Extension/Event 类型）。
`agentcore/event_types.go` 中的 TaskCreatedEvent 需引用 `tasklist.Task`。
这会造成 `agentcore` → `agentcore/tasklist` → `agentcore` 循环。

**解决方案**：事件类型中 `Task` 字段用 `any` 类型或在内联包中定义 Task 结构体。
推荐方案：将 Task 结构体定义在 `agentcore` 包中（`agentcore/task_types.go`），tasklist 包引用它。
这样事件类型和 Task 都在 `agentcore` 包中，tasklist 包只负责 Store + 工具实现。

---

## 6. 文件清单

### 新增文件

| 文件 | 说明 |
|------|------|
| `agentcore/task_types.go` | Task 数据模型 + 状态/优先级常量（避免循环导入） |
| `agentcore/tasklist/store.go` | Store 接口 + MemoryStore |
| `agentcore/tasklist/filestore.go` | FileStore 实现（沙箱化持久化） |
| `agentcore/tasklist/extension.go` | Extension 实现 |
| `agentcore/tasklist/tool_create.go` | TaskCreate 工具 |
| `agentcore/tasklist/tool_get.go` | TaskGet 工具 |
| `agentcore/tasklist/tool_update.go` | TaskUpdate 工具 |
| `agentcore/tasklist/tool_list.go` | TaskList 工具 |
| `agentcore/tasklist/prompts.go` | 双语工具描述提示词 |
| `agentcore/tasklist/*_test.go` | 测试文件 |

### 修改文件

| 文件 | 改动范围 |
|------|---------|
| `agentcore/event_types.go` | 新增 2 个 EventType 常量 + 事件结构体 |
| `agentcore/planmode/policy.go` | `alwaysAllowed` 更新 |
| `cmd/mady/framework.go` | Extension 组装段追加 tasklist 注册 |
| `tui/chat/chat_app.go` | TodoPanel 实例化 + 事件订阅 |
| `tui/chat/state.go` | TodoPanel 状态字段 |
| `docs/decisions/AI_CHANGELOG.md` | 追加变更记录 |

### 不触碰的文件

- `agentcore/agent.go` / `agentcore/agent_run.go` — 核心运行循环不变
- `agentcore/lifecycle.go` — LifecycleHook 系统不变
- `agentcore/checkpoint.go` — 检查点机制不变
- `agentcore/stream.go` — 流式基础设施不变
- `tools/path.go` — 沙箱化机制不变（FileStore 内部调用）
- `tui/component/todo_panel.go` — 渲染逻辑基本不变（可能微调字段映射）

---

## 7. 风险与缓解

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 循环导入 | 中 | 编译失败 | Task 类型放 agentcore 包（5.5 方案） |
| TUI 事件订阅性能 | 低 | 高频刷新卡顿 | TodoPanel 已有 mutex + 批量渲染；task 事件频率低 |
| 并发写入竞争 | 低 | 数据损坏 | per-session sync.Mutex + FileStore 原子写入 |
| planmode 门控遗漏 | 低 | 计划模式下误写 | 单元测试覆盖 planmode 集成 |
| 提示词不符合 tone-style | 低 | 文案问题 | Phase 4 专做 i18n + tone 审查 |

---

## 8. 工作量与时间线

| 阶段 | 内容 | 估算 | 依赖 |
|------|------|------|------|
| Phase 1 | 核心包（tasklist/） | 3 天 | 无 |
| Phase 2 | 事件 + 装配 | 1 天 | Phase 1 |
| Phase 3 | TUI 接线 | 2 天 | Phase 2 |
| Phase 4 | i18n + 提示词 | 1 天 | Phase 1 |
| Phase 5 | 测试 + 文档 | 2 天 | 全部 |
| **总计** | | **~9 个工作日** | |

---

## 9. 验收标准

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test -race ./agentcore/tasklist/...` 通过
- [ ] `go test ./...` 全部通过（无回归）
- [ ] TUI 中 Agent 调用 TaskCreate 后 TodoPanel 实时显示
- [ ] planmode 下 TaskCreate/TaskUpdate 被阻止，TaskGet/TaskList 可用
- [ ] 任务依赖图循环检测生效（单元测试验证）
- [ ] 任务持久化跨会话可用（重启 mady 后 TaskList 能读回）
- [ ] `docs/decisions/AI_CHANGELOG.md` 已追加记录
