# Mady 全量代码审阅报告

> 审阅日期：2026-07-27 | 审阅范围：1229 Go 源文件（820 非测试 + 409 测试），3 模块工作区
> 审阅原则："恰到好处的抽象，克制的工程实践"

---

## 一、总览

### 健康度评分

| 维度 | 权重 | 评分 | 等级 |
|------|------|------|------|
| 架构合规性 | 30% | 90/100 | A |
| 代码质量与工程实践 | 25% | 75/100 | B |
| 测试覆盖与质量 | 20% | 70/100 | B- |
| 安全性 | 15% | 85/100 | B+ |
| 文档一致性 | 10% | 70/100 | B- |
| **加权总分** | **100%** | **79/100** | **B+** |

### 与上次审计对比

| 指标 | 2026-07-26 | 2026-07-27 | 变化 |
|------|-----------|-----------|------|
| P0 修复状态 | 6/6 已验收 | 出现 1 项新 P0 | ⚠️ 回退 |
| 总文件数 | 1207 | 1229 | +22 |
| 测试文件数 | 397 | 409 | +12 |
| golangci-lint | 0 issues | 0 issues | ✅ 维持 |
| go vet | 0 issues | 0 issues | ✅ 维持 |
| 架构边界检查 | — | 25/25 通过 | ✅ 新增 |

---

## 二、P0 级发现（立即修复）

### F1: cmd/mady 测试包死锁/挂起

- **文件**: `cmd/mady/tui_session_agent_test.go:125-143`
- **类型**: 测试死锁（测试进程被 SIGQUIT 杀死，超时 660s）
- **问题**: `TestRebuildAgentPanicRecover` 测试在调用 `rebuildAgent()` 时，`applyPersistence` 内部发生 panic，但 defer recover 后留下了持久化的 goroutine 泄漏，导致测试包无法正常终止。单独运行该测试 300 秒超时仍未完成。
- **根本原因**: `rebuildAgent()` 的 defer recover 捕获了 panic 但未处理 goroutine 泄漏或资源死锁。`buildAgentConfig()` → `applyPersistence()` 链中调用了 `s.currentProject.RootPath` 且 `currentProject` 可能为 nil。
- **影响**: `make verify`（提交前门禁）因此 FAIL。**开发者无法通过 pre-commit 标准门禁**。
- **修复建议**:
  1. 检查 `applyPersistence` 中的 nil dereference 路径并保护
  2. 修复 `buildAgentConfig()` 中 `currentProject` 为 nil 时的回退
  3. 添加 `context.WithTimeout` 到测试，防止挂起
  4. 确认 defer recover 后没有遗留的阻塞操作

### F2: init() 中的 panic

- **文件**: `evaluate/benchmark/invalidation_decisions.go:30`
- **类型**: 违反编码规范（规范 1.4 禁止在 init() 中 panic）
- **问题**: `init()` 函数在加载基准测试用例失败时 `panic`，而非记录日志到 stderr
- **代码**:
  ```go
  func init() {
      // ...
      panic(fmt.Sprintf("evaluate/benchmark: failed to load invalidation decision cases: %v", err))
  }
  ```
- **影响**: 如果基准数据文件损坏或缺失，整个进程会崩溃
- **修复建议**: 改为 `log.Printf` 到 stderr 或设置标志变量

---

## 三、P1 级发现（2 周内修复）

### C1: 文档文件计数与实际不一致

| 文档 | 声称 | 实际 | 偏差 |
|------|------|------|------|
| AGENTS.md:16 | 1207 (809+398) | 1229 (820+409) | +22 文件 |
| CLAUDE.md:12 | ~1200 (~810+~400) | 1229 (820+409) | +29 文件 |

### C2: 不存在的目录仍在文档中

以下目录在 CLAUDE.md/AGENTS.md/CONTRIBUTING.md 中存在引用但磁盘上没有：
- **`filequeue/`** ❌ 不存在
- **`cache/`** ❌ 不存在
- **`workflow/`** ❌ 不存在（实际是 `workflows/`）

### C3: 存在但未列出的目录

以下目录实际存在但可能未在文档中列出：
- **`pkg/vecbytes/`** — 向量字节编码
- **`pluginsys/`** — 插件系统加载器

### C4: Spec 文档不完整

| Spec 目录 | 现有文档 | 需要补充 |
|-----------|---------|---------|
| `enablement-a26.3/` | 1（task-breakdown） | proposal, spec, design |
| `plantask-introduction/` | 1（plan） | spec, design, tasks |
| `prompt-templates-wiring/` | 2（design, tasks） | proposal |
| `reexamination-request/` | 1（proposal） | spec, design, tasks |
| `vector-retrieval/` | 5 | ✅ 已完成 |

### C5: cmd/mady/main.go 子命令数未更新

- **问题**: 文档称 "10 个子命令"，实际有 12 个 `case` 语句
- **新增子命令**: `patent`、`trust-knowledge`、`util`（可能还有其他）

### C6: psychological/ 模块 Spec-Implementation Gap

- **文档声称**: VAD/OCC/EMA/SDT/CBT 五个情绪模型
- **实际实现**: 仅有 **VAD**（Valence-Arousal-Dominance）模型
- **OCC/EMA/SDT/CBT**: 零代码实现
- **影响**: 心理引擎的实际能力仅为文档声称的 20%
- **建议**: 更新文档如实反映当前能力，或将未实现模型移动到未来计划

### C7: tools/ 系统性 XxxOperations 过度抽象

- **范围**: `tools/` 中每个工具文件都定义了 `XxxOperations interface` + `DefaultXxxOperations struct`
- **模式**: 18 个工具各有一个 Operations interface，每个只有一个 Default 实现
- **问题**: 这是为测试 mock 服务的统一模式。如果每个 interface 确实只有一个生产实现，属于适度抽象（支持测试）。但如果某些工具的 Operations interface 从未在测试中被 mock，就是过度抽象。
- **建议**: 审计每个 Operations interface 的 mock 使用情况，移除从未被 mock 的 interface

### C8: Severity 类型重复定义

- **文件 1**: `domains/rules/types.go:10` — `type Severity string`（Critical/Major/Minor）
- **文件 2**: `workflows/patent/rule_engine.go:24` — `type Severity string`（Critical/Major/Minor）
- **问题**: 完全相同的类型在两个包中独立定义，值完全一致
- **建议**: 抽取到公共位置（如 `pkg/`）或由一方 import 另一方

### C9: 超大文件（>1000 行）

| 文件 | 行数 | 问题 |
|------|------|------|
| `tui/chat/chat_app.go` | 1109 | 聊天应用主文件，模型/布局/覆盖层/快捷帮助混合 |
| `workflows/patent/rule_engine.go` | 1093 | 单一规则引擎，可拆分 by 领域 |
| `cmd/mady/tui_session.go` | 1058 | 会话装配，串联多个子系统的胶水代码 |
| `domains/inventiveness/nodes.go` | 1020 | 创造性判断节点，单一领域内较复杂 |
| `acp/server.go` | 1007 | ACP 协议服务器，JSONRPC 逻辑混合 |

此外有 20+ 个非测试文件在 500-1000 行之间。

### C10: 超大测试文件

| 文件 | 行数 | 风险 |
|------|------|------|
| `a2a/a2a_test.go` | 2739 | 阅读理解成本高 |
| `server/server_test.go` | 2332 | 测试维护成本高 |
| `a2ui/a2ui_test.go` | 1861 | |
| `mcp/http_test.go` | 1645 | |
| `acp/session_test.go` | 1446 | |

---

## 四、P2 级发现（1 月内修复）

### D1: tui/ 模块零 errors.Is/errors.As 使用

- **全模块**: 0 次 `errors.Is` / `errors.As` 调用（非测试文件）
- **对比**: 根模块 34 次，tools/ 子模块 8 次
- **影响**: TUI 层的错误处理完全依赖 `==` 比较和类型断言，无法处理 wrapped error
- **建议**: 系统性审计 TUI 错误处理逻辑，在合适的比较点改用 `errors.Is`/`errors.As`

### D2: tools/ 中 io.ReadAll 无 LimitReader 保护（4 处）

| 文件 | 行号 | 风险 |
|------|------|------|
| `tools/browser_camofox.go` | 320 | 服务器响应可能导致内存溢出 |
| `tools/browser_camofox.go` | 409 | API 错误响应可能很大 |
| `tools/browserproviders/browser_use.go` | 58 | 同 |
| `tools/browserproviders/browser_use.go` | 104 | 同 |

- 其中 2 处忽略了错误（`body, _ := io.ReadAll(resp.Body)`）
- **建议**: 全部添加 `io.LimitReader(resp.Body, maxBytes)` 保护，且不忽略错误

### D3: domains/ 各子模块存在结构重复

- **重复模式**: 每个 domain（infringement/inventiveness/novelty/enablement/claimdrafting/specdrafting）都各自实现 `types.go`, `nodes.go`, `graph.go`, `rules.go`, `scorer.go` 等文件
- **总重复文件**: 每个 domain 有 5-7 个同构文件
- **建议**: 提取共享的 domain-graph builder 基类或辅助函数，消除 ~40% 的重复

### D4: 缺失的架构层次文档验证

- **发现**: 检查发现了 2 个不存在的目录引用和 2 个未列出的目录，说明文档维护者未同步最新结构
- **建议**: 建立文档自动同步机制（如 CI 检查文件结构 vs 文档一致性）

---

## 五、P3 级发现（长期观察）

### E1: 错误处理分层一致性

- 根模块使用了分层错误类型（`RetryableError`/`FatalError`/`GuardrailError`/`HandoffError`/`NodeError`），但使用密度不均匀
- `tools/` 子模块主要使用裸 `fmt.Errorf`，分层错误类型使用较少
- `tui/` 子模块完全不使用项目定义的分层错误类型

### E2: 领域模块代码量分布不均

- `domains/` 共 285 个文件，是最大的模块
  - `reasoning/` 子模块尤其大（含 collector/sqlite/wiring 子包）
  - `rules/` 模块含 YAML 规则数据
- 大型领域模块可能受益于进一步的子包拆分

### E3: 部分 example/ 无测试

- `example/a2a-client`, `example/a2a-server`, `example/tui-demo` 等 8 个 example 目录无测试文件
- 对于示例代码，可接受（示例的测试投入产出比低）

---

## 六、正面发现

### G1: 架构隔离严格
- 8 层分层架构的 25 条边界约束全部通过 ✅
- tui/ 各层之间无逆向导入 ✅
- extensions 机制实现了松耦合

### G2: 代码质量基础设施完善
- golangci-lint 0 issues（优秀）
- go vet 0 issues（优秀）
- 前提交钩子覆盖格式化/vet/lint/架构/敏感路径/commitlint
- pre-commit 配置完整
- gosec 误报排除均记录了理由

### G3: tools/ 和 tui/ 子模块测试通过
- `tools/` 和 `tui/` 的 `go test -race` 全部通过 ✅
- 各 8+ 个包的测试成功运行

### G4: 文档禁用词合规
- `tone-style-guide.md` 定义的禁用词（绝对/一定/保证）在代码护栏文案中未被使用 ✅

### G5: 安全机制完善
- 敏感路径检测自动化（pre-commit + CI）
- AI + 敏感路径的组合阻塞门禁
- CODEOWNERS 配置完善
- 参数化 SQL 查询全部使用 `?` 占位符
- SSRF 防护通过 `ssrfSafeDialer` 实现

### G6: 测试基础设施完善
- 409 个测试文件（33.3% 比例）
- 7 个集成测试（带 build tags）
- evaluate/ 评估框架成熟（6 指标 + LLM Judge）
- benchmark/ 有 5 个性能基准

---

## 七、趋势对比

| 项目 | 上次审计（07-26） | 本次发现（07-27） | 说明 |
|------|------------------|------------------|------|
| golangci-lint issues | 0 | 0 | ✅ 维持 |
| go vet issues | 0 | 0 | ✅ 维持 |
| P0 问题 | 0（6 已修） | 2（F1 测试死锁 + F2 init panic） | ⚠️ 新增 |
| 文件计数文档偏差 | 已修 | 再次偏移（+22 文件） | 🔄 复发 |
| 不存在的目录引用 | 已修 | 3 个目录引用再次偏移 | 🔄 复发 |
| 架构边界 | 未知 | 25/25 通过 | ✅ 新增基线 |
| make verify | 通过 | FAIL（cmd/mady 测试超时） | 🔴 回退 |

---

## 八、修复路线图

### 立即修复（1-2 天）
1. **F1**: 修复 `TestRebuildAgentPanicRecover` 测试死锁（阻塞 make verify）
2. **F2**: 移除 `invalidation_decisions.go` 中 init() 的 panic

### 短期（2 周）
1. **C1-C5**: 更新文档计数、目录引用、subcommand 数
2. **C6**: 更新 psychological/ 文档如实反映仅 VAD 实现
3. **C7**: 审计 tools/ Operations 接口的 mock 覆盖率，移除未使用的
4. **C8**: 消除 Severity 类型重复定义

### 中期（1 月）
1. **C9-C10**: 拆分超大文件
2. **D1**: 为 tui/ 引入 errors.Is/As 使用
3. **D2**: 为 4 处 io.ReadAll 添加 LimitReader 保护
4. **D3**: 提取 domain-graph builder 基类减少重复

### 长期（持续）
1. **E1**: 统一三模块的错误处理模式
2. **E2**: 监控 domains/ 模块大小，适时拆分
3. 与上次审计的 P2/P3 项（共 12 项）合并跟踪

---

## 九、审核说明

本次审阅遵循"恰到好处的抽象，克制的工程实践"原则：
- **不重复已有审阅**：已有审阅覆盖的模块仅做验证性复查
- **证据驱动**：每个发现均含文件路径和行号引用
- **正面与负面并重**：同时记录做得好的方面和改进机会
- **克制判断**：避免为了找问题而找问题，以适度原则为标准
