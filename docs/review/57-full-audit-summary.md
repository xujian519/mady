# Mady 全面审查 — 综合汇总报告

> 审查日期：2026-07-31 | 基线：`origin/main` (adaa6e5)
> 审查哲学：克制、中庸、简约
> 输出范围：8 维度 × 50+ 检查项，覆盖 1229 Go 源文件 + 327 前端文件

---

## 一、综合评分

| 维度 | 评分 | 上次评分 | 变化 |
|------|:----:|:--------:|:----:|
| 架构合规性 | **B** | A (90) | ↓ 2 级 |
| 代码质量 | **B** | B (75) | → |
| 安全与数据隐私 | **B-** | B+ (85) | ↓ 1 级 |
| 领域逻辑一致性 | **B-** | — | （新增） |
| 测试质量 | **B-** | B- (70) | → |
| 文档一致性 | **C** | B- (70) | ↓ 2 级 |
| 结构组织 | **B** | — | （新增） |
| 简约化 | **B** | — | （新增） |
| **加权综合** | **B- (73/100)** | **B+ (79/100)** | **↓ 6 分** |

> 评分下降主要是因为：① 首次纳入全量审查的前置维度评分更低（架构/安全/领域/文档是全量审查而非抽样）；② 上次评分时部分 P0 问题（PII 脱敏、AI_CHANGELOG 合规、Domain 导入基础设施）尚未暴露。

---

## 二、问题分级汇总

### P0（阻塞级）— 4 项

| # | 问题 | 维度 | 位置 | 影响 |
|---|------|------|------|------|
| P0-1 | **25 条 LLM 出站路径零 PII 脱敏** | 安全 | provider/, agentcore/ | 真实案件数据直接发送至 LLM API |
| P0-2 | **Domain 层大量导入基础设施具体实现** | 架构 | domains/ 各子包 | 违反分层架构核心原则，15+ 处 |
| P0-3 | **`resumeIfInterrupted` ABBA 潜在死锁** | 测试 | agentcore/agent_run.go | 阻塞 `make verify` 门禁 |
| P0-4 | **AI_CHANGELOG 0% 格式合规** | 文档 | docs/decisions/AI_CHANGELOG.md | 223 条记录全部不符合 CONTRIBUTING 规范 |

### P1（重要级）— 9 项

| # | 问题 | 维度 | 位置 |
|---|------|------|------|
| P1-1 | evidence YAML 规则从未加载进引擎 | 领域 | domains/evidence/, domains/rules/data/ |
| P1-2 | enablement 混用 In re Wands 美国判例 | 领域 | domains/enablement/node_enablement.go:100 |
| P1-3 | memory `SearchAllLayers` 跨项目检索 | 安全 | memory/sqlite_store.go |
| P1-4 | MCP TOCTOU 时序窗口 | 安全 | mcp/config_trust.go |
| P1-5 | `_ = recover()` 静默吞 panic | 代码质量 | tui/tui_loop.go:27 |
| P1-6 | desktop 等 25 处大写开头错误信息 | 代码质量 | desktop/app_files.go 等 |
| P1-7 | 文件计数漂移 +14%（1229→1405） | 文档 | AGENTS.md, CLAUDE.md, README |
| P1-8 | README 6 处错误（目录不存在/死引用） | 文档 | README.md |
| P1-9 | psychological 仅实现 30-35% 声称能力 | 领域 | psychological/ |

### P2（改进建议级）— 15 项

| # | 问题 | 维度 | 优先级内排序 |
|---|------|------|-------------|
| P2-1 | `domains/approval.go` 完整重复（~120 行） | 简约 | 可立即删除 |
| P2-2 | `mcp/client.go` ↔ `mcp/http.go` 3 方法拷贝（~40 行） | 简约 | 可立即删除 |
| P2-3 | `tools/browser_supervisor.go` 接口重复（~15 行） | 简约 | 可立即删除 |
| P2-4 | 三入口权限策略复制（~30 行） | 简约 | 可立即删除 |
| P2-5 | `cmd/mady` 39 个源文件入口包膨胀 | 结构 | 建议提取框架层 |
| P2-6 | 65 处 `//nolint:gocognit` 缺乏原因注释 | 代码质量 | 逐步补充 |
| P2-7 | `cmd/mady` 测试覆盖率仅 18.1% | 测试 | 入口层需补充 |
| P2-8 | 46 处 `time.Sleep` 中 ~60% 可重构 | 测试 | 用 context/channel 替代 |
| P2-9 | guardrails/guardian 测试 1/6 覆盖 | 结构 | 安全关键模块 |
| P2-10 | agentcore/permission 测试 1/6 覆盖 | 结构 | 权限关键模块 |
| P2-11 | ADR-0003 Proposed 但零实现代码 | 文档 | 清除或实现 |
| P2-12 | root ↔ tools 交叉双向依赖影响外部导入 | 架构 | 需修复模块边界 |
| P2-13 | 15+ 全局单例（domainFactoryMap 限制） | 结构 | 扩展回调签名 |
| P2-14 | 前端组件目录扁平 | 结构 | 适度分层 |
| P2-15 | example/cli-chat 复杂度 157（示例不应最高） | 简约 | 拆分 main |

---

## 三、修复路线图

### 第 1 阶段：P0 紧急修复（立即，不可跳过）

| 任务 | 工作量 | 关联问题 |
|------|--------|---------|
| 3.1 修复 ABBA 死锁：`resumeIfInterrupted` 将 `getCurrentAgent()` 移到 goroutine 内部 | **~5 行改动** | P0-3 |
| 3.2 为 25 条 LLM 出站路径增加 PII 脱敏层 | 3-5 天架构设计 + 实现 | P0-1 |
| 3.3 消除 Domain → Infrastructure 直接 import（先处理最高频的 5 个） | 2-3 天 | P0-2 |
| 3.4 新增 AI_CHANGELOG pre-commit 格式门禁 | 半天 | P0-4 |

### 第 2 阶段：P1 重要修复（1-2 周内）

| 任务 | 工作量 |
|------|--------|
| 2.1 激活 evidence YAML 规则加载（`bootstrap/setup.go`） | 半天 |
| 2.2 删除 enablement 中 In re Wands 引用，替换为审查指南标准 | 1 行+PR |
| 2.3 memory `SearchAllLayers` 增加 Scope 校验 | 1 天 |
| 2.4 MCP TOCTOU 增加内存中二次哈希校验 | 1 天 |
| 2.5 `_ = recover()` → 带日志的恢复处理 | 1 行 |
| 2.6 desktop 等 25 处大写错误信息统一小写 | 批量替换 |
| 2.7 修正 AGENTS.md/CLAUDE.md/README 中的文件计数、目录引用 | 半天 |
| 2.8 psychological 补充 VAD 模型或标记已知限制 | 评估后决定 |

### 第 3 阶段：P2 改进（持续优化）

| 任务 | 工作量 |
|------|--------|
| 3.1 删除 4 处零风险重复代码（~205 行） | 半天 |
| 3.2 `cmd/mady` 框架装配逻辑提取到 `internal/` | 2-3 天 |
| 3.3 65 处 `//nolint:gocognit` 补充原因注释 | 逐步完成 |
| 3.4 补充 guardrails/guardian + agentcore/permission 测试 | 2 天 |
| 3.5 46 处 `time.Sleep` 替换（优先安全关键路径） | 按优先级 |
| 3.6 ADR-0003 决策：清除或启动实现 | 讨论决定 |
| 3.7 新测试统一采用 `TestXxx_WhenYyy_ExpectZzz` 命名 | 长期规范 |

---

## 四、总体评估

### 优势总结
- **构建基线极稳定**：`golangci-lint` (29+ linters) 零 issue、`go vet` 零警告、`go build` 全通过
- **安全设计根基扎实**：fail-closed、default-deny、inode pinning、常量时间比较等最佳实践到位
- **接口驱动架构清晰**：~150 接口，层间解耦彻底，工具层的 Operations 接口体系完善
- **技术债务控制优秀**：零 TODO/FIXME/HACK 遗留标记，12 个 init() 全有理有据
- **测试基础设施完善**：409 测试文件（33%），`-race` 竞态检测，Golden Set 基准测试

### 主要风险
1. **数据隐私缺口（P0-1）**：LLM 出站零脱敏是生产部署的真正阻碍
2. **架构合规性退化（P0-2）**：Domain→Infrastructure 直接 import 影响长期可维护性
3. **文档漂移严重（P0-4）**：AI_CHANGELOG 格式规范与实践完全脱节
4. **局部测试薄弱（P2-9/10）**：安全关键模块测试覆盖不足

### 与上次审查对比（2026-07-27：B+ 79/100 → 本次 B- 73/100）

评分下降的主要原因：
1. **范围扩展**：上次审查抽样为主，本次是全量 8 维度的系统化审查
2. **标准提升**：本次新增了"简约化"和"结构组织"维度的定量门槛
3. **新暴露问题**：PII 脱敏缺失、AI_CHANGELOG 零合规、Domain 基础设施 import 等 P0 问题在上次抽样中未被覆盖

综合来看，项目代码质量仍然维持在良好水平，但需要优先解决第 1 阶段的 4 个 P0 阻塞项，才能解锁 `make verify` 门禁和真实数据部署。

---

## 五、审查产出清单

| 文件 | 内容 | 评分 |
|------|------|:----:|
| `docs/review/50-full-audit-arch-summary.md` | 架构合规性审查 | **B** |
| `docs/review/51-full-audit-code-quality.md` | 代码质量审查 | **B** |
| `docs/review/52-full-audit-domain-logic.md` | 领域逻辑审查 | **6.6/10** |
| `docs/review/53-full-audit-security.md` | 安全审计 | **B-** |
| `docs/review/54-full-audit-test-coverage.md` | 测试质量审查 | **B-** |
| `docs/review/55-full-audit-docs-consistency.md` | 文档一致性审查 | **C (55/100)** |
| `docs/review/56-full-audit-simplicity.md` | 简约化审查 | **B** |
| `docs/review/56a-structural-organization.md` | 结构组织审查 | **B** |
| `docs/review/57-full-audit-summary.md` | **本文件（综合汇总）** | **B- (73/100)** |

---

*审查依据：GO-DEVELOPMENT-STANDARDS.md · AGENTS.md · SECURITY.md · tone-style-guide.md · project philosophy（克制/中庸/简约）*
