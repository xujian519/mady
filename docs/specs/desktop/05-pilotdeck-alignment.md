# 05 — PilotDeck 对齐规格：Agent + Files + Skills + MCP

- **功能名**：desktop-pilotdeck-alignment
- **对齐日期**：2026-07-27（Owner 四项决策已确认）
- **状态**：✅ T5.1–T5.8 已实现并验证（2026-07-28），详见 AI_CHANGELOG
- **参照产品**：PilotDeck Desktop 0.1.260623（本机 `/Applications/PilotDeck.app`，Electron + 内嵌 Node 服务）
- **前置文档**：[01-proposal.md](./01-proposal.md) / [02-spec.md](./02-spec.md) / [03-design.md](./03-design.md) / [04-tasks.md](./04-tasks.md)

---

## 1. 背景与目标

Owner 要求以本机安装的 PilotDeck 桌面端为参照，复刻其核心体验。经实机调研（截图存档于 `.analysis/pilotdeck-*.png`），PilotDeck 的关键交互为：

- 左侧栏会话列表 + 顶部 tab 栏（Agent / Files / Skills / Routing / Memory / Always-On）
- **Files tab**：右侧滑出文件浏览器面板，工具栏含新建文件/文件夹、上传、下载、删除、刷新
- **文件查看器浮层**：CodeMirror 编辑器（行号、Ctrl+S 保存、Esc 关闭、行列计数）
- **Skills tab**：技能列表 + SKILL.md 查看/编辑
- 聊天区：空状态大标题 + Composer（Agent 选择器、附件、权限模式）、工具调用折叠摘要、富 Markdown（表格/KaTeX）

**PilotDeck 没有的能力**（本期需自行定义）：PDF 标注查看、图片预览（其 pdf 仅作附件 MIME 处理）。

### 1.1 Owner 已确认的四项决策（2026-07-27）

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | 复刻范围 | 聚焦 **Agent + Files + Skills + MCP** 四个面（Routing/Memory/Always-On 不做） |
| 2 | 布局 | **保留 Mady 现有三栏骨架**，把 PilotDeck 的文件面板工具栏与编辑器浮层移植进来，不推倒重来 |
| 3 | 标注 PDF | 引入 **pdf.js** 内嵌只读查看器，渲染页面 + 批注层（highlight/comment）；**编辑批注本期不做**。**此项取代 [review-2026-07-27.md](./review-2026-07-27.md) §9.4 V1「PDF 一律外部打开」的旧决策** |
| 4 | md 编辑器 | 引入 **CodeMirror 6**（与 PilotDeck 一致），编辑/预览双模式切换，预览复用现有 `MarkdownRenderer` |

### 1.2 三条必做功能（Owner 红线）

1. **查看/修改 Markdown 文件** — 文件树点击 → 编辑器浮层 → 编辑 → Ctrl+S 保存
2. **查看标注 PDF、图片** — 内嵌 PDF 查看器（含批注渲染）+ 图片预览
3. **项目文件浏览器** — 完整闭环：浏览 / 新建 / 重命名 / 查看 / 编辑 / 保存

---

## 2. 差距分析（现状 → 目标）

| 能力 | Mady 现状 | 目标（对齐 PilotDeck） |
|------|-----------|------------------------|
| 文件树 | `ProjectTree`（560 行）+ `ListDirectory`/`CreateFolder`/`RenameFolder` 后端 | ✅ 已有；补**点击文件→打开查看器**闭环 |
| 文件查看 | `DocumentViewer` 仅 `<pre>` 文本 + PDF 外链占位 | 编辑器浮层：文本/md → CodeMirror；图片 → 预览；PDF → pdf.js |
| md 编辑 | ❌ 无 | CodeMirror 6 + 编辑/预览切换 + Ctrl+S 保存（新增 `WriteFile` 后端） |
| 图片预览 | ❌ 无 | 浮层内 `<img>` 渲染（后端返回 base64 data URL） |
| PDF 标注 | ❌ 无 | pdf.js 渲染页画布 + AnnotationLayer（只读） |
| Skills 管理 | ❌ 无（仅 `skills/` 目录约定） | Skills 视图：列表 + SKILL.md 查看/编辑（复用文件 API） |
| MCP 管理 | ❌ 无 UI（`mcp/config_trust.go` 为安全敏感路径） | MCP 视图：**只读**展示已配置服务器；不触碰信任存储写路径 |

---

## 3. 技术设计

### 3.1 新增后端 Binding（`desktop/app.go` + `desktop/types.go`）

| 方法 | 签名 | 说明 |
|------|------|------|
| `ReadFile` | `(relPath string) (*FileContent, error)` | 沙箱校验后读文件；按扩展名归类 `kind: text/md/image/pdf`；图片/PDF 返回 base64，文本返回 UTF-8 字符串；单文件上限 20MB |
| `WriteFile` | `(relPath, content string) error` | 沙箱校验后写文本文件（仅 `text/md` 类可写）；原子写（tmp + rename） |
| `DeleteEntry` | `(relPath string) error` | 删除文件或空目录（对齐 PilotDeck 工具栏；递归删除本期不做） |
| `ListSkills` | `() ([]SkillEntry, error)` | 扫描 `MADY_HOME/skills` 与内置 `skills/`，返回 name + description + SKILL.md 相对路径 |
| `ListMcpServers` | `() ([]McpServerEntry, error)` | 只读解析现有 MCP 配置（项目 `.mcp.json` / 全局配置），不触碰 `mcp/config_trust.go` 写路径 |

**沙箱约束**：`ReadFile`/`WriteFile`/`DeleteEntry` 复用现有 `isPathWithinSandbox`，边界 = `resolveProjectDir()`。Skills/MCP 视图的文件读写边界 = `MADY_HOME`，与项目沙箱分离校验。

### 3.2 前端新增（`desktop/frontend/`）

**新增依赖**（`package.json`）：
- `codemirror` + `@codemirror/state` + `@codemirror/view` + `@codemirror/language` + `@codemirror/lang-markdown` + `@codemirror/theme-one-dark`（CodeMirror 6 按需包）
- `pdfjs-dist`（锁定 4.x，`pdf.worker.min.mjs` 走 Vite `?url` 打包）

**新增组件**：

```
src/components/fileviewer/
├── FileViewerOverlay.tsx   # 浮层容器：标题栏（文件名/路径/保存/关闭）+ Esc 关闭 + 状态栏（行列计数）
├── CodeEditor.tsx          # CodeMirror 6 封装：md 高亮、Ctrl+S/Cmd+S 保存、脏标记
├── MarkdownPreview.tsx     # 编辑/预览切换中的预览面（复用 MarkdownRenderer）
├── ImagePreview.tsx        # 图片渲染 + 缩放（滚轮/双击）
└── PdfViewer.tsx           # pdf.js 页渲染 + AnnotationLayer 批注层（只读）+ 分页/缩放
```

**新增视图**：
- `SkillsView.tsx` — 技能列表（左）+ SKILL.md 查看/编辑（右，复用 CodeEditor）
- `McpView.tsx` — MCP 服务器只读列表（名称/命令/来源配置文件/状态）

**接线改动**：
- `ProjectTree.tsx`：文件节点点击 → 打开 `FileViewerOverlay`；工具栏补「新建文件」「删除」按钮
- `ChatView.tsx` / `Sidebar.tsx`：顶部栏新增 Files / Skills / MCP 入口（图标 + 浮层）
- `stores/files.ts`（新增 Zustand store）：当前打开文件、脏状态、保存动作

### 3.3 pdf.js 批注渲染方案

- `pdfjsLib.getDocument({ data })` → `page.render({ canvasContext })` 渲染页画布
- `page.getAnnotations()` → `pdfjsLib.AnnotationLayer.render(...)` 渲染批注层（highlight/underline/squiggly/strikeout/ink/freeText/stamp）
- 批注点击 → 侧栏展示批注内容（popup 文本、作者、日期）
- **只读**：不提供批注编辑 UI；`pdfjs-dist` 的 editor 模式不启用
- 大文件策略：逐页懒渲染，仅可视页进入 canvas

### 3.4 安全红线

- 不修改 `tools/path.go`、`mcp/config_trust.go`、`agentcore/permission/` 等安全敏感路径；桌面端只消费
- `WriteFile`/`DeleteEntry` 必须过 `isPathWithinSandbox`；测试覆盖越狱用例
- PDF 渲染禁用 JS 执行（pdf.js 默认 `isEvalSupported: false`）
- 图片/PDF 通过 Wails Binding 传 base64，**不**向前端暴露 `file://` 任意读

---

## 4. 任务拆解（接续 04-tasks.md，编号 T5.x）

| 任务 | 内容 | 文件数 | 审查 |
|------|------|--------|------|
| T5.1 | 后端 `ReadFile` + `FileContent` 类型 + 沙箱单测 | 3 | L2 |
| T5.2 | `FileViewerOverlay` + 文本/md 只读查看 + ProjectTree 点击闭环 | 4 | L2 |
| T5.3 | CodeMirror 集成 + 编辑/预览切换 + `WriteFile` 保存（含越狱单测） | 5 | L2 |
| T5.4 | 图片预览（base64 data URL + 缩放） | 2 | L1 |
| T5.5 | pdf.js 内嵌查看器 + 批注层（只读） | 3 | L2 |
| T5.6 | Skills 视图（`ListSkills` + SKILL.md 编辑） | 4 | L2 |
| T5.7 | MCP 只读视图（`ListMcpServers`） | 3 | L2 |
| T5.8 | `DeleteEntry` + 文件树「新建文件/删除」工具栏（含越狱单测） | 4 | L2 |
| T5.9 | 前端 typecheck/build + desktop `go test` 全绿 + AI_CHANGELOG | 2 | L1 |

> 每个任务遵循「单次改动 3-5 文件」，对应一次提交。

---

## 5. 验收标准

| 编号 | 标准 |
|------|------|
| AC-F1 | 文件树点击 `.md` 文件 → 浮层打开 → 编辑 → Cmd+S 保存 → 磁盘内容更新 |
| AC-F2 | 文件树点击 `.png/.jpg` → 浮层内图片预览可缩放 |
| AC-F3 | 文件树点击含批注的 `.pdf` → pdf.js 渲染页面 + 批注可见、批注内容可读 |
| AC-F4 | 文件树可新建文件/文件夹、重命名、删除（沙箱外操作被拒绝并提示） |
| AC-F5 | Skills 视图列出可用技能，可查看/编辑 SKILL.md 并保存 |
| AC-F6 | MCP 视图展示已配置服务器（只读） |
| AC-F7 | 越狱路径（`../..`）读写删全部被后端拒绝（单测） |
| AC-F8 | `make verify`（root + tools + tui + desktop）与前端 `pnpm typecheck && pnpm build` 全绿 |

---

## 6. 非目标（本期不做）

- Routing / Memory / Always-On 三个 PilotDeck tab
- PDF 批注编辑、PDF 文本选择复制增强
- 文件树递归删除、拖拽移动、多选
- MCP 服务器的增删改（只读展示）
- 文件上传/下载按钮（PilotDeck 有，Mady 场景由 Agent 工具链覆盖）
