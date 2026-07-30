# 第六阶段：nolint 指令审计报告

**日期**: 2026-07-30

---

## 6.1 nolint 指令全局统计

| 模块 | 有 nolint 的文件数 | nolint 类别分布 |
|------|-------------------|----------------|
| **根模块** | ~95 文件 | gosec(79), revive(31), errchkjson(18), staticcheck(11), unused(2), gocritic(2) |
| **tools** | ~4 文件 | gosec(2), unused(2) |
| **tui** | ~1 文件 | gosec(1) |
| **desktop** | ~1 文件 | 无 |
| **总计** | ~101 文件 | 约 150+ 条 nolint 指令 |

> 注：一个文件可包含多个 nolint 指令（如 `//nolint:gosec,staticcheck`），因此指令数 > 文件数。

---

## 6.2 按 Linter 分布

| Linter | 计数 | 主要用途 | 合理性评估 |
|--------|------|---------|-----------|
| **gosec** | ~80 | G304 路径注入（`//nolint:gosec // path is from filepath.Walk` 等） | ✅ 合理 - 路径来自可控源 |
| **revive** | ~31 | stutter 命名（`evidence.EvidenceExtension` 等）、unexported-return | ✅ 合理 - 设计决策，重命名破坏 API |
| **errchkjson** | ~18 | `map[string]any` 的 JSON 序列化 | ✅ 合理 - 动态类型无法静态检查 |
| **staticcheck** | ~11 | SA1019 废弃 API （Observer 等保留类型） | ✅ 合理 - 向后兼容 |
| **gocritic** | ~2 | exitAfterDefer（defer 作为 panic safety-net） | ✅ 合理 - 显式调用 + defer 备选 |
| **unused** | ~4 | 测试文件引用的未用代码 | ✅ 合理 - 跨测试文件引用 |
| **errcheck** | ~2 | 清理路径关闭错误 | ⚠️ 需要确认已不在 CI 排除规则中 |

---

## 6.3 合理性审查

### ✅ 合理 — 保留
- **gosec G304**: 路径来自 `filepath.Walk`、`filepath.Join(madyHome, ...)` 等可控源 — 90% 以上有解释注释
- **revive stutter**: `evidence.EvidenceExtension`、`planmode.PlanModeExtension` 等 — 重命名破坏公共 API
- **errchkjson**: `map[string]any` — Go 类型系统无法静态保证 `any` 的可 JSON 序列化
- **staticcheck SA1019**: `Middleware`、`AgentRunObserver` 等 — 保留向后兼容

### ⚠️ 需注意 — 建议审查
- **gosec G104**: 当前在全局 `.golangci.yml` 中有 `exclusions` 规则覆盖部分 Close/Remove/Kill/Wait 的 defer，但 tools 模块的 nolint 仅 2 处 — 新暴露的 30+ 处 G104 **缺少 nolint 注释**，需添加
- **errchkjson in tools**: tools 模块的 `ego_lite_manager.go` 有 2 处 `json.Marshal(req)` 对 `any` 类型 — 未带 nolint

### ❌ 建议修复 — 移除或替换
- 部分 nolint 注释不是标准格式（如 `//nolint:gosec // 中文业务标签` 混杂中英文）
- 无证据表明有过期的 nolint 未移除（代码基线与 last audit 一致）

---

## 6.4 建议

| ID | 严重度 | 描述 |
|----|--------|------|
| N01 | P2 | tools 模块新增暴露的 96 处 gosec 问题中约 40 处需要添加明确的 nolint 注释（与现有 CI 排除规则对齐） |
| N02 | P3 | 建议在 `.golangci.yml` 中启用 `nolintlint` linter（仅 format 检查），确保新添加的 nolint 使用标准格式并附带解释 |
