# Go 生态 Agent TUI 对标调研报告

> 调研日期：2026-08-11 · 调研范围：开源社区以 Go 为主的 AI 智能体（Agent）项目及其 TUI 实现
> 对标对象：Mady `tui/` 模块（7 层分层 Elm 架构，~64K 行含测试）
> 数据来源：GitHub API + 项目源码/README + 本地 SearXNG，当日快照

---

## 1. 对标项目总览

| 项目 | Star | 语言 | TUI 技术栈 | 定位 | 对标价值 |
|---|---|---|---|---|---|
| charmbracelet/crush | 27.3k | Go | bubbletea v2 + ultraviolet + lipgloss v2 + glamour v2 | Charm 旗舰 agentic 编码 TUI | ★★★ 最高 |
| charmbracelet/bubbletea | 44.3k | Go | 自研（Elm 架构） | Go TUI 事实标准框架 | ★★★ |
| charmbracelet/ultraviolet | 369 | Go | cell 渲染内核 | bubbletea v2 底层，ncurses 式差分渲染 | ★★★ |
| charmbracelet/glow | 26.8k | Go | bubbletea + glamour | Markdown 终端渲染 | ★★ |
| charmbracelet/bubbles | 8.8k | Go | bubbletea 组件库 | textarea/list/viewport 等 | ★★ |
| charmbracelet/harmonica | 1.6k | Go | 补间动画库 | 平滑动画（滚动/过渡） | ★★ |
| charmbracelet/catwalk | — | Go | 快照测试 | TUI 组件 golden-file 测试 | ★★ |
| jesseduffield/lazygit | 81.2k | Go | tcell/v3 自研引擎（已弃 gocui） | 自研引擎标杆 | ★★★ |
| sigoden/aichat | 10.3k | **Rust** | reedline + crossterm + 自研流式渲染 | 轻量 Chat-REPL 范式 | ★★（思路参考） |
| rivo/tview | 14.0k | Go | tcell 之上 widget 树 | k9s（34.3k★）的框架 | ★ |
| aaif-goose/goose | 52.7k | Rust+TS | TS TUI（已弃用）→ ACP 协议化生态 | "放弃自研 TUI"路线 | ★（路线参考） |
| aandrew-me/tgpt | 3.2k | Go | bubbletea v2 | 免费 AI 聊天 CLI | ★ |

**关键事实修正**（调研推翻的常见假设）：
- goose 无 Go 代码（Rust+TS），且官方自研 TUI 已弃用，转向 ACP（Agent Client Protocol）客户端生态
- aichat 是 Rust 项目（reedline REPL），非 Go
- charmbracelet 无 jelly；其 agent 栈实为 **crush**（agentic 编码 TUI）+ **fantasy**（Go agent 框架）+ **catwalk**（快照测试）；mods 已于 2026-03 sunset

---

## 2. Mady TUI 现状画像（本地核验结论）

对 `tui/` 源码（渲染管线、事件循环、FSM、组件、主题、终端层）逐文件核验后，Mady TUI 的真实能力如下：

**渲染内核（Level：生产级，与 ultraviolet 同级甚至更细）**
- Cell 级差分渲染：`core/cell*.go` 把字符串解析为 Cell 网格，帧 diff 下沉到 **segment（列段）级**，比行级 diff 更省带宽
- 引擎级优化：DiffFrame 双向扫描（prefix/suffix 跳过未变行）、原始字符串快速路径（byte 相同则复用已解析 Row，避免每帧 ParseLine）、`sync.Pool` 对象池
- 终端协议细节：CSI 2026 同步输出包裹、DECAWM 禁/启管理、**光标状态机**（状态不变时不发命令，保住 blink timer）
- 遥测：FPS 环形缓冲、>16ms 渲染预算告警、每 100 帧内存采样（DebugOverlay 展示）
- 宽字符/中文：原生 East-Asian width 处理、宽字符 diff 边界保护（adjustStart 不劈半宽字符）、IME `CURSOR_MARKER` 光标定位

**状态管理（Level：领先）**
- 显式 FSM（`chat/state.go`）：9 个状态 × 18 种事件，`Transition` 纯函数、可表驱动测试，作为回归 oracle
- Elm 消息架构：Msg/Cmd/Batch/Sequence，单 goroutine 事件循环，渲染请求合并（RequestRender coalesce）

**应用层（Level：领域差异化优势）**
- 23 种 ChatEvent、44 个组件，含专利/法律专业卡片：ToolCard、EvidenceCard、ConclusionCard（置信度条）、ApprovalCard、ReviewGate、JudgmentView、TodoPanel、SkillCenter、CommandCenter（Ctrl+P）、SessionSelector
- 消息渲染缓存：ChatHistory `cachedAll` splice fast path + `cachedMsgRanges` 鼠标命中映射 + Markdown 组件宽度感知块缓存（与 crush `cachedMessageItem` 同级）

**主题/终端层（Level：领先）**
- 语义主题（light/dark）+ a11y 主题（高对比/色盲安全）+ JSON 主题热重载（mtime 轮询）+ macOS NSAppearance 深色模式自动跟随
- Kitty 键盘协议、keymap.json 键位配置化、RGB→16 色量化降采样、Kitty/iTerm2/HalfBlock/ASCII 四路图片显示、F2 鼠标 passthrough

**工程纪律（Level：强）**
- 7 层严格单向依赖（`LAYERS.md` 自动校验脚本）、Update 不做 IO、sanitize 单一口径剥除危险转义（含 OSC 8 注入）、113 个测试文件

---

## 3. 逐维度对比矩阵

| 维度 | Mady | 生态最佳实践 | 结论 |
|---|---|---|---|
| 差分渲染粒度 | Cell/segment 级 + 双向扫描 + 对象池 | ultraviolet：ncurses 式 cell diff；lazygit：tcell 全量重算 | **领先** |
| 宽字符/中文/IME | 原生深度支持（width.go/runeutil/CURSOR_MARKER） | 国外项目普遍用 rivo/uniseg、charm.land/x/ansi 浅层处理 | **绝对领先** |
| 状态管理 | 显式 FSM 纯函数 | crush：唯一 model + 命令式子组件（务实但不可表测） | **领先（可测性）** |
| 主题系统 | 语义+a11y+热重载+系统深色跟随 | lipgloss v2 样式 DSL（无热重载/系统跟随） | **领先** |
| 消息渲染缓存 | cachedAll splice + Markdown 块缓存 | crush cachedMessageItem 缓存+失效 | 同级 |
| 终端协议细节 | CSI 2026 + 光标状态机 + FPS/预算遥测 | ultraviolet 免 terminfo；多数项目无遥测 | **领先** |
| 组件能力接口 | Component/Updatable/Focusable 三接口 | crush：8 种可选能力接口（Focusable/Highlightable/Expandable/Animatable/Compactable/KeyEventHandler…） | **差距** |
| 补间动画 | 仅 Loader spinner | harmonica 补间动画（面板过渡/滚动吸附） | **差距** |
| OSC 8 超链接 | 只剥离不输出（sanitize 安全方向） | glow/crush 在受信任内容输出可点击链接 | **差距** |
| 快照测试 | 大量单元测试，无 golden-file | catwalk：TUI 组件字节级快照 + golden 文件 | **差距** |
| 布局模型 | 字符串行 + Flex + overlay 合成 | crush：ScreenBuffer 矩形布局（StyledString.Draw(scr, rect)），支持 pills/sidebar 不规则形状 | 差异（聊天场景行模型够用） |
| 流式 Markdown | 增量 append + 块缓存 + 流去重 | aichat：syntect 流式渲染双路径（markdown_stream/raw_stream） | 同级（各自路径不同） |
| 鼠标交互 | 命中测试/选择/passthrough/节流 | crush：拖拽+双击/三击选择 | 同级（手势丰富度略逊） |
| 架构路线 | 进程内嵌（agentadapter 直连 agentcore） | goose：TUI 独立进程 + ACP 协议化生态 | 差异（开放协议是机会） |

---

## 4. 差距与可优化点（按 ROI 排序）

### P0 值得做（高 ROI，改动集中在 tui 模块内）

**1. 补间动画系统（对齐 harmonica）**
- 现状：只有 Loader spinner 帧动画；面板出现/覆盖层弹出是硬切换
- 建议：在 `tui.Every` tick 基础上加 Easing 函数库（linear/ease-in-out/cubic），为 overlay 弹出、滚动吸附、状态切换提供 60fps 平滑过渡
- 理由：现代 TUI 的"质感"分水岭；引擎已有 tick 循环与 FPS 遥测，成本低
- 涉及：`tui/core`（新增 easing.go）+ `tui/overlay.go`

**2. OSC 8 超链接输出（受信任内容）**
- 现状：`sanitize.go` 只剥离 OSC 8（防注入方向正确）
- 建议：为**受信任来源**（法条引用 `lawcite`、案件链接、知识库来源、证据出处）提供 `core.Link(text, url)` 输出 OSC 8，同时保持 LLM 原始输出的剥离策略不变
- 理由：专利/法律场景的引用可点击化价值高（法条→权威站、来源→文档跳转）；Mady 已有 citation/evidence 展示体系，天然适配
- 涉及：`tui/core/sanitize.go` + `cellparse.go`（解析侧）+ 引用卡片组件

**3. Golden-file 快照测试（对齐 catwalk）**
- 现状：113 个测试文件覆盖逻辑，但组件渲染输出无字节级回归防线
- 建议：新增 `tui/component/snapshot_test.go` 机制——对 ToolCard/EvidenceCard/StatusBar 等纯渲染组件，固定宽度+主题渲染输出与 `.golden` 文件比对，变更需显式更新
- 理由：TUI 渲染回归最难人工察觉（宽度/样式漂移）；Mady 渲染管线纯函数化程度高，快照成本低

**4. 可选能力接口（对齐 crush 能力组合）**
- 现状：ToolCard 的 Collapsed 状态由调用方（ChatHistory）持有，组件自身无交互状态
- 建议：引入 `Expandable`/`Animatable`/`Compactable` 等可选接口，让卡片组件自包含折叠/展开状态与命中测试，减少 ChatHistory 的簿记负担
- 理由：组件可复用性提升，为"消息项直接可交互"铺路

### P1 值得评估（中等 ROI）

**5. 矩形屏幕缓冲布局（对齐 crush ScreenBuffer）**
- 现状：组件渲染为字符串行，引擎做行拼接 + overlay 合成；Flex 布局不支持不规则形状（如 pills、侧栏 badge）
- 评估：若未来要做"主区+侧栏"多面板或流式工具栏，再引入 `StyledString.Draw(scr, rect)` 式矩形绘制；当前聊天场景的行模型更简单、已足够
- 结论：**暂缓**，记录为架构演进选项

**6. 流式 Markdown 增量渲染（对齐 aichat 思路）**
- 现状：流式 delta 追加后整块重渲染（有块缓存与 splice fast path 兜底）
- 评估：超长回复（>10K token）尾部渲染是否成为瓶颈，用 DebugOverlay 的 renderDur 数据先量化；若 >16ms 预算频繁触发再优化为 token 级增量
- 结论：**先量化再决定**，不盲目引入

**7. TUI 独立进程 + 开放协议双轨（对齐 goose ACP 路线）**
- 现状：TUI 与 agentcore 进程内耦合（`agentadapter` 直连）
- 评估：Mady 已有 A2UI/ACP/Server 协议层。可将 TUI 拆为独立进程，经 A2UI 连接任意 agent 后端，同时让第三方客户端接入 Mady
- 结论：**战略级选项**，与桌面端（desktop/ Wails）规划联动，建议单独立项

### P2 不建议（明确排除）

- **换用 bubbletea/迁移框架**：Mady 渲染内核已到/超过 ultraviolet 水平，迁移是纯损失
- **仿 crush 唯一 model 架构**：Mady 严格 Elm + 纯函数 FSM 是可测试性资产
- **LSP/文件编辑器集成**：领域（专利/法律）不符，crush 的编码场景专属
- **REPL 轻量化**：Mady 是专业工作台定位，全屏框架是正确的产品决策

---

## 5. 结论

1. **Mady TUI 的真实水平在 Go 生态属于第一梯队**：Cell 级差分渲染、显式 FSM、主题/终端协议细节、中文宽字符支持均为领先项；渲染内核可与 bubbletea v2 的 ultraviolet 正面对标，且更细（segment 级）。
2. **差距集中在"表现层细节"而非"内核"**：动画质感、可点击链接、快照测试、能力接口组合——都是可独立增量交付的优化，不涉及架构重构。
3. **最大战略机会是协议化**：goose 的教训是"自研 TUI 绑定框架会锁死生态"；Mady 已有 A2UI/ACP，TUI 独立进程化可同时解锁"官方 TUI 连任意后端"与"第三方客户端连 Mady"。

## Sources

- [aaif-goose/goose](https://github.com/aaif-goose/goose) · [goose ACP 与新 TUI 博客](https://github.com/aaif-goose/goose/blob/main/documentation/blog/2026-04-08-goose-acp-and-new-tui/index.md) · [agentclientprotocol.com](https://agentclientprotocol.com/)
- [charmbracelet/crush](https://github.com/charmbracelet/crush)（[internal/ui/AGENTS.md](https://github.com/charmbracelet/crush/blob/main/internal/ui/AGENTS.md)）· [bubbletea](https://github.com/charmbracelet/bubbletea)（[v2 升级指南](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)）· [ultraviolet](https://github.com/charmbracelet/ultraviolet) · [bubbles](https://github.com/charmbracelet/bubbles) · [lipgloss](https://github.com/charmbracelet/lipgloss) · [glow](https://github.com/charmbracelet/glow) · [harmonica](https://github.com/charmbracelet/harmonica) · [catwalk](https://github.com/charmbracelet/catwalk) · [mods](https://github.com/charmbracelet/mods)
- [jesseduffield/lazygit](https://github.com/jesseduffield/lazygit) · [rivo/tview](https://github.com/rivo/tview) · [derailed/k9s](https://github.com/derailed/k9s)
- [sigoden/aichat](https://github.com/sigoden/aichat) · [aandrew-me/tgpt](https://github.com/aandrew-me/tgpt)
- 本地核验：`tui/LAYERS.md`、`tui/doc.go`、`docs/decisions/tui-layers-architecture.md`、`tui/core/celldiff.go`、`tui/tui_render.go`、`tui/chat/state.go`、`tui/chat/chat_history_render.go`、`tui/component/markdown.go`、`tui/component/tool_card.go`
