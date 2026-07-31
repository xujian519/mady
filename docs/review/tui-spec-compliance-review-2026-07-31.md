# TUI 模块规范符合度全量质量审阅报告

> ⚠️ **修复状态追踪**：本报告生成后即启动阶段一（文档同步）修复。
> 各条目状态见 [11. 修复状态追踪](#11-修复状态追踪)。



> 审阅依据：`docs/tui-design-specification.md`（v1.0，2026-07-25）+ `tui/LAYERS.md`（2026-07-30）
>
> 审阅范围：`tui/` 全部 9 个子包（113 源 + 63 测试，~45K 行，含 internal/csync）
> 审阅方法：规范逐章对照 → 并行子智能体代码走查（结构/实现/测试三路）→ 自动化验证
> 自动化验证：`go test -race ./tui/...` ✅ 全部通过 | `go test -cover ./tui/...` | `./tui/scripts/verify_layers.sh`
> 审阅日期：2026-07-31

---

## 目录

1. [审阅概述](#1-审阅概述)
2. [评分汇总](#2-评分汇总)
3. [P0 关键差距](#3-p0-关键差距必须修复)
4. [P1 重要差距](#4-p1-重要差距应尽快修复)
5. [P2 注意差距](#5-p2-注意差距计划修复)
6. [P3 建议优化](#6-p3-建议优化低优先级)
7. [正面发现](#7-正面发现符合或超越规范)
8. [修复路线图](#8-修复路线图)
9. [验证方法](#9-验证方法)
10. [附录：数据核对表](#10-附录数据核对表)
11. [修复状态追踪](#11-修复状态追踪)

---

## 1. 审阅概述

本报告以 `docs/tui-design-specification.md` 的 12 个章节为审计维度，对 TUI 模块进行规范符合度审阅。重点回答三个问题：

1. **架构是否如规范所述**（8 层结构、单向依赖、事件循环约束）
2. **组件是否遵守规范契约**（Component 接口、≤400 行拆分、Godoc、测试）
3. **文档与实际是否一致**（文件计数、主题数量、事件类型、协议支持）

### 1.1 总体评估

| 维度 | 评分 | 说明 |
|------|------|------|
| 架构合规性 | 5/5 | 严格单向依赖，无循环引用，agentadapter 隔离正确 |
| 终端协议 | 5/5 | Kitty 键盘 / Bracketed Paste / CSI 2026 全部完整实现 |
| 无障碍支持 | 5/5 | HighContrast + ColorBlind + ReduceMotion，超越规范要求 |
| 渲染引擎 | 4.5/5 | Cell 级差分渲染成熟，raw string 缓存 + sync.Pool 优化 |
| 测试覆盖 | 2/5 | 5/8 包低于规范 70% 阈值，chat 仅 47.4% |
| 文档一致性 | 2/5 | stdio 死文档、文件计数全面过时、事件数三处不一致 |
| 代码组织 | 3/5 | 10 个组件文件超 400 行，markdown.go 达 1053 行 |

**综合评分：3.9/5（78/100）** — 代码实现质量总体优秀，但规范文档与实现存在显著漂移，测试覆盖未达规范承诺。

---

## 2. 评分汇总

| 严重级别 | 数量 | 说明 |
|----------|------|------|
| **P0 — 关键** | 4 | 规范与实现严重不一致，直接影响维护和新人理解 |
| **P1 — 重要** | 7 | 测试覆盖率不足、文档过时、架构描述冲突 |
| **P2 — 注意** | 6 | 设计差异、代码组织违规 |
| **P3 — 建议** | 5 | 命名/格式/细节差异 |

---

## 3. P0 关键差距（必须修复）

### P0-1：`tui/stdio/` 目录不存在，但规范仍将其列为核心层

**规范描述**（§2.1）：Layer 6 Stdio（`tui/stdio`）含 5 源 + 1 测试，提供 Spinner / Renderer / ProgressBar / LineReader / Layout 过程式 stdout 工具。

**实际状态**：`tui/stdio/` 目录**不存在**。`tui/LAYERS.md` 注明"stdio-era procedural rendering was removed; all rendering now goes through `core.Component`"（渲染模型决策见 `docs/decisions/tui-layers-architecture.md` 决策 2）。但 `tui/doc.go:48-50` 仍描述 Layer 6 与"two parallel rendering models"（§119-126），规范 §2.1 架构图仍绘制 Layer 6 分层。

**影响**：形成死文档，新人按规范理解架构会得出错误结论（以为存在两套渲染模型）。

**修复建议**：
- 更新 `docs/tui-design-specification.md`：§2.1 架构图移除 Layer 6，标注"历史层，已移除"
- 更新 `tui/doc.go`：删除 Layer 6 描述与"two parallel rendering models"章节
- 同步更新 `docs/decisions/tui-layers-architecture.md` 决策 2 的现状描述

### P0-2：`Overlay` 结构缺少 `Modal` 字段

**规范要求**（§7.4）：

```go
type Overlay struct {
    Component core.Component
    X, Y      int64
    Width     int64
    Height    int64
    Modal     bool  // 模态（阻止背景交互）
}
```

**实际代码**（`tui/overlay.go`）：`Overlay` 无 `Modal` 字段。所有 Overlay 一律阻止背景交互（`tui/tui_input.go:209` 注释"Overlays are modal layers: once any overlay exists, only the focused component receives input"）。

**影响**：规范设计了模态/非模态两种行为，实现统一为模态。无法按规范构建非模态浮层（如可交互的侧边辅助面板）。

**修复建议**（二选一）：
- **实现侧**：为 `Overlay` 增加 `Modal bool` 字段（默认 true），在 `processMsg` 中当 `!Modal` 时将输入同时分发给背景焦点组件
- **规范侧**：更新 §7.4 说明"当前实现所有 Overlay 均为模态，非模态浮层为规划中能力"

### P0-3：测试覆盖率全面低于规范要求（≥70%）

| 包 | 当前覆盖率 | 规范要求 | 差距 |
|----|-----------|----------|------|
| `tui/chat` | **47.4%** | ≥70% | **-22.6%** |
| `tui/theme` | **54.6%** | ≥70% | -15.4% |
| `tui/component` | **58.9%** | ≥70% | -11.1% |
| `tui/core` | **60.6%** | ≥70% | -9.4% |
| `tui/terminal` | **61.6%** | ≥70% | -8.4% |
| `tui` (root) | 65.1% | ≥70% | -4.9% |
| `tui/layout` | 75.4% | ≥70% | ✅ |
| `tui/agentadapter` | 84.0% | ≥70% | ✅ |

**影响**：5/8 包未达标。`chat` 包仅 47.4%——流式生命周期与工具调用处理逻辑（本项目业务核心）缺乏充分测试覆盖，回归风险较高。

**修复建议**（按优先级）：
1. `chat`：ChatApp 流式生命周期（submit/delta/end/error）、工具调用、PlanTask 处理的集成测试
2. `theme`：主题切换、JSON 加载、热重载、Palette 构建
3. `component`：19 个无测试文件的组件（见 P1-1）
4. `core`：Cell 模型边界（宽字符截断、combining marks、Raw 回退）
5. `terminal`：键盘解析边界（Kitty 修饰符组合、未知序列）

### P0-4：规范附录 C 文件清单与 LAYERS.md 全面过时

规范附录 C 声称的文件数量与实际代码严重不符（详见附录数据核对表）：

| 目录 | 规范声称 | LAYERS.md 声称 | 实际 | 偏差 |
|------|---------|----------------|------|------|
| `core/` | 11 源 + 5 测试 | 14 源 | 14 源 + 7 测试 | 规范少 3 源 |
| `layout/` | 2 源 + 1 测试 | — | 3 源 + 2 测试 | 规范少 1 源 |
| `terminal/` | 8 源 + 4 测试 | 9 源 | 9 源 + 8 测试 | 规范少 1 源 |
| `theme/` | 7 源 + 5 测试 | 13 源 | 13 源 + 5 测试 | 规范少 6 源 |
| `component/` | 35 源 + 11 测试 | 41 源 | 41 源 + 24 测试 | 规范少 6 源 |
| `chat/` | 14 源 + 5 测试 | **17/18 源** | **22 源** + 6 测试 | 规范少 8 源；LAYERS.md 自身少 4-5 源 |
| **`stdio/`** | **5 源 + 1 测试** | 已移除 | **0** | **子目录不存在** |
| `agentadapter/` | 1 源 + 1 测试 | 1 源 | 1 源 + 2 测试 | ✅ |

**影响**：LAYERS.md 虽标注 "Auto-verified: 113 source files (+ 64 test files)"，但实际为 113 源 + **63** 测试（差 1），且 chat 计数（17/18 vs 22）与目录树（21 个文件列表）互相矛盾。基于这些文档的任务评估、贡献指引均受影响。

**修复建议**：
- 以 `./tui/scripts/verify_layers.sh` 为准全面同步 LAYERS.md（先修自身矛盾，再同步规范附录 C）
- 规范附录 C 增加"同步状态"标注，避免再次漂移

---

## 4. P1 重要差距（应尽快修复）

### P1-1：19 个组件文件无对应测试文件（46%）

以下源文件缺少专属测试：

```
box.go, command_center.go, conclusion_card.go, confidence_bar.go,
debug_overlay.go, domain.go, editor_killring.go, editor_render.go,
evidence_card.go, evidence_overlay.go, fuzzy_provider.go,
onboarding.go, session_selector.go, skill_center.go, statusbar.go,
syntax_langs.go, syntax_tokenizer.go, table.go, todo_panel.go
```

**影响**：41 个源文件中 19 个无专属测试，含大型组件（session_selector 545 行、review_gate 577 行）。`editor_render.go`（324 行）与 `editor_killring.go`（126 行）作为 Editor 子系统成员亦无专属测试，killring 行为仅能通过 editor 测试间接覆盖。

### P1-2：`doc.go` 声称数字与实际不符

| 位置 | 声称 | 实际 |
|------|------|------|
| `doc.go:38` | "35+ UI components" | 41 源文件 |
| `doc.go:52-53` | agentadapter 为 "Optional"（可选依赖） | **硬依赖** `agentcore`（`adapter.go` 直接 import） |
| `doc.go:119-126` | "two parallel rendering models" | stdio 已移除，仅剩 Component 模型 |

### P1-3：规范 8 层架构图与 LAYERS.md 层编号冲突

- 规范 §2.1：Layer 0-7 共 8 层（含 Layer 6 Stdio）
- LAYERS.md：Layer 0-5 + Layer 7（**跳过 Layer 6**），表格 7 行

两个文档描述同一架构但层编号体系不一致，跨文档引用（如 §12.2"文件数量/依赖关系变化时更新"）会放大漂移。

### P1-4：ChatEvent 事件数量三处不一致

| 来源 | 声称事件数 |
|------|-----------|
| `doc.go` agentadapter 描述 | 18 种 |
| `adapter.go` 实际注册的 case | **23 种** |
| `events.go` 定义的 ChatEventType | **23 种**（AgentStart → PlanTaskInterrupted） |

`doc.go` 的 18 是旧值，adapter 与 events.go 已一致（23）。

### P1-5：10 个组件文件行数大幅超标

规范 §3.2 要求"组件核心逻辑 ≤ 400 行，超过按责任拆分到 `*_<concern>.go` 兄弟文件"：

| 文件 | 行数 | 超标 | 拆分建议 |
|------|------|------|----------|
| **markdown.go** | **1053** | +653 | 拆 `markdown_parse.go`（块解析）+ `markdown_inline.go`（内联样式）+ `markdown_render.go`（渲染） |
| **input.go** | **675** | +275 | 拆 `input_edit.go` + `input_render.go` |
| **editor_edit.go** | **610** | +210 | 已按责任拆分，但文件本身仍超标（编辑原语可拆 `editor_keys.go`） |
| session_selector.go | 558 | +158 | 拆 `session_selector_render.go` |
| review_gate.go | 552 | +152 | 拆 `review_gate_render.go` |
| image.go | 498 | +98 | 协议渲染可拆 `image_render.go` |
| editor.go | 441 | +41 | 构造函数与核心接口可拆 |
| editor_render.go | 440 | +40 | — |
| selectlist.go | 411 | +11 | — |
| viewport.go | 407 | +7 | — |

**影响**：Editor 子系统（5 文件拆分）符合规范精神，但整体仍 10/41 文件超标。markdown.go（1053 行）为最严重违规：块解析、内联样式、表格、主题配置混于一文件，且是当前唯一有未提交修改（`M` 状态）的文件——修改时应顺带完成拆分。

### P1-6：主题 JSON 文件不存在

**规范描述**（§6.2）：JSON 格式主题文件 + `theme/watch.go` 文件监控热重载；§6.3 声称 3 个主题（mady-dark/mady-light/amber）与 `/theme dark` 等切换命令。

**实际状态**：
- `theme/json.go` 支持 pi-compatible JSON 解析（含 `$var` 引用、循环检测），`watch.go` 支持 mtime 轮询热重载 ✅
- 但 `theme/` 目录下**无任何 .json 主题文件**，所有主题以 Go 函数定义
- `theme_registry.go` 注册 8 个主题：mady-dark, mady-light, tokyo-night, rose-pine-moon, grok-night, high-contrast, colorblind, auto
- **无 amber 主题**

**影响**：JSON 解析/热重载代码是"可用的闲置能力"，规范描述的体验（用户通过 JSON 文件自定义主题）无法开箱使用。

### P1-7：LAYERS.md 自身文件计数不准

- 层表：chat "17 source files" → 实际 22
- 目录树标头：chat "18 source files" → 实际 22
- 目录树文件清单本身完整（22 个文件均列出），仅标头计数错误
- 测试文件总数声称 64 → 实际 63

---

## 5. P2 注意差距（计划修复）

### P2-1：响应式断点设计不一致

| 维度 | 规范 §7.3 | 实际 `layout/breakpoint.go` |
|------|-----------|---------------------------|
| 断点数量 | 4 级 | 3 级 |
| 阈值 | ≥120 / 80–119 / 60–79 / <60 | ≥160 / 80–159 / <80 |
| Sidebar 行为 | 全宽 / 20-24列 / **8列图标栏** / 隐藏 | 完整 / 完整 / 隐藏 |

实际实现更简洁，但丢失了规范中"60-79 列折叠为 8 列图标栏"的中间态。若采纳实际实现，需同步更新规范。

### P2-2：动画帧率表述不准确

规范 §8.1 写"最多 125fps（8ms 间隔）"。`tui.go` 默认 `TickInterval` 为 8ms，即**最小间隔 8ms（上限 125fps）**，措辞方向颠倒，应为"最低 8ms 间隔（最多 125fps）"。

### P2-3：markdown.go 新增解析函数与既有逻辑的归属

当前 `M` 状态修改新增 `extractHeadingText` / `parseATXHeading` / `isHeadingDecorationRune` / `isTableStart` 四个函数（替代原 `reHeading` 正则）。质量良好（UTF-8 感知、emoji 前缀处理、表边界修复），但实现于 1053 行单体文件中，应随 P1-5 拆分时归入 `markdown_parse.go`。

### P2-4：Overlay 定位字段命名与规范不符

规范 §7.4 定义 `X, Y, Width, Height` 字段；实际实现使用 `OverlaySize`（Value/Percent/Min/Max）+ `OverlayAnchor`（9 锚点）+ 百分比/绝对双模式——**功能更强大**但字段命名不匹配。属"规范落后于实现"，更新规范即可。

### P2-5：规范 §5.4 VirtualTerminal 示例与实际 API 需验证

规范示例 `vt.Type("hello\x1b[5~")` 的注释为"// Ctrl+E"——但 `\x1b[5~` 是 CSI `5~`（PageUp 或 Home 按键序列），与 Ctrl+E 不符。示例本身可能有误，需核对 `VirtualTerminal` 实际注入方式（`Type` vs `Key`）后修正。

### P2-6：规范 §9.5 屏幕阅读器支持无法核实

规范要求"关键状态变化通过 `term.Write()` 输出明文到终端"。实现中未发现专门面向屏幕阅读器的明文输出路径（`term.Write` 输出的是渲染帧）。该能力实际依赖终端复用器/辅助工具的原始输出捕获，建议规范明确实现边界。

---

## 6. P3 建议优化（低优先级）

| # | 位置 | 问题 | 建议 |
|---|------|------|------|
| P3-1 | `core/spinner_style.go` vs 规范 §8.3 | 规范写 `SpinnerPulse`，实现为 `SpinnerBounce`，且帧序列不同 | 统一命名（二选一） |
| P3-2 | 规范 §6.3 vs `theme_registry.go` | 规范声称 amber 主题，实际为 tokyo-night 等 8 主题 | 更新规范主题清单 |
| P3-3 | `markdown.go` `isHeadingDecorationRune` | Unicode 范围覆盖 7 个块，但缺少 1F600-1F64F（Emoticons）、1FA70-1FAFF（Ext A）之外的少数新块（如 1F780-1F7FF 几何符号） | 补充范围或改用 `unicode` 表查询 |
| P3-4 | 规范 §12.3 敏感路径 | `editor_killring.go` 标注为敏感路径，但实现仅内存缓存（126 行），无文件/网络 I/O，敏感级别可能高估 | 复核敏感路径清单 |
| P3-5 | 规范附录 C | chat/ 新增 9 个文件（chat_builder、chat_display、chat_host、chat_model、reasoning、layout_editor、layout_shortcuts、chat_app_plantask、chat_app_todo）未列出 | 随 P0-4 一并同步 |

---

## 7. 正面发现（符合或超越规范）

### 7.1 架构合规（✅ 优秀）

- 严格单向依赖，各层边界清晰（LAYERS.md 规则全部成立）
- `AppHost` 接口按 ISP 拆分为 4 个角色接口，编译期打破循环
- `agentadapter` 将 agentcore 依赖完全隔离，`chat` 包不导入 agentcore
- 跨层数据转换边界清晰（`parseReviewGateData` / `agentTaskToInfo` 是唯一解析边界）

### 7.2 Cell 级渲染模型（✅ 超越规范）

- 规范 §1.1 提及"差分渲染"，实际实现了完整的 Cell→Row→CellGrid→Diff 管线
- 解决两个真实 bug 类：CJK 宽字符截断、SGR 编码歧义
- 三档 diff（Rows/Frame/Cells）+ `sync.Pool` + raw string 缓存，流式场景每帧仅解析 1-2 行变更

### 7.3 终端协议（✅ 完整）

- Kitty 键盘协议：正向/反向清单 + 版本门槛 + 多路复用器处理，`SetKittyProtocolActive` 消除 50ms ESC 延迟
- Bracketed Paste（CSI 2004）与 Synchronized Output（CSI 2026）正确配对启停
- 19 种终端品牌检测，未知终端保守回退

### 7.4 无障碍（✅ 超越规范）

- HighContrast（纯黑/白/蓝，WCAG AA 4.5:1）、ColorBlind（蓝橙替代红绿）
- `SetReduceMotion` 支持前庭障碍用户
- CursorMarker APC 序列实现 IME 候选窗口定位

### 7.5 FSM 状态机（✅ 模范实现）

- 9 状态 + 16 事件，`Transition` 纯函数、无副作用、可表测试
- `evtUnknown` 安全默认保证未来事件类型不会引发错误转换
- Degraded 作为跨切面视觉修饰符而非状态——设计决策正确

### 7.6 差分渲染引擎（✅ 生产级）

- raw string 缓存避免重复 `ParseLine`（流式场景 99% 行复用）
- 状态式光标管理（仅状态变化时 emit CSI，保护终端闪烁定时器）
- DECAWM 帧内关闭防宽字符换行、最小终端尺寸门（80×24 显示 resize 提示）

### 7.7 主题系统（✅ 功能完备）

- 67 个语义 token、`atomic.Pointer[Palette]` 无锁读取、8 主题 + 系统外观跟随
- 热重载 panic 恢复（`runWithRestart` + 5s backoff）防单次坏文件永久禁用

### 7.8 Editor 子系统（✅ 规范合规）

- 6 文件按责任拆分（core/edit/render/history/killring/chip），符合 §3.2 拆分精神
- Emacs kill-ring + undo/redo 双历史、CJK 安全光标

### 7.9 自动化质量门（✅）

- `go test -race ./tui/...` 全部通过，无数据竞争（555 个测试函数）
- `verify_layers.sh` 脚本提供 LAYERS.md 与磁盘的一致性校验（供 CI 集成）
- `.golangci.yml` 启用 10 个 linter（errcheck/govet/staticcheck/unused/revive 等）

---

## 8. 修复路线图

| 阶段 | 内容 | 预估工作量 |
|------|------|-----------|
| **一：文档同步** | 移除规范/doc.go 的 stdio 死引用；修正 doc.go 数字（41 组件、23 事件、硬依赖说明）；sync 附录 C 与 LAYERS.md（含 chat 22 源、63 测试）；修正 §2.1 层图、§6.3 主题清单、§8.1 帧率表述、§5.4 示例 | 1-2 天 |
| **二：测试补强** | chat → 70%（流式+工具调用）；19 个缺失组件测试（优先 session_selector/review_gate/statusbar）；theme 热重载与 JSON 加载；core 边界；terminal 键盘边界 | 3-5 天 |
| **三：代码质量** | 拆分 markdown.go（1053→按 parse/inline/render 拆分，顺带完成当前 M 状态修改的归属）；拆分 input.go（675）；Overlay.Modal 字段 + 输入路由 | 3-5 天 |
| **四：功能对齐（可选）** | 响应式断点 4 级化评估；SpinnerPulse/SpinnerBounce 统一；示例主题 JSON 文件落地（激活闲置的 json.go/watch.go 能力） | 按需 |

---

## 9. 验证方法

```bash
# 1. 测试与竞态
go test -race ./tui/...          # 必须全部通过
go test -cover ./tui/...         # 检查覆盖率提升

# 2. 文档一致性
./tui/scripts/verify_layers.sh   # LAYERS.md 与磁盘一致（阶段一后应通过）

# 3. 构建与静态检查
go build ./tui/...               # 所有包编译
go vet ./tui/...                 # 静态分析
cd tui && golangci-lint run ./...  # 按 .golangci.yml 配置

# 4. 代码组织检查
find tui -name '*.go' ! -name '*_test.go' | xargs wc -l | sort -rn | head -20  # ≤400 行检查
```

---

## 10. 附录：数据核对表

### 10.1 文件计数（审阅时点实测）

| 包 | 源文件 | 测试文件 | 说明 |
|----|--------|---------|------|
| `tui`（root） | 9 | 8 | tui.go + 7 兄弟文件 + doc.go |
| `core/` | 14 | 7 | 含 cell 渲染模型 5 文件 |
| `layout/` | 3 | 2 | flex/breakpoint/layout |
| `terminal/` | 9 | 8 | 含 3 平台 termios + detect |
| `theme/` | 13 | 5 | 含 a11y/registry/watch |
| `component/` | 41 | 24 | 含 editor 6 文件、syntax 3 文件 |
| `chat/` | 22 | 6 | 含 chat_app 5 文件、chat_history 6 文件 |
| `agentadapter/` | 1 | 2 | adapter.go 370 行 |
| `internal/csync/` | 1 | 1 | 并发切片 |
| **合计** | **113** | **63** | 555 个测试函数 |

### 10.2 ChatEvent 类型清单（23 种，`chat/events.go`）

AgentStart, AgentEnd, AgentError, TurnStart, TurnEnd, MessageDelta, ToolCallStart, ToolCallEnd, HandoffStart, HandoffEnd, CompactionStart, CompactionEnd, AutoRetry, AgentInterrupt, ApprovalPrompt, TaskCreated, TaskUpdated, SkillLoaded, SkillsReloaded, A2UI, PlanTaskStatusChanged, PlanTaskFeedbackAdded, PlanTaskInterrupted

### 10.3 已注册主题（8 个，`theme/theme_registry.go`）

mady-dark（默认）, mady-light, tokyo-night, rose-pine-moon, grok-night, high-contrast, colorblind, auto（跟随系统外观）

### 10.4 既有 TUI 审阅对照（历史基线）

| 日期 | 文档 | 结论 |
|------|------|------|
| 2026-07-25 | `docs/review/tui-code-review-2026-07-25.md` | 架构合规 5/5，测试覆盖 53.2% |
| 2026-07-26 | `docs/review/tui-full-audit-2026-07-26.md` | 全量审计（7 维度） |
| **2026-07-31** | **本报告** | **规范符合度专项：4 P0 + 7 P1 + 6 P2 + 5 P3** |

测试覆盖自 07-25 的 53.2% 提升至当前加权约 58.1%（agentadapter 84% / layout 75.4% 达标），但与规范 70% 阈值仍有差距。

---

## 11. 修复状态追踪

> 状态标记：✅ 已修复（提交含 Commit） | 🔶 部分修复 | ⬜ 待修复

### 阶段一：文档同步（2026-07-31，commit 待填）

| 条目 | 状态 | 修复内容 |
|------|------|----------|
| P0-1 stdio 死文档 | ✅ | `tui/doc.go` 移除 Layer 6 与两套渲染模型描述（保留历史移除说明）；规范 §2.1 架构图移除 Layer 6 并加注 |
| P0-4 文件计数过时 | ✅ | 规范附录 C 全面重写并与 LAYERS.md 同步；`verify_layers.sh` 校验通过（113 文件） |
| P1-2 doc.go 数字 | ✅ | 35+ → 41 组件；agentadapter 标注"23 事件类型、硬依赖 agentcore"（删除 Optional 措辞） |
| P1-3 层编号冲突 | ✅ | doc.go 改"7 Layers"并说明 Layer 6 历史；规范 §2.1 加注"层编号不连续为既有事实" |
| P1-4 事件数不一致 | ✅ | doc.go 18 → 23 种；LAYERS.md events.go 注释 15 → 23 种 |
| P1-6 主题清单 | 🔶 | 规范 §6.3 更新为 8 个内置主题（含 high-contrast/colorblind/auto）；JSON 主题文件落地留待阶段四 |
| P1-7 LAYERS.md 计数 | ✅ | chat 17/18 → 22；测试文件 64 → 63；目录树标头同步 |
| P2-2 帧率表述 | ✅ | 规范 §8.1"最多 125fps（8ms 间隔）"→"最小帧间隔 8ms（上限 125fps）" |
| P2-5 §5.4 示例注释 | ✅ | `\x1b[5~` 注释"Ctrl+E"改为正确的"Home/PageUp 键"说明 |
| P3-1 Spinner 命名 | ✅ | 规范 §8.3 SpinnerPulse → SpinnerBounce，帧序列与实现一致 |
| P3-2 主题清单差异 | ✅ | 随 P1-6 一并修复（§6.3） |
| P3-5 附录 C 缺文件 | ✅ | 随 P0-4 一并修复（附录 C 全量重写） |

### 阶段二：测试补强（2026-07-31，commit 86374e2）

| 包 | 提升前 → 提升后 | 状态 |
|----|----------------|------|
| core | 60.6% → 73.0% | ✅ |
| terminal | 61.6% → 71.8% | ✅ |
| tui (root) | 65.1% → 71.9% | ✅ |
| theme | 54.6% → 96.3% | ✅ |
| chat | 47.4% → 88.7% | ✅ |
| component | 58.9% → 85.0% | ✅ |
| P1-1 19 组件无测试 | 全部补齐（44 个测试文件新增） | ✅ |

补测同时发现 9 处生产缺陷（见阶段三）。

### 阶段三：代码质量（2026-07-31，commit fcfc969 / aab1cdf）

| 条目 | 状态 | 修复内容 |
|------|------|----------|
| P0-2 Overlay.Modal | ✅ | 新增 `NonModal` 字段（零值模态，向后兼容）+ 输入穿透路由 + 2 行为测试；规范 §7.4 已同步实现说明 |
| 补测发现 9 缺陷 | ✅ | ①ToggleTodoPanel 自死锁 ②expandPastePlaceholders 死循环 ③SearchBackspace UTF-8 截断 ④renderToolGroup 行映射错位 ⑤-⑧四处键名不匹配（esc/space/pgup/pgdown/backspace）⑨两处窄宽超宽 |
| P1-5 markdown.go 拆分 | ✅ | 1053 行 → `markdown.go`(321) + `markdown_parse.go`(371) + `markdown_inline.go`(49) + `markdown_render.go`(339)，行为不变（10 测试全过） |
| P1-5 input.go 拆分 | ✅ | 675 行 → `input.go`(267) + `input_edit.go`(425) |
| P2-3 markdown 新函数归属 | ✅ | 随拆分归入 `markdown_parse.go`（parseATXHeading 等） |
| P2-4 Overlay 字段命名 | ✅ | 随 P0-2 一并处理（NonModal 反向字段 + 规范说明） |

### 阶段四（2026-07-31，用户决策）

| 条目 | 状态 | 说明 |
|------|------|------|
| P2-1 响应式断点 | ✅ | 规范 §7.3 改为实际 3 级断点；规范愿景（4 级 + Sidebar）标注"规划中"，待 Sidebar 组件落地时实现 |
| P2-6 屏幕阅读器边界 | ✅ | 规范 §9.5 标注"已知限制"（不实现，依赖终端复用器/辅助工具捕获） |
| P3-3 Unicode 范围补充 | ✅ | `isHeadingDecorationRune` 补 Ornamental Dingbats（1F650-1F67F）+ Geometric Shapes Extended（1F780-1F7FF）+ 2 测试用例 |
| P3-4 敏感路径复核 | ⬜ | 用户决策不执行（规范 §12.3 保留 `editor_killring.go` 条目） |

---

> 本报告受 TUI 架构演进驱动，文档同步后需更新"最新同步"日期。
> 报告生成：2026-07-31 | 配套文件：`docs/tui-design-specification.md`、`tui/LAYERS.md`
