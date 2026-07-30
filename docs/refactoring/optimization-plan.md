# Mady 代码异味优化方案

> 基于 2026-07-30 全量静态分析生成的优化路线图。
> 涉及文件约 130 个，预期总工作量约 5-8 人日。

---

## 修订历史

| 版本 | 日期 | 变更说明 |
|------|------|---------|
| v1.0 | 2026-07-30 | 初版创建 |

---

## 总体路线图

```
 阶段一（基础设施加固）   阶段二（中频重构）       阶段三（深度重构）
 ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
 │ P1-A Linter 增强  │    │ P2-A 高复杂度拆分│    │ P3-A God 包拆分  │
 │ P1-B Worktree 清理│───→│ P2-B 重复代码消除│───→│ P3-B 大文件拆分  │
 │ 1-2 人日          │    │ P2-C 未用参数清理│    │ P3-C panic 审计  │
 └──────────────────┘    │ 2-3 人日          │    │ P3-D goroutine 安全│
                         └──────────────────┘    │ P3-E 架构违规修复  │
                                                 │ P3-F TODO 与文档  │
                                                 │ 2-3 人日          │
                                                 └──────────────────┘
```

- **阶段一**：可独立执行，无重构风险，立刻收益
- **阶段二**：中等风险，需配合测试验证，每项独立提交
- **阶段三**：高风险，需逐项评审，部分项需要预先编写契约测试

---

## 阶段一：基础设施加固（1-2 天）

### P1-A：Linter 配置增强

**目标**：将本次扫描发现的 3 个关键 linter 纳入持续门禁，防止复发。

#### 操作步骤

1. 编辑 `.golangci.yml`，在 `linters.enable` 列表追加：
   ```yaml
   linters:
     enable:
       - gocognit        # 认知复杂度（新增）
       - dupl            # 重复代码检测（新增）
       - unparam         # 未使用参数检测（新增）
   ```

2. 在 `linters.settings` 添加 gocognit 阈值：
   ```yaml
   linters:
     settings:
       gocognit:
         min-complexity: 30
       dupl:
         threshold: 100
   ```

3. 运行验证：
   ```bash
   make lint
   # 预期显示 87 gocognit + 15 dupl + 50 unparam = 152 个新 issue
   ```

4. 提交 `.golangci.yml` 变更，在 CI 和 pre-commit 中生效。

#### 验收清单

- [ ] `golangci-lint run ./...` 正确启用 gocognit/dupl/unparam
- [ ] `make verify` 中包含新的 linter
- [ ] CI pipeline 输出新 linter 的 issue 计数
- [ ] AI_CHANGELOG.md 记录决策

#### 风险评估

- ⚠️ 启用后 CI 会因 152 个现有 issue 而变红
- **解决方案**：阶段一先加入 linter 但用 `issues.max-issues-per-linter: 0` 设为仅警告不阻断；
  阶段二、三逐项修复后再改为阻断

---

### P1-B：Worktree 垃圾清理

**目标**：释放约 470MB 磁盘空间，消除 AI Agent 残留的 23 个工作树。

#### 清理对象

| 来源 | 路径 | 工作树数 | 大小 |
|------|------|---------|------|
| Claude Code | `.claude/worktrees/` | 6 | ~143 MB |
| Grok (历史) | `~/.grok/worktrees/projects-mady/` | 17 | ~327 MB |

#### 操作步骤

1. 清理 Claude Code worktrees（安全版——先反注册再删除）：
   ```bash
   # 先移除 git 注册
   cd /Users/xujian/projects/Mady
   for wt in .claude/worktrees/*/; do
     if [ -d "$wt" ]; then
       git worktree remove --force "$wt" 2>/dev/null || true
     fi
   done
   # 再清理残留目录
   rm -rf .claude/worktrees/
   ```

2. 清理 Grok worktrees：
   ```bash
   for wt in /Users/xujian/.grok/worktrees/projects-mady/*/; do
     if [ -d "$wt" ]; then
       git worktree remove --force "$wt" 2>/dev/null || true
       rm -rf "$wt"
     fi
   done
   ```

3. 运行 `git worktree prune` 清理引用。

#### 验收清单

- [ ] `git worktree list` 只显示主工作树和当前活动的 worktree
- [ ] `.claude/worktrees/` 已删除或为空（不含 active worktree 时）
- [ ] 磁盘释放约 470MB

#### 风险评估

- ⚠️ 注意：当前活动的 worktree 不应删除（有分支名称的非 detached HEAD）
- 安全措施：先 `git worktree remove --force`，不会删除有未提交变更的工作树

---

## 阶段二：中频重构（2-3 天）

### P2-A：高认知复杂度函数拆分（87 个函数）

**原则**：
- 按认知复杂度从高到低逐个修复
- 每次只动一个函数，提交一个 commit
- Top 10 函数（复杂度 > 40）必须拆分，其余可标记 `//nolint:gocognit` 暂缓

#### Top 10 优先列表

| 优先级 | 函数 | 文件 | 复杂度 | 拆分策略 |
|--------|------|------|--------|---------|
| P0 | `NewComputerUseTool` | `tools/desktop/computer_use.go:184` | **120** | 将 switch action 分支提取为独立 handler 函数 |
| P0 | `NewBashTool` | `tools/bash.go:251` | **79** | 将 onData 闭包提取为命名函数；将配置校验前置 |
| P1 | `NewViewTool` | `tools/view.go:61` | **75** | 同 NewBashTool 模式 |
| P1 | `handleNavigate` | `tools/browser_tool_navigate.go:19` | **70** | 将页面准备/导航/等待分解为步骤函数 |
| P1 | `NewReadTool` | `tools/read.go:64` | **62** | 同 NewBashTool 模式 |
| P2 | `cuaDriverCapture` | `tools/desktop/computer_use_cua_driver.go:32` | **62** | 将截图/OCR/SOM 三条路径提取为独立函数 |
| P2 | `waylandGetWindowBounds` | `tools/desktop/computer_use_lin.go:274` | **61** | 将 xdotool 命令构造和输出解析分离 |
| P2 | `renderSOMBody` | `tools/desktop/computer_use_som.go:64` | **40** | 将不同元素类型的渲染拆为函数 |
| P2 | `newEgoLiteHandoffTool` | `tools/ego_lite.go:65` | **48** | 将连接/读取/解析流程分离 |
| P2 | `NewPandocTool` | `tools/pandoc.go:51` | **46** | 同 NewBashTool 模式 |

#### 常见拆分模式（适用于所有 `New*Tool` 函数）

```
// 重构前（一个函数完成所有事）
func NewBashTool(cwd string, cfg *BashToolConfig) *agentcore.Tool {
    // 200 行：配置校验 + schema 构建 + 闭包 handler（含 50 行 onData 逻辑）
}

// 重构后
func NewBashTool(cwd string, cfg *BashToolConfig) *agentcore.Tool {
    cfg = resolveBashConfig(cfg)           // 配置默认值 + 校验
    handler := newBashHandler(cwd, cfg)    // 分离 handler 构造
    return buildTool("bash", desc, schema, handler)
}

func resolveBashConfig(cfg *BashToolConfig) *BashToolConfig { /* ... */ }

func newBashHandler(cwd string, cfg *BashToolConfig) agentcore.ToolFunc {
    return func(ctx context.Context, args json.RawMessage) (any, error) {
        input := parseBashInput(args)
        // ...
    }
}

// onData 闭包提取为独立类型
type bashOutputCollector struct {
    mu          sync.Mutex
    chunks      [][]byte
    totalBytes  int
    tempFile    *os.File
    tempPath    string
    maxBytes    int64
}
func (c *bashOutputCollector) Write(data []byte) { /* ... */ }
func (c *bashOutputCollector) Result() BashResult { /* ... */ }
```

#### 验收清单（每个函数）

- [ ] 拆分后原函数认知复杂度 ≤ 30
- [ ] 新提取的函数有有意义的命名和导出注释
- [ ] 所有现有测试通过（`go test ./tools/... && go test ./...`）
- [ ] 无新 lint warning
- [ ] AI_CHANGELOG.md 记录

---

### P2-B：重复代码消除（15 处）

#### 模式一：JSON-RPC WebSocket handler 模板（`a2a/ws.go`）

**重复位置**：`handleWSGetTask` / `handleWSCancelTask` / `handleWSQueryTasks` / `handleWSSetPushNotification`

**重构策略**：提取通用 handler builder

```go
// 重构后
type wsHandler func(ctx context.Context, wc *wsConn, req JSONRPCRequest) (any, error)

func (s *Server) wsHandlerTemplate(name string, fn wsHandler) func(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
    return func(ctx context.Context, wc *wsConn, req JSONRPCRequest) {
        result, err := fn(ctx, wc, req)
        if err != nil {
            // 统一错误处理
            if err := wc.writeError(req.ID, err); err != nil {
                _ = wc.close()
            }
            return
        }
        if err := wc.writeJSON(JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
            _ = wc.close()
        }
    }
}
```

#### 模式二：ACP Server handler 模板（`acp/server.go:823-881`）

**重复位置**：`handleSetMode` / `handleSetModel`

**重构策略**：提取通用三步骤模板（Unmarshal → Validate → Execute → Respond）

```go
type acpHandler func(req *JSONRPCRequest) error

func (s *Server) acpHandlerTemplate(unmarshalFn, validateFn, execFn func() error) func(req *JSONRPCRequest) {
    return func(req *JSONRPCRequest) {
        if err := unmarshalFn(); err != nil { /* writeError */; return }
        if err := validateFn(); err != nil { /* writeError */; return }
        if err := execFn(); err != nil { /* writeError */; return }
        s.writeResponse(req.ID, nil)
    }
}
```

#### 模式三：工具工厂函数公共模式

**重复位置**：所有 `New*Tool` 函数中的配置校验 + schema 构建 + 闭包 handler

**重构策略**：提取 `ToolBuilder` 辅助类型

```go
type ToolBuilder struct {
    name        string
    description string
    schema      map[string]any
    handler     func(ctx context.Context, args json.RawMessage) (any, error)
    readOnly    bool
}

func (b *ToolBuilder) Build() *agentcore.Tool { /* ... */ }
```

#### 验收清单

- [ ] a2a/ws.go 的 4 个 handler 统一为模板调用
- [ ] acp/server.go 的 handler 重复消除
- [ ] dupl linter 不再报告这些文件
- [ ] 所有测试通过

---

### P2-C：未使用参数清理（50 处）

#### 分类处理

| 类别 | 数量 | 处理方式 |
|------|------|---------|
| `ctx` 参数未使用 | ~15 | 改为 `_` 或删除（看是否满足接口签名） |
| `error` 返回始终为 nil | ~8 | 改为不返回 error |
| 其他参数未使用 | ~14 | 删除或改为 `_` |
| 返回值从未被使用 | ~5 | 删除返回值 |

#### 典型修复示例

```go
// 修复前
func (w *ReasoningWalker) traverseFrom(ctx context.Context, seed KgNode, maxDepth int) []ReasoningChainNode

// 修复后
func (w *ReasoningWalker) traverseFrom(_ context.Context, seed KgNode, maxDepth int) []ReasoningChainNode
// 或：删除 ctx 参数（如果是私有函数且调用方也无 context）
```

```go
// 修复前
func runComprehensiveEval(content string, citations []string, cfg *PatentEvalToolConfig) (PatentEvalResult, error)

// 修复后
func runComprehensiveEval(content string, citations []string, cfg *PatentEvalToolConfig) PatentEvalResult
// 注意：如果是接口实现则不能改变签名，只改接口定义
```

#### 验收清单

- [ ] unparam linter 报告减少至 0（或仅保留约定性豁免的极少几处）
- [ ] 不破坏任何接口签名（满足 Go 接口兼容性）
- [ ] 所有测试通过
- [ ] 按包逐个提交，每个包一个 commit

---

## 阶段三：深度重构（2-3 天）

### P3-A：God 包拆分（domains 47K 行）

#### 当前状态

`domains/` 包含 17 个子包，总量 245 个文件、47K 行代码，覆盖规则引擎、说明书撰写、权利要求撰写、创造性判断、充分公开、证据判断、推理引擎、工作流等多个完全独立的子域。

#### 拆分方案

**第一步：剥离独立的域模块（已验证的子域继续保持隔离）**

| 当前路径 | 建议 | 原因 |
|---------|------|------|
| `domains/infringement/` | ✅ 已为独立子包 | 继续维护 |
| `domains/claimdrafting/` | ✅ 已为独立子包 | 继续维护 |
| `domains/specdrafting/` | ✅ 已为独立子包 | 继续维护 |
| `domains/evidence/` | ✅ 已为独立子包 | 继续维护 |
| `domains/enablement/` | ✅ 已为独立子包 | 继续维护 |
| `domains/inventiveness/` | ✅ 已为独立子包 | 继续维护 |
| `domains/novelty/` | ⚠️ 需确认是否独立于 inventiveness | |

**第二步：解耦 domains 根目录的大文件**

`domains/` 根目录（非子包）的文件仍是大问题：

```bash
# 当前根目录下的大文件（非子包）
domains/unified.go          # UnifiedAgentConfig 融合三个 Agent 配置
domains/patent.go           # BuildProjectAgent + 动态 WorkingDir
domains/legal.go            # Legal Agent 配置
domains/router.go           # 路由白名单
```

**方案：将根目录文件按责任重组**

```
domains/
├── config/          # UnifiedAgentConfig + Agent 配置
├── patent/          # 专利 Agent 配置（从 patent.go 提取）
├── legal/           # 法律 Agent 配置（从 legal.go 提取）
├── router/          # 路由逻辑
├── style.go         # DocumentStyle（保持根目录或移入 config）
├── ...（现有子包保持）
```

#### 验收清单

- [ ] `domains/` 根目录文件数减少 50% 以上
- [ ] 原有功能零变更（回归测试覆盖）
- [ ] 包引用路径更新（`domains/patent.go` → `domains/patent/...`）
- [ ] `go-arch-lint` 配置同步更新
- [ ] `docs/chat-assistant-architecture.md` 同步更新

---

### P3-B：千行级文件拆分（4 个）

| 文件 | 行数 | 函数数 | 拆分方案 |
|------|------|--------|---------|
| `desktop/app.go` | 1,336 | 44 | 按生命周期/事件/状态拆为 3-4 个文件 |
| `tui/chat/chat_app.go` | 1,283 | 77 | 按 Elm 架构拆为 model/update/view |
| `cmd/mady/tui_session.go` | 1,060 | 40 | 按 session lifecycle/approval/agent 拆分 |
| `acp/server.go` | 1,013 | 38 | 按 handler 类型拆分 |'

#### `desktop/app.go` 拆分模板

```
desktop/
├── app.go                # 核心结构体 + NewApp（~300 行）
├── app_lifecycle.go      # Run/Shutdown/Autostart
├── app_events.go         # 事件处理（onFileOpen/onNotification）
├── app_settings.go       # 设置管理（SetAISettings/SyncSettings）
├── app_state.go          # 状态管理
```

#### `tui/chat/chat_app.go` 拆分模板

```
tui/chat/
├── chat_app.go           # NewChatApp + 核心导出 API（~200 行）
├── chat_model.go         # Model 类型定义 + 初始化
├── chat_update.go        # Update 方法（Elm update 逻辑）
├── chat_view.go          # View 方法（Elm view 逻辑）
├── chat_subscriptions.go # 订阅处理
```

#### 验收清单

- [ ] 拆分后每文件 ≤ 500 行
- [ ] `go build ./... && go test ./...` 通过
- [ ] 包外部 API 零变化（导出符号不受影响）

---

### P3-C：生产代码 panic 审计（11 处）

#### 分类处理

| 类别 | 示例 | 处理方式 |
|------|------|---------|
| 前置条件 panic（合理） | `csync.go:32-36`、`scorer.go:14`、`pool.go:68` | 保留，加 `//nolint:gocognit` 豁免 |
| 业务逻辑 panic（危险） | `knowledge/standards/ipc-standards.go:68` | 改为 `return error`，调用方处理 |
| 重复启动 panic | `event_logger.go:62` | 改为 `return error` 或 `sync.Once` |
| 并发竞争 panic | `fact_blackboard.go:55` | `panic → return error`，调用链路由 |

#### 高风险项详细修复

**`knowledge/standards/ipc-standards.go:68`**

```go
// 修复前
if err != nil {
    panic(err)
}

// 修复后
if err != nil {
    return nil, fmt.Errorf("load IPC standards: %w", err)
}
```

> ⚠️ 此文件加载国际专利分类表时若出错直接 panic 会杀死整个进程。调用链上的函数签名需要从 `func() X` 改为 `func() (X, error)`。

**`agentcore/event_logger.go:62`**

```go
// 修复前
func (l *EventLogger) Start() {
    if l.started {
        panic("EventLogger already started")
    }
    // ...
}

// 修复后（推荐）
func (l *EventLogger) Start() error {
    if l.started {
        return fmt.Errorf("event logger already started")
    }
    // ...
    return nil
}
// 同时使用 sync.Once 确保 Start 只会执行一次
```

#### 验收清单

- [ ] 无 `panic(err)` 出现在业务逻辑路径中
- [ ] 错误返回的签名变更已更新所有调用方
- [ ] `go test -race ./...` 通过
- [ ] changeset 记录每个 panic 的处置理由

---

### P3-D：Goroutine 安全管理（15 处）

#### 修复模式

对所有 `go func()` 调用，添加三重防护：

```go
// 修复前
go func() {
    // doWork()
}()

// 修复后
go func() {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("goroutine panicked", "recover", r, "stack", string(debug.Stack()))
        }
    }()
    // doWork()
}()
```

#### 优先处理列表（按风险）

| 优先级 | 位置 | 风险 |
|--------|------|------|
| P0 | `agentcore/executor.go:363` | 工具调用并发执行，panic 会导致 Agent 死亡 |
| P0 | `agentcore/pubsub.go:115` | 事件发布，panic 会垮整个监听链 |
| P0 | `graph/pregel.go:291` | Pregel 图节点执行，panic 导致整图失败 |
| P1 | `graph/graph.go:302` | DAG 节点并发 |
| P1 | `acp/server.go:773` | 连接管理 |
| P2 | 其余 10 处 | 辅助功能 |

#### 验收清单

- [ ] 所有 `go func()` 有 deferred recovery
- [ ] panic recovery 至少记录 `slog.Error` 和堆栈
- [ ] 关键路径（executor/pubsub/pregel）有降级/重试机制

---

### P3-E：架构边界违规修复

#### 问题

`domains/unified.go`、`domains/legal.go`、`domains/patent.go` 直接导入 `tools` 包，与 `AGENTS.md` 的"Domain 层不得 import Infrastructure 层的具体实现，只能依赖接口"原则相悖。

#### 修复方案

1. 在 `agentcore` 或新的 `domaininterface` 包中定义工具接口：
   ```go
   // agentcore/tooliface.go（新增）
   type ToolProvider interface {
       ListTools() []*Tool
       GetTool(name string) (*Tool, bool)
   }
   ```

2. Domain 层只依赖 `agentcore.ToolProvider` 接口
3. `tools` 包实现该接口
4. 依赖注入通过 `bootstrap/setup.go` 在初始化时完成

#### 验收清单

- [ ] `domains/` 不再直接 import `tools/`
- [ ] `go-arch-lint deepScan: true` 模式通过
- [ ] 功能零变更（回归测试覆盖）

---

### P3-F：TODO Stub 处理与文档同步

#### TODO 处理

`workflows/workflow.go` 的 4 个 TODO：

```go
// TODO: 当 WorkflowOrchestrator 持有 Provider 引用时，创建 agentcore.Agent 并运行。
// TODO: 通过 ToolRegistry 按 step.Tool 查找并调用工具。
// TODO: 当 EvaluateProvider 可用时，调用质量检查器评估当前产物。
// TODO: 按 step.SubWorkflowName 查找已注册的工作流模板并递归执行。
```

**决策方案**：

| 选项 | 说明 | 推荐 |
|------|------|------|
| A: 实现 | 完成 TODO 功能 | 如果当前有业务需求 |
| B: 标记为 `//nolint` 并记录 | 加注释说明暂不实现，记入技术债务跟踪 | ✅ 如果无立即需求 |
| C: 删除骨架代码 | 删除未实现的导出方法，保持 API 干净 | 不推荐（可能已外部引用） |

**推荐方案 B**：

```go
// Execute 执行工作流编排。
// 注意：当前 StepTypeAgent/StepTypeTool/StepTypeEvaluate/StepTypeSubWorkflow
// 四种模式尚未实现（参见 AI_CHANGELOG.md %date%），调用时仅返回 nil。
// 计划在 WorkflowOrchestrator 接入 Provider 后实现。
func (o *WorkflowOrchestrator) Execute(ctx context.Context, wf Workflow) error {
    // ...
    return nil
}
```

#### 文档同步

- [ ] `docs/chat-assistant-architecture.md` 更新 domains→tools 边界描述
- [ ] `docs/decisions/AI_CHANGELOG.md` 追加本计划中所有完成项
- [ ] `.go-arch-lint.yml` 新增 `ignoreNotFoundComponents: true` 的原因说明（当前缺失）

---

## 执行次序建议

```
Week 1                 Week 2                  Week 3
┌────┬────┬────┬────┐ ┌────┬────┬────┬────┐ ┌────┬────┬────┬────┐
│P1-A│P1-B│P1-B│    │ │P2-A│P2-A│P2-B│P2-C│ │P3-A│P3-B│P3-C│P3-D│
│    │    │    │    │ │Top │Next│    │    │ │    │    │    │P3-E│
│    │    │    │    │ │10  │~20 │    │    │ │    │    │    │P3-F│
└────┴────┴────┴────┘ └────┴────┴────┴────┘ └────┴────┴────┴────┘
   Linter   Worktree    复杂函数   重复    参数     God包   大文件   panic
   增强     清理         拆分      消除    清理     拆分    拆分    goroutine
```

### 快速胜利（立刻可执行，一个人日内）

1. **P1-A**：改 `.golangci.yml` 配置文件（5 分钟）
2. **P1-B**：运行清理脚本（5 分钟，释放 470MB）
3. **P2-C 中的简单项**：删除那些确定 unused 的参数（30 分钟，约 20 处）

### 收益/成本比矩阵

| 任务 | 预估工时 | 收益 | 成本效益 |
|------|---------|------|---------|
| P1-A Linter 增强 | 0.1 人日 | ⭐⭐⭐⭐⭐ | 防复发、曝露所有问题 |
| P1-B Worktree 清理 | 0.1 人日 | ⭐⭐⭐ | 释放 470MB |
| P2-C 简单参数清理 | 0.3 人日 | ⭐⭐⭐ | 减少 20 个 lint warning |
| P2-A Top 10 函数拆分 | 1.0 人日 | ⭐⭐⭐⭐⭐ | 核心可维护性提升 |
| P2-B 重复代码消除 | 0.5 人日 | ⭐⭐⭐⭐ | 减少模板代码维护成本 |
| P3-C panic 审计（2 处高危）| 0.3 人日 | ⭐⭐⭐⭐⭐ | 消除进程崩溃风险 |
| P3-D goroutine 安全 | 0.5 人日 | ⭐⭐⭐⭐ | 消除静默崩溃 |
| P3-E 架构违规 | 1.0 人日 | ⭐⭐⭐⭐ | 恢复分层约束 |
| P3-A God 包拆分 | 1.5 人日 | ⭐⭐⭐⭐ | 提升可理解性 |
| P3-B 大文件拆分 | 1.0 人日 | ⭐⭐⭐ | 提升可读性 |

---

## 验证脚本

每次重构完成一个任务后，运行以下命令确认无回归：

```bash
# 标准验证
make verify

# 架构检查
go-arch-lint check

# 代码风格
gofmt -d -s .

# 完整测试（含竞态 + 集成）
make test-race
make test-integration
```

## 附录：完整文件索引

### 高认知复杂度函数（87 个）
完整列表可通过以下命令实时获取：
```bash
golangci-lint run --enable=gocognit ./... 2>&1 | grep "cognitive complexity"
cd tools && golangci-lint run --enable=gocognit ./... 2>&1 | grep "cognitive complexity"
```

### 重复代码检测（15 处）
```bash
golangci-lint run --enable=dupl ./... 2>&1
```

### 未使用参数（50 处）
```bash
golangci-lint run --enable=unparam ./... 2>&1
```
