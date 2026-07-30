# 第三阶段：补充 Linter 评估与推荐报告

**日期**: 2026-07-30

---

## 3.1 候选 Linter 分类评估

依据 golangci-lint v2.12.2 可用 linter 列表，对当前未启用的 ~85 个 linter 进行评估：

### 高优先：推荐启用

| Linter | 理由 | 预期新增问题 | 风险 |
|--------|------|-------------|------|
| **`errorlint`** | GO-DEVELOPMENT-STANDARDS.md S4.2/S4.4 要求：错误应使用 `errors.Is`/`errors.As` 而非裸类型断言，`%w` 包装而非 `%v`。当前无自动化检查 | 中等，10-30 处 | 低 - 与项目规范一致 |
| **`nilerr`** | 检测 `return nil` 同时 `if err != nil` 的冲突模式 | 低，0-5 处 | 低 - 真阳性率高 |
| **`nilnil`** | 检测同时返回 nil error 和无效值的函数 | 低，2-8 处 | 低 - 根模块已有类似检查 |
| **`prealloc`** | 07-27 修复了 45 处切片预分配，但未启用 linter 防止复发 | 中，10-20 处 | 低 - 纯性能优化 |
| **`nolintlint`** | 当前 ~104 文件有 `//nolint` 指令，需确保格式正确、有解释 | 低，0-10 处 | 低 - 规范整洁 |

### 中优先：建议讨论后启用

| Linter | 理由 | 预期新增问题 | 风险 |
|--------|------|-------------|------|
| `thelper` | 确保测试 helper 函数调用 `t.Helper()` | 中，10+ 处 | 低 - 测试规范 |
| `tparallel` | 检测 `t.Parallel()` 误用 | 低 | 低 - 并发测试正确性 |
| `paralleltest` | 检测可并行执行但未用 `t.Parallel()` 的测试 | 中 | 中 - 可能在慢测试上制造竞态 |
| `predeclared` | 防止遮蔽 `error`、`len` 等内置标识符 | 低 | 低 - 提高可读性 |
| `dupword` | 检测源码中重复单词（如 "the the"） | 低 | 极低 - 纯格式 |
| `unparam` | 检测未使用的函数参数 | 中-高 | 中 - 部分接口实现需要签名匹配 |

### 低优先：技术债务跟踪/无需启用

| Linter | 理由 | 不启用原因 |
|--------|------|-----------|
| `nestif` | 检测深度嵌套 | 收益递减，当前代码已有良好左对齐风格 |
| `mnd` | 魔法数字检测 | 大量阈值/常量会误报 |
| `funlen`/`gocognit`/`gocyclo` | 复杂度检查 | 已知热点函数（agent_run.go runInnerLoop 79 cogn）但短期内不会重构 |
| `interfacebloat` | 大接口检测 | 第 3 轮审计已识别 20 个大接口，但无需自动门禁 |
| `gocritic` | 综合审查 | 规则集大且误报率高 |
| `makezero` | 检测 make+append 的前导零 | 当前代码无此模式 |
| `wrapcheck` | 检查外部错误包装 | 工具层有大量裸透传，会产生大量噪音 |
| `tagalign` | struct tag 对齐 | 纯美学，无功能收益 |
| `canonicalheader` | HTTP header 规范 | tools 少数 HTTP 代码即将被 noctx 修复覆盖 |

---

## 3.2 启用策略建议

**第一阶段（推荐立即启用）**：
```yaml
# errorlint 先设 severity: warning 以评估影响
errorlint: {}
```
原因：与 GO-DEVELOPMENT-STANDARDS.md 的 S4.2（错误包装 %w）和 S4.4（sentinel 类型断言）直接对应。

**第二阶段（修复 noctx 问题后）**：
```yaml
- nilerr
- nilnil
- prealloc
```
原因：这三个 linter 误报率低，修复成本低。

**第三阶段（增加测试规范）**：
```yaml
- thelper
- testifylint  # 如果项目引入 testify
```
原因：测试质量的持续改进。

---

## 3.3 与 GO-DEVELOPMENT-STANDARDS.md 的对照矩阵

| 规范章节 | 规范要求 | 自动检查 | 当前覆盖率 |
|---------|---------|---------|-----------|
| S2.2 标识符命名 | PascalCase/camelCase | revive (receiver-naming, error-naming) | ✅ 部分覆盖 |
| S2.3 导入分组 | stdlib → 第三方 → 本地 | goimports (pre-commit) | ✅ 已覆盖 |
| S3.2 尽早返回 | happy path 左对齐 | revive (indent-error-flow 已禁用) | ❌ 手动审查 |
| S4.2 错误包装 %w | 使用 fmt.Errorf + %w | **errorlint （未启用）** | ❌ 缺口 |
| S4.4 sentinel 断言 | errors.Is / errors.As | **errorlint （未启用）** | ❌ 缺口 |
| S6.2 Mutex 使用 | sync.RWMutex + xxxMu 命名 | 无自动化检查 | ❌ 手动审查 |
| S7.6 断言风格 | 标准库 testing | testifylint（未引入 testify） | N/A |
| S10.1 导出符号文档 | 每个导出符号必须注释 | revive (exported) | ✅ 已覆盖 |
| S12.2 密钥管理 | 禁止硬编码密钥 | gosec G101 | ✅ 已覆盖 |

**关键缺口**：
1. **errorlint** — 影响 S4.2/S4.4，是最优先填补的缺口
2. **Mutex 命名检查** — 当前无自动化工具可检查 `xxxMu` 命名规范
3. **注释质量** — 目前仅检查"有无注释"，不检查"注释是否高质量"
