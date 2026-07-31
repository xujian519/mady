# Mady 桌面端设计开发依据

> **文档版本**: v1.0
> **日期**: 2026-07-28
> **状态**: ✅ 已适配 — 可直接作为开发依据
> **依据来源**: `codex-desktop-pixel-perfect-design-spec.md`（v2.1，8510 行）
> **适配目标**: 映射到项目现有代码，作为桌面端开发的唯一视觉标准

---

## 目的与使用方法

本文档将 `codex-desktop-pixel-perfect-design-spec.md`（8510 行的像素级设计规范）**适配为本项目可直接使用的开发依据**。它解决了三个关键问题：

1. **Token 映射**：将设计规范中的 Token 名映射到项目现有的 CSS 变量和 TypeScript 类型
2. **差距清单**：指出当前实现 `desktop/frontend/src/styles/globals.css` 与规范之间的差距
3. **增量指引**：说明哪些当前可以复用，哪些需要新增，按优先级排列

### 使用方法

- **前端开发**：先读 [§1 Token 映射表](#1-设计-token-映射表)，了解现有 CSS 变量与规范的对应关系
- **视觉走查**：打开 [§2 实现差距清单](#2-实现差距清单)，按优先级逐项补齐
- **新组件开发**：参考 [§3 组件规范速查](#3-组件规范速查)，直接引用规范中的组件参数
- **走查验收**：使用 [§4 Codex 对标走查表](#4-codex-对标走查表) 进行验收

---

## 1. 设计 Token 映射表

### 1.1 语义色 Token

> 列说明：**设计规范 Token** = source 文档中定义的语义 Token；**当前 CSS 变量** = `globals.css` 中已定义的变量；**状态** = ✅ 已存在 / ⚠️ 值不完全匹配 / ❌ 缺失

| 设计规范 Token | 当前 CSS 变量 | Light 值（规范） | Light 值（当前） | Dark 值（规范） | Dark 值（当前） | 状态 | 说明 |
|---------------|--------------|-----------------|-----------------|----------------|----------------|------|------|
| `bg/base` | `--color-mady-bg-primary` | `#F5F2EE` 暖米灰 | `#FFFFFF` 纯白 | `#1C1A18` 暖深灰 | `#000000` 纯黑 | ⚠️ | 规范使用暖色调，当前为纯色。**此为最大视觉差异** |
| `bg/surface` | `--color-mady-bg-secondary` | `#FAF8F5` | `#F2F2F7` | `#1A1816` | `#1C1C1E` | ⚠️ | 规范暖色，当前冷灰 |
| `bg/elevated` | `--color-mady-bg-tertiary` | `#FFFFFF` | `#FFFFFF` | `#252320` | `#2C2C2E` | ⚠️ | Light 一致，Dark 规范偏暖 |
| `bg/sidebar` | ❌ 缺失 | `rgba(245,242,238,0.72)` | — | `rgba(26,24,22,0.85)` | — | ❌ | 侧栏毛玻璃背景，需新增 |
| `bg/sidebar-solid` | ❌ 缺失 | `#F0EDE8` | — | `#1A1816` | — | ❌ | 侧栏非玻璃态备用 |
| `bg/hover` | ❌ 缺失 | `rgba(0,0,0,0.04)` | — | `rgba(255,255,255,0.06)` | — | ❌ | 列表项、按钮 hover 背景 |
| `bg/active` | `--color-mady-accent-soft` | `rgba(88,86,214,0.08)` | `rgba(88,86,214,0.08)` | `rgba(88,86,214,0.12)` | `rgba(94,92,230,0.12)` | ✅ | 值匹配 |
| `bg/overlay` | ❌ 缺失 | `rgba(0,0,0,0.48)` | — | `rgba(0,0,0,0.64)` | — | ❌ | 模态框 backdrop |
| `bg/composer` | ❌ 缺失 | `#FFFFFF` | — | `#252320` | — | ❌ | 输入区背景（可复用 `bg/elevated`） |
| `text/primary` | `--color-mady-text-primary` | `#1A1814` | `#000000` | `#F0EDE8` | `#FFFFFF` | ⚠️ | 规范带暖调 |
| `text/secondary` | `--color-mady-text-secondary` | `#6B6560` | `rgba(0,0,0,0.55)` | `#8A8580` | `rgba(255,255,255,0.55)` | ⚠️ | 规范用实色（更好），当前用透明度 |
| `text/tertiary` | `--color-mady-text-tertiary` | `#A39E98` | `rgba(0,0,0,0.25)` | `#6B6560` | `rgba(255,255,255,0.25)` | ⚠️ | 同上 |
| `text/inverse` | ❌ 缺失 | `#FFFFFF` | — | `#1A1814` | — | ❌ | 按钮反白文字 |
| `text/accent` | 同 `--color-mady-accent` | `#5856d6` | `#5856d6` | `#4b4ac4` | `#5e5ce6` | ⚠️ | 规范 Dark 用较浅紫保证对比度 |
| `text/link` | `--color-mady-text-link` | `#5e5ce6` | `#007aff` 蓝 | `#6e6cf0` | `#0a84ff` 蓝 | ⚠️ | **应改为品牌紫** |
| `border/default` | `--color-mady-border` | `rgba(0,0,0,0.08)` | `rgba(0,0,0,0.10)` | `rgba(255,255,255,0.08)` | `rgba(255,255,255,0.10)` | ⚠️ | 透明度差 0.02，基本一致 |
| `accent/primary` | `--color-mady-accent` | `#5856d6` | `#5856d6` | `#4b4ac4` | `#5e5ce6` | ⚠️ | 规范 Dark 用 #4b4ac4 |
| `accent/primary-hover` | `--color-mady-accent-hover` | `#4b4ac4` | `#4b4ac4` | `#6e6cf0` | `#6e6cf0` | ✅ | 值匹配 |
| `accent/primary-glow` | `--color-mady-accent-glow` | `rgba(94,92,230,0.3)` | `rgba(88,86,214,0.18)` | `rgba(110,108,240,0.4)` | `rgba(94,92,230,0.22)` | ⚠️ | 透明度不同 |
| `status/success` | `--color-mady-success` | `#5856d6`（品牌紫） | `#34c759`（绿色） | `#4b4ac4` | `#30d158` | ⚠️ | **关键差异**：规范成功色用品牌紫，当前用绿色 |
| `status/warning` | `--color-mady-warning` | `#ff9500` | `#ff9500` | `#ff9f0a` | `#ff9f0a` | ✅ | 值匹配 |
| `status/error` | `--color-mady-danger` | `#ff3b30` | `#ff3b30` | `#ff453a` | `#ff453a` | ✅ | 值匹配 |
| `status/info` | `--color-mady-info` | `#007aff` | `#007aff` | `#0a84ff` | `#0a84ff` | ✅ | 值匹配 |
| `selection/bg` | ❌ 缺失 | `rgba(88,86,214,0.25)` | — | `rgba(94,92,230,0.30)` | — | ❌ | 文字选中高亮背景 |
| `focus/ring` | ❌ 缺失 | `0 0 0 2px rgba(88,86,214,0.3)` | — | `0 0 0 2px rgba(94,92,230,0.4)` | — | ❌ | 焦点环 |

### 1.2 特殊色 Token

| 设计规范 Token | 当前 CSS 变量 | 状态 | 说明 |
|---------------|--------------|------|------|
| `mcp/starting` | ❌ 缺失 | ❌ | `status/warning` #ff9500 |
| `mcp/ready` | ❌ 缺失 | ❌ | `status/success` #5856d6（品牌紫） |
| `mcp/failed` | ❌ 缺失 | ❌ | `status/error` #ff3b30 |
| `mcp/cancelled` | ❌ 缺失 | ❌ | `text/tertiary` #A39E98 |
| `connection/connected` | ❌ 缺失 | ❌ | `status/success` #5856d6 |
| `connection/connecting` | ❌ 缺失 | ❌ | `status/warning` #ff9500 |
| `connection/disconnected` | ❌ 缺失 | ❌ | `status/error` #ff3b30 |

### 1.3 阴影 Token

| 设计规范 Token | 当前 CSS 变量 | Light 值（规范） | 当前 Light 值 | 状态 | 说明 |
|---------------|--------------|-----------------|--------------|------|------|
| `shadow/card` | ❌ 缺失 | `0 1px 3px rgba(0,0,0,0.08)` | — | ❌ | 卡片阴影 |
| `shadow/floating` | `--shadow-mady-popover` | `0 4px 12px rgba(0,0,0,0.08)` | `0 4px 24px rgba(0,0,0,0.15)` | ⚠️ | 当前模糊度/透明度不同 |
| `shadow/modal` | ❌ 缺失 | `0 12px 40px rgba(0,0,0,0.12)` | — | ❌ | 弹窗阴影 |
| `shadow/drag` | ❌ 缺失 | `0 16px 48px rgba(0,0,0,0.16)` | — | ❌ | 拖拽阴影 |

### 1.4 排版 Token

| 规范 Token | 当前 CSS 变量 | 值（规范） | 值（当前） | 状态 |
|-----------|--------------|-----------|-----------|------|
| `font/ui` | `--font-sans` | `Inter, -apple-system, SF Pro Display, sans-serif` | `Inter, -apple-system, "SF Pro Text", sans-serif` | ⚠️ 字体栈顺序略有差异 |
| `font/mono` | `--font-mono` | `"JetBrains Mono", "Fira Code", monospace` | `"JetBrains Mono", "SF Mono", "Fira Code", monospace` | ✅ |
| `type/h1`=20px/600 | `--font-size-mady-h1: 22px` | 20px | 22px | ⚠️ |
| `type/h2`=16px/600 | ❌ 缺失 | 16px | — | ❌ |
| `type/body`=14px/400 | `--font-size-mady-body: 14px` | 14px | 14px | ✅ |
| `type/ui`=13px/400 | `--font-size-mady-ui: 13px` | 13px | 13px | ✅ |
| `type/small`=12px/400 | `--font-size-mady-small: 12px` | 12px | 12px | ✅ |
| `type/caption`=11px/400 | `--font-size-mady-caption: 11px` | 11px | 11px | ✅ |

### 1.5 圆角 Token

| 规范 Token | 值 | 当前实现 | 状态 |
|-----------|-----|---------|------|
| `radius/sm` | 6px | `--radius-sm: 6px` | ✅ |
| `radius/md` | 8px | `--radius-md: 8px` | ✅ |
| `radius/lg` | 10px | `--radius-lg: 10px` | ✅ **注意**：规范 `radius/lg` = 8px，当前 = 10px |
| `radius/xl` | 12px | ❌ 缺失 | ❌ |
| `radius/2xl` | 16px | ❌ 缺失 | ❌ |
| `radius/3xl` | 16px（弹窗） | ❌ 缺失 | ❌ |
| `radius/full` | 50% | ❌ 缺失 | ❌ |

> **注意**：规范中 `radius/lg` = 8px（工具卡片圆角），当前 `--radius-lg` = 10px。开发时使用规范值。

### 1.6 间距 Token

| 规范 Token | 值 | 当前实现 | 状态 |
|-----------|-----|---------|------|
| `space/1` | 4px | 直接在代码中使用 | ⚠️ 未定义 Tailwind 变量 |
| `space/2` | 8px | 同上 | ⚠️ |
| `space/3` | 12px | 同上 | ⚠️ |
| `space/4` | 16px | 同上 | ⚠️ |
| `space/6` | 24px | 同上 | ⚠️ |
| `space/8` | 32px | 同上 | ⚠️ |
| `space/12` | 48px | 同上 | ⚠️ |

> 建议在 `@theme` 中补充完整的 Tailwind spacing 映射。

### 1.7 动画 Token

| 规范 Token | 值 | 当前实现 | 状态 |
|-----------|-----|---------|------|
| `duration/fast` | 100ms | ❌ 缺失 | ❌ |
| `duration/normal` | 150ms | 已在 CSS 中硬编码 `transition-duration: 150ms` | ⚠️ 未定义 Token |
| `duration/slow` | 250ms | ❌ 缺失 | ❌ |
| `duration/stream` | 1000ms | ❌ 缺失 | ❌ |
| `ease/spring` | `cubic-bezier(0.4, 0, 0.2, 1)` | ❌ 缺失 | ❌ |
| `ease/bounce` | `cubic-bezier(0.34, 1.56, 0.64, 1)` | ❌ 缺失 | ❌ |

### 1.8 布局 Token

| 规范 Token | 值 | 当前 CSS 变量 | 状态 |
|-----------|-----|--------------|------|
| `layout/titlebar-height` | 38px | `--mady-titlebar-height: 38px` | ✅ |
| `layout/statusbar-height` | 40px | ❌ 缺失（当前 StatusBar 高度） | ⚠️ 规范 = 40px |
| `layout/sidebar-width` | 260px | `--mady-sidebar-width: 260px` | ✅ |
| `layout/sidebar-collapsed` | 48px | ❌ 缺失 | ❌ |
| `layout/agent-width` | 380px | `--mady-context-width: 320px` | ⚠️ 规范 380px，当前 320px |
| `layout/settings-nav-width` | 200px | ❌ 缺失 | ❌ |

> ⚠️ **重要布局差异**：规范右侧 Agent 面板宽度为 380px，当前为 320px。

---

## 2. 实现差距清单（按优先级）

> ✅ **2026-07-31 审计**：P0（1-5）与 P2（11-13）全部已实现（`globals.css` 117 个 `--mady-*` 令牌），
> 命名统一为 `--color-mady-*` 体系（如 `--color-mady-bg-hover`）。P1 中 7/8/9 已实现；
> 待办仅剩 #6（字号 Token 核对）与 #10（整体色温评估）。

### P0：阻碍发布 — 必须实现

| # | 差距 | 位置 | 工作量 | 建议方案 |
|---|------|------|--------|---------|
| 1 | **链接色用 iOS 蓝而非品牌紫** | `globals.css` | ✅ 已实现 | `--color-mady-text-link: #5e5ce6`（L）/ `#6e6cf0`（D） |
| 2 | **成功色用绿色而非品牌紫** | `globals.css` | ✅ 已实现 | `--color-mady-success: #5856d6`（L）/ `#4b4ac4`（D） |
| 3 | **侧栏背景色缺失** | 新增 | ✅ 已实现 | `--color-mady-bg-sidebar` + `--color-mady-bg-sidebar-solid` + `--color-mady-bg-composer` |
| 4 | **hover/active/overlay 背景缺失** | 新增 | ✅ 已实现 | `--color-mady-bg-hover` / `--color-mady-bg-active` / `--color-mady-bg-overlay` |
| 5 | **全局过渡动画缺失 Token** | `globals.css` | ✅ 已实现 | duration×5（fast/normal/slow/slower/stream）+ easing×4（spring/bounce/decelerate/step） |

### P1：重要 — 影响视觉体验一致性

| # | 差距 | 工作量 | 建议方案 |
|---|------|--------|---------|
| 6 | **中文字号/行高 Token 不完整** | 30min | 补充 `type/h1=20px`、`type/h2=16px`、`type/caption=11px` 等缺失字号 |
| 7 | **圆角系统不完整** | ✅ 已实现 | `--radius-xl=12px` / `--radius-2xl=16px` / `--radius-full=50%` |
| 8 | **阴影系统不完整** | ✅ 已实现 | `--shadow-mady-card/floating/modal/drag` 四级 |
| 9 | **布局 Token 未统一** | ✅ 已实现 | `--mady-statusbar-height: 40px`；Agent 面板宽度差异见上文 ⚠️ 说明（320 vs 380px，待视觉走查） |
| 10 | **整体色温不统一** | 1-2h | 评估是否将 Light/Dark 模式底色从纯色改为规范的暖色调（#F5F2EE / #1C1A18） |

### P2：优化 — 提升体验

| # | 差距 | 工作量 | 建议方案 |
|---|------|--------|---------|
| 11 | MCP/连接状态 Token 缺失 | ✅ 已实现 | `--color-mady-mcp-starting/ready/failed/cancelled` + `--color-mady-connection-connected/connecting/disconnected` |
| 12 | 文字选择高亮色缺失 | ✅ 已实现 | `--color-mady-selection-bg`（L 0.25 / D 0.30） |
| 13 | 焦点环缺失 | ✅ 已实现 | `--focus-ring: 0 0 0 2px rgba(88,86,214,0.3)` |
| 14 | 间距 Token 未映射到 Tailwind | 30min | 补充 `@theme` 中的 `--spacing-*` 或使用 Tailwind 的 `theme.extend.spacing` |

---

## 3. 组件规范速查

以下列出开发新组件时最常查询的组件参数，直接引用设计规范。

### 3.1 主壳层（App Shell）

| 组件 | 关键参数 | 规范值 | 当前实现 |
|------|---------|-------|---------|
| **TitleBar** | 高度 | 40px | 38px（`--mady-titlebar-height`）→ 建议改为 40px |
| | 背景 | `bg/surface` | `bg-secondary` |
| | 交通灯位置 | `x: 16, y: 14` | 需确认 Wails 配置 |
| | 标题字体 | Inter Semibold 13px | ✅ |
| **LeftSidebar** | 展开宽度 | 260px | ✅ `--mady-sidebar-width: 260px` |
| | 收起宽度 | 48px | ⚠️ 未实现收起模式 |
| | 背景 | 毛玻璃 `bg/sidebar` + `backdrop-filter: blur(20px)` | 当前使用 `mady-material` class |
| | Tab 高度 | 40px | 需确认 |
| | 项目树行高 | 28px（文件夹）/ 26px（文件） | 需确认 |
| **StatusBar** | 高度 | 40px | 需确认当前高度 |
| | 背景 | `bg/surface` | `bg-secondary` |
| | 连接芯片圆点 | 8px，状态色 | 需确认 |

### 3.2 Agent 面板（右侧）

| 组件 | 关键参数 | 规范值 | 当前实现 |
|------|---------|-------|---------|
| **AgentHeader** | 高度 | 48px | 需确认 |
| **ThreadSelector** | 行高 | 32px（下拉列表） | 需确认 |
| | 左侧圆点 | 8px，活跃=蓝 `#0066FF` | 需确认 |
| **UserBubble** | 圆角 | `12px 4px 12px 12px` | 需确认 |
| | 最大宽度 | 85% | 需确认 |
| | 背景 | `rgba(88,86,214,0.12)`（品牌紫） | 需确认 |
| **AgentBlock** | 圆角 | `12px 12px 12px 4px` | 需确认 |
| | 左边框（流式中） | 2px `accent/primary` | 需确认 |
| **ToolCallCard** | 圆角 | 8px | 需确认 |
| | 标题行高度 | 36px | 需确认 |
| | 内容区最大高度 | 300px | 需确认 |
| **Composer** | 圆角 | 12px | 需确认 |
| | 最小高度 | 48px | 需确认 |
| | 最大高度 | 200px | 需确认 |
| **AgentFooter** | 高度 | 32px | 需确认 |

### 3.3 设置页

| 组件 | 关键参数 | 规范值 | 当前实现 |
|------|---------|-------|---------|
| **SettingsLayout** | 覆盖方式 | 全屏，z-index 90 | 需确认 |
| **SettingsNav** | 宽度 | 200px | 需确认 |
| | 导航项高度 | 32px | 需确认 |
| **SettingsContent** | padding | `32px 40px` | 需确认 |
| | 最大内容宽度 | 720px | 需确认 |
| **SettingsCard** | padding | `20px 24px` | 需确认 |
| | 圆角 | 10px | 当前 `--radius-lg: 10px` |

### 3.4 引导页与浮层

| 组件 | 关键参数 | 规范值 | 当前实现 |
|------|---------|-------|---------|
| **BootSplash** | Logo 大小 | 64x64px | 需确认 |
| | 进度条宽度 | 160px, 3px 高 | 需确认 |
| **ApprovalDialog** | 宽度 | 480px | 需确认 |
| | 按钮 | 三按钮：Deny / Allow Once / Always Allow | ✅ 已实现三按钮 |
| | 超时 | 5 分钟自动 Deny | ✅ 已实现 |
| **CommandPalette** | 宽度 | 560px | 需确认 |
| | 搜索框高度 | 52px | 需确认 |
| **McpElicitationModal** | 宽度 | 420px | 需确认 |
| **OAuthWaitingSheet** | 宽度 | 400px | 需确认 |

---

## 4. Codex 对标走查表

以下代码走查表用于验收桌面端与 Codex 官方版的对标程度。每个组件需确认：

- ✅ = 完全对标（像素级一致）
- ⚠️ = 基本对标（功能一致，视觉略有差异）
- ❌ = 未实现或不对标

### C 系列：Codex 对标组件

| ID | 组件 | 对标要求 | 状态 | 备注 |
|----|------|---------|------|------|
| C01 | ThreadListDrawer | 行高 32-36px，时间相对格式，选中蓝竖线 3px `#0066FF` | ⚠️ | 需确认布局位置（Mady 在 AgentPanel 内） |
| C02 | UserBubble | 不对称圆角 12-4-12-12，max-width 85%，右对齐 | ⚠️ | 背景需使用品牌紫 |
| C03 | AgentBlock | 左边框 2px，流式 accent 脉动，光标 step 闪烁 | ⚠️ | 颜色需改为品牌紫 |
| C04 | ToolCallCard | 标题行+状态 spinner/done/fail+展开 chevron | ⚠️ | 需确认展开动画 |
| C05 | ApprovalDialog | 三按钮 + 5min 超时 + 倒计时 60s | ✅ | 已实现 |
| C06 | McpServersSettings | four status colors | ❌ | 需要 MCP 设置页 |
| C07 | SettingsNav | 200px 固定，图标+13px 文字，选中背景高亮 | ⚠️ | 需确认导航项（Mady 比 Codex 多专利相关项） |
| C08 | ModelSettings | Pill 形状选择器，数据动态加载 | ✅ | ModelSettings 组件已创建，使用 mock 数据 |
| C09 | UsageStrip | 位置在 AgentHeader 右侧 | ❌ | 需确认实现 |
| C10 | Composer | slash palette、@ 提及、auto-resize | ⚠️ | ✅ 已有 slash 菜单，需确认 @ 提及 |
| C11 | ReasoningBlock | 默认折叠，灰色 italic | ⚠️ | 需确认实现 |
| C12 | AgentFooter | 连接状态+模型 Badge | ⚠️ | 需确认布局 |

### P 系列：Mady 差异化组件

| ID | 组件 | 说明 | 状态 | 优先度 |
|----|------|------|------|--------|
| P01 | StageIndicator | 专利工作流四阶段指示器 | ❌ | P2 |
| P02 | TodoDock | 底部待办列表 | ❌ | P2 |
| P03 | DocumentViewer | 多格式预览 | ✅ | 已实现 |
| P04 | KnowledgeView | 知识库状态 | ⚠️ | 已实现（索引模拟） |
| P05 | TemplatesView | 文档模板 | ✅ | 已实现 |
| P06 | SkillsView | 技能管理器 | ✅ | 已实现 |
| P07 | McpView | MCP 服务器状态 | ✅ | 已实现（只读） |
| P08 | ThemePack | 4 套主题包 | ✅ | 已实现 |

---

## 5. 开发流程指引

### 5.1 新组件开发步骤

1. **查阅规范**：找到组件对应的章节（第 7-12 章），获取视觉参数
2. **查阅走查表**：确认是对标 C 系列还是差异化 P 系列
3. **引用 Token**：使用 §1 Token 映射表中的 CSS 变量名，**禁止硬编码裸色值**
4. **组件实现**：参考 `desktop/frontend/src/components/` 中已有组件的模式
5. **动画实现**：使用 `framer-motion`，引用规范中的动画参数
6. **主题适配**：确保 Light/Dark 双模式均正确渲染

### 5.2 视觉走查步骤

1. **Token 一致性检查**：确保组件引用的所有 CSS 变量在 `globals.css` 中正确定义
2. **像素级检查**：对照规范中的圆角、间距、字号等参数（精确到 px）
3. **主题检查**：切换 Light/Dark 模式，确认色值正确
4. **动画检查**：检查 hover/出现/消失动画时长和缓动函数
5. **无障碍检查**：聚焦环可见、对比度达标、键盘可操作

### 5.3 颜色恒等性原则

以下场景使用 `--color-mady-accent` #5856d6 而非蓝色：
- ✅ 链接文字（取代 iOS 系统蓝 `#007aff`）
- ✅ 成功状态（取代绿色 `#34c759`）
- ✅ 选中态背景
- ✅ 主按钮背景
- ❌ **线程选中指示器**（规范明确保留 Codex 蓝 `#0066FF`）

> 例外：线程列表（ThreadListDrawer）选中态左侧竖线使用 `#0066FF`（Codex 原生蓝），其余所有强调行为使用品牌紫。

---

## 6. 设计决策记录

### 6.1 平台差异

| 平台 | 标题栏 | 字体 fallback | 标题栏高度 | 滚动条 | 快捷键 |
|------|--------|--------------|-----------|--------|--------|
| **macOS**（主平台） | 透明 + TrafficLights | SF Pro | 38-40px | 8px 覆盖式 | ⌘ |
| **Windows**（P2） | 系统标题栏 + Caption | Segoe UI Variable | 32px | 10px 传统式 | Ctrl |

> Windows 适配为 P2 优先级的远期任务，当前聚焦 macOS。

### 6.2 命名对照表（规范 → 项目代码）

| 设计规范中的名称 | 项目代码中的名称 |
|----------------|----------------|
| `LeftSidebar` | `Sidebar` |
| `AgentPanel` | `ChatView` 的右侧部分 |
| `MessageTimeline` | 消息流（`@tanstack/react-virtual`） |
| `WorkspaceToolbar` | 中心工作区顶部（未实现） |
| `DocumentViewer` | `DocumentViewer.tsx` |
| `SettingsLayout` | `SettingsPanel.tsx` |

### 6.3 未在规范中列出但已在项目中实现的功能

这些组件不在 `codex-desktop-pixel-perfect-design-spec.md` 范围内，但已在桌面端实现：
- `ProjectTree`（项目文件树，Mady 专利差异化功能）
- `KnowledgeView` / `TemplatesView` / `SkillsView` / `McpView`（覆盖层视图）
- `A2UI Renderer`（声明式 UI 组件渲染器）

---

## 附录：已适配 Token 与待新增 Token

### 已存在（可直接使用）

```
--color-mady-bg-primary
--color-mady-bg-secondary
--color-mady-bg-tertiary
--color-mady-bg-grouped
--color-mady-text-primary
--color-mady-text-secondary
--color-mady-text-tertiary
--color-mady-accent / --color-mady-accent-hover / --color-mady-accent-soft / --color-mady-accent-glow
--color-mady-danger / --color-mady-success / --color-mady-warning / --color-mady-info
--color-mady-separator / --color-mady-border
--shadow-mady-popover / --shadow-mady-tooltip
--radius-sm / --radius-md / --radius-lg
--font-sans / --font-mono
--mady-sidebar-width / --mady-context-width / --mady-titlebar-height
```

### 待新增（P0 优先）

```
【P0】--color-mady-bg-sidebar: rgba(245,242,238,0.72) L / rgba(26,24,22,0.85) D
【P0】--color-mady-bg-sidebar-solid: #F0EDE8 L / #1A1816 D
【P0】--bg-hover: rgba(0,0,0,0.04) L / rgba(255,255,255,0.06) D
【P0】--bg-active: rgba(88,86,214,0.08) L / rgba(88,86,214,0.12) D
【P0】--bg-overlay: rgba(0,0,0,0.48) L / rgba(0,0,0,0.64) D
【P0】--bg-composer: #FFFFFF L / #252320 D
【P0】--color-mady-text-inverse: #FFFFFF L / #1A1814 D
【P0】--color-mady-text-link: #5e5ce6 L / #6e6cf0 D（修正当前 iOS 蓝色）
【P0】--duration-normal: 150ms
【P0】--duration-slow: 250ms
【P0】--duration-stream: 1000ms
【P0】--ease-spring: cubic-bezier(0.4, 0, 0.2, 1)
```

### 待新增（P1 优先）

```
【P1】--radius-xl: 12px
【P1】--radius-2xl: 16px
【P1】--radius-full: 50%
【P1】--shadow-card / --shadow-floating / --shadow-modal / --shadow-drag
【P1】--layout-statusbar-height: 40px
【P1】--layout-agent-width: 380px（修正当前 320px）
【P1】--layout-settings-nav-width: 200px
【P1】--font-size-mady-h2: 16px
```

### 待新增（P2 优先）

```
【P2】--mcp-starting / --mcp-ready / --mcp-failed / --mcp-cancelled
【P2】--connection-connected / --connection-connecting / --connection-disconnected
【P2】--selection-bg / --focus-ring
```
