# Mady 项目全量 Lint 审查 — 综合报告

**审查日期**: 2026-07-30
**代码基线**: ~4,286 Go 文件，~291K 行代码，4 模块（root/tools/tui/desktop）
**工具**: golangci-lint v2.12.2, go vet, go-arch-lint, 专项模式扫描
**审查范围**: 7 阶段，含配置审查、全量扫描、linter 评估、架构验证、规范复审、nolint 审计

---

## 执行摘要

本次审查完成了对 Mady 项目全部 4 个模块的**首次差异化配置 lint 扫描**（tui/desktop 模块首次拥有独立 `.golangci.yml`，tools 补齐了 5 个缺失 linter）。**无 P0 级问题**，架构边界 100% 合规，`go vet` 4 模块全绿。

**关键数据**：
- `golangci-lint`: root 0 / tools 117（新增）/ tui 29（新增）/ desktop 0
- `go vet`: 4 模块全部 PASS
- `go-arch-lint`: 43 组件全部 PASS，0 架构违规
- Config Validate 覆盖率: 6%（116/7）

---

## 模块健康度总表

| 模块 | Go 文件 | lint 问题 | go vet | 独立配置 | CI 覆盖 |
|------|---------|----------|--------|---------|---------|
| **root** | 2,168 | 0 ✅ | ✅ | `.golangci.yml` | ✅ |
| **tools** | 73 | 117 ⚠️ | ✅ | `tools/.golangci.yml`（已补齐） | ✅ |
| **tui** | 113 | 29 ⚠️ | ✅ | `tui/.golangci.yml` ✨新建 | ❌（待补） |
| **desktop** | 5 | 0 ✅ | ✅ | `desktop/.golangci.yml` ✨新建 | ❌（待补） |

> tools 和 tui 的问题数量高是因为首次以完整配置运行，并非新引入的回归。
> 07-27 审计时 tools 仅有 10 个 linter，tui 使用默认配置，这些问题是长期存在但未被检测的积压。

---

## 优先级排序的问题清单

### P0 — Critical（0 项）
本次未发现安全漏洞、goroutine 泄漏、nil 解引用或数据损坏风险。

### P1 — Major（7 项）

| ID | 模块 | 文件:行号 | 问题 | 建议修复 |
|----|------|----------|------|---------|
| S01 | tools | 多处 `browser_camofox.go` `browserproviders/` `web_search*.go` | noctx: 19 处 HTTP 请求未携带 context，不可取消 | 替换 `http.NewRequest` → `http.NewRequestWithContext` |
| S02 | tools | 多处 `desktop/` `browser_*.go` `grep.go` 等 | gosec G104: 30+ 处错误未检查（Close/Kill/Wait/defer） | 添加 `//nolint:gosec` 或处理错误 |
| S08 | 全局 | — | Config Validate 覆盖率 6%（116/7），109 个 Config struct 无 Validate() | 优先级最高的技术债务，逐模块补齐 |
| SC01 | tools | `browserproviders/`, `web_*.go` | Context 传播（AI 模式 #5）— 与 S01 重叠 | 同上 noctx 修复 |
| C02 | tools | `tools/.golangci.yml` | 缺失 5 个 linter（**已在本轮审查中补齐**） | ✅ 已修复 |
| S03 | tools | `bash.go`, `process.go`, `path.go` 等 | gosec G204/G304: 子进程执行和路径操作缺少 nolint 注释 | 添加解释性 nolint |
| S09 | 全局 | desktop/app.go(1334), tui/chat/chat_app.go(1283) 等 | 10+ 文件超过 500 行，需拆分 | 逐步重构 |

### P2 — Minor（8 项）

| ID | 模块 | 问题 | 数量 |
|----|------|------|------|
| S04 | tui | revive redefines-builtin-id: min/max/new/cap 遮蔽内置函数 | 9 |
| S05 | tui | staticcheck ST1020/QF1012: 注释格式和简化 | 11 |
| S06 | tui | unused: hasSelection 函数未使用 | 1 |
| S07 | tools | errchkjson: json.Marshal of `any`（ego_lite_manager.go） | 2 |
| N01 | tools | 新暴露的 96 处 gosec 缺 nolint 注释 | ~40 处需补 |
| C05 | Makefile | `make lint` 未覆盖 desktop（**已在本轮审查中修复**） | ✅ 已修复 |
| C06 | CI | CI golangci-lint 未覆盖 tui/desktop | 需 CI 配置变更 |
| SC03 | 全局 | Config Validate 覆盖率趋势（持续追踪 P2） | — |

### P3 — Trivial / 技术债务（9 项）

| ID | 问题 |
|----|------|
| C01 | SA1019 全局排除建议缩小到具体文件 |
| C03/C04 | tui/desktop 新建配置（✅ 已修复） |
| C07 | precommit-golangci-lint.sh 未覆盖 tui/desktop |
| A01 | go-arch-lint advanced linter 未开启（vendor/deepScan） |
| A02 | vendor 文件未在组件定义中声明 |
| N02 | 建议启用 nolintlint 确保 nolint 格式规范 |
| SC04 | tools vs agentcore 错误处理风格差异（已知） |
| D6 | ~92 个 Errorf 大写开头的候选（多为合法缩写） |

---

## 本次审查中已修复的问题（配置层面）

| 变更 | 文件 | 说明 |
|------|------|------|
| ✅ tools 补齐 5 个 linter | `tools/.golangci.yml` | 新增 bodyclose/noctx/goconst/errchkjson/gosec |
| ✅ tui 独立配置创建 | `tui/.golangci.yml` | 11 个 linter，排除 bodyclose/noctx/gosec |
| ✅ desktop 独立配置创建 | `desktop/.golangci.yml` | 10 个 linter，排除 bodyclose/noctx/gosec |
| ✅ `make lint` 补齐 desktop | `Makefile` | `(cd desktop && golangci-lint run ./...)` |

---

## 健康度评分

沿用历史公式：`架构合规(30%) + 代码质量(25%) + 测试覆盖(20%) + 安全性(15%) + 文档一致性(10%)`

| 维度 | 权重 | 得分 | 加权得分 | 评分理由 |
|------|------|------|---------|---------|
| 架构合规 | 30% | 95 | 28.5 | go-arch-lint 0 违规，25/25 边界检查通过 |
| 代码质量 | 25% | 75 | 18.8 | 146 个新暴露问题，但均为新配置发现，非回归 |
| 测试覆盖 | 20% | 70 | 14.0 | 整体 33%，mcp(17.8%)/a2ui(12.5%) 偏低 |
| 安全性 | 15% | 85 | 12.8 | root 0 gosec，tools 96 项中 70% 为设计选择 |
| 文档一致性 | 10% | 72 | 7.2 | Config Validate 6%，注释格式 11 项需修复 |
| **总分** | **100%** | | **80.1** | |

### 历史趋势

| 日期 | 评分 | 趋势 |
|------|------|------|
| 2026-07-27 | 79/100 (B+) | 基线 |
| 2026-07-30（审计结果） | 78/100 (B+) | 小幅下降（审计后发现新 P0 待修复） |
| **2026-07-30（本次审查）** | **80/100 (B+)** | **↑ 小幅回升** |

评分解读：80/100（B+）。项目维护了良好的 lint 基线（root 0 issues, go vet 0 issues, arch 0 violations），但 Config Validate 覆盖率（6%）和测试低覆盖模块（mcp/a2ui）是主要扣分项。本次新增的 tools 117 项和 tui 29 项并非质量退化，而是配置补齐后的积压清理，修复后将进一步提升代码质量。

---

## 5 项关键建议

### 1. 🥇 修复 tools noctx（19 项）— P1
将 `http.NewRequest`、`exec.Command`、`net.Listen` 替换为 `NewRequestWithContext`、`CommandContext`、`ListenConfig.Listen`。涉及：
`browser_camofox.go`、`browserproviders/browser_use.go`、`browserproviders/browserbase.go`、
`browserproviders/firecrawl.go`、`web_fetch.go`、`web_search*.go`、`execute_code_ptc.go`

### 2. 🥇 补齐 tools gosec nolint 注释（~40 处）— P1
为 tools 模块的设计性 gosec 违规添加标准格式的 nolint 注释：
```
//nolint:gosec // 路径来自 filepath.Walk 的可控源
```

### 3. 🥈 启用 errorlint — P2
在根模块和 tools `.golangci.yml` 中启用 `errorlint`，与 GO-DEVELOPMENT-STANDARDS.md S4.2/S4.4 对齐：
```yaml
linters:
  enable:
    - errorlint
```

### 4. 🥈 CI 补齐 tui/desktop lint — P2
在 `.github/workflows/ci.yml` 的 `lint` job 中新增 tui 和 desktop 的 golangci-lint 步骤。

### 5. 🥈 Config Validate 覆盖率提升 — 长期
优先为 mcp/、tools/、agentcore/ 中的核心 Config 类型添加 Validate() 方法。

---

## 审查产物清单

| 文件 | 内容 |
|------|------|
| `audit/artifacts/phase1-config-review.md` | 配置基线审查报告 |
| `audit/artifacts/phase2-lint-scan.md` | 全量自动化扫描报告 |
| `audit/artifacts/phase3-linter-evaluation.md` | 补充 Linter 评估与推荐 |
| `audit/artifacts/phase4-arch-boundary.md` | 架构边界验证报告 |
| `audit/artifacts/phase5-standards-compliance.md` | 代码规范合规复审报告 |
| `audit/artifacts/phase6-nolint-audit.md` | nolint 指令审计报告 |
| **`audit/artifacts/phase7-comprehensive-report.md`** | **本文件（综合报告）** |

### 配置变更文件

| 文件 | 变更类型 |
|------|---------|
| `tools/.golangci.yml` | 补齐 5 个 linter |
| `tui/.golangci.yml` | 新建配置 |
| `desktop/.golangci.yml` | 新建配置 |
| `Makefile` | `lint` target 新增 desktop 覆盖 |
