## 2026-07-19: 四大证据源全面接线修复

### 背景
此前对编排层地基和证据接入点的分析发现三个结构性问题：
1. disclosure 分析管线的 `retrieve_prior_art` 节点从未接入 `PatentDomainRetriever`，新颖性评估降级为纯 LLM 自身知识
2. `scholar_search`（学术论文检索）和 `search_knowledge`（知识库检索）虽然在 tools 层注册，但无 Agent 的 SystemPrompt 提及，LLM 不知道何时调用
3. `BuildProjectAgent` 案件模式搜索工具依赖 base extension 隐式提供，缺少显式声明

### 变更

#### 1. disclosure 管线接入 PatentDomainRetriever（P0）
- **`server/server.go`**：新增 `disclosureRetriever atomic.Pointer[domain.DomainRetriever]` 字段 + `SetDisclosureRetriever`/`getDisclosureRetriever` 方法
- **`server/disclosure.go`**：`submitTask`/`executeTask` 沿参数链传递 `domain.DomainRetriever`；`executeTask` 调用 `BuildDisclosureAnalysisGraphWithOpts(provider, WithRetriever(retriever))` 替代原 `BuildDisclosureAnalysisGraph(provider)`
- **`cmd/mady/server.go`**：从 `fc.KnowledgeBackend` 类型断言为 `*knowledge/sqlite.SQLiteStore`，构建 `retrieval/domain/sqlite.PatentDomainRetriever` 并注入 Server

#### 2. SystemPrompt 补充搜索工具指引（P1）
- **`domains/assistant.go`**：职责列表新增 `scholar_search` 和 `search_knowledge / search_laws`
- **`domains/patent.go`**：五步工作法第 2 步新增 `scholar_search` 和 `search_knowledge / search_laws`；第 4 步显式提到 `patent_lookup`
- **`domains/legal.go`**：可用工具列表新增 `scholar_search` 和 `search_knowledge / search_laws`，区分互联网检索与本地知识库检索
- **`domains/chat.go`**：IntegratedChatConfig 的路由描述中补充学术论文检索

#### 3. BuildProjectAgent 搜索工具探究
经代码追踪，`BuildProjectAgent` 的 Agent 实例已通过 `setupFrameworkContext` 的 `baseTools` extension 获得 `web_search`/`web_fetch`/`scholar_search`/`patent_lookup` 等搜索工具（base extension 的 `BuildTools` 无 `EnabledTools` 过滤），无需额外修改。

### 架构决策
- **disclosure 图构建模式从单参扩展为可选 option 链**：`BuildDisclosureAnalysisGraph(provider)` 仍然保持向后兼容（降级为无检索器），新代码路径调用 `BuildDisclosureAnalysisGraphWithOpts(provider, WithRetriever(...))` 激活检索
- **PatentDomainRetriever 依赖注入路径**：`frameworkContext.KnowledgeBackend` → 类型断言 `*SQLiteStore` → `NewPatentDomainRetriever(store)`，复用已在 `loadWikiStore` 中打开的数据库连接，避免二次打开
- **evidence_coverage 传递链路**：`retrieve_prior_art` 节点检测到 retriever != nil 后执行真实 FTS5 检索 → `evidence_coverage` 设为 `"patent_database"` → `check_novelty` 节点据此注入证据上下文至 LLM、而非纯 LLM 知识降级

### 验证
- `go build ./server/... ./cmd/mady/... ./domains/... ./disclosure/...` ✅
- `go test ./disclosure/... ./server/... ./domains/...` ✅
- `go test ./tools/... ./knowledge/... ./retrieval/... ./agentcore/...` ✅

---

## 2026-07-19: A2UI 模块审阅与完备化

### 背景
对 a2ui 包（Agent-to-UI 协议 v0.9.1 实现）进行"完整可运行"审阅，发现：
- 模块自身质量极高（100% 测试覆盖率、零 vet 警告），但 **完全未被运行时集成**（零外部调用者）
- `agui/converter.go` 默认分支错误地传递了完整 A2UIEvent 结构体而非仅信封数据

### 变更

#### 1. 模块自身改进（P1）
- **静默吞错修复**：`UpdateDataModel.UnmarshalJSON` 丢弃 `json.Unmarshal` 错误 → 显式检查
- **补齐 Builder 构造函数**：新增 `Video`、`AudioPlayer`、`Tabs`、`Modal`、`DateTimeInput`、`ChoicePicker` 6 个便捷构造函数
- **新增 `NewCheck` 构造函数**：简化客户端校验声明
- **100% 覆盖率保持**：新增 20+ 测试

#### 2. 运行时集成（P2）
- **新增 `agentcore.A2UIEvent` 事件类型**（EventType = "a2ui"）：承载序列化的 A2UI 信封，避免循环依赖
- **新增 `a2ui.ToAgentCoreEvent()` 桥接函数**：将 Envelope 转为 EventBus 兼容事件
- **修复 `agui/converter.go` 默认分支**：添加 `case *agentcore.A2UIEvent`，传递 `ev.Envelope` 而非整个结构体，保留原始时间戳
- **消除冗余序列化**：`ToAgentCoreEvent` 复用已有的 `envelopeToMap` 函数

#### 3. 测试完备化
- **agentcore A2UIEvent 单元测试**：创建、事件总线发射与接收
- **agui 转换器 A2UIEvent 测试**：验证 CUSTOM 事件名称/值正确性
- **端到端 SSE 流测试**（`TestE2E_A2UIStream`）：通过生命周期钩子发射 A2UIEvent，验证其在 AG-UI SSE 流中呈现为 `CUSTOM (name: "a2ui")` 事件

#### 4. 示例程序（P3）
- **新增 `example/a2ui-demo/`**：演示 Builder API、数据绑定、AG-UI/A2A 传输绑定

### 架构决策
- **事件桥接模式**：a2ui→agui 存在单向依赖（a2ui 导入 agui 的 CustomEvent 类型），agui 反向导入会形成循环依赖。采用 agentcore.A2UIEvent 作为中间事件类型，agui 转换器仅需识别事件名 "a2ui"，不依赖 a2ui 包
- **envelopeToMap 序列化**：将结构体转为泛型 map 后通过事件总线传输，保持 agentcore 对 a2ui 类型的零感知

### 验证
- `go build ./...` ✅
- `go test ./a2ui/...` ✅（100% 覆盖率）
- `go test ./agui/...` ✅（含新 A2UI e2e 测试）
- `go test ./agentcore/...` ✅（含新 A2UIEvent 测试）
- `go run ./example/a2ui-demo/` ✅

---

# AI 决策变更日志

> 本文件记录 AI 协助开发过程中的关键决策。新决策追加在本文件顶部。
> 历史决策（2025-06 ~ 2026-07-17，共 108 条）
> 已归档至 `docs/decisions/archive/`，见下方[归档索引](#归档索引)。

---

## 归档索引

| 归档文件 | 时段 | 章节数 | 主要主题 |
|----------|------|--------|----------|
| [`2025-06-initial-review.md`](archive/2025-06-initial-review.md) | 2025-06-11 | 1 | 2025-06 初始代码质量审查 |
| [`2026-07-w11-handoff-docs.md`](archive/2026-07-w11-handoff-docs.md) | 2026-07-11 ~ 2026-07-12 | 12 | 2026-07-11~12 文档同步与 Invisible Handoff |
| [`2026-07-w13-eval-doomloop.md`](archive/2026-07-w13-eval-doomloop.md) | 2026-07-13 ~ 2026-07-14 | 45 | 2026-07-13~14 评估框架增强与 DoomLoop |
| [`2026-07-w15-citation-gate.md`](archive/2026-07-w15-citation-gate.md) | 2026-07-15 ~ 2026-07-17 | 50 | 2026-07-15~17 引用核验 Gate 实现 |

---

## 2026-07-19: 里程碑 4——原路线图剩余项收尾

### 背景
用户核对原路线图后发现 Sprint 1-3 的 10 项任务中有 5 项未执行。本里程碑完成剩余项。

### 变更

#### 1. AI_CHANGELOG.md 归档化（原 Sprint 3 #9）
- 主文件从 3154 行 / 265KB 降到 1383 行 / 77KB（**-57%**）
- 108 个历史章节（2025-06 ~ 2026-07-17）归档到 `docs/decisions/archive/` 下 4 个文件
- 主文件保留 07-18~07-19 近期决策 + 归档索引表

#### 2. Deprecated API 清理（原 Sprint 3 #10）
清理了 2 处**真正无调用方的死代码**：
- `tui/theme/style.go`：删除 14 个 deprecated 全局 Style 变量及 palette.go 同步代码
- `domains/reasoning/fact_blackboard.go`：删除 deprecated `SetPlan` 方法

**保留** 5 处仍被生产代码使用的 deprecated 字段（需先完成钩子迁移才能删除）。

#### 3. log → slog 迁移（原 Sprint 2 #5）
- 制定迁移方案文档 `docs/decisions/log-to-slog-migration-plan.md`
- 执行阶段 1+2：agentcore/tui-theme/memory/domains/knowledge/server/mcp/tools（14 文件，53 处）
- **非入口包 log.Printf 从 53 处降到 0 处**
- cmd/mady（27 处）按方案保留（入口包 Go 惯用法）

#### 4. 拆分 agent_run.go runLoop（原 Sprint 2 #6）
- `runLoop` 从 280 行 → 36 行（**-87%**）
- 提取 `runInnerLoop` 方法（254 行），行为完全等价
- 9 个 TestAgentRun* 测试 + race detector 全过

#### 5. browser_providers 覆盖率提升（原 Sprint 3 #8）
- 新增 27 个测试，**覆盖率从 4.4% 提升到 50.3%**
- 网络依赖测试用 `testing.Short()` 守护

### 验证
- `make all` ✅（根 + tools 子模块）
- `go test -race ./agentcore/...` ✅
- `pre-commit run --all-files` ✅

### 原路线图执行状态
所有 10 项任务现已全部处理完毕。

---

## 2026-07-19: 里程碑 3——超大函数拆分 + fmt.Errorf 债务复核

### 背景
原路线图里程碑 3 包含三项：
1. 拆分 `tools/browser_legacy.go:NewBrowserNavigateTool`（288 行）
2. 拆分 `agentcore/compaction.go:runCompaction`（235 行）
3. `fmt.Errorf` 分批加 `%w`（审查报告称 450 处未 wrap）

经深入分析，**第 2、3 项均不成立**，详见下文勘误。仅执行第 1 项。

### 变更：拆分 NewBrowserNavigateTool

#### 问题
`NewBrowserNavigateTool` 是返回 `*agentcore.Tool` 的工厂函数，核心是一个 288 行
的闭包（`Func` 字段）。闭包内 `switch session.backendType` 有两个巨大 case：
- `BackendLightpanda`（约 97 行）
- `BackendLocal, BackendCDP, BackendBrowserbase, ...`（约 84 行）

两个 case 的导航逻辑**80%+ 完全相同**：注入 stealth JS → 导航 →
等待 DOM interactive → 重新应用 stealth JS → 获取 title →
生成 snapshot → 更新 session 状态。差异仅：
- 错误前缀（"lightpanda navigation failed" vs "navigation failed"）
- Lightpanda 末尾有 12 行的 fallback 处理

#### 重构
提取共享 helper `chromedpNavigateAndSnapshot(session, parsedURL, navTimeout, navErrPrefix)`，
封装完整的 chromedp 导航流程（92 行）。两个 case 改为单行调用：
- `BackendLightpanda`：调用 helper + fallback 处理
- `BackendLocal/CDP/...`：仅调用 helper

#### 行为等价性
- helper 使用 `defer cancel()`，在函数返回时释放 timeoutCtx
- fallback 使用外层 ctx（非 timeoutCtx），不受影响
- 所有 chromedp 操作在 helper 返回前完成，cancel 时机等价于原代码

#### 效果
- `NewBrowserNavigateTool`：288 行 → 131 行（**-55%**）
- 新增 `chromedpNavigateAndSnapshot`：92 行（单一来源）
- 75 行重复导航逻辑从 2 处合并为 1 处
- 修改导航流程（如调整 stealth JS 应用时机）只需改 1 处

### 不执行的任务与勘误

#### ❌ 不拆分 runCompaction
`agentcore/compaction.go:runCompaction`（235 行）经评估**不适合拆分**：
- 该函数是**线性业务流程**（非重复代码），步骤间有大量局部变量共享
  （msgs, compressStart, compressEnd, summaryIdx, turnsToSummarize 等）
- 强行拆分需要在多个 helper 间传递 10+ 个参数，**降低可读性**
- 该函数是核心上下文压缩逻辑，**风险极高**（任何 bug 影响所有 agent 记忆系统）
- 拆分弊大于利，保持现状

#### ❌ fmt.Errorf 加 %w 不成立
审查报告称"450 处 fmt.Errorf 未用 %w（39%）"是**严重误判**：

经 Python 脚本精确分析（基于 AST 级别的参数类型推断）：
- 449 处未 wrap 的 `fmt.Errorf` 中，**真正应该 wrap 但没 wrap 的数量为 0**
- 63 处是**字面新建错误**（无参数，如 `fmt.Errorf("invalid token")`）
- 384 处是**带业务变量参数的新建错误**
  （如 `fmt.Errorf("session %q not found", sessionID)`，
  参数是 sessionID/name/statusCode 等，不是 err）
- 2 处边界情况（panic recover 的 `%v`、errStr 字符串变量）是合理设计

**关键认知**：`%w` 只用于**包裹另一个 error** 以保持错误链。
用 `%v`/`%q`/`%d` 打印业务变量是**完全正确**的 Go 惯用法。
审查报告的统计方式（"不含 %w 的 Errorf 数量"）技术上准确但语义误导。

Mady 的错误处理实际**非常健康**：
- 694 处 `fmt.Errorf` 正确使用了 `%w`
- 18 处 `errors.Is`、7 处 `errors.As`（数量适中，因为项目主要用 sentinel error）

### 验证
- `make all` ✅（根 + tools 子模块）
- `cd tools && go test ./...` ✅
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- gopls：0 errors

### 涉及文件
- `tools/browser_legacy.go`（重构）
- `docs/decisions/AI_CHANGELOG.md`（本记录）

### 影响范围
- ✅ 浏览器导航工具：代码重复消除，可维护性显著提升
- ✅ 错误处理认知勘误：避免未来基于误判的批量重构
- ❌ 不影响任何运行时行为（重构是行为保持的提取）

### 审查报告勘误（累计）
本次审查（里程碑 1-3）共发现原审查报告 **6 处误判**：
1. ~~"根模块与 tools 子模块循环依赖"~~ → 合理装配耦合（里程碑 2 已记录）
2. ~~"`agui/types.go:36` 312 行大函数"~~ → awk 统计脚本误判（里程碑 2 已记录）
3. ~~"`acp/protocol.go:30` 282 行大函数"~~ → 同上误判
4. ~~"retrieval/embedding.go HTTP client 无 Timeout"~~ → 实际已设置 30s（里程碑 1 已记录）
5. ~~"runCompaction 235 行需拆分"~~ → 线性业务流程，拆分弊大于利（本次记录）
6. ~~"450 处 fmt.Errorf 未 wrap 是债务"~~ → 真正应 wrap 的为 0 处（本次记录）

**结论**：Mady 的代码质量基线**远高于审查报告所暗示的水平**。
审查中的静态扫描数字（行数、百分比）有多处误判，但深度分析后确认实际债务极少。

---


## 2026-07-19: 里程碑 2——代码现代化与重复消除

### 背景
技术债务审查报告中原列"Sprint 2 第 4 项：解开 tools/browser_session.go 与
browser_providers 的循环依赖"。经深入分析，**该项基于审查误判**：
- `tools/browser_session.go` 与 `tools/browser_providers` 同属 tools 子模块内部，
  是正常的子包依赖（非跨模块循环）
- 根模块 ↔ tools 子模块的双向引用是合理的装配耦合
  （cmd/mady 和 domains 装配 tools 扩展，tools 实现 agentcore.Extension 接口）
- agentcore 不 import domains（仅测试文件引用），无架构层循环

因此里程碑 2 转向**低风险高价值**的两项债务清理。

### 变更

#### 1. `interface{}` → `any`（Go 1.25 现代化）
- 全仓库 18 处 `interface{}` 机械替换为 `any`
- 使用 `gofmt -r 'interface{} -> any'` 官方 rewrite 规则
- 包含 2 处注释中的 `interface{}` 引用
- **零运行时行为变更**（`any` 是 `interface{}` 的类型别名，编译后完全等价）

涉及文件（9 个）：
- `agentcore/evaluate/cli/cli.go`、`agentcore/evaluate/loader.go`
- `agentcore/evaluate/reflection.go`、`agentcore/evaluate/reflection_test.go`
- `agentcore/evaluate/tool_accuracy.go`
- `domains/reasoning/multi_hypothesis.go`
- `retrieval/model_rerank_test.go`
- `skill/frontmatter.go`、`tui/terminal/keybindings.go`

#### 2. `agui/converter.go` Convert 函数重复代码消除
- **问题**：Convert 函数 237 行，11 对 value/pointer case 完全对称重复，
  其中 5 对（HandoffStart/End、CompactionStart/End、AutoRetry）包含
  ~15 行的 CustomEvent 字面量构造，修改字段时需同步改两处
- **重构**：提取 5 个 helper 方法（`convertHandoffStart`/`convertHandoffEnd`/
  `convertCompactionStart`/`convertCompactionEnd`/`convertAutoRetry`），
  每对 pointer case 解引用后转发给 value helper
- **遵循已有模式**：与既有的 `convertMessageDelta`/`convertToolCallStart`/
  `convertToolCallEnd` 保持一致的单一职责（helper 只构造事件，
  `closeAll` 由 Convert 主函数调用）
- **效果**：Convert 函数从 237 行降到 156 行（-34%），
  CustomEvent 字段定义从 10 处降到 5 处

### 设计权衡
- **为何不提取所有 11 对 case**：剩余 6 对（AgentStart/End/Error、TurnStart/End、
  MessageDelta、ToolCallStart/End）要么已经是单行 return，要么已有 helper。
  强行提取会引入 6 个新方法但每个只有 1-2 行，收益为负。
- **为何 pointer case 不直接 type-switch 到统一接口**：各事件类型字段不同，
  无法用单一接口抽象；解引用转发是最小化、行为完全不变的方案。

### 验证
- `make all` ✅（根 + tools 子模块）
- `go test -count=1 -v ./agui/...` ✅（13 个测试全过）
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- gopls 诊断：0 errors（与重构前一致）

### 涉及文件
- `agentcore/evaluate/cli/cli.go`、`agentcore/evaluate/loader.go`
- `agentcore/evaluate/reflection.go`、`agentcore/evaluate/reflection_test.go`
- `agentcore/evaluate/tool_accuracy.go`
- `domains/reasoning/multi_hypothesis.go`
- `retrieval/model_rerank_test.go`
- `skill/frontmatter.go`
- `tui/terminal/keybindings.go`
- `agui/converter.go`

### 影响范围
- ✅ 代码现代化：Go 1.25 `any` 别名全面采用
- ✅ 可维护性：CustomEvent 字段修改从改 2 处变为改 1 处
- ✅ 可读性：Convert 函数职责更清晰（分发器 + helper 构造器）
- ❌ 不影响任何运行时行为（`any` 是别名，重构是行为保持的提取）

### 审查报告勘误
原审查报告中以下结论需更正：
1. ~~"根模块与 tools 子模块循环依赖"~~ → 实为合理的装配耦合（非病态循环）
2. ~~"`agui/types.go:36` 312 行大函数"~~ → 误判，types.go 是连续 struct 定义，
   awk 统计脚本将多个 struct 算成了一个"函数"
3. ~~"`acp/protocol.go:30` 282 行大函数"~~ → 同上误判
4. 真实的超大函数排行见本次审查的 Python 脚本输出（基于大括号深度匹配）

---


## 2026-07-19: 里程碑 1——本地开发反馈回路完整性修复

### 背景
全量技术债务审查发现：本地开发工具链与 CI matrix 存在**反馈回路不一致**。

CI（`.github/workflows/ci.yml`）通过 matrix 同时跑根模块和 `tools/` 子模块的
vet/build/test，但本地 `make` 与 `pre-commit` 都**漏掉 `tools/` 子模块**：
- `make all` / `make test` / `make build` / `make vet` 只跑 `go X ./...`（根模块）
- `.pre-commit-config.yaml` 的 `go-vet` hook 同样只覆盖根模块
- pre-commit 未集成 `golangci-lint`，开发者本地提交无法触发 errcheck/gosec/staticcheck 等 10 个 linter

开发者本地 `make all` 通过 → 推送 → CI 才发现 tools 子模块挂掉，反馈延迟 5-15 分钟，
且大量 lint 问题（未 wrap 错误、gosec 违规等）只能在 PR 阶段暴露。

### 变更
1. **Makefile**（`Makefile`）：
   - `all`/`build`/`test`/`test-race`/`test-short`/`test-verbose`/`vet` 七个 target
     全部追加 `cd tools && $(GO) X ./...` 步骤，与 CI matrix 行为对齐
   - `coverage` 保持仅根模块（与 CI codecov 上传路径一致）
   - `help` 输出顶部添加覆盖范围的显式说明
2. **pre-commit 配置**（`.pre-commit-config.yaml`）：
   - `go-vet` hook 改为 `go vet ./... && (cd tools && go vet ./...)`
   - 新增 `golangci-lint` hook（pre-commit 阶段），调用独立包装脚本
3. **新增包装脚本**（`scripts/precommit-golangci-lint.sh`）：
   - 对根模块 + tools 子模块分别运行 golangci-lint
   - 跨机器兼容：优先 `$(go env GOPATH)/bin/golangci-lint`，缺失回退 PATH
   - 未安装时非阻塞降级（提示 `make install-lint`），避免阻塞新贡献者

### 设计权衡
- **非阻塞降级 vs 强制安装**：选择前者。golangci-lint 不在 Go 工具链中，
  新贡献者首次 clone 不应被阻塞；CI 仍会强制执行。
- **抽独立脚本 vs 内联 YAML**：原计划内联 `bash -ec '...'`，
  但 YAML 对 `: `（冒号空格）敏感导致解析错误，脚本方式更可读可维护，
  与既有 `scripts/check-sensitive-paths.sh` 模式一致。

### 验证
- `make vet` ✅（根 + tools 均跑）
- `make build` ✅
- `make test` ✅（tools 子模块测试首次在本地可见）
- `make all` ✅
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- `pre-commit run --all-files golangci-lint` ✅

### 涉及文件
- `Makefile`
- `.pre-commit-config.yaml`
- `scripts/precommit-golangci-lint.sh`（新增）

### 影响范围
- ✅ 本地开发：`make all` 与 CI 行为完全对齐，反馈从 PR 阶段提前到提交前
- ✅ 提交前 lint：errcheck/gosec/staticcheck 等 10 个 linter 在本地可触发
- ✅ 多模块一致性：根模块与 tools 子模块在所有本地工具链中等价对待
- ❌ 不影响任何运行时行为；不修改任何业务代码

### 后续里程碑
- **里程碑 2**（结构性）：解开 tools/browser_session.go 与 browser_providers 的循环依赖
- **里程碑 3**（持续治理）：log → slog 迁移、`fmt.Errorf` 加 `%w`、超大函数拆分

---


## 2026-07-18: 代码质量现代化——slices.Contains/min 替换 + 全面审阅报告

### 背景
对全仓库进行系统性代码质量审阅，包括 lint/vet/race/coverage/gopls
四个维度的静态分析扫描。

### 发现
- **Lint (golangci-lint 含 12 个 linter)**：0 issues ✅
- **go vet**：0 issues ✅
- **gopls**：0 errors, 0 warnings, 仅存 6 个现代化 hints
- **go build ./...**：clean ✅
- **go mod verify**：clean ✅
- **race**：go test -race ./... 全通过 ✅
- **覆盖率**：整体 ~59%，10 个包低于 50%
- **TODOs/FIXMEs/HACKs**：生产代码 0 处
- **time.After 泄漏风险**：0 处
- **panic 使用**：9 处，全部为确定性的编程错误/配置错误 sentinel panic

### 变更
- `a2a/ws.go`：手动循环 `for _, allowed := range s.allowedOrigins` →
  `slices.Contains(s.allowedOrigins, origin)`
- `mcp/client_reconnect.go`：`if x > max { x = max }` → `x = min(x, max)`

### 验证
- `go build ./...` ✅
- `go test -race -count=1 ./a2a/... ./mcp/...` ✅
- `pre-commit run --all-files` ✅

### 涉及文件
- `a2a/ws.go`、`mcp/client_reconnect.go`

### 质量评估总表

| 维度 | 评分 | 说明 |
|------|------|------|
| Lint | 🟢 0/0 | 12 个 linter + 全部 govet 检查，零问题 |
| 类型安全 | 🟢 0/0 | gopls 零错误零警告 |
| 竞态安全 | 🟢 0/0 | full `-race` 全绿 |
| 构建完整性 | 🟢 clean | go build + mod verify |
| 技术债务 | 🟢 极低 | 0 TODO/FIXME/HACK/BUG |
| 覆盖率 | 🟡 59% | 70% 目标，gap 在 TUI/协议层测试 |
| 现代化程度 | 🟢 Go 1.25 | 已应用 min/max/slices/Clear/range-int |
| 架构依赖 | 🟢 严格单向 | 8 层分层已校验 |
| 安全防护 | 🟢 all C/H fixed | Phase 7 全部 8C+16H 已修复 |

**结论：代码质量已达成熟生产级水平，下一阶段重心应转向 P3 专家盲测。**

---


## 2026-07-18: Phase 7 剩余 High 修复（H2 H3 H6）+ 安全性审核（H4 H5 H7）

### 背景
Phase 7 报告列出 18 个 High 问题，此前已修复 11 个（H1/H8-H14）。
剩余 7 个 High（H2-H7）集中于 a2a 和 mcp 包的并发安全问题。

### 变更
- **H2 (a2a/recordTask O(n²) 扫描)**：将单条目逐次淘汰的 while 循环改为
  batch collect → sort.Slice → 批量淘汰，复杂度从 O(k×n) 降为 O(n log n)。
- **H3 (a2a/handleResubscribe 浅拷贝)**：新增 `deepCopyEvent()` 对
  TaskUpdateEvent 的 Result/Artifact/Error 指针字段进行防御性深拷贝，
  在附带锁的 replay 构建过程中保证事件数据在锁释放后不会被意外共享。
- **H4 (a2a/锁顺序)**：经审阅确认安全——`recordTask` 释放 `ts.mu` 之后才获取
  `taskStatesMu`，不存在 ABBA 死锁路径。已在函数文档注释中注明锁顺序约定。
- **H5 (mcp/Close+readLoop 关闭竞争)**：审阅确认 `c.mu` 互斥锁防护了 pending
  通道的清理，readLoop 的通道关闭与 Close() 不会并发执行。Safe.
- **H6 (mcp/tryReconnect 协议版本校验)**：`initialize` 方法发送
  `protocolVersion` 给服务端但未检查服务端返回的版本。新增版本前缀匹配：
  年份不同时记录 `log.Printf` 警告，不阻塞连接（向后兼容旧服务器）。
- **H7 (mcp/callWithRetry TOCTOU)**：`writeMessage` 在 `c.mu` 解锁后调用
  `c.stdin.Write()`。若 `Close()` 在间隙被调用，Write 返回 `errClientClosed`
  并由外层 `callWithRetry` 的重试逻辑处理。误报风险低，文档已注明设计意图。

### 验证
- `go build ./a2a/... ./mcp/... && go build ./...` ✅
- `go test -race -count=1 ./a2a/...` ✅ (1.8s)
- `go test -race -count=1 ./mcp/...` ✅ (21.5s)
- `pre-commit run --all-files` ✅

### 涉及文件
- `a2a/taskstate.go`（recordTask O(n²)→O(n log n)、锁顺序注释）
- `a2a/server_jsonrpc.go`（handleResubscribe 深拷贝 + 防御性 replay）
- `a2a/types.go`（新增 deepCopyEvent 函数）
- `mcp/client.go`（initialize 协议版本核验）

### 备注
- H2-H7 经审阅修复后，Phase 7 全部 **18 个 High 问题中 16 个已修复/安全关闭**，
  H4/H5/H7 经审阅确认为安全或无实际风险。
- 2 个待定项（H2 的极端大量 task 场景 batch 排序开销、H3 的 `Message`/`Part`
  深层数组深拷贝）在当前使用场景下无实际影响，暂不处理。

---


## 2026-07-18: Phase 7 遗留 High 修复（H1 H11）+ 报告同步

### 背景
Phase 7 全量审阅报告（2026-07-14）标识 8 Critical + 18 High + 34 Medium 问题。
此前 `bda2694` / `6171e2e` 已修复全部 8 Critical 和 6 个 High，但报告状态列未同步；
且 H1（WebSocket token URL 日志泄漏）与 H11（ACP CWD 路径沙箱验证）仍未修复。

### 变更
- **H1 (a2a)**：新增 `RedactURL()` 函数（`a2a/middleware.go`），脱敏 token/apiKey 等敏感
  查询参数后输出 URL；`handleWebSocket` 处添加注释说明 query param 的已知风险交换。
  新增 `a2a/middleware_test.go`（5 个用例覆盖脱敏/非脱敏/空参数）。
- **H11 (acp)**：新增 `sanitizeCWD()` 函数，在 `handleNewSession` / `handleForkSession`
  中对 CWD 路径执行 `filepath.Clean` + `filepath.Abs`，防止 `../` 目录遍历。
- `docs/review/phase7-summary-and-roadmap.md`：全量同步——8 Critical 标注 ✅ + 修复 commit 引用；
  High 表新增状态列（11/18 已修复）；CI 门禁表 3 项🔴→✅；安全防护评级 ⭐⭐⭐→⭐⭐⭐⭐；
  总体评级从 ⭐⭐⭐⭐(4/5) 维持。

### 验证
- `go build ./acp/... ./a2a/...` ✅
- `go test -count=1 ./acp/... ./a2a/...` ✅（acp 0.011s / a2a 0.839s）
- `pre-commit run --all-files` ✅（含 sensitive-paths gate 和 commitlint）

### 涉及文件
- `acp/server.go`（sanitizeCWD 新增 + handleNewSession/handleForkSession 改用 sanitizeCWD）
- `a2a/middleware.go`（RedactURL + SensitiveQueryParams 新增）
- `a2a/middleware_test.go`（新建）
- `a2a/ws.go`（handleWebSocket 添加日志脱敏注释说明）
- `docs/review/phase7-summary-and-roadmap.md`（全表同步）

### 备注
- 剩余 7 个 High（H2-H7）未处理，主要是 a2a 锁顺序/竞争和 mcp 关闭竞争，需独立审查。
  从 roadmap 看 P3 盲测完成前不会启动这类协议层修复，符合"停止规则"。

---


## 2026-07-18: M1 门禁加固（4/4）：pre-commit 跨机器兼容 + commit-msg 敏感路径门禁

### 背景
`.pre-commit-config.yaml` 的 `go-imports` hook 硬编码 `/Users/xujian/go/bin/goimports`，
换机器/CI 即失效；同时 sensitive-paths 检查此前仅在 `--ci-base` 模式（GitHub Actions）
和无参数模式（HEAD commit）下运行，**本地 `git commit` 阶段没有任何门禁**，
导致开发者可以在本地静默提交"AI 参与 + 敏感路径"的违规组合，CI 才会拦截——反馈链路过长。

### 变更
- `.pre-commit-config.yaml`：
  - `go-imports` entry 改为动态查找：优先用 `go env GOPATH/bin/goimports`，
    缺失时回退 `PATH` 中的 `goimports`，跨机器/CI 通用
  - 新增 `check-sensitive-paths` hook（`stages: [commit-msg]`），
    调用 `scripts/check-sensitive-paths.sh --msg-file` 读取本次提交消息文件
- `scripts/check-sensitive-paths.sh`：新增 `--msg-file <path>` 参数，
  供 commit-msg 钩子读取 pre-commit 传入的提交消息文件；
  与 `--ci-base` 互斥；空参数/文件不存在/未知参数均显式报错退出

### 验证
- `bash -n scripts/check-sensitive-paths.sh` ✅
- 边界用例 4 项全部按预期：互斥分支 exit=1、未知参数 exit=1、
  缺值 exit=1、msg 文件不存在 exit=1
- `pre-commit run --hook-stage commit-msg --commit-msg-filename <tmp>` ✅
  （`sensitive paths gate ... Passed`、`commitlint ... Passed`）
- 关键阻塞场景：构造"AI Co-authored-by + 暂存 guardrails/levels.go"，
  commit-msg 钩子返回 exit=1 并打印完整阻塞提示文案 ✅
- `pre-commit run --all-files` ✅（trailing-whitespace / end-of-file 顺手清理
  docs/TOOL_CONTRACT.md、docs/design/citation-verification-gate.md、
  docs/evaluation-baseline-invalidation-p2b.json 三处历史格式遗留）

### 涉及文件
- `.pre-commit-config.yaml`、`scripts/check-sensitive-paths.sh`
- （格式顺手清理）`docs/TOOL_CONTRACT.md`、`docs/design/citation-verification-gate.md`、
  `docs/evaluation-baseline-invalidation-p2b.json`

### 备注
- 本地需执行 `pre-commit install --hook-type commit-msg` 注册 commit-msg 钩子
  （`.git/hooks/commit-msg` 不在版本控制内，新克隆需手动安装）
- CI 端 `.github/workflows/ai-code-quality.yml` 仍使用 `--ci-base` 模式，
  与本次新增 `--msg-file` 模式互不影响，无需改动

---


## 2026-07-18: M1 门禁加固（3/4）：清理无效 -short flag 与 forbidigo 死配置

### 背景
- CI race 步骤 `go test -race -count=1 -short ./...` 中的 `-short` 是纯无效 flag：
  全库 0 处 `testing.Short()` 调用，没有任何测试会因此在 CI 被跳过
- `.golangci.yml` 存在针对 `cmd/mady/main.go` 的 forbidigo 排除规则，
  但 forbidigo 未出现在 `linters.enable` 列表中，规则永远不会生效，属死配置

### 变更
- `.github/workflows/ci.yml`：race 步骤命令去掉 `-short`，
  改为 `go test -race -count=1 ./...`（Makefile 的 `test-short` target 保留不动）
- `.golangci.yml`：删除 forbidigo 排除规则（原第 93-95 行）

### 验证
- `golangci-lint run ./...`（根模块）✅ 0 issues
- `cd tools && golangci-lint run ./...`（tools 子模块）✅ 0 issues
- `go build ./... && go vet ./...` ✅

### 涉及文件
- `.github/workflows/ci.yml`、`.golangci.yml`

---


## 2026-07-18: M1 门禁加固（2/4）：codecov 配置合并并启用覆盖率门槛

### 背景
仓库根目录同时存在 `codecov.yml`（project target 30%/threshold 5%）与 `.codecov.yml`
（50%/2%）两份冲突配置，Codecov 实际生效项不确定；且 CI 覆盖率上传步骤
`fail_ci_if_error: false`，上传失败静默通过，覆盖率门禁形同虚设。
当前覆盖率基线约 61.4%，采取渐进收紧策略。

### 变更
- 删除 `.codecov.yml`，配置统一合并进 `codecov.yml`：
  - project target 55%、threshold 2%（低于当前基线，留出缓冲渐进收紧）
  - patch target 50%
  - ignore 取两文件并集：`example/**`、`benchmark/**`、`integration/**`、
    `**/*_test.go`、`**/doc.go`、`vendor/**`
  - 保留原有 precision(2)/round(down)/comment 配置，range 调整为 `55...80`
- `.github/workflows/ci.yml`：覆盖率上传步骤 `fail_ci_if_error` 由 `false` 改为 `true`，
  上传失败将直接使 CI 失败

### 验证
- `go build ./... && go vet ./...` ✅
- pre-commit `check-yaml` 通过（提交时自动校验 codecov.yml / ci.yml 语法）

### 涉及文件
- `codecov.yml`、`.codecov.yml`（删除）、`.github/workflows/ci.yml`

---


## 2026-07-18: M1 门禁加固（1/4）：integration e2e 测试接入 CI

### 背景
`integration/` 下的 e2e 测试（build tag `integration`，本地 21 个用例全绿）此前只能手动跑，
CI 无任何门禁，回归无法被自动拦截。

### 变更
- `Makefile`：新增 `test-integration` target（`go test -tags integration -count=1 ./integration/...`），
  同步加入 `.PHONY` 与 help 文本（Test 段落）
- `.github/workflows/ci.yml`：新增 `integration` job（位于 build-and-test 之后），
  ubuntu-latest + checkout@v7 + setup-go@v6（`env.GO_VERSION`），
  执行 `go test -tags integration -count=1 -timeout 10m ./integration/...`

### 验证
- `go test -tags integration -count=1 ./integration/...` ✅（26 个 PASS，含子测试）
- `go build ./... && go vet ./...` ✅

### 涉及文件
- `Makefile`、`.github/workflows/ci.yml`

---


## 2026-07-18: TUI 质量审查修复落地（P1 主题检测 + P2 测试覆盖 + 支撑构造函数）

### 背景
对 Mady `tui/` 模块进行全面质量审查后，识别出 1 个 P1 功能缺陷、1 个 P2 结构性短板，
以及变更日志与代码状态不一致的问题。本次集中完成这些修复的落地工作。

### 修复内容

1. **P1 修复深色终端主题检测**
   - `tui/theme/global.go`：`DefaultSemanticForTerminal()` 在检测到非浅色终端时返回 `DefaultMadyDark()`
   - 新增 `tui/theme/global_test.go`：覆盖 COLORFGBG 缺失、异常、深色（0–7）与浅色（8+）分支，
     并验证 `SetSemanticTheme`/`CurrentPalette` 全局状态同步
   - 测试通过保存/恢复 `atomicPalette` 避免全局状态交叉污染

2. **P2 提升 TUI 测试覆盖率**
   - 重写 `tui/agentadapter/adapter_test.go`：删除空回调，改为通过真实事件总线 emit agentcore 事件，
     等待异步分发 drain 后断言 chat 事件字段，覆盖 agent start、turn end、message delta、agent error
   - 新增 `tui/terminal/keybindings_test.go`：覆盖 Register、Matches、SetUserBindings、
     冲突检测、All、LoadUserBindingsJSON（含非法 token 与空输入）
   - 新增 `tui/component/settings_test.go`：覆盖空列表、Get/SetValue、导航、左右循环、
     OnChange、OnSubmit、渲染
   - 新增 `tui/component/loader_test.go`：覆盖 Start/Stop 幂等、IsRunning、SetMessage、
     SetStyle、渲染、CancellableLoader abort

3. **支撑性改动：agentcore 事件构造函数**
   - `agentcore/event.go`：为外部测试和调用方新增 `NewAgentStartEvent`、`NewAgentEndEvent`、
     `NewAgentErrorEvent`、`NewAgentInterruptEvent`、`NewSkillLoadedEvent`、`NewTurnStartEvent`、
     `NewTurnEndEvent`、`NewMessageDeltaEvent`、`NewToolCallStartEvent`、`NewToolCallEndEvent`、
     `NewHandoffStartEvent`、`NewHandoffEndEvent`、`NewCompactionStartEvent`、
     `NewCompactionEndEvent`、`NewAutoRetryEvent`（共 15 个）
   - 所有构造函数自动设置正确的 `baseEvent.Kind`，解决外部包无法构造可路由事件的问题

4. **文档与注释同步**
   - `tui/component/domain.go`：修正注释，明确 `agentcore.Message.Metadata["domain"]` 的 JSON
     解码链路尚未实现，避免维护者误以为该链路已完工

5. **误报澄清**
   - `agentcore/evaluate/metrics_test.go:TestSetCitationVerifierConcurrent` 经 10 次 `-race` 实测
     稳定通过，无 `send on closed channel` 风险；其 `sync.WaitGroup` 用法正确，无需改动

### 验证
- `go build ./...` ✅（根模块）
- `go test -race ./tui/theme/ ./tui/component/ ./tui/terminal/ ./tui/agentadapter/ ./agentcore/ ./agentcore/evaluate/` ✅
- `golangci-lint run ./tui/... ./agentcore/...` ✅ 0 issues
- 覆盖率数字待全量跑批后重新测量

### 涉及文件
- `tui/theme/global.go`、`tui/theme/global_test.go`
- `tui/agentadapter/adapter_test.go`
- `tui/terminal/keybindings_test.go`
- `tui/component/settings_test.go`、`tui/component/loader_test.go`
- `agentcore/event.go`
- `tui/component/domain.go`
---


## 2026-07-18: 规范合规全面修复（P0/P1/P2/P3 共 9 项）

### 背景
对照 AGENTS.md / CLAUDE.md / SECURITY.md / CONTRIBUTING.md / tone-style-guide /
chat-assistant-architecture 全套规范审阅项目，发现 4 项 P0、2 项 P1、3 项 P2/P3 真实违规，
无任何"误报"。本次集中修复全部 9 项。

### P0：真实违规（4 项）

1. **敏感路径清单 6 份不一致**：`scripts/check-sensitive-paths.sh` 仅 10 条，
   AGENTS.md/CLAUDE.md 各 18 条，SECURITY.md 12 条，GO-DEVELOPMENT-STANDARDS.md 9 条，
   CI 实际只检测 10 条路径——8 个文档承诺的敏感路径门禁缺口
   - 修复：脚本扩展为 18 条权威源（补 `tools/vision.go` + 7 个上层边界文件），
     4 份管理文档同步为 20 条（脚本 18 + 目录级 `agentcore/permission/`、`guardrails/guardian/`）
   - 在每份文档显式声明"脚本为权威源"，避免未来漂移
2. **`tui/chat` 违规导入 `agentcore`**：LAYERS.md 第 21 行明确禁令，
   `chat_history.go:16` 实际 import agentcore（使用 DomainMessage 类型）
   - 修复：将 `agentcore/message_domain.go` 整体下沉到 `tui/component/domain.go`
     （该文件本就是孤儿——所有引用方都在 TUI 内部，agentcore/evidence 包有独立的同名类型）
3. **`tui/component` 3 个 card 组件违规导入 `agentcore`**：LAYERS.md Layer 4 仅允许依赖 Layer 0–2
   - 修复：随 P0-2 一次性消除——DomainMessage 迁移到本包后，evidence_card/approval_card/conclusion_card
     均去掉 agentcore import，改用同包类型
4. **删除原 `agentcore/message_domain.go`**：无任何外部消费者（agentcore/evidence 包定义独立的
   EvidenceDirection/EvidenceSpan，与 DomainMessage 无关）

### P1：规范执行偏差（2 项）

1. **`server.New` 导出符号无注释**（GO-DEVELOPMENT-STANDARDS.md §10.1 要求每个导出符号必须注释）
   - 修复：补中文注释（SSEKeepAlive 已有注释，原审查误报）
2. **GO-DEVELOPMENT-STANDARDS.md §10.1 与 §10.4 内部矛盾**：§10.1 示例用英文，§10.4 要求中文
   - 修复：§10.1 显式约定"注释使用中文，但首句以英文符号名开头以兼容 godoc/golint"，
     示例改为中文版本
3. **CI 未强制 Conventional Commits**：仅本地 pre-commit 钩子，贡献者未装钩子可绕过
   - 修复：新增 `commitlint.config.js`（放宽 header-max-length 到 120 以兼容中文 subject，
     关闭 subject-case），在 `.github/workflows/ci.yml` 增加 commitlint job（wagoid/commitlint-github-action@v6）

### P2：架构与路径规范（2 项）

1. **cmd/mady 3 处 `./` 相对路径兜底**：framework.go:224、server.go:58、tui.go:81
   违反 AGENTS.md "禁止新增 ./ 相对路径默认值"
   - 修复：兜底分支改用 `util.ResolveDataDir()`，与 `MadyHome()` 三级解析保持一致
   - 注：这些分支理论上不可达（MadyHome 最终回退已调 filepath.Abs），
     修复目的在于统一治理、防止未来扩张
2. **`agentcore/evaluate` 反向依赖 `guardrails`**：metrics.go:260 调用 `guardrails.VerifyCitations`，
   违反"上层不得反向依赖下层"约束
   - 修复：依赖注入模式——在 evaluate 包定义本地 `CitationValidityReport` +
     `CitationVerifier` 函数变量 + `SetCitationVerifier` 注入 API，evaluate 包零 guardrails 引用
   - 注入点：`cmd/mady/eval.go`（顶层装配层）通过 `citationVerifierAdapter` 注入；
     `scripts/replay_citation_metrics/main.go` 同步注入；
     `metrics_test.go` 通过 t.Cleanup 在测试中注入并恢复
   - 测试场景例外：`evaluate/benchmark/live_*_test.go` 的 `provider/chatcompat`、
     `domains/reasoning` 反向引用属合理的测试集成场景（需真实 LLM 才能跑），保留现状

### P3：文档一致性（1 项）

1. **`tui/layout` 在 LAYERS.md 缺定义**：CLAUDE.md:102 标注"Layer 8"错误，
   实际 `tui/layout` 仅依赖 `tui/core`（Layer 0）
   - 修复：LAYERS.md 表格补 `tui/layout` 行（Layer 0 扩展），Directory Structure 补 layout/ 子树，
     CLAUDE.md 描述改为"Layer 0 扩展：布局原语（仅依赖 core）"

### 合规修复复审纠偏（P1-1 ~ P2-2）

对本次合规修复进行第二轮质量复审，发现并修复 7 项遗留问题（3 P0 + 4 P1/P2）：

- **P1-1 脚本扩展为目录前缀匹配**：`scripts/check-sensitive-paths.sh` 原仅 `grep -Fx` 精确匹配，
  无法覆盖 `agentcore/permission/` 与 `guardrails/guardian/` 目录下新增文件。
  新增 `SENSITIVE_PATH_PREFIXES` 数组 + `grep -F` 前缀匹配循环。
- **P1-2 `currentCitationVerifier` 并发安全**：原为裸 `var CitationVerifier`，
  `mady eval --workers N` 并发调用 `Compute()` 存在 data race。
  改为 `atomic.Pointer[CitationVerifier]`，`SetCitationVerifier` 走原子 Store，
  新增 `TestSetCitationVerifierConcurrent` 用确定性 barrier 协调（writer 与 readers
  通过 channel 握手，保证每轮切换的 verifier 都被所有 readers 观察到），断言两个
  可区分 verifier（score=0.0 与 0.5）均被实际观察到——而非仅检查 `score ∈ [0,1]`
  （后者是恒真的假阳性断言）或靠 goroutine 调度运气（后者 flaky）。
  （`go test -race -count=5` 通过）。`metrics_test.go` 改用公开 API `SetCitationVerifier`
  而非直接 var 赋值。两个测试均标注 `// 不可加 t.Parallel()` 以防未来误用。
- **P1-3 commitlint CI 触发口径**：`.github/workflows/ci.yml` 原 `on: [push, pull_request]`
  导致 push 到 main 时会 lint 单个 squash commit（语义无意义）。
  改为 `if: github.event_name == 'pull_request'` 仅 PR 触发，并补注释说明覆盖范围。
- **P1-4 README 示例硬编码 `"./sessions"`**：README.md:380 仍示范旧反模式。
  改为展示 `util.ResolveDataDir("sessions")` + error handling 正确范式。
- **P2-1 `ResolveDataDir` 错误被静默吞掉**：`cmd/mady/{framework,server,tui}.go` 三处
  `sessionDir, _ = util.ResolveDataDir(...)` 用 `_, _` 丢弃 error。
  改为捕获并 `log.Printf` / `fmt.Fprintf(os.Stderr, ...)` 输出，便于兜底分支异常时定位。
- **P2-2 过期"修复："注释前缀**：`cmd/mady/server.go:69` 残留 `// 修复：使用 fc.WorkspaceDir...`
  合并到 P2-1 同一 edit 中清理为现在时态描述。

### 验证（复审后全量重跑）
- `go build ./...` / `go vet ./...`：OK
- `cd tools && go build ./... && go vet ./...`：OK
- `golangci-lint run ./...`（根 + tools）：0 issues
- `go test -race -count=1 ./... 2>&1 | grep -E "^(ok|FAIL|---)"`：70 包全部 `ok`，无 FAIL
- `cd tools && go test -race -count=1 ./...`：2 包全部 `ok`
- 敏感路径清单 5 份文档条目数一致：脚本 18（权威源）、4 份管理文档 20（含目录级 2 条）

> **注**：本次修复声称"全量测试通过"时曾出现失误——首次跑 `go test -race ./...`
> 输出被 `tail -80` 截断，掩盖了工作区中其它无关改动（`tui/agentadapter/adapter_test.go`、
> `tui/theme/global.go` 等）导致的测试失败。本次 commit 前，已通过 `git stash`
> 隔离所有无关改动，本次修复涉及包的测试结果见上。

### 涉及文件（本次修复 24 个 = 21 改 + 2 新增 + 1 删除）
**新增**：`tui/component/domain.go`、`commitlint.config.js`
**删除**：`agentcore/message_domain.go`
**修改**：
- `scripts/check-sensitive-paths.sh`、`SECURITY.md`、`AGENTS.md`、`CLAUDE.md`、`docs/GO-DEVELOPMENT-STANDARDS.md`、`tui/LAYERS.md`、`README.md`
- `tui/chat/chat_history.go`、`tui/component/{evidence,conclusion,approval}_card.go`
- `server/server.go`
- `.github/workflows/ci.yml`
- `cmd/mady/{framework,server,tui,eval}.go`
- `agentcore/evaluate/metrics.go`、`agentcore/evaluate/metrics_test.go`
- `scripts/replay_citation_metrics/main.go`

> **隔离记录**：commit 前工作区另含 5 个无关改动（`agentcore/event.go`、
> `tui/agentadapter/adapter_test.go`、`tui/theme/global.go`、
> `tui/component/input_test.go`、`tui/component/selectlist_test.go`），
> 对应上方"TUI 质量审查后续修复"条目。已 `git stash` 保留，本次修复 commit 不含这些改动。

---


## 2026-07-18: Code Review 第二轮修复（5 项 P0/P1 + 1 项测试质量）

### 背景
`/review` 对未提交改动 + 既有代码扫描后报告 8 项问题（4 P0 + 2 P1 + 2 P2）。逐项核查后发现 **3 项误报**（审查员看的是旧版本），**5 项真问题 + 1 项测试质量建议**。用户选择"全部 8 项修复 + 先修问题再一起提交"。

### 误报核查（3 项）
| # | 审查声称 | 实际状态 | 结论 |
|---|---|---|---|
| 1 | `audit.go` key 派生用 `copy` | 实际已是 `sha256.Sum256([]byte(keyStr))` | 误报（旧版本） |
| 2 | `claim_drafting.go:43` 空输出丢 claims | 已有 `else { state[StateKeyOutput] = claims }` 分支 | 误报（旧版本） |
| 3 | `legal.go:118` 注释引用不存在方法 | 当前文件只有 114 行，该注释不存在 | 误报（旧版本） |

### 真实修复（5 项）
1. **P0** `domains/audit.go` Encryptor 伪加密 → 真 AES-256-GCM 实现
   - 新增 imports：`crypto/aes`、`crypto/cipher`、`crypto/rand`、`encoding/base64`、`io`
   - `Protect`：用 AES-256-GCM 加密，返回 `base64(nonce || ciphertext)`
   - `Reveal`：base64 解码 → AES-GCM 解密；任何失败 fail-open 返回原文（兼容迁移期间未加密数据）
   - 修复 gocritic `appendAssign` 警告：`append(nonce, sealed...)` → `make + 双 append`
   - 验证：Encryptor 无任何调用方，纯占位实现升级，零连锁影响
2. **P0** `disclosure/claim_drafting.go:34,38` Pregel 节点错误返回 `nil` → `state`
   - 两处 `return nil, fmt.Errorf(...)` → `return state, fmt.Errorf(...)`
   - 与 `pregelAgentNode` 等其他节点的 `state, err` 契约对齐，避免后续 Pregel 加入 error-bridge 时 NPE
3. **P0** `disclosure/claim_drafting.go` 孤儿权利要求防护
   - 新增节点入口 guard：`len(ext.PFETriples) == 0` 时直接返回 `state, err`
   - 原问题：当 `Problems` 为空且 `Features` 非空时，`linkPFETriples` 不会回填，会生成 `DependsOn:1` 但权利要求 1 不存在的孤儿，违反专利法第 26 条第 4 款引用清楚性要求
4. **P1** `guardrails/citation_table.go:69` 商标法总条数 75 → 73
   - 核对 CNIPA 公布的《商标法》（2019 年第四次修正）原文，最后一条是第七十三条（非七十五条）
   - 注释同步更新："共八章 75 条" → "共八章 73 条"
   - 影响：条号 74、75 不再被误判为合法引用，S1 存在性核验防线恢复
5. **P1** `disclosure/graph.go:271` 图拓扑注释过时
   - `generate_report → review_gate → __end__` → `... → review_gate → draft_claims → __end__`
   - 与实际拓扑（line 334-336）对齐，避免新接入方误以为 review_gate 是终止节点

### 测试质量补充（1 项）
- `workflows/patent/oa_response_test.go:87-90` `TestParseOANode_InventivenessRejection`
  - 原仅 `t.Logf` 无 assert → 改为 `if rejectionType != string(rules.OaInventiveness) { t.Errorf(...) }`
  - 该测试原本等于无断言，等于不测；现在真正验证"创造性 → OaInventiveness"的映射

### 验证
- `go build ./...` / `go vet ./...`：OK
- `goimports -l` / `gofmt -l`：clean
- `golangci-lint run ./domains/... ./disclosure/... ./guardrails/... ./workflows/...`：0 issues
- `go test -race -count=1`：domains / disclosure / guardrails / workflows/patent 全部 PASS

### 涉及文件（5 个）
`domains/audit.go`、`disclosure/claim_drafting.go`、`disclosure/graph.go`、`guardrails/citation_table.go`、`workflows/patent/oa_response_test.go`

---


## 2026-07-18: P2-5 大文件拆分（4 个 >1000 行文件 → 20 个聚焦文件）

### 背景
全量技术债务扫描后，发现 4 个超过 1000 行的文件集中了多个不相关的职责：
`mcp/client.go` (1005)、`mcp/http.go` (1023)、`a2a/server.go` (1027)、`server/server.go` (1324)。
单文件职责过重影响可读性、增量编译效率、code review 质量，且新增功能时容易产生 merge conflict。
按"小炸弹不是大炸弹"原则，分 4 个 phase 顺序拆分（按风险升序：mcp → a2a → server）。

### 拆分明细

| Phase | 原文件 (行数) | 拆分后文件 | commit |
|---|---|---|---|
| 1 | `mcp/client.go` (1005) | `client.go` + `stdio_extension.go` + `client_jsonrpc.go` + `client_readloop.go` + `client_reconnect.go` (5 文件) | `cbeec0d` |
| 2 | `mcp/http.go` (1023) | `http.go` + `http_jsonrpc.go` + `http_sse.go` + `http_session.go` (4 文件) | `23baa2c` |
| 3 | `a2a/server.go` (1027) | `server.go` + `server_options.go` + `server_taskstate.go` + `server_jsonrpc.go` + `server_card.go` (5 文件) | `aafc6ec` |
| 4 | `server/server.go` (1324) | `server.go` + `types.go` + `pool.go` + `chat.go` + `thread.go` + `skills.go` (6 文件) | `6ee1dd3` |

### 拆分原则
- **同包内拆分**：每个新文件保持 `package` 不变，Go 天然支持同包跨文件可见 unexported 符号
- **按职责聚类**：每个新文件聚焦单一主题（jsonrpc / sse / session / reconnect / thread / skills / pool / types）
- **零外部 API 变更**：导出符号在原 package 内重组，签名不变；调用方（cmd/mady, example/, tools/）零影响
- **测试零修改**：所有 `_test.go` 在 same package，可见 unexported 符号，无需调整
- **每个 phase 一 commit**：便于独立 revert 与 code review

### 收益
- 4379 行 → 20 文件，每文件 80-510 行，单文件平均行数下降 ~70%
- HTTP MCP 客户端的 jsonrpc / sse / session 三个子问题彻底分离，后续维护可独立修改
- server/server.go 的 use-after-free 防护逻辑（pool.go）独立成文件，安全审计边界更清晰
- 增量编译：修改 chat 逻辑不再触发 thread/skills 的重新编译

### 关键陷阱记录
1. **Write 工具偶发"假成功"**：Phase 2/3/4 多次遇到 Write 报"Wrote file successfully"但文件实际未创建，build 时报符号 undefined。**对策**：每写一个文件立即 `ls -la` 验证
2. **goimports 路径**：本机 PATH 不含 `$(go env GOPATH)/bin`，`.pre-commit-config.yaml` 保留绝对路径 `/Users/xujian/go/bin/goimports`，不要改为 PATH 查找（会让 hook 在本机立即失败）
3. **gosec G204 豁免**：`mcp/client.go` 拆分后 `exec.Command` 迁移到 `client_reconnect.go`，`.golangci.yml` 的豁免规则需同步扩展为 `mcp/client\.go|mcp/client_reconnect\.go`

### 验证
每个 phase 完成后均跑全套验证（根 + tools 子模块）：
- `go build ./...`：OK
- `go vet ./...`：OK
- `goimports -l` + `gofmt -l`：clean
- `golangci-lint run ./...`：0 issues
- `go test -race -count=1 ./...`：全部 PASS，无 DATA RACE

---


## 2026-07-18: 全量技术债务清理（P0 真实 Bug + P1 Lint 债务 + P2 结构性改进）

### 背景
对 Mady（858 文件 / ~185K 行 / go.work 多模块）执行全量技术债务扫描，发现 7/16 REVIEW
后的 Tier 3 提交引入了大量未跑 lint 的新代码，lint 从 4 issues 涨到 35 issues。本次
集中清理 3 项真实 Bug + 5 类 lint 债务 + 4 项结构性改进，根模块与 tools 子模块均达到
`golangci-lint run ./...` 0 issues。

### P0：真实 Bug（3 项）

| # | 文件 | 问题 | 修复 |
|---|------|------|------|
| A | `tools/vision.go:244` | 运算符优先级歧义：`len>=2 && II \|\| MM` 等价于 `(len>=2 && II) \|\| (MM)`，当 `len<2` 且首字节为 'M' 时 `data[1]` 越界 panic | 改为 `if len(data) >= 2 { if magic := string(data[:2]); magic == "II" \|\| magic == "MM" {...}}`，补 7 个回归用例（含单字节 'M'/'I' 不 panic） |
| B | `mcp/install.go:304` | MCP 配置文件权限 0o644 允许同组/其他用户读取（配置含 token、命令执行白名单） | `0o644` → `0o600` |
| C | `a2a/push.go` | **SSRF TOCTOU 漏洞**：`validateWebhookURL` 提前 `net.LookupHost` 查私网 IP，但 `http.Client.Do(req)` 内部会再次 DNS 解析，DNS rebinding 攻击可在两次查询间切换 IP 绕过防护 | 新增 `ssrfSafeDialer.DialContext`：自行 `LookupIPAddr` 后选首个公网 IP 用 `net.JoinHostPort` 重组地址直连，**完全绕开 Transport 内部二次解析**；`validateWebhookURL` 移除 `LookupHost` 调用（私网检查下沉到 dialer）。补 4 个 SSRF 测试（dialer 拒私网/放行、URL 校验、端到端 Notify） |

`.golangci.yml` 新增白名单：`a2a/push\.go` 的 G704，附注释说明运行时防护消除 taint。

### P1：Lint 债务批量清理（31 项）

| 类别 | 数量 | 文件 |
|---|---|---|
| gofmt | 10 | `a2a/server.go`、`agentcore/cache/{policy,stats}.go`、`mcp/{client,http}.go`、`memory/compiler/rule_engine_bridge.go`、`server/server.go`、`tools/browser.go`、`workflows/autoresearch/{contract,heartbeat}.go`（一次 `gofmt -w .` 修复） |
| QF1012（`WriteString+Sprintf` → `Fprintf`） | 15 | `domains/style.go` x9、`prompt/loader.go` x4、`domains/doctmpl/loader.go` x2 |
| errcheck | 1 | `provider/adapter/session.go:65` `io.Copy` 改 `_, _ = io.Copy`（goroutine stderr 复制，错误无法返回） |
| gocritic ifElseChain → switch | 3 | `a2a/agent_handler.go:255`、`tui/component/editor_edit.go:34,42` |
| misspell | 1 | `session/session_store.go:181` `serialises` → `serializes` |
| SA9009 | 1 | `domains/style.go:19` 注释含 `//go:embed` 文字被误判，重写注释消除 |

### P2：结构性改进（3 项）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `tools/browser_tool_handlers.go:117` | SPA 渲染缓冲 1s sleep 保留（无对应集成测试可校准），强化注释说明经验值来源及修改前置条件 |
| 2 | `memory/sqlite_store.go:526` | `_ = json.Unmarshal(...)` 保留（与同函数 `time.Parse` 一致的 best-effort 降级策略），加注释说明 metadata 列无 schema 约束、损坏时降级为 nil 以避免单条记录阻塞整次查询 |
| 3 | `knowledge/loader/law_index_test.go:135` | `os.UserHomeDir()` + `/.mady` 硬拼接 → `util.MadyHome()`，对齐 AGENTS.md 的"任意 cwd 启动 / `$MADY_HOME` 覆盖"原则；并加 `t.Setenv("MADY_HOME", t.TempDir())` 隔离副作用（`MadyHome()` 会 `EnsureDir` 创建目录）。其余两处 skip（`patent_retriever_test.go`、`stage2_wiring_test.go`）经审查为合理的 corpus/env 门控，保留 |

> 备注：`.pre-commit-config.yaml` 的 goimports 绝对路径硬编码问题在评估后**未修改**——本机 PATH 不含 `$(go env GOPATH)/bin`，改为 PATH 查找会让 hook 在本机立即失败；AGENTS.md 已有"换机器需重装 goimports 并调整该路径"的警告，保持现状更稳。

### 已审查的非问题（避免误改）
- 32 处 `_ =` 忽略错误中 28 处为合理 defer cleanup；4 处 a2ui `json.Unmarshal` 注释明确合约
- 9 处 `time.Sleep(1s)` 中 `process.go:416` 进程轮询合理，浏览器自动化 8 处合理
- 所有 `MustXxx` panic = Go 惯例；全库无 `recover()` 吞 panic 反模式
- TODO/FIXME/HACK 标记为 0；无循环依赖（8 层洋葱架构）
- 二进制产物（mady/acp-server/cli-chat 等）在 `.gitignore` 中，未入库

### 未做（待评估）
- **4 个大文件拆分**（`server/server.go` 1325 行 / `a2a/server.go` 1031 行 / `mcp/http.go` 1024 行 / `mcp/client.go` 1006 行）：风险较高，需配套测试，标记为 P2-5 单独评估
- Phase 4 backlog（multi_hypothesis 双雄辩论等 6 项）已在 `docs/decisions/phase4-backlog.md` 规划

### 验证
- `go build ./...` ✅（根 + tools 子模块）
- `go vet ./...` ✅（根 + tools 子模块，零警告）
- `golangci-lint run ./...` ✅ **0 issues**（根 + tools 子模块）
- `go test -race ./...` ✅ 全部 60+ 包通过（根 + tools），无数据竞争
- `gofmt -l .` ✅ clean（根 + tools）

### 涉及文件（共 21 个）
`.golangci.yml`、`a2a/a2a_test.go`、`a2a/agent_handler.go`、
`a2a/push.go`、`a2a/server.go`、`agentcore/cache/{policy,stats}.go`、
`domains/doctmpl/loader.go`、`domains/style.go`、`docs/decisions/AI_CHANGELOG.md`、
`knowledge/loader/law_index_test.go`、`mcp/{client,http,install}.go`、
`memory/compiler/rule_engine_bridge.go`、`memory/sqlite_store.go`、`prompt/loader.go`、
`provider/adapter/session.go`、`server/server.go`、`session/session_store.go`、
`tools/browser.go`、`tools/browser_tool_handlers.go`、`tools/vision.go`、
`tools/vision_test.go`、`tui/component/editor_edit.go`、
`workflows/autoresearch/{contract,heartbeat}.go`


## 2026-07-18: Open Design 思路引入 —— Tier 3 实现（最终阶段）

### 背景
完成 Open Design 思路引入的第三阶段（高影响力/高复杂度），覆盖所有剩余 3 项特性。
至此，全部 8 个思路已分三个 Tier 完整引入 Mady。

### 本次实现（Tier 3 — 高影响力 / 高复杂度）

| # | 特性 | 改动 |
|---|------|------|
| 6 | **文档模板库** | `domains/doctmpl/`：`DocTemplate` 类型 + `LoadDocTemplates` / `LoadDocTemplatesFromFS` / `ResolveDoc` / `FindDocByCategory` / `DocIndex` API。`doc-templates/` 含 12 个 Markdown 模板（claims 3 / specification 4 / oa-response 3 / disclosure 2），使用 `{{variable}}` 占位符语法。`go:embed` 嵌为二进制内置模板，用户可在 `$MADY_HOME/doc-templates/` 覆盖或新增 |
| 7 | **Pipeline Atoms** | `agentcore/atom.go`：`Atom` 接口（Name/Description/Category/InputSchema/OutputSchema）+ 5 个具体原子操作（search / extract / compare / reasoning / approval-gate）+ 全局注册表（RegisterAtom / LookupAtom / ListAtoms / ListAtomsByCategory / AtomIndex）。`agentcore/plugin.go`：`PluginStage` 新增 `Atom` 字段，校验阶段验证原子引用有效性；3 个 plugin.json 已更新使用 atom 引用替代裸 tool 名 |
| 8 | **Agent 适配器模式** | `provider/adapter/`：`AgentAdapter` 接口（Name/Description/Detect/Spawn/Capabilities）+ `AgentSession` 接口（Send/Stream/Close）+ `AgentCapabilities` / `SpawnConfig` / `StreamChunk` 类型 + 全局注册表（RegisterAdapter / LookupAdapter / ListAdapters / DetectAll / AdapterIndex）。`claude.go`（Claude Code 适配器）+ `codex.go`（Codex CLI 适配器），均通过 `cliSession`（stdin/stdout 双向通信）实现 AgentSession |

### 技术细节

**文档模板库（借鉴 OD 的 template system）**：
- Markdown + YAML frontmatter 格式，`extractFrontmatter` 解析器
- `LoadDocTemplatesFromFS` 统一 fs.FS 接口，支持 embed.FS 和 os.DirFS
- `ResolveDoc` 使用 `strings.ReplaceAll("{{"+key+"}}", value)` 替换变量
- `DocIndex` 按 category 分组输出可读索引，适合 CLI 帮助

**Pipeline Atoms（借鉴 OD 的可组合 pipeline 原语）**：
- 5 个原子覆盖完整专利工作流：搜索 → 提取 → 对比 → 推理 → 审批
- `PluginStage.Atom` 引用注册表原子，实现从"粗粒度工具名"到"语义原子"的升级
- 向后兼容：Tool 字段仍有效（当 Atom 为空时）；两者共存时 atom 提供语义约束
- 原子通过 init() 自动注册，包导入即可使用

**Agent 适配器模式（借鉴 OD 的 agent composability）**：
- `detect → spawn → stream → capabilities` 四步统一接口
- `cliSession` 共享实现适用于所有 CLI-based 编码助手（Claude/Codex/Cursor/Copilot）
- `DetectAll` 运行时发现已安装的编码助手及其可用性
- `AdapterIndex` 生成终端诊断表格（✓/✗ 状态）
- mock 实现（mockAdapter/mockSession）便于单元测试

### 不影响
- 所有新增特性为纯新增，不修改任何已有生产代码路径
- Pipeline Atoms 通过 PluginStage.Atom 可选引用，不改变已有 plugin.json 的行为
- doc-templates 为独立目录，不影响已有文件
- 不涉及安全红线路径

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部 82 包通过，含新增 3 个测试文件，18+ 新用例）
- `go vet ./...` ✅（零警告）
- `tools/` 子模块 build/test/vet ✅
- 项目全量无新增 lint 问题


## 2026-07-18: Open Design 思路引入 —— Tier 2 实现

### 本次实现（Tier 2 — 中等影响力）

| # | 特性 | 改动 |
|---|------|------|
| 4 | **结构化 DOCUMENT_STYLE.md** | `domains/style.go`：`DocumentStyle` 类型（YAML 风格指南，包含 tone/voice/anti_patterns/disclaimers/citation/output_conventions），`SystemPrompt()` 方法生成注入文本。`styles/` 目录含 4 个默认风格（patent-standard / legal-standard / chat-friendly / assistant-neutral） |
| 5 | **专利工作流插件系统原型** | `agentcore/plugin.go`：`PluginManifest` 类型 + `ValidatePlugin` + `ScanPlugins` + `LoadPlugin`。`plugins/patent/` 含 3 个原型插件（novelty-analysis / infringement-check / oa-response），每个插件 = `plugin.json` + `SKILL.md`，pipeline 由可组合 stages 构成 |

### 技术细节

**DocumentStyle（借鉴 OD 的 DESIGN.md）**：
- YAML 格式，9 个结构化章节替代原有纯 prose `tone-style-guide.md`
- `SystemPrompt()` 自动生成 agent 注入文本，按领域（patent/legal/chat/assistant）可切换
- 反模式（anti_patterns）标注 severity（block/warn）便于自动化检测

**Plugin System（借鉴 OD 的 open-design.json + pipeline）**：
- 插件 = `plugin.json`（manifest）+ `SKILL.md`（agent contract）
- Pipeline 由可组合 stages 构成，每个 stage = id + tool + description
- 复用现有 `ValidateManifest` 校验框架（name regex / domain / guardrail_level）
- `ScanPlugins` 自动发现 `plugins/` 目录下的所有 `plugin.json`
- 3 个原型覆盖核心专利场景：新颖性分析（5 stages）、侵权比对（7 stages）、OA 答复（6 stages）

### 不影响
- 不涉及安全红线路径
- 原有 `tone-style-guide.md` 保留为人可读参考文档
- 插件与现有 SKILL.md 技能系统并行运作

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部通过，含新增 9 个测试文件，30+ 新用例）
- `go vet ./...` ✅


## 2026-07-18: 引入 Open Design 项目思路 —— 三阶段 Tier 1 实现

### 背景
深度研究 [open-design](https://github.com/nexu-io/open-design)（79K+ Stars）项目，
识别出 8 个可引入思路并按影响力排序。详见 plan.md 完整分析报告。

### 本次实现（Tier 1 — 高影响力 / 低风险）

| # | 特性 | 改动 |
|---|------|------|
| 1 | **MCP Install CLI** | `mcp/install.go` + `cmd/mady/mcp_install.go`：`mady mcp-install <agent>` 一键将 Mady 的 ACP 服务端接入 Claude Code / Codex / Cursor / Gemini CLI / GitHub Copilot 等编码 Agent |
| 2 | **SKILL.md 扩展字段** | `skill/skill.go` 新增 `MadyExtension` 结构体（mode/guardrail_level/approval_required/inputs/example_prompt/capabilities/handoff_allowed），`skill/frontmatter.go` 新增 YAML 解析，4 个现有 SKILL.md 已添加 `mady:` 扩展段 |
| 3 | **提示词模板库** | `prompt-templates/` 目录含 16 个 curated 模板（检索/分析/撰写/OA/交底书/法律），`prompt/loader.go` 提供 `LoadPrompts` / `ResolvePrompt` / `FindPromptByTrigger` / `FindPromptByName` API |

### 技术细节
- MCP Install 复用已有 `MCPServerConfig` / `MCPConfigFile` 类型，支持 `--list` / `--print` / `--uninstall`
- SKILL.md `mady:` 扩展使用 `gopkg.in/yaml.v3` 解析，向后兼容（无扩展字段的旧 SKILL.md 不受影响）
- 提示词模板使用 `{{variable}}` 语法，与现有 `prompt.Template` 渲染器独立并存

### 不影响
- 不涉及安全红线路径（不修改 `agentcore/handoff.go`、`guardrails/levels.go`、`tools/bash.go` 等）
- `tools/` 子模块无变更，多模块工作区规则无违反

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部通过，含新增 5 个测试文件）
- `go vet ./...` ✅


## 2026-07-18: Code Review 全部 6 个问题修复

### 修复清单

| 优先级 | 问题 | 改动 |
|--------|------|------|
| 🔴 P1 | `formatEnhancedReport` silient 吞 baseline 错误 | 改用 `fmt.Fprintf(os.Stderr, ...)` 输出读文件/解析JSON的警告信息 |
| 🟡 P2 | `ReasoningStrategyRouter` 每次 BeforeModelCall 原地修改系统消息 | 改为 `cp := msg; cp.Content += hint; mcc.Request.Messages[i] = cp` 先拷贝再赋值 |
| 🟡 P2 | 4 份完全相同的 DoomLoop 配置拷贝 | 提取 `domains/lifecycle.go` 中的 `defaultDoomLoopHook()`，4 个领域统一调用；消除 4 处 `doomloop` 子包 import |
| 🟡 P2 | ChatAgent 收到"逐步推理"策略提示不适合聊天 | 创建 `chatSelector` 并将 `StrategyHintInjection = false`，保留 effort/budget 调整但不注入系统提示 |
| 🟢 P3 | `SuppressPersist` 位置与内容修改分离 | 合并为 `mcc.Response.SuppressPersist = g.config.Level >= LevelStrict` 紧跟在 `Content` 赋值之后 |
| 🟢 P3 | 集成测试 signal 收集 5 处重复模板代码 | 提取 `signalCapture` + `newDoomloopWithCapture` + `runStubAgent` 辅助函数，5 个测试各减少约 30 行重复代码 |

### 额外改进
- 集成测试新增 `signalCapture.requireDetected` / `requireNone` 方法，统一信号断言逻辑
- 新增 `runStubAgent` 辅助函数消除测试中重复的 `agentcore.New` + `defer agent.Close()` + `agent.Run` 样板代码
- 4 个领域文件移除不再需要的 `"github.com/xujian519/mady/agentcore/doomloop"` import


## 2026-07-18: ReasoningStrategyRouter 接入 4 个领域 Agent

### 改动
- **domains/assistant.go**：在 DoomLoop 之后接入 `ReasoningStrategyRouter`，
  通用助理场景默认使用 StepByStep → StructuredAnalysis → VerifiedThinking
  三级复杂度策略
- **domains/chat.go**：同助理模式接入，聊天场景也能在复杂问题时获得结构化推理支持
- **domains/patent.go**：专利分析场景接入，默认策略映射适配专利审查/三性分析需求
- **domains/legal.go**：法律分析场景接入，策略提示涵盖三段论推理和法律适用框架

### 接线模式
每个领域在 Lifecycle 链中的位置：
```
DoomLoop (安全第一: 死循环检测)
  → ReasoningStrategyRouter (优化: effort/budget + strategy hint)
  → CitationGate (引用核验)
  → Guardrails (内容安全)
  → Psychological / Tools (扩展)
```
`ReasoningStrategyRouter` 使用默认配置（`NewDefaultClassifier` +
`NewDefaultStrategySelector`），策略映射为 Low→StepByStep、
Medium→StructuredAnalysis、High→VerifiedThinking。领域可通过注入自定义
`ComplexityClassifier` 或 `StrategySelector` 调整行为。


## 2026-07-18: DoomLoop 集成测试——7 个端到端测试覆盖 5 个探测器和领域接线

### 改动
- **integration/doomloop_e2e_test.go**（新增）：7 个集成测试用例，覆盖：
  - `TestDoomLoopE2E_ToolCallLoop` — mock provider 返回相同工具调用 3 次以上，
    验证 ToolCallLoop 信号正确发射
  - `TestDoomLoopE2E_TextRepetition` — provider 返回重复文本 + 持续工具调用，
    使 Agent 保持循环，验证 TextRepetition 信号正确发射
  - `TestDoomLoopE2E_CircuitBreaker` — provider 返回超出熔断器上限的工具调用，
    验证 CircuitBreaker 信号正确发射
  - `TestDoomLoopE2E_EmptyResult` — 注册返回空结果的工具，验证 EmptyResult
    信号正确发射
  - `TestDoomLoopE2E_NormalOperation` — provider 返回多样化响应（不同文本/
    不同工具），验证无误报信号
  - `TestDoomLoopE2E_DomainLifecycleChain` — 验证 DoomLoop 与自定义
    LifecycleHook 正确组合为 LifecycleChain，BeforeAgentRun 按序调用
  - `TestDoomLoopE2E_DomainConfigAssistant` — 使用 `domains.AssistantAgentConfig`
    构建完整领域 Agent，验证 DoomLoop 接线不导致构造或运行错误

### 测试设计要点
- 使用 `//go:build integration` 构建标签，遵循既有集成测试约定
- doomLoopProvider 结构体封装 mock LLM 响应模式，通过 contentFn/toolCallsFn
  函数字段灵活控制每轮调用返回的内容
- 注意到文本重复探测器的关键约束：Agent 在无工具调用的文本响应后立即退出内层
  循环，因此 TextRepetition 测试必须让 provider 同时返回重复文本和工具调用，
  使 Agent 持续迭代以积累探测器历史


## 2026-07-18: 评估框架增强收尾——新度量注册 + `--format enhanced` CLI

### 改动
- **suite.go**：`DefaultEvaluator()` 新增 `ToolAccuracy` 和 `WorkflowQuality`
  两个度量，跑批时自动覆盖工具调用准确性和工作流执行质量
- **cli/cli.go**：新增 `FormatEnhanced` 输出格式；`formatEnhancedReport()`
  调用 `BuildEnhancedReport` + `FormatEnhancedReport`，支持指标分解、
  百分位分布、最差/最佳用例展示；支持 `BaselineFile` 从 JSON 文件加载
  前次结果做趋势对比（退化/改善用例高亮）
- **evaluator.go**：`BatchReport` 和 `CaseResult` 添加 JSON tags，确保
  baseline JSON 文件可被正确反序列化
- **cmd/mady/eval.go**：新增 `--format enhanced` 和 `--baseline <文件>` 标志
- **main.go**：帮助文本增加 enhanced 格式示例

### 用法示例

```bash
# 增强报告输出（含指标分解 + 百分位 + 最差/最佳用例）
mady eval --format enhanced --suite p2a --mode static

# 带 baseline 的趋势对比
mady eval --format json --suite p2a --mode static -o baseline.json
mady eval --format enhanced --baseline baseline.json --suite p2a --mode static
```


## 2026-07-18: 引用核验 P2a——CitationSource 知识源抽象 + S2 wiki 法条索引（82 条全覆盖）

### 背景
设计方案（docs/design/citation-verification-gate.md）§5 决策二规划了
三层核验源降级：S1 内嵌静态表（P1b 落地）→ S2 知识库法条索引 → S3 联网
法条库。P1b 的 S1 静态表仅覆盖 30 条手工精校条目，核验大量落 Unknown
放行。P2a 落地 S2：运行时从 wiki 拆分法条（`~/.mady/knowledge/wiki/
法律法规/法律/专利法-2020-拆分-*.md`）的 H3 标题（### 第X条 <标题>）
构建「条号 → 主题关键词」索引，专利法 2020 全 82 条覆盖。

### 改动
- `guardrails/citation_source.go`（新增）：`CitationSource` 接口
  （Topics/MaxArticle 两方法）+ S1 静态表适配（DefaultCitationSource）+
  `CompositeCitationSource`（关键词并集去重 primary 在前、上限取 primary
  非零优先）+ `CitationSourceFuncs` 函数适配器（knowledge/loader 不
  import guardrails，装配侧 cmd/mady 组合注入，依赖倒置）
- `guardrails/citation_gate.go`：CitationGateConfig 新增 Source 字段 +
  WithCitationSource 选项；VerifyCitationsWithSource 导出（nil 源退回
  S1，VerifyCitations 行为零改动）；verifyOne 参数化知识源。
  **交叉匹配仍只查 S1 精校词**（crossMatchTopics 不走注入源）——S2 自动
  标题词只参与本条自证，不参与张冠李戴判定，误报防线 #1 的延伸
- `knowledge/loader/law_index.go`（新增）：BuildLawArticleIndex 遍历
  拆分文件（排除目录/实施细则/part 文件），lawcite.ParseChineseNumber
  解析条号，标题按「与/及/和/、」切分子短语（≥2 字去重）。
  **v1 只索引专利法-2020**：实施细则-2023 因 2001/2010/2023 条号漂移
  （考试答案按旧口径引用，用 2023 主题核验必误报）暂缓，留 P3+ 版本感知
- `pkg/lawcite`：导出 ParseChineseNumber（chineseToArabic 包装，供索引器）
- `scripts/smoke_citation_gate`：lint 收口（exitAfterDefer 抽 runSmoke
  函数、两处 G703 #nosec 说明）

### 验收（硬性）
- S1 默认源回放（go run ./scripts/replay_citation_gate）：三层 93 题
  真实幻觉全命中、误报 0 ✅（verifyOne 参数化后行为等价）
- S1+S2 复合源回放（knowledge/loader/law_index_replay_test.go，
  TestCompositeSource_Replay）：L0 TP 3 / L1 TP 2 / L3 TP 4，三层误报
  均 0 ✅——S2 自证未掩盖任何已知幻觉
- 真实 wiki 索引断言：82 条全覆盖、第 9 条含「先申请原则」、第 22 条含
  「创造性」✅

- **影响范围**: guardrails 2 改 2 新 + knowledge/loader 1 新 2 测 +
  lawcite 1 改 + scripts 1 改
- **风险等级**: 低（未注入 Source 时 Gate 行为与 P1b 逐字节等价；
  S2 词不参与交叉匹配，复合源经回放实测零误报）
- **审查要求**: L1（guardrails/ 改动对照 docs/chat-assistant-architecture.md）
- **验证**: go vet/build 双模块 ✅ | golangci-lint run 0 issues ✅ |
  go test -race ./... 全绿 ✅ | 双重回放验收 ✅（见上）


## 2026-07-18: 从 XiaoNuo Agent 引入评估框架增强、死循环检测和推理策略编排

### 背景
对 XiaoNuo Agent（TypeScript Bun 单体仓库，34 个 @nuo/* 包）做深度分析后，
识别出三大值得引入 Mady 的能力：评估框架增强、DoomLoop 死循环检测器、
推理策略编排系统。按优先级依次实施。

### 改动清单

#### Plan 3：评估框架增强（agentcore/evaluate/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `evaluate/cli/cli.go` | **新增** | CLI 评估引擎：RunCLI / FormatResult / OutputResult，支持 static 和 live 模式，table/json/markdown 三种输出格式 |
| `cmd/mady/main.go` | **修改** | 注册 `mady eval` 子命令 |
| `cmd/mady/eval.go` | **新增** | eval 子命令实现，flag 解析（--suite/--domain/--case/--format/--mode/--model 等） |
| `evaluate/tool_accuracy.go` + `_test.go` | **新增** | 工具调用准确性度量：三维度评分（工具选择 + 参数准确 + 调用顺序），12 个测试 |
| `evaluate/workflow_quality.go` + `_test.go` | **新增** | 工作流执行质量度量：步骤完成度 + 顺序 + 精确性，支持 Pipeline/Parallel/Router 模式，11 个测试 |
| `evaluate/reflection.go` + `_test.go` | **新增** | Reflection 自我反思质量评估 + RubricJudge 可定制多准则 LLM 评分，16 个测试 |
| `evaluate/loader.go` + `_test.go` | **新增** | JSON 夹具加载系统：支持单文件/数组/原生数组格式，目录递归扫描，7 个测试 |
| `evaluate/testdata/tool_accuracy_fixtures.json` | **新增** | 工具准确性评估示例夹具 |
| `evaluate/report_enhanced.go` + `_test.go` | **新增** | 增强报告：指标分解（均值/最值/标准差/通过率）、百分位分布、趋势对比（baseline diff），10 个测试 |
| `evaluate/eval_integration_test.go` | **新增** | 集成测试：全流水线测试（加载→评估→报告），4 个测试 |

#### Plan 2：DoomLoop 死循环检测器（agentcore/doomloop/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `doomloop/doc.go` | **新增** | 包文档，6 种检测器概览 |
| `doomloop/doomloop.go` + `_test.go` | **新增** | 核心实现：ToolCallLoop（重复工具调用）、TextRepetition（重复文本）、Cycle（A→B→A→B 循环）、EmptyResult（空结果）、CircuitBreaker（总迭代次数）、CompactionBreaker（重复压缩摘要）。25 个测试，实现 agentcore.LifecycleHook 接口 |

#### Plan 1：推理策略编排系统（agentcore/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `reasoning_strategy.go` + `_test.go` | **新增** | 策略选择器（6 种策略：step_by_step/structured_analysis/debate/tree_of_thoughts/verified_thinking/first_principles）、框架步骤系统（3 级复杂度→不同框架）、策略提示注入（BeforeModelCall 自动追加系统提示）。11 个测试 |

### 技术决策

1. **CLI 子包隔离**：将 CLI 引擎放在 `evaluate/cli` 子包而非 `evaluate` 包内，
   避免 benchmark 导入冲突（import cycle）。
2. **Metric 接口兼容**：所有新度量实现现有的 `Metric` 接口（`Name() + Compute()`），
   无需修改 Evaluator 核心。
3. **DoomLoop 通过 LifecycleHook 接入**：不影响 Agent 运行时的核心循环，
   检测器通过标准钩子机制注入，可独立启用/关闭。
4. **推理策略提示不侵入核心提示**：策略提示通过 `BeforeModelCall` 在每次调用前
   追加到 system message 末尾，不影响原始提示结构。

### 验证

- `go build ./...` ✅
- `go test ./agentcore/doomloop/` ✅（25 个测试）
- `go test ./agentcore/evaluate/...` ✅（全部 78+ 测试，含 7 个新测试文件的 60+ 测试）
- `go test ./agentcore/ -run "TestReasoning..."` ✅（11 个测试）
- `go build ./cmd/mady/` ✅
- `golangci-lint run ./...` 0 issues ✅（提交前收口：测试 nil Context →
  context.TODO、报告落盘权限 0644→0600、string 比较改 bytes.Equal、
  嵌入字段选择器简化、gofmt 全仓、main.go eval 错误返回检查）

- **影响范围**: agentcore/evaluate 10 新（含 cli 子包与 testdata）+
  agentcore/doomloop 2 新 + agentcore/reasoning_strategy 1 新 1 测 +
  cmd/mady 1 新 1 改
- **风险等级**: 低（均为新增包/钩子/子命令，默认 Agent 配置未改动；
  DoomLoop 与推理策略经 LifecycleHook 可选接入）
- **审查要求**: L1
- **验证**: 见上 ✅


## 2026-07-18: fix(tools) computer_use schema 畸形致 oMLX 500 + 引用核验端到端冒烟

### 背景
P1b 域接线后的端到端冒烟（domains.PatentAgentConfig 完整 hook 链 + oMLX
真实生成）首轮即 provider 500。curl 分层诊断（裸请求 ✅ / 简单工具 ✅ /
完整产品配置 ❌）定位到 1971fea 启用的 computer_use 工具 schema 畸形——
TUI assistant / patent agent 走 oMLX 等 OpenAI 兼容端点必现。

### 改动
- fix(tools)：`computerUseSchema()` 的 `"required": ["action"]` 误置于
  `properties` 内部（变成名为 "required" 的非法属性定义，值为数组），
  端点序列化该畸形 schema 时 500；移正为与 properties 平级的顶层键
  （对照 bash/browser 等全部工具写法一致，grep 确认为独例）。
  `TestComputerUseSchema` 补两条防回归断言（properties 内不得含
  "required" 键；顶层 required == [action]）——既有测试只断言字段
  存在故漏检
- feat(scripts)：`scripts/smoke_citation_gate` 端到端冒烟工具——完整
  PatentAgentConfig hook 链（CitationGate + Strict 护栏 + ApprovalGate）
  + oMLX 单题跑批（SMOKE_CASE / SMOKE_MAX_TOKENS / SMOKE_MAX_TURNS 可配）；
  SMOKE_FILE 离线模式对任意文本跑 guardrails.VerifyCitations 出判定报告

### 冒烟结论（patent_exam_2008_a31_02 单题）
- 链路通畅：修复后 1m27s 生成 2059 字，完整 hook 链无异常
- Gate 行为正确：本次生成引用 1 条且 Valid → 正确地未标注（非失灵）
- 幻觉命中实景（对 v0.8 L0 缓存真实幻觉答案离线核验）：4 条引用
  1 Valid / 2 Unknown / 1 Suspect，提示文案精确指出「专利法第47条第1款」
  用途"分案申请"与本条主题（无效宣告效力）不一致、更接近细则第42条

- **影响范围**: tools/ 2 文件改 + scripts/ 1 新增
- **风险等级**: 低（schema 修复经全部同类工具对照为独例）
- **审查要求**: L1
- **验证**: tools 模块 vet/build/test/lint 0 issues ✅ | 冒烟端到端 ✅ |
  离线核验演示 ✅


## 2026-07-18: 启用 computer_use 桌面控制工具（Assistant + Patent Agent）

### 背景
用户需要在专利检索与下载场景中控制本地桌面浏览器，操作 CNIPA/Google Patents 等网站。
`computer_use` 工具已在 `tools/` 中完整实现（macOS/Linux/Windows 三平台后端 + 安全拦截 + 测试），
但此前在所有领域 Agent 中均被禁用。

### 改动
- `domains/assistant.go`：`ExtensionConfig` 新增 `ComputerUse: true`，从 `DisableTools` 移除 `tools.ToolComputerUse`
- `domains/patent.go`：同上
- Legal Agent 保持原有禁用状态（暂无浏览器操作场景）
- 默认审批模式为 `COMPUTER_USE_APPROVAL=none`（仅拦截危险操作，不额外提示）

### 影响范围
- `domains/assistant.go`（+1 行 / -1 行）
- `domains/patent.go`（+1 行 / -1 行）

### 审查要求
- L1（简单配置变更，不涉及敏感路径）
- `computer_use` 已在 SECURITY.md 中列为需用户授权的敏感工具，用户已知悉


## 2026-07-18: 修复 PlanCompiler 边连接 bug——多假设子图被完全绕过

### 背景
`TestDrafting_WorkflowTool` 集成测试写入失败：步骤 4（StrategyMultiHypothesis）未产出任何内容，
步骤 5 的链式节点却消耗了本属于步骤 4 的 LLM 调用。调试发现 Pregel 执行仅 4 次 LLM 调用
（应 5 次），步骤 4 的多假设子图被完全绕过。

### 根因
`CompilePlanToGraph` 中，步骤间的边连接使用 `terminal`（子图最后一个节点）而非 `entry`
（子图入口节点）：
```go
// 原代码：
g.AddEdge(prevTerminal, terminal)  // terminal = rejectName（多假设子图末端）
// 应改为：
g.AddEdge(prevTerminal, stepEntry) // stepEntry = aThink（多假设子图入口）
```
对于 `StrategyChain` 和 `StrategyReact`，入口 == 末端（单节点或 think→observe），
原错误被掩盖。但对 `StrategyMultiHypothesis`，`terminal = rejectName` 而 `entry = aThink`，
前一步直接连到子图末端的 rejection 节点，整个多假设子图从不激活。

### 改动
- `domains/reasoning/plan_compiler.go`：引入 `stepEntry` 和 `stepTerminal` 变量，
  在 switch 分支中分别记录每个步骤的入口和终止节点。边连接改为
  `g.AddEdge(prevTerminal, stepEntry)`，`prevTerminal = stepTerminal`。
  将 `if i == 0 { entryName = ... }` 移出 switch 分支统一在循环体处理。

### 影响范围
- `domains/reasoning/plan_compiler.go`（`CompilePlanToGraph` 方法重写步骤间边连接逻辑）

### 风险等级
低（修正逻辑正确性；既有 graph 测试全部通过；4 个 drafting 集成测试全部通过；
全部 26 个集成测试回归绿色）

### 验证
- `go build ./...` ✅
- `go test ./graph/...` 52 测试全部通过 ✅
- `go test ./domains/reasoning/...` 全部通过 ✅
- `go test -tags integration ./integration/...` 26 集成测试全部通过 ✅
- `TestDrafting_WorkflowTool` 修复后 5 步全部输出、所有 LLM 调用正确 ✅

### Code Review 收尾修复

1. **`buildChainStep` 返回类型统一**：改为返回 `(string, string, error)`（entry == terminal 相同），
   使 `CompilePlanToGraph` 的 switch 四个分支全部用一致的 `(entry, term, err)` 模式，
   消除 StrategyChain 分支需单独赋值的视觉差异。
2. **提取 `injectDraftingTool` helper**：消除 `globalDraftingRunner` nil 守卫在
   `PatentAgentConfig` 和 `BuildProjectAgent` 两处的重复，各从 5 行压缩为 1 行调用。


## 2026-07-18: 知识系统全面优化（5 阶段）

### 背景
知识系统（knowledge.db 6.5GB / 8 万文档 / 14 万分块 / 21 万知识图谱节点）已建设完毕，
但向量检索链路因环境变量未注入进程而被禁用，实际仅使用 FTS-only（BM25 关键词搜索），
浪费了预先投入的 144K BGE-M3 向量索引和 RRF 融合架构。

### Phase 0 — 环境激活
- **`.envrc`**（新建）：direnv 自动加载 `.env`（`dotenv` 指令）
- **`cmd/mady/main.go`**：导入 `github.com/joho/godotenv/autoload`，Go 进程自动读取 `.env`
- **依赖**: 新增 `github.com/joho/godotenv v1.5.1`
- **效果**: OMLX_API_KEY / KNOWLEDGE_RERANK 等环境变量自动注入进程，向量检索链路激活

### Phase 1 — laws-full.db FTS5 索引
- **`~/.mady/knowledge/laws-full-local.db`**：复制 laws-full.db（152MB, 9121 部法规）为本地独立文件并添加 FTS5 trigram 索引（law_fts 虚拟表）
- **`knowledge/sqlite/store.go`**：`OpenLawsDB()` 自动检测 law_fts FTS5 表；`SearchLaws()` 双路搜索——3+ 字符用 FTS5 BM25 排序，短查询/无 FTS 时回退 LIKE
- **`cmd/mady/knowledge.go`**：优先加载 `laws-full-local.db`（FTS5 版），回退 `laws-full.db`（原始版）
- **效果**: 法律搜索从 LIKE 模糊匹配升级为 BM25 排序，相关度大幅提升

### Phase 2 — SQL 回退向量搜索并行化
- **`knowledge/sqlite/store.go`**：`VectorSearch()` 的 SQL fallback 路径从单线程 2000 行/批次扫描改为并行 goroutine（同 CPU 核数），每个 worker 维护 min-heap
- **新增 `vectorSearchSQLParallel()`**：并行 SQL batch 扫描 + 结果合并
- **效果**: SQL 回退搜索从 **13.8 秒降至 188ms**（73 倍提速）

### Phase 3 — EvalHook 默认启用
- **`knowledge/eval.go`**：`DefaultEvalConfig()` 中 `Enabled: false → true`
- **`knowledge/extension.go`**：`NewExtension()` 自动创建 EvalHook；`BackendHook()` 通过 `AppendLifecycle` 将 EvalHook 与后端检索钩子组合
- **效果**: 每次模型调用后自动评估 Faithfulness / AnswerRelevancy / ContextPrecision 并发送到事件总线

### Phase 4 — 启动诊断增强
- **`knowledge/sqlite/store.go`**：新增 `Stats()` 方法，返回文档/分块/向量/维度/内存统计
- **`knowledge/graph/store.go`**：新增 `NodeTypeCounts()` 方法，按节点类型统计
- **`cmd/mady/knowledge.go`**：启动时输出详细诊断（文档/分块/向量数、图谱节点类型分布）
- **效果**: 启动日志从 "active" 一句扩展为完整的知识库统计报告

### Phase 5 — Benchmark 回归套件
- **`Makefile`**：新增 `bench-knowledge` 目标
- **`docs/performance-baseline.md`**（新建）：性能基线文档
- **效果**: 每次知识系统变更后可一键运行基准测试并对比回归

### 性能基线（M4 Pro, 2026-07-18）

| 操作 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| FTS 全文搜索 | 44ms | **11ms** | 4× |
| 向量检索 (内存索引) | 31ms | **18ms** | 1.7× |
| 端到端检索 (FTS+向量+RRF) | 44ms | **30ms** | 1.5× |
| SQL 回退向量搜索 | **13.8s** | **188ms** | 73× |
| 向量预加载 (144K) | 391ms | **269ms** | 1.5× |
| 图谱增强 + 法规 FTS5 | 未测量 | 实时 | — |

### 影响范围
- `cmd/mady/main.go`, `cmd/mady/knowledge.go`, `Makefile`, `.envrc`, `go.mod`, `go.sum`
- `knowledge/eval.go`, `knowledge/extension.go`, `knowledge/ext_test.go`
- `knowledge/sqlite/store.go`, `knowledge/graph/store.go`
- `docs/performance-baseline.md`, `docs/decisions/AI_CHANGELOG.md`

### 风险等级
- 低（不修改现有 API 签名，测试全绿）

### 审查要求
- L1

### 验证
- `go build ./...` ✅ | `go test -count=1 ./knowledge/... ./retrieval/... ./memory/...` ✅ | `make bench-knowledge` ✅


## 2026-07-18: MCP 发现超时优化

### 背景
`mady tui` 启动缓慢（15-20s），经分析发现主因是 `~/.claude.json` 中 9 个 MCP 服务器
并发发现导致 10 秒超时等待。其中 3 个服务器（zai-mcp-server 通过 npx 启动 >15s、
jina-ai-mcp-server 和 tolaria 模块路径不存在）拖慢了整个发现流程。

### 改动
- **`~/.claude.json`**：移除 3 个失效/慢速 MCP 服务器（zai-mcp-server、jina-ai-mcp-server、tolaria），
  保留 6 个正常运行的服务（codegraph、professional-router、gemma4-multimodal、web-reader、web-search-prime、zread）
- **`mcp/config_discovery.go`**：MCP 发现总超时从 10s 缩短至 **3s**，
  即使个别服务器偶发延迟也不会阻塞启动流程

### 效果
| 指标 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| TUI 首帧渲染 | ~15-20s | **~4s** | ≈75% |
| Agent 就绪 | ~18s | **~6s** | ≈67% |

### 影响范围
- `mcp/config_discovery.go`
- `~/.claude.json`（用户外部配置，非仓库文件）

### 风险等级
- 低（不修改现有 API，测试全绿）

### 审查要求
- L1

### 验证
- `go build ./...` ✅ | `go test ./mcp/...` ✅
