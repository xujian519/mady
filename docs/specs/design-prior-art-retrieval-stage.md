# 设计文档：现有技术检索阶段 — `retrieve_prior_art` 节点

> 状态：草案（2026-07-16 已完成代码库核对，节点名/接口签名/DomainRetriever 现状均已订正，见第二、三、五节订正标注）
> 关联：disclosure 管线（10 节点 Pregel：preprocess→三提取并行→merge_extractions→check_consistency→generate_keywords→check_novelty→generate_report→review_gate）
> 关联组件：`retrieval/`、`knowledge/graph/`、`domains/reasoning/`、`disclosure/novelty.go`、`disclosure/report.go`

## 一、问题背景

代码库审计确认：`retrieval/` 与 `knowledge/graph/` 两个包接口完整（详见附录一），但全仓库 grep `(retrieval|knowledge)` 在 `disclosure/` 下零匹配。唯一提及处是 `report.go:78` 的注释——"Phase 2 将增强为 retrieval 模块集成 + domains/reasoning 推理引擎"，纯未来计划，从未落地。

具体表现：

- **三提取节点**（`extract_problem` / `extract_features` / `extract_effects`，均由 `extract.go:22 newExtractionAgent()` 创建）：内部走 `pregelAgentNode()`，仅做一次纯 LLM 调用，输入输出均不经过 `retrieval` 或 `knowledge/graph`。
- **新颖性初判节点**（`check_novelty`，由 `novelty.go:17 noveltyNode(provider)` 创建）：`buildNoveltyInput()` 仅拼接提取结果 + 关键词作为 prompt，未注入任何外部文档；LLM 失败时回退到 `assessNoveltyFromState()`（`report.go:88`），该函数是纯启发式打分（特征分类/重要度/关键词本地匹配），同样不触及外部知识库。

结论：新颖性判断的两条路径（LLM / 启发式回退）均不基于真实专利语料或知识图谱比对，本质是"LLM 凭参数化知识猜测"或"本地统计打分"，与产品定位声明的"每个专业结论绑定可定位的证据"直接矛盾。此问题优先级应高于当前证据包裹生成/DOCX 导出等 P1B/P1C 事项，建议并入 P1A 证据底座范畴。

## 二、接口盘点

### 2.1 `retrieval/` 包

| 类型/接口 | 签名 | 文件 |
|---|---|---|
| `Searcher`（interface） | `Search(query string, chunks []Chunk, topK int) []ScoredChunk` | keyword.go:20 |
| `KeywordSearcher` | `NewKeywordSearcher() *KeywordSearcher` | keyword.go:34-42 |
| `VectorSearcher` | `NewVectorSearcher(embedder Embedder) *VectorSearcher` | vector.go:29-37 |
| `HybridSearcher` | `NewHybridSearcher(keyword, vector Searcher) *HybridSearcher` | vector.go:96-106 |
| `RRFFuser` | `NewRRFFuser() *RRFFuser`　`Fuse(lists [][]ScoredChunk, topK int) []ScoredChunk` | hybrid.go:19-25 |
| `Reranker`（interface） | `Rerank(results []ScoredChunk) []ScoredChunk` | rerank.go:12 |
| `PositionReranker` / `DeduplicatingReranker` / `ChainReranker` / `LegalReranker` | 见文件 | rerank.go:26/59/86/133 |
| `Embedder`（interface） | `Embed(ctx, texts []string) ([][]float32, error)`　`Dimensions() int` | embedding.go:22-26 |
| `APIEmbedder` | `NewAPIEmbedder(baseURL, apiKey, model string)` | embedding.go:54 |
| `ChunkDocument` | `ChunkDocument(docID, text string, opts ChunkOptions) []Chunk` | chunk.go:40 |
| `RetrievalHook` | `NewRetrievalHook(...)`　`BeforeModelCall(...)` — LifecycleHook | agent.go:99-157 |
| `domain.DomainRetriever`（interface） | `Search(ctx, query DomainQuery) (*DomainResults, error)`　`GetDocument(...)`　`SourceName() string` | domain/base.go:25-35 |
| `domain.ImportToStore` | `func ImportToStore(store *knowledge.Store, results *DomainResults, domainName string) (int, error)` | domain/base.go:86 |

注意：`Searcher` 系接口操作的是内存中的 `[]Chunk`，真正的持久化查询（FTS5+向量余弦）在 `knowledge/sqlite` 层，其对外签名尚未核对，需在实现前补齐（见第五节开放问题）。

### 2.2 `knowledge/graph/` 包

| 类型/接口 | 签名 | 文件 |
|---|---|---|
| `GraphStore` | `AddNode/GetNode/HasNode/RemoveNode/AddEdge/RemoveEdges/GetOutgoing/GetIncoming/GetNodeDetail/SearchGraphNodes/AllNodes/NodeCount/SaveFile/LoadFile` | store.go |
| `QueryPaths` | `func QueryPaths(store, sourceID, targetID string, maxDepth int) PathResult` — BFS 查找两节点间最多 5 条路径 | query.go:24 |
| `QueryNeighbors` | `func QueryNeighbors(store, nodeID string, depth int) []*GraphNode` — 多跳 BFS 邻居遍历 | query.go:111 |
| `QueryByRelation` | `func QueryByRelation(store, nodeID, relation, direction string) []*GraphNode` | query.go:144 |
| `QueryCitationChain` | `func QueryCitationChain(store, lawRef string) []*GraphNode` — 引用同一法条的文档 | query.go:182 |
| `QuerySimilar` | `func QuerySimilar(store, nodeID string) []*GraphNode` — SIMILAR_TO 邻居 | query.go:212 |
| `ReasoningStoreAdapter` | 实现 `reasoning.KnowledgeGraphStore`：`SearchNodes`　`GetNodeDetail` | adapter.go:24-50 |
| `GraphEnhancer` | `NewGraphEnhancer(store, config EnhanceConfig)`　`Enhance(seeds []ScoredChunk) any` — 基于种子检索结果扩展图谱上下文（相似案例+法条引用链）。**注意返回类型是 `any`**（内部返回 `EnhancementResult` 但声明为空接口），下游消费需类型断言，不能直接解构字段 | retrieval_enhancer.go:50-88 |
| `GraphCache` | `NewGraphCache(size int)` — LFU 查询缓存 | cache.go |
| `GraphBuilder`（早期版本误写作 `Builder`） | `NewGraphBuilder(store *GraphStore)`　`Build(docs []ParsedDoc) GraphBuildResult` — 文档→节点/边 | builder.go:19-44 |

### 2.3 桥接接口（`domains/reasoning/`）

| 接口 | 签名 | 文件 |
|---|---|---|
| `KnowledgeGraphStore` | `SearchNodes(keyword, nodeType string, limit int) ([]KgNode, error)`　`GetNodeDetail(nodeID string) (*KgNodeDetail, error)` | walker.go:34-37 |
| `ReasoningWalker` | `NewReasoningWalker(store, llm)`　`Walk(ctx, in ReasoningWalkInput) (ReasoningWalkResult, error)` — 多跳推理遍历　`CollectAll(ctx, in CollectAllInput) (CollectAllResult, error)` — 全量规则约束收集 | walker.go:120-184 |

**一句话总结**：`retrieval/` 提供文档分块→关键词/向量/混合搜索→RRF融合→重排序→Agent注入的全链路；`knowledge/graph/` 提供内存图谱存储→多跳BFS遍历→关系查询→引用链→检索结果扩展。两包接口完整，`disclosure/` 管线目前均未接入。

## 三、节点设计：`retrieve_prior_art`

插入位置：`generate_keywords` 之后、`check_novelty` 之前。

```
extract_problem ─┐
extract_features ─┼→ merge_extractions → check_consistency → generate_keywords
extract_effects ─┘                                              │
                                                                 ▼
                                                   retrieve_prior_art   ← 新增节点
                                                                 │
                                                                 ▼
                                                          check_novelty  ← 改 buildNoveltyInput
```

> 节点名订正（2026-07-16 代码核对）：实际注册的节点 ID 为 `merge_extractions` /
> `check_consistency` / `generate_keywords`（见 `disclosure/graph.go` 的
> `BuildDisclosureAnalysisGraph`），早期版本写作 `merge` / `consistency_check` /
> `keyword_gen` 是不准确的。

### 3.1 节点职责

输入：`keyword_gen` 产出的检索关键词 + `merge` 后的技术特征结构。

双路查询，非二选一：

1. **专利领域检索器**（`domain.DomainRetriever` 的专利实现）查询候选现有技术文档，产出带 `EvidenceSpan`（文档 ID、来源、原文片段、相似度分数）的候选集。
2. **知识图谱**（`ReasoningWalker` / `GraphEnhancer`）以检索种子为起点做多跳遍历，查同族专利、引用链、相似案例——这是向量检索查不到的结构化事实。

零候选时的处理：若两路均查不到候选，不应让 `check_novelty` 静默继续判定"新颖"，而应在 state 标记 `evidence_coverage=none`，强制走人工复核。

### 3.2 参考代码框架

```go
package disclosure

import (
    "context"
    "fmt"

    pgraph "github.com/xujian519/mady/graph"
    kgraph "github.com/xujian519/mady/knowledge/graph"
    "github.com/xujian519/mady/retrieval"
    "github.com/xujian519/mady/retrieval/domain"
)

// PriorArtRetrievalDeps 聚合本节点需要的外部依赖，构造时注入
// （风格对齐 novelty.go 的 noveltyNode(provider) 依赖注入方式）
type PriorArtRetrievalDeps struct {
    PatentRetriever domain.DomainRetriever // 专利领域检索器，内部接 patent_kg.db / knowledge.db
    KGStore         *kgraph.GraphStore     // 知识图谱存储
    Reranker        retrieval.Reranker     // 建议 ChainReranker{DeduplicatingReranker, ...}
    TopK            int                    // 默认建议 8-12
}

func retrievePriorArtNode(deps PriorArtRetrievalDeps) pgraph.PregelNodeFunc {
    return func(ctx context.Context, state pgraph.PregelState) (pgraph.PregelState, error) {
        keywords := state.GetStringSlice("keywords")
        features := state.GetStringSlice("features_summary")

        if len(keywords) == 0 && len(features) == 0 {
            state.Set("evidence_coverage", "none")
            return state, nil
        }

        query := domain.DomainQuery{
            Text: joinQuery(keywords, features),
            TopK: deps.TopK,
        }

        results, err := deps.PatentRetriever.Search(ctx, query)
        if err != nil {
            return state, fmt.Errorf("prior art search failed: %w", err)
        }

        seeds := domainResultsToChunks(results)

        if deps.Reranker != nil {
            seeds = deps.Reranker.Rerank(seeds)
        }

        if len(seeds) == 0 {
            state.Set("evidence_coverage", "none")
            return state, nil
        }

        enhancer := kgraph.NewGraphEnhancer(deps.KGStore, kgraph.EnhanceConfig{})
        enhanced := enhancer.Enhance(seeds)

        var citationFacts []*kgraph.GraphNode
        for _, s := range seeds {
            citationFacts = append(citationFacts, kgraph.QueryCitationChain(deps.KGStore, s.DocID)...)
        }

        state.Set("evidence_chunks", seeds)
        state.Set("graph_expansion", enhanced)
        state.Set("citation_chain", citationFacts)
        state.Set("evidence_coverage", coverageLevel(seeds))
        return state, nil
    }
}
```

图接线（`graph.go` 现有 `AddEdge` 调用旁新增）：

```go
pg.AddEdge("generate_keywords", "retrieve_prior_art")
pg.AddEdge("retrieve_prior_art", "check_novelty")
```

### 3.3 `check_novelty` 端的配套改动

- `buildNoveltyInput()`：拼入 `evidence_chunks` 原文片段（带 `DocID`）与 `citation_chain` 引用关系摘要；JSON Schema 新增必填字段 `CitedEvidenceIDs []string`，要求每条新颖性结论回填引用的 `DocID`。引用为空或引用不存在的 ID，判定该结论不可信，走人工复核而非直接进报告。
- `assessNoveltyFromState()`（启发式回退路径）：读取 `evidence_coverage`。若为 `none`，不得输出确定性的"新颖/不新颖"结论，只能输出"无法评估，需人工核实"。

## 四、评估指标配套

建议在 `agentcore/evaluate` 新增 `EvidenceGroundedness` 指标：统计新颖性初判结论中，真正引用了有效 `EvidenceSpan`（`CitedEvidenceIDs` 非空且可解析）的比例。该指标应先在当前代码状态下跑出基线（预期极低甚至为零），作为本次改造前后对比的量化依据，也便于记入 AI_CHANGELOG。

## 五、开放问题（2026-07-16 已核对）

1. **`domain.DomainRetriever` 的专利领域实现不存在**（已核对，非"待核对"）：全仓库零实现，`domains/patent.go` 只有 `PatentAgentConfig` / `BuildProjectAgent`（造 LLM agent 配置），不含 `PatentDomainRetriever`，也不 import `knowledge/sqlite`。`retrieval/domain/base.go` 的 `DomainRetriever` 接口纯属未落地的抽象。**这是本节点落地的前置工作量最大的一步**：需新建 `PatentDomainRetriever`，薄包装已就绪的 `sqlite.SQLiteStore`（`FTSSearch` / `VectorSearch` / `SearchLaws` 均已实现并已装配到 chat agent）。
2. **是否需要 `PatentReranker`**：现有 `LegalReranker`（`rerank.go:133`）说明"每个垂直领域一个 reranker"是既定模式，专利场景大概率也需要一个（例如按同族专利去重、按申请日排序压制"晚于本申请"的证据）。需先核对 `LegalReranker` 具体排序逻辑，避免专利版简单复制粘贴。
3. ~~`report.go:78` 处"Phase 2 将增强"的注释建议同步更新或删除~~ **已完成**（2026-07-16）：该注释已订正为反映 retrieval 已接入 chat agent 与 Stage ②、仅 disclosure 节点未接的现状。
4. **知识数据已就位**（本次核对新增，纠正早期"有机无料"判断）：`~/.mady/knowledge/{knowledge.db 6.5G, patent_kg.db 207M, laws-full.db 152M}` 通过软链接指向 xiaonuo 语料，`knowledge/sqlite` 查询层已可读。零候选→`evidence_coverage=none` 不会是系统常态。

## 六、与获取规则阶段设计的关系

本节点产出的 `evidence_chunks`（语义库检索）与 `citation_chain` / `graph_expansion`（知识图谱）与《获取规则阶段设计文档》（`design-rule-acquisition-stage.md`）中"规则召回"的语义库检索、知识图谱查询在数据源上是同一套基础设施，但服务目标不同：

- 本节点服务的是 disclosure 管线内部的**新颖性初判**（技术特征 vs 现有技术的比对）。
- 获取规则阶段服务的是**法律/规则适用**（案件应适用哪些法条/审查规则，需人工确认）。

两者可共享底层检索/图谱查询实现（`retrieval.HybridSearcher`、`kgraph.QueryCitationChain` 等），但不应合并为同一节点——证据检索是"找现有技术"，规则召回是"找适用规则"，两者的候选来源虽有重叠（如审查指南既是规则依据也可能作为图谱节点），但下游消费者、人工确认粒度、失败时的降级策略均不同，混在一起会让 Markdown 确认单和证据链的语义混乱。
