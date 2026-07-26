# TUI 全方位审阅 — 阶段 1 基线报告

> **阶段**：基线建立 + 静态扫描（规划第一阶段）
> **日期**：2026-07-26
> **配套**：`docs/review/tui-code-review-2026-07-25.md`（前次审阅，27 项问题）
> **后续**：阶段 2（12 维度并行深审）+ 阶段 3（主报告整合）

---

## 一、关键发现（按严重性）

### 🔴 Critical-1：Kitty 协议全局状态污染导致测试确定性失败

**复现**：`go test -race -count=5 ./tui/terminal/` 第 2-5 轮必定失败
**根因**：`TestProcessTerminalKittyKbdMode`（`terminal/terminal_kitty_test.go:69-79`）调用 `tm.SetKittyKeyboardFlags(5)` → `terminal.go:145` 写全局 `kittyFlagsGlobal` → 测试结束后残留为 `1`（clamp 下限）→ `decodeKittyU`（`keys.go:328`）后续轮次读到 `flags=1`，跳过 event/alt/text 字段解析。

**证据**：
- 单独运行受害测试 count=5：PASS ✅
- 与污染测试同运行 count=5：第 1 轮 PASS，第 2-5 轮 FAIL ❌
- 失败信息：`alt: want 65 (A), got 0` / `text: want "HI!", got ""`

**影响**：
- 测试隔离性破坏（违反 Go 测试最佳实践）
- **生产风险**：若 `SetKittyKeyboardFlags` 在运行时被错误调用后未恢复，所有 Kitty 协议解析失效
- **07-25 审阅漏检**：前次报告称"全部 11 个包通过（含 -race）"，实际 `count=5` 即失败

**修复方案**（3 选 1）：
1. 测试侧：`TestProcessTerminalKittyKbdMode` 添加 `t.Cleanup(func(){ SetKittyKeyboardFlagsFromTerminal(0) })`（最小改动）
2. 设计侧：`decodeKittyU` 不依赖全局状态，改为参数传入 flags（根治）
3. 架构侧：`ParseKeys` 接受 `flags` 参数，移除全局可变状态

### 🔴 Critical-2：termios 恢复错误被静默吞掉（P0）

**位置**：`terminal/terminal.go:247`
```go
_ = setTermios(t.in.Fd(), saved)  // 失败被吞
return nil                         // Stop 返回 nil
```
**风险**：若 termios 恢复失败，用户终端停留在 raw 模式，shell 错乱，但调用方收到 `nil` 无从知晓。
**对比**：`Start()` 中 setTermios 失败会返回 `fmt.Errorf("tui: set termios: %w", err)`，处理不对称。
**修复**：`if e := setTermios(...); e != nil { return fmt.Errorf("tui: restore termios: %w", e) }`

### 🔴 Critical-3：`PanicMsg` 丢堆栈（P0）

**位置**：`core/message.go:187-189`
```go
done <- PanicMsg{Err: r, Stack: ""}  // Stack 恒为空
```
**对比**：`tui_input.go:221` 和 `:188` 都正确调用 `captureStack()`，唯独此处遗漏。
**影响**：`WithContext` 执行的 Cmd panic 时，日志无堆栈，无法定位故障点。
**修复**：`Stack: captureStack()`（需将 `captureStack` 提升到 core 或注入）

---

## 二、自动化检查基线

| 检查项 | 结果 | 与 07-25 对比 |
|--------|------|-------------|
| `go vet ./...` | ✅ 通过 | 一致 |
| `golangci-lint run ./...` | ✅ **0 issues** | 🎉 **07-25 的 15 项告警全部修复** |
| `go test -race -count=1 ./...` | ✅ 通过 | 一致（但掩盖了 count=5 的问题） |
| `go test -race -count=5 ./...` | 🔴 **terminal 包失败** | 🆕 **新发现** |
| 测试覆盖率（总体） | 53.4% | 与 07-25 一致 |
| Benchmark 测试 | 仅 2 个（chat_history） | 🆕 严重不足 |

### 覆盖率热力图（按包）

| 包 | 覆盖率 | 与 07-25 对比 | 判定 |
|----|--------|-------------|------|
| `tui/layout` | 75.7% | 一致 | ✅ |
| `tui/stdio` | 72.8% | 一致 | ✅ |
| `tui` (root) | 66.3% | -0.1% | ✅ |
| `tui/core` | 59.1% | 一致 | ⚠️ 核心层建议 ≥70% |
| `tui/terminal` | 57.4% | -0.3% | ⚠️ |
| `tui/theme` | 54.7% | +0.3% | ⚠️ |
| `tui/component` | 50.3% | -0.1% | 🔴 |
| `tui/chat` | 47.9% | 一致 | 🔴 |
| `tui/agentadapter` | 14.9% | 一致 | 🔴 严重不足 |

### 零覆盖关键文件 TOP 10（无对应 `_test.go`）

| 行数 | 文件 | 风险 |
|------|------|------|
| 610 | `component/editor_edit.go` | 🔴 编辑器核心，零测试 |
| 608 | `chat/chat_app_layout.go` | 🔴 布局根组件 |
| 556 | `component/session_selector.go` | 🔴 含 4 处无终止 goroutine |
| 535 | `chat/chat_history_render.go` | 🟡 渲染管线 |
| 527 | `terminal/terminal.go` | 🟡 终端核心 |
| 424 | `core/sgr.go` | 🟡 SGR 状态机 |
| 406 | `chat/chat_history_input.go` | 🟡 输入处理 |
| 385 | `component/editor_render.go` | 🟡 |
| 371 | `component/todo_panel.go` | 🟡 |
| 354 | `component/syntax_tokenizer.go` | 🟡 |

---

## 三、07-25 审阅问题修复追踪

### 已修复 ✅（10/27）

| ID | 问题 | 验证方式 |
|----|------|---------|
| C1 | `EventKindFor` 缺 `ApprovalPromptChatEvent` | `state.go:285` 已包含 |
| C2 | `applyColorKey` 缺 4 个 token | `json.go:124-184` 全部处理 |
| H2 | `watch.go` 失败时更新 mtime | 失败时 `continue` 在 `lastMtime=mt` 之前 |
| M1 | 6 个导出 Msg 类型缺 Godoc | `message.go` 全部有注释 |
| M3 | `FlushEsc` 冗余调用 | eventLoop 不再调用，仅 flushLoop |
| M4 | panic 路径未关 stdin | `tui_loop.go:20` 先 `t.stdin.Close()` |
| M5 | 文件加载失败不重试 | 失败时不更新 mtime，下轮重试 |
| M6 | macOS Light 外观检测 | 检查 `exec.ExitError` 返回 Light |
| M7 | `QuantizeTheme` 未标注 no-op | 明确 NOTE 注释 |
| — | 15 项 lint 告警 | `golangci-lint` 返回 0 issues |

### 未修复 ❌（4/27）

| ID | 问题 | 当前状态 | 建议 |
|----|------|---------|------|
| C3 | `composeOverlays` deep-copy | 保留，注释解释原因（防 caller 复用 base） | 评估 CoW 优化 |
| H4 | Editor/Input 硬编码 `48;5;33` | `editor_render.go:355`、`input.go:246` 仍存在 | 主题化 |
| H6 | `Component.Render` 无并发文档 | 仍缺 | 补 Godoc |

### 部分修复 ⚠️（3/27）

| ID | 问题 | 进展 |
|----|------|------|
| H1 | `itoa` 3 处重复 | 减至 2 处（quantize.go 已删），但 sgr.go(int)/ansi.go(int64) 签名不同 |
| H3 | `tui_render.go` 硬编码 ANSI | 从 10 处减至 4 处 |
| H5 | 颜色检测未统一 | `color_resolve.go` 有注释说明，但仍未委托到 `terminal.CurrentTerminalContext` |

---

## 四、维度深审成果（来自并行子智能体）

### 维度 4：错误处理与健壮性 — 评分 6.5/10

**子智能体**：explore #1（140s，32 工具调用）

**核心发现**：
1. **errors.Is/As 零使用**（生产代码）：所有错误为扁平字符串，无 sentinel 判断，无自定义错误类型
2. **8 处 panic recover 策略不一致**：
   - `tui_loop.go` re-panic 保栈 ✅
   - `theme/watch.go` + `system_appearance.go` recover 后 goroutine **静默死亡，永不重启** 🔴
3. **`core/message.go:187` PanicMsg 丢堆栈**（已列 Critical-3）
4. **%w 包装覆盖率良好但不成体系**：11 处包装，但 `theme/global.go`（4 处）裸 return
5. **资源清理健壮性高**：Stop 链、goroutine 终止、幂等设计是模块最成熟部分

**P0/P1 改进清单**：
- P0：termios 恢复错误吞掉（Critical-2）
- P0：PanicMsg 丢堆栈（Critical-3）
- P1：主题/外观 watcher panic 后永久死亡
- P1：嵌套 Stop panic 静默吞掉
- P1：`errTUIAlreadyStopped` sentinel 形同虚设

### 维度 2：并发与线程安全 — 评分 7.5/10

**子智能体**：explore #2（185s，58 工具调用）

**核心发现**：
1. **整体设计纪律优秀**：事件循环单 goroutine + 状态变更者持锁 + 回调在锁外执行
2. **51 处 mutex 类型选择恰当**：component 层 28 处一致使用 RWMutex（Render 读路径用 RLock）
3. **20/22 处 goroutine 有明确终止**：Stop 路径完整等待（`<-readDone`/`<-doneCh`）
4. **🟡 4 处 session_selector 回调 goroutine 无终止**（`session_selector.go:346,398,407,420`）
5. **🟡 stdio 层持锁 I/O**（`renderer.go:40`、`progress.go:48`）
6. **atomic 使用全部正确**，happens-before 设计明确

**P1/P2 改进清单**：
- P1：session_selector 回调 goroutine 加 ctx/done channel
- P1：stdio 持锁 I/O 改为锁外写入
- P2：chat_bridge 锁顺序（`h.mu → TUI.mu`）无测试固化
- P2：theme 热重载回调链路文档化约束

---

## 五、新增基线指标

| 指标 | 数值 | 评估 |
|------|------|------|
| 文件总数 | 152（102 源 + 50 测试） | — |
| 源代码行 | 28,225 | — |
| 测试代码行 | 8,072 | 测试/源 = 0.29（偏低） |
| 导出符号 | 417 | 公共 API 表面大 |
| `New*` 构造函数 | 51 | 需评估一致性 |
| `context.Context` 使用 | 仅 9 处 | 🟡 偏少 |
| `slog` 使用 | 12 处 | 🟡 可观测性偏弱 |
| `errors.Is/As` | 0 处 | 🔴 系统性缺陷 |
| 并发原语 | 51 mutex + 9 atomic + 11 channel | 设计成熟 |
| goroutine 启动点 | 22 处 | 20 有明确终止 |
| panic recover 点 | 8 处 | 策略不一致 |
| Benchmark 测试 | 仅 2 个 | 🔴 严重不足 |
| 大文件（>500 行） | 11 个 | 需评估拆分 |

---

## 六、下一步（阶段 2 规划）

阶段 1 已建立完整基线并发现 3 个 Critical + 2 个维度深审成果。阶段 2 将派出 10 个并行子智能体覆盖剩余维度：

| 子智能体 | 维度 | 重点 |
|---------|------|------|
| code-reviewer | 维度 1 架构 + 维度 7 API | 跨层调用图、417 导出符号一致性 |
| bug-analyzer | 维度 3 性能 | 帧渲染 hot path、Markdown 复杂度 |
| code-reviewer | 维度 5 安全 | OSC 52、ANSI 注入、路径遍历 |
| explore | 维度 6 终端兼容性 | 30 品牌矩阵、降级链 |
| code-reviewer | 维度 8 测试策略 | 零覆盖关键文件、flaky 测试 |
| explore | 维度 9 可维护性 | 圈复杂度、重复代码 |
| code-reviewer | 维度 10 可观测性 + 11 i18n/a11y | slog、硬编码文案 |
| explore | 维度 12 文档 | LAYERS.md 同步、贡献者指南 |

**阶段 2 启动条件**：用户确认后立即执行
