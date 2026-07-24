# prompt-templates 全接线设计

> 目标：将 `prompt-templates/` 从当前"数据沉睡"状态完全接入 Mady 运行时，使所有 Agent / 工作流 / 基础设施的提示词可集中管理、版本追踪、用户覆盖，并保持对现有内联提示词的向后兼容。

## 1. 现状

- `prompt/` 包已提供加载、渲染、触发词匹配 API（`LoadPrompts` / `ResolvePrompt` / `FindPromptBy*`）。
- `prompt-templates/` 已存放 20 个 curated JSON 模板，覆盖检索 / 分析 / 撰写 / OA / 交底书 / 法律等场景。
- 当前没有任何 Go 代码 import `mady/prompt`，也没有代码引用 `prompt-templates/` 路径。
- AI_CHANGELOG 已两次记录：OA / disclosure 等节点的 SystemPrompt 仍内联在 Go 代码中，**未使用**模板系统。

## 2. 设计原则

1. **复用已有 API**：以 `prompt.PromptTemplate` / `ResolvePrompt` 为数据契约，不另造一套模型。
2. **复用项目惯例**：参考 `agentcore/manifests/` 与 `domains/doctmpl/` 的 `go:embed` + 用户目录覆盖机制。
3. **向后兼容**：不强制迁移所有内联提示词；找不到模板时回退到现有字符串，不阻断启动。
4. **最小侵入**：先建基础设施（Store + 框架注入），再逐层迁移调用方。
5. **单改动收敛**：每个 PR 聚焦一个接线点，避免"大爆炸"式提交。

## 3. 关键决策点

### 3.1 目录位置：是否移动 `prompt-templates/`？

Go 的 `//go:embed` 只能嵌入**当前包目录及其子目录**中的文件，不能引用 `..` 或包外目录。

- `prompt/` 包位于 `prompt/`
- `prompt-templates/` 位于仓库根

因此无法直接在 `prompt/` 中嵌入 `prompt-templates/`。

**推荐方案 A（与本项目其他内嵌资源一致）**：

```text
prompt/
  loader.go / prompt.go / ...
  templates/              ← 将 prompt-templates/ 移入此处
    analysis/
    drafting/
    ...
```

- `prompt/embed.go` 使用 `//go:embed templates/**/*.json`
- 用户覆盖目录仍为 `$MADY_HOME/prompt-templates/`（与 doctmpl 的 `$MADY_HOME/doc-templates/` 对称）
- 需同步更新 `CLAUDE.md`、`architecture.html`、AI_CHANGELOG 中的目录引用

**备选方案 B**：
在根目录新建 `assets/` 包负责嵌入，然后 `prompt/` 通过 `assets` 包读取。此方案增加一个新包，且打破"哪个包消费数据就把数据放哪"的现有惯例。

**建议采用方案 A**。

### 3.2 模板引用语法

为了让 `agentconfig.Config.SystemPrompt` 等字段既能填内联字符串，也能引用模板，引入 URI 前缀：

```yaml
# 内联（保持现有行为）
system_prompt: "你是 helpful assistant"

# 引用模板
system_prompt: "prompt://claim-drafting"
```

识别规则：

- 以 `prompt://` 开头 → 从 `PromptStore` 查找并渲染
- 其它值 → 视为内联 prompt，原样使用

### 3.3 PromptStore 接口

新增 `prompt/store.go`：

```go
type PromptStore struct { /* embedded + overlay + byName */ }

func NewPromptStore(userRoots ...string) (*PromptStore, error)
func (s *PromptStore) FindByName(name string) (PromptTemplate, bool)
func (s *PromptStore) Resolve(name string, vars map[string]string) (ResolvedPrompt, bool)
func (s *PromptStore) FindByTrigger(keyword string) []PromptTemplate
func (s *PromptStore) List(opts ListOptions) []PromptTemplate
func (s *PromptStore) Count() int
func (s *PromptStore) Index() string
```

其中 `ListOptions` 支持按 `Domain`、`Category` 和关键词过滤，便于后续工具/CLI 展示。

## 4. 接线阶段

### Phase 0 — 决策与准备

- [ ] 确认采用方案 A 移动目录
- [ ] 在 `docs/specs/prompt-templates-wiring/02-tasks.md` 中细化每个子任务的文件清单
- [ ] 创建 feature branch

### Phase 1 — prompt 包升级：Store + Embed + 覆盖

改动文件：

- `prompt/embed.go`（新增）：`//go:embed templates/**/*.json`
- `prompt/store.go`（新增）：`PromptStore` + `NewPromptStore` + 查询 API
- `prompt/loader.go`：补充 `LoadPromptsFromFS(fs.FS, root string)` 以支持 embed.FS
- `prompt/loader_test.go` / `prompt/store_test.go`（新增）：覆盖 embed、覆盖、查重、变量解析
- `prompt/templates/`（从 `prompt-templates/` 移动）：20 个 JSON 文件
- `CLAUDE.md`、`architecture.html`、`docs/decisions/AI_CHANGELOG.md`：更新目录引用

验证：

- `go test ./prompt/...` 通过
- 用户覆盖 `$MADY_HOME/prompt-templates/` 时，同名模板优先

### Phase 2 — 框架注入：fc.PromptStore

改动文件：

- `cmd/mady/framework.go`：
  - `frameworkContext` 新增 `PromptStore *prompt.PromptStore`
  - 在 `initReasoningAndTemplates` 中创建 `prompt.NewPromptStore(filepath.Join(fc.MadyHome, "prompt-templates"))`
  - 失败时打印警告，不影响启动
- `domains/` 增加 `SetupPromptStore(*prompt.PromptStore)`（新增 `domains/prompt_store.go`）

验证：

- `go build ./...`
- `mady tui` / `mady serve` 启动日志显示已加载内置 prompt 模板数量

### Phase 3 — AgentConfig 支持模板引用

改动文件：

- `pkg/agentconfig/config.go`：
  - 保持 `SystemPrompt string` 不变
  - 新增 `SystemPromptTemplate string` 字段（可选，显式声明模板名）
  - 或在 `agentconfig` 中不新增字段，仅在解析/构建阶段识别 `prompt://` 前缀
- 推荐后者，因为不新增字段即可复用现有 YAML/JSON 配置。

消费侧：

- 在 `cmd/mady` 构建 Agent 之前，调用 `promptStore.Resolve()` 将 `prompt://xxx` 解析为实际字符串
- 若解析失败，保留原字符串并记录警告

验证：

- 新增 `pkg/agentconfig` 测试：URI 解析与回退

### Phase 4 — 逐层迁移内联 Prompt

按影响面从大到小、风险从低到高分批迁移。每批一个 PR：

#### 4.1 工作流节点（高价值、低风险）

- `workflows/patent/oa_response.go`：OA 答复 system prompt
- `disclosure/novelty.go`：新颖性分析 system prompt
- `disclosure/keywords.go`：关键词提取 system prompt

做法：

1. 将当前 `buildXxxPrompt()` 的字符串提取为 `prompt/templates/xxx.json`
2. 在节点构造时改为 `fc.PromptStore.Resolve("xxx", vars)`
3. 保留"模板未找到则使用内联常量"的兜底

#### 4.2 基础设施模块

- `memory/extractor_llm.go`
- `memory/session_summarizer.go`
- `memory/dedup_llm.go`
- `evaluate/llm_judge.go`
- `guardrails/guardian/guardian.go`

这些模块通常不持有 `frameworkContext`。需要决定：

- **方案 A**：在模块内新增 `SetSystemPromptTemplate(name string)` 风格的 setter，由 `cmd/mady` 在初始化时注入。
- **方案 B**：为模块新增一个轻量 `SystemPromptResolver func(name string) (string, error)` 回调，延迟解析。
- **方案 C**：保持这些模块的内联 prompt 不变，仅迁移 Agent 级 system prompt。这是风险最低的中间状态。

**建议先完成 4.1，再评估 4.2 是否必要。**

### Phase 5 — 可选增强（非阻塞）

- 新增 `list_prompts` 工具，让 Agent 可以枚举可用模板
- 新增 `mady util list-prompts` 子命令
- 在 TUI 中展示 prompt 模板索引

## 5. 安全与合规

- 不修改 `agentcore/handoff.go`、`guardrails/levels.go`、`tools/bash.go` 等敏感路径。
- `pkg/agentconfig` 改动仅涉及字符串解析，不改变护栏等级或权限决策。
- 用户覆盖 `$MADY_HOME/prompt-templates/` 与现有 `doc-templates/`、`manifests/` 的沙箱策略一致：仅替换提示词内容，不执行代码。

## 6. 验证门禁

每个 Phase 完成后必须：

- `go build ./...`
- `cd tools && go build ./...`
- `go test -race ./...`
- `golangci-lint run`
- 手动验证：启动 `mady tui`，确认日志输出 `prompt: 已加载 N 个内置模板`

## 7. 回滚策略

- 所有迁移点保留"模板未找到 → 使用内联常量"的兜底分支。
- 若某 Phase 引入回归，可单独 revert 该 Phase，不影响其他已接线部分。
- 目录移动（Phase 1）如导致问题，可通过 git revert 整包恢复。

## 8. 工作量预估

| Phase | 预计文件数 | 预计时间 |
|-------|-----------|---------|
| Phase 0 | 0-2 | 1 天 |
| Phase 1 | 6-8 | 1-2 天 |
| Phase 2 | 2-3 | 0.5 天 |
| Phase 3 | 2-4 | 1 天 |
| Phase 4.1 | 3-4 | 1-2 天 |
| Phase 4.2 | 5-7 | 2-3 天（可选）|
| Phase 5 | 2-4 | 1-2 天（可选）|
