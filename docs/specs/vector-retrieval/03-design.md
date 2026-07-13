# 03 — 设计：向量检索落地

- **功能名**：vector-retrieval
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **设计日期**：2026-07-13
- **状态**：待人工审阅
- **依赖规格**：[02-spec.md](./02-spec.md)

---

## 1. 技术选型

### 1.1 Embedding：本地 oMLX（BGE-M3）

| 项 | 决策 |
|----|------|
| 服务 | oMLX `127.0.0.1:8000`（已部署） |
| 模型 | `bge-m3-mlx-8bit`（与 knowledge.db 向量同源，1024维） |
| 协议 | OpenAI 兼容 `/v1/embeddings` |
| 复用 | `retrieval.APIEmbedder` 零改动直接指向 oMLX |

**为何不远程 API**：knowledge.db 的 144K 向量由 BGE-M3 生成，查询向量必须同模型同维度，否则相似度无意义。本地 oMLX 跑的就是 BGE-M3，天然对齐；远程 API 维度/模型漂移风险高。

**为何不引入新 Embedder 类型**：APIEmbedder 已是 OpenAI 兼容，oMLX 实测直接可用，零代码改动。

### 1.2 向量索引：复用预建 IVF

knowledge.db 的 `ivf_index` 表由 XiaoNuno 预建（`dim=1024, nlist=N, index_data BLOB`）。当前 `VectorSearch` 忽略它走全量暴力扫描（144K 向量，分批 2000 读，单查询数秒）。

| 方案 | 评估 |
|------|------|
| **A. 解析 ivf_index BLOB，实现 IVF 查询**（选） | 零构建成本，亚秒级；需逆向 BLOB 序列化格式 |
| B. 新建内存 HNSW（hnswlib-go） | 查询最快，但启动加载 144K 向量 ~600MB 内存，引入 CGO 依赖 |
| C. sqlite-vec 扩展 | 需平台二进制/CGO，与 modernc.org/sqlite（纯 Go）冲突 |
| D. 保持暴力 + SIMD 优化 | 零依赖，但性能上限受限 |

**决策**：阶段2 优先 A（解析 IVF）。若 BLOB 格式经逆向仍不透明（风险见 §6），**降级到 D**（暴力 + 并行批次 + 查询向量缓存），并评估 B 作为阶段 2.1 候选。

> IVF BLOB 格式需在实现阶段探查。XiaoNuo 用 Python（faiss 或自实现）构建，BLOB 可能是 pickle/numpy 序列化。Go 侧需纯 Go 解析器。**[NEEDS CLARIFICATION: 确认 XiaoNuo 构建 IVF 的代码位置，以获知序列化格式]**

### 1.3 融合：现有 RRF（零改动）

`retrieval.RRFFuser`（k=60）score-agnostic，天然适合融合 FTS（BM25 分）与 Vector（余弦分），无需归一化。阶段3 增 user.db 后变三路 RRF，算法不变。

### 1.4 重排：Cross-encoder（oMLX Qwen3-Reranker）

现有 Reranker 全是启发式（位置/去重/法律来源层级/领域元数据）。Cross-encoder 对 (query, doc) 做深度语义打分，显著提升 Top-K 精度。

**为何 Qwen3-Reranker**：oMLX 已加载 `Qwen3-Reranker-4B-4bit-MLX`，Cohere 兼容 `/v1/rerank`，实测可用。4B 量化模型在 M4 Pro 上延迟可接受（小批量 < 200ms）。

**重排位置**：RRF 融合 **之后**，取 Top-20 送重排，返回 Top-5。重排是可选的（`KNOWLEDGE_RERANK=0` 关闭）。

### 1.5 写入库：独立 user.db

| 方案 | 评估 |
|------|------|
| **A. 独立 user.db**（选） | 保护 knowledge.db 只读权威性；维度/模型独立可控 |
| B. 写入 knowledge.db | 污染权威数据，软链失效时丢失，违背只读约束 |

user.db schema 与 knowledge.db 同构（§2.1 spec），便于 `WritableStore.Search` 与 `KnowledgeBackend` 接口对齐。检索时三路（knowledge FTS + knowledge Vector + user Search）RRF 融合。

---

## 2. 架构

### 2.1 检索数据流（阶段1-3 完整）

```
用户 query
    │
    ▼
┌─────────────────────────────────────────────┐
│ KnowledgeExtension.search(ctx, query, topK) │
└──────────────────┬──────────────────────────┘
                   │ backend != nil ?
        ┌──────────┴──────────┐
        ▼ backendSearch       ▼ memorySearch (降级: WIKI_PATH 内存模式)
┌──────────────────────────────────────────────┐
│ ① FTS 路: backend.FTSSearch(query, 2·topK)   │  ← knowledge.db docs_fts (BM25)
│ ② Vector 路: embedder.Embed(query)           │
│        → IVFSearch(vec, 2·topK, nprobe)      │  ← 阶段2 (降级 VectorSearch)
│ ③ User 路: writable.Search(query, 2·topK)    │  ← 阶段3 user.db (FTS+Vector)
│                                              │
│   三路 → RRFFuser.Fuse(lists, topK)          │
│   → 启用 rerank: ModelReranker.Rerank(topK)   │  ← oMLX Qwen3-Reranker
└──────────────────┬───────────────────────────┘
                   ▼
        []ScoredChunk (topK, 已重排)
                   │
        ┌──────────┴──────────┐
        ▼ handleSearch        ▼ Provide (自动注入上下文)
   search_knowledge 工具      系统消息 "### 参考文档"
```

### 2.2 生产装配（阶段1 改动点）

```
setupFrameworkContext() (cmd/mady/main.go)
    │
    ├─ buildEmbedder()            [新增] 读取 OMLX_* 环境变量
    │     └─ retrieval.NewAPIEmbedder(OMLX_BASE_URL, OMLX_API_KEY, OMLX_EMBED_MODEL)
    │       → 健康探测 /health，失败返回 nil (降级纯 FTS)
    │
    ├─ loadKnowledgeBackend()     [新增] 替换/包装 loadWikiStore
    │     ├─ sqlite.NewSQLiteStore($KNOWLEDGE_DB_DIR/knowledge.db)  (存在则打开)
    │     ├─ .OpenLawsDB(...) / .OpenPatentKGdb(...)               (可选)
    │     └─ 维度校验: store.dim == embedder.Dimensions()
    │
    ├─ knowledge.NewExtension(store).WithBackend(backend, embedder)  [接线]
    │     (WIKI_PATH 模式作为 fallback 保留)
    │
    └─ fc.WikiHook = ext.LifecycleHook()   (三入口共享)
```

### 2.3 分层合规（ADR-0001）

| 层 | 模块 | 角色 |
|----|------|------|
| 应用入口 | `cmd/mady` | 装配（buildEmbedder/loadKnowledgeBackend），不实现算法 |
| 领域层 | `knowledge/extension.go` | 检索编排（backendSearch/RRF 调度），依赖接口 |
| 基础设施层 | `knowledge/sqlite/`, `retrieval/` | 具体实现（SQLiteStore/IVF/Embedder/Reranker） |

**领域层不 import 基础设施实现**：`KnowledgeExtension` 只依赖 `retrieval.Embedder` / `KnowledgeBackend` 接口，不直接 import `knowledge/sqlite`。

---

## 3. 关键算法

### 3.1 IVF 查询（阶段2）

IVF（Inverted File）将向量空间用 k-means 划分为 `nlist` 个聚类：
1. 查询时计算 query 到 `nlist` 个聚类中心的距离，选最近 `nprobe` 个
2. 只在这 `nprobe` 个聚类的向量里做精确余弦

**精度/速度权衡**：`nprobe` 越大越精确（nprobe=nlist 退化为暴力），默认 8。

**实现要点**：
- BLOB 解析：先探查 XiaoNuo 序列化格式（[NEEDS CLARIFICATION]）。若为 faiss 索引，纯 Go 解析复杂度高，需评估。
- 回退：解析失败返回 `ErrNoIVFIndex`，上层调 `VectorSearch`（暴力）。

### 3.2 RRF 融合（已实现，零改动）

```
score(d) = Σ 1/(k + rank_i(d))   # k=60, rank 从 1 开始
```
score-agnostic，FTS 与 Vector 分数尺度不同也无妨。三路同理。

### 3.3 Cross-encoder 重排（阶段2.5）

```
candidates = RRF 结果 (topK=20)
docs = [c.Content for c in candidates]
resp = POST /v1/rerank {model, query, documents:docs, top_n:5}
return [candidates[r.index].withScore(r.relevance_score) for r in resp.results]
```

**query 传递问题**（§2.2 spec）：
- 现有 `Reranker.Rerank(results)` 无 query 参数
- **方案 A（推荐）**：新增接口方法 `RerankWithQuery(ctx, query, results)`，`ModelReranker` 实现它；`backendSearch` 调用 `ModelReranker` 时显式传 query。启发式 reranker 不受影响（仍实现旧 `Rerank`）。
- 方案 B：`ModelReranker.SetQuery()` 状态化，并发不安全，不推荐。

### 3.4 写入流程（阶段3）

```
WritableStore.AddDocument(ctx, docID, title, content)
    │
    ├─ chunks = retrieval.ChunkDocument(docID, content, opts)
    ├─ texts = [c.Content for c in chunks]
    ├─ vecs = embedder.Embed(ctx, texts)     # 批量，batch=32 (oMLX embedding_batch_size)
    ├─ BEGIN TRANSACTION
    │   ├─ INSERT documents ...
    │   ├─ INSERT chunks ... (拿自增 id)
    │   ├─ for each chunk: INSERT embeddings (vector=BLOB, norm=‖v‖)
    │   └─ INSERT INTO docs_fts ...
    └─ COMMIT
```

**并发**：`sync.Mutex` 单写者；读用 WAL 模式（`?_journal_mode=WAL`）不阻塞读。

---

## 4. 安全考量

### 4.1 沙箱与只读约束

- `knowledge.db` 必须以 `mode=ro` 打开（现有 `NewSQLiteStore` 已是只读，不改）
- `WritableStore` 仅操作 `USER_DB_PATH`，**禁止**指向 knowledge.db（启动期校验路径不等于 KNOWLEDGE_DB_DIR 下文件）
- `tools/path.go` 沙箱边界：user.db 须落在 `$MADY_HOME` 下，不允许任意路径

### 4.2 密钥管理

- `OMLX_API_KEY` 走环境变量，**禁止硬编码**（AGENTS.md 安全红线）
- 不在日志中打印 API Key
- 启动期探测 oMLX 失败不打印 key

### 4.3 注入防护

- FTS 查询已做双引号转义（AI_CHANGELOG 2026-07-12 修复）
- 法律库 LIKE 查询已做 `%`/`_`/`\` 转义
- user.db 写入用参数化查询（`?` 占位），content 不拼 SQL

### 4.4 资源耗尽

- Embed 批量上限 32（对齐 oMLX `embedding_batch_size`）
- Rerank 输入上限 20 文档（topK·2 candidate，截断）
- 暴力 VectorSearch 分批 2000 行，内存有界

### 4.5 安全敏感路径（需 L3 审阅）

| 路径 | 涉及 |
|------|------|
| `knowledge/sqlite/store.go` | 只读约束（不改，新增方法） |
| `knowledge/sqlite/writable.go`（新） | 写入沙箱边界 |
| `cmd/mady/main.go` | 装配逻辑、API Key 处理 |
| `tools/path.go` | user.db 路径校验（若复用） |

---

## 5. 性能预算

| 操作 | 目标 | 当前 |
|------|------|------|
| Embed 单 query | < 50ms (oMLX 本地) | — |
| FTS 查询 | < 20ms | 已达标 |
| Vector 暴力查询 | < 3s（兜底） | 数秒 |
| IVF 查询 | < 50ms p95 | — |
| RRF 融合 | < 1ms | 已达标 |
| Rerank (20 docs) | < 300ms | — |
| 端到端单次检索 | < 500ms（IVF+rerank）/ < 3s（暴力+rerank） | — |

---

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| IVF BLOB 格式无法纯 Go 解析 | 中 | 阶段2 先实现探查脚本；失败则保留暴力，记录为阶段 2.1 候选（HNSW） |
| oMLX 服务未随系统启动 | 中 | `/health` 探测降级；文档说明 `auto_start_on_launch` |
| user.db 与 knowledge.db 模型漂移 | 中 | 写入前维度校验；记录 model 字段 |
| 重排延迟拖慢响应 | 中 | `KNOWLEDGE_RERANK=0` 可关；仅 Top-K 重排 |
| 大规模写入阻塞读 | 低 | WAL 模式 + 单写者 |
| 6.5GB knowledge.db 复制成本 | 低 | 提案建议保留软链；复制为可选 |

---

## 7. 降级矩阵（运行时自动）

```
oMLX 可用?  ─Y→ embedding+rerank
    └N→ 纯 FTS (Vector 路 skip, Rerank skip)
              │
knowledge.db 存在? ─Y→ SQLite 后端
    └N→ WIKI_PATH 内存模式 (若设)
              │
              └N→ 知识检索关闭
```

每个降级点记录一次 WARN 日志（不重复刷屏）。
