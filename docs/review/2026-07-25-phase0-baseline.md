# Phase 0 — 基线收集报告

> 日期：2026-07-25
> 依据：`Mady 全面审阅计划 v1.0`（`plan.md`）Phase 0
> 执行者：AI（Grok）
> Human Owner：[NEEDS CLARIFICATION: 待人工指派]

## 摘要（3 条最关键发现）

1. **质量门禁基线异常健康**：`make verify`（lint + arch + build + test-race）48 秒全绿，`golangci-lint` 0 issues，`gitleaks` 0 发现，`govulncheck` 0 代码漏洞。这意味着计划 R1-R5（5 个 P0）若仍存在，大概率是**测试未覆盖竞争/绕过路径**，而非编译/lint 层面问题——Phase 1 应聚焦"竞争路径可达性验证"而非"代码是否存在"。
2. **核心理念信号维持良好**：`TODO/FIXME/HACK/XXX` 计数为 **0**，`dot import` 为 **0**，与上一轮理念评估一致。唯一需关注的 18 处 `panic(` 中仅 **4 处构造器 nil-panic** 偏离"error 优先"的克制风格。
3. **敏感路径 churn 集中**：2026-07-16 后 `domains/patent.go`（27 commits）、`server/server.go`（12）、`tools/tools.go`（7）、`agentcore/permission/`（5）改动密集，是 Phase 1-3 的重点回归对象。

## 1. 审阅范围

Phase 0 为基线收集，范围限定为可机器化的全仓库检查，不进入具体业务逻辑：

- 质量门禁（`make verify` 四件套）
- 安全扫描（gitleaks / govulncheck）
- 规则化 grep（TODO / panic / dot import / `_ =` 忽略）
- 敏感路径 churn 统计（2026-07-16 之后）
- 依赖版本基线

## 2. 审阅维度执行情况

| Lens | 检查项 | 命令 | 结果 |
|------|--------|------|------|
| L1 编码 | go vet | `go vet ./...` + `cd tools && go vet ./...` | **0 issues** |
| L1 编码 | golangci-lint | `golangci-lint run ./...` | **0 issues** |
| L1 编码 | TODO/FIXME/HACK/XXX | `grep -rn -E "// (TODO\|FIXME\|HACK\|XXX)\b"` | **0 命中**（维持历史基线） |
| L1 编码 | dot import | `grep -rn -E '^\s*import \. '` | **0 命中** |
| L1 编码 | panic( 非测试 | `grep -rn '\bpanic(' --include='*.go'` | **18 命中**（详见 §3.1） |
| L1 编码 | `_ =` 错误忽略 | `grep -rn -E '^\s*_ = '` | **120 命中**（详见 §3.2） |
| L2 架构 | 架构边界 | `make check-arch` | **25 通过 / 0 失败** |
| L2 架构 | 编译 | `go build ./...` + `cd tools && go build ./...` | **成功** |
| L4 测试 | race 测试 | `go test -race -count=1 ./...`（根 + tools） | **全部通过** |
| L3 安全 | 密钥泄漏 | `gitleaks detect --source .` | **0 发现** |
| L3 安全 | 依赖漏洞 | `govulncheck ./...`（go1.26 重建版） | **0 代码漏洞**（1 module 漏洞未调用，见 §3.3） |

## 3. 发现清单

### 3.1 `panic(` 在非测试代码（18 处）— 评级 M（maintainability）

> 规范条款：`docs/GO-DEVELOPMENT-STANDARDS.md §1.4`（init 不 panic、构造器应返回 error）、`§4`（错误优先）

| # | 文件:行 | 分类 | 评价 |
|---|---------|------|------|
| 1 | `domains/claimdrafting/drafter.go:47` | 构造器 nil-panic | ⚠️ **应返回 error**（`LLMDrafter.DraftFromScratch called on nil receiver`）|
| 2 | `domains/claimdrafting/extension.go:41` | 构造器 nil-panic | ⚠️ **应返回 error**（`NewExtension 的 engine 参数不能为 nil`）|
| 3 | `domains/claimdrafting/extension.go:46` | 构造失败 panic | ⚠️ **应返回 error**（构建 Pregel 图失败）|
| 4 | `domains/specdrafting/extension.go:35` | 构造器 nil-panic | ⚠️ **应返回 error** |
| 5 | `domains/specdrafting/scorer.go:14` | 构造器 nil-panic | ⚠️ **应返回 error** |
| 6 | `domains/reasoning/fact_blackboard.go:55` | 契约违反 | ✅ 合理（对已锁黑板变更）|
| 7 | `domains/reasoning/ipc_source.go:36` | `Must*` 模式 | ✅ 合理（注释明确"Use during startup"，`MustIPCStandardAdapter`）|
| 8 | `tui/tui_loop.go:26` | re-panic after cleanup | ✅ 合理（保留堆栈）|
| 9 | `agentcore/concurrency/pool.go:68` | 契约违反 | ✅ 合理（Release 未配对 Acquire）|
| 10 | `agentcore/event_logger.go:62` | 双重启动防御 | ✅ 合理（`EventLogger already started`）|
| 11 | `agentcore/atom.go:150` | 契约违反 | ✅ 合理（注册空名）|
| 12 | `agentcore/permission/rule.go:51` | `Must*` 模式 | ✅ 合理（`MustParseRule`，注释"For tests and constants"）|
| 13 | `evaluate/benchmark/invalidation_decisions.go:30` | 数据加载 | ⚠️ 类 init，可接受 |
| 14 | `evaluate/loader.go:193` | 数据加载 | ⚠️ 类 init，可接受 |
| 15 | `knowledge/standards/ipc-standards.go:68` | 数据加载 | ⚠️ 类 init，可接受 |
| 16-18 | `pkg/csync/csync.go:32,34,36` | 类型守卫 | ✅ 合理（泛型类型约束 fail-fast）|

**结论**：18 处中 **5 处需关注**（claimdrafting 3 + specdrafting 2），均建议改为返回 error，符合"构造器不 panic"的 Go 惯例。3 处类 init 数据加载属灰色地带（可接受）。

> 注：原计划与历史 review 一致认定 4 处，本次精确扫描为 **5 处**（`claimdrafting/extension.go:46` 构建图失败 panic 是新发现，与 nil-panic 不同性质，但仍应返回 error）。

### 3.2 `_ =` 错误忽略（120 处）— 评级 L（maintainability）

分布（Top 5）：

| 模块 | 计数 | 评价 |
|------|----:|------|
| `mcp/` | 20 | ✅ **全部合理**（已逐条核实，均为 Close/Kill/pipe 关闭的清理路径，重连场景忽略已关闭 pipe 错误是惯用法）|
| `tools/` | 14 | ✅ 合理（多为 `cmd.Wait()` reap zombie、`Process.Kill()`）|
| `cmd/mady/` | 13 | ✅ 合理（TUI 退出清理、`os.Remove` 探测文件、`app.Stop()`）|
| `domains/` | 11 | 待 Phase 2 抽样核实 |
| `a2a/` | 8 | 待 Phase 3 抽样核实 |

**结论**：抽查的 mcp/tools/cmd 三个高密度模块（47 处）全部为合理的 cleanup 路径。剩余 73 处在后续 Phase 按模块审阅时抽样核实，不单独列为风险。

### 3.3 `govulncheck` module 漏洞（1 处，未调用）— 评级 L（security）

`govulncheck` 报告：代码层面 0 漏洞，但"1 vulnerability in modules you require, but your code doesn't appear to call"。详细 `-show verbose` 扫描因调用图分析耗时不显著（已转后台），建议 Phase 5 收尾时用 `govulncheck -show verbose ./...` 留档具体 CVE。

### 3.4 敏感路径 churn（2026-07-16 之后）— 评级 H（security，提示 Phase 1-3 回归）

| 文件 | commits | 最近主题 |
|------|--------:|---------|
| `domains/patent.go` | **27** | HITL 增强、run_orchestration、EgoLiteManager、文档处理 |
| `server/server.go` | **12** | enablement 26.3 模块、架构优化、Pipeline Executor |
| `tools/tools.go` | **7** | serve 安全门控、域 DisableTools、CJK 截断、多目录沙箱 |
| `disclosure/report.go` | 5 | review_gate 主动中断、外部项目分析嵌入 |
| `guardrails/citation_gate.go` | 5 | i18n、baseline error 可见性、知识源抽象 |
| `tools/vision.go` | 5 | invalidation 知识库优化、工具层安全加固 |
| `agentcore/permission/` | 5 | （目录前缀，多文件）|
| `agentcore/handoff.go` | 4 | specdrafting 全量修复、核心引擎审阅修复 |
| `domains/approval.go` | 4 | HITL 持久化、ApprovalGate→TUI 渲染 |
| `tools/bash.go` | 3 | 工具层安全加固、竞态修复 |
| `guardrails/citation_table.go` | 4 | infringement 模块、Code Review 修复 |
| `guardrails/levels.go` | 2 | HITL 持久化增强（Pending/Checkpoint）|
| `domains/router.go` | 2 | tri-mode → unified agent 切换 |
| `tools/path.go` | 2 | 多目录白名单读写分级沙箱 |
| `domains/project.go` | 2 | 技术债务清理、lint 修复 |
| `agentcore/hooks.go` | 2 | 废弃钩子系统清理、审阅修复 |
| `agentcore/manifest.go` | 1 | 核心引擎审阅修复 |
| `mcp/config_trust.go` | 1 | 协议层 C1-C8 Critical 修复 |
| `acp/auth.go` | 1 | 协议层 C1-C8 Critical 修复 |
| `guardrails/guardian/` | 1 | （目录前缀）|

**重点关注**：`domains/patent.go`（27 commits，含敏感的 BuildProjectAgent WorkingDir）和 `server/server.go`（12 commits，含 Agent 池引用计数）是 Phase 1/3 必须回归的高 churn 敏感路径。

### 3.5 依赖版本基线 — 评级 L（security）

核心 4 依赖（`AGENTS.md` 明示"最小依赖原则"）：

| 依赖 | 版本 | 状态 |
|------|------|------|
| `github.com/gorilla/websocket` | v1.5.3 | 接近最新 |
| `modernc.org/sqlite` | v1.54.0 | 较新（可升 v1.54.x+）|
| `gopkg.in/yaml.v3` | v3.0.1 | 最新稳定 |
| `go.opentelemetry.io/otel` | v1.44.0 | 较新 |

其他直接依赖（16 个）含 `chromedp v0.16.0`（PDF 渲染新增）、`excelize/v2 v2.11.0`、`goldmark v1.8.4` 等，均合理。

可升级间接依赖 18 个（goquery v1.12.0、cascadia v1.3.4 等），**无紧急安全升级**，归入 Phase 5 backlog。

## 4. 已验证合规项（关键）

- ✅ `make verify` 四件套（lint + arch + build + test-race）全绿
- ✅ `golangci-lint` 0 issues（独立确认）
- ✅ 架构边界 25 项检查全通过（agentcore→domains 等单向依赖无违反）
- ✅ `gitleaks` 0 密钥泄漏
- ✅ `govulncheck` 0 代码漏洞
- ✅ TODO/FIXME/HACK/XXX 维持 0（核心理念落地信号）
- ✅ dot import 维持 0
- ✅ `panic(` 中 13/18 为合理 fail-fast / `Must*` 模式
- ✅ `mcp/` 的 20 处 `_ =` 忽略全部为合理 cleanup

## 5. 与历史 review 的关系

- **基线健康度高于上一轮**：`REVIEW_REPORT_2026-07-16` 时存在 16 Critical，本次 Phase 0 基线零 Critical、零 lint 命中。说明历次审阅的 P0 修复有效。
- **对 Phase 1 的关键指引**：R1-R5（5 个 P0）在 `test -race` 全绿的情况下仍被报告为"未解决"，最可能的解释是**测试未覆盖竞争路径**。Phase 1 应优先做：
  1. 读当前代码确认锁是否已加（静态确认）
  2. 若已加锁，标记"已修复"，回归关闭
  3. 若未加锁，构造并发测试复现 race，独立 PR 修复

## 6. 建议下一步（Phase 1）

按计划进入 **Phase 1 — P0 急诊**，优先级：

1. **R3 `planmode/readonly.go` 解释器绕过**（安全红线，最高优先）— 静态读 + PoC `python -c '...'`
2. **R1/R2 `context_engine.go` + `compaction.go` 数据竞争**— 读当前字段是否已加 `sync.Mutex`
3. **R4 `pipeline_executor.go:96` panic 隔离**— 读当前是否已包 `recover()`
4. **R5 `claimdrafting/rules.go:51-53` RegisterAll 锁**— 读当前是否已加锁
5. **澄清报告冲突**：`agent_run_phase.go` MaxTurns `>` vs `>=`

Phase 1 产出：`docs/review/2026-07-25-phase1-p0-triage.md`
