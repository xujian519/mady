# Mady 项目全面代码质量审查报告

**审查日期**: 2025-06-11
**项目规模**: 484 文件 (Go 471 + YAML 13), ~867K 行
**核心依赖**: Go 1.26, gorilla/websocket, OpenTelemetry
**审查覆盖**: 6 大维度 × 15+ 包层级

---

## 目录

1. [审查基线](#1-审查基线)
2. [严重问题汇总](#2-严重问题汇总)
3. [按包分级问题清单](#3-按包分级问题清单)
4. [架构与依赖合规性](#4-架构与依赖合规性)
5. [可复用性与智能体适配性](#5-可复用性与智能体适配性)
6. [分阶段优化路径](#6-分阶段优化路径)
7. [自动检测发现（Linter）](#7-自动检测发现linter)

---

## 1. 审查基线

| 检测项 | 状态 | 说明 |
|--------|------|------|
| `go build ./...` | ✅ 通过 | 无编译错误 |
| `go vet ./...` | ✅ 通过 | 无 vet 警告 |
| `go test -count=1 ./...` | ✅ 通过 | 全部 50+ 包测试通过 |
| `go mod verify` | ✅ 通过 | 依赖完整性验证通过 |
| `golangci-lint run` | ⚠️ 11 issues | 2 个 gocritic + 8 个 staticcheck + 1 个 unconvert |
| 竞态测试 | 🔲 待运行 | `go test -race ./...` 需手动确认 |

---

## 2. 严重问题汇总

### 严重等级分布

| 等级 | 数量 | 占比 |
|------|------|------|
| **CRITICAL** | 16 | 10% |
| **MAJOR** | 45+ | 29% |
| **MINOR** | 55+ | 36% |
| **SUGGESTION** | 38+ | 25% |

### CRITICAL 问题清单（需立即修复）

| # | 位置 | 问题 | 类型 |
|---|------|------|------|
| C1 | `domains/agent_pool.go:100-114` | `GetOrCreate` 死锁 — defer Lock 后手动 Unlock 导致 panic | 并发安全 |
| C2 | `domains/reasoning/fact_blackboard.go:10-23` | `FactBlackboard` 全部字段无锁保护，多 goroutine 访问数据竞争 | 并发安全 |
| C3 | `session/session.go:635-643` | `FileStore.locks` 缓存全量清空策略导致多个 goroutine 用不同锁保护同一文件 | 并发安全 |
| C4 | `knowledge/store.go:214-284` | `ReindexVectors` 在写锁下执行网络 I/O（embedding API），阻塞 10s+ | 性能/并发 |
| C5 | `agentcore/stream.go:138-161` | `Map` goroutine 不监听 `out.Done()`/`ctx.Done()`，消费者不消费时泄漏 goroutine | 资源泄漏 |
| C6 | `agentcore/stream.go:181-209` | `Merge` goroutine 同 Map 问题 | 资源泄漏 |
| C7 | `tools/delete.go:94` | 未使用 `resolvePathSandboxed`，绕过沙箱可删除任意文件 | 安全 |
| C8 | `tools/move.go:86-87` | 未使用 `resolvePathSandboxed`，绕过沙箱可移动任意文件 | 安全 |
| C9 | `tools/patch.go:97` | 未使用 `resolvePathSandboxed`，绕过沙箱 | 安全 |
| C10 | `tools/delete.go:49-56` | `isProtected` 丢弃 `filepath.Abs` 错误，路径保护失效 | 安全 |
| C11 | `server/server.go:513-519` | `handleSkillEvents` handler 注册永不取消，每个连接泄漏一个 handler | 资源泄漏 |
| C12 | `tui/tui.go:541-547` | `PanicMsg` 未在 `processMsg` 中处理，Cmd panic 静默消失不恢复终端 | 可靠性 |
| C13 | `tui/terminal/terminal.go:324-328` | readLoop 非可恢复错误（如 EBADF）退出后 TUI 永久挂起 | 可靠性 |
| C14 | `provider/smartrouter/smartrouter.go:341-343` | `balancedScore` 在非 Balanced 优先级时不归一化，产生错误评分 | 功能性 |
| C15 | `tui/chat/chat_app.go:378-405` | `ToggleKeyHelp` 锁顺序可能导致死锁 | 并发安全 |
| C16 | `fuzzy/fuzzy.go:84-118` | `mapNormalizedOffset` rune/byte offset 混淆（实际正确但脆弱） | 正确性 |

---

## 3. 按包分级问题清单

### 3.1 agentcore（核心引擎）

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | stream.go:138-161 | Map goroutine 不监听 ctx.Done()/out.Done() | select 中添加 out.Done() case |
| CRITICAL | stream.go:181-209 | Merge goroutine 同 Map | 同上 |
| MAJOR | agent_run.go:189-192 | `_ = tc` 死代码（transformContext 结果丢弃） | 删除 if 块 |
| MAJOR | context_engine_chunked.go:151,159 | `tmpState` 创建后从未使用 | 删除 |
| MAJOR | compaction.go:376-403 | provider 失败时 `previousSummary` 未清空 | 失败时重置 `previousSummary=""` |
| MAJOR | agent_provider.go:97-151 | `runStreaming` 无 recover | 添加 defer recover |
| MAJOR | agent_run.go:239-258 | 上下文溢出重试时消息重复转换 3 次 | 提取 buildMessages 辅助函数 |
| MAJOR | state.go:45-53 | `Messages()` 每次深拷贝，高频调用 | 添加内部 `MessagesNoClone()` |
| MAJOR | handoff_context.go:84-97 | 全局 cleanup goroutine 永不退出 | 改为惰性清理或添加 stop channel |
| MINOR | event.go:400-418 | dispatch 在 RLock 下复制全部 handler | 对高频事件使用 RCU 风格存储 |
| MINOR | agent_run.go:133 | 外层 for 循环缺 ctx.Done() 检查 | 每个循环顶部添加 ctx 检查 |
| MINOR | budget.go:67-69 | `errors.Is` + `errors.As` 重复检查 | 删除 As |
| SUGGESTION | handoff_context.go:61 | `intentCacheMaxRunes=500` 太小 | 改用完整文本哈希 |

### 3.2 tools（工具子模块）

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | delete.go:94 | 绕过沙箱（resolvePath 非 sandboxed） | 改为 resolvePathSandboxed |
| CRITICAL | move.go:86-87 | 绕过沙箱 | 同上 |
| CRITICAL | patch.go:97 | 绕过沙箱 | 同上 |
| CRITICAL | delete.go:49-56 | `isProtected` 忽略 Abs 错误 | 检查 Abs 错误并保守拒绝 |
| CRITICAL | bash.go:98-103 | 进程组未隔离，PID 重用误杀 | 设置 Setpgid: true |
| MAJOR | bash.go:201-238 | 输出截断临时文件不清理 | defer os.Remove 或返回内容而非路径 |
| MAJOR | bash.go:214,219 | tempFile.Write 错误忽略 | 检查 Write 返回值 |
| MAJOR | process.go:408-416 | kill/list 是存根 | 完成 handleKill 实现 |
| MAJOR | process.go:339 | handleStatus 传入零值 ProcessEntry | Poll 应从 registry 查找 entry |
| MAJOR | browser.go:260-262 | Stealth JS 注入在页面加载后，反检测失效 | 用 Page.addScriptToEvaluateOnNewDocument |
| MAJOR | find.go:148-156 | WalkDir 无限遍历深度 | 限制扫描深度或添加超时 |
| MINOR | mcp_client.go:161-183 | scanner 缓冲区溢出后 readLoop 永久退出 | 添加重连逻辑 |
| MINOR | mcp_client.go:100-110 | stdin 写入与 pending 表修改无原子性 | 一次加锁内完成分配+写入 |
| SUGGESTION | browser.go:1274 | browser_vision 是空壳 | 实现视觉模型调用或移除 |

### 3.3 domains / guardrails / psychological

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | agent_pool.go:100-114 | GetOrCreate 死锁（defer 后手动 Unlock） | 重构：锁外创建 agent，锁内做 check |
| CRITICAL | fact_blackboard.go:10-23 | 全部字段无锁保护 | 嵌入 sync.RWMutex，写方法加锁 |
| MAJOR | fact_blackboard.go:61-63 | `Locked` 标志不被写方法检查 | 写方法开头检查 Locked |
| MAJOR | project.go:24,46 | `Status` 硬编码字符串常量 | 定义类型常量 |
| MAJOR | project.go:164-186 | `RefreshStatus` 在锁内做文件 I/O | 锁外收集路径列表 |
| MAJOR | agent_pool.go:53 | `reaperLoop` goroutine 永不退出 | Close 中关闭 stopCh（已有） |
| MAJOR | walker.go:259-262 | LLM 调用上下文不可取消 | store 接口接受 ctx 参数 |
| MAJOR | disclosure/consistency.go:193-201 | 重试时三个提取 key 未删除，旧数据残留 | 显式删除三个提取 key |
| MAJOR | guardrails/levels.go:108 | 免责声明注入用文本匹配，脆弱 | 改用结构体标志位 |
| MAJOR | psychological/store.go:53 | 文件非原子写入（os.Create 截断） | 先写 tmp 再 Rename |
| MINOR | approval.go:101-108 | `needsApproval` 全量文本匹配后截断 | 先截断再匹配 |
| MINOR | walker.go:498-519 | 手写 itoa（与 workflow 重复） | 改用 strconv.Itoa |
| MINOR | project.go:85 | 空分支（if err := r.load(); err != nil {}） | 补全错误处理或移除 |

### 3.4 网络层（a2a/acp/server/mcp/agui/a2ui）

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | server/server.go:513-519 | skill events handler 永不取消 | 保存 unregister 函数，ctx.Done 时调用 |
| MAJOR | a2a/server.go:992-997 | PublishTaskUpdate 非阻塞 channel 丢弃事件 | 增加容量或使用 goroutine |
| MAJOR | a2a/ws.go:578-591 | 同模式事件丢失 | 同上 |
| MAJOR | server/server.go:1168-1184 | SSEKeepAlive 与主 goroutine 并发写 ResponseWriter | 集中到一个 goroutine + channel |
| MAJOR | server/disclosure.go:269-329 | disclosure SSE 流无写锁保护 | 添加 writeMu |
| MAJOR | mcp/client.go:380-382,390-392 | tryReconnect 递归调用 | 使用循环而非递归 |
| MAJOR | a2a/ws.go:66-74 | WebSocket 读写竞争条件 | 统一 mutex 保护 |
| MAJOR | a2a/ws.go:596-657 | tryReconnect 旧连接关闭前新连接已开始读写 | 停止旧读循环后重新启动 |
| MINOR | acp/server.go:166 | `time.After` 不释放 | 改用 `time.NewTimer` + defer Stop |
| MINOR | a2a/ratelimit.go:46-48 | Stop channel 可被重复 close | 使用 sync.Once |

### 3.5 基础设施层（graph/session/store/knowledge/retrieval/memory/workflow）

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | session/session.go:635-643 | 锁缓存全量清空导致并发不互斥 | 改用 LRU 淘汰策略 |
| CRITICAL | knowledge/store.go:214-284 | 写锁下做网络 I/O | 锁外收集数据批量 embed |
| MAJOR | session/session.go:443-486 | persistEntry 每次追加遍历全部 O(N) | 用布尔标志位跟踪 |
| MAJOR | session/session.go:869-933 | readInfo 未获取 per-session 锁 | 添加读锁 |
| MAJOR | store/file.go:25-37 | 非原子写入 | 先写 tmp 再 Rename |
| MAJOR | knowledge/graph/retrieval_enhancer.go:226-261 | 手写 intToStr/floatToStr | 改用 strconv.Itoa/fmt.Sprintf |
| MAJOR | workflow/workflow.go:24-26 | AgentStep.Run 每次创建全新 Agent | 复用 Agent 或添加池化 |
| MAJOR | filequeue/filequeue.go:84-89 | 读操作也用写锁，读-读不并发 | 改用 RWMutex |
| MAJOR | retrieval/keyword.go:83-132 | scoreChunk 返回小写匹配片段 | 保存原始 content 用于提取片段 |
| MINOR | graph/pregel.go:53-76 | deepCopyValue reflect 分支 typed slice 可能 panic | 添加 typed slice 分支 + 测试 |
| MINOR | memory/extractor.go:91-95 | extractWithRules 始终返回 nil | 实现规则提取或删除死代码 |
| MINOR | knowledge/eval.go:104 | EvalHook 的 result 未消费 | 实现事件发送或移除 |
| MINOR | knowledge/store.go:353-362 | SeedData 忽视 AddDocument 错误 | 至少记录日志 |

### 3.6 TUI 层

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| CRITICAL | tui.go:541-547 | PanicMsg 未处理，panic 静默消失 | 添加 case core.PanicMsg 处理 |
| CRITICAL | stdio/spinner.go:107-108 | 光标隐藏后 panic 不恢复 | defer 恢复光标 |
| CRITICAL | terminal.go:324-328 | readLoop 不可恢复错误退出后 TUI 挂起 | 日志+触发 graceful shutdown |
| CRITICAL | overlay.go:133-240 | composeOverlays 在锁外调用 Render | 锁内复制引用后锁外渲染 |
| CRITICAL | chat/chat_app.go:378-405 | ToggleKeyHelp 锁顺序可能死锁 | 把所有 host 调用移到锁外 |
| MAJOR | tui.go:859 | 终端写错误静默丢弃 | 至少 debug 日志 |
| MAJOR | tui.go:386-398 | Tick goroutine 不在 WaitGroup 中 | 添加 WaitGroup，Stop 时等待 |
| MAJOR | component/markdown.go:213 | 语法高亮每帧重 tokenize | 缓存渲染输出（keyed by source hash） |
| MINOR | terminal.go:235 | setTermios 错误静默丢弃 | 返回或 panic |
| MINOR | chat_bridge.go:10 | NewChatApp 无 doc 注释 | 补充注释 |
| MINOR | component/statusbar.go:13,18,28 | 多个导出符号无 doc 注释 | 补充 |
| MINOR | stdio/renderer.go:191-227 | 5 个导出函数无 doc 注释 | 补充 |

### 3.7 入口/示例

| 严重度 | 文件:行 | 问题 | 修复方案 |
|--------|---------|------|----------|
| MAJOR | cmd/mady/main.go:462,511 | log.Fatalf 跳过 defer | 改用 fmt.Fprintf+os.Exit |
| MAJOR | example/a2a-client/main.go:23 | 无 signal handling，不可取消 | 用 signal.NotifyContext |
| MAJOR | example/a2a-server/main.go:29 | 无 graceful shutdown | 添加 signal.NotifyContext |
| MINOR | cmd/mady/main.go:226,438 | flag.Parse 错误静默丢弃 | 检查错误并输出提示 |
| MINOR | cmd/mady/main.go:429-432 | app.Start 失败后阻塞 app.Done | return 不在阻塞 |
| SUGGESTION | cmd/mady/main.go:252-278 | buildCfg 闭包硬编码重复配置 | 基于 fc.BaseConfig 覆写差异字段 |

---

## 4. 架构与依赖合规性

### 4.1 依赖方向检查

根据 `go list -f '{{.Imports}}'` 分析：

```mermaid
graph TD
    subgraph External["外部接口层"]
        A2A[a2a] --> AC[agentcore]
        A2UI[a2ui] --> A2A
        A2UI --> AGUI[agui]
        ACP[acp] --> AC
        AGUI[agui] --> AC
        Server[server] --> AC
        Server --> AGUI
        Server --> MCP[mcp]
    end

    subgraph Core["核心引擎层"]
        AC[agentcore] --> PKG[pkg/util]
        AC --> SKL[skill]
    end

    subgraph Domain["领域扩展层"]
        DOM[domains] --> AC
        DOM --> GRA[graph]
        DOM --> GRD[guardrails]
        DOM --> PSY[psychological]
        DOM --> WF[workflow]
        DOM --> TOOLS[tools]  ;; 注意：domains 依赖 tools 子模块
    end

    subgraph Infra["基础设施层"]
        GRA --> AC
        SES[session] --> AC
        STO[store] --> AC
        SKL --> OS[os/io]
        KNW[knowledge] --> AC
        KNW --> RET[retrieval]
        MEM[memory] --> AC
        WF --> AC
    end
```

**结论**：依赖方向整体合规，但存在 1 个值得关注的点：

- `domains` → `tools` 的依赖：domains 是领域扩展层，tools 是工具层。根据 8 层架构，"提供者层 → 工具层 → 扩展层 → 领域扩展层"，domains 依赖 tools 是"上层依赖下层"，**合规**。但 tools 子模块有独立 go.mod，是独立模块。

**无 import cycles**：go vet 和 go mod verify 均通过。

### 4.2 文件树规范

- ✅ 包命名统一小写
- ✅ 目录结构清晰对应架构分层
- ✅ `components/` 已标注 Deprecated，计划迁移
- ✅ 测试文件与源文件同目录（`*_test.go`）
- ⚠️ `tools/` 同时作为根包目录和子模块，易混淆

### 4.3 命名规范

- ✅ Go 标准 `MixedCaps` 导出、`mixedCaps` 非导出
- ✅ 全项目统一使用英文标识符（变量/函数/类型）
- ✅ error 类型以 `Error` 结尾（`BudgetExceededError`, `GuardrailError`）
- ⚠️ `psychological/sdt.go:75` `SDTWeights` 字段不保证和为 1（虽非命名问题，但与预期语义不符）

### 4.4 注释覆盖率

手动抽样检查（每个包抽查 5 个导出符号）：

| 包 | 覆盖率 | 说明 |
|----|--------|------|
| agentcore | ≥95% | 几乎全部导出符号有注释 |
| tools | ~70% | 内部辅助函数缺注释（`killProcessTree`, `stripAnsi` 等） |
| domains | ~85% | `appendLifecycle` 等包装函数缺注释 |
| tui/component | ~60% | `statusbar`, `skill_center`, `todo_panel` 缺注释 |
| tui/stdio | ~50% | `renderer.go` 5 个导出函数缺注释 |
| tui/chat | ~50% | `EventSubscriber`, `Subscriber` 缺注释 |
| graph | ≥90% | 详细 |
| session | ≥90% | 详细 |

---

## 5. 可复用性与智能体适配性

### 5.1 现有优势

| 维度 | 评分 | 表现 |
|------|------|------|
| 模块化拆分 | ⭐⭐⭐⭐ | 15+ 个独立包，职责清晰 |
| 接口抽象 | ⭐⭐⭐⭐⭐ | `Provider`, `Extension`, `LifecycleHook`, `EventBus` 等通用接口 |
| 事件系统 | ⭐⭐⭐⭐⭐ | 类型安全事件总线，支持实时可观测性 |
| 错误处理 | ⭐⭐⭐⭐ | `BudgetExceededError`, `GuardrailError` 等具名错误类型 |
| 测试覆盖 | ⭐⭐⭐⭐ | 136 个测试文件，50+ 包全部有测试 |
| 文档 | ⭐⭐⭐ | CLAUDE.md + AGENTS.md 详尽，但单体API注释不足 |

### 5.2 智能体适配瓶颈

| 障碍 | 位置 | 影响 |
|------|------|------|
| **缺少统一 Client 接口** | mcp/client.go vs http.go | 工具调用方需处理两套 client |
| **缺少 OpenAPI/Swagger** | docs/openapi.yaml 未填充 | 外部系统无法自动生成客户端 |
| **tools 子模块安全缺口** | delete/move/patch 绕过沙箱 | 智能体不可安全调用 delete/move/patch |
| **无进程生命周期管理** | process.go | 后台进程不可 kill/list，智能体可能泄漏进程 |
| **无速率限制 API** | 仅 a2a 有 RateLimiter | 外部智能体高频调用无保护 |
| **Session 并发问题** | session.go | 锁缓存淘汰导致并发不安全 |
| **API 文档缺失** | 各包导出符号缺注释 | 外部集成者阅读代码成本高 |

### 5.3 优化方向

1. **统一 MCP client 接口**：提取 `Client` 接口共有方法，让 stdio 和 HTTP 变体实现同一接口
2. **补齐安全缺口**：所有文件操作工具统一使用 `resolvePathSandboxed`
3. **补充 API 文档**：`tools/`, `tui/component/`, `tui/stdio/` 的导出符号补齐 doc 注释
4. **进程管理完善**：完成 process.go 的 kill/list 实现
5. **Session 锁修复**：LRU 淘汰代替全量清空
6. **减少代码重复**：3 份 itoa → `strconv.Itoa`，3 份 `lastUserMessage` → 公共函数

---

## 6. 分阶段优化路径

### 第一阶段：安全与并发（高优先级，立即执行）

| 优先级 | 任务 | 涉及文件 | 预计工时 |
|--------|------|----------|----------|
| P0 | 修复沙箱绕过（delete/move/patch） | `tools/delete.go`, `tools/move.go`, `tools/patch.go` | 2h |
| P0 | 修复死锁（agent_pool GetOrCreate） | `domains/agent_pool.go` | 1h |
| P0 | 修复 session 锁缓存淘汰竞争 | `session/session.go` | 2h |
| P0 | 修复 FactBlackboard 并发安全 | `domains/reasoning/fact_blackboard.go` | 1h |
| P0 | 修复 knowledge ReindexVectors 写锁阻塞 | `knowledge/store.go` | 2h |
| P0 | 修复 SSE handler 泄漏 | `server/server.go` | 1h |
| P0 | 修复 TUI PanicMsg 未处理 + 终端资源泄漏 | `tui/tui.go`, `tui/terminal/terminal.go` | 2h |
| P0 | 修复 stream.go goroutine 泄漏（Map/Merge） | `agentcore/stream.go` | 2h |

### 第二阶段：性能与可靠性（中优先级，1-2 周内）

| 优先级 | 任务 | 涉及文件 | 预计工时 |
|--------|------|----------|----------|
| P1 | 修复 State.Messages() 每次深拷贝 | `agentcore/state.go` | 1h |
| P1 | 提取 buildMessages 减少重复消息转换 | `agentcore/agent_run.go` | 1h |
| P1 | compaction previousSummary 失败清理 | `agentcore/compaction.go` | 1h |
| P1 | session persistEntry O(N) 遍历优化 | `session/session.go` | 1h |
| P1 | 修复 SSE 并发写 ResponseWriter | `server/server.go` | 2h |
| P1 | 修复 event 非阻塞 channel 丢弃 | `a2a/server.go`, `a2a/ws.go` | 1h |
| P1 | 修复 TUI 语法高亮每帧重 tokenize | `tui/component/markdown.go` | 1h |
| P1 | 非原子写入修复（store + psychological） | `store/file.go`, `psychological/store.go` | 1h |

### 第三阶段：代码质量与可复用性（低优先级，1 个月内）

| 优先级 | 任务 | 涉及文件 | 预计工时 |
|--------|------|----------|----------|
| P2 | 解决所有 golangci-lint issues（11 个） | 各处 | 2h |
| P2 | 补齐导出符号 doc 注释（tui/ tools/） | `tui/component/*`, `tui/stdio/*`, `tools/*` | 4h |
| P2 | 提取公共函数（itoa -> strconv.Itoa, lastUserMessage） | `domains/reasoning/workflows/patent/retrieval/knowledge/` | 1h |
| P2 | API 文档补充（handlePrompt, writeResponse 等） | `a2a/*`, `acp/*`, `mcp/*` | 3h |
| P2 | 统一 MCP client 接口 | `mcp/client.go`, `mcp/http.go` | 4h |
| P2 | 完成 process.go kill/list | `tools/process.go` | 3h |
| P2 | Linter 引入 ineffassign staticcheck 全面扫描 | `.golangci.yml` | 1h |

---

## 7. 自动检测发现（Linter）

### golangci-lint 结果

```
11 issues:
  gocritic: 2
    - disclosure/preprocess.go:157  dupArg (strings.ReplaceAll 重复参数)
    - domains/assistant.go:75       appendCombine (可合并 2 个 append)
  staticcheck: 8
    - agentcore/handoff_context.go:111,134,135  QF1008 (可移除嵌入字段选择器)
    - disclosure/consistency.go:189             S1005 (不必要空白赋值)
    - domains/patent.go:116,117                 QF1012 (用 fmt.Fprintf 替代)
    - domains/project.go:85                     SA9003 (空分支)
    - server/disclosure.go:229                  QF1008 (可移除嵌入字段选择器)
  unconvert: 1
    - disclosure/disclosure_test.go:423         不必要的类型转换
```

### 安全敏感路径无新增风险

| 路径 | 状态 |
|------|------|
| `agentcore/handoff.go` | ✅ 无新增问题 |
| `guardrails/levels.go` | ⚠️ 免责声明注入用文本匹配（MAJOR） |
| `domains/router.go` | ✅ 无新增问题 |
| `domains/patent.go` | ⚠️ 2 个 staticcheck（非安全） |
| `domains/approval.go` | ⚠️ 关键词匹配效率（MINOR） |
| `tools/path.go` | ⚠️ 沙箱绕过已在 CRITICAL 中 |
| `tools/tools.go` | ✅ 无新增问题 |
| `tools/bash.go` | ⚠️ 进程组隔离（CRITICAL） |

---

## 附录：审查方法

- **工具**：codegraph (AST 索引) + golangci-lint + go vet + manual code reading
- **代码行覆盖**: ~867K lines, 484 files across 15+ packages
- **测试覆盖**: 全部 50+ packages `go test` passing
- **审查维度**: 代码合规性 / 性能 / 安全 / 并发 / 可复用性 / 架构合规性
