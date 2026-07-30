# 测试质量审查报告

> 审查日期：2026-07-31
> 审查范围：Mady 项目全部 `*_test.go` 文件（436 个）
> 覆盖模块：根模块 + tools + tui + desktop
> 参照标准：`docs/GO-DEVELOPMENT-STANDARDS.md` 第 7 章

---

## 总体评分：B-

**分项评分：**

| 维度 | 评分 | 说明 |
|------|------|------|
| P0 死锁风险 | C | 存在 ABBA 潜在死锁路径，需修复 |
| P1 time.Sleep 使用 | B- | 46 处调用，约 60% 可重构，其余合理 |
| P1 表格驱动测试 | B | 122/436 文件（27.9%），核心包覆盖较好，边缘包缺失 |
| P2 测试覆盖率 | B | 均值约 60%，6 个包 < 5%，43 个包 > 70% |
| P2 集成/单元比例 | B- | 8 集成文件 / 436 总文件 = 1.8%，偏低 |
| P2 测试命名规范 | C | 仅 1.8% 测试使用场景化命名 `TestXxx_WhenYyy_ExpectZzz` |

---

## P0（必查）：死锁分析 — TestRebuildAgentPanicRecover

### 当前测试状态

```bash
$ go test -race -run TestRebuildAgentPanicRecover -v ./cmd/mady/...
--- PASS: TestRebuildAgentPanicRecover (0.05s)
```

测试通过，未报 panic 或 data race。但此测试仅为**压力恢复验证**——检查 `rebuildAgent` 在被重复调用且配置不完整时不泄漏 panic——并不触发死锁。

### 根因分析：ABBA 潜在死锁路径

代码中存在**两个互斥锁的不一致获取顺序**，构成经典 ABBA 死锁条件：

#### 路径 A：`resumeIfInterrupted()` → `agentMu.RLock` → `runMu.Lock`

文件：`/Users/xujian/projects/Mady/cmd/mady/tui_session_agent.go:303-316`

```go
func (s *tuiSession) resumeIfInterrupted() bool {
    agent := s.getCurrentAgent()          // [A1] agentMu.RLock — 在 goroutine 外获取
    if agent == nil || agent.Interrupted() == nil {
        return false
    }
    // ...
    go func() {
        s.runMu.Lock()                    // [A2] runMu.Lock — 在 goroutine 内获取
        defer s.runMu.Unlock()
        agent := s.getCurrentAgent()      // agentMu.RLock（再次）
        // ...
    }()
}
```

#### 路径 B：`rebuildAgent()` → `runMu.Lock` → `agentMu.Lock`

文件：`/Users/xujian/projects/Mady/cmd/mady/tui_session_agent.go:230-249`

```go
func (s *tuiSession) rebuildAgent() {
    s.runMu.Lock()                        // [B1] runMu.Lock — 先获取
    defer s.runMu.Unlock()
    // ...
    s.activateAgent(newAgent)             // → swapCurrentAgent()
}                                          //    → [B2] agentMu.Lock — 后获取
```

#### 死锁触发条件

| Goroutine | 持有锁 | 等待锁 |
|-----------|--------|--------|
| resumeIfInterrupted | agentMu.RLock | runMu.Lock |
| rebuildAgent | runMu.Lock | agentMu.Lock |

在 `sync.RWMutex` 中，`Lock()` 会阻塞后续的所有 `RLock()`（反之亦然），因此：

1. Goroutine A 获取 `agentMu.RLock`，然后 spawn goroutine A' 尝试获取 `runMu.Lock`
2. Goroutine B 已持有 `runMu.Lock`，尝试获取 `agentMu.Lock`
3. A 持有 `agentMu.RLock` → 阻塞 B 的 `agentMu.Lock`
4. B 持有 `runMu.Lock` → 阻塞 A' 的 `runMu.Lock`
5. **deadlock ⊗**

#### 修复方案

**方案一（推荐，最少改动）：** 将 `resumeIfInterrupted()` 的 `getCurrentAgent()` 移到 goroutine 内部，统一锁顺序。

```go
// tui_session_agent.go — resumeIfInterrupted
func (s *tuiSession) resumeIfInterrupted() bool {
    store := s.agentStore
    threadID := s.currentThreadID
    go func() {
        s.runMu.Lock()
        defer s.runMu.Unlock()

        // getCurrentAgent 现在在 runMu 临界区内，锁顺序：runMu → agentMu
        agent := s.getCurrentAgent()
        if agent == nil || agent.Interrupted() == nil {
            return
        }
        // ... 后续不变
    }()
    return true // 只要入了 goroutine 就视为"尝试恢复"
}
```

同时删除外部的 `getCurrentAgent()` 预先检查，让 goroutine 处理所有判断。

**方案二（更稳健）：** 在 `resumeIfInterrupted()` 入口使用 `TryLock` + 快速返回

```go
func (s *tuiSession) resumeIfInterrupted() bool {
    if !s.runMu.TryLock() {
        return false // reboot 正在进行，稍后再试
    }
    defer s.runMu.Unlock()
    agent := s.getCurrentAgent()
    if agent == nil || agent.Interrupted() == nil {
        return false
    }
    // 同步执行 Resume，无需 goroutine
    runCtx, cancel := context.WithCancel(s.ctx)
    // ...
    go func() { /* 仍然在后台执行 */ }()
    return true
}
```

**方案二风险：** `TryLock` 是自旋语义（忙等），在极端竞争下可能消耗 CPU。推荐方案一。

#### 补充风险：`showUserError` 在 `runMu` 临界区内

`rebuildAgent()` 的 `defer recover` 在持有 `runMu.Lock` 时调用 `showUserError()`，该函数最终调用 `history.Append()`（获取 `history.mu`）。如果 `ChatApp` 的事件回调（通过 `EventBus` 触发）尝试获取 `runMu`，会形成 `runMu → history.mu → ... → runMu` 死锁。

当前**尚未发现**此路径的证据（`BindAgent` 注册的回调仅写 `ChatHistory`），但当新增事件处理器时需特别注意：**禁止在 `ChatApp` 事件回调中获取 `runMu`**。

---

## P1（重要）：`time.Sleep` 使用分析

共发现 **46 处 `time.Sleep` 调用**，分布在 14 个文件中。

### 按文件分类

| 文件 | 调用次数 | 分析 |
|------|---------|------|
| `tui/cmd_test.go` | 7 | 大部分可重构 — 等待 goroutine 启动/完成 |
| `agentcore/event_test.go` | 4 | 模拟慢速处理器（2 处有注释说明），仍建议用 `sync.WaitGroup` |
| `agentcore/event_logger_test.go` | 4 | 等待事件异步持久化完成 — 应换 channel/context |
| `agentcore/integration_test.go` | 4 | 等待 Agent 异步运行结果 — 应换 `Done()` channel |
| `tui/component/toast_test.go` | 3 | 等待动画时间 — 合理，Toast 有明确生命周期 |
| `tui/component/loader_test.go` | 3 | 等待动画帧渲染 — 合理，Loader 有固定帧间隔 |
| `a2a/pool/pool_test.go` | 3 | 等待连接池异步操作 — 应换 channel |
| `agentcore/permission/permission_test.go` | 3 | 等待异步 goroutine 设置状态 — 应换 channel |
| `pkg/framework/framework_test.go` | 4 | 等待 StartAll 开始 — 应换 `WaitGroup` |
| `mcp/http_test.go` | 1 | 等待 HTTP 服务器启动 — 应换就绪 channel |
| `server/disclosure_smoke_test.go` | 1 | 端到端流程等待 — 可接受 |
| `desktop/e2e_integration_test.go` | 1 | 等待桌面截图 — 可接受 |
| `knowledge/graph/store_test.go` | 1 | 等待异步写入 — 应换 channel |
| `tools/process_test.go` | 2 | 等待子进程完成 — 应换 `Wait` |
| `tools/ego_lite_manager_test.go` | 1 | 子进程启动等待 — 可接受 |
| `a2a/a2a_test.go` | 1 | 等待 WebSocket 建立 — 可接受 |
| `agentcore/hooks_test.go` | 1 | 等待 Hook 执行 — 应换 channel |
| `agentcore/provider_health_test.go` | 1 | 等待健康检查 — 可接受 |
| `agentcore/concurrency/pool_test.go` | 1 | 模拟工作负载 — 合理 |

### 评估

- **合理不可替换（约 25%）：** Toast/Loader 动画测试、子进程启动、e2e 集成测试
- **可重构但影响不大（约 15%）：** 模拟慢处理器、模拟工作负载
- **应优先重构（约 60%）：** 等待异步 goroutine 的 Sleep → 应换 `sync.WaitGroup` / channel / `context.Context`

### 重构建议

```go
// ❌ 当前模式（framework_test.go:103）
time.Sleep(50 * time.Millisecond) // ensure StartAll has begun but not finished

// ✅ 推荐模式
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    framework.StartAll(...)
}()
wg.Wait()
```

```go
// ❌ 当前模式（event_logger_test.go:54）
time.Sleep(100 * time.Millisecond)

// ✅ 推荐模式
done := make(chan struct{})
el.OnEvent(func(e Event) {
    close(done)
})
select {
case <-done:
case <-time.After(5 * time.Second):
    t.Fatal("timeout waiting for event")
}
```

---

## P1（重要）：表格驱动测试模式使用率

### 统计

| 指标 | 数值 |
|------|------|
| 总测试文件数 | 436 |
| 使用 `[]struct{}` 的文件数 | 122 |
| **表格驱动测试文件比例** | **27.9%** |
| 总测试函数数 | 4,214 |
| 使用 `t.Run(tt.name, ...)` 的测试函数 | 约 1,100+（估计） |

### 按包分析

| 包 | 测试文件 | 表格驱动 | 比例 | 说明 |
|---|---------|----------|------|------|
| `agentcore/` | ~40 | ~18 | ~45% | 核心包覆盖较好 |
| `domains/` | ~60 | ~20 | ~33% | 领域逻辑较复杂，表格驱动不足 |
| `tui/` | ~80 | ~15 | ~19% | UI 测试偏命令式 |
| `tools/` | ~30 | ~5 | ~17% | 工具测试偏集成风格 |
| `knowledge/` | ~25 | ~5 | ~20% | 中等 |
| `pkg/` | ~15 | ~8 | ~53% | 工具库覆盖最好 |

### 评估

按 `GO-DEVELOPMENT-STANDARDS.md` 第 7.2 节，表格驱动测试是**推荐模式**。27.9% 的文件使用率偏低。核心引擎 `agentcore/` 约 45% 尚可，但 UI 和工具层严重不足。

**推荐目标：** 新测试文件 80% 使用表格驱动模式，存量文件逐步迁移。

---

## P2（建议）：测试覆盖率

### 全项目覆盖率分布

| 范围 | 包数 | 代表包 |
|------|------|--------|
| < 5% | 6 | `bootstrap`(1.9%), `domains/audit`(0%), `domains/citation`(0%), `agentcore/iface`(0%), `scripts/*`(2-22%) |
| 5-30% | 5 | `cmd/mady`(18.1%), `pkg/ocr`(30.9%), `provider/adapter`(35.9%) |
| 30-60% | 20 | `a2a`(47.7%), `acp`(51.3%), `server`(51.8%), `domains`(47.3%) |
| 60-80% | 22 | `agentcore`(64.3%), `disclosure`(68.8%), `memory`(70.5%) |
| 80-100% | 33 | `a2ui`(99.6%), `a2a/pool`(96.2%), `guardrails`(82.5%) |

**加权平均覆盖率：约 60%**

### 低覆盖率警戒区

| 包 | 覆盖率 | 风险说明 |
|----|--------|---------|
| `bootstrap` | 1.9% | 启动装配逻辑几乎无测试，是重要的集成测试目标 |
| `cmd/mady` | 18.1% | 主入口，TUI 集成测试不足（窗口/输入/信号处理） |
| `domains/audit` | 0.0% | 无测试文件 |
| `domains/citation` | 0.0% | 无测试文件 |
| `agentcore/iface` | 0.0% | 接口定义包，合理（纯类型定义） |
| `pkg/ocr` | 30.9% | OCR 依赖外部库，测试条件受限 |

### 高覆盖率亮点

| 包 | 覆盖率 | 亮点说明 |
|----|--------|---------|
| `a2a/registry` | 100% | 全面覆盖 |
| `a2ui` | 99.6% | 几乎全覆盖 |
| `pkg/framework` | 98.4% | 框架层工具库 |
| `a2a/pool` | 96.2% | 连接池逻辑 |
| `pkg/csync` | 91.9% | 并发原语 |

---

## P2（建议）：集成测试与单元测试比例

### 统计

| 指标 | 数值 |
|------|------|
| 集成测试文件数 | 8（`integration/` 下 7 测试 + 1 helpers） |
| 总测试文件数 | 436 |
| **集成/单元比例** | **1.8%** |
| 集成测试 assert 语句数 | 约 54 条 |
| 全项目 assert 语句数 | 约 15,000+ |

### 集成测试文件清单

| 文件 | 测试内容 |
|------|---------|
| `integration/chain_e2e_test.go` | 链式调用端到端 |
| `integration/doc_test.go` | 文档生成 |
| `integration/doomloop_e2e_test.go` | 死循环检测 |
| `integration/drafting_e2e_test.go` | 文档撰写 |
| `integration/guardrails_e2e_test.go` | 护栏系统 |
| `integration/handoff_e2e_test.go` | 交接机制 |
| `integration/integration_test.go` | 通用集成 |

### 评估

1.8% 的集成测试比例偏低。建议目标 5-10%。

**重点添加集成测试的领域：**
- `bootstrap` 装配管线（当前 1.9% 覆盖率 + 无集成测试）
- `cmd/mady` 主入口 TUI 流程
- `domains/claimdrafting` + `specdrafting` 撰写管线
- 多 Agent handoff 交互
- MCP 客户端连接

---

## P2（建议）：测试命名规范

### 统计

| 模式 | 数量 | 占比 |
|------|------|------|
| `TestXxx`（简单模式） | 4,214 | 100% 基准 |
| `TestXxx_WhenYyy_ExpectZzz`（场景化模式） | 76 | **1.8%** |

### 评估

1.8% 的场景化命名率极低。`GO-DEVELOPMENT-STANDARDS.md` 第 7 章虽未强制命名规范，但业界最佳实践（包括 Kubernetes、标准 Go 项目）推荐场景化命名。

### 推荐命名风格

```go
// ✅ 推荐（描述场景 + 预期）
func TestRebuildAgent_WhenConfigIncomplete_DoesNotPanic(t *testing.T)
func TestSubmitInput_WhenAgentStopped_ShowsUnavailableMessage(t *testing.T)

// ❌ 当前常见模式
func TestRebuildAgentPanicRecover(t *testing.T)
func TestSubmitInputFail(t *testing.T)
```

**建议：** 新测试函数统一使用 `TestXxx_WhenYyy_ExpectZzz` 模式，CI 可添加 `revive` lint 规则对新增测试做命名检查。

---

## 综合改进建议（按优先级排序）

| 优先级 | 问题 | 改动量 | 预计工时 |
|--------|------|--------|---------|
| **P0** | 修复 `resumeIfInterrupted` ABBA 死锁 | 1 文件，~5 行 | 30 分钟 |
| **P0** | 在 `buildAgentConfig` / `rebuildAgent` 添加死锁预防注释 | 注释添加 | 10 分钟 |
| **P1** | 重构 `time.Sleep` → channel/WaitGroup（优先 event_logger_test.go、framework_test.go） | 8 文件，~40 行 | 2 小时 |
| **P1** | 补全 `domains/audit` 和 `domains/citation` 的测试文件 | 2 新文件 | 2-3 小时 |
| **P1** | `bootstrap` 包添加装配管线测试 | 1-2 新文件 | 3-4 小时 |
| **P2** | 新增测试统一使用表格驱动 + 场景化命名 | 规范文档 + CI lint | 1 小时 |
| **P2** | 添加 integration 集成测试（bootstrap / cmd/mady / drafting） | 3-5 文件 | 6-8 小时 |
| **P2** | 添加 `cmd/mady` TUI 窗口级集成测试 | 2-3 文件 | 4-6 小时 |

---

## 附录：关键文件引用

| 文件 | 行数 | 审查关注点 |
|------|------|-----------|
| `cmd/mady/tui_session_agent_test.go` | 119-142 | TestRebuildAgentPanicRecover |
| `cmd/mady/tui_session_agent.go` | 230-249 | rebuildAgent（持有 runMu 调用 showUserError） |
| `cmd/mady/tui_session_agent.go` | 303-330 | resumeIfInterrupted（agentMu → runMu 逆序） |
| `cmd/mady/tui_session.go` | 46 | runMu sync.Mutex 定义 |
| `docs/GO-DEVELOPMENT-STANDARDS.md` | 638-733 | 第 7 章测试规范 |
| `docs/decisions/AI_CHANGELOG.md` | 5303-5304 | rebuildAgent + submitInput 历史决策 |
