# Mady Go 规范开发文档

> 结合 Go 业界最佳实践与 Mady 项目实际代码风格，制定的规范开发指南。
>
> 参考来源：[Effective Go](https://go.dev/doc/effective_go)、[Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)、[Kubernetes Coding Conventions](https://github.com/kubernetes/community/blob/master/contributors/guide/coding-conventions.md)、[Standard Go Project Layout](https://github.com/golang-standards/project-layout)、以及 Mady 仓库实际代码中沉淀的模式。

---

## 目录

1. [项目结构与模块组织](#1-项目结构与模块组织)
2. [包设计与命名](#2-包设计与命名)
3. [代码风格与格式化](#3-代码风格与格式化)
4. [错误处理](#4-错误处理)
5. [接口设计](#5-接口设计)
6. [并发编程](#6-并发编程)
7. [测试规范](#7-测试规范)
8. [配置管理](#8-配置管理)
9. [依赖管理](#9-依赖管理)
10. [文档与注释](#10-文档与注释)
11. [构建与CI](#11-构建与ci)
12. [安全规范](#12-安全规范)
13. [附录：Mady 项目已有模式速查](#13-附录mady-项目已有模式速查)

---

## 1. 项目结构与模块组织

### 1.1 多模块布局

Mady 使用 Go 1.25 多模块（`go.work`），根模块 + `./tools` 子模块：

```go
// go.work
go 1.25.0
use (
    .
    ./tools
)
```

> **何时拆分子模块**：当一组包与主项目生命周期不同、或需要独立版本号、或被外部复用。工具层（`tools/`）作为独立子模块，可以从外部导入。

### 1.2 目录约定

```
mady/
├── cmd/                    # 应用入口（每个 main 包一个子目录）
│   └── mady/main.go        # 统一的入口点，mady tui | mady serve | mady acp
├── agentcore/              # 核心引擎包（包名与目录名一致）
├── domains/                # 领域包
├── tools/                  # 独立子模块（可用 go get 外部导入）
├── pkg/                    # 可被外部导入的公共库
│   ├── agentconfig/
│   └── util/
├── internal/               # （需要时使用）禁止外部导入的包
├── integration/            # 端到端集成测试
├── benchmark/              # 性能基准测试
├── docs/                   # 文档
├── example/                # 示例应用
├── scripts/                # 构建/辅助脚本
└── skill/ skills/          # 技能加载器 + 内置技能定义
```

**核心原则**：
- 每个目录一个包，包名 = 目录名。
- `cmd/` 下只放 `package main`，`main()` 函数极简短——调用 `realMain` 或直接调用库代码。Mady 实践：`cmd/mady/main.go` 统一入口，通过子命令模式支持 `tui`、`serve`、`acp`。
- 不创建 `common/`、`utils/`、`base/` 等通用包。参见 [1.4 反模式](#14-反模式)。

### 1.3 分层架构

Mady 8 层架构，严格单向依赖：

```
外部接口层：A2A | A2UI | Server | AGUI | MCP | ACP
                    ↓
               核心引擎层：agentcore
             ↙     ↓       ↘        ↘
        提供者层  工具层(35)  扩展层    领域扩展层
             ↘     ↓       ↙        ↙
             基础设施层：graph/ session/ skill/ prompt/ store/
                      mcp/ knowledge/ retrieval/ disclosure/ memory/ ...
                    ↓
                TUI 层（8 层 Elm 架构）
                    ↓
                应用入口（cmd/mady, server/, example/）
```

- **上层可导入下层，反向禁止。**
- 模块间通信通过接口而非具体类型。

### 1.4 反模式

| 反模式 | 正确做法 |
|--------|---------|
| 创建 `common/`、`utils/` 包 | 按领域拆分为 `fuzzy/`、`filequeue/` 等有明确语义的包 |
| `import . "package"`（dot import） | 禁止使用（测试中也不要默认使用） |
| 在 `init()` 中 panic | 记录日志到 stderr 而不是 panic（见 `agentcore/evaluate/benchmark/invalidation_decisions.go` 修复） |
| 使用 `_` 忽略错误 | 始终检查所有错误返回值 |
| 全局状态（`http.DefaultServeMux` 等） | 创建专用实例注入 |

---

## 2. 包设计与命名

### 2.1 包命名

```go
// ✅ 正确：简短、小写、单数
package fuzzy
package filequeue
package agentcore

// ✅ 正确：缩写全部小写
package http   // 不是 Http, HTTP
package mcp    // 不是 Mcp, MCP

// ❌ 错误：下划线或大小写混用
package util_functions  // 下划线
package utilFunctions   // 驼峰
```

### 2.2 标识符命名

| 类型 | 约定 | Mady 示例 |
|------|------|-----------|
| 导出类型 | `PascalCase` | `EventBus`, `AgentState`, `NodeError` |
| 未导出类型 | `camelCase` | `baseEvent`, `messageQueue` |
| 导出字段/方法 | `PascalCase` | `Config()`, `AgentName` |
| 未导出字段/方法 | `camelCase` | `configMu`, `runLoop()` |
| 常量 | 导出 `PascalCase` / 未导出 `camelCase` | `EventAgentStart`, `defaultMaxTurns` |
| 接口 | `-er` 后缀（有意义时） | `Tracer`, `Store`, `EventHandler` |
| 接收者 | 1-2 字母缩写 | `a *Agent`, `eb *EventBus`, `r *RateLimitHook` |

### 2.3 导入分组

强制三组顺序，组间空行分隔：

```go
import (
    "context"
    "errors"
    "fmt"

    "github.com/gorilla/websocket"
    "gopkg.in/yaml.v3"

    "github.com/xujian519/mady/pkg/util"
    "github.com/xujian519/mady/skill"
)
```

使用 `goimports`（或 `gofmt` + 手动排序）确保格式一致。

### 2.4 文件组织

Mady 每个包遵循的典型文件结构：

```
agentcore/
├── agent.go              # 核心类型定义（Agent、Config struct）
├── agent_run.go          # Run/Continue/Resume 实现
├── agent_test.go         # 测试文件（_test.go 与源文件同包，使用 package agentcore）
├── errors.go             # 错误类型定义
├── event.go              # 事件类型 + EventBus 实现
├── lifecycle.go          # LifecycleHook 接口 + 内置实现
├── config.go             # 配置结构 + Validate()
├── extension.go          # 扩展机制
├── pubsub.go             # 发布订阅
├── retry.go              # 重试逻辑
├── tool.go               # 工具定义
└── ...                   # 按职责拆分
```

规则：
- 一个文件一个主要职责：不把所有类型放在一个 `types.go` 中。
- 测试文件与源文件同包（white-box testing），`_test.go` 后缀。
- `doc.go` 用于包级文档（可选的）。

---

## 3. 代码风格与格式化

### 3.1 自动格式化

```bash
# 必须运行
go fmt ./...
go vet ./...
golangci-lint run ./...
```

项目已配置 `.golangci.yml` 和 `.pre-commit-config.yaml`，提交前自动检查。

### 3.2 尽早返回（Happy Path 左对齐）

```go
// ✅ 好：错误处理提前返回，主线逻辑清晰
func (s *ptcServer) handle(ctx context.Context, conn net.Conn) {
    defer conn.Close()
    conn.SetDeadline(time.Now().Add(30 * time.Second))

    data, err := io.ReadAll(conn)
    if err != nil || len(data) == 0 {
        return
    }
    var req ptcRequest
    if err := json.Unmarshal(data, &req); err != nil {
        s.reply(conn, ptcResponse{Error: "invalid request"})
        return
    }
    if req.Token != s.token {
        s.reply(conn, ptcResponse{ID: req.ID, Error: "unauthorized"})
        return
    }
    // 主线逻辑...
}

// ❌ 不好：深层嵌套
func (s *ptcServer) handle(ctx context.Context, conn net.Conn) {
    // ...大量 if-else 嵌套
}
```

### 3.3 Switch 与 Type Switch 的惯用写法

```go
// 错误类型判断
switch err := err.(type) {
case *RetryableError:
    // 重试
case *FatalError:
    // 降级
default:
    // 通用处理
}

// 使用 switch 替代 if-else 链
func errorType(err error) string {
    if err == nil { return "" }
    switch err {
    case context.Canceled:
        return "context.Canceled"
    case context.DeadlineExceeded:
        return "context.DeadlineExceeded"
    }
    return ""
}
```

### 3.4 零值有用

```go
// ✅ 好：零值即默认配置
type Config struct {
    MaxTurns  int64     // 零值 0，在 New() 中替换为 defaultMaxTurns
    Store     Store     // nil 表示不使用
    Tracer    Tracer    // nil 兜底为 noopTracer
}

func New(cfg Config) *Agent {
    if cfg.MaxTurns <= 0 {
        cfg.MaxTurns = defaultMaxTurns
    }
    // ...
}
```

### 3.5 字符串与格式化

- 错误信息小写开头（除非专有名词），不加标点——因为 `fmt.Errorf("...: %w", err)` 会拼接。
- 用户可见的消息（中文）使用 `fmt.Sprintf` 拼接，不要硬编码语言标识符。
- 构建字符串：小量用 `+` / `fmt.Sprintf`，大量用 `strings.Builder`。

---

## 4. 错误处理

### 4.1 错误类型体系

Mady 使用**分层错误类型**，每个类型有明确的语义和检查函数：

```go
// 可重试错误（超时、网络抖动）
type RetryableError struct {
    Op      string
    Details string
    Err     error
}
func (e *RetryableError) Error() string { ... }
func (e *RetryableError) Unwrap() error  { return e.Err }

// 致命错误（配置错误、Provider 不可用）
type FatalError struct { ... }
func (e *FatalError) Error() string { ... }
func (e *FatalError) Unwrap() error  { return e.Err }

// Handoff 错误
type HandoffError struct { ... }

// 护栏拦截错误
type GuardrailError struct { ... }

// 带执行路径的结构化错误
type NodeError struct {
    Path    []string // 执行路径 ["coordinator", "turn:3", "tool:get_weather"]
    Message string
    Err     error
}
```

配套检查函数：

```go
func IsRetryable(err error) bool
func IsFatal(err error) bool
func IsHandoffError(err error) bool
func IsGuardrailError(err error) bool
```

### 4.2 错误包装与传播

```go
// ✅ 包装：使用 fmt.Errorf + %w
if err != nil {
    return fmt.Errorf("读取文件失败: %w", err)
}

// ✅ 结构化的 NodeError 包装（Mady 特有模式）
return NewNodeError("provider call failed", err, agentName, "turn:3", "provider")

// ✅ 追加执行路径
func WrapNodeError(err error, pathSegment string) error
```

### 4.3 错误分类与处理策略

| 错误类型 | 处理策略 | Mady 代码示例 |
|---------|---------|-------------|
| `RetryableError` | 指数退避重试 | `agentcore/retry.go` |
| `FatalError` | 立即返回，停止执行 | `agentcore/errors.go` |
| `GuardrailError` | 记录并反馈给 LLM（不中断循环） | `agentcore/lifecycle.go:GuardrailHook` |
| `HandoffError` | 返回 FallbackMsg | `agentcore/handoff.go` |
| `context.Canceled` | 安静退出 | `agent_run.go:224` |
| `NodeError` | 携带完整执行路径用于调试 | 全程使用 |

### 4.4 Sentinel 与类型断言

```go
// sentinel 错误定义（导出）
var ErrExceedMaxSteps = fmt.Errorf("超出最大执行步数")

// 类型断言判断
func IsRetryable(err error) bool {
    if err == nil { return false }
    _, ok := err.(*RetryableError)
    return ok
}

// 使用 errors.Is 判断 sentinel
if errors.Is(err, context.Canceled) {
    // 安静退出
}
```

---

## 5. 接口设计

### 5.1 小接口原则

Mady 的接口通常 1-3 个方法：

```go
// 最小接口示例
type Store interface {
    SaveState(ctx context.Context, id string, data []byte) error
    LoadState(ctx context.Context, id string) ([]byte, error)
}

type Tracer interface {
    Start(ctx context.Context, name string, attrs ...Attr) (context.Context, Span)
}

type ContextBuilder interface {
    Build(ctx context.Context, input BuildInput) BuildOutput
}
```

### 5.2 接口在消费端定义

```go
// 在 consumer 包中定义接口，而不是 producer 包
// producer 包只返回具体类型

// --- retrieval/domain/provider.go ---
type Retriever interface {
    Retrieve(ctx context.Context, query string) ([]Result, error)
}

// --- knowledge/graph/ ---
// GraphStore 不定义 Retriever 接口，而是实现它
```

### 5.3 接受接口，返回具体类型

```go
// ✅ 好
func NewExecutor(reg *Registry, cfg ExecutorConfig) *Executor { ... }

// ❌ 避免：返回接口
func NewExecutor(reg RegistryInterface, cfg ExecutorConfig) ExecutorInterface { ... }
```

### 5.4 嵌入与零值接口

```go
// BaseLifecycleHook 提供零值默认实现
type BaseLifecycleHook struct{}

func (BaseLifecycleHook) BeforeAgentRun(_ context.Context, _ *AgentRunContext) error { return nil }
func (BaseLifecycleHook) AfterAgentRun(_ context.Context, _ *AgentRunContext, _ string, _ error) {}
// ...

// 实现者只需覆盖关心的钩子
type GuardrailHook struct {
    BaseLifecycleHook
    Validate func(ctx context.Context, response *ProviderResponse) error
}

func (g *GuardrailHook) AfterModelCall(ctx context.Context, arc *AgentRunContext, mcc *ModelCallContext) {
    // ...
}
```

### 5.5 LifecycleChain 模式（Mady 特色）

多个 LifecycleHook 通过 `LifecycleChain` 组合为链式调用：

```go
type LifecycleChain []LifecycleHook

// Before 系列：顺序调用，首个 error 终止
func (lc LifecycleChain) BeforeAgentRun(ctx context.Context, arc *AgentRunContext) error {
    for _, h := range lc {
        if err := h.BeforeAgentRun(ctx, arc); err != nil {
            return err
        }
    }
    return nil
}

// After 系列：逆序调用（类似 defer 栈）
func (lc LifecycleChain) AfterAgentRun(ctx context.Context, arc *AgentRunContext, output, err error) {
    for i := len(lc) - 1; i >= 0; i-- {
        lc[i].AfterAgentRun(ctx, arc, output, err)
    }
}
```

---

## 6. 并发编程

### 6.1 goroutine 生命周期管理

**规则：启动 goroutine 时必须知道它如何退出。**

```go
// ✅ 好：通过 context 管理生命周期
func (eb *EventBus) dispatch(ready chan<- struct{}) {
    defer close(eb.done)

    ctx := context.Background()
    ch := eb.broker.Subscribe(ctx)
    close(ready)

    for e := range ch {
        // 处理事件
    }
}
```

### 6.2 Mutex 使用

```go
// ✅ 好：命名锁 + RWMutex 优化读性能
type EventBus struct {
    mu       sync.RWMutex
    handlers map[EventType]map[uint64]EventHandler
    // ...
}

func (eb *EventBus) On(t EventType, h EventHandler) func() {
    eb.mu.Lock()
    defer eb.mu.Unlock()
    // ...
}

func (eb *EventBus) DropCount() uint64 {
    eb.mu.RLock()
    defer eb.mu.RUnlock()
    return eb.broker.DropCount()
}
```

### 6.3 atomic 用于快速状态切换

```go
// ✅ 好：atomic.Pointer 用于可替换配置
type Agent struct {
    interrupted atomic.Pointer[InterruptReason]
    // ...
}

// ✅ 好：atomic.Uint64 用于单调递增 ID
type EventBus struct {
    nextID atomic.Uint64
}
```

### 6.4 通道与 Broker 模式

Mady 使用泛型 `Broker[T]` 实现事件总线，注意：

- **Emit（非阻塞、善意的丢失）**：高频流式 delta
- **EmitMustDeliver（有界阻塞）**：终端事件（完成、错误）

```go
func (eb *EventBus) Emit(e Event) {
    // 非阻塞 Publish，缓冲区满时丢弃
    eb.broker.Publish(e)
}

func (eb *EventBus) EmitMustDeliver(ctx context.Context, e Event) {
    // 有界阻塞 Publish，每个订阅者 50ms 超时
    eb.broker.PublishMustDeliver(ctx, e)
}
```

### 6.5 panic 恢复

在执行 goroutine 中必须防止未捕获的 panic 导致整个进程退出：

```go
// ✅ 好：dispatch goroutine 中的 safeCall
func (eb *EventBus) safeCall(h EventHandler, e Event) {
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr, "agentcore: event handler panicked (event=%s): %v\n%s\n",
                e.EventKind(), r, debugStack())
        }
    }()
    h(e)
}

// ✅ 好：Pregel 节点 goroutine 的 panic recovery
// 参见 graph/pregol.go
```

### 6.6 并发池

```go
// agentcore/concurrency/pool.go
// 泛型 Worker Pool，控制最大并发数
type Pool[T any] struct {
    sem chan struct{}
}

func (p *Pool[T]) Go(fn func() T) <-chan T {
    result := make(chan T, 1)
    p.sem <- struct{}{} // 获取信号量
    go func() {
        defer func() { <-p.sem }()
        result <- fn()
    }()
    return result
}
```

---

## 7. 测试规范

### 7.1 测试文件组织

- 测试文件与源文件同包（`package agentcore`），使用 `_test.go` 后缀。
- 集成测试使用单独的 `package agentcore_test`（external test package）。
- 测试辅助函数放 `testhelpers_test.go`（不对外导出）。
- 服务/提供者的 stub 实现直接写在测试文件顶部。

### 7.2 表格驱动测试

```go
func TestAgentRun_GuardrailReject(t *testing.T) {
    tests := []struct {
        name       string
        maxTurns   int64
        rejectOn   int // which call to reject
        wantMsgs   int
        wantErr    bool
    }{
        {name: "first call rejected, second succeeds", maxTurns: 3, rejectOn: 1, wantMsgs: 3, wantErr: false},
        {name: "max turns exceeded", maxTurns: 0, rejectOn: -1, wantMsgs: 0, wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ... setup + assert
        })
    }
}
```

### 7.3 Provider Stub 模式（Mady 常用）

```go
type stubProvider struct {
    responses []string
    callCount int
}

func (p *stubProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
    p.callCount++
    if p.callCount > len(p.responses) {
        return &ProviderResponse{Content: "final answer"}, nil
    }
    return &ProviderProviderResponse{
        Content: p.responses[p.callCount-1],
    }, nil
}
```

### 7.4 用辅助构造函数减少重复

```go
func newTestAgent(t *testing.T, opts ...func(*Config)) *Agent {
    t.Helper()
    cfg := Config{
        ModelConfig: ModelConfig{
            Model:    "test-model",
            Provider: &stubProvider{},
        },
        ExecutionConfig: ExecutionConfig{
            MaxTurns: 5,
        },
    }
    for _, opt := range opts {
        opt(&cfg)
    }
    return New(cfg)
}
```

### 7.5 竞态检测（必须）

```bash
go test -race ./...
```

每次提交前必须运行。CI 中 `ci.yml` 已配置。

### 7.6 断言风格

当前代码未使用 testify 等第三方断言库——保持一致，使用标准库 `testing` 的 `t.Fatalf`/`t.Errorf`：

```go
if err != nil {
    t.Fatalf("unexpected error: %v", err)
}
if len(msgs) < 3 {
    t.Fatalf("expected at least 3 messages, got %d", len(msgs))
}
if msgs[1].Role != RoleAssistant {
    t.Fatalf("msgs[1] role = %q, want %q", msgs[1].Role, RoleAssistant)
}
```

### 7.7 避免 time.Sleep

在异步测试中，使用 Channel 等待或 `assert.Eventually` 风格而非 `time.Sleep`，防止脆弱测试。

---

## 8. 配置管理

### 8.1 嵌入式子配置

Mady 的 `Config` 使用嵌入式子配置分组：

```go
type Config struct {
    ModelConfig
    SkillConfig
    ExecutionConfig
    CompactionConfig

    Tools        []*Tool
    SystemPrompt string
    // ...
}
```

这样字段被提升到顶层，可以 `cfg.Model` 或 `cfg.ModelConfig.Model` 两种方式访问。

### 8.2 函数式选项 vs 结构体字面量

两种方式都支持，Mady 偏向 struct 构造 + `New()` 中设置默认值：

```go
// ✅ 方式一：结构体字面量（推荐）
agent := agentcore.New(agentcore.Config{
    ModelConfig: agentcore.ModelConfig{
        Model:    "gpt-4",
        Provider: provider,
    },
    ExecutionConfig: agentcore.ExecutionConfig{
        MaxTurns: 10,
    },
})

// ✅ 方式二：功能选项（替代方案）
agent := agentcore.NewConfig().
    WithModel("gpt-4").
    WithProvider(provider).
    WithMaxTurns(10).
    Build()
```

### 8.3 Validate 模式

配置结构体提供 `Validate()` 方法，在 `New()` 中提前调用，错误存储而非 panic：

```go
func (c Config) Validate() error {
    // ... validation logic
}

func New(cfg Config) *Agent {
    configErr := cfg.Validate()
    if configErr != nil {
        log.Printf("[agentcore] WARNING: config validation failed: %v", configErr)
    }
    a := &Agent{configErr: configErr, ...}
    // ...
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
    if a.configErr != nil {
        return "", fmt.Errorf("agentcore: agent configuration is invalid: %w", a.configErr)
    }
    // ...
}
```

### 8.4 运行时配置更新

对于需要在运行时更新的字段，使用 `sync.RWMutex` 保护：

```go
func (a *Agent) ApplyCallConfig(cc *CallConfig) {
    a.configMu.Lock()
    defer a.configMu.Unlock()
    if cc.Model != "" {
        a.config.Model = cc.Model
    }
    // ...
}

func (a *Agent) Config() Config {
    a.configMu.RLock()
    defer a.configMu.RUnlock()
    return a.config
}
```

---

## 9. 依赖管理

### 9.1 Go Modules

- 最小依赖原则。Mady 核心依赖仅 4 个（`gorilla/websocket`、`modernc.org/sqlite`、`gopkg.in/yaml.v3`、OpenTelemetry）。
- 间接依赖通过 go.mod 自动管理，不要手动添加。

### 9.2 版本管理

```bash
# 添加依赖
go get github.com/gorilla/websocket@v1.5.3

# 更新全部
go get -u ./...

# 整理依赖
go mod tidy
```

### 9.3 避免不必要的依赖

代码审查时必须检查：
- 这个功能能否用标准库实现？（如 `log/slog` 取代第三方日志库）
- 是否需要引入完整的框架？（Mady 不使用 gin/echo 等路由框架，使用标准库 `net/http`）
- 依赖是否有安全漏洞？

---

## 10. 文档与注释

### 10.1 导出符号文档

```go
// EventBus provides async pub/sub for agent lifecycle events.
// Events are dispatched via an internal Broker for fan-out delivery.
// Event ordering is preserved — a single goroutine processes events sequentially.
//
// Delivery semantics:
//   - Emit: best-effort, non-blocking...
//   - EmitMustDeliver: bounded-blocking...
type EventBus struct { ... }
```

格式规则：
- 每个导出符号必须注释。
- 首句以符号名开头：`// EventBus provides...`
- 句尾句号结束。

### 10.2 关键逻辑注释

```go
// safeCall invokes a handler with panic recovery. A panicking handler is
// logged to stderr but does NOT kill the dispatch goroutine — without this
// guard, a single buggy handler would permanently take down the event bus
// (dispatch exits, close(done) fires, all subsequent events are silently
// dropped once the channel fills). Other handlers and subsequent events
// must continue to flow.
func (eb *EventBus) safeCall(h EventHandler, e Event) {
    // ...
}
```

注释说明**为什么**这样实现，而非**什么**（代码本身应表达"什么"）。

### 10.3 包级文档

```go
// agentcore 提供核心 Agent 运行时，包括 LLM-工具循环、事件系统、
// 生命周期钩子、上下文引擎、以及 Handoff 委托机制。
//
// 架构概览：
//   Agent 是核心运行时。它管理一个 LLM 提供者、一组工具、一个事件总线、
//   以及可选的扩展和生命周期钩子。
//
// 使用示例：
//   agent := agentcore.New(cfg)
//   result, err := agent.Run(ctx, "hello")
package agentcore
```

### 10.4 注释中的中文

根据项目约定：**文档和注释使用中文，代码和标识符使用英文**。

```go
// 上下文引擎：根据配置管理消息窗口和压缩策略。
// 当 ContextWindow > 0 时自动启用。
type ContextEngine interface { ... }
```

---

## 11. 构建与 CI

### 11.1 标准构建命令

```bash
# 构建所有包
go build ./...

# 运行所有测试
go test ./...

# 竞态检测
go test -race ./...

# 覆盖率
go test -coverprofile=coverage.out ./...

# 静态检查
go vet ./...

# Lint（golangci-lint）
golangci-lint run ./...
```

### 11.2 CI 工作流（`.github/workflows/ci.yml`）

CI 必须包含：
1. `go build ./...` + `go build ./tools/...`
2. `go vet ./...`
3. `go test -race ./...`（单元测试）
4. `golangci-lint run`
5. `scripts/check-sensitive-paths.sh`

### 11.3 构建检查清单（提交前）

- [ ] `go build ./...` 成功
- [ ] `cd tools && go build ./...` 成功
- [ ] `go test -race ./...` 通过
- [ ] `go vet ./...` 无警告
- [ ] `golangci-lint run ./...` 通过
- [ ] `go fmt ./...` 已运行

### 11.4 Release

使用 `.goreleaser.yaml` 配置的 goreleaser 发布二进制文件。

---

## 12. 安全规范

### 12.1 敏感路径

以下文件变更需要特别的人工审查（L4 审查要求）：

| 文件 | 安全边界 |
|------|---------|
| `agentcore/handoff.go` | 交接白名单校验 (`isHandoffAllowed`) |
| `guardrails/levels.go` | 护栏等级枚举 (Light/Standard/Strict) |
| `domains/router.go` | 路由白名单 AllowedSources |
| `domains/approval.go` | ApprovalGate 生命周期钩子 |
| `tools/path.go` | 文件系统沙箱隔离 |
| `tools/tools.go` | 工具能力门控 (ExtensionConfig) |
| `agentcore/manifest.go` | Manifest 校验规则 |
| `domains/project.go` | ValidateProjectPath 路径校验 |
| `tools/bash.go` | Bash 工具（非沙箱模式） |

### 12.2 密钥管理

- 密钥通过环境变量注入，禁止硬编码。
- 测试/示例代码中使用占位符（如 `sk-your-test-key`）。
- 配置文件中的密钥通过 `pkg/agentconfig` 统一管理。

### 12.3 免责声明与措辞规范

面向用户的文案遵循 `docs/tone-style-guide.md`：
- 不使用绝对化表述（绝对/一定/百分百 → 通常/大概率）
- 结论性表述附带置信度标注
- 拒绝类文案提供替代性帮助
- 日常对话中不提及中观/佛教出处

---

## 13. 附录：Mady 项目已有模式速查

### 13.1 核心设计模式

| 模式 | 示例 | 文件 |
|------|------|------|
| LifecycleChain | 多 Hook 组合为链式调用 | `agentcore/lifecycle.go` |
| ExtensionRegistry | 通过扩展注入工具/钩子 | `agentcore/extension.go` |
| EventBus + Broker | 异步事件发布-订阅 | `agentcore/event.go`、`agentcore/pubsub.go` |
| HandoffDelegate | 委派给子 Agent，含白名单校验 | `agentcore/handoff.go` |
| ContextEngine | 可切换的上下文引擎（分层/压缩/自定义） | `agentcore/context_engine*.go` |
| Config 嵌入 + Validate | 子配置嵌入 + 提前校验 | `agentcore/config.go`、`agentcore/agent.go` |
| 结构化错误类型 | Retryable/Fatal/Node/Guardrail/HandoffError | `agentcore/errors.go` |
| 内置 Hook 零值基类 | BaseLifecycleHook 提供所有方法的空实现 | `agentcore/lifecycle.go:91` |
| Worker Pool | 泛型 Worker Pool 控制并发 | `agentcore/concurrency/pool.go` |
| Pregel 图计算 | 分布式图处理范式 | `graph/pregel.go` |

### 13.2 目录职责速查

| 目录 | 职责 | 外部依赖 |
|------|------|---------|
| `agentcore/` | Agent 运行时、事件、钩子、压缩、配置 | 极少（otel, websocket） |
| `a2a/` | Agent-to-Agent 协议 | gorilla/websocket |
| `a2ui/` | Agent-to-UI 声明式协议 | 无 |
| `acp/` | Agent 通信协议（JSON-RPC） | 无 |
| `agui/` | Agent GUI 事件（SSE） | 无 |
| `domains/` | 领域 Agent 配置 + 推理引擎 | 依赖 agentcore |
| `tools/` | 内置工具扩展（独立子模块） | 可被外部导入 |
| `tui/` | 终端 UI（8 层 Elm） | 无外部 GUI 依赖 |
| `graph/` | 图引擎（DAG + Pregel） | 无 |
| `guardrails/` | 三级安全护栏 | 无 |
| `knowledge/` | 知识管理 | modernc.org/sqlite |
| `mcp/` | MCP 客户端 | gorilla/websocket |

### 13.3 Git 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)：

```
类型: 描述

feat:    新功能
fix:     修复
docs:    文档
test:    测试
refactor: 重构
chore:   构建/工具变更

AI 参与的提交附加 Co-authored-by 标记：
Co-authored-by: Claude <noreply@anthropic.com>
```

### 13.4 代码审查等级

| 层级 | 适用场景 | 要求 |
|------|----------|------|
| L1 | 纯格式/文档/测试补充 | 常规 review，可 AI 自审 |
| L2 | Bug 修复、单函数调整 | 至少一位非提交者人工 review |
| L3 | 新功能、架构变更 | 强制人工 review + 对照 Spec |
| L4 | 触及安全红线 | 两人 approve，AI 不能作为唯一 approver |

### 13.5 常用开发流程

| 场景 | 步骤 |
|------|------|
| 添加新工具 | `tools/` 下创建文件 → `tools/tools.go` 注册 → 编写测试 |
| 添加新领域 | `domains/` 下创建配置 → 实现 System Prompt → `domains/router.go` 注册 → `skills/` 添加 SKILL.md |
| 添加新技能 | `skills/<domain>/` 下创建 `SKILL.md` → YAML frontmatter → 使用说明 |
| 新功能/架构调整 | `docs/specs/` 下 proposal → spec → design → tasks 四阶段 |

---

> 本文档持续更新。发现新的最佳实践或项目模式变化时，请同步更新本文档。
>
> 参考链接：  
> - [Effective Go](https://go.dev/doc/effective_go)  
> - [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)  
> - [Standard Go Project Layout](https://github.com/golang-standards/project-layout)  
> - [Kubernetes Coding Conventions](https://github.com/kubernetes/community/blob/master/contributors/guide/coding-conventions.md)  
> - [Uber Go Style Guide](https://github.com/uber-go/guide)  
> - [Go Wiki: Go Code Review Comments (中文)](https://golang.google.cn/wiki/CodeReviewComments)
