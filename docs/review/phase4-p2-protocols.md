# Phase 4: P2 协议层审阅报告

> 日期：2026-07-14
> 审阅范围：a2a/ (8+4) + a2ui/ (13+2) + acp/ (5+1) + agui/ (3+2) + mcp/ (8+3) + server/ (4+1)

## 审计总结

| 模块 | Critical | High | Medium | Low | 状态 |
|------|----------|------|--------|-----|------|
| a2a/ | 2 | 4 | 1 | 2 | 🟡 需关注 |
| mcp/ | 2 | 3 | 3 | 0 | 🟡 需关注 |
| server/ | 2 | 2 | 1 | 1 | 🟡 需关注 |
| **acp/** | **2** | **2** | 3 | 0 | 🔴 **严重** |
| agui/ | 0 | 0 | 1 | 1 | 🟢 良好 |
| a2ui/ | 0 | 0 | 2 | 0 | 🟢 良好 |
| **合计** | **8** | **11** | **11** | **4** | **34** |

---

## 🔴 Critical 发现（8 个）

### acp/ — 最严重

| # | 文件 | 问题 | 建议 |
|---|------|------|------|
| C1 | `acp/server.go:399-410` | **`handleAuthenticate` 是空操作！** 接收任意凭据，不做任何校验，总是返回成功。ACP 协议认证形同虚设 | 实现实际认证校验，调用 `s.authProv.Authenticate(params)` |
| C2 | `acp/server.go:362-396` | **`handleInitialize` 无条件接受客户端声明的所有 Capabilities**（包括文件读写 FS.ReadTextFile/WriteTextFile） | 添加 `AllowedFSCapabilities` 白名单配置 |

### a2a/

| # | 文件 | 问题 | 建议 |
|---|------|------|------|
| C3 | `a2a/ws.go:17-23` | WebSocket **`CheckOrigin` 无条件返回 `true`** — 任意来源均可连接 | 实现基于白名单的 CheckOrigin |
| C4 | `a2a/ratelimit.go:90` | 速率限制基于 `r.RemoteAddr` — **反向代理后全部失效** | 支持 `X-Forwarded-For` / `X-Real-IP` |

### mcp/

| # | 文件 | 问题 | 建议 |
|---|------|------|------|
| C5 | `mcp/http.go:408等6处` | `io.ReadAll(resp.Body)` **无大小限制** — 已知问题 #4 | `io.LimitReader` 限制为 1-10MB |
| C6 | `mcp/config_discovery.go:58-76` | `$PWD/.mcp.json` **可执行任意命令** — StdioConfig.Command 通过 `exec.CommandContext` 执行 | 文件所有权校验 + 命令路径白名单 |

### server/

| # | 文件 | 问题 | 建议 |
|---|------|------|------|
| C7 | `server/server.go:813-838` | **Agent 池并发释放竞争** — use-after-free：两个请求用同一 threadID，agent 被提前 Close 但另一个 goroutine 仍在使用 | 加 threadID 级 Mutex 或引用计数 |
| C8 | `server/server.go:206-212` | **无 TLS** — 已知问题 #1 | 实现 TLS 或文档要求反向代理 |

---

## 🟠 High 发现（关键摘要）

- **a2a/ws.go:52** — WebSocket 凭据通过 URL 查询参数传递（易被日志记录）
- **a2a/server.go:946** — `recordTask` 持写锁时 O(n) 扫描全量任务
- **a2a/server.go:832** — `handleResubscribe` 浅拷贝导致并发修改 data race
- **a2a/server.go:238** — `purgeOldTasks` 和 `PublishTaskUpdate` 锁顺序不一致（死锁隐患）
- **mcp/http.go:252** — `Close()` 与 `runServerStream()` 中 `streamDone` 关闭竞争
- **mcp/client.go:552** — `tryReconnect` 未校验新协议版本兼容性
- **mcp/client.go:360** — `callWithRetry` 与 `Close()` 之间存在 TOCTOU 竞争
- **server/server.go:1115** — CORS 默认 `AllowOrigins: ["*"]`
- **server/server.go:614** — `handleDeleteThread` 无所有权检查
- **acp/session.go:90** — 会话 ID 可预测（`time.Now().UnixNano()`）
- **acp/server.go:414** — `handleNewSession` 无 CWD 路径沙箱验证

---

## 修复优先级

### 🔴 立即修复

| 优先级 | ID | 问题 |
|--------|----|------|
| 1 | C1 | ACP 认证空操作 — **安全体系致命缺陷** |
| 2 | C7 | Agent 池 use-after-free |
| 3 | C5 | MCP io.ReadAll 无限制 |
| 4 | C6 | MCP config 命令执行风险 |
| 5 | C2 | ACP Capabilities 无条件接受 |
| 6 | C3 | WebSocket CheckOrigin 通配符 |

### 🟡 2周内修复

| ID | 问题 |
|----|------|
| H1-H11 | 11 个 High 问题 |

### 🟢 正常迭代

| ID | 问题 |
|----|------|
| M1-M11, L1-L4 | 15 个 Medium/Low 问题 |
