# 知识系统性能基线

> 记录时间：2026-07-18
> 硬件环境：Apple M4 Pro, 48GB RAM
> 知识库：knowledge.db (6.5GB, 144K 向量, 1024-dim BGE-M3)
> 运行命令：`make bench-knowledge`

## 端到端检索

| 操作 | 耗时 | 内存分配 | 分配次数 |
|------|------|---------|---------|
| **BackendSearch** (FTS+向量+RRF 融合) | **30ms** | 166KB | 1,407 |
| **RRFFusion** (仅融合步骤) | **5μs** | 13KB | 51 |

## 检索组件

| 操作 | 耗时 | 内存分配 | 分配次数 |
|------|------|---------|---------|
| **FTSSearch** (BM25 + trigram) | **11ms** | 19KB | 174 |
| **VectorIndexSearch** (内存并行暴力) | **18ms** | 29KB | 362 |
| **VectorSearchInMemory** (内存+SQL chunk) | **18ms** | 43KB | 571 |
| **VectorSearchSQL** (并行 SQL 回退) | **188ms** | 879MB | 639K |
| **GetChunk** (单条 chunk 检索) | **7.7μs** | 843B | 28 |

## 启动成本

| 操作 | 耗时 | 内存 |
|------|------|------|
| **PreloadVectorIndex** (144K 向量 → 内存) | **269ms** | **1.8GB** |
| Graph Load (214K 节点/234K 边 → 内存) | ~2-3s (次) | ~500MB |

## 外部服务延迟（本机 OMLX localhost:8000）

| 模型 | 首请求 | 后续请求 | 说明 |
|------|--------|---------|------|
| **bge-m3-mlx-8bit** (embedding) | 260ms | **10ms** | 1024-dim 向量 |
| **Qwen3-Reranker-4B-4bit-MLX** (rerank) | 845ms | **580ms** | 4 文档重排 |

## 检索质量

EvalHook 默认启用（Phase 3），在每次模型调用后自动计算：

| 指标 | 方法 | 范围 | 说明 |
|------|------|------|------|
| Faithfulness | 启发式 (关键词覆盖) | 0.3–1.0 | 答案忠实于检索上下文 |
| AnswerRelevancy | 启发式 (问题词覆盖率) | 0.4–1.0 | 答案针对问题程度 |
| ContextPrecision | 启发式 (上下文标记检测) | 0.7–1.0 | 检索结果噪声比例 |

## 优化历程

| 日期 | 变更 | 效果 |
|------|------|------|
| 2026-07-18 | FTS5 trigram 索引 (laws-full.db) | 法律搜索从 LIKE 升级为 BM25 排序 |
| 2026-07-18 | SQL 回退并行化 | 13.8s → **188ms** (73×) |
| 2026-07-18 | 环境变量自动注入 (direnv+godotenv) | 向量检索从禁用变为激活 |

## 回归检测

```bash
make bench-knowledge
# 对比 bench-knowledge.txt 中的关键指标:
#   BenchmarkFTSSearch        → 应 < 15ms
#   BenchmarkVectorIndexSearch → 应 < 25ms
#   BenchmarkBackendSearch     → 应 < 50ms
#   BenchmarkVectorSearchSQL   → 应 < 500ms
```
