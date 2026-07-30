# 附录 A：结构组织审查

> 审查日期：2026-07-31
> 审查范围：全部 Go 源文件 + `desktop/frontend/`（React/TypeScript）
> 对照标准：`docs/GO-DEVELOPMENT-STANDARDS.md` 第 1、2 章
> 总体评分：**B（良好，但有结构性问题需关注）**

---

## P1. 重要问题

### P1.1 `init()` 函数必要性审查

共 12 个非 vendor 非测试源文件包含 `init()` 函数：

| # | 文件 | 用途 | 必要性评估 |
|---|------|------|-----------|
| 1 | `evaluate/benchmark/invalidation_decisions.go:25` | 加载嵌入 JSON 基准数据 | ✅ 必要，数据加载模式，遵循规范（log 不 panic） |
| 2 | `evaluate/benchmark/patent_exam_extended.go:5` | 追加扩展测试用例 | ✅ 可接受，标准注册模式 |
| 3 | `evaluate/metrics.go:291` | 通过 atomic.Pointer 设置默认 CitationVerifier | ✅ 必要，并发安全初始化 |
| 4 | `guardrails/fact_check.go:54` | 动态编译法条正则 | ⚠️ 可改用 `sync.Once`，但 init() 可接受 |
| 5 | `pkg/i18n/catalog.go:200` | 创建全局 Catalog 实例 | ⚠️ **有改进空间**—全局单例反模式，可改用依赖注入 |
| 6 | `provider/adapter/claude.go:14` | `RegisterAdapter(ClaudeAdapter)` | ✅ 标准自注册模式 |
| 7 | `provider/adapter/codex.go:14` | `RegisterAdapter(CodexAdapter)` | ✅ 标准自注册模式 |
| 8 | `tui/terminal/detect.go:302` | 设置 NerdFontStatus=Unknown | ❌ **不必要**—Unknown 是 iota 0，已是零值 |
| 9 | `tui/theme/color_resolve.go:155` | 构建灰度查找表 | ⚠️ 可改用 `sync.Once` 或延迟初始化，但可接受 |
| 10 | `tui/theme/palette.go:235` | 调用 `SyncPaletteGlobals()` 有副作用 | ⚠️ **有改进空间**—init() 调用函数有副作用，全局状态 |
| 11 | `tui/theme/theme_registry.go:32` | 注册 8 个内置主题 | ✅ 标准注册模式 |
| 12 | `workflows/templates.go:35` | 加载默认模板 YAML | ✅ 可接受，标准初始化模式 |

**判断**：12 个 init() 中 9 个合理，1 个完全不必要（`detect.go`），2 个有改进空间。符合规范"不在 init() 中 panic"的要求（所有 init() 均使用 log/stderr 而非 panic）。

### P1.2 测试辅助工具位置

- **`agentcore/testhelpers_test.go`**：✅ 正确位置。包含 stub providers（`constantProvider`、`echoProvider`、`interruptProvider`、`multiTurnToolProvider`）、stub tools（`interruptTool`、`slowTool`、`errTool`）和断言辅助函数（`assertStatus`、`assertInterrupted`）。使用 `t.Helper()` 标记。
- **其他包**：未在非测试生产代码中发现测试辅助函数。生产代码中出现的 `//nolint:unused` 注释仅用于标注被测试引用的导出符号，不属于测试辅助函数。
- **testdata 目录分布**：`domains/claimdrafting/testdata/`、`domains/reasoning/wiring/testdata/`、`domains/enablement/testdata/`、`evaluate/testdata/`、`knowledge/fileindex/testdata/` — 分散在各领域包中，是推荐的模式。
- **结论**：✅ 符合规范要求。

---

## P2. 建议

### P2.1 文件大小审查 — >500 行源文件

**66 个**非测试源文件超过 500 行，其中 **6 个**超过 800 行：

| 行数 | 文件 | 所属包 | 建议 |
|------|------|--------|------|
| 916 | `example/cli-chat/main.go` | example ✅ 示例文件不强制拆分 |
| 881 | `tui/component/markdown.go` | tui/component | ⚠️ Markdown 渲染块缓存逻辑，考虑拆分渲染/缓存逻辑 |
| 879 | `tui/terminal/detect.go` | tui/terminal | ⚠️ 终端检测逻辑密集，含 init() + NerdFont + 多平台检测 |
| 874 | `tui/chat/chat_history.go` | tui/chat | ⚠️ 聊天历史管理，考虑拆分数据层/渲染层 |
| 822 | `domains/workflows/patent/reexamination.go` | domains/workflows/patent | ⚠️ 复审工作流，考虑将节点定义拆出 |
| 812 | `tui/chat/chat_app_layout.go` | tui/chat | ⚠️ 应用布局，考虑拆分子布局 |

**高密度包分布**（>500 行文件的集中度）：
- `domains/workflows/patent/` — 6 个大文件（最密集）
- `tui/component/` — 5 个大文件
- `tui/chat/` — 4 个大文件
- `agentcore/` — 4 个大文件
- `tools/` — 4 个大文件

### P2.2 包内文件数量

**>10 个非测试文件的包**（共 23 个）：

| 文件数 | 包 | 风险评估 |
|--------|-----|---------|
| 69 | `agentcore` | ⚠️ 核心引擎，69 个非测试文件较多但职责明确（事件/钩子/工具/配置/运行/错误/压缩等），可通过子包拆分 |
| 59 | `tools` | ✅ 子模块，工具集合自然膨胀，但考虑按功能类别拆分文件（已隐含 browser/desktop 子包） |
| 44 | `domains` | ⚠️ 领域配置层，44 个文件但跨多个领域（patent/legal/case/mcp/citation/classifier），建议按领域拆分子包，已有部分子包 |
| 41 | `tui/component` | ⚠️ 组件目录，41 个文件 + 29 组件合理，但非测试文件过多（41 源 vs 14 测试），测试覆盖率偏低 |
| 39 | `cmd/mady` | ⚠️ **值得注意** — `cmd/mady` 是入口包，39 个源文件偏多。标准要求 cmd 下只放 `package main`，`main()` 极简短。当前 `cmd/mady` 包含大量框架装配逻辑（tui_session_*.go、settings_store.go、framework.go 等）。建议将装配逻辑迁移至 `bootstrap/` 或 `server/` |
| 28 | `domains/workflows/patent` | ⚠️ 专利工作流，含 6 个 >500 行大文件，建议拆分子节点文件 |
| 24 | `mcp` | ⚠️ 24 个源文件但仅 5 个测试文件，测试覆盖率 0.20 |
| 21 | `domains/reasoning` | ✅ 推理引擎，subdirs 自然分区 |
| 21 | `tui/chat` | ⚠️ 4 个大文件，重渲染密集 |
| 18 | `domains/evidence` | ✅ 证据规则引擎，职责聚合度高 |

### P2.3 测试文件分布

**测试覆盖率薄弱包**（生产代码 >3 个文件但测试比例 <0.20）：

| 测试/总数 | 比例 | 包 | 风险评估 |
|-----------|------|-----|---------|
| 0/2 | 0.00 | `domains/audit` | ❌ 无测试 |
| 0/1 | 0.00 | `domains/citation` | ❌ 无测试 |
| 1/13 | 0.07 | `pkg/ocr` | ❌ 测试严重不足 |
| 1/11 | 0.09 | `tools/desktop` | ❌ 桌面控制工具测试不足 |
| 1/9 | 0.11 | `domains/infringement` | ❌ 侵权分析核心逻辑测试不足 |
| 1/8 | 0.12 | `domains/provisions` | ❌ 条款分析测试不足 |
| 1/6 | 0.16 | `guardrails/guardian` | ❌ 安全guardian（敏感路径）测试不足 |
| 1/6 | 0.16 | `intent` | ❌ 意图分析测试不足 |
| 1/6 | 0.20 | `agentcore/permission` | ⚠️ 权限模块（敏感路径）仅 1 测试 |
| 1/6 | 0.20 | `domains/writing` | ⚠️ 撰写质量评估测试不足 |
| 1/6 | 0.20 | `domains/sqlite` | ⚠️ 领域持久化测试不足 |
| 1/12 | 0.08 | `tools/desktop` | ❌ 桌面工具测试不足（含安全敏感 computer_use_safety） |

**测试覆盖良好的包**（>0.50）：
- `agentcore` — 69/112 (0.61) ✅
- `evaluate` — 18/30 (0.60) ✅
- `tui/component` — 24/41 (0.58) ✅
- `knowledge` — 26/46 (0.56) ✅
- `memory` — 12/23 (0.52) ✅

### P2.4 不必要的全局变量

共约 **402 个**顶级 `var` 声明（非测试文件）。按风险分类：

#### 高风险：可变全局状态（可替代为依赖注入）

| 全局变量 | 文件 | 风险说明 |
|----------|------|---------|
| `globalPromptStore *prompt.PromptStore` | `domains/prompt_store.go:12` | 可变全局单例，通过 `SetupPromptStore()` 注入 |
| `globalDraftingRunner` | `domains/patent.go:39` | 同上模式，10 个类似的 `global*` 变量 |
| `globalTemplateStore` | `domains/patent.go:64` | |
| `globalPatentRetriever` | `domains/patent.go:69` | |
| `globalKnowledgeExt` | `domains/patent.go:75` | |
| `globalInfringementKR` | `domains/patent.go:90` | |
| `globalEvidenceExt` | `domains/patent.go:103` | |
| `globalClaimDraftingExt` | `domains/patent.go:116` | |
| `globalSpecDraftingExt` | `domains/patent.go:122` | |
| `globalRuleExt` | `domains/patent.go:126` | |
| `globalWritingExt` | `domains/patent.go:130` | |
| `globalPatentEvalTool` | `domains/patent.go:202` | |
| `globalPatentRoleSet` | `domains/patent.go:248` | |
| `globalWorkflowStore` | `domains/reasoning/manifest.go:214` | |
| `defaultRegistry` | `workflows/templates.go:33` | 可变单例 |
| `citationWiring` (atomic.Value) | `domains/citation_wiring.go:33` | Atomic 包装减轻但仍是全局状态 |
| `defaultDOCXConverter` | `disclosure/export.go:18` | 可变全局 |
| `atomicPalette` | `tui/theme/palette.go:29` | 主题全局状态（atomic 保护） |
| `nerdFontStatus` | `tui/terminal/detect.go:300` | atomic.Value |
| `currentCitationVerifier` | `evaluate/metrics.go:289` | atomic.Pointer |

**代码中的说明**（`domains/patent.go:35-38`）解释了为何使用全局变量：*"使用全局而非参数传递的原因是 PatentAgentConfig 签名受 domainFactoryMap 约束（func(agentcore.Config) agentcore.Config），无法添加额外参数。"*

这是一个真实的技术约束，但也暴露了架构设计上的问题：`domainFactoryMap` 的回调签名过于固定，导致依赖无法通过参数传递，只能走全局状态。

#### 中风险：常量性质的大 map/slice（应改为 const 或函数内定义）

- `domains/claimdrafting/rules.go:241` — `uncertainWords`
- `domains/claimdrafting/rules.go:258` — `forbiddenWords`
- `domains/claimdrafting/rules_scope.go:28` — `generalizationHints`
- `tools/desktop/computer_use_keys.go:9-111` — 多个键映射表
- `tools/desktop/computer_use_safety.go:16-47` — 安全相关列表

#### 低风险：不可变配置/常量/编译时注入

- `cmd/mady/main.go:53` — `commitHash` / `buildTime`（ldflags 注入，合理）
- `tools/path.go:20` — `ErrOutsideSandbox`（sentinel error，合理）
- `tools/execute_code.go:18` — `ansiEscapeRe`（编译时 regex，合理）

#### ⚠️ 特别关注：`tools/browser_manager.go:11` — `defaultBrowserMgr struct { ... }`

这个全局浏览器管理器实例涉及安全敏感操作（浏览器自动化）。如果此全局状态在测试之间泄露，可能导致竞态条件。建议使用依赖注入。

### P2.5 前端结构审查 (`desktop/frontend/`)

#### 优点
- ✅ **清晰的模块划分**：`a2ui-renderer/`（A2UI 声明式 UI 渲染）、`agui-bridge/`（AGUI 事件桥接）、`components/`（通用 React 组件）、`stores/`（Zustand 状态管理）、`theme/`（主题系统）、`lib/`（工具函数）
- ✅ **组件原子化**：`a2ui-renderer/components/` 18 个原子组件（Button/Text/Card 等），命名规范
- ✅ **测试分离**：`__tests__/` 目录放在组件旁，E2E 在 `e2e/`
- ✅ **单文件职责**：`stores/` 每个 store 一个文件（chat/commands/files/project/settings/threads）

#### 改进建议

| 问题 | 说明 |
|------|------|
| ⚠️ `components/` 目录扁平化 | 29 个组件 + 4 个子目录 `fileviewer/` + `__tests__/` 全部平铺，建议按功能子目录分组（如 chat/、workspace/、settings/） |
| ⚠️ `components/index.ts` | Barrel export 导致模块耦合，建议删除或仅用于公开公共 API |
| ⚠️ `dist/` 提交到仓库 | `dist/`（构建产物）、`node_modules/`（pnpm 依赖）、`test-results/` 均被追踪，应在 `.gitignore` 中排除 |
| ⚠️ `wailsjs/` 自动生成代码被追踪 | Wails 绑定自动生成，不应在版本控制内 |
| ✅ Vite + Vitest + Playwright | 构建/测试工具链标准清晰 |
| ✅ PNPM workspace | 包管理合理 |

---

## 综合评价

| 维度 | 得分 | 说明 |
|------|------|------|
| 分层架构 | A | 8 层架构清晰，严格单向依赖，符合标准 §1.3 |
| 目录命名 | A | 无 common/utils/base 反模式（`pkg/util` 例外，已在标准中列明） |
| 包命名 | A | 全部小写单数，无下划线/驼峰混用 |
| 导入分组 | A | goimports 强制三组排序，pre-commit 已配置 |
| `init()` 使用 | B | 12 个 init()，1 个不必要、2 个有改进空间 |
| 测试辅助工具位置 | A | testhelpers_test.go 正确位置，未发现生产代码中的测试辅助函数 |
| 文件大小 | C | 66 个 >500 行文件，6 个 >800 行，部分需拆分 |
| 包内文件数 | C | `cmd/mady`（39 源文件）偏大，`agentcore`（69）较多 |
| 测试分布 | C | 多个核心包测试覆盖率 <0.20，如 `domains/infringement`、`pkg/ocr`、`guardrails/guardian` |
| 全局变量 | D | 402 个 var 声明，15+ 个高风险可变全局单例，受限于 domainFactoryMap 签名约束 |
| 前端结构 | B | 模块划分清晰，但目录扁平化和构建产物追踪需改进 |

### 总体评分：B

> **B 级含义**：结构基本良好，遵循了多模块布局、分层架构、包命名等核心规范。主要扣分项为文件过大（66 个 >500 行）、全局变量过多（402 个 var + 15+ 可变单例）、以及部分领域包测试覆盖率不足。建议优先重构 `cmd/mady` 的 39 个源文件（将装配逻辑移至 `bootstrap/` 或 `server/`），逐步替换全局变量为依赖注入，并将 >800 行的关键文件拆分。

### 推荐行动项（按优先级）

1. **P1** `cmd/mady` 瘦身：将框架装配逻辑（`tui_session_*.go`, `framework.go`）迁移至 `bootstrap/`，保持入口包 ≤15 个文件
2. **P1** `tui/terminal/detect.go:302` 删除不必要 init()
3. **P1** `domains/patent.go` 重构：考虑扩展 `domainFactoryMap` 回调签名以消除 10+ 个 `global*` 变量
4. **P2** 拆分 >800 行的 6 个大文件（`reexamination.go`、`chat_history.go`、`chat_app_layout.go`、`markdown.go`、`detect.go`、`cli-chat/main.go`）
5. **P2** 为测试覆盖率薄弱的敏感路径包补充测试（`guardrails/guardian`、`agentcore/permission`、`tools/desktop/computer_use_safety.go`）
6. **P2** 前端 `.gitignore` 追加 `dist/`、`node_modules/`、`test-results/`、`wailsjs/`
