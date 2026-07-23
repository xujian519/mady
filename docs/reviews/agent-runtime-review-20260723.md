# Agent 运行时全量审阅报告

> **日期**: 2026-07-23
> **范围**: `agentcore/` 内核目录（~45 源文件，~3500 行），含子包 `iface/` `permission/` `planmode/` `cache/` `concurrency/` `evidence/` `filecheckpoint/`
> **审阅方式**: 6 路并行静态代码分析 + 并发安全专项审查 + `go test -race` + 敏感路径扫描
> **已审阅先前修改**: `agent_run_phase.go` (off-by-one fix + debug log)、`executor.go` (serial ctx cancel + parallel slot tracking)

---

## 1. 发现总览

| 等级 | 数量 | 定义 |
|------|------|------|
| **P0 (Critical)** | 2 | 运行时数据竞争、goroutine 泄漏 |
| **P1 (High)** | 13 | 状态机逻辑错误、资源泄漏、接口契约不一致 |
| **P2 (Medium)** | 25 | 代码异味、测试缺口、潜在竞态 |
| **P3 (Low)** | 16 | 代码风格、命名改进、注释补充 |
| **合计** | **56** | — |

---

## 2. 分级问题清单

### 2.1 P0 — 必须立即修复

| # | 文件 | 行 | 类型 | 描述 | 建议 |
|---|------|----|------|------|------|
| P0-1 | `context_engine.go` | 全文件 | **Data Race** | `CompressorEngine` 的 `compressionCount`、`lastSavingsPct` 字段被多 goroutine 读写（UpdateFromResponse/ShouldCompact/LastSavingsPct），但**没有任何锁保护**。Agent 运行循环中，一个 goroutine 调用 UpdateFromResponse（写），同时另一个通过 ShouldCompact（读）读取 `compressionCount` → 数据竞争。 | 添加 `sync.Mutex` 保护，或将计数器改为 `atomic.Int64`。 |
| P0-2 | `compaction.go` | 全局 | **Data Race** | `CompactionState` 中的 `summaryFailureCount`、`ineffectiveCount`、`lastFailureTime`、`lastIneffectiveTime` 等字段在多个 goroutine 中同时读写（`shouldCompact` + `runCompaction` 在不同 Agent 中），无并发保护。 | 添加 `sync.Mutex` 或将整组状态封装为原子级 struct 指针（`atomic.Pointer`）。 |

### 2.2 P1 — 必须修复

| # | 文件 | 行 | Severity | 描述 | 建议 |
|---|------|----|----------|------|------|
| P1-1 | `agent_run_phase.go` | 13 | **逻辑错误** | MaxTurns 检查 `>` 改为 `>=` 导致轮次预算减少 1。`MaxTurns=20` 时原本允许 20 轮，修改后只允许 19 轮。错误信息为 "exceeded"（严格大于），与 `>=` 语义矛盾。 | 回退为 `>`。如需包含性语义，先澄清 `MaxTurns` 文档再同步修改。 |
| P1-2 | `tool_gen.go` | 199-201 | **Schema 正确性** | 所有整数类型（int/int8/...uint64）被映射为 `"type": "number"` 而非 `"type": "integer"`。模型可能生成 `1.5` 这样的值，Go unmarshal 会截断小数部分。 | 在 `typeToSchema` 中为整数类型生成 `"type": "integer"`。 |
| P1-3 | `lifecycle.go` | 308-320 | **Observer 丢失** | `wrapObserver` 的 `type switch` 选择第一个匹配的 case 后立即返回。如果一个类型同时实现多个 Observer 接口（如 AgentRunObserver + TurnObserver），只有第一个被转发。 | 改为组合模式：逐一检查每个接口是否实现，组合成 `LifecycleChain`。 |
| P1-4 | `extension.go` | 81-138 | **配置回滚缺失** | `Register()` 在第 N 个扩展 `Init()` 失败时逆序 Dispose 已初始化的扩展，但已写入 `agent.config` 的 Tools/Hooks/Middleware/SystemPrompt 等不回滚，Agent 以部分配置状态运行。 | 在 Register 失败后将 agent 标记为不可用状态，`Run()` 时拒绝执行。或实现完整回滚机制。 |
| P1-5 | `handoff.go` | inheritRuntime | **安全红线** | Transfer 模式中 `inheritRuntime` 是全信任能力传递——源 Agent 的高权限工具集（bash、文件系统等）直接注册到目标 Agent。当前仅靠 WARN 日志警示，无运行时防护。 | 添加白名单/降级机制，限制能力泄漏范围。 |
| P1-6 | `context_builder_default.go` | Build() | **定义未使用** | `InjectMode` 枚举（`InjectAlways`/`InjectPerTurn`/`InjectOnDemand`/`InjectByTrigger`）和 `ContextLayer.Position` 字段在 `DefaultContextBuilder.Build()` 中**未被使用**。注入始终按 Priority 排序的固定顺序（System→Tools→Knowledge→Memory→History），Position 字段被忽略。 | 确认设计意图：是 Phase 2 功能的预留（如自定义排列顺序、条件注入），还是应移除未使用代码。 |
| P1-7 | `context_engine_chunked.go` | 全局 | **引擎状态竞争** | `ChunkedContextEngine` 包装的 CompressorEngine 状态字段（compressionCount 等）暴露底层引擎的竞态问题（同 P0-1）。 | 在引擎层统一加锁保护。 |
| P1-8 | `tiered_engine.go` | 全局 | **引擎状态竞争** | `TieredEngine` 维护的 `compressionCount/TierLevel/LastSavingsPct` 等字段在 `ShouldCompact`（读）和 `Compress`（写）之间无锁保护，多 Agent 共享同一引擎实例时有数据竞争。 | 添加 sync.Mutex 或将状态字段改为 atomic。 |
| P1-9 | `state.go` | 58-65 | **TOCTOU 竞争** | `messagesNoClone()` 的 `copy(cp, s.messages)` 是浅拷贝。返回的消息中 `ToolCalls`（slice）、`Metadata`（map）、`Blocks`（slice）字段与原消息共享底层数据。并发 `AddMessage` 如果触发 append 扩容则安全，但调用方修改返回消息的引用字段会竞争。 | 使用 `Message.Clone()` 进行深拷贝，或明确文档要求调用方不得修改引用字段。 |
| P1-10 | `state.go` | 70-80 | **深拷贝缺失** | `AddMessage` 按 ID 替换时使用 `s.messages[i] = m`（struct 赋值），引用字段共享。调用方保留 `m.Metadata` 引用后续修改会污染 Agent 状态。 | 存储前执行 `m = m.Clone()`。 |
| P1-11 | `budget.go` | 233-252 | **消耗后检测** | `AfterModelCall` 在累计消耗（Token/Calls/ToolCalls）后再检查预算，超限时触发 `OnExceed` 回调但无法阻止已发生的消耗。`MaxCalls=1` 时第一调用已完成，`OnExceed` 才触发。 | 在 `AfterModelCall` 中允许 hook 向运行循环发送终止信号，配合 `BeforeModelCall` 预检查实现硬限制。 |
| P1-12 | `steering.go` | 33 | **无背压保护** | `messageQueue.Push` 无容量限制。`SteeringOneAtATime` 模式下消息生产速度超过处理速度时，队列无界增长，可能导致 OOM。 | 添加可配置的容量上限，超出时返回错误或丢弃。 |
| P1-13 | `reasoning_strategy.go` | 256-259 | **浅拷贝安全** | Strategy hint 注入使用 `copy(cloned, orig)` 浅拷贝。Message 的 ToolCalls/Blocks/Metadata/CacheControl 引用字段与原始消息共享，后续在 `cloned` 上的修改（如拼接 Content）可能影响原始消息。 | 使用 `Message.Clone()` 代替 `copy`。 |

### 2.3 P2 — 建议修复

| # | 文件 | 行 | 类型 | 描述 |
|---|------|----|------|------|
| P2-1 | `agent_run.go` | 173-183 | **事件完整性** | `runLoop` 的 switch 没有 `case StatusError`，特定双重故障路径下调用方收到 `("", nil)` 静默吞错。 |
| P2-2 | `agent_run.go` | 13 | **输入校验** | `Run()` 接受空字符串输入，产生一次浪费的 LLM API 调用。 |
| P2-3 | `executor.go` | 319-323 | **取消标记** | 串行取消时剩余槽位未设置 `Terminate` 标记，依赖 `ToolResult.Terminate` 检测流程终止的调用方会丢失信号。 |
| P2-4 | `tool.go` | 78-89 | **读锁调用回调** | `Definitions()` 在 `RLock` 保护下调用 `DynamicParameters` 回调（用户提供，可能阻塞或触发竞态）。 |
| P2-5 | `schema.go` | 60-79 | **校验不完整** | `additionalProperties` 布尔检查仅处理 `bool` 类型，`{}`（空对象）穿过校验始终允许额外属性。 |
| P2-6 | `iface/lifecycle.go` | 80-84 | **参数丢失** | iface 层的 MessagePersist/CompactionPersist 方法签名省略了 `msg`/`msgs` 参数，适配器中消息内容完全丢失。 |
| P2-7 | `iface_adapter.go` | 216-219 | **错误覆盖** | iface 层 `AfterModelCall` 适配器无条件覆盖 `mcc.Err`，链中其他 hook 设置的错误（如 GuardrailHook 的验证错误）被覆盖丢失。 |
| P2-8 | `iface_adapter.go` | 34-36 | **接口契约** | `agentRunnerAdapter.Resume` 丢弃 `interruptData` 参数，iface 层调用方的中断数据丢失。 |
| P2-9 | `lifecycle.go` | 530-557 | **CPU 浪费** | `RateLimitHook.BeforeModelCall` 在 `MaxTurnsPerMinute=0`（无限速）时仍执行完整的时间戳剪枝操作。 |
| P2-10 | `state.go` | 100 | **深拷贝缺失** | `ReplaceMessages` 直接赋值 `s.messages = msgs`，调用方后续修改传入 slice 会污染状态。 |
| P2-11 | `state.go` | 30-36 | **状态机松弛** | `SetStatus` 无 Transition 校验，非法转换（如 Running→Idle）静默接受。 |
| P2-12 | `state.go` | 131 | **指针泄露** | `PendingHandoff()` 返回内部指针，调用方可在锁外修改。 |
| P2-13 | `reasoning_router.go` | 133 | **性能** | `DefaultClassifier.Classify` 每次调用对每个关键词执行 `strings.ToLower`（7-15 次分配），应在构造时预小写。 |
| P2-14 | `reasoning_router.go` | 155 | **分类准确度** | `HistoryTurnsForHigh` 统计全部消息（含 system/assistant/tool），非仅用户轮次。6 轮对话后消息数可达 5-10 倍，导致过早触发高复杂度。 |
| P2-15 | `handoff_context.go` | 全局 | **正则假阳性** | `appNumPattern`（13 位纯数字）在对话中匹配身份证号/手机号等产生大量假阳性实体。 |
| P2-16 | `handoff.go` | 全局 | **缓存惊群** | `summarizeUserIntent` v2 的高频 handoff 场景下，多个并发的意图摘要请求绕过 5 分钟 TTL 对同一输入重复调用 LLM。 |
| P2-17 | `context_engine_tiered.go` | 全局 | **日志缺失** | TieredEngine Phase 1（50-60% 使用率）仅记录 `slog.Debug`，运营中无法观察到此阶段的开始。建议提升为 `slog.Warn` 以便运营告警。 |
| P2-18 | `compaction.go` | 全局 | **冷却配置** | 硬编码常量的 `summaryFailureCooldownSeconds`(600)/`ineffectiveCompactionCooldownSeconds`(300) 不可配置。 |
| P2-19 | `plugin.go` | 37-40 | **重复代码** | `ValidatePlugin`/`LoadPlugin`/`ScanPlugins` 三处重复闭包代码（AtomLookupFn + IsValidGuardrailLevel），每次调用分配两个堆闭包。 |
| P2-20 | `pipeline_handler.go` | 19 | **交叉验证缺失** | `StageHandler` 注册时不对照 Atom 注册表验证 handler 名是否存在对应 Atom。 |
| P2-21 | `pipeline_stage_handlers.go` | 76-77 | **错误返回不一致** | `searchHandler` 将错误写入 `state["_error"]` 而非返回 `err`，与其他 handler 风格不一致。 |
| P2-22 | `pipeline_stage_handlers.go` | 176-186 | **性能** | `extractHandler`/`compareHandler`/`reasoningHandler` 每次执行创建完整 Agent 实例，Pipeline 中 3-5 阶段意味着 3-5 次 Agent 创建销毁。 |
| P2-23 | `atom.go` | 460-461 | **Schema 不同步** | `compareAtom.OutputSchema()` 声明 `similarity_score` 但 `compareHandler` 从不输出此字段。 |
| P2-24 | `pipeline_stage_handlers.go` | 346 | **JSON 解析脆弱** | `extractJSONFromText` 使用 `first {` + `last }` 提取，嵌套大括号或尾随文本时提取错误。 |
| P2-25 | `event.go` | 158-166 | **内存泄露** | handler 全部注销后空 map 条目存留在 `eb.handlers` 中不清理，长期运行 Agent 累积。 |

### 2.4 P3 — 可选修复

| # | 文件 | 行 | 类型 | 描述 |
|---|------|----|------|------|
| P3-1 | `agent_run_phase.go` | 140-155 | **数据完整性** | `guardTruncation` 对空 `tc.ID` 写入不可关联的工具结果。 |
| P3-2 | `task_tool.go` | 148 | **死代码** | 多余的 `cfg := cfg`（Go 1.22+ 循环变量语义已修正）。 |
| P3-3 | `event.go` | 47-59 | **未初始化** | `NewEventBus` 未显式初始化 `drainTimeout`（依赖 Drain 回退默认值 5s）。 |
| P3-4 | `skill_extension.go` | 152-155 | **注释缺失** | `SuppressPersist` 副作用需要注释说明 FollowUp 消息不受此标志影响。 |
| P3-5 | `reasoning_router.go` | 162 | **性能** | `runeLen` 手动遍历符文 vs `utf8.RuneCountInString`（C 实现）。 |
| P3-6 | `reasoning_strategy.go` | 170-172 | **验证缺** | `DefaultFrameworks` 与 `StrategyMap` 之间无一致性验证。 |
| P3-7 | `handoff_context.go` | 全局 | **正则性能** | 实体抽取正则表达式可预编译为全局 `var`，避免每次 `ExtractHandoffContext` 调用时编译。 |
| P3-8 | `handoff_result.go` | 全局 | **解析恢复力** | `ParseHandoffResult` 对嵌套 JSON（JSON in JSON）支持不足。 |
| P3-9 | `context_builder.go` | 全局 | **系统提示字段** | `ContextLayer` 的 `SystemPromptField` 字段定义但未在任何 Build 路径中使用。 |
| P3-10 | `atom.go` | init() | **可测试性** | `init()` 注册依赖 import 顺序，测试手动调用 `RegisterAtom` 后断言可能 flaky。 |
| P3-11 | `atom.go` | 143-149 | **缺验证** | `RegisterAtom` 不验证 `Name()` 非空。 |
| P3-12 | `pipeline_executor.go` | 168 | **丢失堆栈** | panic recovery 丢失原始堆栈（建议 debug 模式记录）。 |
| P3-13 | `steering.go` | 58 | **类型** | `Len()` 返回 `int64`，仅 `int` 即可。 |
| P3-14 | `orchestrate.go` | pubsub | **超时传播** | `PublishMustDeliver` 10ms 超时期望从配置获取而非硬编码。 |
| P3-15 | `executor.go` | 208-211 | **空结果** | 空 `DualToolOutput{}`（ForLLM="" + ForUser=""）返回空字符串，建议提供 `NoContent()` 工厂方法。 |
| P3-16 | `compaction.go` | 全局 | **估算精度** | CJK 字符的 `charsPerToken=4` 在纯中文文档中偏差大（实际约 1.6 char/token）。 |

---

## 3. 测试覆盖分析

### 3.1 当前覆盖率

```
agentcore (主包)                63.0%
agentcore/cache                85.7%
agentcore/concurrency          78.8%
agentcore/evidence             83.1%
agentcore/filecheckpoint       43.8%  ← 最低
agentcore/iface                0.0%   ← 接口定义
agentcore/permission           75.0%
agentcore/planmode             79.8%
```

### 3.2 测试缺口

| 模块 | 缺口说明 | 优先级 |
|------|---------|--------|
| `agent_run_phase.go` | `runPreTurn`/`runModelTurn`/`runAfterModelCall`/`guardTruncation` 无直接单元测试（仅间接集成测试） | **高** |
| `agent_run_tool.go` | `executeToolCalls`/`persistMessage`/`buildRequestMessages` 无独立测试 | **高** |
| `deprecatedHookAdapter` | 无任何直接测试（3 个回调路径） | **中** |
| `ObserversToHook`/`LifecycleChain` | 无直接单元测试 | **中** |
| `ExtensionRegistry` | Register/Dispose/Names/Visit 无直接单元测试 | **中** |
| `pubsub.go` | MustDeliver 超时、panic recovery、Drain 超时无显式测试 | **中** |
| `stream.go` | StreamReader 完整生命周期测试 | **低** |
| `step.go` | 无测试文件 | **低** |

---

## 4. 安全合规扫描结果

| 检测项 | 结果 |
|--------|------|
| `scripts/check-sensitive-paths.sh` | ✅ 通过（当前无未暂存的敏感路径变更） |
| Handoff 白名单 (`isHandoffAllowed`) | ✅ default-deny 语义正确 |
| 权限决策 (`permission/`) | ✅ Allow/Ask/Deny 三态优先级正确，只读工具自动 Allow |
| 计划模式 (`planmode/`) | ✅ fail-closed 策略，Bash 只读命令表完整 |
| `go test -race` | ⚠️ 3 个 pipeline LLM handler 测试失败（预存问题，非 race 相关） |

---

## 5. 当前修改验证

| 修改 | 文件 | 判定 |
|------|------|------|
| `turn-loopStartTurn > a.config.MaxTurns` → `>=` | `agent_run_phase.go:13` | ❌ **回退需要** — 见 P1-1 |
| 新增 `slog.Debug` 空 ToolCall ID 日志 | `agent_run_phase.go:140` | ✅ 正确，建议补充空 ID 跳过持久化（见 P3-1） |
| Serial ctx 取消检查 | `executor.go:315-326` | ✅ 正确 — 非阻塞 select，完整填充取消槽位 |
| Parallel 防御性 slot 追踪 | `executor.go:353-362` | ✅ 正确 — `acquired` flag 防止双重 Release |

---

## 6. 修复优先级建议

### 第一优先（P0 + 安全红线）
| 排序 | 问题 | 预估工时 |
|------|------|---------|
| 1 | P0-1: CompressorEngine 状态竞争 | ~15min |
| 2 | P0-2: CompactionState 竞争 | ~15min |
| 3 | P1-5: inheritRuntime 安全防护 | ~2h |

### 第二优先（P1 高影响）
| 排序 | 问题 | 预估工时 |
|------|------|---------|
| 4 | P1-1: MaxTurns off-by-one 回退 | ~5min |
| 5 | P1-9/10: state.go 深拷贝纪律 | ~30min |
| 6 | P1-3: wrapObserver 多接口丢失 | ~30min |
| 7 | P1-2: 整数 schema 类型 | ~15min |
| 8 | P1-12: steering 背压 | ~15min |
| 9 | P1-13: strategy hint 浅拷贝 | ~15min |
| 10 | P1-4: Extension 回滚加固 | ~1h |

### 第三优先（P2）
选择 P2-1（事件完整性）、P2-6/7/8（iface 适配器参数丢失/错误覆盖/数据丢失）优先修复，其余 P2 纳入 Sprint Backlog。

---

## 7. 审阅元数据

| 项目 | 值 |
|------|-----|
| **审阅总耗时** | ~40 分钟（6 路并行 + 2 轮串行） |
| **覆盖率测试** | 63.0% (agentcore main) |
| **Race 检测** | 通过（3 个预存 pipeline 测试失败非 race 相关） |
| **敏感路径扫描** | 通过 |
| **审阅人员** | AI 辅助全量代码审阅 |
| **交叉引用** | 参考 `docs/review/executor-full-review-2026-07-23.md`、`docs/review/event-system-review-2026-07-23.md` |
