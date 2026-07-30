# 代码异味优化 — 执行追踪清单

> 对应 `docs/refactoring/optimization-plan.md` 的任务拆解。
> 每个任务完成后在此打勾，并在 `docs/decisions/AI_CHANGELOG.md` 追加记录。

---

## 阶段一：基础设施加固

### P1-A: Linter 配置增强

| # | 任务 | 负责人 | 状态 | PR |
|---|------|--------|------|-----|
| 1 | `.golangci.yml` 追加 gocognit | | ☐ | |
| 2 | `.golangci.yml` 追加 dupl | | ☐ | |
| 3 | `.golangci.yml` 追加 unparam | | ☐ | |
| 4 | 设置 `min-complexity: 30` / `threshold: 100` | | ☐ | |
| 5 | `make verify` 包含新 linter | | ☐ | |
| 6 | AI_CHANGELOG.md 记录 | | ☐ | |

### P1-B: Worktree 清理

| # | 任务 | 负责人 | 状态 | PR |
|---|------|--------|------|-----|
| 7 | `git worktree remove --force` 清理 `.claude/worktrees/` | | ☐ | |
| 8 | `git worktree remove --force` 清理 `~/.grok/worktrees/` | | ☐ | |
| 9 | `git worktree prune` 清理引用 | | ☐ | |
| 10 | 验证 `git worktree list` 只显示必要项 | | ☐ | |

---

## 阶段二：中频重构

### P2-A: 高认知复杂度函数拆分

| # | 函数 | 文件 | 当前复杂度 | 负责人 | 状态 | PR |
|---|------|------|-----------|--------|------|-----|
| 11 | `NewComputerUseTool` | `tools/desktop/computer_use.go` | 120 | | ☐ | |
| 12 | `NewBashTool` | `tools/bash.go` | 79 | | ☐ | |
| 13 | `NewViewTool` | `tools/view.go` | 75 | | ☐ | |
| 14 | `handleNavigate` | `tools/browser_tool_navigate.go` | 70 | | ☐ | |
| 15 | `NewReadTool` | `tools/read.go` | 62 | | ☐ | |
| 16 | `cuaDriverCapture` | `tools/desktop/computer_use_cua_driver.go` | 62 | | ☐ | |
| 17 | `waylandGetWindowBounds` | `tools/desktop/computer_use_lin.go` | 61 | | ☐ | |
| 18 | `renderSOMBody` | `tools/desktop/computer_use_som.go` | 40 | | ☐ | |
| 19 | `newEgoLiteHandoffTool` | `tools/ego_lite.go` | 48 | | ☐ | |
| 20 | `NewPandocTool` | `tools/pandoc.go` | 46 | | ☐ | |
| 21-31 | 其余 `New*Tool` 函数（~11 个） | 各文件 | 30-42 | | ☐ | |

### P2-B: 重复代码消除

| # | 位置 | 描述 | 负责人 | 状态 | PR |
|---|------|------|--------|------|-----|
| 32 | `a2a/ws.go:277-351` | 提取 wsHandler 模板 | | ☐ | |
| 33 | `acp/server.go:823-881` | 提取 acpHandler 模板 | | ☐ | |
| 34 | 工具工厂函数 | 提取 ToolBuilder | | ☐ | |

### P2-C: 未使用参数清理

| # | 范围 | 数量 | 负责人 | 状态 | PR |
|---|------|------|--------|------|-----|
| 35 | `tools/` 子模块 | 13 处 | | ☐ | |
| 36 | `a2a/` 包 | ~3 处 | | ☐ | |
| 37 | `acp/` 包 | ~2 处 | | ☐ | |
| 38 | `domains/` 包 | ~10 处 | | ☐ | |
| 39 | `evaluate/` 包 | ~2 处 | | ☐ | |
| 40 | `example/` 包 | ~2 处 | | ☐ | |
| 41 | `graph/` 包 | ~2 处 | | ☐ | |
| 42 | `intent/` 包 | ~2 处 | | ☐ | |
| 43 | `knowledge/` 包 | ~5 处 | | ☐ | |
| 44 | `mcp/` 包 | ~3 处 | | ☐ | |
| 45 | `memory/` 包 | ~2 处 | | ☐ | |
| 46 | `provider/` 包 | ~2 处 | | ☐ | |
| 47 | `server/` 包 | ~2 处 | | ☐ | |
| 48 | `workflows/` 包 | ~2 处 | | ☐ | |

---

## 阶段三：深度重构

### P3-A: God 包拆分（domains）

| # | 任务 | 负责人 | 状态 | PR |
|---|------|--------|------|-----|
| 49 | `domains/config/` 子包创建 + `unified.go` 迁入 | | ☐ | |
| 50 | `domains/router/` 子包创建 + `router.go` 迁入 | | ☐ | |
| 51 | `go-arch-lint.yml` 组件配置更新 | | ☐ | |
| 52 | `docs/chat-assistant-architecture.md` 同步 | | ☐ | |

### P3-B: 千行级文件拆分

| # | 文件 | 拆分目标 | 负责人 | 状态 | PR |
|---|------|---------|--------|------|-----|
| 53 | `desktop/app.go` (1336行) | 4 文件 | | ☐ | |
| 54 | `tui/chat/chat_app.go` (1283行) | 5 文件 | | ☐ | |
| 55 | `cmd/mady/tui_session.go` (1060行) | 4 文件 | | ☐ | |
| 56 | `acp/server.go` (1013行) | 4 文件 | | ☐ | |

### P3-C: 生产 panic 审计

| # | 文件 | 行 | 处置 | 负责人 | 状态 | PR |
|---|------|-----|------|--------|------|-----|
| 57 | `knowledge/standards/ipc-standards.go` | 68 | `panic → return error` | | ☐ | |
| 58 | `agentcore/event_logger.go` | 62 | `panic → return error + sync.Once` | | ☐ | |
| 59 | `agentcore/concurrency/pool.go` | 68 | 保留（合理前置条件） | | ☐ | |
| 60 | `domains/reasoning/fact_blackboard.go` | 55 | `panic → return error` | | ☐ | |
| 61 | `pkg/csync/csync.go` | 32-36 | 保留（前置条件） | | ☐ | |
| 62 | `domains/specdrafting/scorer.go` | 14 | 保留（nil 检查合理） | | ☐ | |

### P3-D: Goroutine 安全管理

| # | 位置 | 风险级 | 负责人 | 状态 | PR |
|---|------|--------|--------|------|-----|
| 63 | `agentcore/executor.go:363` | P0 | | ☐ | |
| 64 | `agentcore/pubsub.go:115` | P0 | | ☐ | |
| 65 | `graph/pregel.go:291` | P0 | | ☐ | |
| 66 | `graph/graph.go:302` | P1 | | ☐ | |
| 67 | `acp/server.go:773` | P1 | | ☐ | |
| 68 | 其余 10 处 `go func()` | P2 | | ☐ | |

### P3-E: 架构边界违规修复

| # | 任务 | 负责人 | 状态 | PR |
|---|------|--------|------|-----|
| 69 | `agentcore` 新增 `ToolProvider` 接口 | | ☐ | |
| 70 | `tools` 实现 `ToolProvider` 接口 | | ☐ | |
| 71 | `domains/*.go` 改为依赖接口 | | ☐ | |
| 72 | `bootstrap/setup.go` 注入具体实现 | | ☐ | |
| 73 | `go-arch-lint deepScan: true` 验证通过 | | ☐ | |

### P3-F: TODO 处理与文档同步

| # | 任务 | 负责人 | 状态 | PR |
|---|------|--------|------|-----|
| 74 | `workflows/workflow.go` TODO 加说明注释 | | ☐ | |
| 75 | `docs/chat-assistant-architecture.md` 同步 | | ☐ | |
| 76 | `.go-arch-lint.yml` 变更说明补全 | | ☐ | |
| 77 | `docs/decisions/AI_CHANGELOG.md` 追加全量记录 | | ☐ | |

---

## 汇总

| 阶段 | 任务数 | 预估工时 | 完成 | 剩余 |
|------|--------|---------|------|------|
| P1 基础设施 | 10 | 1-2 人日 | ☐/10 | 10 |
| P2 中频重构 | 38 | 2-3 人日 | ☐/38 | 38 |
| P3 深度重构 | 29 | 2-3 人日 | ☐/29 | 29 |
| **总计** | **77** | **5-8 人日** | **0/77** | **77** |
