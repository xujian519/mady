# 专利撰写流程修复与优化计划

> 基于代码审查发现：Patent Agent ↔ FiveStepRunner 完全隔离、`analyze_patent_novelty` 未注册、
> `SetupCitationWiring` 从未调用、`patent_lookup` 等工具依赖外部 CLI 且无显式配置、
> Skill 搜索路径不包含 `~/.agents/skills/`、无法端到端走通专利撰写流程。
>
> 目标：用最小改动（5 个 P0-P1 任务，每个 1-3 文件）使发明专利撰写端到端可运行。

---

## 总体策略

| 因素 | 策略 |
|------|------|
| 每次改动范围 | ≤5 文件（遵守 AGENTS.md 任务粒度约束） |
| 优先修复装配缺陷 | 先让现有代码可用，再添加新功能 |
| 验证 | 每阶段均 `go build` + `go test` 确认 |
| 风险控制 | 集成模式（默认）改动先于 Router 模式 |
| 决策记录 | 每阶段完成后在 `AI_CHANGELOG.md` 追加记录 |

---

## Phase 1 — P0 阻断级修复（3 个 Sprint）

### Sprint 1.1: 注册 `analyze_patent_novelty` 工具

**目标**：LLM 可在 Patent Agent 中调用 `analyze_patent_novelty`。

**当前问题**：`workflows/patent/tool.go` 的 `NewPatentNoveltyTool()` 定义存在，但从未注册。

**改动方案**：在 `tools/tools.go` 的 `BuildTools()` 中添加 `analyze_patent_novelty` 工具。
但该工具在 `workflows/patent` 包中，而 `tools/tools.go` 在 `tools` 包中。
为避免循环依赖（`tools` 不能 import `workflows/patent`），采用注入模式：

**方案**：在 `tools.ExtensionConfig` 中添加 `ExtraTools []*agentcore.Tool` 字段，
`BuildTools` 将它们追加到返回的工具列表中。Patent Agent 通过此字段注入该工具。

| 改动文件 | 改动内容 |
|----------|----------|
| `tools/tools.go` | `ExtensionConfig` 新增 `ExtraTools []*agentcore.Tool`，`BuildTools` 末尾追加 |
| `domains/patent.go` | `PatentAgentConfig` 创建 `toolExt` 时传入 `ExtraTools: []*agentcore.Tool{patent.NewPatentNoveltyTool()}` |

**验证**：`go build ./...` + `go test ./tools/...` + Patent Agent 工具列表含 `analyze_patent_novelty`。

---

### Sprint 1.2: Wire FiveStepRunner into PatentAgentConfig

**目标**：Patent Agent（Router Handoff 模式下创建的实例）能够调用 `run_five_step_workflow` 工具。

**当前问题**：FiveStepRunner 仅由 `tui_session.go:applyPersistence()` 在案件模式（`/case`）下注入。
Router Handoff 创建的子 Agent 通过 `agentcore.New(PatentAgentConfig(base)).Run()` 启动，
这段路径完全不经过 `applyPersistence()`。

**改动方案**：

1. `domains/patent.go` 中添加包级别单例注入：
   ```go
   var globalDrafting struct {
       once      sync.Once
       runner    *reasoning.FiveStepRunner
   }

   func SetupPatentDraftingEngine(retriever *reasoning.MultiSourceRetriever, llm reasoning.LlmClient) {
       globalDrafting.runner = reasoning.NewWorkflowRunner(
           "router-patent", reasoning.CaseDrafting, "", retriever, llm,
       )
   }
   ```

2. `PatentAgentConfig()` 从全局获取 runner 并注入为工具：
   ```go
   if globalDrafting.runner != nil {
       cfg.Tools = append(cfg.Tools, reasoning.AsWorkflowTool(globalDrafting.runner))
   }
   ```

3. `cmd/mady/framework.go` 的 `setupFrameworkContext()` 末尾调用 `SetupPatentDraftingEngine`。

| 改动文件 | 改动内容 |
|----------|----------|
| `domains/patent.go` | 新增 `SetupPatentDraftingEngine()` + 全局 runner + 在 `PatentAgentConfig()` 追加工具 |
| `cmd/mady/framework.go` | 末尾调用 `SetupPatentDraftingEngine` |

**验证**：`go build ./...` 编译通过；`go test ./domains/...` 测试通过。

---

### Sprint 1.3: 调用 SetupCitationWiring

**目标**：引用核验 Gate 运行在 P2b Strict 模式（S2 知识源 + 留痕 store）。

**当前问题**：`SetupCitationWiring()` 定义在 `domains/citation_wiring.go`，但从未被调用。
`newCitationGate` 内部 `currentCitationWiring()` 始终返回零值 → `Source=nil`（退回到 S1 静态表）、
`Store=nil`（命中疑点不写留痕）。

**改动方案**：

```go
// cmd/mady/framework.go setupFrameworkContext() 末尾
citationSource := buildCitationSource(fc) // S1+S2 复合知识源
approvalStore := openApprovalStore(fc.MadyHome)
domains.SetupCitationWiring(domains.CitationWiring{
    Source: citationSource,
    Store:  approvalStore,
})
```

| 改动文件 | 改动内容 |
|----------|----------|
| `cmd/mady/framework.go` | 末尾注入引用核验装配 |

**验证**：`newCitationGate` 内部可读取到非零值的 `CitationSource`。

---

## Phase 2 — P1 高优修复（3 个 Sprint）

### Sprint 2.1: 修正 PatentAgentConfig 工具配置

**目标**：移除不恰当的 `ComputerUse`、显式配置 `PatentTool`。

```go
// domains/patent.go PatentAgentConfig()
toolExt := tools.NewExtension(tools.ExtensionConfig{
    WorkingDir:     workingDir,
    SandboxEnabled: true,
    Vision: &tools.VisionToolConfig{
        Provider: base.Provider,
        Model:    base.Model,
    },
    WebSearch:   &tools.WebSearchToolConfig{},
    WebFetch:    &tools.WebFetchToolConfig{},
    // 移除 ComputerUse: true
    PatentTool:  tools.PatentToolConfigDefaults(), // 显式配置
    DisableTools: []string{
        tools.ToolBash, tools.ToolGitStatus, tools.ToolGitDiff, tools.ToolGitLog,
        tools.ToolBrowser, tools.ToolExecuteCode,
    },
    MaxBytes: 100 * 1024,
})
```

| 改动文件 | 改动内容 |
|----------|----------|
| `domains/patent.go` | 移除 `ComputerUse: true`、添加 `PatentTool` 配置 |

**验证**：`patent_lookup` 等工具在运行时正确初始化。

---

### Sprint 2.2: 扩展 Skill 搜索路径

**目标**：`~/.agents/skills/patent-legal/patent-drafting-v2/SKILL.md` 能被自动发现。

```go
// framework.go setupFrameworkContext()
agentSkillsDir := filepath.Join(homeDir, ".agents", "skills")
skillPaths = append(skillPaths, agentSkillsDir)
```

| 改动文件 | 改动内容 |
|----------|----------|
| `cmd/mady/framework.go` | `skillPaths` 追加 `~/.agents/skills/` |

**验证**：启动日志显示 `patent-drafting-v2` 被加载。

---

### Sprint 2.3: 添加 CaseDrafting 端到端集成测试

**目标**：覆盖 Patent Agent + FiveStepRunner + `patent_drafting_default` 的 claim drafting 场景。

新建 `integration/drafting_e2e_test.go`（带 `//go:build integration` 标签）：

| 测试 | 场景 |
|------|------|
| `TestDrafting_WorkflowTool` | 直接调用 `AsWorkflowTool(runner)` 验证 drafting 5 步 |
| `TestDrafting_PatentAgentHandoff` | Mock LLM + PatentAgentConfig 验证 handoff 路径 |
| `TestDrafting_CitationGateWired` | 验证 `SetupCitationWiring` 已生效 |

**验证**：`go test -tags integration ./integration/... -run 'TestDrafting'` 全部通过。

---

## Phase 3 — P2 中期优化（按需实施）

### Sprint 3.1: YAML 驱动的 WorkflowManifest 加载

- 创建 `~/.mady/workflows/` 目录及其 `.yaml` 文件
- `framework.go` 启动时调用 `store.LoadDir(workflowDir)`
- Manifest 优先级：YAML > 内置默认值

### Sprint 3.2: 说明书撰写工具

- 新增 `workflows/patent/specification.go`，定义说明书各章节 Pregel 图
- 新增 `tools/specification.go`，封装为 `specification_drafter` 工具

### Sprint 3.3: 审查员模拟辩论

- 新增 `workflows/patent/debate.go`，实现审查员-代理人辩论 Pregel 图
- 使用双 LLM 角色（审查员、代理人），至少 3 轮交互

---

## 依赖关系

```
Phase 1.1 (ExtraTools)
  │
  ▼
Phase 1.2 (FiveStepRunner) ─── 依赖 Phase 2.1 (修正工具配置)
  │
  ▼
Phase 1.3 (CitationWiring)
  │
  ▼
Phase 2.2 (Skill路径) ──── 独立
  │
  ▼
Phase 2.3 (集成测试) ──── 依赖 Phase 1.1+1.2+1.3 全部完成
```

各 Phase 可部分并行：1.1+1.3+2.1+2.2 可并行；1.2 需要在 2.1 之后；2.3 需要在 1.1-1.3 之后。

---

## 风险与缓解

| 风险 | 可能 | 影响 | 缓解措施 |
|------|------|------|----------|
| `NewWorkflowRunner` 的 LLM 依赖导致回退 | 中 | FiveStepRunner 降级为 noop | `NewLLMNodeBuilder` 在 `llm==nil` 时自动降级 |
| `nuo-patent` CLI 不可用 | 高 | `patent_lookup` 等工具失败 | 工具内部错误消息提示 |
| `CaseDrafting` manifest 的 Stage 3 为空 | 低 | Plan 生成跳过 | Planner 有默认 template 路径 |
| 全局 runner 并发安全 | 中 | 多 Agent 共享同一 runner | `AsWorkflowTool` 每次调用新建执行路径，runner 本身无状态 |

---

## 文件改动索引

| 文件 | Phase | 改动类型 |
|------|-------|----------|
| `tools/tools.go` | 1.1 | 新增 `ExtraTools` 字段 |
| `domains/patent.go` | 1.1, 1.2, 2.1 | 新增 setter + runner 注入 + 工具配置修正 |
| `cmd/mady/framework.go` | 1.2, 1.3, 2.2 | 注入点（3 处） |
| `domains/citation_wiring.go` | 1.3 | 无改动（仅调用侧） |
| `integration/drafting_e2e_test.go` | 2.3 | 新增文件 |
| 新文件 | 3.1 | `~/.mady/workflows/*.yaml` |
| 新文件 | 3.2 | `workflows/patent/specification.go` |
| 新文件 | 3.3 | `workflows/patent/debate.go` |

---

## 验收标准

| 标准 | 验证方式 |
|------|----------|
| `analyze_patent_novelty` 出现在 Patent Agent 工具列表 | 单测检查 `cfg.Tools` |
| FiveStepRunner 作为工具出现在 Patent Agent 工具列表 | 单测检查 |
| `newCitationGate` 读取非零值 CitationWiring | 单元测试 |
| `patent_lookup` 等工具使用显式配置而非 PATH 回退 | 单测检查 `PatentTool` 值 |
| 启动日志显示 `patent-drafting-v2` 被加载 | 人工运行验证 |
| `go test -tags integration -run TestDrafting` 全部通过 | CI 验证 |
