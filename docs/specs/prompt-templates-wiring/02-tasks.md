# prompt-templates 全接线任务分解

> 对应设计文档：`01-design.md`

## Phase 0 — 决策与准备

| # | 任务 | 决策产出 | 负责人 |
|---|------|---------|--------|
| 0.1 | 确认目录移动方案（`prompt-templates/` → `prompt/templates/`） | 采用方案 A | 待分配 |
| 0.2 | 创建 feature branch | branch 名如 `feat/prompt-templates-wiring` | 待分配 |
| 0.3 | 通知相关文档维护者（CLAUDE.md / architecture.html） | 变更清单 | 待分配 |

## Phase 1 — prompt 包升级：Store + Embed + 覆盖

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 1.1 | `prompt/templates/` | 从 `prompt-templates/` 移动全部 JSON 文件，保持子目录结构 | `ls prompt/templates/` 与原 `prompt-templates/` 一致 |
| 1.2 | `prompt/embed.go` | 新增：`//go:embed templates/**/*.json` + `embeddedPromptsFS embed.FS` + `embeddedPromptsDir = "templates"` | `go build ./prompt/...` 通过 |
| 1.3 | `prompt/loader.go` | 新增 `LoadPromptsFromFS(fsys fs.FS, root string) ([]PromptTemplate, error)` | 可读取 embed.FS |
| 1.4 | `prompt/store.go` | 新增 `PromptStore`：嵌入加载 + 用户目录覆盖 + `byName` 索引 + `FindByName` / `Resolve` / `FindByTrigger` / `List` / `Count` / `Index` | 线程安全，用户覆盖优先 |
| 1.5 | `prompt/store_test.go` | 覆盖：内置加载、用户覆盖、查重、变量解析、并发读 | `go test -race ./prompt/...` 通过 |
| 1.6 | `prompt/loader_test.go` | 补充 `LoadPromptsFromFS` 测试 | 通过 |
| 1.7 | `CLAUDE.md` | 更新目录结构描述：`prompt/templates/` | 文档与代码一致 |
| 1.8 | `architecture.html` | 更新 prompt-templates 卡片路径 | 图表显示正确目录 |
| 1.9 | `docs/decisions/AI_CHANGELOG.md` | 追加 Phase 1 变更记录 | 记录完成 |

## Phase 2 — 框架注入：fc.PromptStore

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 2.1 | `cmd/mady/framework.go` | `frameworkContext` 新增 `PromptStore *prompt.PromptStore` | 编译通过 |
| 2.2 | `cmd/mady/framework.go` | `initReasoningAndTemplates` 中创建 `prompt.NewPromptStore(filepath.Join(fc.MadyHome, "prompt-templates"))` | 启动日志打印加载数量 |
| 2.3 | `domains/prompt_store.go` | 新增 `var promptStore *prompt.PromptStore` + `SetupPromptStore(s *prompt.PromptStore)` + `PromptStore()` getter | 可被 domains 子包调用 |
| 2.4 | `cmd/mady/framework.go` | 调用 `domains.SetupPromptStore(fc.PromptStore)` | 不破坏现有 domains 包依赖方向 |
| 2.5 | AI_CHANGELOG | 追加 Phase 2 记录 | 完成 |

## Phase 3 — AgentConfig 支持模板引用

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 3.1 | `pkg/agentconfig/config.go` | 决策：不新增字段，复用 `SystemPrompt` 的 `prompt://` 前缀识别 | 无新增字段 |
| 3.2 | `pkg/agentconfig/resolve.go`（或复用 `load.go`） | 新增 `ResolveSystemPrompt(raw string, store *prompt.PromptStore) (string, error)` | `prompt://name` 解析，普通字符串原样返回 |
| 3.3 | `pkg/agentconfig/config_test.go` | 新增测试：`ResolveSystemPrompt` 的模板解析与内联回退 | 通过 |
| 3.4 | `cmd/mady/framework.go` | 在构建 Agent 前，对 `SystemPrompt` 调用 `ResolveSystemPrompt` | `mady tui` 正常启动 |
| 3.5 | AI_CHANGELOG | 追加 Phase 3 记录 | 完成 |

## Phase 4.1 — 迁移工作流节点内联 Prompt

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 4.1 | `prompt/templates/workflow/oa-response.json` | 新建：提取 `workflows/patent/oa_response.go` 的 system prompt | JSON 校验通过 |
| 4.2 | `workflows/patent/oa_response.go` | 改为从 `domains.PromptStore()` 解析 `oa-response` 模板；失败时使用内联常量 | `go test ./workflows/patent/...` 通过 |
| 4.3 | `prompt/templates/disclosure/novelty-analysis.json` | 新建：提取 `disclosure/novelty.go` 的 system prompt | JSON 校验通过 |
| 4.4 | `disclosure/novelty.go` | 改为模板解析；保留内联兜底 | `go test ./disclosure/...` 通过 |
| 4.5 | `prompt/templates/disclosure/keyword-extraction.json` | 新建：提取 `disclosure/keywords.go` 的 system prompt | JSON 校验通过 |
| 4.6 | `disclosure/keywords.go` | 改为模板解析；保留内联兜底 | `go test ./disclosure/...` 通过 |
| 4.7 | AI_CHANGELOG | 追加 Phase 4.1 记录 | 完成 |

## Phase 4.2 — 迁移基础设施模块内联 Prompt（可选）

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 4.8 | `memory/extractor_llm.go` | 评估是否接入 PromptStore；若接入则新增 setter / resolver | 测试通过 |
| 4.9 | `memory/session_summarizer.go` | 同上 | 测试通过 |
| 4.10 | `memory/dedup_llm.go` | 同上 | 测试通过 |
| 4.11 | `evaluate/llm_judge.go` | 同上 | 测试通过 |
| 4.12 | `guardrails/guardian/guardian.go` | 同上（注意：guardian 属 Guardian AI 熔断器，需人工审阅） | 测试通过 |
| 4.13 | AI_CHANGELOG | 追加 Phase 4.2 记录 | 完成 |

## Phase 5 — 可选增强

| # | 文件 | 具体改动 | 验收标准 |
|---|------|---------|---------|
| 5.1 | `tools/list_prompts.go`（或新增） | 新增 `list_prompts` 工具 | Agent 可调用的 JSON 输出 |
| 5.2 | `cmd/mady/` | 新增 `mady util list-prompts` 子命令 | CLI 输出模板索引 |
| 5.3 | `tui/chat/` | 在 TUI 中展示可用 prompt 模板（可选） | 不影响主流程 |
| 5.4 | AI_CHANGELOG | 追加 Phase 5 记录 | 完成 |

## 全局验收

每个 Phase 完成后必须执行：

```bash
make verify
# 或等价：
go build ./...
cd tools && go build ./... && cd ..
go test -race ./...
golangci-lint run
```

并追加 `docs/decisions/AI_CHANGELOG.md` 记录。
