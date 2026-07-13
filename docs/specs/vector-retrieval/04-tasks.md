# 04 — 任务拆解：向量检索落地

- **功能名**：vector-retrieval
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **拆解日期**：2026-07-13
- **状态**：阶段1已完成 ✅ | 阶段2全部完成 ✅ | 阶段3全部完成 ✅ | 端到端验证通过 ✅
- **依赖设计**：[03-design.md](./03-design.md)

> 每个任务标注：**涉及文件范围**、**验收**、**风险等级**、**审查要求**。
> 遵循 AGENTS.md「单次改动 3-5 文件」「小炸弹不是大炸弹」原则，任务粒度对应一次提交。

---

## 阶段 1：接线 MVP（让现有代码跑起来）✅ 已完成

**阶段目标**：setupFrameworkContext 装配 SQLiteStore + APIEmbedder，FTS+Vector RRF 默认生效。

### T1.1 — 新增 buildEmbedder 装配函数

- **文件**：`cmd/mady/main.go`（新增函数，~30 行）
- **内容**：
  - 读取 `OMLX_BASE_URL`/`OMLX_API_KEY`/`OMLX_EMBED_MODEL` 环境变量
  - `retrieval.NewAPIEmbedder(...)` 构造
  - `/health` 探测：失败返回 `(nil, warn)`，不报错
  - 返回 `(embedder retrieval.Embedder, err error)`
- **验收**：oMLX 未启动返回 nil 不 panic；启动时 stderr 打印 `[knowledge] embedder: oMLX bge-m3-mlx-8bit (1024d)` 或降级提示
- **风险**：低 | **审查**：L1

### T1.2 — 新增 loadKnowledgeBackend 装配函数

- **文件**：`cmd/mady/main.go`（新增函数，~40 行）
- **内容**：
  - 解析 `KNOWLEDGE_DB_DIR`（默认 `$MADY_HOME/knowledge`，复用 `util.ResolveDataDir`）
  - 存在性检查 `knowledge.db`：不存在返回 `(nil, nil)`
  - `sqlite.NewSQLiteStore(path)`；可选 `.OpenLawsDB`/`.OpenPatentKGdb`
  - 维度校验：`store.EmbeddingDim() == embedder.Dimensions()`，不一致返回错误并跳过
  - 返回 `(backend knowledge.KnowledgeBackend, closer func(), err error)`
- **验收**：缺库优雅跳过；维度不一致跳过并 WARN
- **风险**：低（只读打开）| **审查**：L2

### T1.3 — 改造 loadWikiStore → 接线 KnowledgeExtension

- **文件**：`cmd/mady/main.go`（重构 `loadWikiStore`，~50 行改动）
- **内容**：
  - 新函数 `loadKnowledgeHook(embedder)`：
    1. `backend = loadKnowledgeBackend(embedder)`
    2. 若 backend != nil：`ext := knowledge.NewExtension(store).WithBackend(backend, embedder)`；配置 `ExposeTool:true`；返回 `ext.LifecycleHook()`
    3. 否则回退原 `loadWikiStore()` 内存模式（WIKI_PATH）
  - `setupFrameworkContext` 调用顺序：先 `buildEmbedder` → `loadKnowledgeHook(embedder)` → 填充 `fc.WikiHook`
- **验收**：AC-1（search_knowledge 返回 RRF 结果）；AC-3（无库降级）
- **风险**：低 | **审查**：L2（触及入口装配）

### T1.4 — 新增环境变量与文档同步

- **文件**：`.env.example`（新增 8 个变量说明）、`README.md`（环境变量表 + 发展路线更新）、`docs/knowledge.md`（架构图补 oMLX）
- **验收**：README 环境变量表含全部新变量；.env.example 有注释
- **风险**：低（纯文档）| **审查**：L1

### T1.5 — 阶段1 集成测试 + benchmark 基线

- **文件**：`knowledge/ext_backend_test.go`（新）、`cmd/mady/main_test.go`（新增装配测试）
- **内容**：
  - 用小规模测试 SQLite 库（testdata/）验证 backendSearch 三路（此时 user 路为空）
  - 降级链路测试：embedder=nil → 仅 FTS；backend=nil → 内存模式
  - benchmark：VectorSearch 暴力扫描 p95（作为阶段2 对比基线）
- **验收**：`go test -race ./knowledge/... ./cmd/...` 全绿
- **风险**：低 | **审查**：L1

**阶段1 出口标准**：AC-1/AC-2/AC-3 通过；三入口冒烟一致；现有测试全绿。✅ 已验证（go build/vet/test -race 全绿 + 端到端 `mady serve` 确认 SQLite backend active）

---

## 阶段 2：性能（IVF 索引 + Cross-encoder 重排）✅ T2.1-T2.4 已完成

**阶段目标**：向量查询 < 50ms（IVF）；Top-K 经模型重排。

### T2.1 — IVF BLOB 格式探查（前置调研）✅

- **结论**：`ivf_index` 表为空（0行），无预建 IVF 索引可复用。决策走 T2.2' 暴力优化路线。

- **文件**：`knowledge/sqlite/ivf_probe.go`（临时探查脚本/测试，可选保留为工具）
- **内容**：
  - 读取 `ivf_index.index_data` BLOB
  - 探查序列化格式（magic number / pickle / numpy / 自定义）
  - 参考 XiaoNuo 构建 IVF 的 Python 代码 [NEEDS CLARIFICATION: 需 Owner 提供 XiaoNuo ivf 构建脚本路径]
  - 输出：格式说明 + 纯 Go 解析可行性评估
- **验收**：产出探查报告（写入本 spec 目录 `ivf-format.md` 或 design 补充）
- **风险**：中（可能结论是"无法解析"）| **审查**：L2
- **分支**：
  - 可解析 → T2.2 实现 IVFSearch
  - 不可解析 → 转 T2.2'（暴力优化）+ 记录 HNSW 为后续候选

### T2.2 — 实现 IVFSearch（若 T2.1 可解析）

- **文件**：`knowledge/sqlite/store.go`（新增方法，~120 行）、`knowledge/sqlite/ivf.go`（新增，BLOB 解析 + 聚类查询，~150 行）
- **内容**：
  - `ivf.go`：`parseIVFIndex(blob) (*ivfIndex, error)`、`(*ivfIndex).search(queryVec, nprobe) []chunkID`
  - `store.go`：`IVFSearch(queryVec, topK, nprobe)`；解析失败返回 `ErrNoIVFIndex`
  - 缓存解析结果（启动一次，复用）
- **验收**：AC-4（IVF p95 < 50ms）；与 VectorSearch 结果 recall@10 对比 > 0.9
- **风险**：中（算法正确性）| **审查**：L3

### T2.2' — 暴力查询优化（若 T2.1 不可解析，备选）✅

- **文件**：`knowledge/sqlite/vector_index.go`（新增，~150行）、`knowledge/sqlite/store.go`（修改）、`cmd/mady/main.go`（修改）
- **实际实现**：
  - `VectorIndex` 类型：扁平 `[]float32`（N×dim 连续内存）+ chunkIDs + docIDs
  - `PreloadVectorIndex()`：一次性全量加载 144K 向量（`unsafe.Slice` 零拷贝 BLOB→float32）
  - `Search(queryVec, topK)`：并行 goroutine 分片计算点积（利用归一化跳过除法），合并排序
  - `VectorSearch` 改造：`vecIndex != nil` 走 `vectorSearchInMemory` 快速路径，否则回退 SQL 批量
- **验收**：p95 < 500ms（非目标 50ms，但可接受兜底）
- **风险**：中 | **审查**：L2

### T2.3 — 新增 ModelReranker（cross-encoder 重排）✅

- **文件**：`retrieval/model_rerank.go`（新增，~200行）、`retrieval/model_rerank_test.go`（新增，8个测试）
- **实际实现**：
  - `QueryReranker` 接口（扩展 `Reranker`，新增 `RerankWithQuery(ctx, query, results)`）
  - `ModelReranker` 类型：Cohere 兼容 `/v1/rerank` 端点，支持 `MaxDocuments`(默认20) + `TopN` + 降级
  - `Rerank` 无 query 时 no-op；`RerankWithQuery` 调 API 重打分，API 错误返回原结果
- **验收**：AC-5（Top-3 命中）；单元测试 mock oMLX 响应
- **风险**：低 | **审查**：L2

### T2.4 — backendSearch 接入 Rerank ✅

- **文件**：`knowledge/extension.go`（改 `backendSearch` + 新增 `WithReranker`）、`cmd/mady/main.go`（新增 `buildReranker` + 接入）、`knowledge/backend_hook_test.go`（新增 `TestBackendHook_RerankerApplied`）
- **实际实现**：
  - `KnowledgeExtension` 新增 `queryReranker` 字段 + `WithReranker()` 方法
  - `backendSearch`：RRF 融合 candidateK 个候选 → `RerankWithQuery` → 截取 topK
  - `buildReranker()`：读 `KNOWLEDGE_RERANK`(on/true/1) + `OMLX_RERANK_MODEL` → `NewModelReranker`
  - 无 reranker 时走原路径 `fuser.Fuse(lists, topK)`
- **验收**：端到端 rerank 生效；可 `KNOWLEDGE_RERANK=0` 关闭
- **风险**：中（触及 extension 核心路径）| **审查**：L3

### T2.5 — 阶段2 benchmark + 评测基线 ✅

- **文件**：`knowledge/sqlite/bench_test.go`（新）、`knowledge/bench_test.go`（新）、`knowledge/sqlite/store.go`（改：SampleVector）、`knowledge/extension.go`（改：Search 导出方法）、`docs/specs/vector-retrieval/benchmark-baseline.md`（新）
- **内容**：
  - 底层 benchmark：PreloadVectorIndex / FTSSearch / VectorIndexSearch / VectorSearchInMemory / VectorSearchSQL / GetChunk
  - 端到端 benchmark：BackendSearch（FTS+Embed+Vector+RRF）/ RRFFusion
  - 基线文档：性能预算对比（内存版 15.2ms < 50ms 预算 ✅ / 端到端 29.8ms < 500ms 预算 ✅ / RRF 4.6μs）
  - 关键发现：内存版 vs SQL 版 87x 加速；预加载 251ms 在 17 次查询后摊销；并行效率 ~14x（M4 Pro 14核）
- **验收**：产出基线数据；性能预算全部达标 ✅
- **风险**：低 | **审查**：L1

**阶段2 出口标准**：AC-4/AC-5 通过；rerank 可开关；降级链路完整。✅ 阶段2全部完成

---

## 阶段 3：写入侧（用户文档向量化）✅ 已完成

**阶段目标**：用户文档 → user.db → 参与检索。

### T3.1 — WritableStore 实现 ✅

- **文件**：`knowledge/sqlite/writable.go`（新增，~310 行）、`knowledge/sqlite/writable_test.go`（新增，11个测试）
- **实际实现**：
  - `WritableStore` struct：`db`/`dim`/`embedder`/`mu`(sync.Mutex)
  - `OpenWritable(path, embedder, knowledgeDBPath)`：WAL 模式打开/创建 user.db，建表（documents/chunks/embeddings/docs_fts，同 knowledge.db schema），路径冲突检测（`filepath.Abs` 比对，拒绝指向 knowledge.db → `ErrKnowledgeDBConflict`）
  - `AddDocument(ctx, docID, title, content)`：`retrieval.ChunkDocument` 分块 → 批量 `Embed`(batch=32) → 事务写入（DELETE 旧 + INSERT documents/chunks/embeddings/docs_fts）
  - `Search(ctx, query, topK)`：FTS(`ftsSearch`) + Vector(`vectorSearch` 暴力余弦) → `RRFFuser.Fuse`
  - `float32ToBytes`/`vecNorm`/`hashString`(FNV-1a) 辅助函数
  - `Close()`/`Dim()` 方法
- **验收**：单元测试 11 个全绿（创建/FTS命中/无匹配/替换/路径冲突/nil embedder/空docID/并发写-race/schema幂等/hash/BLOB往返）；并发写安全 ✅
- **风险**：中（写入沙箱）| **审查**：L3 ✅（路径冲突检测 + WAL + Mutex + 参数化查询）

### T3.2 — backendSearch 接入 user.db 三路融合 ✅

- **文件**：`knowledge/extension.go`（修改，~40 行）
- **实际实现**：
  - 新增 `WritableBackend` 接口（`Search(ctx, query, topK)` + `AddDocument(ctx, docID, title, content)`），领域层不 import sqlite
  - `KnowledgeExtension` 新增 `writable WritableBackend` 字段 + `WithWritableStore()` 方法
  - `backendSearch` 新增第三路：`e.writable.Search(ctx, query, candidateK)`，与 knowledge FTS / knowledge Vector 一起 `RRFFuser.Fuse`
  - reranker 对三路合并后的 Top-K 生效（逻辑不变，只是 lists 多一路）
- **验收**：三路 RRF 测试（`TestExtension_ThreeLaneRRF`）验证 knowledge + user 结果均出现 ✅
- **风险**：中 | **审查**：L2 ✅

### T3.3 — 装配 + 工具暴露 ✅

- **文件**：`cmd/mady/main.go`（修改，~30 行）、`knowledge/extension.go`（修改，~30 行）
- **实际实现**：
  - `loadKnowledgeBackend` 改返回 `(KnowledgeBackend, string)` 附带 knowledgeDBPath
  - 新增 `openWritableStore(madyHome, embedder, knowledgeDBPath)`：读 `USER_DB_PATH`(默认 $MADY_HOME/knowledge/user.db) → `os.MkdirAll` 建目录 → `sqlite.OpenWritable`（路径冲突检测内置）→ 失败打印警告不阻断
  - `loadWikiStore` 中 `ext.WithWritableStore(ws)` 注入
  - `Tools()` 条件性暴露 `add_document`（writable!=nil 时）：`{doc_id, title, content}` → `handleAddDocument` → `writable.AddDocument`
  - 参数校验：空 doc_id / 空 content 返回提示
- **验收**：TUI 中调用 add_document 后 search_knowledge 命中 ✅（集成测试 `TestExtension_AddDocumentThenSearch`）
- **风险**：中（新增工具 + 写入）| **审查**：L3 ✅

### T3.4 — 阶段3 集成测试 + 文档 ✅

- **文件**：`knowledge/ext_writable_test.go`（新，package knowledge_test）、`.env.example`、`README.md`、`AI_CHANGELOG.md`、`04-tasks.md`
- **实际实现**：
  - 4 个集成测试：`TestExtension_AddDocumentToolExposed`（工具暴露条件）/ `TestExtension_AddDocumentThenSearch`（端到端 add→search 命中）/ `TestExtension_ThreeLaneRRF`（三路融合）/ `TestExtension_AddDocumentValidation`（参数校验）
  - mockBackend + mockEmbedder + realWritableStore 混合测试策略
  - 环境变量文档更新（USER_DB_PATH）
- **验收**：AC-6/AC-7 通过；go build/vet/test -race 全绿 ✅
- **风险**：低 | **审查**：L1 ✅

**阶段3 出口标准**：AC-6 通过；写入并发安全；user.db 与 knowledge.db 隔离。✅ 全部验证通过

---

## 横切任务（贯穿各阶段）

### X1 — AI_CHANGELOG 同步

- 每个任务完成后在 `docs/decisions/AI_CHANGELOG.md` 追加一条（AGENTS.md「变更即记录」）
- 阶段出口更新 `README.md` 发展路线、`CHANGELOG.md`

### X2 — lint/vet/test 全绿

- 每次提交前：`go build ./... && go vet ./... && go test -race ./... && golangci-lint run`
- `tools/` 子模块单独验证（go.work 多模块 gotcha）

---

## 依赖与顺序

```
T1.1 ─┐
T1.2 ─┼─→ T1.3 ──→ T1.4 ──→ T1.5 ──→ [阶段1 出口]
      │
      └──────────────────────→ T2.1 ──→ T2.2 (或 T2.2')
                                          │
                       T2.3 ──→ T2.4 ←────┘
                                          │
                                    T2.5 → [阶段2 出口]
                                          │
                                    T3.1 → T3.2 → T3.3 → T3.4 → [阶段3 出口]
```

- T1.1/T1.2 可并行；T2.1 是阶段2 前置；T2.3 可与 T2.1/T2.2 并行。
- 每个阶段出口需人工 Sign-off 后再进入下一阶段。

---

## 工作量估算

| 阶段 | 任务数 | 新增/改动文件 | 新增代码量 | 估算 |
|------|--------|--------------|-----------|------|
| 阶段1 | 5 | ~6 | ~200 行 | 0.5-1 天 |
| 阶段2 | 5 | ~7 | ~500 行 | 1.5-2.5 天（T2.1 探查不定） |
| 阶段3 | 4 | ~6 | ~300 行 | 1-1.5 天 |
| **合计** | 14 | ~19 | ~1000 行 | 3-5 天 |

> 估算含测试与文档，不含 IVF 格式不可解析时的 HNSW 备选方案（另计 1-2 天）。
