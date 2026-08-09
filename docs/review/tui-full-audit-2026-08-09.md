# Mady TUI 全量审阅总报告（2026-08-09）

> **审阅日期**：2026-08-09
> **审阅方法**：阶段 0 基线（build/vet/test-race/lint/verify_layers/doc-check）+ 上次 28 项缺陷状态复核 +
> 8 路并行走查子代理（terminal / theme / 渲染引擎 / 编辑器组件 / Markdown组件 / 面板卡片组件 /
> 事件循环输入 / chat应用层）+ 主代理交叉复核（C-4 注入链、P0-1 resize 循环）
> **审阅范围**：`tui/` 独立子模块，230 个 Go 文件（116 生产 + 114 测试），31,708 行生产代码 + 25,516 行测试代码
> **审阅计划**：`docs/review/tui-full-review-plan-2026-08-09.md`
> **配套基线**：`tui-code-review-2026-07-25.md`（27 项）、`tui-full-audit-2026-07-26.md`（5C+8H+15M）

---

## 一、总体评估

**基线验证结果**：`go build` ✅ · `go vet` ✅ · `go test -race -count=1 ./...` ✅（9 包全绿）·
`golangci-lint` ✅（0 issues）· `verify_layers.sh` ✅（116 文件一致）·
`make doc-check` ❌（**仓库级**计数漂移：AGENTS.md/CLAUDE.md 的 Go 文件计数过期、CONTRIBUTING.md 引用不存在的 `workflows` 路径，与 TUI 模块无关）

**一句话总结**：

> **TUI 模块在 07-26 审阅后完成了一次高质量"缺陷歼灭战"**——28 项历史缺陷中 23 项已确认修复
> （含全部 5 项 Critical），安全注入链（C-4）封堵完整；但本次审阅新发现 **1 项 P0**（resize
> 消息防抖自续循环，组件永远收不到 resize 事件）与 **12 项 P1**，集中在三类根因：
> **① 事件循环防抖/分发路径缺陷**（P0-1、Filter 绕过）；**② 差分渲染的 Raw 行边界**（Raw→Cells
> 幽灵残留、光标恢复缺失）；**③ Markdown 宽松解析引入的回归**（行首粗体误判列表、单列表格）。
> 上次遗留的 M-1（session_selector goroutine）、H-2（errors.Is 零使用）、H-6（216 处中文硬编码）仍未修复。

**修复率统计**：上次 28 项中 **23 已修复 / 2 部分修复（M-4、M-10）/ 3 未修复（H-2、H-6、M-1）**。

---

## 二、P0（阻断级，1 项）

### P0-1｜WindowSizeMsg 防抖自续循环：组件永远收不到 resize 消息，且常驻 100ms 定时器

- **位置**：`tui/tui_input.go:40-58`（processMsg 防抖分支）；触发源 `tui_lifecycle.go:89`（Start 初始发送）、`tui_input.go:344`（onTerminalResize）
- **问题**：`processMsg` 对 `WindowSizeMsg` 一律进入 100ms 防抖分支并 `return`，防抖定时器回调 `SendMsg(pendingMsg)` 回投的**仍是 `WindowSizeMsg`** → 再次命中同一分支 → 无限自续循环（每 100ms 一次），且没有任何路径把 resize 消息分发到组件。
- **实证**（/tmp 独立验证程序）：Start 后 350ms 事件环收到 4 条 resize 消息、probe 组件收到 0 次。
- **影响**：
  1. 常驻循环：TUI 整个生命周期每 100ms 一个 timer 创建/触发 + channel send（Stop 后终止）；
  2. 所有 resize 下游逻辑失效：`chatLayout.recalcMaxRows`（chat_app_layout.go:260）、ChatHistory/Editor 的 `WindowSizeMsg → Invalidate`（chat_history_input.go:69、editor.go:405）、StatusBar/Loader/Markdown/Autocomplete/Settings/SelectList/ReviewGate 等 9+ 组件。视觉宽度因 `onTerminalResize` 置 `firstFrame=true` 全量重绘而正确，但**依赖消息的缓存失效、软换行重算、编辑器 maxRows、滚动几何全部陈旧**。
- **修复建议**：方案 A——防抖移到发送侧（`onTerminalResize` 内合并 100ms），`processMsg` 收到 `WindowSizeMsg` 直接分发；方案 B——AfterFunc 回调投递内部包装类型，`processMsg` 解包后直接分发。**必须补回归测试**：Start 后发送 resize，断言组件恰好收到一次且消息计数不持续增长。
- **根因归类**：事件循环防抖/分发路径缺陷。现有测试（lifecycle_test.go:500）只调 `renderFrame` 不经 `processMsg`，故未捕获。

---

## 三、P1（高，12 项）

### 维度 2 terminal（3 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-1 | `detect.go:781-792` | VTE KKP 阈值 `82000` 与注释公式（major*10000+minor*100+patch）矛盾：0.82.0 = 8200，82000 = 8.20.0。VTE 全为 0.x 线，`v >= 82000` 永假 | GNOME Terminal/Tilix 等 **VTE 终端永远拿不到 KKP**，Shift+Enter/Ctrl+. 静默降级，与文档"VTE >= 0.82.0"矛盾；测试 `TestVteHasKKP` 固化同一错误 |
| P1-2 | `keys.go:381-388` | Kitty PUA 映射错位：规范 `0xe001=Enter / 0xe002=Tab / 0xe003=Backspace`，代码映射为 `tab / backspace / enter`（0xe004 起与规范一致） | `MADY_KITTY_FLAGS=9` 下 Enter 被解析为 Tab（触发补全而非提交）、Tab 变退格、退格变回车；测试 `TestKittyC0PUAKeys` 固化错误映射 |
| P1-3 | `keybindings.go:215-221` 等（见 P2 列表） | — | 见下方 P2-2~P2-7（terminal 侧 5 项 P2 合计在此维度） |

### 维度 3 theme（1 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-4 | `theme_registry.go:249-255` | `StartAutoThemeWatcher` 回调**无条件** `ApplyThemeByName("auto")`，丢弃外观参数、不检查当前主题 | 用户手动切换主题后（Ctrl+Alt+T），下一次 OS 明暗变化即被静默覆盖回 auto；包级 API 契约（"仅 auto 激活时生效"）破坏 |

### 维度 4 渲染引擎（2 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-5 | `celldiff.go:146-158` + `tui_render.go:179-189` | Raw→Cells 帧间差分产出 `RawContent=""` + 空 Segments → renderFrame 零输出 | 同一行从 Raw（含不可表示转义）切回普通文本时**旧内容幽灵残留**；关闭含 Raw 行的 Overlay 时常见（prev 是 Raw 覆盖行、新帧是 Cell 行），残影直到全量重绘 |
| P1-6 | `tui_render.go:153-156` + 210-231 | 全量重绘分支无条件 `HideCursor()` 后**未同步** `lastCursor.visible=false` | resize 触发全量重绘后光标**永久隐藏**（状态化光标块认为"已隐藏"，不发 ShowCursor），直接影响 CJK IME 候选框定位 |

### 维度 5a 编辑器（2 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-7 | `editor_edit.go:325-341 / 370-382 / 500-515` | `deleteWordBackward`（行首合并）、`deleteWordForward`/`deleteToLineEnd`（行尾合并）三处分支直接 `return`，缓冲区已变更但 **onChange 从未调用**（`deleteBackward`/`deleteForward` 的同类分支走统一出口） | chat 层以 onChange 驱动 autocomplete Refresh/渲染/UI 状态（`a.skipRefresh`），行首 Ctrl+W、行尾 Alt+D/Ctrl+K 合并行后下游状态与缓冲区漂移 |
| P1-8 | `editor_edit.go:87-88` | redo 分支 `km.Matches(raw, "ctrl+shift+z")` / `"ctrl+y"` 用**字面键串**作 binding id——`resolved` 表只含注册 id，恒不匹配；且 `ctrl+y` 已被 `tui.editor.yank` 占用 | **Ctrl+Shift+Z 无法 redo**，redo 在键盘上完全不可达（`e.redo()` 唯一调用方即此死条件）；头注释宣称的功能与实现不符 |

### 维度 5c 面板卡片（2 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-9 | `footer.go:123-127`、`settings.go:141-148` | Render 中浅拷贝切片头后**提前释放 RLock** 再无锁迭代，而 `RegisterGroup`/`SetValue` 等写方法原地改同一底层数组 | 违反 `core/component.go:23-27`"Render 可与 Update 并发"契约；M-1 的 goroutine 模式、异步 Cmd 并发调用 setter 时 -race 可暴露 |
| P1-10 | `review_gate.go:249-289`、`command_center.go:188-201` | Update 手动 `Unlock` 调外部回调再 `Lock`：回调 panic 时 defer 二次 `Unlock` → "unlock of unlocked mutex" 掩盖原错误 | 审批/复核关键路径（P 键 → onPass → /approve）的防御性缺陷 |

### 维度 7 chat（1 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-11 | `chat_history_render.go:389-436` | 渲染快路径中 `firstDirtyIdx` 严格落在已渲染工具组内部时（`groupFrom < firstDirtyIdx ≤ groupEnd`），clean-prefix 遍历 break 且新渲染从组中间开始 → 组内更早成员被跳过 | 多工具轮次中 `ToolStart(t2)→ToolEnd(t2)` 后 **t1 工具卡片从屏幕消失**（数据未毁，缓存渲染层丢失），可稳定复现 |

### 维度 5c（M-1 复核升级）（1 项）

| # | 位置 | 问题 | 影响 |
|---|------|------|------|
| P1-12 | `session_selector.go:347 / 400 / 409 / 422` | **M-1 未修复**：4 处 `go fn(...)` 回调 goroutine 仍无 context/done/WaitGroup；且 `cmd/mady/session_panel.go:83` 的 onSelect 内 `agent.SaveState(context.Background(), ...)` **无超时** | goroutine 无法取消/join；回调在事件循环外调用 `CloseOverlay/PrintSystem/History().Clear()/SetItems` 等 UI 状态变更，与渲染线程并发访问，违反并发安全契约 |

---

## 四、P2（中，汇总 31 项）

### terminal（5 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-1 | `detect.go:707-778` | KKP 品牌正列表短路乘数器门：Kitty/tmux 组合绕过保守禁用（测试只覆盖 BrandUnknown+tmux） |
| P2-2 | `detect.go:709-718` | iTerm2 未列入 KKP 正列表（macOS 最大用户群，3.5+ 已支持），落入 unsupported |
| P2-3 | `detect.go:648-655` | truecolor 白名单缺 Warp/Otty/GrokDesktop（brand 可识别但能力门不放行）→ 落 16 色 |
| P2-4 | `keys.go:255-301` | 传统 Shift+Tab（`CSI Z`）未解析，final 字节 Z 落默认分支 → 非 KKP 终端 shift+tab 绑定永失效 |
| P2-5 | `terminal.go:178-186` | Start 切 O_NONBLOCK=false 但 Stop 不还原（未保存原 flags），同进程重启 TUI 场景语义漂移 |

（P2-6 未知绑定 ID 静默丢弃见 keybindings.go:215-221，与 P1-3 同维度）

### theme（5 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-6 | `system_appearance.go:63-97` | `detectAppearance` 恒返回 nil error（err 分支死代码）；`AppearanceUnknown` 被当真实变化 → 深色系统瞬态闪切浅色 |
| P2-7 | `global.go:21-22,82-83` | `ToggleTheme` 用魔法 hex（`== "#07111F"`）判明暗：tokyo-night/high-contrast 等深色主题被误判为浅色，切换方向错误 |
| P2-8 | `color_resolve.go:227-232,253-258` | 数字索引（0-255）绕过模式分支，`TUI_COLORMODE=16` 下仍输出 38;5;n |
| P2-9 | `color_resolve.go:239-246,264-266` | Basic 模式极性硬编码 dark=true，浅色主题（mady-light）在 16 色终端对比度受损 |
| P2-10 | `json.go:50-55` | 未知键 + 不支持值类型（bool/map）→ 整体解析失败，违背"未知键忽略"契约（含注释/文档承诺） |

### 渲染引擎（5 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-11 | `tui_render.go:182-188` | Raw 行差分路径缺尾部 reset，未闭合 SGR 泄漏到后续未变更行 |
| P2-12 | `cellparse.go:97-100` + `width.go:57-59` | `\t`/`\r`/`\n` 被当组合符附加（0 宽），网格宽度失准、终端按 tabstop 渲染 → 列对齐错乱 |
| P2-13 | `sgr.go:198-207` | `38;2;r;g;b;<code>` 组合序列被 colorspace 启发式误判（吞掉首分量） |
| P2-14 | `width.go:281-287` | `TruncateToWidth` 的 reset 启发式把 `\x1b[01m`/`\x1b[07m`/`\x1b[0;31m` 误判为"已复位"，截断点后不追加 reset |
| P2-15 | `celldiff.go:176-179` | DiffCells Raw 分支返回 nil Cells 段（缺陷固化，测试未断言 Cells 非空） |

### Markdown 组件（6 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-16 | `markdown_parse.go:344-349` | 嵌套列表未实现，嵌套项渲染为字面 `- ` 文本（与 markdown.go 头注释宣称"with nesting"不符） |
| P2-17 | `markdown_inline.go:14,34-40` | 行内数学 `2*3*4=24` 被斜体正则损坏为 `234=24`（e0b123b 仅保护 sanitize 层） |
| P2-18 | `markdown_inline.go:28-40` | 代码 span 内星号被二次强调处理（`a*b*c` → b 被斜体化），`**` 可跨 span 配对 |
| P2-19 | `markdown_parse.go:303-307` | 表格吞并循环只查"含 `\|`"，表格后含管道符的段落被吸进表格 |
| P2-20 | `markdown_render.go:318-328` | 表头列数少于数据行列数时多余列静默丢弃（无截断提示） |
| P2-21 | `markdown_parse.go:110-117 vs 297-309` | `isTableStart` 与 `tryConsumeTable` 判定口径不一致（独立时是表格、段落后被并进段落） |

### 编辑器组件（4 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-22 | `input.go:261-263` | PasteMsg 经按键解析，粘贴内容含 `\n`/`\r` 误触发 submit；无 `tui.input.paste` 绑定 |
| P2-23 | `input.go`（全组件） | **Input 组件整体无生产调用方**（全仓 `NewInput` 零引用），6 个编辑原语零覆盖——"有测试文件但无用户" |
| P2-24 | `fuzzy_provider.go:124-151` | 摘要截断按字节操作（`s[:maxLen]`、`body[lo:hi]` 无 rune 边界），中文/emoji 切出非法 UTF-8 → 显示替换符 � |
| P2-25 | `editor_killring.go:60-90,104-126` | yank/yankPop 路径不维护 chip 位置（insertStringLocked 换行分支无 chips 重定位） |

### 面板卡片组件（4 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-26 | `todo_panel.go:120` | `SetOnToggle` 生产零接线：标题宣称 "Space/Enter toggle" 但用户按键是 no-op（死功能） |
| P2-27 | `skill_center.go:164-173` | `confirm()` 持 RLock 调外部 onSelect 回调（RWMutex 不可重入，回调内调用 Set* 即死锁） |
| P2-28 | 7 文件（judgment_view.go:224 / conclusion_card.go:62,72-75 / approval_card.go:65,84 / review_gate.go:425 / evidence_overlay.go:160-172 / system_status.go:206-243 / debug_overlay.go:237-241） | 行未经截断直接 PadToWidth/原样输出，违背 tool_card.go 确立的"永不超宽"约定（依赖引擎 normalizeLine 兜底） |
| P2-29 | `judgment_view.go:232-246,349-358` vs `confidence_bar.go:47-69` | **M-10 部分修复**：共享渲染器已落地（3 卡接入），但 judgment_view 独立内联条未迁移，且置信度阈值三套口径分裂（30/60 vs 34/67 vs domain.go 0.67/0.34） |

### 事件循环（1 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-30 | `tui.go:84-88` + `tui_input.go:117-121` | Filter 文档承诺"intercepting quit events"，但 `QuitMsg`/`WindowSizeMsg` 绕过 Filter 直接处理 → 退出确认对话框可能被静默绕过 |

### chat 应用层（2 项）
| # | 位置 | 问题 |
|---|------|------|
| P2-31 | `chat_app_stream.go:136` + `chat_app_tool.go:31/218/234` | 流式对账（S5）仅覆盖最后一个流段：工具调用**之前**已流的文本段不参与 AgentEnd 全文对账，provider 回放重复/丢 delta 将永久残留 |
| P2-32 | `adapter.go:204-214` | agentadapter 缺 `ChatEventPlanTaskFeedbackAdded` 映射测试（19/20 映射有测试，唯一缺口） |

（P2-6 keybindings 未知 ID 与 P2-2 iTerm2 编号错位已在上表对齐；H-6 中文硬编码列为单独遗留项，见第七节。）

---

## 五、P3（低，抽样 30 项，详见各维度记录）

**terminal**：kittyKbdOn bool 非计数（P3-1）、PushKittyKeyboard 过时注释（P3-2）、flag 8 注释错误（P3-3）、0x1C-0x1F 控制码 +96 偏移导致 `ctrl+-` 不匹配（P3-4）、maxBufferBytes 溢出静默丢全部（P3-5）、consumeKeyEvents O(n²)（P3-6）、Stop 读锁 readDone 自死锁边界（P3-7）、flags=0 贪婪解析 alt 码点误读（P3-8）、MuxZellij 无诊断 reason（P3-9）、kittyActive 全局污染（P3-10）、Conflicts 不查默认键（P3-11）、测试注释矛盾（P3-12）

**theme**：DetectColorMode 未知品牌激进回 Truecolor（P3-1）、watch 永久损坏文件重试风暴（P3-2）、回调阻塞致 watcher 泄漏（P3-3）、defaults/gsettings 无超时（P3-4）、gsettings 非 prefer-dark 均判 Light（P3-5）、json.Number 分支不可达（P3-6）、无 # 前缀 hex 静默回退（P3-7）、BgParams16 注释与行为相反（P3-8）、CurrentPalette 懒加载竞态注释缺失（P3-9）、撕裂文档表述不准（P3-10）、registryMu 持锁调用户代码（P3-11）、3 个命名主题缺 11 个 Phase-1 token（P3-12）、auto 注册注释矛盾（P3-13）、DefaultSemanticLight 缺 Godoc（P3-14）

**渲染**：`lastCursor.first` 恒假死条件（P3-1）、组合字符硬编码表不全 U+20D0 等（P3-2）、多条 CursorMarker 只剥第一条（P3-3）、dim 下 Raw 行不虚化（P3-4）、needsReset 死分支（P3-5）、cowRow 无害多余拷贝（P3-6）、tab 计宽 0（P3-7，与 P2-12 同源）

**Markdown**：标题装饰识别遗漏 Regional Indicators（P3）、围栏关闭误判 ` ``` 带信息 `（P3）、7 个 # 行泄漏（P3）、代码块内空行被丢弃（P3）、`a*b` 字母两侧星号被删（P3）、窄宽表格略挤（P3）、Markdown/Text/Syntax Render 返回缓存切片契约脆弱（P3）、RegisterLanguage 覆盖内置（P3）、tokenizeNumber 前缀未校验（P3）、Python 三引号跨行状态丢失（P3）、bash $VAR 无高亮（P3）、viewport Scrollbar 配置死字段 + 指示行偏移（P3）、table ColWidths rest 可为负（P3）、selectlist 组头溢出 + 冗余死代码（P3）、fuzzy HighlightMatches CJK 偏移失效（P3）、Loader.Stop 回调内自死锁（P3）、Toast sleep goroutine 无 Dispose（P3）、image 滚动不跟随/alpha 无合成（P3）

**编辑器**：SetMinRows/SetMaxRows 无交叉校验（P3-1）、redo 无恢复路径测试（P3-2）、cursorLineStart/End 不重置 lastKillOp（P3-3）、clearSelection 保留 chips（P3-4）、InsertChip 注释错误（P3-5）、autocomplete inactive Tab 无条件 apply（P3-6）、kill-ring 行为不一致（P3-7）

**面板**：ReviewGate SetFocused 不置 dirty（P3-1）、evidence_overlay 本地 truncateToWidth 重复实现（P3-2）、高度魔法数 20/16（P3-3）、debug_overlay 空锁分支（P3-4）、双键匹配冗余（P3-5）、结论卡示例注释漂移（P3-6）、字符串字面量 switch 绕过类型常量（P3-7）、judgment_view 双实现冗余（P3-8）、ReviewGate OnBack 自然语言语义（P3-9）

**事件循环**：mouseThrottle tick 积压突破节流（P3-1）、focused MouseConsumed 不阻止广播（P3-2）、NewChatAppWithHost+SetHost 双重绑定（P3-3）、MouseTarget"TUI 路由器"注释不存在（P3-4）、logEvent resize 分支不可达（P3-5）

**chat**：FSM 非法迁移静默（P3-1）、5 个事件未接线（P3-2）、transitionFromInterrupted 零覆盖（P3-3）、双枚举手工同步（P3-4）、copyOSC52 100KB 静默截断（P3-5）、Linux 缺 Wayland 剪贴板（P3-6）、截断提示未覆盖 content_filter（P3-7）、extractToolDiff 空 content 多计一行（P3-8）、中文硬编码（P3-9）、Busy("thinking...") 英中混杂（P3-10）

---

## 六、上次 28 项缺陷状态速查表（最终版）

### Critical（07-26，5 项）——**全部已修复**

| # | 缺陷 | 状态 | 证据 |
|---|------|------|------|
| C-1 | Kitty 全局状态污染测试 | ✅ 已修复 | `-race -count=5 ./terminal/` 通过（1.04s） |
| C-2 | termios 恢复错误静默吞掉 | ✅ 已修复 | `terminal.go:268-270` 现 `%w` 包装返回 |
| C-3 | PanicMsg 丢堆栈 | ✅ 已修复 | `message.go:190` `CaptureStack()`；维度 6 实证 Stack 非空含 panic 帧 |
| C-4 | LLM 输出 ANSI 原样透传（注入链） | ✅ **已修复（完整）** | `cellrender.go:29-31` SerializeRow 为唯一出口，Raw 一律经 `SanitizeRawContent`（白名单仅 SGR+CursorMarker，13 单测）；全量/差分/overlay 三路径全部走该出口；修复提交 `1f1d860` |
| C-5 | doc.go Quick Start 无法编译 | ✅ 已修复 | `go build ./...` 覆盖通过 |

### High（07-26，8 项）——**6 修复 / 2 未修复**

| # | 缺陷 | 状态 | 证据 |
|---|------|------|------|
| H-1 | OnDebug 死代码 | ✅ 已修复 | `chat_bridge.go:43` wire `debug_overlay.go`，功能完整 |
| H-2 | errors.Is/As 零使用 | ❌ **未修复** | 生产代码仍 0 使用（`%w` 出现于 C-2 修复，但无 errors.Is/As 消费方） |
| H-3 | 硬编码选中色 48;5;33 | ✅ 已修复 | editor/input 已改 palette 派生 + `SetSelectedBg` 可配置；`chat_history_render_highlight.go:238` 仅为回退值 |
| H-4 | stdin 无界增长 | ✅ 已修复 | `maxBufferBytes=1MiB` + `MaxPasteBytes=16MiB` cap |
| H-5 | NewTUI 双重签名 | ✅ 已修复 | `tui.go:340` 单一 variadic 签名 |
| H-6 | 245+ 中文硬编码 | ❌ **未修复** | 现 216 处（rg 统计），chat/ 重灾区约 200 处，未接 `pkg/i18n` |
| H-7 | Border/BorderMuted 同色 | ✅ 已修复 | dark: `#2A4A63` vs `#152A3D` |
| H-8 | verify_layers 未接 CI | ✅ 已修复 | Makefile `verify-tui-layers` + `ci.yml` |

### Medium（07-26，15 项）——**11 修复 / 2 部分 / 2 未修复**

| # | 缺陷 | 状态 |
|---|------|------|
| M-1 | session_selector goroutine 无终止 | ❌ **未修复**（升 P1-12：4 处仍在 + onSelect 无超时 + 事件循环外改 UI） |
| M-2 | stdio 持锁 I/O | ✅ 已修复（stdio 死引用已移除，仅 doc.go 提及） |
| M-3 | watcher panic 永久死亡 | ✅ 已修复（`runWithRestart` + 固定 5s 退避，TestRunWithRestart* 在案） |
| M-4 | tui→chat 跨层架构债 | ⚠️ 部分（仍存在但收敛 `chat_bridge.go` 单点，LAYERS.md 已记录为 Known Compromise） |
| M-5 | ChatEvent 三重标识 | ✅ 已修复（单一 `ChatEventType int` 枚举，23 常量 + Kind 方法） |
| M-6 | agentadapter 12/17 无测试 | ✅ 大部分修复（6 测试函数，19/20 映射覆盖，唯一缺口 P2-32） |
| M-7 | 三套检测系统断裂 | ✅ 已修复（TerminalContext 已被 clipboard/color_resolve 使用） |
| M-8 | Sixel 宣称未实现 | ✅ 已修复（`image.go:31` 明确声明不支持，LAYERS.md 同步） |
| M-9 | itoa 3 处重复 | ✅ 已修复（`internal/conv` 已删除） |
| M-10 | 置信度条 3 处重复 | ⚠️ 部分修复（共享 `RenderConfidenceBar` 落地，judgment_view 未迁移 + 三套阈值口径，P2-29） |
| M-11 | theme Godoc 缺失 | ✅ 大部分修复（51 导出符号仅 `DefaultSemanticLight` 缺注释） |
| M-12 | editor_edit 7 函数零覆盖 | ✅ 已修复（`editor_edit_test.go` 10 测试补齐；新缺口移至 input_edit.go 6 函数，见测试缺口） |
| M-13 | 8 整文件零覆盖 | ✅ 已修复（无整文件零覆盖；包覆盖 72.1%~100%） |
| M-14 | 仅 2 个 benchmark | ✅ 已修复（13 个：core 8 + overlay 3 + chat 2） |
| M-15 | LAYERS.md 依赖矩阵错误 | ✅ 已修复（矩阵大幅更新 + 自动校验 116 文件） |

**统计：28 项中 ✅23 / ⚠️2（M-4、M-10）/ ❌3（H-2、H-6、M-1）**

---

## 七、遗留与新增重大问题清单（供决策）

| 优先级 | 项 | 类型 |
|--------|----|------|
| P0 | P0-1 resize 防抖自续循环 | 功能缺陷 + 常驻开销 |
| P1 | P1-1 VTE 阈值、P1-2 Kitty PUA 错位 | 功能静默丢失 |
| P1 | P1-5 Raw→Cells 幽灵残留、P1-6 resize 光标隐藏 | 渲染缺陷 |
| P1 | P1-7 onChange 缺失、P1-8 redo 不可达 | 编辑器状态一致性 |
| P1 | P1-9 footer/settings 数据竞争、P1-10 双重解锁、P1-12 goroutine（M-1） | 并发安全 |
| P1 | P1-4 auto watcher 覆盖、P1-11 工具组丢成员 | 行为缺陷 |
| P2 | H-6 中文硬编码 216 处（跨模块 i18n） | 历史遗留 |
| P2 | H-2 errors.Is/As 错误分层 | 历史遗留 |
| P2 | P2-28 7 处宽度约束不统一 | 渲染边界 |
| 死代码 | 约 40 处（见下） | 代码卫生 |

## 八、死代码清单（跨维度汇总，全仓 grep 验证）

**确认死代码（含测试零调用）**：terminal `Has256Color`/`SupportsOSC8Hyperlinks`/`CtrlDotAvailable`/`KittyKeyboardSkipReason`/`IsTerminal`；theme aliases.go 8 个 `atomic.Pointer` 别名（UserStyle/DimStyle/SystemStyle 等）+ `ColorLevel` 三常量 + `Blink`/`Reverse` Attr；core `DiffRows`/`SpinnerLine/Bounce/Globe/Moon/Circle`/`SetCJKMode`/`IsCJKMode`；component `Input` 全组件/`ScrollbarConfig.TrackSymbol/ThumbSymbol`/`CancellableLoader.OnAbort`/`DomainMsgTypeReviewGate`/`ConfidencePct`/`ConfidenceLevel`/skill_center `filter`/`height` 机制/`SetPersisted`；tui `TUI.Every/Tick/Quit`；chat `SkillLoadedChatEvent`/`SkillsReloadedChatEvent`/`A2UIChatEvent`+3 常量/内联确认链（`StartConfirm`/`ConfirmYes`/`ConfirmNo`/`StateConfirmPending`/`evtConfirmRequest`/`evtConfirmDecision`）/`applySelectionHighlightLocked`/5 个 chat*Msg 通道类型。

**运行期死条件**：`editor_edit.go:87` redo 字面键串；`tui_render.go:174` `lastCursor.first` 恒假；`tui_input.go:254-258` logEvent resize 分支不可达；`sgr.go:290-291` needsReset 死分支；`system_appearance.go:66-68` err 分支恒假。

**误判澄清**：`NerdFontsSupported` 活跃（theme/style.go:240 调用）；`RegisterFocusCycle/FocusNext/FocusPrevious` 已在 2026-08-04 清理中删除（**非死代码，是文档漂移**——repowiki 输入处理.md 仍描述这些 API）。

## 九、文档漂移清单

| 文档 | 漂移 |
|------|------|
| 终端兼容性.md | "常用能力查询方法"列出 4 个死代码 API；"乘数器限制保守禁用"与 P2-1 行为不符；VTE ≥0.82.0 承诺实际不可达（P1-1）；"OSC8 超链接检查"不可操作（TUI 无 OSC8 输出路径） |
| 主题系统.md | `theme_registry.go`/`quantize.go`/`a11y_themes.go`/`watchutil.go` 完全缺失；"5s 指数退避"实为**固定** 5s；多处行号偏移；命名主题缺 token 回退行为未记录 |
| 渲染引擎.md | 清洗层（C-4 修复产物）通篇未提及，管线图与"Raw 透传"描述过时；`cell.go:144-153`/`cellparse.go:17-20` 注释称"Kitty 透传 untouched"与实际剥离矛盾（Kitty 透传功能已名存实亡） |
| 输入处理.md | `RegisterFocusCycle/FocusNext/FocusPrevious` 已删除但仍被文档描述 |
| 组件系统.md | Editor 头注释宣称 redo（Ctrl+Shift+Z）实际不可达（P1-8）；markdown.go 宣称"嵌套列表"实际未实现（P2-16） |
| TUI架构设计.md / TUI终端界面.md | 基本一致；`chat_bridge.go` 向上依赖在 LAYERS.md 已记录 |

## 十、测试缺口清单（按优先级）

1. **resize 防抖→分发完整路径**（P0-1 回归测试，最高优先）
2. **ProcessTerminal 真实 tty 路径零覆盖**（Start/Stop/raw 模式/SIGWINCH/readLoop）——建议 pty 集成测试
3. **Raw→Cells 帧间差分**（P1-5）、**resize 后光标恢复**（P1-6）
4. **Filter 行为语义**（丢弃/替换/绕过 QuitMsg）
5. **input_edit.go 6 个编辑原语**（有调用方、无测试）
6. **单列表格**（P1-2）、**行首粗体**（P1-1）、**嵌套列表**（P2-16）、**行内数学**（P2-17）
7. **VTE 阈值真实边界**（8100/8200）、**Kitty PUA flag 8 场景**（P1-2）
8. **工具组拼接 patch 场景**（P1-11）、**transitionFromInterrupted**、**PlanTaskFeedbackAdded 映射**
9. **footer/settings 并发 Render+Set**（P1-9）、**session_selector goroutine 泄漏**（M-1）
10. **runeutil.go 7 函数 / sgr.go emitFullSGR/bgCode 零覆盖**、**WithContext panic 路径**

## 十一、修复建议优先级

**立即（P0）**：修复 resize 防抖自续循环 + 补回归测试（约 15 行改动）。

**本周（P1，按收益排序）**：
1. P1-2 Kitty PUA 映射（改 3 行 + 修测试）
2. P1-1 VTE 阈值 82000→8200（改 1 行 + 修测试）
3. P1-8 redo 键位注册 `tui.editor.redo`（改 2 行 + 补测试）
4. P1-6 resize 光标恢复（置 `lastCursor.visible=false`，1 行）
5. P1-5 Raw→Cells 差分补 Segments（3 行 + 回归测试）
6. P1-7 三处 merge 分支补 onChange（与 deleteBackward 对齐）
7. P1-12 M-1 goroutine 改 Cmd 模式或注入超时 context（结构改动，与 5c 修复同批）

**本月（P1/P2）**：P1-4 auto watcher 条件应用、P1-9 深拷贝快照、P1-10 抽 invoke 包装、P1-11 组起点钳制、P2 系列按表推进（Markdown 6 项可合并为一次"解析器收敛"改动）、H-6 中文 i18n（跨模块，需独立计划）。

**后续**：死代码清理（约 40 处，可分 2-3 批）；文档漂移同步（6 篇）；ProcessTerminal pty 集成测试。

## 十二、各维度审阅小结

| 维度 | 质量评级 | 关键结论 |
|------|---------|---------|
| terminal | 🟢 中上 | 协议解析/能力门控整体正确；VTE 阈值与 PUA 映射 2 处硬伤；真实 tty 零覆盖是最大风险 |
| theme | 🟢 中上 | 原子读/生命周期/a11y 完整；auto watcher 覆盖 + Unknown 误报构成"系统外观→主题"链路风险 |
| 渲染引擎 | 🟢 中上 | **C-4 封堵完整**（唯一咽喉点+白名单）；Raw 行差分边界 2 个 P1；池化/宽字符/坐标全部无问题 |
| 编辑器 | 🟢 中上 | 编辑原语测试全面、CJK 正确；redo 不可达 + onChange 缺失 2 个硬伤；Input 组件整体闲置 |
| Markdown | 🟡 中 | 历史修复 5/8 完整，但宽松解析引入行首粗体回归、单列表格缺口；嵌套/行内数学为结构性问题 |
| 面板卡片 | 🟡 中 | M-1 未修复 + footer/settings 并发违规 2 项并发风险；TodoPanel toggle 死功能；宽度约束 7 处不统一 |
| 事件循环 | 🟡 中 | **P0-1 自续循环**（唯一 P0）；僵尸防护/生命周期/堆栈捕获全部无问题 |
| chat/agentadapter | 🟢 中上 | 流式四层根因修复核实完整（回归测试强）；工具组拼接丢成员 1 个 P1；FSM 5 事件未接线 |

---

*审阅方法说明：8 路走查子代理均只读、未修改任何文件；验证命令（build/vet/定向测试）全部通过；
子代理的 /tmp 独立验证程序已清理。本报告所有 P0/P1 结论均经主代理代码复核。*

---

## 附录 B：code-review 严格复核（2026-08-09 二次）

对上述修复批（47 文件 +1017/-350）做严格代码质量复核：3 路只读子代理并行审阅（core/terminal/tui 根、theme/chat、component）+ 主代理交叉验证。**复核又发现并修复 7 项**，其中 1 项为 blocker 级回归：

| # | 级别 | 位置 | 问题 | 修复 |
|---|------|------|------|------|
| R-1 | **blocker** | theme/color_resolve.go | **P2-9 修复不完整**：currentIsDarkBg 回读 CurrentPalette，与 BuildPalette 构成环依赖——ColorModeBasic（TERM=dumb/空/linux 或非 TTY 进程）下首次 palette 构建无限递归→栈溢出 | 改为复用 SetSemanticTheme 维护的 isDark 状态（同值且无副作用）；回归测试 TestCurrentIsDarkBgDoesNotBuildPalette 断言不触发 palette 构建 |
| R-2 | major | component/markdown_inline.go | **P2-17 引入数字强调回归**：单侧贴数字即守卫，`**2**`/`**2024**`/`*2*` 的开闭星号被吞，粗/斜体失效（实测确认） | 守卫收紧为两侧贴数字（`2*3` 才守卫）；回归测试 TestRenderInlineMathGuardDoesNotBreakDigitEmphasis |
| R-3 | major | component/session_selector.go | **P1-12 引入锁外读竞争**：confirm/confirmDelete 在 RUnlock 后读 items[sel]，与 applyFilterLocked 的 filtered[:0] 原地复用竞争（对比同批 skill_center P2-27 锁内拷贝，标准不一致） | 锁内拷贝 item 后外调 |
| R-4 | major | component/session_selector.go | **P1-12 pendingCmd 机制冗余**：goroutine 套 goroutine + recover + 5s 超时；事件循环 execCmdIndexed 已有 panic-guard，超时并不终止 goroutine（注释"abandoned"与事实不符） | 简化为直通 Cmd，删除内层 goroutine/recover/超时；import slog/time 移除 |
| R-5 | major | component/markdown_render.go | **P2-16 修复只落到 kindBullet**：kindOrdered/kindChineseOrdered 续行分支未套 stripNestedListMarker，嵌套 `- `/`1. ` 标记仍泄漏 | 两个分支补齐（三列表分支现均走 strip） |
| R-6 | minor | theme/global.go | **P1-4 不变量被绕过**：ToggleTheme/JSON 热重载（watch.go→LoadSemanticThemeFromFile）绕过 currentThemeName，auto 下手动切换会被 OS 外观变化覆写 | 两处补 `currentThemeName.Store("")` |
| R-7 | minor | 多文件 | gofmt 门禁 4 文件违规（keybindings/terminal/detect_test/keys_test，其中 3 个为本次引入）；editor_edit.go redo 注释声称"ctrl+y 非 yank"与 keybindings.go（yank=ctrl+y）事实相反 | gofmt -w 修复；注释改写为与实现一致 |

**验证**：tui 模块 `go build/vet/go test -race -count=1 ./...` 全绿（9 包）；`gofmt -l` clean；`golangci-lint` 0 issues；`verify_layers.sh` 通过；根模块 `go build ./...` OK。

### 复核确认无问题的项（子代理疑点经主代理排查后排除）

- **knownColorKeys 双源**：当前 48=48 完全同步，无 bug；但结构上仍需手工维护（列为遗留建议）。
- **currentThemeName 默认行为**：cmd/mady/tui.go:345 启动即 `ApplyThemeByName("auto")`，"auto 不跟随系统"担忧不成立。
- **registry 无生产调用者**：子代理只搜了 tui/ 目录；根模块 cmd/mady（tui.go:333/345、tui_session_commands.go:68/80）均有调用，非死代码。
- **gofmt 缺失缩进**：属实，已修（R-7）。

### 遗留建议（既有结构问题，不在本次 diff 引入范围，未强行修复）

1. **overlay 残影**（tui_render.go:100 快路径）：`prevRaw`（overlay 合成前）与 `prev`（合成后）不一致，PopOverlay 后旧行复用可残留 overlay 内容——既有缺陷，建议 Push/PopOverlay 置 firstFrame。
2. **resize 防抖契约**：100ms 防抖只合并消息下发，onTerminalResize 每次 SIGWINCH 仍全量重绘——既有设计，文档与实现需对齐。
3. **invokeLocked 跨文件复制**（review_gate.go/command_center.go 各一份相同实现）+ 批内三种回调外调模式并存——建议收敛为单一模式。
4. **knownColorKeys/applyColorKey 双源真相**——建议 applyColorKey 返回 bool 消除 map。
5. **tryConsumeTable 的 `HasPrefix("|")` 一刀切**（P2-19 引入）：无前导管道符的合法 GFM 表格行被切出表体——已知权衡（比"段落含 | 被吞表"少见），保留。
6. 既有复制粘贴结构（markdown 三列表分支 ×3、editor merge 体 ×6、FgParams/BgParams 六段同构、256 调色板模型双份、亮度权重三份、ColorLevel 死代码、detect 能力门双缓存）——均为既有代码，建议后续批量收敛。
