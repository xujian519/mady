# 01 — 提案：向量检索落地

- **功能名**：vector-retrieval
- **Human Owner**：[NEEDS CLARIFICATION: 待指派]
- **提案日期**：2026-07-13
- **状态**：待人工 Sign-off

---

## 1. 背景

### 1.1 现状

Mady v0.3.0 已经移植了 XiaoNuo 的完整知识系统数据资产，并且**检索算法层全部实现**，但生产链路**完全未接线**：

| 组件 | 代码状态 | 生产接线 |
|------|----------|----------|
| `retrieval.APIEmbedder`（OpenAI 兼容 /embeddings） | ✅ 已实现 | ❌ 仅测试实例化 |
| `knowledge/sqlite.SQLiteStore`（FTS5 + 向量扫描） | ✅ 已实现 | ❌ 仅测试使用 |
| `knowledge/sqlite.SQLiteStore.VectorSearch` | ✅ 暴力余弦扫 144K 向量 | ❌ 忽略预建 IVF 索引 |
| `retrieval.RRFFuser`（FTS+Vector 排名融合） | ✅ 已实现 | ❌ |
| `knowledge.KnowledgeExtension.backendSearch` | ✅ FTS+Vector RRF 完整 | ❌ `WithBackend` 全项目零 caller |
| `retrieval.Reranker` 接口 | ✅ 4 种启发式实现 | ❌ 无 cross-encoder 模型重排 |

**生产入口**（`cmd/mady/main.go:58` `loadWikiStore`）当前行为：
- 仅读取 `WIKI_PATH`（环境变量，默认空）→ 加载本地 wiki 到内存 `knowledge.Store` → 关键词搜索
- SQLiteStore / APIEmbedder / WithBackend **从未被实例化**
- 结果：`WIKI_PATH` 未设时知识检索整体关闭；即便设置，也只是内存关键词搜索，**向量召回未上线**

README 中"下季度计划：向量召回上线（当前仅结构化过滤）"指向的就是这个缺口。

### 1.2 数据资产现状（已就位）

`~/.mady/knowledge/` 下通过软链引入 XiaoNuo 知识库（可改为复制到本目录，脱离原项目依赖）：

| 数据库 | 规模 | 内容 |
|--------|------|------|
| `knowledge.db` | 6.5GB | 81,038 文档 / 138,609 chunks / 144,069 向量（BGE-M3 1024维）+ FTS5 trigram + **预建 IVF 索引** + 知识图谱 |
| `laws-full.db` | 152MB | 9,121 条法律全文 |
| `patent_kg.db` | 207MB | 专利知识图谱 |

### 1.3 本地模型服务（已就位）

oMLX 服务（`127.0.0.1:8000`，OpenAI/Cohere 兼容协议）已加载：
- `bge-m3-mlx-8bit` — 嵌入模型（1024维，与 knowledge.db 向量同源）
- `Qwen3-Reranker-4B-4bit-MLX` — Cross-encoder 重排模型

### 1.4 为什么现在做

1. 数据 + 模型 + 算法三层均已就绪，只差装配，**投入产出比最高**
2. 知识检索是专利/法律专业智能体的核心能力，未上线则领域 Agent 退化为裸 LLM
3. `ivf_index` 预建索引当前被忽略，性能优化几乎"免费"

---

## 2. 目标

### 2.1 总目标

让 Mady 生产链路默认消费 `~/.mady/knowledge/` 知识库，实现 **FTS + 向量 RRF 融合召回 + 模型重排** 的完整检索闭环，并支持用户自建文档向量化入索引。

### 2.2 阶段目标（本期覆盖阶段 1-3）

| 阶段 | 目标 | 一句话验收 |
|------|------|-----------|
| **阶段 1：接线 MVP** | 现有代码接入生产入口，默认开启 FTS+Vector RRF | 启动 mady 即可用知识检索，无 knowledge.db 时优雅降级 |
| **阶段 2：性能** | 复用预建 IVF 索引 + 接入 cross-encoder 重排 | 向量查询 < 50ms（当前数秒）；重排提升 Top-3 命中率 |
| **阶段 3：写入侧** | 用户文档可向量化入独立 user.db，参与检索 | 上传新文档后检索能命中 |

> 阶段 4（本地化备选/降级策略）与阶段 5（评测闭环）**不在本期**，作为后续迭代候选，但在设计中预留接口。

### 2.3 非目标（本期不做）

- 远程 embedding API 支持（默认本地 oMLX，远程作为未配置时的降级，不做主动优化）
- 知识库的批量重建/迁移工具（写入侧仅支持增量新增）
- 多语言向量索引（中英文混合由 BGE-M3 原生支持，不做额外分语言处理）

---

## 3. 成功标准

### 3.1 功能验收

| 编号 | 标准 | 验证方式 |
|------|------|----------|
| AC-1 | 不设任何知识库环境变量启动 `mady tui`，调用 `search_knowledge` 工具，返回 RRF 融合结果 | 手动 + 集成测试 |
| AC-2 | `/api/chat` 在 patent/legal 域自动注入"参考文档"上下文段 | e2e 测试 |
| AC-3 | 删除/重命名 `~/.mady/knowledge/knowledge.db` 后启动，不报错，降级为无知识检索 | 手动 |
| AC-4 | 向量查询走 IVF 索引，单次延迟 < 50ms（p95） | benchmark 测试 |
| AC-5 | 检索结果经过 cross-encoder 重排，Top-3 包含至少 1 条语义高度相关结果 | 评测集（阶段 3 同步建立基线） |
| AC-6 | 用户向 user.db 写入新文档 → 向量化 → 检索可命中 | 集成测试 |
| AC-7 | TUI/Serve/ACP 三个入口行为一致 | 各入口冒烟 |

### 3.2 质量验收

- `go build ./...` / `go vet ./...` / `go test -race ./...` 全绿
- `golangci-lint run` 零 issue
- 新增代码符合分层架构（领域层不 import 基础设施实现，详见 ADR-0001）
- 不引入硬编码密钥；API Key 走环境变量
- 不破坏现有 `WIKI_PATH` 内存模式（作为降级保留）

### 3.3 回归红线

- 现有 `knowledge/`、`retrieval/`、`knowledge/sqlite/` 包的公开 API 不做破坏性变更（只新增）
- `WIKI_PATH` 行为保持不变
- SQLiteStore 对 `knowledge.db` 保持**只读**（写入只落在 user.db）

---

## 4. 关键约束

1. **本地模型优先**：Embedding/Rerank 默认走本地 oMLX（`127.0.0.1:8000`），远程 API 仅作未配置降级
2. **只读与可写隔离**：`knowledge.db`（预构建，只读）与 `user.db`（用户新增，可写）物理分离，避免污染权威数据
3. **任意目录可用**：遵循 `MADY_HOME` 统一路径解析约定（AGENTS.md 资源定位 gotcha），不新增 cwd 相对路径
4. **安全敏感路径**：`tools/path.go` 沙箱边界、`knowledge/sqlite/store.go` 只读约束受影响时需人工审阅（L3）

---

## 5. 决策摘要（详见 03-design.md）

| 决策点 | 选择 | 备选 | 理由 |
|--------|------|------|------|
| Embedding 来源 | 本地 oMLX `bge-m3-mlx-8bit` | 远程 API | 与 knowledge.db 同源模型，零网络依赖 |
| 向量索引 | 复用 knowledge.db 预建 `ivf_index` | 新建 HNSW | 现成索引，零额外内存/构建成本 |
| 融合算法 | 现有 RRF（k=60） | 加权融合 | 已实现，score-agnostic 鲁棒 |
| 重排 | cross-encoder（oMLX Qwen3-Reranker） | 仅启发式 | 语义精度更高，Top-K 质量显著提升 |
| 写入库 | 独立 user.db | 写入 knowledge.db | 保护权威只读数据 |
| 配置方式 | 环境变量（沿用现有约定） | 配置文件 | 与 PROVIDER/API_KEY 约定一致 |

---

## 6. 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| oMLX 未启动时检索不可用 | 中 | 自动降级为纯 FTS（无需向量）；启动时探测并提示 |
| IVF 索引格式（BLOB）解析复杂 | 中 | 阶段 2 先实现，若格式不透明则保留暴力扫描作兜底 |
| knowledge.db 软链失效 | 低 | 启动时存在性检查，缺失则降级 |
| 写入侧并发安全 | 中 | user.db 单写者 + WAL 模式 |
| 重排增加延迟（每次额外 LLM 调用） | 中 | 仅对 Top-K（如 20）重排，可配置关闭 |

---

## 7. 下一步

人工 Sign-off 本提案后，进入 `02-spec.md`（详细规格）。规格中标记 `[NEEDS CLARIFICATION]` 的点需 Owner 决策后方可进入实现。
