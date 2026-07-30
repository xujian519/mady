# 超大文件拆分设计文档

> 日期：2026-07-30 | 来自：审计报告 C9/C10 | 范围：70 个 ≥500 行源码文件

## 一、拆分原则

1. **单一职责优先** — 新文件只做一件事。判断标准：能否用一句话描述职责。
2. **接口先行** — 拆分后通过接口通信，保持 package 不变或拆分子包，避免循环依赖。
3. **最小变更** — 不改变公开 API，不改变行为逻辑，只挪代码位置。
4. **验证闭环** — 每个文件拆分后 `go build` + `go test -race` + `go vet` 通过。
5. **特殊豁免** — main.go 入口、代码生成文件、纯类型/常量文件不强制拆分。

## 二、拆分模式分类

| 模式 | 适用场景 | 拆法 |
|------|----------|------|
| 模式1：按职责分离 | 文件做了多件不相关的事 | 识别独立职责 → 每职责一文件 |
| 模式2：按类型/结构体分离 | 多个大 struct + 各自方法集 | 一类型一文件 |
| 模式3：提取类型/常量/接口 | 类型定义占据大量行数 | 抽到 types.go / constants.go |
| 模式4：按功能/工作流分离 | 同一领域的不同工作流混合 | 每个工作流一文件 |
| 模式5：提取辅助/工具函数 | 大量 helper/utility | 抽到 helpers.go 或按功能分组 |
| 模式6：按协议/层分离 | 不同协议实现或架构层混合 | 按协议/层分离 |

## 三、验证标准与风险控制

### 验证闭环

```bash
go build ./...                        # 编译
go test -race ./<pkg>/...            # 单元测试+竞态
go vet ./<pkg>/...                   # 静态分析
```

### 回归风险分级

| 风险 | 特征 | 额外措施 |
|------|------|----------|
| 🟢 低 | 纯内部重构，公开 API 不变 | 标准闭环 |
| 🟡 中 | 涉及导出符号移动/重命名 | + `go test ./...` 全量 |
| 🔴 高 | agentcore/、guardrails/、acp/、domains/patent.go | + `./integration/...` |

### 敏感路径（需额外谨慎）

- `agentcore/agent.go` — 核心生命周期
- `agentcore/compaction.go` — 上下文压缩
- `doomloop/doomloop.go` — 死循环检测
- `domains/patent.go` — Patent Agent 装配

拆分要求：行为完全不变，测试零回归，不引入任何"顺手优化"。

## 四、Phase 分群

### Phase 1 — 快速收割（15 文件，预计 2 天）

有明显自然拆分边界的文件。

| # | 文件 | 行数 | 拆分模式 | 拆法 |
|---|------|------|----------|------|
| 1 | `domains/workflows/patent/rule_engine.go` | 1203 | 模式4 | 按专利领域拆分规则构造函数（新颖性/创造性/侵权/复审/无效/外观/推理/客体） |
| 2 | `bootstrap/setup.go` | 1195 | 模式1+5 | 类型定义 + 核心 Setup + 各领域初始化函数独立文件 |
| 3 | `cmd/mady/tui_session.go` | 1194 | 模式1+6 | 类型+访问器 / agent管理 / slash handlers / 存储初始化 |
| 4 | `domains/inventiveness/nodes.go` | 1041 | 模式4 | 四步法节点各一文件 + 辅助函数独立 |
| 5 | `a2a/ws.go` | 883 | 模式6 | WebSocket 服务端 / 客户端分离 |
| 6 | `tui/component/markdown.go` | 881 | 模式1+4 | 组件包装 / 内联渲染 / 主题 / 缓存分离 |
| 7 | `domains/evidence/engine.go` | 815 | 模式4 | 三性判断 / 类型识别 / 公开使用评估各一文件 |
| 8 | `knowledge/sqlite/store.go` | 802 | 模式2 | 向量搜索 / FTS 搜索 / 法律搜索 / 图加载各一文件 |
| 9 | `tui/terminal/detect.go` | 795 | 模式5 | 各能力检测函数按类别分组 |
| 10 | `domains/enablement/nodes.go` | 779 | 模式4 | 充分公开各分析步骤节点独立 |
| 11 | `tools/browser_session.go` | 768 | 模式1+2 | 按浏览器后端（CDP/Camofox/Lightpanda/Cloud）分离 |
| 12 | `cmd/mady/slash_registry.go` | 765 | 模式1 | Registry / 各命令组定义分离 |
| 13 | `domains/case_index.go` | 756 | 模式2 | 按实体分组 CRUD（Case/Path/Document/Event）+ 状态流转 |
| 14 | `tui/chat/chat_app_layout.go` | 753 | 模式1 | Header/Footer/Sidebar/Main 布局分离 |
| 15 | `domains/case_extension.go` | 720 | 模式1+4 | 各 tool handler 独立文件 |

### Phase 2 — 模块深耕（41 文件，预计 5-7 天）

需要深入理解模块架构。按模块顺序处理。

**domains/（12 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 16 | `domains/workflows/patent/reexamination.go` | 809 | 紧密耦合的图工作流，节点共享状态类型 |
| 17 | `domains/workflows/patent/invalidation.go` | 796 | 同上 |
| 18 | `domains/workflows/patent/oa_response.go` | 746 | OA 答复工作流 |
| 19 | `domains/claimdrafting/builder.go` | 667 | 权利要求构建器流水线 |
| 20 | `domains/infringement/nodes.go` | 644 | 侵权判断图节点 |
| 21 | `domains/workflows/patent/reasoning_patterns.go` | 634 | 推理模式库 |
| 22 | `domains/workflows/patent/analysis.go` | 539 | 专利分析工作流 |
| 23 | `domains/workflows/legal/comparison.go` | 543 | 法律比较引擎 |
| 24 | `domains/patent.go` | 531 | 🔴 敏感路径 Patent Agent 装配 |
| 25 | `domains/rules/slop_engine.go` | 530 | 反套话引擎 |
| 26 | `domains/novelty/prompts.go` | 520 | 新颖性提示词模板 |
| 27 | `domains/evidence/date.go` | 500 | 证据日期判断规则 |

**tui/（11 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 28 | `tui/chat/chat_app.go` | 1184 | ChatApp + chatModel 深度耦合 |
| 29 | `tui/component/input.go` | 675 | 多模态输入组件 |
| 30 | `tui/chat/chat_history.go` | 680 | 聊天历史管理 |
| 31 | `tui/chat/chat_history_render.go` | 585 | 历史渲染逻辑 |
| 32 | `tui/component/editor_edit.go` | 610 | 编辑器编辑模式 |
| 33 | `tui/component/session_selector.go` | 558 | 会话选择器 |
| 34 | `tui/component/review_gate.go` | 541 | 审批关卡 UI |
| 35 | `tui/overlay.go` | 672 | 覆盖层管理 |
| 36 | `tui/terminal/stdin_buffer.go` | 618 | 标准输入缓冲 |
| 37 | `tui/terminal/keys.go` | 594 | 键盘映射（大量常量，可能豁免） |
| 38 | `tui/terminal/terminal.go` | 512 | 终端 I/O 主文件 |

**tools/（5 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 39 | `tools/browser_supervisor.go` | 662 | 浏览器监督 Agent |
| 40 | `tools/edit.go` | 544 | 文件编辑工具 |
| 41 | `tools/process.go` | 520 | 进程管理工具 |
| 42 | `tools/desktop/computer_use.go` | 512 | 桌面控制工具 |
| 43 | `tools/vision.go` | 508 | 视觉工具 |

**agentcore/（3 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 44 | `agentcore/compaction.go` | 644 | 🔴 敏感路径 上下文压缩 |
| 45 | `agentcore/agent.go` | 536 | 🔴 敏感路径 Agent 生命周期 |
| 46 | `agentcore/event_types.go` | 531 | 事件类型定义（可能豁免） |

**knowledge/（2 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 47 | `knowledge/extension.go` | 698 | 知识扩展主文件 |
| 48 | `knowledge/fileindex/store.go` | 688 | 文件索引存储 |

**a2a/（2 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 49 | `a2a/client.go` | 722 | A2A 客户端 |
| 50 | `a2a/server_jsonrpc.go` | 544 | JSON-RPC 服务端 |

**其他模块（6 文件）**

| # | 文件 | 行数 | 关键挑战 |
|---|------|------|----------|
| 51 | `desktop/app.go` | 1268 | App struct 30 个 Wails binding 方法 |
| 52 | `acp/server.go` | 1013 | ACP 协议服务器 |
| 53 | `mcp/discovery.go` | 796 | MCP 发现管线 |
| 54 | `acp/session.go` | 594 | ACP 会话管理 |
| 55 | `session/session_store.go` | 585 | 会话持久化 |
| 56 | `session/agent_store.go` | 529 | Agent 状态持久化 |

### Phase 3 — 收尾（14 文件，预计 1-2 天）

| # | 文件 | 行数 | 备注 |
|---|------|------|------|
| 57 | `example/cli-chat/main.go` | 919 | **豁免**（入口文件） |
| 58 | `disclosure/novelty.go` | 728 | 技术交底书新颖性 |
| 59 | `agui/converter.go` | 720 | AGUI 事件转换 |
| 60 | `provider/chatcompat/chat.go` | 707 | Chat Completions 兼容 |
| 61 | `memory/sqlite_store.go` | 663 | 记忆 SQLite 存储 |
| 62 | `memory/store.go` | 645 | 记忆存储接口 |
| 63 | `doomloop/doomloop.go` | 625 | 🔴 敏感路径，**建议豁免**（结构清晰、单文件更安全） |
| 64 | `evaluate/benchmark/patent_exam_real_a22.go` | 619 | 评估基准 |
| 65 | `server/disclosure.go` | 562 | HTTP disclosure endpoint |
| 66 | `evaluate/metrics.go` | 557 | 评估指标 |
| 67 | `agentcore/tool_gen.go` | 512 | **豁免**（代码生成） |
| 68 | `session/session.go` | 511 | 会话核心 |
| 69 | `provider/chatcompat/responses.go` | 501 | Responses API 兼容 |
| 70 | `tui/layout/flex.go` | 539 | Flex 布局引擎 |

## 五、预估汇总

| Phase | 文件数 | 实际拆分 | 豁免 | 预计耗时 |
|-------|--------|----------|------|----------|
| Phase 1 | 15 | 15 | 0 | 2 天 |
| Phase 2 | 41 | ~39 | ~2 | 5-7 天 |
| Phase 3 | 14 | ~10 | ~4 | 1-2 天 |
| **合计** | **70** | **~64** | **~6** | **8-11 天** |

## 六、执行顺序

Phase 1 → 全量验证 → Phase 2（按模块）→ 全量验证 → Phase 3 → 全量验证。

每个文件完成后更新进度清单，标记 `pending → in_progress → completed`。

每完成一个模块运行 `go test -race ./<module>/...`。
每完成一个 Phase 运行 `go test ./...` 全量回归。
