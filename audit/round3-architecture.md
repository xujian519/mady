# 第 3 轮：架构与设计审阅报告

> **日期**: 2026-07-30 | **范围**: 架构边界、大文件拆分、接口设计、依赖合理性
> **方法**: go-arch-lint + 脚本扫描 + 统计分析 | **耗时**: ~10 分钟

---

## 一、执行摘要

- ✅ **架构边界 100% 合规** — 25/25 分层依赖检查通过
- 🟡 **69 个超大文件** — 7 个 >1000 行，20 个 700-999 行，42 个 500-699 行
- 🟡 **20 个大接口**（>3 方法）— 28 方法的 MemoryStore 是最大接口
- 🟡 **5 个单次使用依赖**— 各有合理用途，不建议移除
- ⚠️ **go-arch-lint 未安装** — 分层检查靠简化脚本，缺少 .go-arch-lint.yml 的完整 43 组件规则

---

## 二、分层架构边界验证

### 2.1 内置脚本结果

```
=== 结果: 25 通过, 0 失败 ===

PASS: agentcore → domains, server, tui
PASS: graph → agentcore, domains, server
PASS: knowledge → server, tui, domains
PASS: memory → server, tui
PASS: retrieval → server, domains, tui
PASS: tui/chat → agentcore
PASS: server → tui, tools
PASS: provider → domains, server, tui
PASS: disclosure → tui, server, domains
PASS: tools → domains, server
```

**结论**：核心分层约束（上层不反向依赖下层）全部通过。项目架构设计在静态依赖层面执行到位。

### 2.2 注意事项

| 项 | 说明 |
|----|------|
| `go-arch-lint` 未安装 | `.go-arch-lint.yml`（524行，43组件）定义了更精细的分层规则，但 `go-arch-lint` 二进制未安装，无法执行完整验证 |
| `scripts/check-arch-boundaries.sh` | 仅覆盖 25 条关键边界，建议补全到与 `.go-arch-lint.yml` 等价 |

---

## 三、超大文件重构评估

### 3.1 统计概览

| 行数范围 | 文件数 | 占比 |
|---------|--------|------|
| >1000 行 | 7 | 10% |
| 700-999 行 | 20 | 29% |
| 500-699 行 | 42 | 61% |
| **合计** | **69** | — |

### 3.2 按目录分布（Top 10）

| 目录 | 文件数 | 主要文件 |
|------|--------|---------|
| `domains/workflows/patent/` | 6 | rule_engine(1203), reexamination(809), invalidation(796), oa_response(746) |
| `tui/component/` | 5 | markdown(881), syntax(665), text(584), debug_overlay(567) |
| `tools/` | 5 | browser_session(768), browser_supervisor(662), vision(508) |
| `agentcore/` | 4 | compaction(644), agent(536), config(458), event_types(531) |
| `tui/terminal/` | 4 | detect(795), terminal(589), keybindings(543) |
| `tui/chat/` | 4 | chat_app(1184), chat_history(587), chat_app_layout(503) |
| `domains/` | 3 | case_index(756), case_extension(720), approval(470) |
| `session/` | 3 | session(562), tree(555), manager(534) |
| `a2a/` | 3 | ws(883), server_jsonrpc(664), client(536) |

### 3.3 拆分优先级

按 (行数 × 职责复杂度 × 修改频率) 排序：

| 优先级 | 文件 | 行数 | 建议拆分方案 | 预估文件数 |
|--------|------|------|-------------|-----------|
| **P0** | `domains/workflows/patent/rule_engine.go` | 1203 | 核心引擎 + 默认规则集 + 关键词匹配 | 3 |
| **P0** | `tui/chat/chat_app.go` | 1184 | 模型管理 + 布局 + 覆盖层 + handlers | 4 |
| **P0** | `cmd/mady/tui_session.go` | 1194 | session CRUD + agent管理 + commands + storage | 4 |
| **P1** | `domains/inventiveness/nodes.go` | 1041 | 节点 + schemas + parsers + prompts | 4 |
| **P1** | `bootstrap/setup.go` | 1195 | provider + agent + retriever 装配分离 | 3 |
| **P1** | `desktop/app.go` | 1268 | 待第 2 轮审阅（未在 T1 内） | — |
| **P2** | `acp/server.go` | 1013 | transport + jsonrpc + handlers | 3 |
| **P2** | `example/cli-chat/main.go` | 919 | 示例文件，低优先级 | — |
| **P2** | `a2a/ws.go` | 883 | readLoop + writeLoop + ping + forwardEvents | 4 |
| **P2** | 其余 60 个 500-899 行文件 | — | 按领域分批次处理 | — |

---

## 四、接口设计审查（§5）

### 4.1 大接口清单（>3 方法）

| 接口 | 方法数 | 文件 | 评估 |
|------|--------|------|------|
| **MemoryStore** | **28** | `memory/types.go:253` | 分层清晰（Remember/Recall/Get/Update/Forget/List/Prune），但可考虑拆为 `MemoryWriter` + `MemoryReader` + `MemoryManager` |
| **PendingStore** | **19** | `domains/pending.go:41` | 待处理案件存储，方法数多但有明确领域语义 |
| ClaimRule | 9 | `domains/claimdrafting/rules.go:14` | 9 个检查方法，可拆为 `ClaimCheck` + `ClaimFormat` |
| NodeBuilder | 9 | `domains/reasoning/plan_compiler.go:40` | Builder 模式自然多方法，可接受 |
| EvidenceJudgmentEngine | 7 | `domains/evidence/types.go:287` | 三性判断方法集，语义聚合合理 |
| KnowledgeRetriever | 7+6 | `domains/enablement/` + `domains/infringement/` | **重复定义** — 两个包各自定义了同名接口，应统一到 `retrieval/domain/` |
| MemoryStore | 28 | `memory/types.go` | 最大接口，拆分为 Reader/Writer 可降低实现复杂度 |

### 4.2 返回接口的函数

仅 1 处：`tui/chat_bridge.go:87` — 返回 `interface{ String() string }`。与规范 §5.3 "接受接口，返回具体类型" 一致（此处返回的是最小接口，可接受）。

### 4.3 接口在消费端定义？

抽查 10 个接口的定义位置，全部在消费端包中定义——符合 §5.2。特别是 `KnowledgeRetriever` 在 `domains/enablement/types.go` 和 `domains/infringement/knowledge.go` 各自定义（但重复定义是个问题）。

### 4.4 仅 1 实现者的接口

| 接口 | 文件 | §5 评估 |
|------|------|---------|
| `BashOperations` | `tools/bash.go` | 仅用于测试 mock？实际无 mock 使用 — **考虑移除** |
| `GitOperations` | `tools/git.go` | 同 BashOperations |
| `GlobOperations` | `tools/glob.go` | 同 BashOperations |
| `WebSearchOperations` | `tools/web_search.go` | 同 BashOperations |
| `FileContentReader` | `domains/case_extension.go:21` | **应直接使用 `os.ReadFile`** |

---

## 五、依赖审查（§9）

### 5.1 直接外部依赖：16 个

| 依赖 | 引用数 | 用途 | 必要性 |
|------|--------|------|--------|
| `chromedp/cdproto` + `chromedp/chromedp` | 19 | 浏览器自动化 | ✅ 核心功能 |
| `gorilla/websocket` | 2 | A2A WebSocket | ✅ 核心协议 |
| `yuin/goldmark` | 4 | Markdown 渲染 | ✅ TUI 核心 |
| `modernc.org/sqlite` | 多处 | 纯 Go SQLite | ✅ 核心存储 |
| `gopkg.in/yaml.v3` | 多处 | YAML 配置 | ✅ 核心配置 |
| `opentelemetry/*` | 4 包 | 分布式追踪 | ✅ 可观测性 |
| `getcharzp/onnxruntime` | 3 | OCR/模型推理 | ⚠️ 评估：是否有替代方案？ |
| `google/uuid` | 1 | Case ID 生成 | ✅ 可接受（可改用标准库 `crypto/rand` 但 UUID v4 专业） |
| `joho/godotenv` | 1 | .env 自动加载 | ✅ 可接受（dotenv import） |
| `signintech/gopdf` | 1 | PDF 生成 | ✅ 可接受（标准库无替代） |
| `xuri/excelize` | 1 | 电子表格读取 | ✅ 可接受（标准库无替代） |
| `fsnotify/fsnotify` | 1 | 文件监控 | ✅ 可接受（标准库无替代） |

### 5.2 间接依赖问题

`alecthomas/chroma` 在 go.mod 中标记为 indirect，但代码中 0 引用。可能是未清理的残留依赖——建议运行 `go mod tidy`。

### 5.3 最小依赖原则遵守情况

✅ **agentcore/ 零外部依赖** — 仅依赖标准库 + 内部包，严格执行最小依赖原则。值得在整个项目中推广。

### 5.4 tools/ 子模块依赖

tools/go.mod 有自己的外部依赖（chromedp 系列、browserproviders 系列），与浏览器自动化功能紧密相关，合理性可接受。

---

## 六、架构发现汇总

| 优先级 | 发现 | 影响 |
|--------|------|------|
| **P1** | 69 个文件 >500 行（§2.4 文件职责违规） | 代码可维护性下降，新人上手困难 |
| **P1** | `KnowledgeRetriever` 接口在 2 个包重复定义 | 类型碎片化，修改时易遗漏 |
| **P1** | 5 个单实现 Operations 接口 | 增加间接层，不带来实际好处 |
| P2 | `go-arch-lint` 未安装，无法验证完整 43 组件分层 | 依赖简化脚本，可能漏检 |
| P2 | `MemoryStore` 28 方法大接口 | 实现负担重，可拆为 Reader/Writer |
| P2 | `alecthomas/chroma` 可能为残留间接依赖 | 二进制体积膨胀 |
| P3 | 5 个单次使用依赖各有合理用途 | 无需移除，但需记录理由 |
| ✅ | 25/25 架构边界通过 | 项目分层设计执行到位 |
| ✅ | 仅 1 处返回接口的函数 | §5.3 规范执行良好 |
| ✅ | agentcore/ 零外部依赖 | 最小依赖原则标杆 |
