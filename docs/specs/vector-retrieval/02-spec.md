# 02 — 规格：向量检索落地

- **功能名**：vector-retrieval
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **规格日期**：2026-07-13
- **状态**：待人工审阅
- **依赖提案**：[01-proposal.md](./01-proposal.md)

---

## 1. 数据模型

### 1.1 现有数据模型（knowledge.db，只读消费）

以下 schema 已由 XiaoNuo 预建，本期**只读消费，不修改**：

```sql
-- 文档元数据
CREATE TABLE documents (
    id TEXT PRIMARY KEY,           -- 文档 ID（如 law://专利法/第22条）
    source TEXT NOT NULL,          -- 来源
    doc_type TEXT NOT NULL,        -- document/patent/case/law/...
    domain TEXT DEFAULT 'patent',  -- patent/legal
    title TEXT NOT NULL,
    content_hash TEXT,
    indexed_at TEXT NOT NULL,
    char_count INTEGER, chunk_count INTEGER
    -- ... 其它可选字段（module/priority/level/court/...）
);

-- 文档分块
CREATE TABLE chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id TEXT NOT NULL REFERENCES documents(id),
    chunk_index INTEGER NOT NULL,
    chunk_type TEXT NOT NULL,      -- section/paragraph/...
    heading TEXT,
    content TEXT NOT NULL,
    char_count INTEGER
);

-- 嵌入向量（BGE-M3 1024维 float32 LE，BLOB 4096字节）
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL REFERENCES chunks(id),
    document_id TEXT NOT NULL,
    vector BLOB NOT NULL,          -- 1024 × float32 LE
    model TEXT DEFAULT 'bge-m3',
    dim INTEGER DEFAULT 1024,
    norm REAL NOT NULL DEFAULT 0.0,-- 预计算范数（余弦相似度用）
    indexed_at TEXT NOT NULL
);
CREATE INDEX idx_embeddings_document ON embeddings(document_id);
CREATE INDEX idx_embeddings_chunk ON embeddings(chunk_id);

-- FTS5 全文索引（trigram 分词，CJK 友好）
-- 虚拟表 docs_fts，BM25 评分

-- 预建 IVF 近似最近邻索引（当前 VectorSearch 忽略，阶段2 复用）
CREATE TABLE ivf_index (
    id INTEGER PRIMARY KEY CHECK (id = 1),  -- 单行
    dim INTEGER NOT NULL,                   -- 1024
    nlist INTEGER NOT NULL,                 -- 聚类中心数
    index_data BLOB NOT NULL,               -- 序列化的 IVF 结构
    built_at TEXT NOT NULL
);

-- 知识图谱
CREATE TABLE kg_nodes (id TEXT PRIMARY KEY, node_type, name, ...);
CREATE TABLE kg_edges (source_id, target_id, relation, weight, ...);
```

`index_meta` 记录：`total_docs=81038`, `total_chunks=138609`。

`laws-full.db`：`law(id, level, name, content, ...)` + `category(id, name, ...)`。

### 1.2 新增数据模型（user.db，阶段3 可写）

用户自建文档向量化落库，物理独立于 knowledge.db：

```sql
-- 复用与 knowledge.db 相同的表结构，便于统一查询
CREATE TABLE documents ( /* 同上 schema */ );
CREATE TABLE chunks ( /* 同上 schema */ );
CREATE TABLE embeddings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL,
    document_id TEXT NOT NULL,
    vector BLOB NOT NULL,
    model TEXT NOT NULL,           -- 记录嵌入模型，避免维度混淆
    dim INTEGER NOT NULL,
    norm REAL NOT NULL,
    indexed_at TEXT NOT NULL
);
CREATE INDEX idx_user_embeddings_doc ON embeddings(document_id);

-- user.db 的 FTS5 虚拟表（与 knowledge.db 同构）
CREATE VIRTUAL TABLE docs_fts USING fts5(content, heading, document_id, chunk_id, tokenize='trigram');

-- user.db 不强制建 IVF（数据量小，暴力扫描足够）；预留 ivf_index 表以便后期按需构建
```

**写入流程**：`文档文本 → ChunkDocument → 批量 Embed(oMLX) → 写入 chunks + embeddings + docs_fts`。

### 1.3 维度一致性约束

knowledge.db 向量为 1024 维（BGE-M3）。user.db 写入必须使用**同一模型**（`bge-m3-mlx-8bit`）。启动时校验：
- `SQLiteStore.dim`（从 embeddings 表探测）必须等于 `Embedder.Dimensions()`
- 不一致时拒绝该后端并记录错误日志（不 panic）

---

## 2. 接口定义

### 2.1 Embedder（复用现有，零改动）

`retrieval.Embedder` 接口与 `APIEmbedder` 实现**完全复用**，无需新增类型：

```go
// retrieval/embedding.go（已存在，不改）
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}
```

`APIEmbedder` 指向 oMLX：
```go
embedder := retrieval.NewAPIEmbedder(
    "http://127.0.0.1:8000/v1",   // BaseURL，/embeddings 自动拼接
    os.Getenv("OMLX_API_KEY"),    // APIKey
    "bge-m3-mlx-8bit",            // Model
)
// Dimensions() 首次调用后缓存为 1024
```

> **验证**：oMLX `/v1/embeddings` 实测返回 1024 维，OpenAI 兼容格式，APIEmbedder 直接可用。

### 2.2 Reranker（新增 cross-encoder 实现）

现有 `retrieval.Reranker` 接口保留，新增基于 oMLX 的模型重排实现：

```go
// retrieval/rerank_model.go（新增）
type ModelReranker struct {
    BaseURL string   // http://127.0.0.1:8000/v1
    APIKey  string
    Model   string   // Qwen3-Reranker-4B-4bit-MLX
    TopN    int      // 重排后返回数量，默认与输入相同
    client  *http.Client
}

func NewModelReranker(baseURL, apiKey, model string) *ModelReranker

// 实现 retrieval.Reranker
func (r *ModelReranker) Rerank(results []ScoredChunk) []ScoredChunk
```

**请求格式**（Cohere 兼容，实测确认）：
```http
POST /v1/rerank
Authorization: Bearer <OMLX_API_KEY>
Content-Type: application/json

{"model":"Qwen3-Reranker-4B-4bit-MLX",
 "query":"<上次检索的 query>",
 "documents":["<chunk1 content>","<chunk2 content>",...],
 "top_n":N}
```

**响应格式**（实测确认）：
```json
{"id":"rerank-xxx",
 "results":[
   {"index":0,"relevance_score":0.95,"document":{"text":"..."}},
   {"index":2,"relevance_score":0.82,"document":{"text":"..."}}
 ],
 "model":"Qwen3-Reranker-4B-4bit-MLX",
 "usage":{"total_tokens":156}}
```

**注意**：`Reranker.Rerank(results)` 当前签名不含 query；cross-encoder 需要 query。设计上采用 **RerankContext 包装**（详见 03-design.md §3.3）：
- 方案 A：扩展接口为 `RerankWithQuery(query string, results []ScoredChunk)`
- 方案 B：`ModelReranker` 内部缓存上次 query（由调用方 `SetQuery`）

**[NEEDS CLARIFICATION: 04-tasks 选方案 A（新增方法，不破坏现有接口）还是 B？倾向 A]**

### 2.3 SQLiteStore 扩展（阶段2 IVF + 阶段3 写入）

#### 2.3.1 阶段2：IVF 加速查询（新增方法，不改现有）

```go
// knowledge/sqlite/store.go（新增）
// IVFSearch 利用预建 IVF 索引加速查询；返回与 VectorSearch 相同类型。
// 若 ivf_index 表不存在或解析失败，返回 ErrNoIVFIndex，调用方回退 VectorSearch。
func (s *SQLiteStore) IVFSearch(queryVec []float32, topK int, nprobe int) ([]retrieval.ScoredChunk, error)

var ErrNoIVFIndex = errors.New("sqlite: ivf_index not available")
```

`nprobe`（探测聚类数）默认 8，可配置，控制精度/速度权衡。

#### 2.3.2 阶段3：可写模式（新类型，不改只读 SQLiteStore）

```go
// knowledge/sqlite/writable.go（新增）
type WritableStore struct {
    db       *sql.DB
    dim      int
    embedder retrieval.Embedder
    mu       sync.Mutex  // 单写者
}

// OpenWritable 打开/创建 user.db（若不存在则建表）
func OpenWritable(path string, embedder retrieval.Embedder) (*WritableStore, error)

// AddDocument 写入文档：分块 → 向量化 → 落 chunks/embeddings/docs_fts
func (w *WritableStore) AddDocument(ctx context.Context, docID, title, content string) error

// Search 仅在 user.db 内检索（FTS+Vector），供上层与 knowledge.db 结果再融合
func (w *WritableStore) Search(ctx context.Context, query string, topK int) ([]retrieval.ScoredChunk, error)

func (w *WritableStore) Close() error
```

### 2.4 KnowledgeBackend 接口（已存在，可能扩展）

```go
// knowledge/extension.go（已存在）
type KnowledgeBackend interface {
    FTSSearch(query string, topK int) ([]retrieval.ScoredChunk, error)
    VectorSearch(queryVec []float32, topK int) ([]retrieval.ScoredChunk, error)
}
```

阶段3 新增 user.db 后，`backendSearch` 增加 user.db 检索分支，三路结果（knowledge FTS + knowledge Vector + user Search）统一 RRF。

### 2.5 配置项（环境变量）

新增环境变量（沿用现有约定，全部可选，有合理默认值）：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `KNOWLEDGE_DB_DIR` | `$MADY_HOME/knowledge` | 知识库目录（含 knowledge.db 等） |
| `OMLX_BASE_URL` | `http://127.0.0.1:8000/v1` | oMLX 服务基址 |
| `OMLX_API_KEY` | — | oMLX API Key（必填才启用模型；未填则降级纯 FTS） |
| `OMLX_EMBED_MODEL` | `bge-m3-mlx-8bit` | 嵌入模型名 |
| `OMLX_RERANK_MODEL` | `Qwen3-Reranker-4B-4bit-MLX` | 重排模型名 |
| `KNOWLEDGE_RERANK` | `1` | 是否启用 cross-encoder 重排（`0` 关闭） |
| `KNOWLEDGE_IVF_NPROBE` | `8` | IVF 探测聚类数 |
| `USER_DB_PATH` | `$MADY_HOME/knowledge/user.db` | 用户可写库路径 |

**降级策略**（优先级从高到低，自动探测）：
1. knowledge.db + oMLX embedding + IVF + rerank → 完整能力
2. knowledge.db + oMLX embedding（暴力） + 无 rerank → 向量召回，无重排
3. knowledge.db 仅 FTS（oMLX 未启动/未配 key）→ 纯关键词
4. 无 knowledge.db → 关闭知识检索（仅当 WIKI_PATH 设时走内存模式）

---

## 3. 验证规则

### 3.1 启动期校验

| 校验项 | 失败行为 |
|--------|----------|
| `KNOWLEDGE_DB_DIR/knowledge.db` 存在性 | 跳过该后端，记录日志，不报错 |
| embeddings.dim == Embedder.Dimensions() | 拒绝该后端，记录错误 |
| oMLX `/health` 可达 + API Key 有效 | 降级为纯 FTS |
| IVF 索引可解析 | 降级为暴力 VectorSearch |
| user.db 维度与 knowledge.db 一致 | 拒绝 user.db，记录错误 |

### 3.2 运行期校验

- 每次检索：FTS / Vector / user 三路任一失败，记录日志但不中断，用可用路的结果继续 RRF
- Embed 调用失败（oMLX 临时不可用）：本次跳过向量路，仅 FTS
- Rerank 调用失败：返回未重排的 RRF 结果

### 3.3 测试矩阵

| 测试 | 覆盖 |
|------|------|
| 单元测试 | IVFSearch 解析、ModelReranker 请求构造、WritableStore 读写、维度校验 |
| 集成测试 | backendSearch 三路 RRF（用小规模测试库）、降级链路、user.db 命中 |
| benchmark | VectorSearch vs IVFSearch 延迟对比（p95）、Rerank 延迟 |
| e2e | TUI `search_knowledge`、`/api/chat` 参考文档注入 |

---

## 4. 输入输出契约

### 4.1 检索工具输出（`search_knowledge`）

```
搜索结果:
[1] (相关度: 0.92) <chunk content>
[2] (相关度: 0.85) <chunk content>
...
共 N 条结果
```

相关度为 rerank 后的 `relevance_score`（启用重排）或 RRF 融合分（未启用）。

### 4.2 自动注入上下文（`KnowledgeExtension.Provide`）

```
### 参考文档
--- [1] (0.92) ---
<chunk content>
--- [2] (0.85) ---
<chunk content>
```

---

## 5. 决策记录

| # | 问题 | 决策 | 确认日期 |
|---|------|------|----------|
| 1 | Reranker 接口扩展方案 A vs B | **方案 A**：新增 `RerankWithQuery(query, docs)` 方法，不改现有 `Rerank` 签名 | 2026-07-13 |
| 2 | knowledge.db 保留软链 vs 复制 | **复制**：将 6.5GB 实际文件复制到 `~/.mady/knowledge/`，删除软链，脱离 XiaoNuo 依赖 | 2026-07-13 |
| 3 | Human Owner 指派 | [待指派] | — |
| 4 | XiaoNuo IVF 构建 Python 脚本路径 | [待探查] T2.1 前置任务，不阻塞阶段1 | — |
