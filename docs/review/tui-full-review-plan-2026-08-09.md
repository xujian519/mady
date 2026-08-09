# Mady TUI 全量审阅计划（2026-08-09）

> **审阅日期**：2026-08-09
> **计划依据**：`.qoder/repowiki/zh/content/用户界面/TUI终端界面/` 7 篇文档
> （TUI架构设计 / TUI终端界面 / 主题系统 / 渲染引擎 / 组件系统 / 终端兼容性 / 输入处理）
> **审阅范围**：`tui/` 独立子模块，230 个 Go 文件（116 生产 + 114 测试），
> 31,708 行生产代码 + 25,516 行测试代码，8 层 Elm 架构
> **配套基线**：
> - `docs/review/tui-code-review-2026-07-25.md`（前次审阅，27 项问题）
> - `docs/review/tui-full-audit-2026-07-26.md`（12 维度全量审阅：5 Critical + 8 High + 15 Medium）
> - `docs/review/tui-dimension3-performance-2026-07-26.md`（13 项 Benchmark 实测）
> - `docs/review/tui-spec-compliance-review-2026-07-31.md`（规范符合性）
> - `docs/decisions/tui-dual-sgr-style-models.md`（双 SGR 模型 ADR）

---

## 一、背景与目标

自 2026-07-26 上次全量审阅以来，TUI 模块经历约 60 次提交（累计 145 条），
规模从 152 文件 / ~28K 源代码增长至 230 文件 / 31.7K 生产代码。
期间的高风险变更包括：流式输出对账重构（`7b2639f`）、FSM 全面接管（`80f5a30`）、
Markdown 渲染修复（`f7b1fbb` 等 4 条）、死代码清理（`ad99d87`）、input 拆分（`aab1cdf`）。

**审阅目标**：
1. 基于 7 篇 repowiki 文档作为代码理解入口，对 8 层架构做全量代码走查
2. 核对上次审阅 28 项缺陷（5C + 8H + 15M）的修复状态，建立回归核对表
3. 产出按 P0–P3 分级的缺陷清单（逻辑错误 / 竞态 / 资源泄漏 / 边界问题）
4. 核对 7 篇文档与代码实现的一致性（文档漂移）
5. 评估测试充分性（覆盖率、边界用例、竞态覆盖）
6. 输出可执行的修复建议清单（修复本身不在本计划内）

**审阅基线**：`make verify` 全绿为审阅起点，所有结论基于可复现验证命令。

## 二、审阅范围（8 层）

| # | 文档域 | 目录 | 生产文件 | 特征 |
|---|--------|------|----------|------|
| 1 | 架构与分层 | `tui/` 根包 + `LAYERS.md` + `doc.go` | 11 | 容器/事件循环/覆盖层/生命周期 |
| 2 | 终端兼容性 | `tui/terminal/` | 12 | 能力探测 / 按键解析 / 快捷键 |
| 3 | 主题系统 | `tui/theme/` | 15 | 语义主题/调色板/热重载/a11y |
| 4 | 渲染引擎 | `tui/core/` + `tui_render.go` + `overlay.go` | 17 | 差分算法/宽字符/覆盖层合成 |
| 5 | 组件系统 | `tui/component/` | 45 | Editor/Input/Markdown/表格/面板等 |
| 6 | 输入处理 | `tui_input.go` / `tui_loop.go` / `tui_focus.go` + `core/message.go` | 5 | 消息分发/焦点栈/鼠标节流/Filter |
| 7 | 应用层 | `tui/chat/` + `tui/agentadapter/` | 20 | ChatApp FSM/流式/事件转换 |
| 8 | 基础设施 | `tui/layout/` + `tui/internal/` | 4 | Flex/断点/csync |

> 排除项：第三方依赖、`go.sum`、纯测试辅助文件（`wait_helpers_test.go` 等仅审正确性）。

## 三、与既有基线的衔接：上次缺陷状态速查表

> 状态为 2026-08-09 初步验证结果；❓ 项须在阶段 0 实测复核。

### Critical（07-26 报告，5 项）

| # | 缺陷 | 位置 | 状态 | 说明 |
|---|------|------|------|------|
| C-1 | Kitty 协议全局状态污染测试 | `terminal_kitty_test.go` → `keys.go:328` | ❓ 疑似修复 | 测试已改用 `t.Setenv`，需 `-race -count=5` 实测 |
| C-2 | termios 恢复错误被静默吞掉 | `terminal/terminal.go:247` | ❓ 待验证 | `_ = setTermios(...)` 后 `return nil` |
| C-3 | `PanicMsg` 丢堆栈（`Stack` 恒空） | `core/message.go:189` | ❓ 待验证 | 与 `captureStack()` 一致性 |
| C-4 | **LLM 输出 ANSI 原样透传（注入链）** | `core/cellparse.go:45-52` → `tui_render.go` | ⚠️ **疑似未修复** | `hasUnrepresentableEscape` 回退后 Raw 原样写 stdout；本次审阅最高优先级 |
| C-5 | doc.go Quick Start 无法编译 | `tui/doc.go` | ❓ 待验证 | 无 `Example*` 测试函数，需编译实测 |

### High（07-26 报告，8 项）

| # | 缺陷 | 位置 | 状态 |
|---|------|------|------|
| H-1 | OnDebug 死代码（ctrl+shift+d 无效果） | `tui.go:188` | ✅ **已修复**（`chat_bridge.go:43` 已 wire `debug_overlay.go`） |
| H-2 | `errors.Is/As` 零使用，错误无分层 | 全局 | ❓ 待验证 |
| H-3 | Editor/Input 硬编码选中色 `48;5;33` | `editor_render.go:355`, `input.go:246` | ❓ 待验证 |
| H-4 | stdin 缓冲区未终止 CSI/OSC 无界增长 | `stdin_buffer.go` | ❓ 待验证 |
| H-5 | `NewTUI` 双重签名陷阱 | `tui.go:209,240` | ❓ 待验证 |
| H-6 | 245+ 处硬编码中文，零 i18n | `chat/`、`component/` | ❓ 待验证 |
| H-7 | Border/BorderMuted dark 主题同色 | `semantic_theme.go:128-129` | ✅ **已修复**（dark: `#2A4A63` vs `#152A3D`） |
| H-8 | verify_layers.sh 未接入 CI | `.github/workflows/` | ✅ **已修复**（Makefile `verify-tui-layers` + `ci.yml`） |

### Medium（15 项，抽查）

| # | 缺陷 | 状态 |
|---|------|------|
| M-1 | session_selector 回调 goroutine 无终止条件 | ❓ 待验证 |
| M-2 | stdio 层持锁 I/O | ❓ 待验证 |
| M-3 | theme/appearance watcher panic 后永久死亡 | ❓ 待验证 |
| M-4 | `tui`→`chat` 跨层依赖（L3→L5 架构债） | ❓ 待验证（`chat_bridge.go` 是否仍为唯一通道） |
| M-5 | ChatEvent 三重标识冗余 | ❓ 待验证（FSM 重构后状态） |
| M-6 | agentadapter 事件映射 12/17 无测试 | ❓ 待验证（`adapter_events_test.go` 是否补齐） |
| M-7 | 三套终端检测系统断裂（TerminalContext 死代码） | ✅ **已修复**（`clipboard.go`/`color_resolve.go` 已使用） |
| M-8 | Sixel 宣称支持但未实现 | ❓ 待验证（LAYERS.md 与实现一致性） |
| M-9 | `itoa` 3 处重复 | ❓ 待验证 |
| M-10 | 置信度条渲染器 3 处重复 | ❓ 待验证 |
| M-11 | theme 包 Godoc 缺失 | ❓ 待验证 |
| M-12 | `editor_edit.go` 7 个核心函数零覆盖 | ❓ 待验证（`editor_edit_test.go` 已存在） |
| M-13 | 8 个整文件零覆盖（~2470 行） | ❓ 待验证（覆盖率复测） |
| M-14 | 仅 2 个 benchmark | ❓ 待验证（`render_bench_test.go` 存在） |
| M-15 | LAYERS.md 依赖矩阵错误 | ✅ **已修复**（07-26 后 LAYERS.md 多次同步，`f96d0aa`） |

## 四、七个审阅维度（对应 7 篇 repowiki 文档）

### 维度 1 — 架构与分层（TUI架构设计.md）
- **对象**：`tui.go` / `tui_loop.go` / `tui_lifecycle.go` / `LAYERS.md` / `doc.go` / `chat_bridge.go`
- **重点**：分层单向依赖；`chat → tui` 循环依赖是否仅经 `AppHost` 接口；Start/Stop 恢复顺序（先禁 mouse/alt-screen 再 `term.Stop`）；`sendMsgSafe` 僵尸消息防护；watchdog 阈值；`chat_bridge.go` 桥接层边界
- **验证**：`bash tui/scripts/verify_layers.sh`、`make check-arch`、`go vet`

### 维度 2 — 终端 I/O 与兼容性（终端兼容性.md）
- **对象**：`terminal/` 12 个生产文件
- **重点**：能力门控（品牌白/黑名单、tmux/screen 乘数器、VTE 版本阈值）；Kitty 协议协商与 alt-screen 切换后 `PushKittyKeyboard` 状态保持；CSI u 解析（修饰键/关联文本/百分号编码）；用户覆盖 JSON 校验与冲突检测；darwin/linux termios 与非平台回退；**C-2（termios 吞错）与 C-4（ANSI 注入）复核**
- **验证**：`go test -race -count=5 ./terminal/...`（C-1 实测）

### 维度 3 — 主题系统（主题系统.md）
- **对象**：`theme/` 15 个生产文件
- **重点**：JSON 变量引用环与回退；`atomic.Pointer` 读路径零锁；hex/256/真彩映射与 `RGBTo256` 最近邻；`watch.go` 轮询 goroutine 退避与退出路径（泄漏）；`CurrentPalette` 懒加载竞态；`a11y_themes.go` 覆盖完整性；**H-3 硬编码色复核**
- **验证**：`go test -race ./theme/...`

### 维度 4 — 渲染引擎（渲染引擎.md）
- **对象**：`core/cell*.go` / `celldiff.go` / `width.go` + `tui_render.go` + `overlay.go`
- **重点**：`DiffCells` 宽字符不拆分与 `ClearTail`；`diffCellPool` 归还路径；raw 行复用正确性；`normalizeLine` 超宽截断；overlay 的 `spliceOverlayRows` / `clearWideBoundary` / `TranslateMouse` 坐标一致性；`DimBackground` 与 Raw 行混用；**C-4 注入链复核（Raw 行清洗缺失）**
- **验证**：`go test -race ./core/...`、`go test -bench`（对照 07-26 dimension3 基线）

### 维度 5 — 组件系统（组件系统.md）
- **对象**：`component/` 45 个生产文件
- **重点**：接口契约执行（`Render` 并发安全、`Update` 非阻塞、`Dispose` 单次）；高危组件专项——Editor（软换行/undo/IME 光标）、Input（滚动/选中）、Markdown 四文件（解析/渲染/宽度）、表格、viewport、todo_panel、tool_card；**M-12/M-13 覆盖复核**
- **验证**：`go test -race ./component/...`、按组件走查 Dispose 路径

### 维度 6 — 输入处理与事件循环（输入处理.md）
- **对象**：`tui_input.go` / `tui_loop.go` / `tui_focus.go` / `core/message.go`
- **重点**：消息分发顺序（特殊 → 焦点 → 广播 → MouseConsumer 终止）；鼠标节流 ~30fps 与 `pendingMotion` 拖拽终点；resize 100ms 防抖；`Filter` 拦截语义；`execCmdIndexed` panic 保护；focus 栈 Tab 循环；**C-3 PanicMsg 堆栈复核**
- **验证**：`go test -race .`（tui 根包）

### 维度 7 — 应用层（TUI终端界面.md 综合）
- **对象**：`chat/` 20 个生产文件 + `agentadapter/` 3 个生产文件
- **重点**：ChatApp 显式状态机（`state.go`）迁移完整性；流式渲染管线（`chat_app_stream.go`）与去重/对账；`chat_history_*` 渲染与滚动；agentcore → chat 事件转换覆盖；**M-5/M-6 复核**
- **验证**：`go test -race ./chat/... ./agentadapter/...`

## 五、历史高发区专项（git log 提炼，优先审）

| 热点 | 相关提交 | 专项审阅点 |
|------|----------|-----------|
| 流式输出完整性 | `7b2639f`（四层根因）、`cd7e125`（look-behind 重叠去重）、`32af5d9` | delta 去重算法边界、对账状态、截断提示触发条件 |
| FSM 重构 | `80f5a30`（FSM 全面接管） | 状态迁移覆盖、非法迁移防护、`state_test.go` 充分性 |
| Markdown 渲染 | `f7b1fbb`、`0919ac8`、`e0b123b` | 列表/表格/标题边界、CJK 排版、emoji 无空格标题 |
| 宽度/宽字符 | `32af5d9`、`a46b816`、`e0b123b` | 宽字符钳制、滚动条恒定列预留 |
| 竞态 | `32af5d9` | `-race` 全绿基础上走查共享状态（editor lastVisuals、palette、流式缓冲） |

## 六、方法与工具链

- **静态**：`cd tui && go vet ./...`、`golangci-lint run ./...`、`go build ./...`
- **动态**：`cd tui && go test -race ./...`（关键包加 `-count=5`）、`go test -short`
- **覆盖率**：`go test -coverprofile` 复测，对照 M-13/M-14
- **文档一致性**：`make doc-check`、`bash tui/scripts/verify_layers.sh`、`python3 scripts/check-doc-consistency.py`、7 篇 repowiki 文档交叉核对
- **辅助**：codegraph 符号级定位；grep 遗留标记（当前非测试 9 处，需逐个判定）
- **执行方式**：维度 2–7 并行派发只读走查子代理，主代理汇总分级；高危结论人工复核

## 七、分阶段执行计划

| 阶段 | 内容 | 产出物 | 验收 |
|------|------|--------|------|
| 0 | 基线验证 + 上次缺陷状态复核（速查表 ❓ 项实测） | `make verify` 结果、更新后的速查表 | lint/build/test-race 全绿；28 项状态定性 |
| 1 | 架构分层 + 死代码 + 文档一致性 | 分层违规清单、verify_layers/doc-check 结果 | 无违规或已记录 |
| 2–3 | terminal / theme 走查 | 每层审阅记录（P0–P3 缺陷表） | 每层验证命令输出附档 |
| 4 | core 渲染 + overlay 走查（含 C-4 注入链专项） | 同上 + 注入链风险评估 | race 全绿 |
| 5 | component 走查（45 文件分批，高危组件专项） | 同上 | 每批验证命令附档 |
| 6 | input/loop/focus 走查 | 同上 | race 全绿 |
| 7 | chat + agentadapter 走查 | 同上 | race 全绿 |
| 8 | 历史高发区专项（五热点） | 专项审阅报告 | 每热点验证命令附档 |
| 9 | 汇总分级 | **审阅总报告**：缺陷汇总表、文档漂移清单、修复建议、测试缺口清单 | 缺陷可复现、分级明确 |

## 八、缺陷分级标准

- **P0**：崩溃 / 数据丢失 / 死锁 / 安全边界突破（含 C-4 ANSI 注入链若仍成立）
- **P1**：功能错误（渲染错乱、流式丢字、焦点错乱、覆盖层坐标错误）
- **P2**：边界缺陷 / 性能 / 资源泄漏 / 竞态隐患
- **P3**：文档漂移 / 代码风格 / 可维护性

## 九、产出物与验收

1. `docs/review/tui-full-review-plan-2026-08-09.md`（本计划，执行时勾选完成）
2. 阶段 0 更新的「上次缺陷状态速查表」（28 项全定性）
3. 每层审阅记录（含验证命令输出）
4. **审阅总报告**（独立文件，命名 `tui-full-audit-2026-08-09.md`）：
   缺陷汇总表（P0–P3）、文档漂移清单、修复建议（含优先级）、测试缺口清单

**验收**：所有 P0/P1 结论可复现；修复建议与缺陷一一对应；文档漂移项标注「文档待同步」。

## 十、风险与约定

1. **多模块 gotcha**：TUI 是独立子模块，命令须 `cd tui && ...`；根目录 `go test ./...` 不覆盖
2. **敏感路径**：TUI 不在 `check-sensitive-paths.sh` 清单内，但 `agentadapter` 与 agentcore
   （含 `tools/path.go` 沙箱）交互，涉边界结论须人工复核
3. **变更即记录**：审阅若落地修复，须经 `scripts/changelog/main.go` 追加条目，
   且遵守 pre-commit（gofmt/goimports/vet/commitlint）
4. **修复不并入审阅**：本计划只产出缺陷与建议；修复作为后续独立任务，遵循 3–5 文件粒度
5. **文档漂移处理**：发现即标注 P3「文档待同步」，不在审阅中直接改文档
6. **执行预算**：维度 2–7 可并行（每层一路只读子代理），阶段 0/1/8/9 串行
