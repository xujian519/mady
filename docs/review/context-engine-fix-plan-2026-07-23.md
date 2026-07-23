# 上下文构建引擎 — 根因分析 & 修复计划

| 字段 | 值 |
|------|------|
| 基准报告 | `context-engine-full-review-2026-07-23.md` |
| 总发现 | 40 项（P0×2, P1×16, P2×15, P3×7） |
| 本文档 | 根因聚类 + 修复工作流 + 任务拆解 + 验证清单 |

---

## 一、根因分析

40 项发现可聚类为 **5 大根因簇**，每个簇对应一类系统性成因和一组修复策略。

### 簇 A: 架构演进中的设计缺口（6 项）

**根因：** 功能迭代快于架构抽象层的提取，导致接口不一致、路径重复、设计未完成。

| 发现 | 根因描述 |
|------|---------|
| **RTV-001** P0 | `BackendRetrievalHook` 以"避免修改 retrieval 包"为由独立实现，但共享格式化/注入逻辑未提取为公共工具函数 |
| **RTV-005** P1 | `KnowledgeExtension` 同时暴露 `LifecycleHook()` 和 `BackendHook()` 两个提供者，标准 Agent 初始化无法自动选择 |
| **MEM-008** P2 | Memory Extension 同时实现 `LayerProvider`（ContextBuilder 路径）和 `TransformContextProvider`（传统路径），路径选择不明确 |
| **CMP-004** P1 | `Compress()` 接口未定义返回值语义契约，CompressorEngine 返回消息位置而 TieredEngine 返回 token 节省数 |
| **CMP-003** P1 | `compressionBaseURL/compressionAPIKey` 作为"未来扩展"预留字段但从未被消费，成为死配置 |
| **ContextBuilder 禁用** P1 | 5 层分层架构设计完成但未激活，`Enabled: false` + `nil` builder 双重锁定 |

**修复策略：** 提取公共职责 → 统一接口契约 → 消除未完成的设计债务。

### 簇 B: 实现级逻辑缺陷（7 项）

**根因：** 代码拆分时逻辑归属不清晰、算法选择失误、边界条件遗漏。

| 发现 | 根因描述 |
|------|---------|
| **CMP-001** P1 | `ineffectiveCompactions` 计数逻辑拆分在 `runCompaction` 和 `CompressorEngine.Compress` 两处，各增一次导致一次低效即触发冷却 |
| **RTV-008** P1 | 引用链扩展中 `APPLIES` 边方向误用：从法律条文出发的出站边会引入无关文档 |
| **RTV-007** P1 | `MaxChars` 标头开销 `+100` 未经校准，基于经验值而非实际计算 |
| **RTV-010** P1 | `indexOfItem` 线性搜索嵌套形成 O(n²) |
| **CMP-005** P2 | `TieredEngine.ShouldCompact` 独立实现且缺少对断路器冷却状态的检查 |
| **CMP-006** P2 | `pruneOldToolResults` 尾部保护用固定消息计数而非基于 token 预算，与管线其他部分不一致 |
| **CMP-008** P2 | `thresholdTokens` 字段声明但从未赋值，为拷贝残留代码 |

**修复策略：** 归并重复职责 → 修正算法 → 补齐边界条件。

### 簇 C: 并发与数据完整性缺口（5 项）

**根因：** 并发原语使用不当、原子性操作假设不成立、数据迁移有损。

| 发现 | 根因描述 |
|------|---------|
| **RTV-002** P0 | `byDomain` 的 `csync.Map.Get+append+Set` 非原子，两个 goroutine 可同时读同一个 `[]string` 然后写回，丢失其中一个追加 |
| **RTV-006** P1 | `ReindexVectors` 同理：`chunks.Get` → 修改 → `chunks.Set` 是竞态窗口 |
| **EngineRegistry** P1 | `factories map` 被 `Register/Create/List` 并发访问时无任何同步保护 |
| **MEM-003** P2 | v2→v3 迁移移除了 `firstKeptEntryIndex` 但未写入替代字段 `first_kept_entry_id` |
| **MEM-006** P2 | Compiler.Save 直接用 `os.WriteFile` 覆写，无原子写入保护 |

**修复策略：** 加锁包裹 RMW 模式 → 迁移逻辑补全 → 原子写入模式。

### 簇 D: 安全与隐私缺口（4 项）

**根因：** LLM 数据出站路径缺少防护层、错误传播可能含敏感内容、取消信号未传播。

| 发现 | 根因描述 |
|------|---------|
| **MEM-001** P1 | Memory Extractor 将对话全文（含可能密码/API Key/PII）直接发送给 LLM Provider |
| **MEM-009** P2 | `fmt.Errorf` 包装原始提取错误时，错误可能包含敏感对话内容 |
| **RTV-003** P1 | `RetrievalHook.BeforeModelCall` 丢弃 `ctx`，Embedding 搜索使用 `context.Background()`，取消信号丢失 |
| **RTV-009** P1 | ModelReranker 在所有错误情况下静默返回原始结果，配置错误完全不可见 |

**修复策略：** 添加出站过滤 → 错误清理 → 上下文传播 → 可观测性日志。

### 簇 E: 测试覆盖缺口（3 项 — 模块级）

**根因：** 持久化关键路径测试投入不足、异步处理分支缺乏覆盖。

| 模块 | 当前比例 | 根因 |
|------|---------|------|
| `session` | 0.40 | Manager 14+ 个导出方法仅 2 个测试文件覆盖基础操作 |
| `memory` | 0.50 | LLM Extractor/SessionSummarizer 分支（异步、错误路径）未覆盖 |
| `knowledge` 核心 | 57.1% | Extension 生命周期和并发文档加载缺乏测试 |

**修复策略：** 逐模块补齐测试，以关键路径为优先。

---

## 二、修复工作流（按批次）

### 批次 1: 安全与并发急诊（P0 + 安全 P1）— 预估 3 天

| # | 文件 | 修复内容 | 工作量 |
|---|------|---------|--------|
| 1.1 | `knowledge/store.go:75-77` | byDomain RMW 加锁包裹 | 0.5d |
| 1.2 | `knowledge/store.go:265-283` | ReindexVectors RMW 加锁包裹 | 0.5d |
| 1.3 | `memory/extractor_llm.go:35-65` | 添加 `sensitiveDataFilter()` 预过滤 | 1d |
| 1.4 | `memory/manager.go:133` | 截断或清理错误中的对话文本 | 0.5d |
| 1.5 | `retrieval/agent.go:157` | BeforeModelCall 传播 ctx | 0.5d |

**可验证成果：**
- `go test -race` 全部通过
- 敏感模式（password=, api_key=, secret=, token=）在 Memory Extractor 中被屏蔽
- RetrievalHook 嵌入搜索支持 `ctx.Done()` 取消

### 批次 2: 架构债务消除（P1 架构 + 逻辑缺陷）— 预估 6 天

| # | 文件 | 修复内容 | 工作量 |
|---|------|---------|--------|
| 2.1 | `retrieval/` + `knowledge/backend_hook.go` | 提取 `FormatContextBlock`、`InjectRetrievalContext`、`ShouldTrigger` 到 `retrieval` 包 | 2d |
| 2.2 | `knowledge/extension.go` | 统一 Hook 提供者（`LifecycleHook()` 根据 backend 存在选择） | 1d |
| 2.3 | `compaction.go:532` + `context_engine.go:259` | 删除 `CompressorEngine.Compress` 中的冗余计数（保持 `runCompaction` 为唯一标准位置） | 0.5d |
| 2.4 | `context_engine.go:167-168` + 新方法 | 实现 `compressionProvider()` 利用 `compressionBaseURL/APIKey` | 1d |
| 2.5 | 所有 ContextEngine | Compress 返回值统一为 token 节省语义 | 0.5d |
| 2.6 | `knowledge/graph/retrieval_enhancer.go:123-130` | 修正 APPLIES 边方向过滤 | 0.5d |
| 2.7 | `agentcore/context_engine.go:91` + 适配器 | EngineRegistry 添加 `sync.RWMutex` | 0.5d |

### 批次 3: 完整性补强（P1 剩余 + P2 高优先级）— 预估 5 天

| # | 文件 | 修复内容 | 工作量 |
|---|------|---------|--------|
| 3.1 | `compaction.go:521` | 实现 `BeforeCompactionPersist` / `AfterCompactionPersist` 钩子对 | 1.5d |
| 3.2 | `memory/compiler/learning.go:116-132` | Compiler 切换器改为 per-strategy（Strategy 结构体加两个 bool）或 `rand.Float64()>0.5` | 1d |
| 3.3 | `memory/compiler/learning.go:232-239` | Compiler.Save 改为临时文件+重命名模式 | 0.5d |
| 3.4 | `retrieval/agent.go:250` | MaxChars 标头开销从 `+100` 校准为 `+76` 或基于实际计算 | 0.5d |
| 3.5 | `context_engine_tiered.go:115-122` | TieredEngine.ShouldCompact 增加断路器冷却检查 | 0.5d |
| 3.6 | `retrieval/model_rerank.go:98` | 错误路径添加 `slog.Warn` 日志 | 0.5d |
| 3.7 | `session/session_store.go:135-149` | v2→v3 迁移补写 `first_kept_entry_id` | 0.5d |

### 批次 4: 技术债务清理（P2 剩余 + P3）— 预估 4 天

| # | 文件 | 修复内容 | 工作量 |
|---|------|---------|--------|
| 4.1 | `compaction.go:175-230` | `pruneOldToolResults` 改为基于 token 的尾部边界 | 1d |
| 4.2 | `retrieval/citation.go:156-163` | `FormatCitationChain` 优化（建 map 替代线性搜索） | 0.5d |
| 4.3 | `context_engine_tiered.go:35` | 移除 `thresholdTokens` 死字段 | 0.25d |
| 4.4 | `knowledge/extension.go:476-478` | memorySearch 缓存无状态搜索器/重排器 | 0.5d |
| 4.5 | `knowledge/store.go` | 添加 `RemoveDocument` API | 1d |
| 4.6 | `retrieval/agent.go` | injectContext 添加去重检查 | 0.5d |
| 4.7 | `context_engine_chunked.go:147` | 系统提示位置重构（按类型而非按索引标识） | 0.5d |
| 4.8 | P3 各项（CMP-009~012, RTV-018~020, MEM-010~013） | 低优先级代码美化 | 1d |

### 批次 5: 测试覆盖补强 — 预估 5 天

| # | 模块 | 新增测试 | 工作量 |
|---|------|---------|--------|
| 5.1 | `session` | Manager Append rollback、版本迁移 v1→v4、Branch、压缩感知 MessagesOnPath | 2d |
| 5.2 | `memory` | Extractor 边界+错误路径、Compile 统计验证、SQLite store 并发写入 | 1.5d |
| 5.3 | `knowledge` 核心 | Extension 生命周期（Init→Layer→Hook→Dispose）、并发文档加载 | 1d |
| 5.4 | `agentcore` | Compaction 断路器逻辑新增测试、TieredEngine.ShouldCompact 冷却测试 | 0.5d |

### 批次 6: ContextBuilder 启用（中长期规划）— 预估 5 天

| # | 步骤 | 内容 | 工作量 |
|---|------|------|--------|
| 6.1 | 阶段 1 — 功能验证 | 编写集成测试覆盖 ContextBuilder 5 层组装路径，验证 token 预算截断 | 2d |
| 6.2 | 阶段 2 — A/B 测试 | 添加 `WithContextBuilder` 配置开关，在测试环境中启用对比 TransformContext | 1d |
| 6.3 | 阶段 3 — 默认启用 | `Enabled: true` + 清理 TransformContext 回退路径 | 2d |

---

## 三、任务依赖关系图

```
批次 1（安全急诊）──┐
                    ├──→ 批次 3（完整性补强）──→ 批次 4（技术债清理）──→ 批次 5（测试补强）
批次 2（架构债务）──┘                                      ↑
                                                    批次 6（ContextBuilder启用）
```

- 批次 1/2 是 P0+P1 修复，**建议在当前 sprint 完成**
- 批次 3/4/5 是 P1 剩余 + P2，**建议下一 sprint 完成**
- 批次 6 是中长期规划，**独立排期**

---

## 四、可执行任务清单

### 阶段 1: 安全与并发急诊

- [ ] **1.1** `knowledge/store.go` — 在 `AddDocument()` 的 byDomain 写入周围加锁
  - 具体：提取 `s.byDomain.Get` + `append` + `s.byDomain.Set` 为 `s.addToDomainLocked(domain, docID)`（内部用 `sync.Mutex`）
- [ ] **1.2** `knowledge/store.go` — `ReindexVectors()` 的 `Get`+修改+`Set` 加锁
  - 具体：用同一个 `sync.Mutex` 保护 `ReindexVectors` 的主体
- [ ] **1.3** `memory/extractor_llm.go` — 新增 `sensitiveDataFilter(text string) string` 函数
  - 正则匹配: `(?i)(password|api[_-]?key|secret|token|bearer)\s*[:=]\s*\S+` → 替换为 `***`
  - 在 `extractWithLLM` 中将 `conversation` 传入 filter 后再拼接 prompt
- [ ] **1.4** `memory/manager.go:133` — 错误包装截断
  - 将 `fmt.Errorf("memory: extract failed ...: %w", err)` 改为截断错误消息至 200 字符
- [ ] **1.5** `retrieval/agent.go:157` — `BeforeModelCall(_ context.Context, ...)` → `BeforeModelCall(ctx context.Context, ...)`
  - 将 `ctx` 传递给 `h.searcher.Search(ctx, ...)`
  - `HybridSearcher.Search` 中的 `context.Background()` 改为入参 `ctx`

### 阶段 2: 架构债务消除

- [ ] **2.1** 提取检索通用工具函数
  - 在 `retrieval/` 包中新增 `context_util.go`
  - 提取的函数：
    - `FormatContextBlock(results []ScoredChunk, cfg RetrievalConfig) string`
    - `InjectRetrievalContext(req *ProviderRequest, block string)`
    - `ShouldTrigger(policy TriggerPolicy, turnCount int, arc ...) bool`
    - `ShouldTriggerSmart(policy TriggerPolicy, turnCount int, ...) bool`
  - `RetrievalHook` 和 `BackendRetrievalHook` 改为调用这些函数
  - 删除 `knowledge/backend_hook.go` 中的重复实现（改为导入 `retrieval` 包）

- [ ] **2.2** `knowledge/extension.go` 统一 Hook 提供者
  - `LifecycleHook()` 方法内判断：如果 `e.backend != nil` 返回 BackendRetrievalHook，否则返回 RetrievalHook
  - 移除 `BackendHook()` 非标准方法

- [ ] **2.3** 修复断路器双重计数
  - `context_engine.go:257-263` 删除整个 `if displayTokens > 0 { ... }` 块
  - `runCompaction`（compaction.go:526-543）保留为唯一计数位置
  - 更新 `CompressorEngine.Compress` 中 `cut` 的计算确保不影响断路器

- [ ] **2.4** 实现 `compressionProvider()`
  - `CompressorEngine` 新增 `compressionProvider() Provider` 方法
  - 如果 `compressionBaseURL != ""` 且 `compressionAPIKey != ""`，构建新的 Provider 实例
  - 否则返回 `p.CompressionProvider`（如有）或 `p.Provider`
  - `runCompaction` 中用于构建摘要请求

- [ ] **2.5** 统一 Compress() 返回值
  - 接口契约：第二个 `int64` 返回值定义为 **"本次压缩节省的 token 数"**
  - `CompressorEngine.Compress()` 返回 `saved`（已计算，第 255 行）
  - `TieredEngine.Compress()` 返回 `newTokens - estimated`（需要计算）
  - `TruncateEngine.Compress()` 返回 `before - after`
  - `agent_persist.go:160` 事件发射的 `MessagesCut` 字段注释更新

- [ ] **2.6** 修正 APPLIES 边方向
  - `retrieval_enhancer.go:123-130` 改为：从法律条文出发，仅搜索 `CITES` 入边的文档，移除出站 `APPLIES` 搜索

- [ ] **2.7** EngineRegistry 加锁
  - `EngineRegistry` 结构体添加 `mu sync.RWMutex` 字段
  - `Register` — `Lock`
  - `Create` — `RLock`
  - `List` — `RLock`
  - `Default` — `RLock`

### 阶段 3: 完整性补强

- [ ] **3.1** 实现 Compaction Persist 钩子对
  - `lifecycle.go` 添加 `BeforeCompactionPersist(ctx, arc, msgs []Message) ([]Message, error)` 和 `AfterCompactionPersist(ctx, arc, msgs []Message)`
  - `LifecycleChain` 实现这两个方法的调用
  - `state.ReplaceMessages()` 内部触发这两个钩子
  - Guardrail/Evidence hooks 实现这两个接口以覆盖审计缺口

- [ ] **3.2** Compiler 切换器改为 per-strategy
  - `Strategy` 结构体添加 `successToggle, failureToggle bool` 字段
  - `FinishTurn` 更新时使用 `c.strategies[i].successToggle` / `c.strategies[i].failureToggle`
  - 移除 Compiler 结构体的 `successToggle / failureToggle` 全局字段

- [ ] **3.3** Compiler Save 原子化
  - 改为: 写入临时文件 → `os.Rename` → 旧的 `os.WriteFile` 路径

- [ ] **3.4** MaxChars 标头校准
  - 根据 `buildContextBlock` 的实际格式计算标头开销：
    ```
    --- 知识上下文 ---\n域: {domain}\n---\n{chunk}\n\n(与下一个 chunk 之间的开销)
    ```
  - 实际值约为 56-76 字符，改为动态计算或固定 `+80`

- [ ] **3.5** TieredEngine.ShouldCompact 冷却检查
  - 在 `context_engine_tiered.go:115-122` 的 `ShouldCompact` 中添加：
    ```go
    if e.compressor != nil && e.compressor.state != nil {
        if time.Now().Before(e.compressor.state.summaryFailureCooldown) { return false }
        if e.compressor.state.ineffectiveCompactions >= 2 &&
           time.Now().Before(e.compressor.state.ineffectiveCooldownUntil) { return false }
    }
    ```

- [ ] **3.6** ModelReranker 错误日志
  - 在每个 `return results, nil` 错误路径前添加 `slog.Warn` 或 `slog.Error`

- [ ] **3.7** v2→v3 迁移补写 `first_kept_entry_id`
  - 在 `migrateV2ToV3` 中，移除 `firstKeptEntryIndex` 的同时，根据 compaction entry 的第一个被压缩条目 ID 写入 `first_kept_entry_id`

### 阶段 4: 技术债务清理

- [ ] **4.1** `pruneOldToolResults` 改为基于 token 的尾部边界
  - 从 `int(keepRecentTokens / 100)` 改为调用 `findTailCutByTokens`
- [ ] **4.2** `FormatCitationChain` O(n²) 优化
  - 建 `map[string]int` 预计算索引，替代 `indexOfItem` 线性搜索
- [ ] **4.3** 移除 `thresholdTokens` 死字段
- [ ] **4.4** memorySearch 缓存搜索器/重排器
- [ ] **4.5** 添加 `RemoveDocument` API
- [ ] **4.6** injectContext 去重检查
- [ ] **4.7** ChunkedEngine 系统提示位置重构
- [ ] **4.8** 各项 P3 代码美化

### 阶段 5: 测试覆盖补强

- [ ] **5.1** session 新增测试文件
  - `session_test.go` — 测试 Manager 创建/加载/追加/回退/分支
  - `session_migration_test.go` — 测试 v1→v4 全链路迁移完整性
- [ ] **5.2** memory 新增测试
  - `extractor_test.go` — 测试边界条件（空输入、超长输入、敏感数据过滤）
  - `compiler_test.go` — 测试 per-strategy 切换器统计准确性
- [ ] **5.3** knowledge 新增测试
  - `extension_lifecycle_test.go` — Init → Layer → Hook → Dispose 全流程
- [ ] **5.4** agentcore 新增测试
  - `compaction_breaker_test.go` — 断路器在 `ineffectiveCompactions≥2` 时触发冷却的可测试性

### 阶段 6: ContextBuilder 启用

- [ ] **6.1** ContextBuilder 集成测试（阶段 1）
- [ ] **6.2** 配置开关 + A/B 测试（阶段 2）
- [ ] **6.3** 默认启用 + 清理回退路径（阶段 3）

---

## 五、可验证的检查清单

### 构建与测试

- [ ] `go build ./...` 零错误
- [ ] `go vet ./...` 零警告
- [ ] `go test -race -count=1 ./...` 全部通过
- [ ] `go test -cover` 覆盖率未下降

### 安全（批次 1.3-1.4）

- [ ] Memory Extractor 发送给 Provider 的 prompt 中不包含 `password=xxx`、`api_key=xxx` 等模式
- [ ] Extractor 错误消息不包含对话原始文本（截断 ≤200 字符）
- [ ] RetrievalHook 的 Embedding 搜索可被 ctx.Done() 取消

### 并发安全（批次 1.1-1.2, 2.7）

- [ ] `knowledge/store.go` 的 `AddDocument()` 并发调用 N 次，`byDomain` 的 docID 总数 = N
- [ ] `knowledge/store.go` 的 `ReindexVectors()` 并发调用不丢失嵌入
- [ ] EngineRegistry 并发 `Register` + `Create` 不 panic 不丢数据

### 架构债务（批次 2.1-2.2）

- [ ] `retrieval/` 包导出 `FormatContextBlock`、`InjectRetrievalContext`、`ShouldTrigger`
- [ ] `RetrievalHook` 和 `BackendRetrievalHook` 都导入并使用上述函数
- [ ] `knowledge/backend_hook.go` 中不再存在 `buildContextBlock` / `injectContext` 的独立实现
- [ ] `KnowledgeExtension` 不再暴露 `BackendHook()`（或已标记为废弃）  <!-- 这里是markdown注释，不是HTML注释 -->
- [ ] 已实现 `compressionProvider()`，配置 `CompressionBaseURL` 后压缩请求使用独立 Provider

### 断路器（批次 2.3, 3.5）

- [ ] `context_engine.go` 中不再递增 `ineffectiveCompactions`
- [ ] `runCompaction` 是 `ineffectiveCompactions` 唯一修改点
- [ ] TieredEngine.ShouldCompact 在冷却期间返回 false
- [ ] 一次低效压缩不触发冷却，连续两次才触发

### 记忆与编译器（批次 3.2-3.3, 4.3）

- [ ] Compiler 切换器 per-strategy：策略 A 的 medium 成功不影响策略 B 的统计
- [ ] Compiler Save 使用临时文件 + `os.Rename`
- [ ] Compiler `thresholdTokens` 死字段已移除

### 版本迁移（批次 3.7）

- [ ] v2→v3 迁移后 compaction 条目包含 `first_kept_entry_id`
- [ ] 迁移后 `MessagesOnPath()` 能正确跳过已压缩消息

### 测试覆盖（阶段 5）

| 模块 | 当前比例 | 目标比例 | 达成 |
|------|---------|---------|------|
| `session` | 0.40 | ≥0.70 | [ ] |
| `memory` | 0.50 | ≥0.65 | [ ] |
| `knowledge` 核心 | 57.1% | ≥70% | [ ] |

### ContextBuilder 启用（阶段 6）

- [ ] ContextBuilder 启用后，5 层消息组装与 TransformContext 输出一致
- [ ] `LayerProvider` 注册路径均正确触发 Provide()
- [ ] Token 预算截断在各层正确工作

---

## 六、工作量汇总

| 批次 | 描述 | 预估人天 |
|------|------|---------|
| 批次 1 | 安全与并发急诊 | 3 天 |
| 批次 2 | 架构债务消除 | 6 天 |
| 批次 3 | 完整性补强 | 5 天 |
| 批次 4 | 技术债务清理 | 4 天 |
| 批次 5 | 测试覆盖补强 | 5 天 |
| 批次 6 | ContextBuilder 启用 | 5 天 |
| **合计** | | **28 人天** |

**关键路径：** 批次 1 → 批次 3 → 批次 5（13 天）
**并行路径：** 批次 2 与批次 1 可并行执行（6 天）
**ContextBuilder 启用**可独立排期，不影响其他批次。
