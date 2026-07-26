# Mady TUI 模块全方位审阅主报告

> **审阅日期**：2026-07-26
> **审阅方法**：12 维度并行子智能体（10 个 code-reviewer/explore/bug-analyzer）+ 主线交叉关联
> **审阅范围**：`tui/` 子模块 152 文件（102 源 + 50 测试）/ ~28K 源代码 + ~10K 测试代码 / 8 层 Elm 架构
> **配套基线**：`docs/review/tui-code-review-2026-07-25.md`（前次审阅，27 项问题）+ `docs/review/tui-audit-phase1-baseline-2026-07-26.md`（阶段 1 基线）

---

## 一、总体评估

### 1.1 维度评分汇总

| # | 维度 | 评分 | 等级 | 关键发现 |
|---|------|------|------|---------|
| 1 | 架构与设计哲学 | **7/10** | 🟢 中上 | 分层严格但 `tui`→`chat` 跨层是真实架构债 |
| 2 | 并发与线程安全 | **7.5/10** | 🟢 中上 | 设计纪律优秀；4 处 goroutine 无终止 |
| 3 | 性能与资源 | **7/10** | 🟢 中上 | 流式场景仅用 0.65% 帧预算；Overlay deep-copy 待优化 |
| — | （详见 `tui-dimension3-performance-2026-07-26.md`） | — | — | 含 13 项 Benchmark 实测数据（M4 Pro） |
| 4 | 错误处理与健壮性 | **6.5/10** | 🟡 中 | errors.Is/As 零使用；panic 策略不一致 |
| 5 | 安全 | **6.5/10** | 🟡 中 | LLM 输出 ANSI 序列原样透传（注入风险） |
| 6 | 终端兼容性 | **7.5/10** | 🟢 中上 | 20 品牌覆盖；三套检测系统断裂 |
| 7 | API 设计与公共契约 | **6.5/10** | 🟡 中 | NewTUI 双重签名陷阱；ctx 几乎缺席 |
| 8 | 测试策略与质量 | **5.5/10** | 🔴 低 | 46.6% 函数零覆盖；关键路径裸奔 |
| 9 | 可维护性与代码质量 | **7.5/10** | 🟢 中上 | 分层清晰；theme/stdio Godoc 欠债 |
| 10 | 可观测性 | **3.5/10** | 🔴 低 | OnDebug 死代码；零性能指标 |
| 11a | 国际化 | **2/10** | 🔴 低 | 245+ 处硬编码中文 |
| 11b | 无障碍 | **3/10** | 🔴 低 | 对比度未校验；Border=BorderMuted bug |
| 12 | 文档与开发者体验 | **6.5/10** | 🟡 中 | doc.go Quick Start 无法编译 |

**综合评分**：**6.1 / 10**（加权平均，权重：架构/安全/测试/错误处理各 1.5，其余 1.0；维度 3 性能 7/10 已纳入）

### 1.2 一句话总结

> **TUI 是一个"工程内核扎实、外围体验缺失"的模块**：8 层 Elm 架构执行严格、Cell 级渲染创新、panic recovery 链路完整，但在错误分层、测试有效性、可观测性、国际化、无障碍五个维度存在系统性缺口。`OnDebug` 死代码（声明了 `ctrl+shift+d` 但无人 wire）是这一现状的最强信号——有人开了头但没走完。

---

## 二、关键发现（按严重性排序）

### 🔴 Critical（5 项，阻断级）

| # | 发现 | 维度 | 位置 | 影响 |
|---|------|------|------|------|
| **C-1** | **Kitty 协议全局状态污染**：`TestProcessTerminalKittyKbdMode` 污染全局 `kittyFlagsGlobal=1`，`go test -race -count=5 ./tui/terminal/` 确定性失败 | 8 | `terminal/terminal_kitty_test.go:69` → `keys.go:328` | 测试隔离性破坏；07-25 审阅漏检 |
| **C-2** | **termios 恢复错误被静默吞掉**：`_ = setTermios(...)` 后 `return nil` | 4 | `terminal/terminal.go:247` | 终端可能卡 raw 模式，调用方无从知晓 |
| **C-3** | **`PanicMsg` 丢堆栈**：`Stack: ""` 恒为空，与 `tui_input.go` 的 `captureStack()` 不一致 | 4 | `core/message.go:189` | panic 诊断信息缺失 |
| **C-4** | **LLM 输出 ANSI 序列原样透传**：OSC 8/标题注入/DCS/APC 经 Raw 行直接写 stdout | 5 | `core/cellparse.go:113` → `tui_render.go:156` | 完整注入链；prompt injection 可诱导恶意序列 |
| **C-5** | **doc.go Quick Start 无法编译**：3 处 API 不存在 | 12 | `tui/doc.go:79,82` | 新贡献者上手即受阻 |

### 🟠 High（8 项，本周修复）

| # | 发现 | 维度 | 位置 |
|---|------|------|------|
| H-1 | `OnDebug` 死代码：`ctrl+shift+d` 无任何效果 | 10 | `tui.go:188`, `tui_input.go:262` |
| H-2 | `errors.Is/As` 零使用，错误无分层 | 4 | 全局 |
| H-3 | Editor/Input 硬编码选中色 `48;5;33`（07-25 H4 未修复） | — | `editor_render.go:355`, `input.go:246` |
| H-4 | stdin 缓冲区对未终止 CSI/OSC 无界增长 | 5 | `stdin_buffer.go:340` |
| H-5 | `NewTUI` 双重签名陷阱（结构体 vs 函数选项） | 7 | `tui.go:209,240` |
| H-6 | 245+ 处硬编码中文 UI 文案，零 i18n 集成 | 11a | `chat/`、`component/` 多文件 |
| H-7 | `Border` 与 `BorderMuted` 在 dark 主题颜色完全相同 | 11b | `semantic_theme.go:128-129` |
| H-8 | `verify_layers.sh` 未接入 CI，文档漂移无拦截 | 12 | `.github/workflows/` |

### 🟡 Medium（15 项，本月修复）

| # | 发现 | 维度 |
|---|------|------|
| M-1 | 4 处 session_selector 回调 goroutine 无终止条件 | 2 |
| M-2 | stdio 层持锁 I/O（renderer/progress） | 2 |
| M-3 | theme/appearance watcher panic 后永久死亡，无重启 | 4 |
| M-4 | `tui`→`chat` 跨层依赖（L3→L5）是架构债 | 1 |
| M-5 | `ChatEvent` 三重标识冗余（字符串+结构体+FSM 枚举） | 1 |
| M-6 | `agentadapter` 仅 1 导出符号，12/17 事件映射无测试 | 1,8 |
| M-7 | 三套终端检测系统断裂（TerminalContext 能力为死代码） | 6 |
| M-8 | Sixel 未实现但 LAYERS.md 宣称支持 | 6,12 |
| M-9 | `itoa` 实际 3 处重复（含未迁移的 internal/conv） | 9 |
| M-10 | 置信度条渲染器 3 处重复 | 9 |
| M-11 | theme/stdio 包 Godoc 严重缺失（~60 符号） | 9 |
| M-12 | `editor_edit.go` 7 个核心编辑函数零覆盖 | 8 |
| M-13 | 8 个整文件零覆盖（~2470 行生产代码） | 8 |
| M-14 | 仅 2 个 benchmark，渲染热路径零基线 | 8 |
| M-15 | LAYERS.md 依赖矩阵错误（stdio 漏写 Layer 1） | 12 |

---

## 三、跨维度关联分析

### 3.1 "LLM 输出信任边界"贯穿多维度

```
维度5（安全）        维度4（错误处理）      维度10（可观测性）
     │                    │                      │
     ▼                    ▼                      ▼
C-4: ANSI注入      errors.Is零使用        OnDebug死代码
H-4: stdin无界     termios吞错            零性能指标
                   watcher静默死亡
```

**根因**：TUI 将 LLM 输出视为半可信但缺少系统性清洗层。`hasUnrepresentableEscape` 的 Raw 回退设计正确（防 panic），但回退后原样透传是安全漏洞。

**联动修复**：在 `cellparse.go` 或 `tui_render.go` 增加白名单清洗函数，同时修复 C-4 + 提升安全评分。

### 3.2 "全局可变状态"贯穿多维度

```
维度8（测试）          维度2（并发）          维度6（终端兼容）
     │                    │                      │
     ▼                    ▼                      ▼
C-1: Kitty污染       atomic使用正确        三套检测断裂
    colorOverride       channel所有权清晰    TerminalContext死代码
    非atomic!
```

**根因**：模块设计早期使用全局变量简化 API（`kittyActive`/`kittyFlagsGlobal`/`colorOverride`/`atomicPalette`），随着代码增长，全局状态的测试隔离性和检测一致性成为负担。

### 3.3 "文档与代码漂移"贯穿多维度

```
维度12（文档）         维度9（可维护性）      维度6（终端兼容）
     │                    │                      │
     ▼                    ▼                      ▼
C-5: doc.go无法编译   itoa迁移半途          Sixel文档不符
    LAYERS计数过期      置信度条重复          品牌检测死分支
    verify未接CI        theme Godoc欠债
```

**根因**：缺少"文档即代码"的编译期保护。`Example*` 测试函数零存在，doc.go 示例从不参与编译。

---

## 四、改进路线图

### 4.1 Quick-win（1 周内，低风险，高收益）

| # | 任务 | 维度 | 预估 | 收益 |
|---|------|------|------|------|
| 1 | 修复 C-1：测试 `t.Cleanup` 重置全局 Kitty flags | 8 | 5min | 测试隔离性恢复 |
| 2 | 修复 C-2：termios 恢复返回错误 | 4 | 5min | 终端安全 |
| 3 | 修复 C-3：`PanicMsg` 加 `captureStack()` | 4 | 10min | 诊断信息 |
| 4 | 修复 C-5：doc.go Quick Start API 修正 | 12 | 10min | 新人上手 |
| 5 | 修复 H-7：区分 Border/BorderMuted 颜色 | 11b | 5min | a11y |
| 6 | 完成 `itoa` 去重迁移（3→1） | 9 | 20min | DRY |
| 7 | `theme/global_test.go` 的 `os.Setenv`→`t.Setenv` | 8 | 5min | 测试并行安全 |
| 8 | LAYERS.md 依赖矩阵修正 + 计数更新 | 12 | 15min | 文档准确性 |

### 4.2 中期（1 月内，中风险）

| # | 任务 | 维度 | 预估 |
|---|------|------|------|
| 1 | 修复 C-4：实现 Raw 行白名单清洗 | 5 | 1-2 天 |
| 2 | 修复 H-1：实现 debug overlay（FPS/队列/状态） | 10 | 2-3 天 |
| 3 | 集成 `pkg/i18n`，迁移 245+ 硬编码文案 | 11a | 3-5 天 |
| 4 | 补 `agentadapter` 事件映射表驱动测试（14 类） | 8 | 1 天 |
| 5 | 补 `editor_edit.go` 7 个核心函数表驱动测试 | 8 | 1 天 |
| 6 | 补 P0 级 Benchmark（layout/markdown/sgr/quantize） | 8 | 1 天 |
| 7 | `verify_layers.sh` 接入 CI + Makefile | 12 | 2h |
| 8 | session_selector 回调加 ctx/done channel | 2 | 4h |
| 9 | 统一三套终端检测系统 | 6 | 1 天 |
| 10 | 拆分 `AppHost` 为 Lifecycle/Container/Overlay 三接口 | 1 | 4h |

### 4.3 长期（季度级，需 ADR）

| # | 任务 | 维度 | 说明 |
|---|------|------|------|
| 1 | 消除 `tui`→`chat` 跨层依赖（L3→L5 反转） | 1 | 需 ADR + 迁移指南 |
| 2 | `Component.Render` 返回 `[]core.Row` 而非 `[]string` | 1,3 | 消除 string↔Cell 反复解析 |
| 3 | 引入错误类型分层（`TermError`/`ThemeError`/`ClipboardError`） | 4 | 配合 `errors.Is/As` |
| 4 | 评估 `agentadapter` 是否保留为独立包 | 1 | 若无第二适配器则合并 |
| 5 | 补 Sixel 图像协议实现 | 6 | Linux 终端覆盖 |
| 6 | 新增 high-contrast + 色盲主题 | 11b | a11y 达标 |
| 7 | `Component.Render` 加并发文档 + 渲染锁时序 ADR | 2 | 固化不变量 |

---

## 五、与前次审阅（07-25）的对比

### 5.1 修复进度

| 类别 | 07-25 发现 | 已修复 | 未修复 | 新增 |
|------|-----------|--------|--------|------|
| Critical | 3 | 2（C1/C2） | 1（C3 Overlay deep-copy 保留） | **5**（本次新发现） |
| High | 6 | 3（H2/H6 部分） | 3（H4/H5 部分/H6 部分） | **8** |
| Medium | 8 | 6 | 2 | **15** |
| Low | 10 | 10（lint 全清） | 0 | — |
| **合计** | **27** | **21（78%）** | **6** | **28** |

### 5.2 新增发现的特点

07-25 审阅聚焦**代码缺陷**（lint/类型/魔法字符串），本次审阅发现的 28 项新问题更偏向**系统性短板**：
- 错误处理分层（errors.Is 零使用）
- 测试有效性（46.6% 函数零覆盖）
- 安全信任边界（LLM 输出注入）
- 可观测性（OnDebug 死代码）
- 国际化/无障碍（硬编码/对比度）

这反映了审阅深度的提升——从"代码对不对"到"架构是否经得起演进"。

---

## 六、维度详细报告索引

各维度的完整报告分散在以下位置：

| 维度 | 报告位置 | 评分 |
|------|---------|------|
| 维度 2 并发 | 阶段 1 基线报告 §四 | 7.5/10 |
| 维度 4 错误处理 | 阶段 1 基线报告 §四 | 6.5/10 |
| 维度 1+7 架构+API | 子智能体 #1 产出（本会话） | 7/10, 6.5/10 |
| 维度 3 性能 | `tui-dimension3-performance-2026-07-26.md`（13 项 Benchmark 实测） | 7/10 |
| 维度 5 安全 | 子智能体 #3 产出（本会话） | 6.5/10 |
| 维度 6 终端兼容 | 子智能体 #4 产出（本会话） | 7.5/10 |
| 维度 8 测试 | 子智能体 #5 产出（本会话） | 5.5/10 |
| 维度 9 可维护性 | 子智能体 #6 产出（本会话） | 7.5/10 |
| 维度 10+11 观测/i18n/a11y | 子智能体 #7 产出（本会话） | 3.5/2/3 |
| 维度 12 文档 | 子智能体 #8 产出（本会话） | 6.5/10 |

> 各子智能体报告已在本会话中完整呈现，含具体文件:行号引用与修复建议。

---

## 七、建议的后续行动

1. **立即**：执行 Quick-win 清单（8 项，~1 天），修复 5 个 Critical
2. **本周**：补 `agentadapter`/`editor_edit`/`ansi.go` 测试（关闭关键路径裸奔）
3. **本月**：实现 LLM 输出清洗层 + debug overlay + i18n 集成
4. **季度**：启动 `tui`→`chat` 依赖反转 ADR + 错误类型分层

> 审阅完成：2026-07-26
> 审阅人：Grok Build（12 维度并行子智能体编排）
> 配套：`tui/LAYERS.md` + `docs/tui-design-specification.md`
