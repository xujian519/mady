# 第 1 轮：自动化全量扫描报告

> **日期**: 2026-07-30 | **范围**: 4 模块（root + desktop + tools + tui），~830 非测试 + ~400 测试 Go 源文件
> **方法**: 全自动脚本 + 工具链 | **耗时**: ~15 分钟

---

## 一、构建与基础检查

| 检查项 | Root | tools | tui | desktop | 结论 |
|--------|------|-------|-----|---------|------|
| `go build ./...` | ✅ | ✅ | ✅ | ✅ | **4/4 通过** |
| `go vet ./...` | ✅ | ✅ | ✅ | ✅ | **4/4 通过** |

---

## 二、Lint 全量扫描

### 2.1 根模块（root + domains + agentcore + ...）
```
golangci-lint run ./...
→ 0 issues
```

### 2.2 desktop 子模块
```
golangci-lint run ./desktop/...
→ 0 issues
```

### 2.3 tui 子模块 — 🟡 30 issues

| Linter | 数量 | 说明 |
|--------|------|------|
| revive | 17 | 导出符号注释格式、命名规范 |
| gosec | 6 | G115 整数溢出、G104 错误忽略 |
| staticcheck | 5 | ST1020 注释格式、未使用代码 |
| noctx | 2 | HTTP 请求未携带 context |

### 2.4 tools 子模块 — 🔴 162 issues（需重点关注）

| Linter | 数量 | 说明 |
|--------|------|------|
| errcheck | **65** | 大量错误返回值未检查 |
| gosec | **51** | G104 错误忽略、G306 文件权限、G304 路径遍历 |
| noctx | **19** | HTTP 请求未携带 context |
| staticcheck | **19** | 未使用变量、冗余操作 |
| revive | 4 | 导出符号注释缺失 |
| errchkjson | 2 | JSON 编码类型未检查 |
| unused | 2 | 未使用代码 |

> ⚠️ **关键发现**：`tools/.golangci.yml` 仅启用 `govet` + `ineffassign`（2 个 linter），而根模块有 13 个。这导致了大量问题在工具层未被检测。

---

## 三、10 条硬约束逐项扫描

### D1 — Error 检查 ✅🟡

golangci-lint errcheck 在根模块 0 issues，但在 tools 子模块发现 **65 处**未检查错误。另外专项扫描发现：

| 模式 | 数量 | 典型代码 |
|------|------|---------|
| 类型断言 `v, _ := state[key].(Type)` | ~30 处 | `disclosure/`、`domains/claimdrafting/` |
| io.ReadAll / json.Marshal 忽略 error | ~20 处 | `tools/vision.go:204`、`provider/chatcompat/` |
| `_, _ = fmt.Fprintf(stderr, ...)` | ~20 处 | `cmd/mady/evidence.go` |
| `_ = db.Close()` / `_ = file.Close()` | ~12 处 | `domains/case_index.go:103`、`memory/sqlite_store.go:63` |

### D2 — 禁止 dot import ✅

**0 violations** — 全仓无 `import .` 使用。

### D3 — init() 无 panic ✅

**0 violations** — 9 个 `func init()` 均无 `panic()` 调用。

### D4 — 无 common/utils/base 包 ✅

**0 violations** — 顶级 `common/`/`utils/`/`base/`/`helpers/` 均不存在。仅 `pkg/util/`（5 文件，~600 行，体量可控）。

### D5 — Import 三组排序 🟡 1 violation

| 文件 | 问题 |
|------|------|
| `tracing/otel.go:12-22` | import 顺序异常：本地包 (`agentcore`) → 第三方 (`go.opentelemetry.io`) → stdlib 别名 (`sdktrace`)，应重新排序 |

其余抽样 200 文件中未发现违规。

### D6 — 错误信息小写开头 🟡 ~24 处

已过滤专有名词（JSON/YAML/HTTP/SQL/OCR/PDF/LLM/SSE），保留真正的违规：

| 文件 | 示例 |
|------|------|
| `tools/netutil.go:37,41,44` | `"BLOCKED: refusing to dial..."` |
| `tools/web_fetch.go:176,180,183` | `"BLOCKED: the target site..."`、`"BLOCKED: rate limited..."` |
| `tools/desktop/computer_use.go:246` | `"DENIED by user"` |
| `tools/desktop/computer_use_safety.go:61,74,96` | `"BLOCKED: ..."` |
| `tools/browser_session.go:389,403` | `"CDP URL is required"`、`"CDP supervisor failed..."` |
| `cmd/mady/ocr.go:76,84,135` | OCR 错误信息大写开头 |

> 注：`BLOCKED`/`DENIED` 是安全类错误，全大写可能是刻意设计，建议统一为小写开头。

### D7 — Goroutine 生命周期管理 🟡 3 处高风险

| 严重度 | 文件:行 | 说明 |
|--------|---------|------|
| 🔴 高 | `server/evidence.go:97` | `go func()` 无 context、无 cancel、无 select |
| 🔴 高 | `server/evidence.go:159` | 同上，批量证据判断 goroutine |
| 🔴 高 | `knowledge/extension.go:438,452,481` | 多处 `go func()` 无 context 管理 |
| 🟡 中 | `memory/extension.go:151,331` | 有 nolint 标记，使用 `context.WithTimeout(Background, 30s)` 兜底 |

其他 `agentcore/`、`graph/`、`a2a/`、`tui/` 中的 goroutine 均有 context/select/WaitGroup 管理，规范合规。

### D8 — 导出符号必须有注释 🟢

revive exported 规则检查：根模块 0 issues。抽查 20 个文件发现导出函数基本都有 doc 注释。

### D9 — 配置 Validate 模式 🔴 系统性缺失

**96%+ 的 Config struct 无 Validate() 方法。** 在 `tools/` 子模块中尤为突出——30 个工具 Config 全部无 Validate。规范要求 "配置结构体提供 `Validate()` 方法，在 `New()` 中提前校验"，但实际执行严重不足。

### D10 — Mutex 使用规范 ✅

**0 violations** — 所有含 `sync.Mutex`/`sync.RWMutex` 的结构体均使用指针接收者，无复制风险。Mutex 命名以 `xxxMu` 为主，与规范一致。

---

## 四、AI 高频违规 6 类专项扫描

### D11 — 幻觉 API 🟢（初步）

通过 `go list -m all` 对照所有 import 的第三方包：未发现引用不存在的包或函数。根模块中所有第三方依赖均可追溯到 go.mod。

### D12 — 重复造轮子 🟡（标记关注）

- `fuzzy/fuzzy.go` vs `tui/internal/fuzzy/fuzzy.go` — 两个模糊匹配实现，需确认是否可合并
- `domains/rules/` 与 `domains/workflows/patent/` 中 `Severity` 类型重复定义（已知 07-27 P1 项，未修复）

### D13 — 风格漂移 🟡（标记关注）

- `tools/` 子模块错误处理风格与 `agentcore/` 不一致：agentcore 使用 `NodeError`/`RetryableError`/`FatalError` 分层错误，tools 大量使用裸 `fmt.Errorf`
- `tools/` 中 `%w` vs `%v` 混用

### D14 — 过度工程化 🟡（标记关注）

- `tools/` 中 18 个 `XxxOperations` 接口（已知 07-27 P1 项），仅 1 个实现者
- 多个仅使用 1 次的泛型函数

### D15 — Context 传播 🟡 19+ violations

| 模块 | noctx violations | 说明 |
|------|-----------------|------|
| tools | **19** | HTTP 请求未携带 context |
| tui | 2 | 同上 |

### D16 — 测试覆盖 🟡

- `domains/checker/`: **0% 测试覆盖**（4 源文件，0 测试）
- tools 子模块: **37 个工具文件无对应 `_test.go`**
- 测试中 `time.Sleep` 使用: **43 处 / 18 文件**（脆弱测试风险）

---

## 五、文件规模与复杂度

### 超大文件（>500 行非测试）

```
1268 desktop/app.go
1203 domains/workflows/patent/rule_engine.go
1195 bootstrap/setup.go
1194 cmd/mady/tui_session.go
1184 tui/chat/chat_app.go           ← 5 个 >1000 行
1041 domains/inventiveness/nodes.go
1013 acp/server.go
 919 example/cli-chat/main.go
 883 a2a/ws.go
 881 tui/component/markdown.go
 815 domains/evidence/engine.go
 809 domains/workflows/patent/reexamination.go
 800 knowledge/sqlite/store.go
 796 mcp/discovery.go
 796 domains/workflows/patent/invalidation.go
 795 tui/terminal/detect.go
 779 domains/enablement/nodes.go
 768 tools/browser_session.go
 765 cmd/mady/slash_registry.go
 756 domains/case_index.go
```

**共 20 个文件 >500 行，其中 5 个 >1000 行。**

---

## 六、汇总统计

| 维度 | 发现数 | 严重度分布 |
|------|--------|-----------|
| D1 Error 检查 | ~82 处（tools 65 + 手工 ~17） | P1 |
| D2 Dot import | 0 | ✅ |
| D3 init() panic | 0 | ✅ |
| D4 反模式目录 | 0 | ✅ |
| D5 Import 分组 | 1 | P2 |
| D6 错误信息格式 | ~24 | P2 |
| D7 Goroutine 管理 | 3 高风险 + 2 中风险 | **P0** |
| D8 导出注释 | 0 (根模块) | ✅ |
| D9 Config Validate | 系统性缺失（>96%） | P1 |
| D10 Mutex 规范 | 0 | ✅ |
| D15 Context 传播 | 21（tools 19 + tui 2） | P1 |
| tools lint | **162 issues** | P0-P2 |
| tui lint | 30 issues | P1-P2 |
| 超大文件 | 20（5 >1000行） | P2 |
| 测试中 time.Sleep | 43 处 | P2 |

### 健康度评估（与 07-27 对比基线 79/100 B+）

| 维度 | 07-27 得分 | 本次评估 | 变化 |
|------|-----------|---------|------|
| 构建/编译 | 90 | 95 | +5（新增 desktop 模块全绿） |
| Lint 门禁 | 90 | 70 | **-20**（暴露 tools 162 issues / tui 30 issues） |
| 安全红线 | 85 | 85 | 持平（敏感路径门禁完整） |
| 代码质量 | 75 | 70 | -5（D9 Validate 系统性缺失、D15 21 处） |
| 文档/测试 | 70 | 70 | 持平 |

> **本次健康度暂估：72/100（B）** — tools 子模块 lint 盲区是主要降分项。

---

## 七、修复优先级（第 1 轮可立即采取的行动）

| 优先级 | 行动 | 影响 |
|--------|------|------|
| **P0** | 修复 `server/evidence.go:97,159` 和 `knowledge/extension.go:438` 的 goroutine 泄漏风险 | 3 处高风险 |
| **P0** | **为 `tools/.golangci.yml` 补充 linter 规则**，与根模块保持一致（至少 errcheck + gosec + staticcheck） | 防止 162 issues 继续增长 |
| **P1** | 分批次修复 tools/ 中 65 处 errcheck + 19 处 noctx | 改善代码健壮性 |
| **P1** | 要求所有 Config struct 提供 `Validate()` 方法（尤其是 `tools/` 30 个工具 Config） | 建立配置校验基线 |
| **P2** | 修复 `tracing/otel.go` import 分组 | 1 行修改 |
| **P2** | 修正 ~24 处错误信息大小写 | 代码一致性和规范遵从 |
| **P3** | 逐步替换测试中的 `time.Sleep` 为 channel/retry | 防止脆弱测试 |
