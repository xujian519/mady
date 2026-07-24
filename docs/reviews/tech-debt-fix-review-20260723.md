# 技术债务修复代码审查

> 审查日期：2026-07-23 | 审查范围：20 文件，140 行新增 / 101 行删除
> 审查类型：Strict Code Quality Review

---

## 总体评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 行为正确性 | ✅ 通过 | `make verify` 全绿，行为无变更 |
| 结构性改进 | ⚠️ 1 项需推敲 | `deprecatedHookAdapter` 重命名未减少概念 |
| 可维护性 | ✅ 良好 | 大多数改动清理了遗留问题 |
| 文件大小 | ✅ 无问题 | 无文件超过 1000 行 |
| 回归风险 | ✅ 低 | 所有改动为局部重构或死代码移除 |

---

## Findings（按优先级排序）

### F1 — 重命名未降低复杂度（结构性）

**文件**：`agentcore/hooks.go`、`agentcore/agent.go`、`agentcore/agent_run_tool.go`、`agentcore/hooks_test.go`

**问题**：`deprecatedHookAdapter` → `legacyHookBridge` 是纯重命名。结构体定义、三个回调字段、两个方法、`blockedTools` map、nil 守卫逻辑完全一致。改名前和改名后，读者仍然需要理解"有一个适配器桥接旧回调接口到新 Lifecycle 接口"这个概念。

代码-judo 机会被错过了。这里有两条更干净的路径：

**路径 A（激进）**：完全移除 `legacyHookBridge`、`Config.BeforeToolCall`、`Config.AfterToolCall`、`Config.PostProcessResults` 以及 `WithBeforeToolCall`/`WithAfterToolCall` 设置函数。这是 v0.6.0 的 TODO，但提前做不会破坏任何生产代码（grep 确认无人设置这些字段）。

**路径 B（保守）**：保留适配器不重命名，接受"这是已知的向后兼容设计"。重命名不解决任何实际问题，只是把"我已读"标签从一个词换到另一个词。

**建议**：走路径 A，真正删除这些代码。三个 field + 一个 struct + 两个 setter 函数 + adapter 的全部逻辑从代码库中消失。或者走路径 B，revert 重命名，加注释说明这是有意为之的兼容设计。

### F2 — `server/metrics.go` 移除 nolint 未处理 error（边界安全性）

**文件**：`server/metrics.go`

**问题**：6 处 `//nolint:errcheck` 被移除，`w.Write()` 的返回值现在被静默丢弃。注释说"写入内存 buffers 不会失败"，但实际上 `http.ResponseWriter.Write()` 的接口契约允许返回错误（如客户端断开连接时）。

在实际使用中，Prometheus 风格的 `/metrics` 端点通常不处理写入错误（scraper 断开是常态），所以行为上没问题。但移除 nolint 标注意味着：
- 任何配置了 `errcheck` 的 linter 都会重新标记
- 移除了之前显式的"我了解这个错误"声明

**建议**：加一行注释说明为什么忽略是安全的：
```go
// Metrics 端点为 Prometheus scrape 设计；客户端断开时写入错误可安全忽略。
w.Write([]byte(...))
```

### F3 — `guardrails/disclaimer.go` 移除了公开导出的函数（API 兼容性）

**文件**：`guardrails/disclaimer.go`

**问题**：`func DisclaimerFor(domain string) string` 被删除。虽然 grep 确认没有生产代码调用它，但它是公开导出的符号（首字母大写），属于包的公共 API。删除公开导出的函数在没有 semver 标记的项目中是一种隐含的破坏性变更。

**建议**：将其标记为 `Deprecated` 并保留（与之前相同），或者在 `AI_CHANGELOG.md` 中明确说明已确认无调用方。当前 CHANGELOG 条目已经说明了这一点，可以接受。

### F4 — 其余改动评价（无问题）

| 文件 | 改动 | 评价 |
|------|------|------|
| `tools/vision.go` | panic → error 返回 | ✅ 正确修复，消除了生产代码中不应存在的 panic |
| `domains/claimdrafting/builder.go` | if-else → switch | ✅ gocritic 标准修复 |
| `domains/claimdrafting/rules_formality.go` | SA9003 空分支反转 | ✅ 正确，语义保留 |
| `domains/rules/engine_formatters.go` | nolint:gosec | ✅ 正确标注假阳性 |
| `guardrails/doc.go` | 示例更新 | ✅ 改用非废弃 API |
| `tui/chat/chat_app.go` | TODO 更新 | ✅ 注释质量改善 |
| `workflows/patent/infringement.go` | 包文档更新 | ✅ 准确反映实际架构 |
| `domains/infringement/doc.go` | 包文档更新 | ✅ 同上 |
| `infringement/*.go` (5 files) | gofmt 对齐 | ✅ 格式化修正 |
| `docs/decisions/AI_CHANGELOG.md` | 记录变更 | ✅ 符合项目规范 |

---

## 总结

大多数改动是正确的、有针对性的修复。唯一值得重新审视的是 **F1**：`deprecatedHookAdapter` 重命名——要么真的删除它（路径 A：移除废弃字段 + 设置函数 + 适配器），要么还原重命名作为纯 no-op 保留（路径 B）。当前的重命名状态是"改了东西但没解决问题"。

**建议处理方式**：如果确认没有生产代码设置这三个废弃字段，走路径 A 删除整个适配器链——这会让 agentcore 的钩子系统更清晰。否则走路径 B 还原重命名。
