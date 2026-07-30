# 第一阶段：配置基线审查报告

**日期**: 2026-07-30
**审查者**: lint 审查流程

---

## 1.1 根模块 `.golangci.yml` 审查结果

**结论：总体良好，2 项待优化**

| 检查项 | 状态 | 说明 |
|--------|------|------|
| 15 个 linter 配置合理性 | ✅ OK | 当前配置覆盖了错误检查、代码质量、正确性、性能安全四类 |
| revive 禁用规则 | ✅ OK | `indent-error-flow` 和 `package-comments` 禁用理由合理（Go 风格争议 + 项目约定） |
| gosec 排除 | ✅ OK | G404(弱随机)/G115(整数溢出)/G402(TLS) 均属项目设计选择，排除合理 |
| exclusions.paths | ✅ OK | 覆盖 graphify-out/build/.grok/.git，排除 _test.go/_gen.go/.pb.go |
| **SA1019 排除范围过宽** | ⚠️ 建议优化 | 全局排除所有已弃用 API，应缩小到具体文件 |

**SA1019 优化建议**：
```yaml
staticcheck:
  checks:
    - all
    - "-SA1019"  # 改为针对特定文件排除而非全局
```
当前 4 处已知废弃 API：
- `agentcore/config.go` — Middleware (已弃用)
- `agentcore/lifecycle.go` — AgentRunObserver (已弃用)
- `domains/reasoning/types.go` — ExecutionPlan (已弃用)
- `domains/reasoning/fact_blackboard.go` — Plan (已弃用)

建议在每个使用处添加 `//nolint:staticcheck // SA1019: 保留向后兼容` 替换全局排除。

---

## 1.2 tools 子模块配置差距

**结论：缺 5 个 linter，需补齐**

| 缺失 linter | 理由 | 预期影响 |
|-------------|------|---------|
| `bodyclose` | tools 有 HTTP 客户端（web_fetch, browser_session） | 低误报 |
| `noctx` | 上次审计发现 19 处无 context | 低误报 |
| `goconst` | 安全相关重复字符串（"BLOCKED"/"DENIED" 等） | 中等 |
| `errchkjson` | tools 有 JSON 序列化操作 | 低误报 |
| `gosec` | tools 是安全敏感模块（bash/browser/path 沙箱） | 中等，需配置排除 |

**保留的 tools 特有规则**：15 条 errcheck 细粒度排除（browser/desktop 的 defer Close/Remove/Kill/Wait、os.Remove、filepath.WalkDir 等）必须保留。

---

## 1.3 tui/desktop 配置状态

| 模块 | Go 文件 | 当前 lint 方式 | 结论 |
|------|---------|---------------|------|
| **tui** | 113 源文件 | `make lint` 中 `(cd tui && golangci-lint run ./...)` — **无独立配置，使用默认 linter 设置** | ⚠️ 需创建独立配置 |
| **desktop** | 5 源文件 | **`make lint` 未覆盖 desktop 模块** | ⚠️ 需创建配置 + 加入 lint target |

**tui 配置建议**：
- 不需要 bodyclose/noctx（无 net/http 使用）
- 不需要 gosec（仅 1 处 UI label 的 nolint:gosec）
- 核心 linter：errcheck, govet, ineffassign, staticcheck, unused, misspell, revive, copyloopvar, durationcheck

**desktop 配置建议**：
- 不需要 bodyclose/noctx/gosec（无 HTTP，仅 5 个 Go 文件）
- 核心 linter: errcheck, govet, ineffassign, staticcheck, unused, revive

---

## 1.4 CI/预提交缺口

| 检查项 | 当前覆盖 | 缺口 |
|--------|---------|------|
| CI `lint` job | root + tools | ❌ 未覆盖 tui + desktop |
| CI `build-and-test` | root + tools + tui (含 go vet) | ✅ desktop 也有独立 job 含 vet |
| CI `check-arch` | 已安装并运行 go-arch-lint | ✅ |
| `make lint` target | root + tools + tui | ❌ 未覆盖 desktop |
| `precommit-golangci-lint.sh` | root + tools | ❌ 未覆盖 tui + desktop |
| `make verify` | lint + check-arch + build + test-race | ✅ 但 lint 自身缺 desktop |

---

## 发现摘要

| ID | 严重度 | 描述 | 文件 |
|----|--------|------|------|
| C01 | P2 | SA1019 全局排除应缩小到具体文件 | `.golangci.yml:96-97` |
| C02 | P1 | tools 缺失 5 个 linter（bodyclose/noctx/goconst/errchkjson/gosec） | `tools/.golangci.yml` |
| C03 | P2 | tui 无独立 .golangci.yml，运行使用默认配置 | 需新建 `tui/.golangci.yml` |
| C04 | P2 | desktop 无独立 .golangci.yml | 需新建 `desktop/.golangci.yml` |
| C05 | P2 | `make lint` 未覆盖 desktop 模块 | `Makefile:182-190` |
| C06 | P2 | CI golangci-lint 未覆盖 tui 和 desktop | `.github/workflows/ci.yml:28-48` |
| C07 | P3 | `precommit-golangci-lint.sh` 未覆盖 tui 和 desktop | `scripts/precommit-golangci-lint.sh` |
