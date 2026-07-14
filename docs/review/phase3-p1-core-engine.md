# Phase 3: P1 核心引擎深度审阅报告

> 日期：2026-07-14
> 审阅范围：agentcore/ (90+48) + domains/reasoning/ (16+11)

## 审计总结

| 模块 | 评估 | 关键发现 |
|------|------|---------|
| `agentcore/agent_run.go` | 🟢 优秀 | 执行循环设计成熟，重复检测、截断防护、中断处理完备 |
| `agentcore/event.go` | 🟢 优秀 | 类型安全事件系统 |
| `agentcore/pubsub.go` | 🟢 优秀 | 泛型 Broker，双语义投递，可观测性好 |
| `agentcore/stream.go` | 🟢 优秀 | 生命周期管理完善，防止 goroutine 泄漏 |
| `agentcore/executor.go` | 🟢 优秀 | 洋葱中间件链，双输出支持 |
| `agentcore/permission/rule.go` | 🟢 良好 | MustParseRule panic 仅限测试/初始化 |
| `agentcore/evidence/ledger.go` | 🟢 良好 | 简单、线程安全 |
| `domains/reasoning/fact_blackboard.go` | 🟢 良好 | panic 设计有文档说明，是合理的不变量保护 |
| `domains/reasoning/planner.go` | 🟡 有债务 | TODO Phase 4 未实现 |

## 详细审查

### 1. agentcore/agent_run.go — 执行循环 ✅

**评分**：优秀。706 行核心循环设计成熟：

- ✅ **双层循环**：外层 follow-up 消息，内层 tool-call turn
- ✅ **MaxTurns 保护**：防止无限循环（line 138）
- ✅ **重复检测**：文本重复 + 工具调用签名重复（line 416-450），3 次重复注入 steering 中断循环
- ✅ **截断防护**：`finish_reason="length"` + 无效 JSON 参数 → 逐条返回错误结果（line 313-342）
- ✅ **上下文溢出重试**：先压缩再重试一次（line 210-215）
- ✅ **中断处理**：`context.Canceled` → 干净退出而非报错（line 217-225, 385-392）
- ✅ **Transfer 交接**：在内循环结束后处理（line 409-412）
- ✅ **Checkpoint 集成**：每 turn start/end 可选保存（line 175-179, 299-303）
- ✅ **Steering 消息**：turn 开始前注入（line 154-163）

**无发现问题**

### 2. agentcore/event.go — 事件系统 ✅

- 类型安全的事件接口 + baseEvent 组合模式
- 20+ 事件类型覆盖完整生命周期

### 3. agentcore/pubsub.go — 事件总线 ✅

- ✅ 泛型 `Broker[T]`，双语义：`Publish`（非阻塞/可丢失）vs `PublishMustDeliver`（带 50ms 超时）
- ✅ `DropCount()`/`MustDeliverDropCount()` 可观测
- ✅ `Subscribe()` goroutine 正确清理：ctx.Done 或 broker.Shutdown 时 close channel
- ✅ `Shutdown()` 幂等（done channel select）
- ✅ 关闭竞态处理：Subscribe 中 double-check broker 未关闭

### 4. agentcore/executor.go — 工具执行 ✅

- ✅ 洋葱中间件链（buildChain，line 132-138）
- ✅ 无条件 JSON 有效性检查（line 156-161）— 防止截断参数执行
- ✅ Optional Schema 校验（ValidateArguments）
- ✅ DualToolOutput：ForLLM/ForUser/Silent/Terminate
- ✅ Interrupt 透传不包装（line 173-182）
- ✅ UnknownToolHandler 回退

### 5. domains/reasoning/fact_blackboard.go — 事实黑板 ✅

- ✅ 所有方法 goroutine-safe（RWMutex）
- ✅ `Lock()` / `checkNotLocked()` 模式防止锁定后修改
- ⚠️ panic 设计是**有意的**不变量保护（文档明确标注）
  - 评估：合理。在 locked 后修改是编程错误，panic 是最清晰的信号

### 6. domains/reasoning/planner.go — 规划器 🟡

| # | 严重度 | 位置 | 问题 |
|---|--------|------|------|
| P1-1 | Low | `planner.go:62` | `TODO(Phase 4): detectPlanIntent currently never returns ReAct/MultiHypothesis` — 已存在但未实现的规划意图 |

**评估**：Phase 4 路线图项目，非阻塞问题。当前总是回退到模板/fallback Plan，功能正确只是不完整。

### 7. agentcore/permission/rule.go — 权限规则 ✅

- `MustParseRule` panic 仅用于 `var` 初始化和测试 — 合理
- Rule 匹配支持：Bash 命令前缀 `prefix:*`、文件 glob `**`
- `extractMatchValue` 智能字段优先级

### 8. agentcore/evidence/ledger.go — 证据账本 ✅

- nil-safe（所有方法检查 `l == nil`）
- Snapshot 返回副本（防止外部修改）

---

## 整体评估

P1 核心引擎状态：**优秀**。agentcore 运行时循环、事件总线、执行器和推理引擎设计成熟，无 Critical/High 问题。唯一的开放项是 planner.go 中已规划的 Phase 4 TODO。
