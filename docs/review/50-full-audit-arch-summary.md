# 架构合规性审查报告

> **审查时间**: 2026-07-31
> **审查范围**: Mady 项目全部 Go 源文件，覆盖根模块 + tools + tui + desktop（不含 desktop/frontend/ 的 React/TypeScript 部分）
> **审查方法**: `go list` 依赖分析、`grep` 导入模式匹配、`go build` 编译验证、文件行数统计、接口定义位置抽查
> **参考标准**: `docs/GO-DEVELOPMENT-STANDARDS.md`（8 层分层架构 + §1.3 依赖方向）、`AGENTS.md`（Domain→Infrastructure 接口依赖原则）
> **文件统计**: 1405 Go 源文件（969 非测试 + 436 测试），~199,593 行非测试代码

---

## 总体评分：**B**

| 维度 | 评分 | 说明 |
|------|------|------|
| P0 合规 | **C** | 多项系统性的分层依赖方向违规，需架构层面解决 |
| P1 质量 | **A** | 包命名、接口位置、模块边界等均表现良好 |
| 整体 | **B** | 架构质量整体高于行业平均水平，但存在需要认真处理的架构债务 |

---

## P0 必查项

### 1. 8 层分层依赖方向违规 ⚠️ 告警（系统性问题）

**标准**: _上层可导入下层，反向禁止。_

**检查结果**: `build` 编译通过，但从架构规范角度看，存在以下系统性的反向依赖问题。

#### 1.1 基础设施层 → 核心引擎层（大规模反向依赖）

`graph/`, `session/`, `prompt/`, `store/`, `disclosure/`, `memory/`, `knowledge/`, `retrieval/`, `evaluate/`, `doomloop/`, `tracing/` 等基础设施层包均直接导入 `agentcore` 包。按架构规范，基础设施层位于核心引擎层之下，下层导入上层属于反向依赖。

```
(架构规定)           (实际情况)
核心引擎层               agentcore
    ↓                       ↑
基础设施层  ←←←  graph/ session/ prompt/ store/ disclosure/ memory/
                   knowledge/ retrieval/ evaluate/ doomloop/ tracing/
```

具体导入统计：

| 基础设施包 | 导入 agentcore? | 文件数 |
|-----------|----------------|--------|
| `session/` | ✅ | 4 个文件 |
| `prompt/` | ✅ | 2 个文件 |
| `store/` | ✅ | 1 个文件 |
| `disclosure/` | ✅ | 7 个文件 |
| `memory/` | ✅ | 7 个文件 |
| `knowledge/` | ✅ | 5 个文件 |
| `retrieval/` | ✅ | 2 个文件 |
| `evaluate/` | ✅ | 5 个文件 |
| `doomloop/` | ✅ | 1 个文件 |
| `tracing/` | ✅ | 1 个文件 |

**建议**: 这是一个全项目的架构风格问题，不宜零散修复。建议：
- 短期：明确文档中承认 agentcore 是"共享内核"而非严格的"上层"，更新架构图为双向/全向模式
- 长期：通过 `agentcore/iface/` 包逐步收窄基础设施对 agentcore 的具体类型依赖，推动面向接口编程

#### 1.2 提供者层 → 核心引擎层（反向依赖）

```
provider/chatcompat/  → agentcore
```

`provider/chatcompat/chat.go` 和 `provider/chatcompat/responses.go` 导入 `agentcore`。按架构规定提供者层在核心引擎层之下，属反向依赖。

**建议**: provider 依赖 agentcore 的核心类型（`Provider`, `ProviderRequest` 等）是实际需要。应考虑通过 `agentcore/iface` 提供这些类型，或明确在文档中将 provider 标注为"同级"而非"下层"。

#### 1.3 TUI 层 → 核心引擎层（反向依赖）

```
tui/agentadapter/adapter.go  → agentcore
```

TUI 层为最底层之一，直接导入 agentcore 核心引擎。这是 TUI 桥接 Agent 运行时的实际需求。

**建议**: 这是层间桥接的自然结果。保持现状但明确文档化 —— TUI agentadapter 是 TUI 与 agentcore 之间的"适配器桥梁"，允许有限例外。

#### 1.4 外部接口层 → 各层（允许的上→下依赖 ✅）

| 外部接口包 | 导入的目标层 | 合规性 |
|-----------|-------------|--------|
| `a2a/` → `agentcore` | 核心引擎层 | ✅ 允许（外部接口→核心引擎） |
| `a2ui/` → `a2a`, `agentcore`, `agui` | 核心引擎层 | ✅ 允许 |
| `agui/` → `agentcore` | 核心引擎层 | ✅ 允许 |
| `mcp/` → `agentcore`, `pkg/util` | 核心引擎层 | ✅ 允许 |
| `acp/` → `agentcore`, `domains`, `pkg` | 核心引擎+领域 | ✅ 允许 |
| `server/` → `agentcore`, `domains`, `a2ui`, `disclosure`, `graph`, `knowledge`, `retrieval`, `agui`, `session` | 多层级 | ✅ 允许（属于应用入口层，可依赖所有上层） |

#### 1.5 核心引擎层 → 下层（允许的上→下依赖 ✅）

| agentcore 导入 | 目标层 | 合规性 |
|---------------|--------|--------|
| `graph` | 基础设施层 | ✅ 允许 |
| `tools` | 工具层 | ✅ 允许 |
| `skill` | 基础设施层 | ✅ 允许 |
| `pkg/util` | 通用库 | ✅ 允许（属于公用层） |

---

### 2. Domain 层具体实现导入基础设施层 ❌ 不通过

**标准**: _Domain 层不得 import Infrastructure 层的具体实现，只能依赖接口。_

**检查结果**: domains/ 下多个子包直接导入基础设施层的具体类型（struct、函数调用），构成明确违规。

#### 违规清单

| Domain 包 | 导入的基础设施包 | 使用的具体类型/函数 |
|-----------|----------------|-------------------|
| `domains/claimdrafting/extension.go` | `graph` | `graph.CompiledPregelGraph`, `graph.PregelState` |
| `domains/enablement/tool.go` | `graph`, `disclosure` | `graph.PregelState`, `disclosure.AnalysisReport` |
| `domains/patent.go` | `disclosure`, `retrieval/domain` | `disclosure.NewDisclosureTool()` |
| `domains/prompt_store.go` | `prompt` | `prompt.PromptStore`, `prompt.SetDefaultStore()`, `prompt.ResolveSystemPrompt()` |
| `domains/lifecycle.go` | `doomloop` | `doomloop.New()`, `doomloop.WithToolCallLoop()` |
| `domains/regression.go` | `evaluate` | `evaluate.TestCase` |
| `domains/specdrafting/extension.go` | `disclosure`, `graph` | `disclosure` types, `graph` types |
| `domains/inventiveness/tool.go` | `disclosure`, `graph` | concrete types |
| `domains/infringement/tool.go` | `graph` | concrete types |
| `domains/novelty/tool.go` | `graph` | concrete types |
| `domains/workflows/patent/analysis.go` | `graph`, `retrieval/domain` | concrete types |
| `domains/workflows/patent/oa_response.go` | `graph`, `prompt` | concrete types |
| `domains/reasoning/sqlite/` | `store` | `store.CheckpointSQLStore` |
| `domains/sqlite/` | `store`, `graph` | concrete store/graph types |

> 注意：部分 domain 包（如 `domains/claimdrafting/drafter.go: Provider`、`domains/pending.go: PendingStore`、`domains/reasoning/rule_retrieval.go: RuleVectorStore`）已遵循"消费端定义接口"模式，定义了供下层实现的接口。这是正确的做法 ✅。但同一包中同时存在直接导入基础设施具体实现的违规代码。

**修复建议**:
1. 领域层定义接口（如 `domains/pending.go: PendingStore` 模式），让基础设施实现这些接口
2. 通过 DI（依赖注入）传入基础设施实现，而非直接 import
3. 将 `domains/sqlite/` 和 `domains/reasoning/sqlite/` 等存储实现迁至基础设施层（如 `knowledge/sqlite/` 模式）
4. `graph` 包引用尤其广泛 — 可考虑将 `graph` 提升为核心层的公有接口，或为 domain 创建 `domains/graph` 接口包

---

### 3. 循环依赖 ✅ 无问题

**标准**: _禁止包之间的循环依赖。_

| 模块 | go build | go list cycle check |
|------|---------|-------------------|
| 根模块 `.` | ✅ 通过 | ✅ 无循环 |
| `./tools` | ✅ 通过 | ✅ 无循环 |
| `./tui` | ✅ 通过 | ✅ 无循环 |
| `./desktop` | ✅ 通过 | ✅ 无循环 |

无 import cycle 报告。Go 编译器本身会阻止循环依赖，所有模块编译通过确认了这一点。

---

### 4. Monorepo 模块边界 ⚠️ 告警

**标准**: _根模块不得直接依赖 tools/ 或 tui/ 的内部实现（它们通过独立 go.mod + replace 隔离）。_

| 方向 | 是否允许 | 实际情况 |
|------|---------|---------|
| 根模块 → `tools/` | 允许（通过 go.mod replace） | ✅ 通过 go.mod `replace github.com/xujian519/mady/tools => ./tools` |
| 根模块 → `tui/` | 允许（通过 go.mod replace） | ✅ `replace github.com/xujian519/mady/tui => ./tui` |
| `tools/` → 根模块 | 允许（编译上，但影响外部可导入性） | ⚠️ `tools/go.mod replace github.com/xujian519/mady => ../` |
| `tui/` → 根模块 | 允许 | ⚠️ `tui/go.mod replace github.com/xujian519/mady => ../` |
| `desktop/` → 根模块+`tools/` | 允许 | ✅ 预期的桌面应用集成 |

**主要问题: 交叉双向依赖**

```
根模块 ─── import ──→ tools/（agentcore/permission/permission.go）
  ↑                        │
  └─── replace ../ ←───────┘ （tools/go.mod: replace github.com/xujian519/mady => ../）
```

root ↔ tools 形成编译层面的双向依赖链。在 monorepo 内通过 go.work 正常工作，但若外部用户 `go get github.com/xujian519/mady/tools`，会遇到依赖解析问题（tools 依赖根模块，根模块又依赖 tools）。

具体来说，`tools/tools.go` 导入了 `agentcore`（根模块包），而 `agentcore/permission/permission.go` 导入了 `tools`。这意味着：
- 外部用户要使用 `tools`，必须同时有根模块
- 但根模块又依赖 `tools` — 造成 chicken-and-egg 问题

**建议**:
1. 从 `agentcore/permission/permission.go` 中移除对 `tools` 包的直接依赖（当前仅用于 `tools.AllowAll` 等权限检查），改为在 `cmd/mady/` 入口层进行集成
2. 使 `tools` 模块仅单向依赖根模块（即只有 tools → root，没有 root → tools）

---

## P1 重要项

### 5. 包命名合规 ✅ 通过

| 规则 | 检查结果 |
|------|---------|
| 禁止 `common/` 包 | ✅ 无此类包 |
| 禁止 `utils/` 包 | ✅ 无此类包（`pkg/util` 在 `pkg/` 下，按 Go 惯例可为外部消费提供工具函数，属可接受边界情况） |
| 禁止 `base/` 包 | ✅ 无此类包 |
| 包名 = 目录名 | ✅ 各目录包名一致 |
| 禁止下划线包名 | ✅ 无（`_test.go` 后缀除外） |
| 禁止驼峰包名 | ✅ 无 |

**备注**: `pkg/i18n/translations/zh-CN/common.yaml` 和 `en-US/common.yaml` 是国际化数据文件（YAML），不是 Go 包名，不违反规则。

---

### 6. 接口定义位置 ✅ 基本符合

**标准**: _接口在消费端定义，不在生产端。生产端只返回具体类型。_

**抽查结果**: 抽查了 20+ 接口定义，大部分符合消费端定义原则。

#### 符合规范的示例 ✅

```
领域层（消费端）定义接口，基础设施层实现：
  domains/claimdrafting/drafter.go:24  → Provider interface
  domains/pending.go:41                → PendingStore interface
  domains/reasoning/rule_retrieval.go  → RuleVectorStore / IPCStandardSource
  domains/reasoning/checkpoint.go:29   → CheckpointStore interface
  domains/evidence/types.go:287        → EvidenceJudgmentEngine interface
  domains/workflows/patent/oa_response.go:620  → OARuleRetriever interface

agentcore 定义接口，外部模块依赖：
  agentcore/iface/     → 窄视图接口包（LifecycleHook, EventBus, AgentRunner）
```

`agentcore/iface/` 包的 `doc.go` 清晰地文档化了"接口收缩策略（Narrow View Strategy）"和"最小信息原则"，值得肯定。✅

#### 可改进的模式

```
graph/checkpoint.go: CheckpointStore 定义在基础设施层
  但 domains/reasoning/checkpoint.go 也定义了 CheckpointStore（不同方法签名）
  → 两者用途不同（graph 的 Pregel 执行 checkpoint vs 工作流 stage checkpoint）
  → 建议考虑是否可统一接口，或至少交叉引用
```

---

### 7. GOD 包残留 ✅ 通过

**标准**: _检查超过 500 行仍未拆分的包，或包名含 "utils" 变体。_

| 包 | 总行数 | 文件数 | 是否 GOD |
|----|--------|--------|---------|
| `domains/reasoning/` | 5612 | 21 | ❌ 拆分为多文件，职责清晰 |
| `domains/claimdrafting/` | 3912 | 16 | ❌ 同上 |
| `domains/evidence/` | 2873 | 18 | ❌ 同上 |
| `domains/specdrafting/` | 2690 | 14 | ❌ 同上 |
| `tui/component/` | 14949 | 多文件 | ❌ 多组件 |
| `tools/` | 14250 | 多文件 | ❌ 多工具 |
| `pkg/util` | ~20K | 6 | ❌ 文件数少、每个文件职责单一 |

超大单文件清单（>700 行）:

| 文件 | 行数 | 说明 |
|------|------|------|
| `tui/component/markdown.go` | 881 | TUI Markdown 渲染器 — 功能内聚 |
| `domains/workflows/patent/reexamination.go` | 822 | 复审流程 — 功能内聚 |
| `domains/workflows/patent/invalidation.go` | 796 | 无效宣告 — 功能内聚 |
| `mcp/discovery.go` | 752 | MCP 发现 — 规模偏大 |
| `domains/patent.go` | 727 | patent agent 配置 — 规模偏大 |
| `agui/converter.go` | 720 | 协议转换 — 规模偏大 |
| `provider/chatcompat/chat.go` | 716 | 聊天补全适配 — 规模偏大 |

**建议**: `mcp/discovery.go`（752行）、`domains/patent.go`（727行）、`agui/converter.go`（720行）、`provider/chatcompat/chat.go`（716行）可考虑拆分，但非紧急。

---

### 8. go.work 模块边界完整性 ✅ 通过

**标准**: _所有模块的 go.mod 是否正确配置。_

| 模块 | go.mod 路径 | module 声明 | 状态 |
|------|-----------|------------|------|
| 根模块 | `./go.mod` | `github.com/xujian519/mady` | ✅ |
| tools | `./tools/go.mod` | `github.com/xujian519/mady/tools` | ✅ |
| tui | `./tui/go.mod` | `github.com/xujian519/mady/tui` | ✅ |
| desktop | `./desktop/go.mod` | `github.com/xujian519/mady/desktop` | ✅ |

`go.work` 中 `use` 指令覆盖所有 4 个模块 ✅
所有 `go.mod` 均使用 `go 1.26` ✅
根模块与 tools/tui 之间的 `replace` 指令对称配置 ✅

---

## 汇总

| # | 检查项 | 优先级 | 结果 | 影响范围 |
|---|--------|--------|------|---------|
| 1 | 8 层分层依赖方向 | P0 | ⚠️ 告警 | 全项目（影响架构文档与实际的一致性） |
| 2 | Domain 层具体实现导入 | P0 | ❌ 不通过 | 15+ 个 domain 包（需要批量修复） |
| 3 | 循环依赖 | P0 | ✅ 通过 | — |
| 4 | Monorepo 模块边界 | P0 | ⚠️ 告警 | root ↔ tools 交叉依赖（影响外部可导入性） |
| 5 | 包命名合规 | P1 | ✅ 通过 | — |
| 6 | 接口定义位置 | P1 | ✅ 基本符合 | agentcore/iface/ 做法值得推广 |
| 7 | GOD 包残留 | P1 | ✅ 通过 | 4 个文件可考虑拆分（非紧急） |
| 8 | go.work 模块边界 | P1 | ✅ 通过 | — |

### 最紧急的修复项

1. **Domain → Infrastructure 具体实现导入**（P0 ❌）— 这是文档明确禁止且影响领域层纯净度的核心问题。建议分批修复：
   - 第一批：`domains/patent.go` 中对 `disclosure` 和 `retrieval/domain` 的具体依赖
   - 第二批：各 drafting 包（`claimdrafting`, `specdrafting`）对 `graph` 的具体类型依赖
   - 第三批：`domains/lifecycle.go` 中对 `doomloop` 的函数调用依赖

2. **root ↔ tools 交叉依赖**（P0 ⚠️）— `agentcore/permission/permission.go` 中对 `tools` 包的导入影响外部可导入性

3. **分层依赖方向与文档一致性**（P0 ⚠️）— 建议架构负责人（Rust）决策：是调整代码适配文档，还是调整文档反映实际

### 值得推广的最佳实践

- ✅ `agentcore/iface/` 窄视图接口包的`接口收缩策略`设计模式
- ✅ 多数 domain 子包遵循"消费端定义接口"原则（`pending.go: PendingStore`, `evidence/types.go: EvidenceJudgmentEngine` 等）
- ✅ 外部接口层（a2a, a2ui, agui, mcp, acp）均不产生反向依赖，边界清晰
- ✅ 4 个构建模块均可独立编译通过，`go build ./...` 在所有模块上零错误

---

## 评级标准

| 等级 | 说明 |
|------|------|
| **A** | 所有 P0 通过，P1 无系统性问题 |
| **B** | P0 有非致命问题（可短期修复），P1 良好 |
| **C** | P0 存在系统性问题（需架构级改动） |
| **D** | P0 多项不通过，架构存在根本缺陷 |
