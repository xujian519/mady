# Go TUI P0 优化点实施方案

> 依据：`docs/design/go-tui-ecosystem-benchmark.md`（Go 生态 Agent TUI 对标调研，2026-08-11）
> 状态：草案 · 待评审
> 关联代码：`tui/` 模块（Layer 0–7 分层），核心文件已逐一核验

---

## 0. 总览

### 0.1 四项 P0 与建议实施顺序

| # | 优化点 | 核心落点 | 建议顺序 | 工作量 |
|---|--------|----------|---------|--------|
| P0-3 | Golden 快照测试 | `tui/component` 测试基建 | **① 最先** | 1–2 天 |
| P0-1 | 补间动画系统 | `tui/core` + `tui/overlay.go` | ② | 1–2 天 |
| P0-2 | OSC 8 超链接输出 | `tui/core` cell 模型 | ③ | 2–3 天 |
| P0-4 | 可选能力接口 | `tui/core` + `tui/chat` | ④ | 1–2 天 |

**顺序理由**：P0-1/P0-2/P0-4 都会改变渲染输出或组件契约，P0-3（golden）先落地可为后续改动提供字节级回归防线；P0-4 依赖 P0-1 的动画基础设施（Animatable 需要 tick 驱动）。

### 0.2 通用约束（每次改动遵守）

- 单次改动 ≤ 5 个文件（项目任务粒度规范），每个 P0 内部已拆分为 2–3 步
- 每步完成即验证：`cd tui && go test -race ./... && go vet ./...`，根目录 `go build ./...`
- 提交前：`make verify`（lint + build + test-race 覆盖全部四模块）
- 完成后追加 AI_CHANGELOG：
  ```bash
  go run scripts/changelog/main.go --type=feat --scope=tui --title="..." --body="..."
  ```
- 导出符号必须有注释（Effective Go），文档/注释用中文、代码用英文

---

## P0-3 Golden 快照测试（先做）

### 目标

为组件渲染输出建立字节级回归防线，防止宽度/样式/布局漂移（目前 113 个测试文件均为"断言包含子串"式，无法捕获 1 列偏移、SGR 编码变化等回归）。

### 设计

**机制**（对齐 charmbracelet/catwalk，但用标准库实现，不引外部依赖）：

1. 新增 `tui/component/golden.go`（test helper，`_test.go` 后缀放 helper 亦可）：
   ```go
   // goldenMatch 将 lines 与 testdata/<name>.golden 比对。
   // 环境变量 GOLDEN_UPDATE=1 时重新生成 golden 文件（用于有意变更）。
   func goldenMatch(t *testing.T, name string, lines []string) {
       t.Helper()
       path := filepath.Join("testdata", name+".golden")
       got := strings.Join(lines, "\n") + "\n"
       if os.Getenv("GOLDEN_UPDATE") == "1" {
           if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
               t.Fatal(err)
           }
           return
       }
       want, err := os.ReadFile(path)
       if err != nil { t.Fatalf("missing golden %s (run with GOLDEN_UPDATE=1): %v", path, err) }
       if got != string(want) {
           t.Errorf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
       }
   }
   ```

2. **首批对象**：纯渲染函数（输入 = 数据 + theme + width，无全局状态，天然可快照）：
   - `RenderToolCard(cfg, theme, width)` — `tool_card.go`
   - `RenderEvidenceCard(msg, collapsed, theme, width)` — `evidence_card.go`
   - `RenderConclusionCard(msg, theme, width)` — `conclusion_card.go`
   - `RenderApprovalCard(msg, theme, width)` — `approval_card.go`

3. **矩阵组织**：每个组件一个 `Test<Card>_Golden`，用 `t.Run` 展开尺寸 × 折叠矩阵，golden 文件名编码矩阵（如 `tool_card_w80_collapsed.golden`）：
   - width ∈ {40, 80, 120}
   - collapsed ∈ {true, false}（仅对支持折叠的卡片）

4. **全局调色板组件的固定**（第二阶段，如 StatusBar/TodoPanel）：测试内
   ```go
   prev := theme.CurrentPalette()
   theme.SyncPaletteGlobals(theme.DefaultSemanticDark(), theme.ColorMode256) // 或 Light
   defer theme.SyncPaletteGlobals(prev.Semantic, prev.Mode)
   ```
   已核验 `SyncPaletteGlobals` 可原子替换全局调色板（`theme/palette.go:229`），且组件默认主题构造器（`DefaultConclusionCardTheme()` 等）不依赖全局，第一阶段无需处理。

### 实施步骤

| 步 | 文件 | 内容 |
|----|------|------|
| 1 | `tui/component/golden_test.go`（新） | golden helper + ToolCard/EvidenceCard/ConclusionCard/ApprovalCard 快照测试 + testdata 生成 |
| 2 | `tui/component/testdata/*.golden`（新） | 首轮 `GOLDEN_UPDATE=1` 生成的基线文件 |
| 3 | `tui/component/statusbar_test.go` 等 | 第二阶段：全局调色板组件的快照（可后置） |

### 验证

```bash
cd tui && go test ./component/ -run Golden -count=1   # 首次需 GOLDEN_UPDATE=1 生成基线
# 后续：有意改动渲染 → GOLDEN_UPDATE=1 更新 → 审 diff → 提交
```

### 风险与权衡

- **golden 文件膨胀**：矩阵 4 组件 × 6 组合 ≈ 24 文件，每个 <1KB，可接受；限制矩阵规模即可
- **ANSI 输出平台差异**：纯渲染函数输出与平台无关（无终端 I/O），无此风险
- **误更新**：golden 是"有意变更的显式记录"，审 diff 是流程的一部分

---

## P0-1 补间动画系统

### 目标

为 overlay 弹出/关闭、状态切换提供 60fps 平滑过渡，对齐 charmbracelet/harmonica。当前仅 Loader 有帧动画，overlay 为硬切换。

### 设计

**Step A — `tui/core/tween.go`（纯数据，零依赖，Layer 0）**：

```go
// Easing 将归一化时间 t∈[0,1] 映射为进度 p∈[0,1]（可轻微越界，如 EaseOutBack）。
type Easing func(t float64) float64

// 预置缓动函数（均为纯函数，可表驱动测试）：
var (
    EaseLinear      Easing
    EaseInQuad      Easing
    EaseOutQuad     Easing
    EaseInOutQuad   Easing
    EaseOutCubic    Easing
    EaseOutBack     Easing // overshoot ~10%
)

// Tween 描述 from→to 的一次插值。Value(now) 返回当前值；
// Done(now) 返回 true 表示动画已结束（此时 Value == To）。
type Tween struct {
    From, To float64
    Duration time.Duration
    Start    time.Time
    Ease     Easing
}
func NewTween(from, to float64, d time.Duration, e Easing) *Tween
func (t *Tween) Value(now time.Time) float64
func (t *Tween) Done(now time.Time) bool
```

**Step B — Overlay 过渡（对齐调研报告 P0-1，接入点已核验 `composeOverlays`）**：

`tui/overlay.go` 增加（零值 = 无动画，全部现有 overlay 行为不变）：

```go
type OverlayTransitionKind int
const (
    OverlayTransitionNone OverlayTransitionKind = iota
    OverlayTransitionSlideUp // 从视口底部滑入
    OverlayTransitionFade    // 淡入（dim 强度插值）
)

type OverlayTransition struct {
    Kind     OverlayTransitionKind
    Duration time.Duration
    Easing   core.Easing // nil → EaseOutCubic
}
```

- `Overlay` 增加字段 `Transition OverlayTransition` + 内部 `openAt time.Time`（`PushOverlay` 时置 `time.Now()`）
- `composeOverlays`（`tui/tui_render.go` 调用侧）按进度插值：
  - `SlideUp`：`resolveOverlayOrigin` 结果的行偏移从 `rows`（屏外）插值到目标（`origin.row = targetRow + (1-p)*rows`）
  - `Fade`：dim 强度乘 p（`dimBackgroundRows` 的着色强度参数化）
- 驱动：`PushOverlay` 时若带 Transition，启动 `t.Every(16*time.Millisecond, ...)` 定时 `RequestRender()`，`composeOverlays` 内检测 `p>=1` 后停止 ticker（或 ticker 常驻至动画结束自停，仿照 `TUI.Every` 生命周期模式）

**Step C（可选，第二阶段）**：ChatHistory 滚动吸附 — `chat_history_input.go` 的滚动目标改为 Tween 驱动（`ScrollTo` 的 Y 插值），对长对话滚动体验提升明显；涉及 `chat_history_input.go` + `chat_history.go` 两文件。

### 实施步骤

| 步 | 文件 | 内容 |
|----|------|------|
| A1 | `tui/core/tween.go`（新）、`tui/core/tween_test.go`（新） | Easing 预置 + Tween 纯函数 |
| B1 | `tui/overlay.go` | Transition 类型 + Overlay 字段 + openAt |
| B2 | `tui/tui_focus.go` | PushOverlay 启动动画 ticker |
| B3 | `tui/tui_render.go`（composeOverlays 调用点）或 `tui/overlay.go` | 按进度插值 origin / dim 强度 |
| B4 | `tui/overlay_animation_test.go`（新） | 动画进度插值单测 + 冒烟 |

### 验证

- `go test -race ./core/ ./...`（tui 模块）
- 手动：`mady tui` 打开会话选择器 / 命令面板，观察 200–300ms 滑入过渡；Ctrl+C 中断无残留
- 性能：DebugOverlay 观察动画期间 FPS ≥ 55、renderDur < 16ms

### 风险与权衡

- **动画期间每帧全量 diff**：tick 16ms ≈ 60fps，与现有 loader/光标闪烁同量级；`RequestRender` 有合并机制，无风险
- **关闭动画**（淡出）需要延迟移除 overlay，复杂度高于打开动画：**第一阶段只做打开动画**，关闭保持硬切换，避免 RemoveOverlay 的时序竞态（`tui_focus.go` 的 overlay 栈管理）
- `openAt` 需要 TUI 在 push 时写入，`composeOverlays` 是纯函数——保持"动画状态存 Overlay 字段、合成时读取"即可，不破坏现有 CoW 优化

---

## P0-2 OSC 8 超链接输出（受信任内容）

### 目标

为**受信任来源**（证据卡片引用项、法条引用、知识库来源）输出可点击 OSC 8 超链接；同时保持"LLM 原始输出中的 OSC 8 一律剥离"的既有安全边界不变。

### 现状核验（关键）

- `ParseLine`（`tui/core/cellparse.go`）：遇 OSC/APC/DCS/PM（除 CursorMarker）→ 整行 Raw fallback
- `SanitizeRawContent`（`tui/core/sanitize.go`）：严格白名单（SGR + CursorMarker），OSC 8 被剥离
- 即：**当前任何来源的 OSC 8 都无法上屏**。需要为受信内容开一条显式通道

### 设计

**核心思路**：仿照 `CursorMarker`（`\x1b_pi:c\x07`，受信 APC 通道）的模式，新增受信链接标记；cell 模型中链接为 **Row 级元数据**，不污染 cell 内容；序列化时转换为 OSC 8。

**Step A — `tui/core/link.go`（新）**：

```go
// TrustedLinkMarker 是受信链接的结束标记（APC，终端忽略）。
// 链接以 "\x1b_pi:l;<url>\x07" 开始，文本为普通 cell 内容，以 TrustedLinkMarker 结束。
const TrustedLinkMarker = "\x1b_pi:l\x07"

// Link 构造受信超链接字符串，供组件在受信任内容中使用。
// LLM 原始输出中的 OSC 8 不走此通道（仍被 sanitize 剥离）。
func Link(text, url string) string

// LinkSpan 记录一行内一段超链接的可见列区间 [StartCol, EndCol)。
type LinkSpan struct {
    StartCol, EndCol int64
    URL              string
}
```

**Step B — cell 模型扩展（`tui/core/cell.go` + `cellparse.go` + `cellrender.go`）**：

1. `Row` 增加 `Links []LinkSpan`
2. `ParseLine`：`hasUnrepresentableEscape` 放行受信链接标记（与 CursorMarker 并列）；主循环识别 `\x1b_pi:l;<url>\x07` 记录 span 起始列（用 `visibleWidthOfCells`），遇 `TrustedLinkMarker` 记录结束列
3. `SerializeRow`：遍历 cells 累加可见列，进入/离开 LinkSpan 时注入 `\x1b]8;;<url>\x1b\\` / `\x1b]8;;\x1b\\`（OSC 8）
4. `RowsEqual`（`tui/core/cell.go`）：比较 `Links` 切片（链接变化必须触发重绘）——已核验 `DiffFrame` 的 prefix/suffix 与 `DiffCells` 均走 `RowsEqual`

**Step C — diff 简化路径**：`DiffFrame`/`DiffCells` 中，含非空 `Links` 的行走**整行重写**（复用现有 Raw 行处理的分支形态），避免 segment 级 diff 在链接边界补全 OSC 8 的复杂度。链接行频率低（引用卡片、法条），性能损失可忽略。

**Step D — 能力开关**：`tui/core/sanitize.go` 或 `link.go` 提供包级开关 `SetOSC8Enabled(bool)`；`tui/terminal/detect.go` 加 OSC 8 支持检测；TUI 启动时按检测结果设置。禁用时 `SerializeRow` 跳过链接注入（纯文本退化，无下划线/无链接）。

**Step E — 组件接入（首个用例）**：`tui/component/evidence_card.go` 的证据引用项 `Snippet` 用 `core.Link(snippet, url)` 包装（`EvidenceRef` 增加可选 `URL` 字段）。引用是 Mady 专利/法律场景最高价值的可点击对象。

### 实施步骤

| 步 | 文件 | 内容 |
|----|------|------|
| A | `tui/core/link.go`（新）、`tui/core/link_test.go`（新） | Link/LinkSpan/TrustedLinkMarker + 开关 |
| B | `tui/core/cell.go`、`tui/core/cellparse.go`、`tui/core/cellrender.go` | Row.Links + Parse/Serialize + RowsEqual |
| C | `tui/core/celldiff.go` | 含 Links 行走全行重写 |
| D | `tui/terminal/detect.go` | OSC 8 能力检测 |
| E | `tui/component/evidence_card.go` | 引用项接入（`EvidenceRef.URL` 可选字段） |

### 验证

- 单测：`Link()` 往返（parse → serialize 字节等价）；LLM 原始 OSC 8 仍被剥离（现有 `sanitize_test.go` 用例不回归）；宽字符行内链接列区间正确
- 集成：`chat_history_render_message.go` 的 evidence_card 渲染在终端可点击（iTerm2/kitty/WezTerm 实测）
- 安全：恶意 URL 不经受信通道（只有组件代码可构造 `core.Link`）

### 风险与权衡

- **老终端不支持 OSC 8**：由 Step D 能力开关兜底，退化无链接纯文本
- **Row 结构变化**：`Row` 是热路径核心类型，加一个 `[]LinkSpan` 字段有轻微内存开销（slice header 24B/行）；绝大多数行 Links 为 nil，可接受
- **SerializeRow 复杂度**：链接注入需在 segment 序列化时维护列游标，是本次改动最易错处——对应验证步骤的往返测试必须覆盖（含宽字符 + 多链接同行）

---

## P0-4 可选能力接口

### 目标

对齐 crush 的"可选能力接口"组合模式（Focusable/Highlightable/Expandable/Compactable/Animatable…）。核验结论：**Mady 已有该模式雏形**（`Sizer`/`Disposable`/`Focusable`/`MouseTarget`/`MouseConsumer` 均为可选接口，见 `tui/core/component.go`），差距在于**卡片交互状态的分发仍是散落分支**（`chat_history_render_message.go` 中 `m.Collapsed` 三处读取 + `expandedGroups` 集合），而非接口缺失。

### 设计

**Step A — 接口定义（`tui/core/component.go` 追加，向后兼容）**：

```go
// Expandable 是可选接口：组件自身持有折叠/展开状态。
type Expandable interface {
    ToggleExpanded()
    IsExpanded() bool
}

// Compactable 是可选接口：组件支持紧凑/完整两种渲染模式。
type Compactable interface {
    SetCompact(bool)
    IsCompact() bool
}
```

**Step B — ChatMessage 状态访问统一（`tui/chat/chat_model.go`）**：
折叠状态继续存储在 `ChatMessage.Collapsed`（**数据模型层是对的**——会话持久化需要，不迁移到组件内），但提供统一访问器：

```go
// IsCollapsible 报告该消息是否可折叠（DomainMsg 卡片 / 工具结果 / 长文本）。
func (m *ChatMessage) IsCollapsible() bool
```

**Step C — ChatHistory 渲染/交互收敛（`tui/chat/chat_history_render_message.go` + `chat_history_input.go`）**：
将 `renderDomainCard` 的 switch 与 `renderMessage` 的 RoleAssistant/RoleTool 折叠分支统一走 `IsCollapsible()` + `core.Expandable` 契约的渲染入口 `renderCollapsible(...)`，click-to-toggle（`chat_history_input.go`）不再直接改 `expandedGroups` 而是调用统一方法。**效果**：新增卡片类型只需实现渲染函数 + 标记可折叠，交互与状态分发零改动。

**Step D（联动 P0-1）**：`Animatable` 接口（`SetAnimation(*core.Tween)`）在动画基建（P0-1）落地后定义，供消息项动画使用——**不单独实现**，避免无基建空接口。

### 实施步骤

| 步 | 文件 | 内容 |
|----|------|------|
| A | `tui/core/component.go`、`tui/core/component_test.go` | Expandable/Compactable 接口定义 |
| B | `tui/chat/chat_model.go`、`tui/chat/chat_model_test.go` | `IsCollapsible()` 统一访问器 |
| C | `tui/chat/chat_history_render_message.go`、`tui/chat/chat_history_input.go` | 折叠分支收敛到统一入口 |

### 验证

- 单元：`IsCollapsible` 对四类消息（DomainMsg/工具/长文本/普通）判定正确；接口断言 `var _ core.Expandable = ...`
- 回归：现有 `chat_history_render_*` 测试全绿（折叠/展开交互行为不变）
- 手动：evidence card / tool card 点击折叠展开行为与改动前一致

### 风险与权衡

- **Step C 是渲染重构**：`renderMessage` 的折叠分支涉及 RoleAssistant 折叠摘要（截断首行 + `▸ expand`）等细节，重构时须保留行为等价——用 golden（P0-3）覆盖后风险大降，这是 P0-3 先行的又一理由
- 若 Step C 评估风险过高，可降级为仅交付 Step A+B（接口 + 访问器），交互收敛留作后续

---

## 附：完整验证清单

| 阶段 | 命令 | 期望 |
|------|------|------|
| 每步完成 | `cd tui && go test -race ./... && go vet ./...` | 全绿 |
| 提交前 | `make verify` | lint + build + test-race 全过 |
| P0-1 性能 | DebugOverlay（Ctrl+Shift+D） | 动画期间 FPS≥55、renderDur<16ms |
| P0-2 安全 | `go test ./tui/core/ -run Sanitize` | LLM 原始 OSC 8 仍被剥离 |
| P0-3 基线 | `GOLDEN_UPDATE=1 go test ./tui/component/ -run Golden` | 生成基线后二次运行全绿 |
| 变更记录 | `go run scripts/changelog/main.go --type=feat --scope=tui --title=... --body=...` | 追加 AI_CHANGELOG |

## 待澄清项（[NEEDS CLARIFICATION]）

1. **P0-1 关闭动画**：第一阶段只做打开动画；关闭动画（淡出）是否需要，取决于产品对 overlay 切换质感的要求——建议先交付打开动画，实测后再定
2. **P0-2 首个接入点**：本方案选 evidence_card（证据引用）；备选法条引用（lawcite 联动）。若法条场景优先级更高，Step E 换接入点即可（core 层设计不变）
3. **P0-4 Step C 范围**：完整收敛 vs 仅接口定义，取决于 P0-3 golden 覆盖后的回归信心
