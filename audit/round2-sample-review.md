# 第 2 轮：分层抽样审阅报告

> **日期**: 2026-07-30 | **范围**: T1 高风险文件（~35 个）逐行审阅 + T2 抽样 + 6 类 AI 违规专项
> **方法**: 3 并行 agent + 14 项前台专项扫描 | **耗时**: ~25 分钟

---

## 一、执行摘要

第 2 轮审阅覆盖了全仓约 5% 的高权重文件（T1），这些文件占修改频率的 ~60%、安全敏感路径的 100%、超大文件的 80%。

**核心发现**：
- 🔴 3 个高风险 D15 违规（Context 传播缺失）：`domains/project.go`（6 个函数）、`tools/bash.go`（Exec 不用 CommandContext）、`knowledge/sqlite/store.go`（18 处 Background()）
- 🔴 1 个 nil 解引用风险：`domains/patent.go:256` 静默丢弃 error
- 🟡 9 个 Config struct 无 Validate() 方法
- 🟡 中英错误信息混用：`cmd/mady/tui_session.go`、`agentcore/errors.go`
- 🟡 `%v`/`%w` 混用：`knowledge/extension.go`（%v=5, %w=0）、`tools/bash.go`（混用）
- 🟡 37 个 tools 文件无测试，browser 模块 12+ 文件全无测试

---

## 二、T1 文件审阅结果（3 组汇总）

### 2.1 安全敏感文件组（15 文件，Agent 1）

| 文件 | 关键发现 | 严重度 |
|------|---------|--------|
| `agentcore/handoff.go` | 裸错误透传未用 `%w` 包装 (L84)；应使用 `HandoffError` (L93) | P2 |
| `guardrails/levels.go` | `Config` 无 `Validate()` | P1 |
| `guardrails/citation_gate.go` | `CitationGateConfig` 无 `Validate()` | P1 |
| `guardrails/citation_table.go` | 纯数据文件，无问题 | ✅ |
| **`domains/patent.go`** | **L256: `_` 忽略 error → nil tool 追加到 ExtraTools，可能 nil 解引用崩溃** | **P0** |
| `domains/approval.go` | `ApprovalConfig` 无 `Validate()`；L406 静默丢弃 `store.Save` error | P1 |
| `domains/router.go` | `appendLifecycle` 薄包装函数可移除 | P2 |
| **`domains/project.go`** | **6 个 I/O 函数不接受 `context.Context`（Register/Delete/Touch/SaveMeta/LoadMeta/RefreshStatus）** | **P0** |
| **`tools/bash.go`** | **`Exec` 不接受 ctx，用 `exec.Command` 而非 `CommandContext`，进程不响应取消；`BashToolConfig` 无 `Validate()`** | **P0** |
| `tools/path.go` | `WorkingDirSandbox` 无 `Validate()` | P1 |
| `tools/vision.go` | `VisionToolConfig` 无 `Validate()` | P1 |
| `mcp/config_trust.go` | 错误处理符合规范 | ✅ |
| `acp/auth.go` | 防时序侧信道；ctx 正确传播 | ✅ |
| `server/server.go` | 4 个 Set/Get 方法重复模式 | P2 |
| `disclosure/report.go` | 正确使用 `InterruptError` | ✅ |

### 2.2 核心基础设施文件组（16 文件，Agent 2）

| 文件 | 关键发现 | 严重度 |
|------|---------|--------|
| `agentcore/agent.go` | `mu` 命名偏离 `xxxMu` 规范；A2UIPromise 设计良好 | P2 |
| `agentcore/agent_run.go` | 通过全部检查 | ✅ |
| `agentcore/event.go` | `mu` 命名偏离 | P2 |
| `agentcore/lifecycle.go` | Observer 模式分化合理 | ✅ |
| `agentcore/compaction.go` | `mu` 命名偏离；`compactionState` 并发正确 | P2 |
| `agentcore/config.go` | 40+ With* 选项函数偏多但属正常 | P3 |
| `agentcore/provider.go` | 通过 | ✅ |
| **`agentcore/errors.go`** | **`RetryableError`/`FatalError` 用中文，`HandoffError`/`GuardrailError` 用英文——同文件风格不统一** | P1 |
| `tools/tools.go` | 中英注释混合；沙箱传播机制完善 | P2 |
| **`tools/browser_session.go`** | **5 处 error 消息大写开头；ErrNoActiveBrowserSession 句末有句号** | P1 |
| `tools/browser_supervisor.go` | `mu` 命名；goroutine 管理规范 | P2 |
| `mcp/discovery.go` | `mu` 命名；异步刷新有 panic recovery | P2 |
| `mcp/client.go` | 中英注释混合；生命周期管理规范 | P2 |
| **`knowledge/sqlite/store.go`** | **系统性 D15 违规：18 处 `context.Background()`，公有方法均不接受 ctx 参数** | **P0** |
| **`cmd/mady/tui_session.go`** | **7 处中英混用 error；L542 goroutine 中调用 UI 方法有竞态风险** | P1 |
| `cmd/mady/framework.go` | 薄 shim 层，无问题 | ✅ |

### 2.3 领域大文件组（14 文件，Agent 3 ✅）

所有 14 个领域文件均 >600 行，逐个审阅结果：

| 文件 | 行数 | sec2.4 职责 | D15 Context | 其他发现 |
|------|------|------------|-------------|---------|
| `domains/workflows/patent/rule_engine.go` | 1203 | **5 职责超载** | N/A | D14: 50 个术语常量仅用 1 次；建议拆 3 文件 |
| `domains/inventiveness/nodes.go` | 1041 | **5 职责超载** | ✅ | D13: Schema 字符串 vs jsType 常量不一致；建议拆 4 文件 |
| `domains/evidence/engine.go` | 815 | **5 职责超载** | ✅ | D13: 三性评估函数不一致（方法 vs 包级）；建议拆 4 文件 |
| `domains/workflows/patent/reexamination.go` | 809 | **6 职责超载** | ✅ | D14: 口审 4 函数（211行）仅 1 处使用；建议拆 2 文件 |
| `domains/workflows/patent/invalidation.go` | 796 | **4 职责超载** | ✅ | D14: YAML 闭包增强模式仅 1 处使用；建议拆 3 文件 |
| `domains/workflows/patent/oa_response.go` | 746 | **5 职责超载** | ✅ | D14: OA 兜底 prompt 与 novelty.go 重复；建议拆 3 文件 |
| `domains/enablement/nodes.go` | 779 | **4 职责超载** | ✅ | D13: Schema 字面字符串 vs jsType 常量不一致；建议拆 3 文件 |
| `domains/case_index.go` | 756 | **10 职责超载** | ✅ | D12: FTS5 模式与 knowledge/ 类似但领域不同；建议拆 3 文件 |
| `domains/case_extension.go` | 720 | **6 职责超载** | ✅ | **D14: `FileContentReader` 接口仅 1 使用方，用 `os.ReadFile` 即可**；建议拆 3 文件 |
| **`disclosure/novelty.go`** | 722 | **8 职责超载** | **🔴 L157 goroutine 不检查 ctx.Done()** | D13: `ExtractJSON` vs `ExtractJSONSimple` 混用；D14: 2 个 sync.Once singleton |
| `domains/claimdrafting/builder.go` | 667 | **✅ 唯一通过** | N/A | **14 个中最符合 §2.4 的文件——单一职责、组织良好** |
| `domains/infringement/nodes.go` | 644 | **2 职责超载** | ✅ | D14: 深层嵌套 Schema map；建议拆 2 文件 |
| `domains/workflows/patent/reasoning_patterns.go` | 634 | **数据超载** | N/A | D14: 18 模式中 12 个无频次数据（可能未完成）；建议拆 4 文件 |
| **`knowledge/extension.go`** | 690 | **8 职责超载** | **🔴 L438 goroutine 不检查 ctx.Done()** | **D9: `KnowledgeExtConfig` 无 Validate()**；建议拆 3 文件 |

> 完整拆分建议见 `audit/round2-domain-files.md`（待从 Agent 3 输出提取）

---

## 三、AI 高频违规 6 类专项扫描结果

### D11 — 幻觉 API 检测 🟢 通过

- 对 35 个 T1 文件的所有 import 进行了 `go.mod` 对照
- agentcore/ 无任何第三方导入（纯 stdlib + 内部包）—— 最小依赖原则执行到位
- 所有第三方导入均可追溯到 go.mod require
- **0 处幻觉 API 发现**

### D12 — 重复造轮子 🟡 2 项

| 发现 | 文件 | 说明 |
|------|------|------|
| LifecycleHook → Observer 迁移仅 ~44% | `agentcore/lifecycle.go` | Hook 引用 108 次 vs Observer 86 次，迁移进度缓慢 |
| `Severity` 类型重复定义 | `domains/rules/` + `domains/workflows/patent/` | 07-27 P1 已知项，仍未修复 |
| `appendLifecycle` 薄包装 | `domains/router.go:166` | 1:1 包装 `agentcore.AppendLifecycle`，可移除 |
| fuzzy 双重实现 | `fuzzy/fuzzy.go` vs `tui/core/fuzzy_match.go` | 两个实现功能重叠，可能可合并 |

### D13 — 风格漂移 🟡 4 类

| 模式 | 涉及文件 | 严重度 |
|------|---------|--------|
| **中英错误混用** | `cmd/mady/tui_session.go`（7 处中文 error）、`agentcore/errors.go`（中英混合） | P1 |
| **`%w` vs `%v` 混用** | `knowledge/extension.go`（%v=5, %w=0）、`tools/bash.go`（%w=5, %v=3 混用） | P1 |
| **注释中英混合** | `tools/tools.go`、`mcp/client.go` | P2 |
| **`mu` vs `xxxMu` 命名** | ~10 文件：`event.go`、`browser_session.go`、`browser_supervisor.go`、`compaction.go`、`store.go`、`discovery.go`、`client.go` | P2 |

### D14 — 过度工程化 🟡 2 项

| 发现 | 文件 | 说明 |
|------|------|------|
| **4 个 Operations 接口仅 1 实现** | `tools/bash.go`, `tools/git.go`, `tools/glob.go`, `tools/web_search.go` | BashOperations/GitOperations/GlobOperations/WebSearchOperations — 从无 mock 使用 |
| 4 个冗余 Set/Get 方法 | `server/server.go:183-205` | 相同模式可泛型统一 |

### D15 — Context 传播 🔴 3 项高风险

| 严重度 | 文件 | 问题 |
|--------|------|------|
| **P0** | `domains/project.go` | 6 个 I/O 函数（Register/Delete/Touch/SaveMeta/LoadMeta/RefreshStatus）不接受 ctx |
| **P0** | `tools/bash.go:30` | `Exec` 不接受 ctx，用 `exec.Command` 而非 `CommandContext` |
| **P0** | `knowledge/sqlite/store.go` | 系统性违规：18 处 `context.Background()`，所有公有方法无 ctx 参数 |
| P1 | `tools/` 子模块 | 19 处 noctx（golangci-lint 第 1 轮发现） |

### D16 — 测试覆盖 🟡 系统性问题

| 指标 | 数据 |
|------|------|
| T1 文件无测试 | 6/14 核心文件无对应 `_test.go`：`agent_run.go`、`patent.go`、`config.go`、`case_extension.go`、`novelty.go`、`report.go` |
| tools 无测试文件 | **37 个**工具源文件无测试，Browser 模块 12+ 文件全无 |
| `domains/checker/` | 0% 测试覆盖（4 源文件，0 测试） |

---

## 四、逐维度抽样统计（D6/D9 全覆盖）

### D6 错误信息格式

| 检查范围 | 发现 |
|---------|------|
| T1 35 个文件全量 + 前台扫描 | 大部分合规 |
| 违规 | `tools/browser_session.go` 5 处大写开头；`tools/netutil.go` 3 处 "BLOCKED" 大写；`tools/web_fetch.go` 3 处 "BLOCKED" 大写；~24 处总计 |
| 裁决 | "BLOCKED"/"DENIED" 是安全类错误，建议保留大写 → 实际需要修改的约 **12 处** |

### D9 Config Validate 模式

| 检查范围 | 数据 |
|---------|------|
| 项目全局 Config struct 数 | **116 个** |
| 有 `Validate()` 方法的 | **<5 个**（仅 `agentcore/config.go`、`domains/domainconfig/` 等少数） |
| T1 文件中缺失 | **9 个**：`levels.Config`、`CitationGateConfig`、`ApprovalConfig`、`BashToolConfig`、`VisionToolConfig`、`WorkingDirSandbox`、`BrowserSessionConfig`、`BrowserSupervisorConfig`、`CDPConfig` |
| 结论 | **>96% 缺少 → 系统性 P1 问题** |

---

## 五、高风险发现汇总（P0 + P1）

| 优先级 | 文件:行 | 问题 | 维度 |
|--------|---------|------|------|
| **P0** | `knowledge/sqlite/store.go` 全局 | 18 处 `context.Background()` 替代 ctx，查询无法取消 | D15 |
| **P0** | `domains/project.go:122,171,184,246,265,195` | 6 个 I/O 函数无 ctx 参数 | D15 |
| **P0** | `tools/bash.go:30` | `Exec` 不用 `CommandContext`，进程不响应取消 | D15 |
| **P0** | `domains/patent.go:256` | `_` 忽略 `NewInfringementTool` error，nil 解引用风险 | D1/S4 |
| P1 | 9 个文件 | Config struct 无 `Validate()` | D9 |
| P1 | `cmd/mady/tui_session.go` 7 处 | 中英 error 混用 | D13 |
| P1 | `agentcore/errors.go` | RetryableError/FatalError 中文 vs HandoffError/GuardrailError 英文 | D13 |
| P1 | `tools/browser_session.go:81,98,270,389,403` | 5 处 error 大写开头 + 句号 | D6 |
| P1 | `knowledge/extension.go` | `%v` 5 处，`%w` 0 处 | D13 |
| P1 | `tools/` 子模块 | 19 处 noctx + 65 处 errcheck（第 1 轮） | D1/D15 |
| **P0** | `disclosure/novelty.go:157-177` | goroutine 不检查 `ctx.Done()`，取消时继续运行 | D15 |
| **P0** | `knowledge/extension.go:438-491` | 3 个并发 goroutine 不检查 `ctx.Done()` | D15 |
| P1 | 37 个 tools 文件 | 无测试 | D16 |
| P1 | 14 个超大文件 | 全部 >600 行且职责超载，需拆分（rule_engine.go 1203 行最严重） | §2.4 |
| P1 | `domains/case_extension.go:21-36` | `FileContentReader` 接口单使用方，可删除 | D14 |
| P1 | `knowledge/extension.go:158` | `KnowledgeExtConfig` 无 `Validate()` | D9 |
| P1 | `disclosure/novelty.go` | `ExtractJSON` vs `ExtractJSONSimple` 风格漂移 | D13 |
| P1 | `domains/enablement/nodes.go` | Schema 字面字符串 vs `jsTypeObject` 常量不一致 | D13 |

---

## 六、与第 1 轮发现的交叉验证

| 第 1 轮发现 | 第 2 轮深度审阅结果 |
|------------|-------------------|
| tools 162 lint issues | 确认：errcheck(65) + gosec(51) + noctx(19) 集中在 browser_*.go 和 web_*.go |
| D9 Config Validate 缺失 | 确认：**116 个 Config，>96% 无 Validate()** |
| D15 Context 传播 21 处 | 升级：发现 3 个系统性违规（project.go/bash.go/store.go），影响远大于数量 |
| D6 错误大写 ~24 处 | 确认：browser_session.go 为主要违规源，"BLOCKED" 系列可保留 |
| D13 风格漂移 | 确认：4 类漂移模式（中英混用、%w/%v、注释、mu 命名） |
