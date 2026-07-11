# AI 决策变更日志

## 格式

```
## YYYY-MM-DD: 标题

- **变更**: 做了什么
- **原因**: 为什么做
- **影响范围**: 涉及哪些包/文件
- **风险等级**: 低/中/高
- **审查要求**: L1-L4
```

## 2025-06-11: 初始化代码质量全面审查报告

- **变更**: 完成 Mady 项目首次全面代码质量审查，覆盖 484 个文件的 6 大维度
- **原因**: 系统性识别性能瓶颈、安全漏洞、架构合规性问题，支撑智能体高效调用
- **审查结果**: 审查报告已输出至 `docs/decisions/REVIEW_REPORT_2025-06-11.md`
- **风险等级**: 中（大量安全/性能问题需修复）
- **审查要求**: L2

## 2026-07-11: 修复三个 CRITICAL 并发安全问题

- **变更**:
  1. `domains/agent_pool.go` GetOrCreate 消除 defer+手动 Unlock 混合模式导致的 double-unlock panic，改为显式 Lock/Unlock + 锁外批量 Close
  2. `domains/reasoning/fact_blackboard.go` 为 FactBlackboard 添加 sync.RWMutex 保护所有字段，写方法检查 Locked 并 panic，MarshalJSON/UnmarshalJSON 加锁
  3. `domains/project.go` 提取 StatusActive/StatusArchived/StatusUnreachable 常量替换硬编码字符串
- **原因**: 消除运行时 panic 风险和并发数据竞争
- **影响范围**: domains/agent_pool.go, domains/reasoning/fact_blackboard.go, domains/project.go
- **风险等级**: 中（涉及安全敏感路径 agent_pool 和并发同步）
- **审查要求**: L3

## 2026-07-11: 全面代码质量修复（CRITICAL + MAJOR + Lint + 重复消除）

- **变更**:
  1. **CRITICAL 安全修复**: tools/ delete.go/move.go/patch.go 改用 resolvePathSandboxed 堵住沙箱绕过；tools.go BuildTools 传播 Sandbox 配置；bash.go 添加 Setpgid 进程组隔离 + 临时文件延迟清理 + Write 错误检查
  2. **CRITICAL 并发/泄漏修复**: agentcore/stream.go Map/Merge 添加 out.Done() 监听取消 goroutine 泄漏；session/session.go 锁缓存改 LRU 淘汰替代全量清空；knowledge/store.go ReindexVectors 锁外批量 Embed；server/server.go handleSkillEvents defer unregister；tui/tui.go PanicMsg 处理 + terminal.go readLoop 错误日志 + 写错误记录
  3. **MAJOR agentcore 修复**: 删除死代码(`_ = tc`/tmpState)；compaction 失败时清空 previousSummary；runStreaming 添加 recover；提取 buildRequestMessages 辅助函数；handoff_context 全局 goroutine 简化 + 移除 intentCacheStopCh；handoff.go fmt.Printf → slog；新增 messagesNoClone 内部方法；agent.go map 直接访问改为 Create 调用
  4. **MAJOR tools 修复**: process.go handleKill/handleList 从 stub 改为 Registry 实现；handleStatus/handleWait 从 registry 查真实 entry；browser.go Stealth JS 改用 AddScriptToEvaluateOnNewDocument；find.go WalkDir 深度限制 5 层；grep.go Kill 后立即 Wait
  5. **MAJOR 网络层修复**: a2a PublishTaskUpdate/ReadLoop 事件丢弃添加 slog；SSEKeepAlive 添加 mu 参数；disclosure SSE 添加写锁；mcp/client.go tryReconnect 递归深度限制 3
  6. **MAJOR 基础设施修复**: store/file.go + psychological/store.go 原子写入(tmp+rename)；filequeue RWMutex 替代 Mutex；session persistEntry O(1) hasAssistant 标志；session readInfo 加锁；knowledge/graph 手写 intToStr/floatToStr → 标准库
  7. **MAJOR 其他修复**: guardrails 免责声明完整文本匹配；psychological SDT 权重归一化；disclosure 重试时删除三个提取 key；cmd/mady log.Fatalf → return；example a2a-client/a2a-server signal handling
  8. **Lint 清零**: 18 个 golangci-lint issues 全部修复（dupArg/appendCombine/exitAfterDefer/gofmt/ineffassign/QF1008/QF1012/S1005/SA9003/unconvert/unused）
  9. **代码重复消除**: 4 处 itoa → strconv.Itoa；3 处 lastUserMessage → agentcore.LastUserMessage；2 处 validateKey → util.ValidateKey
- **原因**: 系统性消除审查报告中的 16 CRITICAL / 45+ MAJOR / golangci-lint 问题
- **影响范围**: 全项目（agentcore/tools/domains/session/knowledge/server/tui/a2a/mcp/disclosure/guardrails/psychological/store/filequeue/workflow/cmd/example）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全部通过 | golangci-lint 0 issues
- **风险等级**: 中（涉及安全敏感路径 tools/path 沙箱 + handoff + guardrails）
- **审查要求**: L3
