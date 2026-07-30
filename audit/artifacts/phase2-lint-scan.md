# 第二阶段：全量自动化扫描报告

**日期**: 2026-07-30
**工具**: golangci-lint v2.12.2, go vet, 专项模式扫描

---

## 2.1 golangci-lint 四模块扫描结果

| 模块 | 结果 | 发现数 | 与 07-27 基线对比 |
|------|------|--------|-----------------|
| **root** | ✅ 0 issues | 0 | 维持 0 问题 |
| **tools** | ⚠️ 117 issues | 117 | 新增 117（因启用了 bodyclose/noctx/gosec/errchkjson） |
| **tui** | ⚠️ 29 issues | 29 | 新增 29（因新创建了独立 .golangci.yml 并启用 revive/staticcheck） |
| **desktop** | ✅ 0 issues | 0 | 首次扫描，干净通过 |

**趋势**: 配置补齐后暴露了之前未被检测的积压问题。这些是"新发现"而非"新引入"。

---

## 2.2 问题分类（tools 模块 117 项）

| Linter | 数量 | 严重度 | 说明 |
|--------|------|--------|------|
| **gosec** | 96 | P1-P2 | G104 错误未处理(30+), G204 子进程(20+), G304 路径注入(20+), G301 权限(3) |
| **noctx** | 19 | P1 | 未使用 WithContext 的 HTTP 请求和 exec.Command |
| **errchkjson** | 2 | P2 | json.Marshal 对 `any` 类型调用 |

**关键发现（与 CI 中的 nolint 排除对比）**：
- tools 模块的 gosec 问题中约 70% 是已知安全设计（subprocess, file operations），需要添加明确的 nolint 注释
- noctx 的 19 项是**需修复的真实缺陷** — HTTP 请求缺少可取消的 context
- gosec G104 中部分可能已在 CI 的 nolint 规则中覆盖但因配置迁移后失效

---

## 2.3 问题分类（tui 模块 29 项）

| Linter | 数量 | 严重度 | 说明 |
|--------|------|--------|------|
| **revive** | 17 | P2-P3 | 9 个 `redefines-builtin-id`（min/max/new/cap 遮蔽内置函数），1 个 `unexported-return` |
| **staticcheck** | 11 | P2 | 6 个 ST1020（注释格式），5 个 QF1012（WriteString+fmt.Sprintf → fmt.Fprintf） |
| **unused** | 1 | P2 | `hasSelection` 函数未使用 |

---

## 2.4 专项模式扫描

| 检查项 | 结果 | 说明 |
|--------|------|------|
| D2 dot import | ✅ 0 violations | 无退化 |
| D3 init() panic | ✅ 0 violations | 无退化 |
| D4 反模式目录 | ✅ 0 violations | 无 common/utils/base/helpers |
| D5 import 分组 | ✅ 正常 | goimports 无输出，分组正确 |
| D6 错误大写开头 | ⚠️ 92 potential violations | 多数为合法缩写（URL/HTTP/API 等），需人工抽样 |
| D7 goroutine 管理 | ✅ 无新问题 | 07-27 P0 均已修复 |
| D8 Config Validate | ⚠️ 116 Config / 7 Validate（6%） | 与 07-30 一致，无改善 |
| D9 超大文件 | ⚠️ 10+ 文件 >500 行 | desktop/app.go(1334), chat_app.go(1283), tui_session.go(1060) 等 |
| D10 go vet | ✅ 4 模块全 PASS | 维持 0 问题 |
| D11 变更增量 | ⚠️ 51 个 Go 文件在最近 10 次 commit 中变更 | 无 lint 退化迹象 |

---

## 2.5 新增配置文件的首次扫描结果分析

### tui/.golangci.yml（新建）
首次以 11 个 linter 配置运行，发现 29 个问题。其中：
- `redefines-builtin-id` 在 `core/celldiff.go` 和 `layout/layout.go` 中集中出现 — 是 Go 1.21 引入 `min/max`/`clear` 内置函数后的已知问题
- 注释格式问题集中在 `terminal/ansi.go`（分组注释而非函数级注释）

### desktop/.golangci.yml（新建）
5 个源文件，首次以 10 个 linter 配置运行，0 问题。

---

## 发现摘要

| ID | 严重度 | 模块 | 描述 | 数量 |
|----|--------|------|------|------|
| S01 | P1 | tools | noctx: HTTP 请求缺少 context（browser_camofox/browserproviders/web_search 等） | 19 |
| S02 | P1 | tools | gosec G104: 错误未处理（Close/Kill/Wait 等操作） | ~30 |
| S03 | P1-P2 | tools | gosec G204/G304: 子进程/路径操作需要 nolint 注释 | ~40 |
| S04 | P2 | tui | revive redefines-builtin-id: min/max/new/cap 遮蔽内置函数 | 9 |
| S05 | P2 | tui | staticcheck ST1020/QF1012: 注释格式和简化 | 11 |
| S06 | P2 | tui | unused: hasSelection 函数 | 1 |
| S07 | P2 | tools | errchkjson: json.Marshal of `any` | 2 |
| S08 | P2 | 全局 | Config Validate 覆盖率低（116/7 = 6%） | 109 缺口 |
| S09 | P2 | 全局 | 超大文件（>500 行）共 10+ 个 | 10+ |
