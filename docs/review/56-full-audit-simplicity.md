# 简约化审查报告

**审查范围**：Mady 项目全部 Go 源文件 + `desktop/frontend/` 前端
**审查日期**：2026-07-31
**审查哲学**：克制、中庸、简约（对照 `AGENTS.md` 项目哲学）
**总体评分**：**B**（整体良好，存在若干可优化的重复和冗余）

---

## 审查方法

1. gocognit 分析函数认知复杂度（>30 标记为高复杂度）
2. dupl 分析重复代码片段
3. 接口定义与实现映射分析（171 个项目自有接口）
4. Config 结构体字段使用分析
5. 调用链冗余层次抽查
6. 前端组件/依赖分析

---

## 一、可立即删除（Clear Wins）

### 1.1 `domains/approval.go` 与 `domains/approval/approval.go` 完整重复 ⭐

**类型**：项目分支/重构遗留物

`domains/approval.go`（旧位置）和 `domains/approval/approval.go`（新包位置）中包含**完全相同的**：

| 符号 | 行数 |
|------|------|
| `ApprovalStore` 接口（3 方法） | ~10 行 |
| `MemoryApprovalStore` 结构体+方法（4 方法） | ~60 行 |
| `DeferredPersistManager` 接口（2 方法） | ~5 行 |
| `DeferredPersistFuncs` 结构体+方法（2 方法） | ~15 行 |
| `WithApprovalStore` / `WithPendingStore` / `WithDeferredPersist` 函数 | ~30 行 |
| `RestorePending` 方法 | ~5 行 |

**建议**：删除 `domains/approval.go` 中的重复定义，将所有引用迁移到 `domains/approval/package`。
`domains/approval.go` 已退化仅为重复的 Gateway。

**预期收益**：消除 ~**120 行完全重复代码**

---

### 1.2 `tools/browser_supervisor.go` 与 `tools/browserproviders/base.go` 接口重复

**类型**：重复定义

两个文件各自定义了完全相同的 `CloudBrowserProvider` 接口（5 个方法）和 `CloudSessionInfo` 结构体：

```go
type CloudBrowserProvider interface {
    ProviderName() string
    IsConfigured() bool
    CreateSession(taskID string) (map[string]string, error)
    CloseSession(sessionID string) error
    EmergencyCleanup(sessionID string)
}
```

- `tools/browser_supervisor.go`（第 565 行）
- `tools/browserproviders/base.go`（第 5 行）

**建议**：删除 `tools/browser_supervisor.go` 中的定义，导入 `browserproviders` 包。

**预期收益**：消除 ~**15 行重复代码**，消除编译时潜在的歧义

---

### 1.3 `mcp/client.go` 与 `mcp/http.go` 完整方法拷贝

**类型**：拷贝粘贴

`Client`（stdio MCP 客户端）和 `HTTPClient` 的以下 3 个方法完全一致：

- `ListTools()` — ~20 行分页循环
- `CallTool()` — ~10 行调用 + 解包
- `AgentTools()` — ~5 行委托

```go
// client.go:271 和 http.go:261 完全一致
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
    var out []Tool
    cursor := ""
    for {
        params := map[string]any{}
        if cursor != "" {
            params["cursor"] = cursor
        }
        var result toolListResult
        if err := c.call(ctx, "tools/list", params, &result); err != nil { ... }
        out = append(out, result.Tools...)
        if result.NextCursor == "" { return out, nil }
        cursor = result.NextCursor
    }
}
```

**建议**：提取公共实现到共享 helper（如 `mcp/internal.go`），或使用嵌入/委托模式。
`toolBridge`（client.go:175）和 `extensionClient`（http.go:153）两个内部接口也可考虑统一。

**预期收益**：消除 ~**40 行拷贝重复代码**

---

### 1.4 `cmd/mady/acp.go` / `cmd/mady/server.go` / `desktop/app.go` 权限策略代码块

**类型**：三重复制

三处入口创建相同的 `permission.NewExtension(permission.Policy{...AlwaysDenyApprover})` 块：

```go
// acp.go:39-50, server.go:107-118, desktop/app.go:280-291
cfg.Extensions = append(cfg.Extensions,
    permission.NewExtension(permission.Policy{
        Mode: permission.DecisionAllow,
        Deny: []permission.Rule{
            {Tool: tools.ToolBash},
            {Tool: tools.ToolProcess},
            {Tool: tools.ToolExecuteCode},
            {Tool: tools.ToolBrowser},
            {Tool: tools.ToolComputerUse},
        },
    }, permission.AlwaysDenyApprover{}),
)
```

后面还跟了 `if fc.KnowledgeExt != nil` 和 `if fc.WikiHook != nil` 的条件块（再次重复）。

**建议**：提取为 `buildDefaultDenyPolicy()` 或 `denyDangerousTools()` 公共函数，放在 `bootstrap` 包中。

**预期收益**：消除 ~**30 行重复代码**

---

## 二、建议重构

### 2.1 `session/session.go` 中 `AppendXxx` 方法的三次重复模板

**类型**：模板重复

`AppendMessage`（第 255 行）、`AppendCompaction`（第 260 行）、`AppendBranchSummary`（第 269 行）遵循完全相同的模式：

```go
func (m *Manager) AppendXxx(ctx context.Context, data XxxData) error {
    data, err := json.Marshal(data)
    if err != nil {
        return fmt.Errorf("marshal xxx: %w", err)
    }
    return m.Append(ctx, Entry{Type: EntryXxx, Data: data})
}
```

三函数仅类型参数和错误信息字符串不同。

**建议**：用泛型 helper 合并：

```go
func appendEntry[T any](m *Manager, ctx context.Context, typ EntryType, data T) error {
    raw, err := json.Marshal(data)
    if err != nil { return fmt.Errorf("marshal %T: %w", data, err) }
    return m.Append(ctx, Entry{Type: typ, Data: raw})
}
```

**预期收益**：消除 ~**15 行重复代码**，同时提升新增 EntryType 的便利性

---

### 2.2 `a2a/ws.go` 中四次 switch-case 能力检查模板

**类型**：重复模式

`ws.go` 第 224-257 行四个 switch case 处理 `tasks/send`、`tasks/resubscribe`、`tasks/pushNotification/set`、`tasks/pushNotification/get`，每个执行相同的 cap check → error response → close → continue 模式。

两个 Streaming 检查和两个 PushNotifications 检查分别配对。

**建议**：提取为 `checkCapability(wc, cond *bool, code, msg string) bool` 辅助函数。

**预期收益**：消除 ~**20 行重复错误处理代码**

---

### 2.3 `cmd/mady/slash_commands.go` 中 39 次 SlashCommand 注册

**类型**：声明式 VS 构建器

虽然完全消除 SlashCommand 注册的重复需要权衡（显式声明有时更清晰），但 39 次 `r.Register(SlashCommand{...})` 结构体字面量存在大量重复字段（`Category`、`Risk`、`Desc` 等）。

**建议**：考虑引入 `cmd.Register(name, desc, handler)` 便捷包装，或按 Category 分组声明。**但不强制**——显式注册的清晰度可能超过抽象收益。

**预期收益**：减少 ~**200 行**（但保留与否取决于风格偏好）

---

### 2.4 `domains/*/framework.go` 四个领域的模式重复

**类型**：架构级重复

`domains/enablement/framework.go`、`domains/inventiveness/framework.go`、`domains/novelty/framework.go`、`domains/infringement/framework.go` 四个框架文件中的 `GetArticleFramework()` 方法结构一致：

```go
func (f *Framework) GetArticleFramework() string {
    if f.provider != nil {
        if af := f.provider.Article("<法条键>"); af.Name != "" { return formatArticleData(af) }
        if af := f.provider.Article("<别名>"); af.Name != "" { return formatArticleData(af) }
    }
    return defaultXxxFramework()
}
```

差异仅为法条键和 `defaultXxxFramework()` 中的静态字符串。`ArticleFrameworkProvider` 接口在四个领域各定义一次（文件名相同、位置不同）。

**建议**：
- 将 `ArticleFrameworkProvider` 接口定义统一移到 `domains/domainconfig/` 或 `domains/reasoning/`
- 将 `GetArticleFramework` + `formatArticleData` 提取为公共方法

**预期收益**：消除 ~**80 行重复接口和函数定义**，降低添加新领域的门槛

---

### 2.5 高复杂度函数（gocognit > 50）

| 函数 | 复杂度 | 文件 | 建议 |
|------|--------|------|------|
| `example/cli-chat/main.go:main()` | **157** | `example/cli-chat/main.go:66` | 拆分：配置构建、Agent 创建、交互循环分离 |
| `(*SQLiteStore).vectorSearchSQLParallel` | **91** | `knowledge/sqlite/vector.go:123` | 并行 SQL 查询，复杂度合理，但内部 SQL 构建可提取 |
| `(*Flex).renderVertical` | **89** | `tui/layout/flex.go:143` | 考虑拆分为 `computeFlexLayout` + `renderChildren` |
| `parseBlocks` | **88** | `tui/component/markdown.go:219` | Markdown 解析天然复杂，接受 |
| `(*TUI).processMsg` | **87** | `tui/tui_input.go:22` | 状态机，复杂但合理 |
| `runCompaction` | **84** | `agentcore/compaction.go:293` | 可拆分为 2-3 个阶段函数 |
| `(*PlanCompiler).CompilePlanToGraph` | **80** | `domains/reasoning/plan_compiler.go:132` | 可拆分为 plan segment 构建器 |
| `(*Agent).runInnerLoop` | **79** | `agentcore/agent_run.go:214` | 核心循环，谨慎重构 |
| `(*CompiledGraph).Run` | **78** | `graph/graph.go:257` | 图执行引擎，接受 |
| `handleNavigate` | **70** | `tools/browser_tool_navigate.go:19` | 浏览器导航可拆分 |

**观察**：
- `example/cli-chat/main.go:main()` 是示例代码，复杂度 157 严重过高——示例代码应注重可读性
- 核心引擎（agentcore/graph）的复杂度多来自天然的业务复杂性，合理化
- TUI 层的渲染逻辑有中度重构空间

**建议**：优先重构 `example/cli-chat/main.go:main()`（示例代码不应比生产代码复杂）

---

### 2.6 `domains/rules/evaluate.go` 中 evaluatePresence/evaluateAbsence 相似模板

```go
// evaluatePresence（第 52-63 行）和 evaluateAbsence（第 82-93 行）
for _, p := range patterns {
    re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(p))
    if err != continue
    if re.MatchString(text) { hits++ } else { ... }
}
```

差异仅为 `evaluatePresence` 累加 `hits` 要求 >=80% 通过，`evaluateAbsence` 累加 `violations` 要求 ==0。

**建议**：合并为 `evaluatePrefixed(evaluateType, text)`，接受 `exists bool` 参数

**预期收益**：消除 ~**15 行重复**

---

### 2.7 `domains/rules/engine.go` 中第 296-320 行和第 361-385 行重复

**类型**：runRuleGroup 复制

第 296-320 行（`runRuleGroup` 的一个分支）和第 361-385 行（另一个分支）的 for-range rule → check → error handling 逻辑高度相似。

**建议**：检查是否需要保留两个分支，或可合并为一个通用函数。

---

### 2.8 `domains/sqlite/checkpoint_store.go` 和 `event_store.go` 的 initSchema 模板重复

两个文件中的 `initSchema` 方法重复了 `openDB` → `initSchema` → error return 的完整模式。
这个模式在 SQLite 存储的各个实现中反复出现（共至少 5+ 次）。

**建议**：为 SQLite 存储引入公共基类或嵌入类型。但需注意 Go 的嵌入模式在此场景的限制。

---

### 2.9 `knowledge/extension.go` 中并发搜索模板

`go func() { wg.Add(1); defer wg.Done(); ... mu.Lock(); lists = append(lists, ...); mu.Unlock() }()` 模式在三个搜索通道中重复（第 450-456 行、第 478-484 行、第 490-498 行）。

**建议**：提取 `runSearchLane(ctx, wg, fn func() []Result)` 辅助，仅需提供搜索函数。

---

## 三、值得讨论

### 3.1 工具层 17 个 Operations 接口（1 对 1 映射）

**现状**：每个工具定义自己的 `XxxOperations` 接口（如 `ReadOperations`、`WriteFileOperations` 等），每个接口只有 1 个默认实现（`DefaultReadOperations`、`DefaultWriteFileOperations` 等）。

```go
// tools/read.go 中的模式
type ReadOperations interface {
    Read(ctx context.Context, path string) ([]byte, error)
    Stat(ctx context.Context, path string) (os.FileInfo, error)
}
type DefaultReadOperations struct{}
func (DefaultReadOperations) Read(ctx context.Context, path string) ([]byte, error) { ... }
```

**判断**：**保留** ✅

**理由**：
- 这些接口用于依赖注入——代码中定义 Config.Operations 字段允许测试时注入 mock 实现
- 这是"接受接口，返回结构体"的 Go 谚语的合理应用
- 17 个接口的拆分粒度对应 17 个独立工具，合并不干净

**讨论点**（非必须）：
- 可考虑引入泛型统一的 `ToolOperations[T]`（但需要函数泛型支持，当前 Go 1.26 可考虑）
- 可合并为 `FileOperations` + `ProcessOperations` + `NetworkOperations` 三组，但会丢失工具粒度

**结论**：不过度工程化判断下，现行方案是合理的

---

### 3.2 前端简约化评估

**文件数**：95 个（39 个 TS/TSX + CSS + 配置）
**组件**：18 个 a2ui-renderer 组件 + 16 个应用组件
**依赖**：React 18 + zustand + tanstack/react-query + codemirror + framer-motion + tailwind v4

**评估结果**：**简约，合理**

- 所有 export 的组件（`src/components/index.ts`）均有合理用途
- 依赖最小化——无冗余 UI 库（只用了 React + 状态管理 + CodeMirror + Tailwind）
- Tailwind v4 按需编译，无运行时样式膨胀
- zustand 是极简状态管理（优于 Redux）

**观察**：
- `lodash` 未在依赖中（好——常用冗余来源）
- 无 `axios`（用原生 fetch 替代，好）
- `framer-motion` 在入口被 `MotionConfig reducedMotion="user"` 约束（尊重系统动画设置，好）
- 仅 2 个测试目录，测试覆盖率可进一步提升

**结论**：前端简约化表现良好，无需立即调整

---

### 3.3 `tui/theme/` 中主题结构体的重复

`a11y_themes.go`（HighContrast、ColorBlind 两个主题）和 `semantic_theme.go`（Light、Dark 两个主题）共四个主题遵循相同的 `&SemanticTheme{Name: "...", ...}` 字面量模式。

**判断**：**保留** ✅

**理由**：结构化的主题字面量本身是"数据"而非"代码"，提取外部 YAML/JSON 会增加加载逻辑反而不简约。Go 编译器为字面量做常量折叠优化，零运行时开销。

---

### 3.4 `domains/reasoning/plan_compiler.go:noopNodeBuilder` 空实现

```go
var _ NodeBuilder = (*noopNodeBuilder)(nil)
```

`noopNodeBuilder` 是一个空实现，仅用于占位。2020 年 Go 中常见的接口验证模式。

**判断**：**保留** ✅。这是 Go 生态惯用的编译时接口校验模式，不增加运行时成本。

---

### 3.5 重复的 `PendingStore` 接口（仅在 domains/包内定义两次）

`PendingStore` 在 `domains/pending.go` 和 `domains/approval/pending.go` 中定义——恰好是 approval 重构的又一遗留物。

**建议**：统一为 `domains/approval/pending.go` 中的定义。

---

### 3.6 高复杂度：`domains/evidence/date.go:extractDateFromText`（复杂度 61）

中文日期提取函数（如"2023年3月15日"、"2023-03-15"、"上个月"等）的复杂正则匹配链。

**判断**：**保留** ✅。中文日期格式多样，正则分支复杂但必然。如引入 `go-dateparser` 可减轻，但会引入新依赖。

---

## 四、接口统计总览

| 类别 | 数量 |
|------|------|
| 项目自有接口（排除 vendor） | **171** |
| 其中 1 对 1 实现（含编译期断言） | ~**60** |
| 其中 1 对 1 但合理（工具/mock） | ~**45** |
| 其中 1 对 1 有争议 | ~**15** |

**判断**：171 个接口对于 ~820 个非测试源文件来说，接口密度 ~0.21 。相比典型的 Go 项目（0.15-0.25），处于合理区间。没有明显的"接口癌"问题。

---

## 五、总体评价与建议优先级

### 总体评分：**B**

> **B = 整体良好，存在若干可优化的重复和冗余**

积极面：
- 项目架构分层清晰，依赖方向明确
- 工具层接口设计合理（为 mock 测试服务）
- 前端简约化表现良好
- 没有过度使用泛型或元编程

主要问题：
- 存在包级别重构遗留物（approval 包拆分未清理干净）
- MCP 客户端两个实现的拷贝粘贴
- 入口层代码三重复制
- 少量高复杂度函数可拆分

### 按优先级排序的行动建议

| 优先级 | 项目 | 收益 | 风险 |
|--------|------|------|------|
| P0 | 清理 `domains/approval.go` 重复 | ~120 行，消除编译歧义 | 低（纯删除重复） |
| P0 | 清理 `tools/browser_supervisor.go` 接口重复 | ~15 行编译一致 | 极低 |
| P0 | 合并 `mcp/client.go`/`http.go` 公共方法 | ~40 行 | 低（提取 helper） |
| P0 | 提取入口权限策略公共函数 | ~30 行 | 极低 |
| P1 | 合并 `session/session.go` AppendXxx 模板 | ~15 行 + 可维护性 | 低 |
| P1 | 重构 `a2a/ws.go` 能力检查模板 | ~20 行 | 低 |
| P1 | 统一 `ArticleFrameworkProvider` 接口 | ~80 行 + 架构清晰度 | 中（涉及多个领域包） |
| P1 | 分解 `example/cli-chat/main.go:main()` | 示例示范效果 | 中低 |
| P2 | 合并 `domains/rules/evaluate.go` 评估函数 | ~15 行 | 低 |
| P2 | 提取 `knowledge/extension.go` 并发搜索模板 | ~15 行 | 低 |
| P2 | 简化 `agentcore/compaction.go:runCompaction` | 模块可读性 | 中（需理解 compaction 语义） |

### 确定性中的克制

> **"宁可漏报，不可错杀"**

以下被评估后认为"保持现状"：
- 工具层 17 个 Operations 接口（合理 DI 模式）
- 主题结构体重复（数据驱动，非代码重复）
- TUI 层渲染高复杂度函数（UI 渲染天然复杂）
- `noopNodeBuilder` 空实现（Go 惯用模式）
- `cmd/mady/slash_commands.go` 的 39 次注册（显式声明 > DSL 抽象）
- 前端依赖和组件结构（当前已简约）

### 最大单项收益

清理 `domains/approval.go` 与 `domains/approval/` 的重复 + 三入口权限策略提取：
**~150 行立即删除，零风险，零测试回归**。

---

*审查工具：gocognit + dupl + grep 静态分析*
*审查版本：Mady commit at 2026-07-31*
