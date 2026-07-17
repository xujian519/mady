# Mady TUI 设计规划（已归档）

> **归档日期**: 2026-07-18
> **归档原因**: Phase 1-6 未执行，5 个待决策问题悬置。当前 TUI 功能完整可用（8 层 Elm 架构、20+ 组件、主题系统、slash 命令），此设计规划的 Sidebar/Focus 统一等大改动暂不启动。
> **决策**: 归档保存设计思路，未来如需重构 TUI 布局可参考。部分 Phase 4 标记（statusbar 增强字段、sidebar）已汇总至 [docs/decisions/phase4-backlog.md](../decisions/phase4-backlog.md)。
> **替代方案**: 当前 `chatLayout` 垂直布局已满足使用需求，`Overlay` 浮层承载会话选择/技能/待办/设置等辅助功能。

---

# Mady TUI 设计规划

> 基于项目 Logo 视觉语言，为 Mady（中观智能体）规划终端用户界面（TUI）的下一阶演进。
> 范围：TUI 入口、布局、导航、组件与主题系统；不涉及 Web/GUI。

---
（原始内容保留，共 294 行，详见下方）
---

## 1. 背景与目标

Mady 是面向专利/法律专业场景的 Go 1.25 Agent 运行时框架。当前 TUI 入口为 `cmd/mady/main.go` 的 `runTui()`，启动 `chat.ChatApp`；自研组件体系位于 `tui/`，未使用 Bubble Tea，事件驱动（`Msg`/`Update`/`Cmd`），组件渲染为 `[]string`。

本次设计目标：

1. 把 Logo 的"汇聚、中道、冷光"意象转译为 TUI 视觉与交互语言。
2. 在现有自研组件基础上，构建更清晰的信息架构：从"单一聊天界面"演进为"可导航的专业 Agent 工作台"。
3. 统一主题、组件、动效与无障碍标准，为后续 GUI 留下可复用的设计 token。
4. 保持小步快跑：每次改动限定在 3–5 个文件内，先验证再扩展。

---

## 2. 现有 TUI 资产盘点

| 层级 | 位置 | 关键资产 |
|------|------|----------|
| 入口 | `cmd/mady/main.go` | `runTui()`、`ChatAppConfig`、slash 命令处理（`/help`、`/clear`、`/thinking`、`/theme`、`/plan`、`/quit`） |
| 应用 | `tui/chat/` | `ChatApp`、`chatLayout`、`ChatHistory`、reasoning 渲染 |
| 核心 | `tui/core/` | `Component` 接口、事件模型、单元格模型 |
| 组件 | `tui/component/` | `Box`、`Editor`、`SelectList`、`Table`、`Autocomplete`、`Loader`、`StatusBar`、`KeyHelp`、`SettingsList`、`SessionSelector`、`SkillCenter`、`TodoPanel`、`Image`、`Markdown`、`Overlay` |
| 主题 | `tui/theme/` | `SemanticTheme` + `Palette`、JSON 加载、深浅色切换 |
| 示例 | `example/tui-demo*` | 早期组件用法参考 |

当前布局 `chatLayout` 的垂直顺序为：

`header → history → autocomplete → loader → editor(border) → footer → statusBar`

所有内容都堆叠在单一聊天上下文里，导航靠 slash 命令和 Overlay 浮层完成。这对"开箱即用"够用，但当会话、技能、待办、设置增多后，认知负荷会上升。

---

## 3. 从 Logo 提取视觉语言

Logo 为两条蓝色发光弧线从左右向中心收束，形成对称汇聚图形；中心亮度最高，末端渐隐。

| 维度 | 观察 | 设计转化 |
|------|------|----------|
| **形态** | 对称双弧、向中心汇聚 | 布局采用"两侧向中心聚焦"：左侧导航 → 中央对话 → 右侧/底部状态；重要操作居中高亮 |
| **负空间** | 两弧之间形成"中"字意象 | 用"中心线"或"聚焦态"表示当前 Agent/任务；避免左右信息过载 |
| **色彩** | 深蓝背景 + 青蓝/冰蓝光弧 + 白色高光 | 深色主题以"深空蓝"为底、"青蓝光"为强调色；浅色主题以"冰白/浅灰"为底、"深蓝"为强调色 |
| **动势** | 从两端向中心收敛 | 转场、加载、光标动效采用"向中心收束"或"脉冲扩散" |
| **氛围** | 理性、深邃、专业、略带禅意 | 减少暖色装饰；用冷色、低饱和灰、清晰边框传达律令感 |

**设计关键词**：汇聚、收敛、对称、中道、弧光、深邃、理性、律令。

> 注意：当前默认深色主题使用暖琥珀（amber）作为 accent，与 Logo 冷色不一致。建议将默认深色改为冷色主题，暖琥珀可作为可选"柔和主题"保留。

---

## 4. TUI 信息架构

### 4.1 总体布局：Sidebar + Main Workspace

```text
┌────────────────────────────────────────────────────────────┐
│  header: Mady · mode=router · provider=openai · model=gpt  │
├──────────┬───────────────────────────────────────────────┤
│ Sidebar  │  Main Workspace                               │
│  [会话]   │  ┌─────────────────────────────────────────┐  │
│  [技能]   │  │ ChatHistory                             │  │
│  [待办]   │  │ ...                                     │  │
│  [设置]   │  │                                         │  │
│          │  └─────────────────────────────────────────┘  │
│          │  autocomplete / loader / editor / footer      │
│          │  statusBar                                    │
└──────────┴───────────────────────────────────────────────┘
```

- **Sidebar（左侧导航）**：会话、技能中心、待办、设置、（未来）项目/案件。宽度固定 20–24 列，可折叠。
- **Header（顶部）**：Logo 水印/标题 + 当前模式 + provider/model。
- **Main Workspace**：以聊天为主，未来可扩展为"分屏"或"详情页"。
- **Editor + StatusBar（底部）**：保持当前 chatLayout 的底部输入与状态提示。
- **Overlay（浮层）**：保留浮层系统用于快捷键帮助、大图预览、表单弹窗、确认对话框。

### 4.2 导航模型

| 模式 | 说明 | 当前实现 | 目标 |
|------|------|----------|------|
| **Slash 命令** | `/help`、`/clear`、`/theme` 等 | 已存在 | 保留，作为全局快捷入口 |
| **Sidebar 切换** | `Tab`/`Shift+Tab` 或数字键切换面板 | 无 | 新增，替代部分浮层 |
| **Overlay 弹层** | 帮助、设置、会话选择、技能详情 | 已存在 | 精简为"详情/配置"场景；列表类移入 Sidebar |
| **Focus 模式** | 当前是 editor 常驻焦点 | 已存在 | 支持多焦点：editor、history、sidebar、overlay |

### 4.3 屏幕/视图映射

| 视图 | 现有组件 | 规划位置 | 说明 |
|------|----------|----------|------|
| 聊天 | `ChatHistory` + `Editor` | 主工作区 | 不变，持续优化 |
| 会话管理 | `SessionSelector` | Sidebar → 会话面板 | 从全屏 Overlay 改为常驻可折叠面板 |
| 技能中心 | `SkillCenter` | Sidebar → 技能面板 | 展示可用 skills，支持 @ 提及 |
| 待办 | `TodoPanel` | Sidebar → 待办面板 | 展示当前任务清单 |
| 设置 | `SettingsList` | Overlay / Sidebar 可切换 | 简单设置放 Sidebar；复杂配置弹层 |
| 帮助 | `KeyHelp` | Overlay | 保留浮层，统一快捷键提示 |

### 4.4 响应式策略

终端宽度有限，需要分断点：

| 宽度 | 行为 |
|------|------|
| `≥ 120` | 完整 Sidebar + 主工作区 + 右侧辅助列（未来可扩展） |
| `80–119` | 完整 Sidebar（20 列） + 主工作区 |
| `60–79`  | 折叠 Sidebar 为图标栏（8 列），hover/快捷键展开 |
| `< 60`   | 隐藏 Sidebar，通过 `Ctrl+Space` 或 `/panel` 切换浮层 |

---

## 5. 设计系统

### 5.1 色彩

基于 Logo 重新设计默认主题，保持 `SemanticTheme` 字段不变，只调整 hex 值。

**深色主题（默认）**

| Token | 建议色 | 用途 |
|-------|--------|------|
| `Background` | `#0B1120` | 背景 |
| `Surface` | `#111827` | 面板、Sidebar |
| `Border` | `#1E3A5F` | 普通边框 |
| `BorderAccent` | `#22D3EE` | 聚焦边框、高亮分隔 |
| `Accent` | `#38BDF8` | 用户消息、强调文字、按钮 |
| `Text` | `#E2E8F0` | 正文 |
| `Dim` | `#64748B` | 提示、次要信息 |
| `Error` | `#F87171` | 错误 |
| `Success` | `#34D399` | 成功 |
| `Warning` | `#FBBF24` | 警告 |
| `ThinkingText` | `#94A3B8` | 推理块文字 |
| `MdCode` | `#A5F3FC` | 行内代码 |
| `MdCodeBlock` | `#22D3EE` | 代码块文字 |
| `LoaderSpinner` | `#38BDF8` | 加载动画 |

**浅色主题**

| Token | 建议色 | 用途 |
|-------|--------|------|
| `Background` | `#F8FAFC` | 背景 |
| `Surface` | `#FFFFFF` | 面板 |
| `Border` | `#CBD5E1` | 普通边框 |
| `BorderAccent` | `#0284C7` | 聚焦边框 |
| `Accent` | `#0EA5E9` | 强调 |
| `Text` | `#0F172A` | 正文 |
| `Dim` | `#64748B` | 次要 |

> 暖琥珀主题可作为 `amber` 主题文件保留，通过 `/theme amber` 切换，满足老用户偏好。

### 5.2 字形与排版

终端内无法使用自定义字体，但可控制：

- **粗细**：标题/关键词 `Bold`，正文默认，提示/引用 `Italic`。
- **颜色**：通过前景色区分角色（User=Accent、Assistant=Text、System=Warning、Error=Error）。
- **下划线/反色**：用于链接、焦点、选中项。
- **行高**：列表项 1 行，卡片内 1–3 行，代码块按内容。
- **对齐**：标题左对齐，数字右对齐，状态居中。

### 5.3 间距与边框

- 基础单位：1 行 / 1 列。
- 面板内边距：1 行、2 列。
- 组件间距：1 行。
- 边框：单线框（`│├─┤`），聚焦面板用 `BorderAccent`。
- 圆角：终端内不可用，用"顶部/底部双线"或"标题栏背景色"模拟卡片感。

### 5.4 组件规范

| 组件 | 规范 |
|------|------|
| **Sidebar** | 左侧固定宽度，标题行高亮，选中项反色 + accent 边框，可折叠 |
| **Panel** | 带标题栏的框，标题用 `Bold` + `Accent`，内容区 1 行内边距 |
| **TabBar** | 顶部或 Sidebar 内切换，当前 tab 下划线/反色 |
| **Button** | `[ 确认 ]` 样式，聚焦时 `[ 确认 ]` 反色 |
| **List/SelectList** | 当前项 `SelectHighlight`，描述文字 `Dim` |
| **Editor** | 底部固定，prompt 用 `Accent`，placeholder 用 `Dim` |
| **ChatHistory** | 按角色分色，支持 Markdown、代码块、思考块、工具折叠 |
| **Loader** |  Spinner + "thinking..." 文字，使用 `LoaderSpinner` |
| **StatusBar** | 底部一行，左侧标题/模式，右侧系统状态/错误提示 |

### 5.5 动效

TUI 的动效依赖帧重绘，应克制：

- **流式光标**：输入区垂直条或方块，频率 500ms。
- **思考块展开/折叠**：`▶`/`▼` 符号切换，高度动画逐行展开。
- **工具调用折叠**：可折叠为单行，点击展开详情。
- **加载动画**：Spinner 旋转，采用 `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`。
- **Sidebar 展开/折叠**：宽度动画（可选），或即时切换。
- **焦点切换**：边框颜色变化，1 帧完成，避免闪烁。

### 5.6 无障碍

- **焦点可见**：所有可交互组件必须显示焦点边框或反色背景。
- **键盘导航**：`Tab`/`Shift+Tab` 在组件间移动；`Enter` 激活；`Esc` 返回/关闭。
- **高对比度模式**：提供 `/theme high-contrast` 主题，确保 4.5:1 对比度。
- **屏幕阅读器**：关键状态变化（如错误、模式切换）通过 `PrintSystem` 输出文本。
- **色彩不唯一**：状态不仅用颜色，也使用图标/文字前缀（如 `⚠`/`✓`/`✗`）。

---

## 6. 实施阶段

### Phase 1：主题与 Logo 对齐（1–2 天）

- 新增 `LogoDark` 和 `LogoLight` 两套语义主题，替换默认深色主题。
- 保留 `Amber` 主题作为可选，通过 `/theme` 切换。
- 调整 `StatusBar` 与 `ChatHistory` 的默认配色，确保在新主题下可读。
- 验收：`/theme dark`、`/theme light`、`/theme amber` 切换后所有组件正常渲染。

涉及文件：`tui/theme/semantic.go`（或新增文件）、`cmd/mady/main.go` 的 `/theme` 处理、`tui/theme/palette.go` 相关。

### Phase 2：布局抽象（2–3 天）

- 在 `tui/core/` 引入 `Layout` 接口：`Render(width, height)`、`Invalidate()`、`FocusNext()`、`FocusPrev()`。
- 新建 `tui/layout/sidebar_layout.go`：实现 Sidebar + Main Workspace 布局。
- 改造 `chatLayout` 为 `mainWorkspaceLayout`，作为 sidebar_layout 的右侧内容。
- 保持 `ChatApp` 最小改动：仅替换布局根节点。

涉及文件：`tui/core/layout.go`（新）、`tui/layout/sidebar_layout.go`（新）、`tui/chat/chat_app.go` 的 `chatLayout`。

### Phase 3：Sidebar 组件与导航（3–4 天）

- 实现 `Sidebar` 组件：可折叠、Tab 切换、快捷键 `Ctrl+1..5` 切换面板。
- 将 `SessionSelector`、`SkillCenter`、`TodoPanel` 适配为 Sidebar 面板内容。
- 保留 `SettingsList` 的 Overlay 入口，同时在 Sidebar 提供"快速设置"摘要。
- 验收：键盘可切换 Sidebar 面板，不影响 editor 输入。

涉及文件：`tui/component/sidebar.go`（新）、`tui/component/session_selector.go`、`tui/component/skill_center.go`、`tui/component/todo_panel.go`。

### Phase 4：Focus 与交互统一（2 天）

- 在 `tui/core/` 引入 `FocusManager`，支持 editor、history、sidebar、overlay 之间焦点切换。
- 统一 `Tab`/`Shift+Tab`、`Esc`、`Ctrl+Space` 行为。
- 为 `Box`、`SelectList`、`Editor` 增加焦点样式。
- 验收：所有交互有可见焦点；无焦点丢失。

涉及文件：`tui/core/focus.go`（新）、`tui/component/box.go`、`tui/component/selectlist.go`、`tui/component/editor.go`。

### Phase 5：动效与无障碍（2 天）

- 实现思考块/工具块的展开动画。
- 实现 high-contrast 主题。
- 添加键盘导航测试。
- 验收：`go test -race ./tui/...` 通过；高对比度下可辨识所有状态。

涉及文件：`tui/theme/`、`tui/component/chat_history.go`、`tui/component/loader.go`、测试文件。

### Phase 6：设计验证与文档（1–2 天）

- 更新 `docs/design/README.md` 索引。
- 编写组件使用示例与主题 token 表。
- 进行一次走查：从启动到切换主题、切换面板、使用 slash 命令。
- 同步更新 `docs/decisions/AI_CHANGELOG.md`。

---

## 7. 验收标准

- [ ] `go build ./...` 通过。
- [ ] `go test -race ./...` 通过（TUI 并发相关代码必须带 `-race`）。
- [ ] `golangci-lint run` 通过。
- [ ] `/theme dark`、`/theme light`、`/theme amber` 切换后无可见异常。
- [ ] 键盘可完成：切换 Sidebar 面板、聚焦 editor、打开/关闭 Overlay、滚动历史。
- [ ] 所有新增组件均有单元测试覆盖核心渲染逻辑。
- [ ] 设计文档与 `AI_CHANGELOG.md` 已更新。

---

## 8. 待决策问题（已随归档悬置）

1. **是否保留暖琥珀主题作为默认？** 建议：默认改为 Logo 冷色，amber 作为可选。
2. **是否需要 Nerd Font 图标？** 使用 `nf-fa-*` 图标可提升 Sidebar 识别度，但会降低通用终端兼容性。建议：提供配置开关，默认使用 ASCII 符号。
3. **是否需要在 TUI 启动时显示 Logo 动画？** 可作为启动画面（splash），但需控制时长，避免拖慢启动。
4. **Sidebar 是否支持鼠标？** 当前 `tui` 已支持鼠标事件，建议 Sidebar 支持鼠标点击切换。
5. **项目/案件（Project/Case）面板是否在本次实现？** 建议先预留接口，不实现具体业务，避免一次改动过大。

---

## 9. 参考文件

- `cmd/mady/main.go`：TUI 入口与 slash 命令。
- `tui/chat/chat_app.go`：`ChatApp` 与 `chatLayout`。
- `tui/theme/theme.go` 与 `palette.go`：主题系统。
- `tui/core/component.go`：组件接口。
- `tui/component/`：现有组件库。
- `docs/tone-style-guide.md`：面向用户文案规范。
