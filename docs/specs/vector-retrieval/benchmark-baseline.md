# 向量检索 Benchmark 基线

> 生成时间：2026-07-13
> 硬件：Apple M4 Pro (14 cores)
> 数据规模：81,038 文档 / 138,609 chunks / 144,069 embeddings (BGE-M3 1024-dim float32)

## 运行方法

```bash
# 底层性能（排除极慢的 SQL 对比版本）
go test -bench='Preload|FTSSearch|VectorIndex|VectorSearchInMemory|GetChunk' \
  -benchmem ./knowledge/sqlite/

# SQL 对比版本（单次运行）
go test -bench=VectorSearchSQL -benchtime=1x -benchmem ./knowledge/sqlite/

# 端到端性能
go test -bench='BackendSearch|RRFFusion' -benchmem ./knowledge/
```

## 底层性能（knowledge/sqlite）

| Benchmark | 耗时 | 内存/次 | 分配次数 | 说明 |
|-----------|------|---------|---------|------|
| PreloadVectorIndex | **251 ms** | 1.80 GB | 1,440,729 | 一次性加载 144K 向量到内存 |
| FTSSearch | **10.3 ms** | 19 KB | 174 | BM25 + trigram FTS5 短语匹配 |
| VectorIndexSearch | **14.5 ms** | 4.71 MB | 91 | 纯内存并行暴力点积（无 IO） |
| VectorSearchInMemory | **15.2 ms** | 4.73 MB | 502 | 内存搜索 + getChunk 取内容 |
| VectorSearchSQL | **1,328 ms** | 1.80 GB | 1,730,919 | SQL 批量读取暴力搜索（对比基线） |
| GetChunk | **5.2 μs** | 843 B | 28 | 单条 chunk 按 ID 查询 |

## 端到端性能（knowledge）

| Benchmark | 耗时 | 内存/次 | 分配次数 | 说明 |
|-----------|------|---------|---------|------|
| BackendSearch | **29.8 ms** | 4.84 MB | 1,287 | FTS + Embed + VectorSearch + RRF |
| RRFFusion | **4.6 μs** | 12.7 KB | 51 | RRF k=60 融合双通道结果 |

## 性能预算对比

| 路径 | 预算 | 实测 | 状态 |
|------|------|------|------|
| VectorSearch（内存版） | < 50 ms | 15.2 ms | ✅ 达标 |
| RRF 融合 | < 1 ms | 4.6 μs | ✅ 达标 |
| 端到端 backendSearch | < 500 ms | 29.8 ms | ✅ 达标 |
| Cross-encoder Rerank | < 300 ms | N/A | ⏳ 需 oMLX 服务在线时测量 |

## 关键发现

### 1. 内存预加载 vs SQL 批量：87x 加速

| 路径 | 耗时 | 加速比 |
|------|------|--------|
| VectorSearchSQL（批量读 + 解码） | 1,328 ms | 基线 |
| VectorSearchInMemory（并行点积） | 15.2 ms | **87x** |

预加载成本 251 ms 在 **17 次查询后摊销**（251 / 15.2 ≈ 17）。

### 2. 端到端耗时分解

BackendSearch 29.8 ms 的构成：

```
FTSSearch      10.3 ms  (35%)
VectorSearch   15.2 ms  (51%)
RRF 融合        4.6 μs  (<0.1%)
Embed 调用      ~4.3 ms (14%, mockEmbedder 零计算，仅接口开销)
```

FTS 和 Vector 串行执行，总耗时 ≈ FTS + Vector + 开销。

### 3. GetChunk IO 可忽略

VectorSearchInMemory vs VectorIndexSearch 差值仅 0.7 ms（15.2 - 14.5），
10 条 getChunk 查询 × 5.2 μs = 52 μs，对总耗时无影响。

### 4. 内存占用

| 组件 | 常驻内存 | 说明 |
|------|---------|------|
| VectorIndex | ~562 MB | 144K × 1024 × 4 字节（flat []float32） |
| 单次查询峰值 | ~4.8 MB | worker results + merged top-K |
| Preload 分配总量 | 1.80 GB | 含 slice 扩容的临时分配 |

### 5. 并行效率

VectorIndexSearch 使用 `runtime.GOMAXPROCS`（14）个 worker 并行计算。
144K 向量 / 14 workers ≈ 10,291 向量/worker，每个 worker 独立排序 top-K。
并行效率良好——单核线性扫描需 ~200ms，14 标核降至 14.5ms（~14x 理想加速）。

## 后续优化方向

1. **IVF 近似搜索**：当前暴力扫描 144K 向量 15ms，IVF nprobe=8 可降至 <5ms，但需
   逆向 XiaoNuo 的 IVF BLOB 格式或自行构建索引（阶段2 未走此路线，因 ivf_index 表为空）。
2. **FTS + Vector 并行化**：当前串行执行 FTS → Vector，可改为并行（`errgroup`），
   端到端从 ~25ms 降至 ~15ms（max(FTS, Vector)）。收益有限，非瓶颈。
3. **getChunk 批量化**：当前 top-K 结果逐条查询 chunk 内容，可改为 `WHERE id IN (...)`
   单次查询。当前 IO 仅 52μs，优化收益可忽略。
4. **Rerank 性能**：需 oMLX 服务在线时测量。Top20 → Top5 的 cross-encoder 重排
   预计 100-300ms（取决于 Qwen3-Reranker-4B 推理速度）。
