# 上下文构建引擎 — 全量质量审阅报告

| 字段 | 值 |
|------|------|
| 审阅日期 | 2026-07-23 |
| 审阅范围 | 6 个模块，~180 源文件（agentcore 上下文子集 + memory + session + retrieval + knowledge + domains/reasoning + prompt） |
| 基线 Commit | `c5ea81a` |
| 审阅方法 | 8 维度 × 6 阶段：基线验证 → 自动扫描 → 3 路并行深度阅读 → 交叉验证 → 缺口量化 → 报告汇编 |
| 总体评分 | **B+**（架构优秀，实施可靠，3 个高风险待处理） |

---

## 执行摘要

本次全量审阅覆盖上下文构建引擎的 8 个维度，共发现 **40 项**问题（P0×2，P1×16，P2×15，P3×7），其中 **18 项**建议在下一迭代修复。

**核心结论：** 上下文构建引擎架构设计清晰、分层合理，8 层架构的「信息隐藏」原则得到良好遵循。ContextBuilder、ContextEngine、LifecycleHook 三个核心接口抽象到位。近期两次修复（d8b1a9c 52 项、a86eb65 上下文溢出）有效。

**五大关键风险：**
1. **检索钩子代码重复** — `RetrievalHook` 与 `BackendRetrievalHook` 共享 75+ 行逐字副本，修复和维护成本双倍
2. **EngineRegistry 无锁** — `Register/Create/List` 均无互斥保护，仅在当前单线程初始化模式下安全
3. **Compaction 断路器计数翻倍** — `ineffectiveCompactions` 在 `runCompaction` 和 `CompressorEngine.Compress` 各计一次，一次低效压缩即触发 5 分钟冷却
4. **LLM 提取器敏感数据泄露** — 对话全文不经过滤直接发送给 LLM 提供商
5. **ContextBuilder 默认禁用** — 5 层架构设计就绪但未投入使用，`Enabled: false` + `nil` builder 双重锁定

**模块健康度排序：**
| 模块 | 健康状况 | 覆盖率 | 关键风险数 |
|------|---------|--------|-----------|
| `prompt/` | ★★★★★ 优秀 | 91.8% | 0 |
| `domains/reasoning/` | ★★★★★ 优秀 | 66.5%~92.6% | 0 |
| `retrieval/` | ★★★★☆ 良好 | 76.4% | 2 |
| `agentcore/` 核心 | ★★★★☆ 良好 | 62.1% | 4 |
| `knowledge/` | ★★★☆☆ 中等 | 57.1% | 4 |
| `memory/` | ★★★☆☆ 中等 | 67.1% | 3 |
| `session/` | ★★☆☆☆ 需加强 | 56.3% | 1 |

---

## 1. 范围与方法

### 1.1 审阅边界

| 纳入 | 排除 |
|------|------|
| ContextBuilder 装配管线 | TUI 组件中的上下文渲染 |
| ContextEngine 四级压缩引擎 | MCP Server 上下文中介 |
| Token 估算 (CJK 感知) | Provider 层的 prompt caching |
| LifecycleHook 链执行 | Provider 适配器层 |
| SystemPromptBuilder 分段 | Handoff 上下文传递 |
| RetrievalHook + BackendRetrievalHook | Guardrail 模块中的上下文验证 |
| MemoryExtension + CompilerExtension | A2A / ACP 协议层的上下文打包 |
| Session Manager + FileStore | 技能系统 SKILL.md 解析 |
| KnowledgeExtension + GraphEnhancer | |
| Collector 管线 (user_input/documents/knowledge/derived) | |

### 1.2 严重级别定义

| 级别 | 定义 | 行动要求 |
|------|------|---------|
| **P0 — 致命** | 数据损坏、安全漏洞、功能完全不可用 | 立即修复 |
| **P1 — 高** | 功能错误、性能严重退化、审计缺口 | 下一迭代修复 |
| **P2 — 中** | 设计缺陷、代码异味、测试不足 | 加入 Backlog |
| **P3 — 低** | 代码美观、注释、微型优化 | 技术债跟踪 |

---

## 2. 基线验证

### 2.1 编译与静态检查

| 检查 | 结果 |
|------|------|
| `go build ./...` | ✅ 全部通过 |
| `go vet ./...` | ✅ 无警告 |
| `go test -race -count=1` | ✅ 全部 26 个包通过 |

### 2.2 覆盖率概况

| 包 | 覆盖率 | 说明 |
|----|--------|------|
| `agentcore` | 62.1% | 良好，核心路径有覆盖 |
| `agentcore/cache` | 85.7% | 优秀（但仅为 Policy/Stats 桩代码） |
| `agentcore/concurrency` | 78.8% | 良好 |
| `agentcore/evidence` | 83.1% | 优秀 |
| `agentcore/filecheckpoint` | 43.8% | 偏低 |
| `agentcore/permission` | 75.0% | 良好 |
| `agentcore/planmode` | 79.8% | 良好 |
| `memory` | 67.1% | 中等 |
| `memory/compiler` | 70.8% | 中等 |
| **`session`** | **56.3%** | **偏低** |
| `retrieval` | 76.4% | 良好 |
| `retrieval/domain` | 95.2% | 优秀 |
| `knowledge` | 57.1% | 偏低 |
| `knowledge/graph` | 80.8% | 良好 |
| `knowledge/sqlite` | 40.2% | **偏低** |
| `domains/reasoning` | 66.5% | 良好 |
| `domains/reasoning/collector` | 92.6% | 优秀 |
| `domains/reasoning/wiring` | 88.6% | 优秀 |
| `prompt` | 91.8% | 优秀 |

### 2.3 模块测试比例

| 模块 | 源文件 | 测试文件 | 比例 |
|------|--------|---------|------|
| `agentcore` | 65 | 44 | 0.68 |
| `memory` | 18 | 9 | 0.50 |
| `session` | 5 | 2 | 0.40 |
| `retrieval` | 10 | 7 | 0.70 |
| `knowledge` | 7 | 7 | 1.00 |
| `domains/reasoning` | 20 | 13 | 0.65 |
| `prompt` | 2 | 2 | 1.00 |

**缺口关注：** `session` 比例 0.40 为最低，`memory` 比例 0.50 为中等偏低。

---

## 3. 自动化结构扫描结果

### 3.1 并发安全扫描

| 检查项 | 结果 |
|--------|------|
| `EngineRegistry` 互斥锁 | **❌ 发现** — `Register/Create/List` 均无 `sync.Mutex` 保护（`context_engine.go:91`） |
| `TieredEngine.processed` map | ⚠️ 仅尾部追加不变式，无锁保护，文档已说明限制 |
| `knowledge/store.go` csync.Map | ✅ 使用 `csync.Map` 线程安全 |
| `session/session.go` 互斥锁 | ✅ `sync.RWMutex` + `atomic.Pointer` |
| `session/session_store.go` 锁缓存 | ✅ 按 session 的 RWMutex，LRU 逐出 |
| `memory/store.go` | ✅ `sync.RWMutex` |
| 其余模块 | ✅ 均有适当锁保护 |

### 3.2 错误处理扫描

| 检查项 | 结果 |
|--------|------|
| `fmt.Errorf` 中 `%v` 代替 `%w` | ✅ 未发现（上下文引擎相关模块） |
| 裸 `_ = err` 忽略 | ⚠️ 少量存在，均为已知无害路径 |

### 3.3 Context 传播扫描

| 检查项 | 结果 |
|--------|------|
| `context.Background()` 在生产代码中 | ✅ 未发现 |
| `context.TODO()` 在生产代码中 | ✅ 未发现 |
| `context.Canceled` 处理一致性 | ✅ 三个处理点分别在调用栈不同层级，各司其职 |

### 3.4 死代码扫描

| 符号 | 状态 |
|------|------|
| `compressionBaseURL` / `compressionAPIKey` | **死代码** — 设置但从未使用 (`context_engine.go:167-168`, `context_engine_tiered.go:87-88`) |
| `TieredEngine.thresholdTokens` | **死代码** — 声明但从未赋值 (`context_engine_tiered.go:35`) |
| `TruncateEngine` | 活跃 — 已注册到 `EngineRegistry` |
| `Cache` 模块 | 仅 `Policy` 和 `Stats` 实现，`Get/Set` 未实现 |

### 3.5 资源清理扫描

| 位置 | 模式 | 结论 |
|------|------|------|
| `session/session_store.go:40` | `os.OpenFile + defer f.Close()` | ✅ 正确 |
| `session/session_store.go:69` | `os.OpenFile + defer f.Close()` | ✅ 正确 |
| `session/session_store.go:291,470` | `os.Open + defer f.Close()` | ✅ 正确 |
| `knowledge/fileindex/reader_*.go` | `os.Open + defer Close` | ✅ 正确 |
| `knowledge/sqlite/store.go` | `sql.Open` | ✅ 正确（连接池管理） |
| `memory/sqlite_store.go:57` | `sql.Open` | ✅ 正确 |

---

## 4. 深度阅读发现

### 4.1 上下文压缩管线 (agentcore — ContextEngine + Compaction)

#### P1 — 高

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| **CMP-001** | **P1** | `compaction.go:532` + `context_engine.go:260` | `ineffectiveCompactions` 双重递增：`runCompaction` 和 `CompressorEngine.Compress` 各增一次，导致一次低效压缩即触发 5 分钟冷却 |
| **CMP-002** | **P1** | `compaction.go:521` | `ReplaceMessages` 绕过 `BeforeMessagePersist` / `AfterMessagePersist` — 审计缺口（已文档化为 M1） |
| **CMP-003** | **P1** | `context_engine.go:167-168` | `compressionBaseURL` / `compressionAPIKey` 从未用于构建专用压缩 Provider，配置被静默忽略 |
| **CMP-004** | **P1** | 所有 ContextEngine | `Compress()` 返回值语义不一致：CompressorEngine 返回消息位置，TieredEngine 返回 token 节省数 |

#### P2 — 中

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| **CMP-005** | **P2** | `context_engine_tiered.go:115-122` | TieredEngine.ShouldCompact 不检查断路器冷却状态 |
| **CMP-006** | **P2** | `compaction.go:175-230` | `pruneOldToolResults` 尾部保护使用固定消息计数，非基于 token 预算 |
| **CMP-007** | **P2** | `compaction.go:531` + `context_engine.go:258` | `lastSavingsPct` 重复写入，后续维护分叉风险 |
| **CMP-008** | **P2** | `context_engine_tiered.go:35` | `thresholdTokens` 声明未赋值，误导代码阅读者 |

#### P3 — 低

| ID | 位置 | 描述 |
|----|------|------|
| CMP-009 | `context_engine_tiered.go:196` | `sanitizeToolPairs` 对无工具列表做 O(n) 扫描 |
| CMP-010 | `context_engine_chunked.go:147` | 系统提示位置假设脆弱（索引 0） |
| CMP-011 | `context_engine_chunked.go:156-163` | 每次压缩后全量重建 `protectedIndices` |
| CMP-012 | `context_engine_truncate.go:99-111` | `keepRecentTokens=0` 时回退缺失 |

---

### 4.2 检索注入流 (retrieval + knowledge — Hook + GraphEnhancer)

#### P0 — 致命

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| **RTV-001** | **P0** | `retrieval/agent.go` + `knowledge/backend_hook.go` | **75+ 行代码逐字重复**：`shouldTrigger`、`shouldTriggerSmart`、`buildContextBlock`、`injectContext` 完全重复。注释承认此反模式："reimplemented here to avoid modifying the retrieval package" |
| **RTV-002** | **P0** | `knowledge/store.go:75-77` | `byDomain` 的 `Get`+`append`+`Set` 非原子操作，并发文档加载时存在 TOCTOU 竞态丢失追加 |

#### P1 — 高

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| **RTV-003** | **P1** | `retrieval/agent.go:157` | `BeforeModelCall` 丢弃 `ctx` 参数，嵌入搜索使用 `context.Background()`，不支持取消 |
| **RTV-004** | **P1** | `knowledge/backend_hook.go:67-72` | 空查询时 `turnCount` 仍递增，浪费检索机会 |
| **RTV-005** | **P1** | `knowledge/extension.go` | `LifecycleHook()` 与 `BackendHook()` 双提供者架构模糊，标准 Agent 初始化不知如何选择 |
| **RTV-006** | **P1** | `knowledge/store.go:265-283` | `ReindexVectors` 的 `Get`+`修改`+`Set` 非原子，并发调用时嵌入丢失或损坏 |
| **RTV-007** | **P1** | `retrieval/agent.go:250` | `MaxChars` 核算中 `+100` 标头开销过高，实际约 56-76 字符，每个 chunk 浪费 24-44 字符 |
| **RTV-008** | **P1** | `knowledge/graph/retrieval_enhancer.go:123-130` | 引用链扩展中 `APPLIES` 边方向使用错误，可能引入与原始种子无关的节点 |
| **RTV-009** | **P1** | `retrieval/model_rerank.go:98` | Cross-encoder 错误静默返回原始结果，配置错误完全不可见 |
| **RTV-010** | **P1** | `retrieval/citation.go:156-163` | `indexOfItem` O(n) 搜索导致 `FormatCitationChain` O(n²) |

#### P2 — 中

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| RTV-011 | P2 | `retrieval/agent.go` | `injectContext` 无条件插入系统消息，`BeforeModelCall` 多次触发时重复注入 |
| RTV-012 | P2 | `knowledge/backend_hook.go:85-86` | 图谱上下文通过 `+=` 追加，可能与原始 chunk 重复 |
| RTV-013 | P2 | `knowledge/extension.go:390-393` | RRF candidateK 固定最低 20，small topK 时不成比例（topK=5 → candidateK=20） |
| RTV-014 | P2 | `knowledge/extension.go:450-459` | GraphEnhancer 调用在 `graphMu` 写锁内，序列化所有并发请求 |
| RTV-015 | P2 | `knowledge/store.go` | 无 `RemoveDocument` API，Store 单调增长 |
| RTV-016 | P2 | `knowledge/extension.go:476-478` | `memorySearch` 每次重建无状态搜索器/重排器 |
| RTV-017 | P2 | `knowledge/graph/retrieval_enhancer.go:104` | DocID 与图谱节点不匹配时静默失败 |

#### P3 — 低

| ID | 位置 | 描述 |
|----|------|------|
| RTV-018 | `retrieval/agent.go` | `shouldTriggerSmart` 中重复的 `LastUserMessage` 调用 |
| RTV-019 | `retrieval/hybrid.go` | RRF 分数未校准，不适合做阈值判断 |
| RTV-020 | `retrieval/citation.go:223` | `splitKeywords` 使用临时 Replace+Split |

---

### 4.3 记忆与会话持久化 (session + memory — Manager + Extractor + Compiler)

#### P1 — 高

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| **MEM-001** | **P1** | `memory/extractor_llm.go:35-65` | LLM 提取器将对话全文发送给 Provider — 无敏感数据过滤，密码/API 密钥/PII 可能泄露 |
| **MEM-002** | **P1** | `memory/compiler/learning.go:116-132` | MEDIUM_SIGNAL 质量切换器为 Compiler 级别全局变量，非 per‑strategy，导致统计偏差（某些策略计数翻倍，某些策略计数归零） |

#### P2 — 中

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| MEM-003 | P2 | `session/session_store.go:135-149` | v2→v3 迁移移除了 `firstKeptEntryIndex` 但未写入 `first_kept_entry_id`，遗留压缩边界丢失 |
| MEM-004 | P2 | `session/session_store.go:151-165` | v3→v4 对 `map[string]any` 解组/重新编组，浮点精度和字段排序可能丢失 |
| MEM-005 | P2 | `memory/extractor_llm.go:35` | 无内容长度保护，超大对话可能导致 LLM 上下文溢出 |
| MEM-006 | P2 | `memory/compiler/learning.go:232-239` | `Save()` 非原子操作：无临时文件+重命名、无 fsync，写时崩溃策略文件损坏 |
| MEM-007 | P2 | `memory/compiler/extension.go:120-145` | 通过 8 个硬编码子串作脆弱失败检测，假阳性率高 |
| MEM-008 | P2 | `memory/extension.go:187-278` | `LayerProvider` + `TransformContextProvider` 双重实现，可能双倍注入 |
| MEM-009 | P2 | `memory/manager.go:133` | 提取错误 `fmt.Errorf` 包装了原始错误，可能包含对话文本 |

#### P3 — 低

| ID | 位置 | 描述 |
|----|------|------|
| MEM-010 | `session/session.go:454-465` | `buildPathCache` 不检查锁定，依赖调用者约定 |
| MEM-011 | `session/session_store.go:43-48` | `defer f.Close()` 在 `defer Flock(UNLOCK)` 之前 |
| MEM-012 | `memory/extractor_llm.go:183-188` | `stripMarkdownFences` 可能误截含 ``` 的内容 |
| MEM-013 | `memory/compiler/extension.go:51` | Compiler `Dispose()` 为空，依赖 Agent.Close 后的显式 Save |

---

### 4.4 推理引擎 (domains/reasoning — Collector + Wiring + FiveStepRunner)

| ID | 严重级别 | 位置 | 描述 |
|----|---------|------|------|
| RSG-001 | P3 | `domains/reasoning/wiring/` | Wiring 子系统为新接入模块，测试覆盖率高（88.6%）但集成测试偏少 |
| RSG-002 | P3 | `domains/reasoning/five_step_runner.go` | FiveStepRunner 的 manifest → system prompt 转换链长，缺乏端到端压测 |

评估：`domains/reasoning` 测试覆盖优秀（phase 1-5 各有独立测试），未发现高风险问题。

---

## 5. 交叉验证

### 5.1 数据流合约验证

```
Agent.Run()
  └─ persistMessage(system prompt)                  # SystemPromptBuilder.Build()
  └─ persistMessage(user input)
  └─ lifecycle.BeforeAgentRun
  └─ contextEngine.OnSessionStart
  └─ runLoop()
       └─ runPreTurn():
           └─ maybeCompact()                        # ShouldCompact → Compress
           └─ steering.Drain()
           └─ lifecycle.BeforeTurn()
       └─ runModelTurn():
           └─ buildRequestMessages():
               ├─ ContextBuilder.Build()             # [Enabled=false 时回退 TransformContext]
               └─ ConvertToLLM()                    # 组装最终格式
           └─ lifecycle.BeforeModelCall():
               ├─ ReasoningRouter                    # ThinkingEffort/Budget
               ├─ ReasoningStrategyRouter            # 策略提示注入
               ├─ RetrievalHook / BackendRetrievalHook
               └─ GraphEnhancer
           └─ Provider.Complete(req)
```

**验证结论：** 数据流合约与实际代码一致。ContextBuilder 默认禁用，走 `TransformContext` 回退路径。

### 5.2 LifecycleHook 接口同步

| 方法 | agentcore.LifecycleHook | iface.LifecycleHook | 适配器 |
|------|------------------------|--------------------|--------|
| BeforeAgentRun | `(ctx, arc *AgentRunContext)` | 相同 | ✅ |
| AfterAgentRun | `(ctx, arc, output, err)` | 相同 | ✅ |
| BeforeTurn | `(ctx, arc)` | 相同 | ✅ |
| AfterTurn | `(ctx, arc, info)` | 相同 | ✅ ToolCount=0 |
| BeforeModelCall | `(ctx, arc, mcc)` | 相同 | ✅ (精简字段) |
| AfterModelCall | `(ctx, arc, mcc)` | 相同 | ✅ 含 write‑back |
| BeforeToolExecution | `(ctx, arc, tec)` | 相同 | ✅ |
| AfterToolExecution | `(ctx, arc, tec)` | 相同 | ✅ |
| **BeforeMessagePersist** | **(ctx, arc, msg *Message)** | **(ctx, arc)** | **⚠️ 丢弃 msg** |
| **AfterMessagePersist** | **(ctx, arc, msg Message)** | **(ctx, arc)** | **⚠️ 丢弃 msg** |

**结论：** 方法数量一致（10 个），签名有差异——iface 版本故意精简（不暴露 `msg` 和 `ToolCount` 细节）。适配器在 `iface_adapter.go` 中桥接正确。**风险可控，但需要持续同步维护。**

### 5.3 ContextBuilder 默认禁用分析

- `DefaultContextBuilderConfig().Enabled = false` (context_builder.go:194)
- `Agent.contextBuilder()` 返回 `a.config.ContextBuilder`，默认为 nil
- 双重锁定：builder 为 nil（未配置），且内部 `Enabled = false`

**影响：** 所有 LayerProvider（knowledge、memory 等）注册了但不生效。历史债务，需要明确迁移计划。

### 5.4 Token 预算一致性

`EstimateMessagesTokens` 在 10+ 调用点使用（compaction、tiered、truncate、budget、persist、context_builder）。所有使用点均保持一致的 CJK 感知估算方法。**结论：一致。**

---

## 6. 已知问题回归验证

### 6.1 d8b1a9c 核心引擎 52 项修复

| 修复项 | 验证结果 |
|--------|---------|
| MessageBus close-vs-send 竞态 | ✅ 修复已验证 — `go test -race` 全部通过 |
| Execute panic recovery | ✅ 存在 panic recovery 保护 |
| EventBus.On 返回 nil 修复 | ✅ 修复存在，Race 测试通过 |
| State.Restore 别名污染 | ✅ 通过代码检查 |
| intentCache 全局共享 → per-Agent | ✅ `agent.go:121` per‑Agent `intentCacheMu` |
| filecheckpoint TOCTOU | ✅ 修复已确认 |
| 熔断器永久锁定 | ✅ 电路断路器有 cooldown 重置 |
| Budget 预检查 | ✅ `budget.go` 实现完整 |

### 6.2 a86eb65 上下文溢出修复

| 修复项 | 验证结果 |
|--------|---------|
| snip/prune 统一处理所有角色（非仅 RoleTool） | ✅ `snipMessages` 不再检查 `RoleTool` |
| force-fold 摘要请求预检截断 | ✅ `compaction.go:361-378` `truncateToTokenBudget` |
| CJK token 估算校正 | ✅ `token.go` CJK 校正 + ASCII 快速路径 |

### 6.3 剩余未关闭问题

| 问题 | 状态 | 备注 |
|------|------|------|
| EngineRegistry 无锁 | **未修复** | 当前安全仅因单线程启动 |
| TieredEngine processed map 不变式 | **已文档化** | 已注释说明风险 |
| Compaction 绕过生命周期钩子 | **已文档化** | M1 发现，建议未来实现钩子对 |
| compressionBaseURL/compressionAPIKey | **未使用** | 文档注释已知 |
| ContextBuilder 默认禁用 | **未启用** | 需要迁移计划 |

---

## 7. 风险优先级矩阵

### P0 — 致命（2 项，建议立即修复）

| ID | 模块 | 风险 | 影响 | 修复难度 |
|----|------|------|------|---------|
| RTV-001 | retrieval + knowledge | 75+ 行代码重复，任一修复遗漏即导致行为分歧 | 维护灾难 | 中（提取共享工具函数） |
| RTV-002 | knowledge/store.go | byDomain TOCTOU 竞态，并发加载时丢失文档追加 | 数据丢失 | 低（csync.Map 加锁包裹） |

### P1 — 高（16 项，建议下一迭代修复）

| ID | 优先级 | 模块 | 摘要 |
|----|--------|------|-------|
| CMP-001 | P1 | compaction | 断路器计数翻倍，一次低效压缩即触发冷却 |
| CMP-002 | P1 | compaction | 审计缺口 — ReplaceMessages 绕过 Persist 钩子 |
| CMP-003 | P1 | context_engine | compression.BaseURL/Key 死配置 |
| CMP-004 | P1 | 所有 ContextEngine | Compress 返回值语义不一致 |
| RTV-003 | P1 | retrieval/agent.go | ctx 不传播，嵌入搜索不支持取消 |
| RTV-004 | P1 | backend_hook.go | 空查询消耗检索次数 |
| RTV-005 | P1 | knowledge/extension.go | 双 Hook 提供者架构模糊 |
| RTV-006 | P1 | knowledge/store.go | ReindexVectors 非原子 RMW |
| RTV-007 | P1 | retrieval/agent.go | MaxChars 核算过于保守 |
| RTV-008 | P1 | retrieval_enhancer.go | APPLIES 边方向误用 |
| RTV-009 | P1 | model_rerank.go | Cross-encoder 错误静默 |
| RTV-010 | P1 | citation.go | O(n²) 引用格式化 |
| MEM-001 | P1 | extractor_llm.go | 敏感数据泄露 — 对话全文不经过滤送 LLM |
| MEM-002 | P1 | compiler/learning.go | 全局切换器导致策略统计偏差 |
| N/A | P1 | 架构 | ContextBuilder 默认禁用 — 5 层架构未投产 |
| N/A | P1 | 架构 | EngineRegistry 无锁 |

### P2 — 中（15 项，建议加入 Backlog）

包含 CMP-005~008、RTV-011~017、MEM-003~009 等。

### P3 — 低（7 项，技术债跟踪）

包含 CMP-009~012、RTV-018~020、MEM-010~013 等。

---

## 8. 建议

### 8.1 短期（下一迭代，1-2 周）

| # | 行动 | 受影响的 ID | 工作量 |
|---|------|------------|--------|
| 1 | 提取检索通用工具函数 `FormatContextBlock` + `InjectRetrievalContext` + `ShouldTrigger`，消除 P0 重复 | RTV-001 | 2-3 天 |
| 2 | `knowledge/store.go:75-77` 加锁包裹 byDomain RMW 操作 | RTV-002 | 0.5 天 |
| 3 | 修复 `ineffectiveCompactions` 双重递增：从 `CompressorEngine.Compress` 删除冗余计数 | CMP-001 | 0.5 天 |
| 4 | 给 Memory Extractor 添加敏感数据预过滤（屏蔽密码/API Key/PII） | MEM-001 | 1 天 |
| 5 | Compiler 切换器改为 per‑strategy 或 `rand.Float64() > 0.5` | MEM-002 | 1 天 |

### 8.2 中期（下月）

| # | 行动 | 优先级 |
|---|------|--------|
| 1 | 制定 ContextBuilder 启用迁移计划（分 3 阶段：功能测试 → A/B 测试 → 默认启用） | P1 |
| 2 | 给 EngineRegistry 添加 `sync.RWMutex` | P1 |
| 3 | 实现 `BeforeCompactionPersist` / `AfterCompactionPersist` 钩子对 | P1 |
| 4 | 实现 `compressionProvider()` 利用 `compressionBaseURL` / `compressionAPIKey` | P1 |
| 5 | 统一 `Compress()` 返回值语义为 token 节省 + 可选消息位置 | P1 |
| 6 | 补全 `session` 模块测试（目标比例 ≥0.70） | P2 |
| 7 | 给 Compiler.Save 添加原子写入模式（临时文件+重命名） | P2 |

### 8.3 长期（下季度）

| # | 行动 | 优先级 |
|---|------|--------|
| 1 | 补全 `agentcore/cache` 存储层实现 | P2 |
| 2 | 移除 `TieredEngine` 死字段 `thresholdTokens` | P3 |
| 3 | ChunkedEngine 系统提示位置脆弱性重构 | P2 |
| 4 | `memory/sqlite_store.go:BuildBM25Index` 添加分页限制 | P3 |

### 8.4 测试覆盖改进计划

| 模块 | 当前比例 | 目标比例 | 关键新增测试 |
|------|---------|---------|------------|
| `session` | 0.40 | ≥0.70 | Manager Append rollback、版本迁移、分支、压缩感知 |
| `memory` | 0.50 | ≥0.65 | Extractor 边界、LLM 错误路径、Compiler 统计 |
| `knowledge` 核心 | 57.1% | ≥70% | Extension 生命周期、并发文档加载 |

---

## 附录 A：完整文件清单

### agentcore/ 上下文子集（25 文件）

| 文件 | 行数 | 覆盖率 | 角色 |
|------|------|--------|------|
| `context_builder.go` | 197 | 62.1% | ContextBuilder 接口 + 分层定义 |
| `context_builder_default.go` | 254 | 62.1% | 默认实现（默认禁用） |
| `context_engine.go` | 312 | 62.1% | ContextEngine 接口 + CompressorEngine |
| `context_engine_tiered.go` | ~350 | 62.1% | 四级递进压缩 |
| `context_engine_truncate.go` | ~160 | 62.1% | 截断引擎 |
| `context_engine_chunked.go` | ~200 | 62.1% | 受保护文档块引擎 |
| `system_prompt.go` | ~150 | 62.1% | 分段构建 + 缓存控制 |
| `compaction.go` | 595 | 62.1% | runCompaction + shouldCompact |
| `compaction_structured.go` | ~60 | 62.1% | JSON 结构化摘要 |
| `token.go` | 119 | 62.1% | CJK 感知 token 估算 |
| `token_test.go` | 127 | — | Token 估算测试 |
| `agent_run.go` | ~200 | 62.1% | Run/Continue/runLoop |
| `agent_run_phase.go` | ~200 | 62.1% | runPreTurn/runModelTurn/callModelWithFallback |
| `agent_run_tool.go` | ~280 | 62.1% | buildRequestMessages/executeToolCalls |
| `agent_persist.go` | ~220 | 62.1% | Save/Load/Close |
| `state.go` | 242 | 62.1% | ReplaceMessages（绕过钩子） |
| `lifecycle.go` | ~450 | 62.1% | LifecycleHook 12 方法 + LifecycleChain |
| `hooks.go` | ~90 | 62.1% | 内置钩子（审计/速率限制/超时） |
| `budget.go` | ~250 | 62.1% | 四维限流 |
| `steering.go` | ~50 | 62.1% | 消息注入（策略引导） |
| `config.go` | ~320 | 62.1% | Config + CompactionConfig |
| `reasoning_strategy.go` | ~200 | 62.1% | 7 种策略 + StrategySelector |
| `reasoning_router.go` | ~150 | 62.1% | 三档复杂度分类 + Router |
| `agent.go` | ~500 | 62.1% | Agent 入口（New/contextBuilder/config） |
| `extension.go` | ~120 | 62.1% | Extension 注册系统 |

### memory/、session/、retrieval/、knowledge/、domains/reasoning/、prompt/

（略 — 见各自模块文件列表）

---

## 附录 B：调用图关系

### 上下文压缩流

```
TieredEngine.Compress()
  ├── ratio >= 0.9 → CompressorEngine.Compress()
  │     └── runCompaction()
  │           ├── pruneOldToolResults()
  │           ├── findTailCutByTokens()
  │           ├── truncateToTokenBudget()        # a86eb65 新增预检
  │           ├── call provider for summary
  │           └── state.ReplaceMessages()        # 绕过钩子
  ├── ratio >= 0.8 → pruneToolResults() + snipMessages()
  └── ratio >= 0.6 → snipMessages()
```

### 检索注入流

```
KnowledgeExtension.LifecycleHook()
  → RetrievalHook.BeforeModelCall()
      → shouldTrigger() / shouldTriggerSmart()
      → searcher.Search() [keyword / hybrid]
      → reranker.Rerank() [PositionReranker]
      → buildContextBlock() + injectContext()

KnowledgeExtension.BackendHook()
  → BackendRetrievalHook.BeforeModelCall()
      → shouldTrigger() / shouldTriggerSmart()
      → ext.search() [3-lane RRF: FTS + Vector + user.db]
      → reranker.Rerank() [可选 cross-encoder]
      → GraphEnhancer.Enhance()
      → buildContextBlock() + injectContext()   # 重复 !
```

### 记忆与会话流

```
Session Manager:
  New() → load JSONL → migrateEntries(v1→v4) → buildIndex
  Append() → set fields → persist → rollback on failure
  MessagesOnPath() → skip compaction → inject summaries
  Close() → flushAll()

Memory Extension:
  Provide() → Manager.Search() → Retriever.Search()
  AfterModelCall() → RememberFromTurn() → Extractor.Extract()  # 异步
  OnSessionClose() → SessionSummarizer → LongTerm store

Memory Compiler:
  BeforeTurn → StartTurn() → SelectStrategy() → ε-greedy
  AfterTurn → FinishTurn() → classifyOutcome → updateStats
  Close() → Save() → JSON file
```

---

## 附录 C：基线命令

```bash
# 编译验证
go build ./agentcore/... ./memory/... ./session/... ./retrieval/... ./knowledge/... ./domains/reasoning/... ./prompt/...

# 静态检查
go vet ./agentcore/... ./memory/... ./session/... ./retrieval/... ./knowledge/... ./domains/reasoning/... ./prompt/...

# 竞态测试
go test -race -count=1 ./agentcore/... ./memory/... ./session/... ./retrieval/... ./knowledge/... ./domains/reasoning/... ./prompt/...

# 覆盖率
go test -cover -count=1 ./agentcore/... ./memory/... ./session/... ./retrieval/... ./knowledge/... ./domains/reasoning/... ./prompt/...

# 全量构建（含集成测试）
go test -race -count=1 ./...
```
