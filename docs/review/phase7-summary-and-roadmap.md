# Phase 7: 汇总与修复路线图

> 日期：2026-07-14
> 审阅范围：全仓库 643 Go 源文件，7 个阶段

## 全量审阅执行摘要

| 阶段 | 范围 | 状态 | 发现问题 |
|------|------|------|---------|
| Phase 1 | 自动化扫描 | ✅ | 11 个已知问题确认，lint/vet/race 全部通过 |
| Phase 2 | P0 安全红线 (10文件) | ✅ | 6 个问题，2 High 已修复 |
| Phase 3 | P1 核心引擎 (~120文件) | ✅ | 1 Low（TODO 债务） |
| Phase 4 | P2 协议层 (~40文件) | ✅ | **34 个问题（8 Critical, 11 High）** |
| Phase 5 | P3 领域扩展 (~100文件) | ✅ | 31 个问题（0 Critical, 4 High） |
| Phase 6 | P4 TUI + 入口 (~100文件) | ✅ | 5 个问题 |
| Phase 7 | 汇总与路线图 | ✅ | — |

---

## 发现总览（合计 77+ 个问题）

### 按严重度统计

| 严重度 | 数量 | 说明 |
|--------|------|------|
| 🔴 **Critical** | **8** | ACP 认证空操作、Agent池use-after-free、WebSocket CheckOrigin通配符等 |
| 🟠 **High** | **18** | 速率限制绕过、锁顺序不一致、CORS通配符、ACL缺失、CircuitBreaker并发等 |
| 🟡 Medium | ~34 | io.ReadAll限制、context传播、测试覆盖率、goroutine泄漏等 |
| 🟢 Low | ~17 | 符号链接、TODO债务、OpenAPI规范、公式权重等 |

### 按模块统计

| 模块 | Critical | High | Medium | Low |
|------|----------|------|--------|-----|
| a2a/ | 2 | 4 | 1 | 2 |
| mcp/ | 2 | 3 | 3 | 0 |
| server/ | 2 | 2 | 1 | 1 |
| acp/ | 2 | 2 | 3 | 0 |
| agui/ | 0 | 0 | 1 | 1 |
| a2ui/ | 0 | 0 | 2 | 0 |
| guardrails/guardian/ | 0 | 1 | 3 | 0 |
| knowledge/ | 0 | 2 | 3 | 1 |
| disclosure/ | 0 | 0 | 1 | 1 |
| graph/ | 0 | 0 | 1 | 1 |
| workflow/ | 0 | 1 | 3 | 0 |
| retrieval/ | 0 | 0 | 1 | 1 |
| psychological/ | 0 | 0 | 1 | 2 |
| memory/compiler/ | 0 | 0 | 1 | 1 |
| agentcore/ | 0 | 1 | 2 | 1 |
| domains/ | 0 | 1 | 1 | 1 |
| tools/ | 0 | 2 | 3 | 0 |
| tui/ | 0 | 1 | 3 | 0 |
| cmd/ | 0 | 1 | 1 | 0 |

---

## 🔴 Critical 问题清单

| # | 模块 | 文件 | 问题 | 状态 |
|---|------|------|------|------|
| C1 | acp | `server.go:399-410` | **handleAuthenticate 空操作** — ACP 认证形同虚设 | ❌ |
| C2 | server | `server.go:813-838` | **Agent 池 use-after-free** — 并发释放导致 agent 被提前 Close | ❌ |
| C3 | acp | `server.go:362-396` | **ClientCapabilities 无条件接受** — 客户端可声明任意文件读写能力 | ❌ |
| C4 | a2a | `ws.go:17-23` | **CheckOrigin 通配符** — 任意来源 WebSocket 连接 | ❌ |
| C5 | a2a | `ratelimit.go:90` | **速率限制反向代理后失效** — RemoteAddr 全为代理 IP | ❌ |
| C6 | mcp | `http.go:408等6处` | **io.ReadAll 无大小限制** — OOM 风险 | ❌ |
| C7 | mcp | `config_discovery.go:58-76` | **$PWD/.mcp.json 命令执行** — StdioConfig.Command 通过 exec 执行 | ❌ |
| C8 | server | `server.go:206-212` | **无 TLS** — 纯 HTTP 监听 | ❌ |

## 🟠 High 问题清单（关键）

| # | 模块 | 问题 |
|---|------|------|
| H1 | a2a | WebSocket 凭据通过 URL 查询参数（日志泄露风险） |
| H2 | a2a | recordTask 持写锁 O(n) 扫描（性能） |
| H3 | a2a | handleResubscribe 浅拷贝 data race |
| H4 | a2a | purgeOldTasks/PublishTaskUpdate 锁顺序不一致（死锁隐患） |
| H5 | mcp | Close/runServerStream streamDone 关闭竞争 |
| H6 | mcp | tryReconnect 未校验协议版本兼容性 |
| H7 | mcp | callWithRetry/Close TOCTOU 竞争 |
| H8 | server | CORS 默认 AllowOrigins=["*"] |
| H9 | server | handleDeleteThread 无 ACL 检查 |
| H10 | acp | 会话 ID 可预测（time.Now().UnixNano()） |
| H11 | acp | handleNewSession 无 CWD 路径沙箱验证 |
| H12 | tools | Shell 命令注入（P0-2 ✅ 已修复） |
| H13 | domains | Manifest AllowedSources 通配符（P0-5 ✅ 已修复） |
| H14 | cmd | main.go 1594行需拆分 |

---

## 修复路线图

### 🔴 第1周：安全体系修复

| 优先级 | ID | 问题 | 修复成本 |
|--------|----|------|---------|
| **1** | C1 | ACP 认证空操作 — **安全体系致命缺陷** | 中 |
| **2** | C2 | Agent 池 use-after-free | 中 |
| **3** | C3 | ACP Capabilities 无条件接受 | 低 |
| **4** | C4 | WebSocket CheckOrigin 通配符 | 低 |
| **5** | C5 | 速率限制反向代理 | 低 |
| **6** | C6 | MCP io.ReadAll 加 LimitReader | 低 |
| **7** | C7 | MCP config 命令执行风险 | 中 |
| **8** | C8 | 无 TLS（文档 + 反向代理方案） | 低 |

### 🟡 第2-3周：协议安全加固

| ID 范围 | 问题 | 数量 |
|---------|------|------|
| H1-H4 | a2a/ 协议安全 | 4 |
| H5-H7 | mcp/ 协议安全 | 3 |
| H8-H9 | server/ 路由安全 | 2 |
| H10-H11 | acp/ 会话安全 | 2 |
| P0-1,P0-4,P0-6 | P0 审计遗留 | 3 |

### 🟢 第4周+：质量提升

| 类别 | 内容 |
|------|------|
| 测试补充 | tui/agentadapter (24.6%), tui/terminal (34.5%), tui/component (38.5%) |
| 大文件拆分 | main.go (1594), computer_use.go (2552), a2a/server.go (1710) |
| 技术债务 | planner.go TODO, sync.Pool 优化, unsafe.Slice 替换 |
| 文档 | OpenAPI 规范完善, govulncheck CI 集成 |

---

## CI 质量门禁状态

| 门禁 | 当前状态 | 目标 |
|------|---------|------|
| golangci-lint | ✅ 0 issues | 保持 |
| go vet | ✅ 0 issues | 保持 |
| go test -race | ✅ 全部通过 | 保持 |
| go mod tidy | ✅ clean | 保持 |
| 覆盖率 | 🟡 ~65% | 目标 70%+ |
| govulncheck | ⚠️ Go 1.25 兼容性 | 等待工具更新 |
| ACP 认证 | 🔴 **空操作** | 实现实际校验 |
| WebSocket 来源检查 | 🔴 **通配符** | 白名单 |
| MCP 配置安全 | 🔴 **命令执行** | 所有权校验 |

---

## 代码质量总评

| 维度 | 评级 | 说明 |
|------|------|------|
| 架构设计 | ⭐⭐⭐⭐⭐ | 8层分层清晰，Extension模式贯穿，依赖方向正确 |
| 并发安全 | ⭐⭐⭐⭐ | go test -race 零问题，但有锁顺序和池竞争隐患 |
| 安全防护 | ⭐⭐⭐ | 沙箱/护栏/权限门控三大支柱扎实，但协议层认证有缺口 |
| 代码质量 | ⭐⭐⭐⭐ | 错误处理一致，命名规范，无技术债务堆积 |
| 测试覆盖 | ⭐⭐⭐ | 核心模块良好，TUI/协议层薄弱 |
| 文档规范 | ⭐⭐⭐⭐ | AGENTS.md/CONTRIBUTING.md/SECURITY.md 完善 |

**总体评级：⭐⭐⭐⭐ (4/5)** — 工程成熟度高的 Go 项目，安全加固和测试补充后可达生产级。
