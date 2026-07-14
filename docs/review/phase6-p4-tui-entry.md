# Phase 6: P4 TUI 层与入口审阅报告

> 日期：2026-07-14
> 审阅范围：tui/ (61+32) + cmd/mady/ + session/ + memory/ + provider/ + tools/其余 + example/

## 审计总结

| 模块 | 评估 | 关键发现 |
|------|------|---------|
| `tui/tui.go` | 🟢 优秀 | 事件循环健壮，panic 恢复 + 终端恢复，原子渲染 |
| `tui/chat/` | 🟢 良好 | 覆盖率 46.6%，异步消息处理正确 |
| `tui/component/` | 🟡 一般 | 覆盖率 38.5%，editor.go 1340行需要拆分 |
| `tui/agentadapter/` | 🔴 测试薄弱 | 覆盖率 24.6%，需补充测试 |
| `tui/terminal/` | 🔴 测试薄弱 | 覆盖率 34.5%，需补充测试 |
| `cmd/mady/main.go` | 🟡 一般 | 1594行巨大，28处 context.Background() 大部分合理 |
| `session/` | 🟢 良好 | JSONL 树存储设计合理 |
| `memory/` | 🟢 良好 | 编译器学习策略成熟 |
| `provider/` | 🟢 良好 | LLM 接入层安全 |
| `example/` | 🟢 良好 | 示例代码无安全风险 |

## 详细审查

### 1. tui/tui.go — 引擎核心 ✅

**评分**：优秀。1048 行核心引擎设计模式正确：

- ✅ **panic 恢复**（line 541-548）：先恢复终端，再 re-panic 保留堆栈
- ✅ **非阻塞渲染**（line 567）：`atomic.SwapInt64(&t.renderRequested, 0)` 防止重复渲染
- ✅ **BatchMsg 并发**（line 585-589）：每个 Cmd 独立 goroutine，用 BatchMsg 包装时保证顺序
- ✅ **一次性 TUI**（line 232-246）：started 标志 + doneCh 关闭检测，防止重复启动
- ✅ **term.Start 失败回滚**（line 252-258）：完整的失败清理路径
- ✅ **context.Background()**（line 197）：TUI 根 context，生命周期与应用一致 — 合理

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P4-1 | Medium | `tui.go:197` | `context.WithCancel(context.Background())` — 创建方式正确，但可通过 `signal.NotifyContext` 统一管理 | 考虑与 main 的 signal context 关联 |

### 2. cmd/mady/main.go — 入口点 🟡

**评分**：一般。1594 行单个文件，28 处 `context.Background()`。

**context.Background() 合理性分析**：
- 信号处理（line 439）：`signal.NotifyContext(context.Background(), ...)` — ✅ 合理
- MCP 发现（line 368）：`mcp.DiscoverMCPExtensions(context.Background(), ...)` — ⚠️ 应使用 ctx
- 文件索引刷新（line 957）：`fi.Refresh(context.Background())` — ⚠️ 应使用 ctx
- 状态保存（line 1033）：`agent.SaveState(context.Background(), ...)` — ⚠️ 应使用 ctx
- 其余多为初始化阶段调用 — ✅ 合理

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P4-2 | **High** | `main.go:1594` | 1594 行单文件，函数职责混杂 | 拆分为 setup/signal/runTUI/runServer/runAcp 等独立文件 |
| P4-3 | Medium | `main.go` 多处 | 运行时阶段的 `context.Background()` 应改用 ctx 参数（约 5 处） | 传递 signal context |

### 3. tui/agentadapter/ — 测试最薄弱模块 🔴

覆盖率 24.6%。该模块是 TUI 与 Agent 之间的关键适配层。

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P4-4 | Medium | `agentadapter/` | 覆盖率仅 24.6%，Agent 交互逻辑未充分测试 | 补充核心适配流程的单元测试 |

### 4. tui/terminal/ — 终端 I/O 🔴

覆盖率 34.5%。涉及 raw mode 切换、stdin 读取、CSI 解析等底层操作。

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P4-5 | Medium | `terminal/` | 覆盖率 34.5%，raw mode 切换错误处理未充分测试 | 补充终端恢复、stdin 读取错误场景测试 |

### 5. session/ — 会话管理 ✅

- ✅ JSONL 树存储：支持分支（BranchThread）、消息追加
- ✅ ID 生成器：纳秒时间戳 + 计数器，线程安全
- ✅ `MessagesFromEntries`：安全解析，`json.Unmarshal` 错误静默跳过

### 6. memory/ — 记忆系统 ✅

- ✅ 编译器学习策略：decay、promoter、strategy 模型成熟
- ✅ review_queue 线程安全

### 7. provider/ — LLM 接入 ✅

- ✅ API Key 从环境变量读取（无硬编码）
- ✅ chatcompat 兼容 OpenAI Chat Completions API
- ✅ smartrouter 智能模型路由

### 8. example/ — 示例代码 ✅

- ✅ 无 API Key 硬编码
- ✅ 无危险操作默认启用

---

## 整体评估

P4 TUI 层与入口状态：**基本健康**。TUI 引擎设计优秀，但两个子模块（agentadapter 24.6%、terminal 34.5%）测试覆盖严重不足。`cmd/mady/main.go` 是最大的单点复杂度——1594 行需要拆分。
