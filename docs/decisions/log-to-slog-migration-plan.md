# log → slog 迁移方案

> **状态**：方案待评审
> **创建**：2026-07-19（技术债务里程碑 4）
> **背景**：全量技术债务审查发现项目同时使用 `log` 标准库（22 文件）与 `log/slog`（18 文件），需统一到 `log/slog` 以获得结构化日志、级别过滤、可观测性集成。

## 现状

| 维度 | log 标准库 | log/slog |
|------|-----------|----------|
| 文件数 | 22 | 18 |
| 调用数 | 80（77 Printf + 2 Println + 1 Fatal） | ~30 |
| 集中包 | cmd/mady(3)、mcp(4)、server(3)、tools(3)、knowledge(4) | a2a(6)、agentcore(4)、acp(3) |
| 风格 | `log.Printf("[pkg] action: %v", err)` | `slog.Error("action", "err", err)` |

**关键事实**：无文件同时使用两者，迁移无冲突。

## 目标

1. 所有日志统一到 `log/slog`，消除 `log.Printf`
2. 保留现有的 `[pkg]` 前缀约定（过渡期），逐步迁移到 `slog.With("pkg", "xxx")` logger
3. 不改变日志的**可见性级别**（Printf → Info，WARNING 标注 → Warn，错误 → Error）
4. `log.Fatal` 改为 `slog.Error` + `os.Exit(1)`（保留退出行为）

## 迁移规则

### 规则 1：级别映射

| 原 log 调用 | slog 级别 | 判定依据 |
|------------|-----------|----------|
| `log.Printf("xxx failed: %v", err)` 含 "failed/error" | `slog.Error` | 错误关键词 |
| `log.Printf("[pkg] WARNING: ...")` | `slog.Warn` | WARNING 标注 |
| `log.Printf("starting on %s", addr)` 启动信息 | `slog.Info` | 启动/状态 |
| `log.Printf("[pkg] xxx: %v", err)` 错误恢复 | `slog.Warn` | 已处理但不期望 |
| `log.Fatal(...)` | `slog.Error` + `os.Exit(1)` | 退出前记录 |

### 规则 2：参数风格

**过渡期**（最小改动，保持 `%v` 格式化）：
```go
// 原
log.Printf("[memory] RememberFromTurn failed: %v", err)
// 迁移后
slog.Error("memory: RememberFromTurn failed", "err", err)
```

**目标态**（完全结构化，按包渐进推进）：
```go
// 包级 logger
var logger = slog.Default().With("pkg", "memory")
// 使用
logger.Error("RememberFromTurn failed", "err", err)
```

### 规则 3：不迁移的场景

- `example/` 下的示例程序保留 `log.Printf`（教学性质，简单直观）
- `scripts/` 下的脚本保留 `log.Printf`（一次性工具）

## 迁移顺序（按风险从低到高）

### 阶段 1：低风险包（5 个，约 20 处）
- [ ] `knowledge/`（4 文件）—— 内部错误日志
- [ ] `memory/`（1 文件）—— panic recover 日志
- [ ] `domains/`（2 文件）—— registry 错误
- [ ] `tui/theme/`（1 文件）—— watcher panic
- [ ] `agentcore/`（1 文件）—— config 警告

### 阶段 2：基础设施包（3 个，约 30 处）
- [ ] `server/`（3 文件）—— SSE 错误、启动信息
- [ ] `mcp/`（4 文件）—— client 错误
- [ ] `tools/`（3 文件）—— sandbox 警告

### 阶段 3：入口包（1 个，约 25 处）
- [ ] `cmd/mady/`（3 文件）—— 启动信息、shutdown 日志
- [ ] 保留 `log.Fatal` → `slog.Error` + `os.Exit(1)` 的转换

### 不迁移
- `example/a2a-server/`（1 文件）—— 示例代码
- `cmd/mady/`（3 文件，27 处）—— **入口包保留 `log`**：
  - Go 惯用法：入口包的启动/关闭日志用标准库 `log` 是社区主流做法
  - slog 需要额外配置 handler，入口包用 slog 增加复杂度而无收益
  - 这些日志只在启动/关闭时打印一次，无结构化查询需求
  - 如需统一日志格式，应在 `main()` 中配置 slog 作为 `log` 的后端（`slog.NewLogLogger`）

## 验证

每阶段完成后：
1. `grep -r "log\.Printf\|log\.Println" --include="*.go" <包路径>` 确认无残留
2. `go build ./...` + `go test ./...` 全过
3. `pre-commit run --all-files` 全过
4. 人工抽查日志输出格式（确保级别和字段正确）

## 风险评估

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 级别判断错误（Error 误判为 Info） | 中 | 日志可见性变化 | 按规则 1 的关键词严格判定 |
| slog 默认级别过滤掉 Info | 低 | 启动日志消失 | 确认 slog 默认级别为 Info |
| 性能影响 | 极低 | slog 比 log 略慢 | 可接受（日志非热路径） |

## 工作量估算

- 总计 ~80 处调用，分布在 13 个文件
- 每处平均 2 分钟（含级别判定）
- 总工作量：约 3 小时（含测试验证）
- 建议分 3 次 PR 完成（对应 3 个阶段）
