# 决策：TUI 双 SGR 样式模型——现状、统一策略与合并方向

> **状态**：已采纳（现状记录 + 缓解措施）· 2026-08-04
> **关联**：`tui/core/sgr.go`、`tui/theme/style.go`、`tui/sgr_roundtrip_test.go`

---

## 背景

TUI 渲染管线中存在**两套并行的样式模型**：

| 模型 | 位置 | 颜色表示 | 输出编码 |
|------|------|---------|---------|
| `core.Style`（cell 级） | `tui/core/cell.go` + `sgr.go` | packed uint32（`Color`：默认/调色板索引/真彩 RGB 三态） | `fgCode`/`bgCode`：调色板色输出 `38;5;n`（`48;5;n`），真彩输出 `38;2;r;g;b` |
| `theme.Style`（主题级） | `tui/theme/style.go` | ANSI 基本色码（`30-37`/`90-97`）+ 可选 `fgParams`/`bgParams` 字符串（`38;5;n`、`38;2;r;g;b`） | `Render`：直接输出 `\x1b[31m` 等基本色，或透传 params |

**数据流**：`theme.Palette` → `color_resolve.go`（hex → params 字符串）→ `theme.Style.WithFgParams` → 拼进渲染字符串 → 进入 `core` 的 cell 模型（`ParseSGR` 再解析回 `core.Style`）→ 帧 diff → `SerializeRow`/`RenderSGR` 重编码输出。

即：**同一批颜色先编码成字符串、再解析回去**，两套编码规则在桥接处交汇。

## 关键事实（2026-08-04 验证）

1. **编码差异**：`core.RenderSGR` 对调色板色（0-255）一律输出 `38;5;n`；`theme.Style.Render` 对基本色（0-15）输出 `30-37`/`90-97`。`core/celldiff_test.go` 注释自认：`ParseSGR canonicalises 31 → Palette(1); fgCode emits it as "38;5;1"`。
2. **渲染管线内无实际输出差异**：所有 theme 渲染字符串都会经过 `core.ParseSGR` 规范化后再重编码，`31` 与 `38;5;1` 在支持 256 色的终端上显示相同。新增 `tui/sgr_roundtrip_test.go` 的 `TestThemeStyleCoreRoundTrip` 锁定该不变量：theme.Style.Render → ParseSGR → RenderSGR → ParseSGR 往返稳定、信息无损（覆盖基本色/亮色/256/真彩/属性混合 10 个用例）。
3. **已知限制（未在本期修复）**：`core` 输出 `38;5;n` 不感知终端协商的色级——在 16 色老终端上，调色板色用 `38;5;n` 编码可能不被支持。`theme` 侧 `color_resolve.go` 的 `FgParams` 已按 `ColorMode` 量化（16 色走 `FgParams16`），但 `core` 的 `fgCode` 无此分支。这是 16 色降级的残余缺口，影响面小（现代终端普遍 ≥256 色）。

## 决策

1. **维持双模型现状**（不合并）。合并（`theme.Style` 直接生成 `core.Style`，消灭字符串桥接）是大工程，涉及 `core`/`theme` 全部渲染路径，风险高，作为独立立项评估。
2. **以测试锁定桥接不变量**：`TestThemeStyleCoreRoundTrip` 成为跨模型 SGR 的回归护栏——任何一侧修改编码规则，必须保持往返稳定。
3. **新属性/新颜色类型必须双端同步实现**（`core.Style` 与 `theme.Style` 各一处），并在 `sgr_roundtrip_test.go` 的用例表中追加用例。
4. **16 色降级缺口**：`core.fgCode/bgCode` 的调色板编码暂不感知 `ColorMode`；如需修复，应在 `core` 引入色级协商（类似 `theme.color_resolve.go` 的 `DetectColorMode`），或让 `theme` 在量化阶段将调色板色直接降级为基本色再交 `core`。

## 影响

- 双端维护成本持续存在（SGR 解析/生成各实现一遍），但已被测试护栏约束。
- 完整合并方向的候选方案：`theme.Style` 携带 `core.Style`（或将 `WithFgParams` 的字符串在 theme 侧直接解析为 `core.Color`），渲染时零桥接。评估窗口：下次 TUI 大版本重构。
