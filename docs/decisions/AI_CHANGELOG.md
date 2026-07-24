# AI 变更记录

## 2026-07-24: 专利撰写路由修复 — System Prompt 显式指定 draft_claims / draft_specification

### 背景
专利 Agent 的 System Prompt 在 Step 4（执行）中只提到 `patent_lookup` 工具，未显式告知 LLM 撰写权利要求和说明书时必须使用 `draft_claims` / `draft_specification` 工具。导致 LLM 倾向于直接手写文本，完全绕过了 claimdrafting（8 节点 Pregel 图 + 规则校验）和 specdrafting（12 节点 Pregel 图 + 16 条校验）工作流。

### 改动清单

| 文件 | 改动 |
|------|------|
| `domains/patent.go` | System Prompt Step 4：从"分析权利要求、生成文书"改为"撰写权利要求时，必须调用 draft_claims 工具（禁止手写）；撰写说明书时，必须调用 draft_specification 工具（禁止手写）" |
| `domains/claimdrafting/extension.go` | draft_claims 工具描述新增"当用户要求撰写权利要求、写权利要求书时，必须调用此工具，严禁自行手写权利要求文本" |
| `domains/specdrafting/extension.go` | draft_specification 工具描述新增"当用户要求撰写说明书、写专利申请文件时，必须调用此工具，严禁自行手写说明书文本" |

### 影响
从 TUI 发起专利撰写时，LLM 将在 System Prompt 和工具描述的双重指引下优先调用结构化工作流工具，而不是直接输出文本。

## 2026-07-24: 结构化任务管理 — Plantask 工具集 + TUI TodoPanel

### 背景
Mady 缺乏结构化任务列表管理能力。LLM 在处理复杂多步骤任务时无法自行规划、追踪进度。通过对比 Eino 框架和 Claude Code 的 TodoWrite 设计，发现 Mady 已有一个预构建但从未实例化的 `tui/component/todo_panel.go` 组件和一个 planmode 中的幽灵 `"todo": true` 条目。本次正式引入 Plantask 功能，填补这一空缺。

### 改动清单

| 文件 | 改动 |
|------|------|
| `agentcore/task_types.go` | 新增 Task 数据模型（TaskStatus / TaskPriority / Task + Clone/Order），定义在 agentcore 包避免循环导入 |
| `agentcore/tasklist/store.go` | 新增 Store 接口 + MemoryStore（内存实现，用于测试） |
| `agentcore/tasklist/filestore.go` | 新增 FileStore（文件系统实现，原子写入 + .nextid 计数器持久化 + 启动时从现有文件推断 ID） |
| `agentcore/tasklist/prompts.go` | 双语（中/英）工具描述，遵循 tone-style-guide（无绝对化表述） |
| `agentcore/tasklist/tool_create.go` | task_create 工具：创建任务，发射 TaskCreatedEvent |
| `agentcore/tasklist/tool_get.go` | task_get 工具（ReadOnly）：查询单个任务详情 |
| `agentcore/tasklist/tool_update.go` | task_update 工具：更新状态/优先级/依赖，含 DFS 循环依赖检测 + 双向 blocks/blockedBy 维护，发射 TaskUpdatedEvent |
| `agentcore/tasklist/tool_list.go` | task_list 工具（ReadOnly）：列出所有任务含统计摘要 |
| `agentcore/tasklist/extension.go` | tasklist Extension（实现 Extension + ToolProvider + EventSnapshotProvider） |
| `agentcore/tasklist/*_test.go` | 30+ 单元测试：Store CRUD、工具全路径、Extension 生命周期、FileStore 持久化、循环依赖检测 |
| `agentcore/event_types.go` | 新增 EventTaskCreated / EventTaskUpdated 事件类型 |
| `cmd/mady/framework.go` | 注入 tasklist Extension（~/.mady/sessions/tasks/） |
| `agentcore/planmode/policy.go` | alwaysAllowed 新增 task_list / task_get（ReadOnly 工具在计划模式下始终可用） |
| `tui/chat/events.go` | 新增 ChatEventTaskCreated / ChatEventTaskUpdated 事件类型 |
| `tui/agentadapter/adapter.go` | agentcore 事件 → ChatEvent 桥接 |
| `tui/chat/chat_app.go` | ChatApp 新增 todoPanel / tasks 字段，Subscribe 注册任务事件 |
| `tui/chat/chat_app_todo.go` | 新增任务事件处理器 + ToggleTodoPanel / CloseTodoPanel overlay 管理 |
| `tui/chat/chat_app_layout.go` | Ctrl+T 快捷键切换 TodoPanel |
| `tui/chat/chat_app_test.go` | 订阅测试更新（17 handlers） |
| `tui/component/todo_panel.go` | 新增 archived 状态支持 + onClose 回调 + todo.close 键处理 |
| `tui/terminal/keybindings.go` | 新增 tui.todo.toggle 绑定（Ctrl+T） |

### 设计决策
- **Task 类型定义在 agentcore 包**：避免循环导入——tasklist 包引用 agentcore，事件类型在 agentcore/event_types.go 中引用 *Task。
- **archived 而非 deleted**：保留审计留痕，不物理删除任务。List 默认排除 archived，可通过参数查询。
- **Store.UpdateFunc 原子原语**（code review 采纳）：工具层的 read-modify-write 通过 `UpdateFunc(ctx, id, mutate)` 在 Store 锁保护下原子完成，消除了 Extension 层的 `sync.Mutex`。依赖关系的反向写入各自独立调用 `UpdateFunc`，校验全部在写入前完成。
- **planmode 门控**：task_get 和 task_list 为 ReadOnly（计划模式始终可用），task_create 和 task_update 被门控阻止。
- **FileStore 原子写入**：写临时文件 + rename 模式防止崩溃导致数据损坏；.nextid 计数器持久化保证重启后 ID 单调递增。
- **EventSnapshotProvider + TUI 绑定**：新 TUI 挂载时在 `BindAgent` 后调用 `agent.EmitExtensionSnapshots()`，自动恢复任务列表状态（修复 code review 发现的 dead snapshot 问题）。

### 安全敏感路径
- `agentcore/planmode/policy.go` 修改了 alwaysAllowed 白名单，新增 task_list / task_get 两个 ReadOnly 工具。这两个工具不修改任何状态，仅查询任务列表和详情，安全等级与已有的 "ask" 工具一致。
- `cmd/mady/framework.go` 注入 tasklist Extension，数据目录通过 `util.ResolveDataDir("sessions")` 解析，遵循统一的资源定位规范。

### 验证结果
- `go build ./...`：通过 ✅
- `go vet ./...`：通过 ✅
- `go test -race ./agentcore/tasklist/...`：30+ 测试全部通过 ✅
- `go test ./...`：全部通过 ✅


## 2026-07-24: PDF 质量升级 — chromedp Chrome 引擎 + Pandoc 启用

### 背景
P0 阶段已完成 HTML 渲染器（goldmark）和 Pandoc 集成。本次 P1 将 PDF 输出从 gopdf 手动布局升级为 headless Chrome 引擎渲染（CSS 级排版），同时在 PatentAgentConfig 和 BuildProjectAgent 中正式启用 Pandoc convert_document 工具。

### 改动清单

| 文件 | 改动 |
|------|------|
| `domains/doctmpl/renderer_pdf_chrome.go` | 新增 PDFChromeRenderer（Markdown→HTML→Chrome PrintToPDF）和 PDFAutoRenderer（惰性探测 Chrome，不可用自动回退 gopdf） |
| `domains/doctmpl/renderer_pdf_chrome_test.go` | 新增 7 个测试：Format、nil 安全×2、端到端 PDF 渲染、探测一致性、probeChrome 超时保护 |
| `domains/doctmpl/store.go` | PDF 渲染器从 `&PDFRenderer{}` 替换为 `NewPDFAutoRenderer()` |
| `domains/patent.go` | PatentAgentConfig 和 BuildProjectAgent 的 ExtensionConfig 注入 `Pandoc: tools.PandocToolConfigDefaults()` |
| `go.mod` | chromedp/cdproto 从 indirect 提升为 direct 依赖 |

### 设计决策
- **Chrome 引擎 vs gopdf**：gopdf 手动布局无法精确还原 CSS 排版（表格列宽、代码高亮、引用块样式）。Chrome 引擎通过 page.PrintToPDF() 实现 W3C 标准排版，同时利用系统级中文字体（不需要搜索 TTF/TTC 文件）。
- **优雅降级架构**：PDFAutoRenderer 使用 sync.Once 在首次 Render 时惰性探测 Chrome 可用性。探测成功→委托 PDFChromeRenderer；失败→回退 PDFRenderer(gopdf)。探测仅执行一次，后续调用零开销。CI/无 Chrome 环境自动降级，开发者无感。
- **HTML 作为中间格式**：复用 P0 的 HTMLRenderer（goldmark + 内嵌 CSS）作为 Chrome PDF 的输入。一套 CSS 同时服务于 HTML 输出和 PDF 输出，保证视觉一致性。
- **ChromeWSURL 字段**：PDFChromeRenderer 支持远程 Chrome（WebSocket），为容器化/集群部署预留扩展点；为空时使用本地 headless Chrome。
- **Pandoc 在两个 Agent 构建点启用**：PatentAgentConfig（默认专利 Agent）和 BuildProjectAgent（案件级动态 Agent）均注入 PandocToolConfigDefaults()，确保所有专利 Agent 均可用 convert_document 工具。

### 安全敏感路径
- `domains/patent.go` 是安全敏感路径（BuildProjectAgent 动态 WorkingDir）。Pandoc 注入不修改 WorkingDir/沙箱边界，仅新增工具注册。Pandoc 的沙箱路径校验已在 tools/pandoc.go 的 resolvePath 中实现。
- `domains/doctmpl/store.go` 的渲染器注册变更不影响安全边界——PDFAutoRenderer 仅替换 PDF 格式的渲染实现，不改变接口契约。

### 验证结果
- `go build ./...`：通过 ✅（根模块 + tools 子模块）
- `go vet ./...`：通过 ✅
- `go test ./domains/doctmpl/`：57 个测试全部通过 ✅（含 7 个新增 Chrome PDF 测试）
- `golangci-lint run`：0 issues ✅




## 2026-07-24: 文档处理增强 — HTML 渲染器 + Pandoc 集成

### 背景
doctmpl 模块定义了 5 种输出格式（markdown/docx/pdf/html/email），但 HTML/Email 渲染器未实现；项目缺乏双向文档转换能力（DOCX→Markdown 等）。本次补全 HTML 渲染器并集成 Pandoc 外部工具。

### 改动清单

| 文件 | 改动 |
|------|------|
| `domains/doctmpl/renderer_html.go` | 新增 HTMLRenderer，使用 goldmark（CommonMark+GFM）将 Markdown 渲染为独立 HTML5 文档，含响应式 CSS、打印样式、免责声明/标题注入 |
| `domains/doctmpl/renderer_html_test.go` | 新增 6 个测试：Format、nil 安全、基本 Markdown、完整文档（标题/作者/免责/表格/代码块）、语言属性、GFM 扩展特性 |
| `domains/doctmpl/format.go` | RenderMeta 新增 Language 字段（影响 HTML lang 属性，默认 zh-CN） |
| `domains/doctmpl/store.go` | NewTemplateStore 注册 HTMLRenderer |
| `tools/pandoc.go` | 新增 convert_document 工具，通过 Pandoc CLI 实现文档格式互转（markdown/docx/html/pdf/epub/latex 等），支持 reference-doc 模板、TOC、沙箱路径校验 |
| `tools/tools.go` | ExtensionConfig 新增 Pandoc 字段；BuildTools 条件注册（Pandoc 非空时注册）；新增 ToolPandoc 常量 |
| `go.mod` | 新增 github.com/yuin/goldmark v1.8.4 依赖 |

### 设计决策
- **HTML 渲染器用 goldmark**：项目此前无 Markdown 解析库，PDF/DOCX 渲染器使用手写字符串匹配。HTML 需要语义正确的标签结构，goldmark（CommonMark + GFM 扩展）是 Go 生态最成熟的解析器，MIT 许可、纯 Go、零 CGO。
- **Pandoc 作为外部工具集成**：而非自研 OOXML 引擎。Pandoc 支持 40+ 格式互转，通过 CLI 调用（遵循 PatentToolConfig 模式），GPL 许可但作为外部进程调用无传染性。
- **Pandoc 条件注册**：只有 ExtensionConfig.Pandoc 非空时才注册工具，与 Browser 工具模式一致。用户需系统安装 Pandoc，通过 domains/patent.go 的 PatentAgentConfig 启用。
- **沙箱路径校验**：PandocToolConfig.resolvePath 阻止输入/输出路径逃逸 WorkingDir（当 WorkingDir 非空时）。

### 安全敏感路径
- `tools/tools.go` 新增 Pandoc 配置字段和条件注册，不修改现有安全边界（ExtensionConfig.DisableTools/EnabledTools 门控逻辑不变）。
- `tools/pandoc.go` 实现路径校验（resolvePath），防止目录遍历攻击。
- 不暴露 pandoc 的危险参数（如 --lua-filter 可执行任意代码），仅开放 reference-doc/toc/standalone 等安全参数。

### 验证结果
- `go build ./...`：通过 ✅（根模块 + tools 子模块）
- `go vet ./...`：通过 ✅
- `go test ./domains/doctmpl/`：50 个测试全部通过 ✅
- `golangci-lint run`：0 issues ✅


## 2026-07-24: B 档高价值接线 — guardian/tracing/evidence/disclosure

### 背景
原始 B 档计划中的 4 项高价值未接线模块（实现完整、设计合理、只差装配代码）。本次全部激活。

### 改动清单

| 文件 | 改动 |
|------|------|
| `cmd/mady/framework.go` | 注入 `evidence.NewExtension()` 到 BaseConfig.Extensions（全局工具调用审计账本） |
| `cmd/mady/framework.go` | 新增 `MADY_TRACING=stdout` 条件启用 OTel 追踪 + frameworkContext.TracerFlush 字段 |
| `cmd/mady/framework.go` | 新增 `MADY_GUARDIAN=1` 条件启用 AI 安全审查熔断器 + frameworkContext.GuardianExt 字段 |
| `domains/patent.go` | ExtraTools 新增 `disclosure.NewDisclosureTool(base.Provider)`，11 节点 Pregel 管线对 Agent 可见 |

### 设计决策
- **evidence 默认启用**：审计账本是无副作用的只读记录（BeforeTurn 重置 + AfterToolExecution 记录 Receipt），对所有 Agent 透明接入。
- **tracing 条件启用**：OTel 追踪通过 `MADY_TRACING=stdout` 环境变量控制，默认关闭（零开销）。启用后 Agent 执行的每个阶段生成 span。
- **guardian 条件启用**：AI 安全审查每次非只读工具调用会触发额外 LLM 调用，通过 `MADY_GUARDIAN=1` 显式启用。内置熔断器在连续拒绝时自动放行，防止 Guardian 故障阻塞工作流。
- **disclosure 默认注册**：11 节点交底书分析 Pregel 管线是 PatentAgent 的核心能力，直接加入 ExtraTools 列表。

### 安全敏感路径
- `guardrails/guardian/` 接线涉及安全边界（AI 熔断器），采用条件启用（默认关闭）避免影响生产稳定性。
- `disclosure` 工具注册到 `domains/patent.go` PatentAgentConfig.ExtraTools，不影响 WorkingDir 沙箱或路由白名单。

### 验证结果
- `go build ./...`：通过 ✅
- `go vet ./...`：通过 ✅
- `go test ./...`：80 包全部通过 ✅

---

## 2026-07-24: 孤儿代码清理 + 未接线模块激活

### 背景
通过 3 个并行 explore 子代理 + codegraph 调用图分析 + 全量 grep 验证，发现 14 个完全孤儿包（零外部导入者）和 7 处已接线包内的死代码。本次处理用户确认的 A 档（删除废弃代码）和 B 档（接线已实现但未装配的扩展）。

### 改动清单

**A 档：删除废弃代码（6 项）**

| 操作 | 目标 | 理由 |
|------|------|------|
| 删除整个包 | `filequeue/` | 从未被任何代码导入 |
| 删除整个包 | `agentcore/cache/` | doc.go 自承认"存储未实现" |
| 删除整个包 | `workflow/` | Pipeline/Parallel/Router 被 Pregel + Handoff 双重替代 |
| 删除整个文件 | `domains/graph.go` | BuildDomainGraph 被 Handoff 替代（零外部调用） |
| 删除函数 | `workflows/patent/tool.go` NewSpecificationTool | Deprecated，被 specdrafting 替代 |
| 删除函数 | `workflows/patent/tool.go` NewInfringementTool | Deprecated，被 domains/infringement 替代 |

**B 档：接线未装配扩展（3 项）**

| 文件 | 改动 |
|------|------|
| `cmd/mady/framework.go` | 新增 `filecheckpoint.NewExtension()` 注入到 BaseConfig.Extensions，提供编辑前快照安全网 |
| `cmd/mady/framework.go` | 新增 `planmode.NewExtension()` 注入到 BaseConfig.Extensions + frameworkContext.PlanModeExt 引用（默认不激活，零开销） |
| `a2a/options.go` | 新增 `WithFederation(reg, pool)` ServerOption |
| `a2a/server.go` | 新增 federationRegistry/federationPool 字段、pool 生命周期管理（Start/Shutdown）、`/federation/agents` 端点 |

### 设计决策
- **filecheckpoint 默认启用**：编辑安全网是无副作用的只读快照，对所有 Agent 透明接入，不需要配置开关。
- **planmode 默认不激活**：通过 `atomic.Bool` 控制开关，不激活时零开销。frameworkContext 存储 PlanModeExt 引用，TUI 后续可通过 Activate/Deactivate 控制开关。
- **a2a 联邦向后兼容**：WithFederation 传 nil 时不启用联邦功能（默认行为），已有 server 代码无需修改。
- **infringement.go 保留**：子代理报告说"被 domains/infringement 替代"，但 `BuildInfringementGraph()` 仍被 `cmd/mady/tui_session.go:805` 和 `cmd/mady/patent.go:281` 调用，删除会破坏编译。

### 暂缓项（C 档重构）
- **protocol/jsonrpc 统一**：a2a 的 `Result any` 与 protocol/jsonrpc 的 `Result json.RawMessage` 不兼容，`Code int` vs `int64` 差异，直接替换会改变协议序列化行为。需独立设计迁移方案。
- **retrieval/internal 复用**：Go internal 包机制限制 knowledge/memory 无法导入 retrieval/internal，需先移到公共位置（如 pkg/vecbytes/）。

### 验证结果
- `go build ./...`：通过 ✅
- `cd tools && go build ./...`：通过 ✅
- `cd tui && go build ./...`：通过 ✅
- `go test ./...`：全部通过 ✅
- `go vet ./...`：通过 ✅

### C 档：重构统一（2 项）

**C-1：浮点编码统一 → pkg/vecbytes/**

| 文件 | 改动 |
|------|------|
| `pkg/vecbytes/vecbytes.go` | 新建公共包，提供 `FloatsToBytes` / `BytesToFloats` |
| `pkg/vecbytes/vecbytes_test.go` | 新建测试 |
| `knowledge/sqlite/store.go` | 删除 `bytesToFloat32`，改用 `vecbytes.BytesToFloats` |
| `knowledge/sqlite/writable.go` | 删除 `float32ToBytes`，改用 `vecbytes.FloatsToBytes` |
| `knowledge/sqlite/writable_test.go` | 测试改用 `vecbytes` |
| `memory/sqlite_store.go` | 删除 `floatsToBytes` / `bytesToFloats`，改用 `vecbytes` |
| `retrieval/internal/` | 删除整个孤儿包（浮点编码 + L2Norm/TopKByScore/RRFScore 全部未使用） |

**C-2：删除 protocol/jsonrpc 孤儿包**

| 文件 | 改动 |
|------|------|
| `protocol/jsonrpc/` | 删除整个目录（36 行孤儿包，a2a/acp 各自维护 JSON-RPC 类型且工作正常） |

### 设计决策（C 档）
- **vecbytes 提取而非保留 internal**：`retrieval/internal` 受 Go internal 包机制限制，knowledge/memory 无法导入。提取到 `pkg/vecbytes/` 后三方复用同一实现，消除 3 处重复编码逻辑。
- **protocol/jsonrpc 删除而非统一**：a2a 的 `Result any` 与 acp 的 `Result json.RawMessage` 存在本质差异，`Code int` vs `int64`、`ID` json tag 也不一致。真正统一需修改两个协议层的序列化行为，风险过高。删除孤儿包是最安全的选择，a2a/acp 保持各自副本。

### 背景
前序阶段已完成 `prompt/templates/` 目录、`prompt.PromptStore`、模板解析与内联 prompt 迁移。Phase 5 将模板仓库暴露给用户和 Agent，支持浏览可用模板。

### 改动清单

| 文件 | 改动 |
|------|------|
| `prompt/tool.go` | 新建 `list_prompts` Agent 工具，按 category/domain/query 筛选并返回模板摘要 |
| `cmd/mady/util.go` | 新建 `mady util list-prompts` 子命令，打印人类可读的模板目录 |
| `cmd/mady/util_test.go` | 新建：捕获 stdout 测试 `runListPrompts` |
| `cmd/mady/main.go` | 新增 `case "util":` 分支与 usage 说明 |
| `domains/patent.go` | `BuildProjectAgent` 调用 `injectPromptTools(&cfg)` 注册 `list_prompts` |
| `domains/legal.go` | `BuildLegalAgent` 调用 `injectPromptTools(&cfg)` 注册 `list_prompts` |
| `domains/prompt_store.go` | 新增 `injectPromptTools`：在全局 PromptStore 存在时注入 `list_prompts` 工具 |

### 设计决策
- **工具与 CLI 分离**：Agent 工具返回结构化 JSON，便于 LLM 消费；CLI 返回文本表格，便于人类查看。
- **不强制依赖完整框架**：`mady util` 子命令独立加载 `prompt.PromptStore`，不需要 `frameworkContext`，方便在任意目录快速查看模板列表。
- **按领域注册工具**：仅在 `patent` / `legal` 领域 Agent 显式注册 `list_prompts`，避免向所有 Agent 暴露无关工具；后续可在 `domains/chat.go` 等更多领域按需注入。
- **敏感路径**：本次未改动 `guardrails/guardian/` 等安全敏感文件；`domains/patent.go` 与 `domains/legal.go` 仅增加一行 `injectPromptTools(&cfg)` 调用，不影响原有 WorkingDir 或路由白名单逻辑。

### 验证结果
- `go build ./...`：通过 ✅
- `cd tools && go build ./...`：通过 ✅
- `go test -race ./...`：通过 ✅
- `go vet ./...`：通过 ✅
- `golangci-lint run`：通过 ✅（0 issues）

---

## 2026-07-24: prompt-templates 接线 — Phase 4.2（基础设施内联 prompt 迁移）

### 背景
Phase 4.1 已迁移 3 个工作流节点内联 prompt。Phase 4.2 将记忆、评估、Guardian 等基础设施模块中的内联 system prompt 迁移到模板库，完成核心内联提示词的集中管理。

### 改动清单

| 文件 | 改动 |
|------|------|
| `prompt/templates/memory/fact-extraction.json` | 新建：记忆事实提取（structured output 模式） |
| `prompt/templates/memory/fact-extraction-fallback.json` | 新建：记忆事实提取（纯文本回退模式） |
| `prompt/templates/memory/session-summary.json` | 新建：会话记忆汇总 |
| `prompt/templates/memory/dedup-decision.json` | 新建：记忆去重判定 |
| `prompt/templates/evaluate/llm-judge.json` | 新建：LLM 评分裁判 |
| `prompt/templates/guardian/patent-legal-policy.json` | 新建：Guardian 专利/法律安全策略 |
| `memory/extractor_llm.go` | `extractionSystemPrompt` / `extractionFallbackPrompt` 改为常量兜底 + `prompt.ResolveSystemPromptOr` |
| `memory/session_summarizer.go` | `summarySystemPrompt` 改为常量兜底 + 模板解析 |
| `memory/dedup_llm.go` | `dedupSystemPrompt` 改为常量兜底 + 模板解析 |
| `evaluate/llm_judge.go` | `DefaultLLMJudgePrompt` 改为未导出 `defaultLLMJudgePromptFallback` + 模板解析；同步更新 `evaluate/reflection.go` 注释 |
| `guardrails/guardian/guardian.go` | `NewSession` 默认 Policy 通过模板解析获取，保留 `PatentLegalPolicy` 作为兜底 |
| `prompt/store.go` | `ResolveSystemPromptOr` 已在前序阶段提供 |

### 设计决策
- **敏感路径处理**：`guardrails/guardian/guardian.go` 属于 Guardian AI 熔断器（敏感路径）。本次改动仅将硬编码 Policy 字符串改为"模板解析 + 原字符串兜底"，未修改风险分级、判定逻辑或熔断器行为；Policy 内容保持不变。
- **基础设施层不反向依赖 domains**：`memory` / `evaluate` / `guardian` 直接 import `prompt` 包，使用 `prompt.SetDefaultStore` 注册的全局默认 Store。
- **保持 API 兼容**：`guardrails/guardian` 的 `PatentLegalPolicy` 公共常量继续保留作为兜底；`evaluate` 的 `DefaultLLMJudgePrompt` 改为未导出，但它在包外无直接使用（grep 仅发现注释引用）。

### 验证结果
- `go build ./...`：通过 ✅
- `go test -race ./memory/... ./evaluate/... ./guardrails/... ./prompt/... ./cmd/mady/... ./domains/... ./pkg/agentconfig/... ./workflows/patent/... ./disclosure/...`：通过 ✅
- `golangci-lint run ...`：通过 ✅

### 下一步
Phase 5（可选）：暴露 `list_prompts` 工具或 `mady util list-prompts` CLI，让用户/Agent 可以浏览可用模板。

---

## 2026-07-24: prompt-templates 接线 — Phase 4.1（工作流节点内联 prompt 迁移）

### 背景
Phase 3 已提供 `prompt://<name>` 解析能力，但尚未有实际调用方。Phase 4.1 将 3 个高频工作流节点的内联 system prompt 迁移到模板库，实现集中管理。

### 改动清单

| 文件 | 改动 |
|------|------|
| `prompt/templates/workflow/oa-response-enhance.json` | 新建：OA 答复增强节点的 system prompt |
| `prompt/templates/disclosure/novelty-analysis.json` | 新建：新颖性 batch 分析 system prompt |
| `prompt/templates/disclosure/novelty-per-feature.json` | 新建：新颖性 per-feature 分析 system prompt |
| `prompt/templates/disclosure/keyword-extraction.json` | 新建：检索关键词生成 system prompt |
| `workflows/patent/oa_response.go` | SystemPrompt 改为 `prompt.ResolveSystemPromptOr("prompt://oa-response-enhance", 内联兜底)` |
| `disclosure/novelty.go` | `buildNoveltyPrompt` / `buildPerFeatureNoveltyPrompt` 改为常量兜底 + `prompt.ResolveSystemPromptOr`；新增 `prompt` import |
| `disclosure/keywords.go` | 内联 system prompt 改为 `prompt.ResolveSystemPromptOr(...)` + 常量兜底 |
| `prompt/store.go` | 新增 `ResolveSystemPromptOr(raw, fallback string)` 便捷函数 |
| `domains/prompt_store.go` | 新增 `ResolveSystemPromptOr` 包装 |
| `domains/prompt_store_test.go` | 补充 `ResolveSystemPromptOr` 缺失模板回退测试 |

### 设计决策
- **避免层间循环依赖**：`disclosure` 属于基础设施层，不反向 import `domains`；通过 `prompt.SetDefaultStore` + `prompt.ResolveSystemPromptOr` 解析模板。
- **保留内联兜底**：每个迁移点保留原始字符串常量作为 fallback，模板缺失时行为与旧代码完全一致。
- **模板命名空间**：工作流节点用 `workflow/oa-response-enhance`，disclosure 用 `disclosure-*` 前缀，避免与 `analysis/novelty.json` 等通用模板混淆。

### 验证结果
- `go build ./...`：通过 ✅
- `go test -race ./workflows/patent/... ./disclosure/... ./prompt/... ./cmd/mady/... ./domains/... ./pkg/agentconfig/...`：通过 ✅
- `golangci-lint run ...`：通过 ✅

### 下一步
Phase 4.2（可选）：评估是否迁移 `memory/*`、`evaluate/llm_judge.go`、`guardrails/guardian/guardian.go` 中的内联 prompt。其中 `guardian.go` 属敏感路径，需人工审阅。

---

## 2026-07-24: prompt-templates 接线 — Phase 3（模板引用解析）

### 背景
Phase 2 已将 `prompt.PromptStore` 注入 `frameworkContext` 与 `domains`。Phase 3 提供 `prompt://<name>` 引用语法的解析能力，使内联 system prompt 可以无缝替换为模板库中的模板。

### 改动清单

| 文件 | 改动 |
|------|------|
| `pkg/agentconfig/resolve.go` | 新增：`ResolveSystemPrompt` / `ResolveSystemPromptStrict` + `PromptTemplateStore` 接口；`system_prompt: "prompt://name"` 解析为对应模板的 `system_prompt` |
| `pkg/agentconfig/resolve_test.go` | 新增：内联保留、模板解析、缺失回退、nil store、strict 模式等测试 |
| `domains/prompt_store.go` | 新增 `ResolveSystemPrompt(raw string) string` 便捷函数，基于已注入的全局 `PromptStore` |
| `domains/prompt_store_test.go` | 新增：内联保留、模板解析、缺失回退测试 |

### 设计决策
- **无字段侵入**：不新增 `agentconfig.Config` 字段，复用现有 `SystemPrompt` 字符串；以 URI 前缀区分模板引用与内联文本。
- **安全降级**：模板未找到或 Store 未注入时，原样返回原始字符串，避免启动或运行时崩溃。
- **接口隔离**：`PromptTemplateStore` 接口让 `pkg/agentconfig` 测试无需构造完整 `PromptStore`。

### 验证结果
- `go build ./...`：通过 ✅
- `go test -race ./prompt/... ./pkg/agentconfig/... ./domains/...`：通过 ✅
- `golangci-lint run ./prompt/... ./pkg/agentconfig/... ./domains/...`：通过 ✅

### 下一步
Phase 4.1：迁移工作流节点内联 prompt（`workflows/patent/oa_response.go`、`disclosure/novelty.go`、`disclosure/keywords.go`）。

---

## 2026-07-24: prompt-templates 接线 — Phase 2（框架注入）

### 背景
Phase 1 已建立 `prompt.PromptStore` 和 `//go:embed` 加载能力，但 Store 尚未被任何运行时入口持有。Phase 2 将 Store 注入三个入口（tui/serve/acp）共享的 `frameworkContext`，并通过 `domains.SetupPromptStore` 暴露给领域层。

### 改动清单

| 文件 | 改动 |
|------|------|
| `cmd/mady/framework.go` | `frameworkContext` 新增 `PromptStore *prompt.PromptStore`；`initReasoningAndTemplates` 中创建 `prompt.NewPromptStore` 并调用 `domains.SetupPromptStore` |
| `domains/prompt_store.go` | 新增：`globalPromptStore` + `SetupPromptStore` + `PromptStore()` getter |

### 设计决策
- **注入位置**：与 `doctmpl.NewTemplateStore` 并列放在 `initReasoningAndTemplates`，因为二者都是"模板类资源"的初始化。
- **用户覆盖目录**：`$MADY_HOME/prompt-templates/`，与 `doc-templates/` 对称。
- **启动失败降级**：加载失败仅打印警告，不影响启动；上层使用 `prompt://` 时若未找到模板会回退到内联提示词。
- **全局 getter 模式**：与 `domains.GetPatentRetriever` / `SetupDocTemplateStore` 保持一致，领域层无需持有 `frameworkContext`。

### 验证结果
- `go build ./...`：通过 ✅
- `go test -race ./...`：通过 ✅
- `golangci-lint run ./prompt/... ./cmd/mady/... ./domains/...`：通过 ✅

### 下一步
Phase 3：在 `pkg/agentconfig` 中支持 `system_prompt: "prompt://<name>"` 引用语法，并在 Agent 构建前解析。

---

## 2026-07-24: prompt-templates 接线 — Phase 1（Store + Embed + 目录迁移）

### 背景
`prompt/` 包已提供模板加载与渲染 API，`prompt-templates/` 目录已存放 20 个 curated JSON 模板，但两者从未被任何代码引用，处于"数据沉睡"状态。本阶段为全接线计划的第一阶段，目标是建立可嵌入二进制、可被运行时消费、支持用户覆盖的 PromptTemplate Store。

### 改动清单

| 文件 | 改动 |
|------|------|
| `prompt/templates/` | 从仓库根 `prompt-templates/` 移入 `prompt/` 包内，保持子目录结构不变 |
| `prompt/embed.go` | 新增：`//go:embed templates/*.json templates/**/*.json`，将 20 个模板编译进二进制 |
| `prompt/store.go` | 新增：`PromptStore`（线程安全）+ `NewPromptStore(userRoots...)`，支持内置模板 + `$MADY_HOME/prompt-templates/` 用户覆盖 |
| `prompt/loader.go` | 新增 `LoadPromptsFromFS(fsys fs.FS, root string)`；抽离 `unmarshalPrompt` 供磁盘与 embed 加载复用；更新包注释 |
| `prompt/store_test.go` | 新增：嵌入加载、用户覆盖、并发读、触发词匹配、List/Index 等测试 |
| `CLAUDE.md` / `README.md` / `architecture.html` | 更新目录结构引用：`prompt-templates/` → `prompt/templates/` |

### 设计决策
- **目录迁移**：遵循 `agentcore/manifests/` 与 `domains/doctmpl/templates/` 的既有惯例——"哪个包消费数据，数据就放在哪个包下"，从而可直接使用 `go:embed`。
- **覆盖语义**：先加载内置模板，再叠加 `$MADY_HOME/prompt-templates/` 中的用户模板；同名模板用户优先。
- **向后兼容**：`prompt` 包原有 API（`LoadPrompts` / `ResolvePrompt` / `FindPromptBy*`）保持不变，新增 Store 不破坏现有调用方。

### 验证结果
- `go build ./prompt/...`：通过 ✅
- `go test -race ./prompt/...`：通过 ✅

### 下一步
Phase 2：在 `cmd/mady/framework.go` 的 `initReasoningAndTemplates` 中创建并注入 `PromptStore`。

---

## 2026-07-24: claimdrafting Pregel 图 — 8 节点编排替代串行 Builder

### 背景
权利要求起草（`domains/claimdrafting`）之前只有串行 `ClaimBuilder.Build()` 五步法（特征分类→独立→从属→验证），没有独立 Pregel 工作流，与 `domains/specdrafting` 的 12 节点 Pregel 图架构不对称。用户选了路线 A：在 `domains/claimdrafting/` 内增 Pregel 图。

### 改动清单

| 文件 | 改动 |
|------|------|
| `domains/claimdrafting/types.go` | 追加 9 个 `StateKey*` 常量（Pregel 状态键）和 `timestamp()` 辅助函数 |
| `domains/claimdrafting/graph.go` | **新建** — `BuildClaimGraph(engine, scorer) *CompiledPregelGraph`，8 节点 Pregel 图（load_input→classify_features→draft_primary→draft_parallel→draft_dependents→validate→score→finalize） |
| `domains/claimdrafting/nodes.go` | **新建** — 8 个节点实现 + `stateHasSkip`/`extractInput`/`collectAllClaims`/`buildClaimSet` 辅助 |
| `domains/claimdrafting/extension.go` | `Extension` 新增 `graph` 字段，`NewExtension` 预编译图（engine 不可为 nil），`handleDraftClaims` 无 drafter 时改走 `graph.Run()` |

### 设计决策
- **架构对标 specdrafting**：完全相同模式（`BuildClaimGraph` → `AddNode/AddEdge` → `Compile` → `Run`），注入共享 `builder`/`engine`/`scorer` 闭包给节点
- **节点职责拆分**：`buildIndependentClaims` 中的策略分支拆为 `draft_primary`（主权要）+ `draft_parallel`（并列权要），前者总是执行，后者在 `StrategyProductOnly` 时跳过
- **8 节点 vs specdrafting 12 节点**：claimdrafting 不需要说明书的多章节节点，按五步法自然映射为 5 个起草节点 + 3 个辅助节点（load/validate/score/finalize），最少化但职责完整
- **drafter 路径不变**：`LLMDrafter.DraftFromScratch` 仍使用内部 `builder.Build()` 做降级输出，不受 Pregel 图影响
- **向后兼容**：工具签名、输入参数、输出结构不变；`handleDraftClaims` 的 drafter/非 drafter 双路径保留

### Code Review 修复

| 问题 | 所在 | 修复 |
|------|------|------|
| `finalizeNode` 重复 warnings | `nodes.go:301-307` | 移除第二个 violations 循环（`ScoreReport.Violations` 已是完整来源） |
| `validateNode` 的 `engine.Validate` 结果仅被 `scoreNode` 消费 | `nodes.go:229-231` | 改 `violations :=` 为 `_ =`（验证仍执行，结果不再写 state） |
| graph 路径 `err` 变量始终 nil | `extension.go:143,169` | 将 `var err` 移入 drafter 分支内，移除图路径的 `if err != nil` 死码 |

### 验证结果
- `go build ./...`：通过 ✅
- `go vet ./domains/claimdrafting/...`：通过 ✅
- `go test ./domains/claimdrafting/...`：20 tests PASS ✅
- `go test ./domains/specdrafting/...`：通过 ✅

## 2026-07-24: 跨案件写作习惯复用 — styles 接线 + memory UserID 修复

### 背景
用户在案件目录内形成的写作习惯受工具沙盒限制，无法跨案件复用。调研发现两条
断路：(1) `~/.mady/styles` 用户自定义风格扩展点 `AddStylePath` 存在但 `cmd/mady`
从未调用，且 `LoadDefaultStyles` 的"先到先得 break"会吞掉用户目录；(2) memory
的 `LayerUser`（跨会话用户偏好层）`UserID` 被错配为 `currentThreadID`，导致偏好
锁死在单会话内，无法跨案件复用。

### 改动清单

| 文件 | 改动 |
|------|------|
| `domains/style_embed.go` | `LoadDefaultStyles` 去掉 break，改为遍历所有路径合并、同名后者覆盖前者；提取可测试纯函数 `loadStylesFromPaths`；解析错误降级为跳过（不阻断有效风格） |
| `domains/style_embed_test.go` | 新增 4 个测试：不同域合并 / 用户覆盖内置 / 坏文件跳过 / 不存在目录跳过 |
| `cmd/mady/framework.go` | `loadManifests` 后接通 `domains.AddStylePath($MADY_HOME/styles)`，让用户风格目录生效 |
| `cmd/mady/tui_session_config.go` | 新增 `stableUserID()`（$MADY_USER_ID > 系统用户名 > "default"），`buildMemoryExtension` 用它替换 `currentThreadID` 作为 UserID |
| `cmd/mady/tui_session_config_test.go` | 新增 2 个测试：env 覆盖优先级 / 回退值非空且稳定 |

### 设计决策
- **styles 合并语义**：内置目录在前、用户目录在后，同名覆盖。用户无需重编译即可在
  `~/.mady/styles/*.yaml` 覆盖内置 patent/legal 风格或定义新域风格。
- **UserID 稳定化**：不破坏 SessionID（仍按会话隔离），仅 LayerUser 用稳定身份。
  旧的以 threadID 为 key 的偏好记录将自然失效（向前不兼容，可接受——新会话正常工作）。
- **不触发安全敏感路径**：改动文件均不在 `scripts/check-sensitive-paths.sh` 清单内。

### 验证结果
- `go build ./...` + `go vet`：通过 ✅
- `go test ./...`（根模块全量）：无 FAIL ✅
- `cd tools && go build ./... && go test ./...`：通过 ✅
- 新增 6 个测试全部通过 ✅

---

## 2026-07-23: 创造性模块优化 — 三期 12 项任务全覆盖（Phase 1-3）

### 背景
基于宝宸知识库 28 篇创造性文档的对比分析报告，对 `domains/inventiveness` 模块实施
3 个阶段共 12 项优化任务，覆盖分析报告识别的全部 8 个缺口（GAP-1~GAP-9）。

### 涉及文件
- `domains/inventiveness/nodes.go` — 提示词增强 + 新增实验数据核验节点
- `domains/inventiveness/types.go` — 新增 `ExperimentalData` 类型
- `domains/inventiveness/graph.go` — 图拓扑扩展（6 节点 → 7 节点）
- `domains/inventiveness/tool.go` — `parseInventivenessArgs` 向后兼容扩展
- `domains/inventiveness/framework.go` — 多源一致性标记参考
- `domains/inventiveness/integration_test.go` — stateKey 测试覆盖扩展
- `domains/inventiveness/inventiveness_test.go` — 新术语验证

### Phase 1: 提示词增强（P0 缺口，零代码架构变更）

| 任务 | 对应缺口 | 改动 |
|------|---------|------|
| 1.1 改进动机结构化 | GAP-1 | 三维度逐维度分析框架（发现难度/结合动机/趋势引导），取代单段文字 |
| 1.2 发明构思因果链 | GAP-2 | 两阶段推理：先构思比对→改进动机，再技术启示判断 |
| 1.3 现有技术质量检查 | GAP-3 | Step1+Step3 追加准确理解指引（整体把握/明显错误/负面描述） |
| 1.4 辅助因素结构化 | GAP-5 | 四类辅助因素（预料不到/商业成功/技术偏见/长期需求）结构化分析框架 |

### Phase 2: 节点拓扑扩展（P1 缺口）

| 任务 | 对应缺口 | 改动 |
|------|---------|------|
| 2.1 技术问题五种情形 | GAP-6 | 补充情形一~四完整框架 + 红线规则（不含手段/保护范围覆盖/回传Step3） |
| 2.2 新增实验数据节点 | GAP-4 | 新 Pregel 节点（Step4→generate_conclusion之间），含 ExperimentalData 类型 |

### Phase 3: 精度精化（P2/P3 缺口）

| 任务 | 对应缺口 | 改动 |
|------|---------|------|
| 3.1 参数特征规则 | GAP-7 | 推定规则：结构/组分决定性能时参数特征不构成区别 |
| 3.2 分析推理细化 | GAP-8 | 三维度综合评估框架（手段/难度/教导）结构化判断指引 |
| 3.3 多源一致性标记 | GAP-9 | 崔国斌《专利法》+尹新天《专利法详解》来源备注 |

### 验证结果
- `make verify`（lint + build + test-race 双模块覆盖）：全部通过 ✅
- 包测试 `go test -race ./domains/inventiveness/...`：全部通过 ✅
- 所有改动向后兼容（新增字段 omitempty，无 ExperimentalData 输入时节点正常跳过）

---

## 2026-07-23: Agent 运行时审阅修复 — 全面修复 56 项发现

### 背景
基于 `docs/reviews/agent-runtime-review-20260723.md` 的审阅结果，对 agentcore 内核实施全面修复。涵盖 13 个工作包、11 个 Git 提交预设，覆盖 56 项全部发现的修复或验证。

### 修复统计
- **P0 (Data Race)**: 2 项 — CompressorEngine/CompactionState 加锁 + atomic.Int64
- **P1 (High)**: 13 项 — 含 MaxTurns 验证、wrapObserver 组合模式、State 深拷贝、inheritRuntime 安全过滤等
- **P2 (Medium)**: 25 项 — 含 iface 适配器、Pipeline 错误路径、EventBus 清理、推理分类器等
- **P3 (Low)**: 16 项 — 含死代码、注释、日志级别、测试加固等

### 新增测试
- 6 个测试文件（新建 4 + 追加 2），共 71 个新测试用例
- 覆盖 agent_run_phase、agent_run_tool、deprecatedHookAdapter、ObserversToHook、ExtensionRegistry、EventBus 深度场景

### 关键改动

**并发安全**
- `context_engine.go`: CompressorEngine.compressionCnt → atomic.Int64
- `compaction.go`: compactionState 新增 sync.Mutex 保护所有字段
- `context_engine_tiered.go`: TieredEngine 自有状态加锁
- `state.go`: messagesNoClone/AddMessage/ReplaceMessages 统一深拷贝 Clone()

**安全加固**
- `handoff.go`: inheritRuntime 新增 isSensitiveTool 过滤，跳过 bash/write/delete/edit/file_/computer 等高危工具

**Observer/扩展**
- `lifecycle.go`: wrapObserver 改用接口断言模式，支持多 Observer 组合
- `extension.go`: Register 失败设置 configErr，防止部分配置运行
- `iface_adapter.go`: AfterModelCall 改用 errors.Join；Resume 添加中断数据告警

**Schema/类型**
- `tool_gen.go`: 整数类型生成 "type": "integer" 而非 "number"
- `tool.go`: Registry.Definitions 在锁外调用 DynamicParameters
- `executor.go`: DualToolOutput 空结果回退 [empty]；Serial 取消设置 Terminate 标记

**状态机**
- `state.go`: SetStatus 添加合法转换校验 + WARN 日志；PendingHandoff 返回值拷贝

**推理系统**
- `reasoning_strategy.go`: StrategyHint 注入使用 Clone() 深拷贝
- `reasoning_router.go`: 关键词预小写、HistoryTurnsForHigh 仅统计 RoleUser、runeLen → utf8.RuneCountInString

**上下文管理**
- `context_engine_tiered.go`: Phase 1 日志提升至 slog.Warn
- `config.go` + `compaction.go`: Compaction 冷却超时可配置化

**Pipeline**
- `pipeline_handler.go`: StageHandler/Atom 交叉验证
- `pipeline_stage_handlers.go`: searchHandler 错误返回统一、extractJSONFromText 深度追踪
- `plugin.go`: 闭包提取 defaultValidateOptions()

**预算/流量**
- `budget.go`: OnExceed 回调 fire-once 防护
- `steering.go`: messageQueue 添加容量上限 + ErrQueueFull

### 验证
- `make verify` (lint + build + test-race, 根模块 + tools 子模块) — ✅ 全部通过
- `go test -race -count=1 ./...` — 全模块 80+ 子包通过，零竞态
- `scripts/check-sensitive-paths.sh` — 通过

## 2026-07-23: Agent 运行时全量系统性审阅

### 背景
对 `agentcore/` 内核目录（~45 源文件，~3500 行）进行全量系统性代码审阅，覆盖六大维度：架构一致性、并发安全、错误处理、代码质量、测试充分性、安全合规。

### 审阅产出
- `docs/reviews/agent-runtime-review-20260723.md` — 完整审阅报告
- 参考 `docs/review/executor-full-review-2026-07-23.md`、`docs/review/event-system-review-2026-07-23.md`

### 发现统计
- **P0 (Critical)** 2 项: CompressorEngine/CompactionState 数据竞争 — 需立即修复
- **P1 (High)** 13 项: MaxTurns off-by-one、wrapObserver 多接口丢失、state.go 深拷贝纪律、inheritRuntime 安全红线等
- **P2 (Medium)** 25 项: 事件完整性 iface 参数丢失、测试缺口等
- **P3 (Low)** 16 项: 死代码、性能微优化等
- **合计**: 56 项发现

### 验证
- `go test -race -count=1 ./agentcore/...` — 通过（3 个预存 pipeline LLM 测试失败非 race 相关）
- `bash scripts/check-sensitive-paths.sh` — 通过

## 2026-07-23: 执行器全量质量审阅与修复

### 背景
对 agentcore 执行器（核心运行循环、工具分发、子模块）进行全量质量审阅，覆盖 10+ 核心模块。发现 4 个 🟠 严重问题、6 个 🟡 建议、6 个 🔵 优化，其中 7 项已修复。

### 审阅产出
- `docs/review/executor-full-review-2026-07-23.md` — 14 页完整审阅报告
- 与已有 `docs/review/event-system-review-2026-07-23.md` 事件系统审阅报告交叉引用

### 改动

**🔴 严重问题修复**
- **串行执行支持 context 取消** (`agentcore/executor.go:executeSerial`): 每个工具调用前检查 `ctx.Done()`，取消时跳过剩余工具调用
- **并行模式 slot 释放加固** (`agentcore/executor.go:executeParallel`): 添加 `acquired` flag，仅在 Acquire 成功后执行 `pool.Release()`，防止未来代码修改引入 leak
- **InvokeTool 配置校验** (`agentcore/agent.go:InvokeTool`): 添加 `configErr` 前置检查，无效配置的 Agent 拒绝执行

**🟡 建议问题修复**
- **EvalHook 无上下文评分** (`knowledge/eval.go`): `scoreFaithfulness` 无上下文时返回 0（先前 0.8），使 EvalConsumer 的低忠实度警告可被触发
- **Config() 值拷贝保护** (`agentcore/agent.go:Config`): 6 个切片字段（Tools/Handoffs/Extensions/Middleware/GlobalBefore/GlobalAfter）执行浅拷贝，防止调用方通过返回的 Config 与 Agent 内部状态竞态

**🔵 优化修复**
- **RateLimitHook 时间戳修剪** (`agentcore/lifecycle.go:BeforeModelCall`): 每轮自动修剪超过 1 分钟的历史时间戳，防止长期运行 Agent 中切片无限增长
- **guardTruncation 空 ID 日志** (`agentcore/agent_run_phase.go`): 截断检测中发现空 TC.ID 时记录 `slog.Debug`

### 验证
- `go test -race -count=1 ./agentcore/...` ✅ 全部通过（9 个子包）
- `go test -race -count=1 ./knowledge/...` ✅ 全部通过（6 个子包）
- `go build ./...` ✅ 通过
- `go vet ./agentcore/... ./knowledge/... ./server/...` ✅ 无警告

## 2026-07-23: 事件系统代码审阅与全部修复

### 背景
对 agentcore 事件系统进行全方位技术审阅，发现 14 个问题（2 🟠 严重、6 🟡 建议、6 🔵 优化），全部修复。

### 改动

**🔴 严重问题修复**
- **A2UIEvent/ApprovalPromptEvent SSE 映射** (`server/stream_events.go`): 新增 `A2UIStreamPayload`/`ApprovalPromptStreamPayload` 结构体及对应的 `agentEventPayload` case 分支，前端 SSE 流不再得不到这两个事件的结构化负载
- **终端事件发射使用 EmitMustDeliver** (`agentcore/agent.go` 新增 `emitMustDeliver` 方法; `agentcore/agent_run.go` 终端 switch + 跟随错误路径; `agentcore/agent_run_phase.go` `failLoop`): 关键终态事件（AgentEnd/AgentError/AgentInterrupt）改用有界阻塞投递，高负载时不再可能被静默丢弃

**🟡 建议问题修复**
- **SkillsReloadedEvent 返回值类型统一** (`agentcore/event_types.go`): 改为返回 `*SkillsReloadedEvent` 指针，消除与所有其他事件构造函数的不一致
- **PublishMustDeliver 超时日志降级** (`agentcore/pubsub.go`): `slog.Error` → `slog.Warn`，高负载时不再日志风暴
- **HandoffEndEvent JSON 序列化缺失字段** (`agentcore/event_types.go`): 自定义 `MarshalJSON`/`UnmarshalJSON` 补上 `Invisible` 字段
- **eval_result 事件类型常量** (`knowledge/eval.go` 新增 `EventTypeEvalResult`; `knowledge/eval_consumer.go` 改用常量): 消除硬编码字符串，消费方通过命名常量引用

**🔵 优化建议修复**
- **PanicCount 监控指标** (`agentcore/event.go`): EventBus 新增 `panicCount` 原子计数器和 `PanicCount()` 方法，`safeCall` 每次捕获 panic 时递增
- **Drain TOCTOU 竞态优化** (`agentcore/event.go`): Drain 的 select 增加 `<-eb.done` 分支，Close 竞态下立即返回而非等待完整 5s 超时

**测试改进**
- 新增 14 个测试用例: handler 注销 (3)、JSON 序列化往返 (4)、DrainTimeout 配置 (2)、DropCount (1)、PanicCount (1)、关闭安全 (2)、MustDeliver 超时 (1)、Broker Subscribe 后 Shutdown (1)

**代码结构重构**
- **agentEventPayload 值/指针双 case → 单 pointer 调度** (`server/stream_events.go`): 消除所有 15 个事件类型的值类型 case 分支。确认所有内置事件通过指针构造函数产生，`skill_extension.go` 中的值类型发射点改用 `NewSkillLoadedEvent` 构造函数（返回指针）。共减少 ~70 行 case 分支代码

### 验证
- `go test -race -count=3 ./agentcore/ -run "TestEventBus|TestA2UI|TestBroker"` 全 3 次 PASS
- `go test -race ./agentcore/ ./server/ ./knowledge/` 全 PASS
- `go build ./...` 全 PASS

## 2026-07-23: enablement 模块知识库联动补强

### 背景
`domains/enablement/` 的 `EnablementInput` 预定义了 `GuidelineRefs` 和 `SimilarCases` 字段但从未填充，
LLM 节点无法利用知识库中的审查指南和类案。需要打通知识库检索链路。

### 改动
- **KnowledgeRetriever 接口** (`domains/enablement/types.go`): 定义 `SearchGuidelines` + `SearchSimilarCases` 两方法
- **EnrichInput** (`domains/enablement/tool.go`): 图执行前自动调用知识检索填充，nil 安全降级
- **renderSimilarCases 提取** (`domains/enablement/nodes.go`): 消除 buildCompletenessInput/buildPFEInput 中的重复渲染代码
- **Step 3 prompt 类案参考** (`domains/enablement/nodes.go`): 在 26.3 核心判断步骤追加类案对比指令
- **serverKnowledgeRetriever** (`server/enablement_events.go`): 组合 LawSearcher + GraphContext 的实现
- **LawSearcher() getter** (`knowledge/extension.go`): 暴露 KnowledgeExtension 的 LawSearcher 供 server 层注入
- **装配链路** (`cmd/mady/server.go`): EnablementTrigger 通过 `WithEnablementKnowledgeRetriever` 接入知识检索
- **测试**: 新增 10 个 KnowledgeRetriever 测试 + 3 个 BuildPFEInput 测试，全部 65 PASS

### 决策
- 接口在 enablement 包定义，实现在 server 层（依赖倒置）
- `NewServerKnowledgeRetriever` 接受 `agentcore.Extension` 接口，避免下游导入 concrete 类型
- `NewEnablementToolFromReport` 暂不接入知识检索（仅在注释标记），纯代码层面无侵入已有调用方

## 2026-07-23: 文档格式转换能力纯 Go 补齐（XLSX 解析 + DOCX/PDF 渲染器）

### 背景
验证 disclosure 文档处理流水线完整性时盘点文件格式转换能力，发现三处缺口：
(1) `knowledge/fileindex` 的 XLSX/XLS 仅占位提示，无真实解析；
(2) `disclosure/export.go` 的 DOCX 导出硬依赖外部 `pandoc` 进程；
(3) `domains/doctmpl` 仅注册 MarkdownRenderer，FormatDOCX/FormatPDF 常量存在但无渲染器实现。
按"纯 Go 补齐全部能力、外部依赖降级为备选+提示"目标统一补齐。

### 改动
- **P1 XLSX 解析** (`knowledge/fileindex/reader_spreadsheet.go`): `.xlsx` 分支改用 `xuri/excelize/v2`（纯 Go）按 sheet 转 markdown 表格，支持多 sheet 标题、空行过滤、千行截断；`.xls`（OLE 二进制老格式）保留降级提示建议转 xlsx/csv。新增测试 `reader_xlsx_test.go`（单 sheet/多 sheet/空表三类）。
- **P2 DOCX 渲染器** (`domains/doctmpl/renderer_docx.go`): 纯标准库 `archive/zip`+手写 OOXML，零外部依赖、零 cgo。支持标题分级、段落合并、无序列表、表格（首行表头加粗+边框）、行内 `**加粗**`/`` `等宽` ``、XML 特殊字符转义、meta.Title 注入。**未用 unioffice（AGPL 传染）**。
- **P3 PDF 渲染器** (`domains/doctmpl/renderer_pdf.go`): `signintech/gopdf`（BSD，纯 Go）。中文支持依赖嵌入系统 TTF/TTC 字体，自动搜索 macOS/Linux/Windows 常见候选（如 `/Library/Fonts/Arial Unicode.ttf`、Noto CJK、msyh.ttc）；找不到中文字体时返回明确错误并建议安装字体或改用 DOCX/Markdown，可经 `PDFRenderer.FontPath` 显式指定。
- **P4 disclosure DOCX 导出去 pandoc** (`disclosure/export.go`): `convertToDOCX` 改为纯 Go（doctmpl.DOCXRenderer）优先，失败降级到 pandoc（若已安装），两者皆不可则返回清晰错误。原 pandoc 逻辑保留为 `convertToDOCXViaPandoc` 备选。
- **P5 注册渲染器** (`domains/doctmpl/store.go`): `NewTemplateStore` 在 MarkdownRenderer 后注册 DOCXRenderer、PDFRenderer。

### 新增依赖
- `github.com/xuri/excelize/v2 v2.11.0`（MIT，纯 Go）
- `github.com/signintech/gopdf v0.37.0`（BSD，纯 Go）
- 项目仍零 cgo，go.mod 极轻量。

### 验证
- `go mod tidy`/`go build ./...`/`go vet ./...`: 全 PASS
- `go test ./knowledge/fileindex/... ./domains/doctmpl/... ./disclosure/... ./workflows/patent/...`: 全绿（XLSX×3、DOCX×4、PDF×4 新增测试 + 原有测试回归通过）

### AI 参与级别
- L2（新增功能模块 + 基础设施层新依赖，无敏感路径改动；disclosure→domains/doctmpl 依赖方向经确认无循环）

---

## 2026-07-23: agentcore 第二轮代码质量审阅修复（8 项）

### 背景
在第一轮 P0-P3 全量修复后，进行第二轮结构级审阅，发现 2 项 P1（语义正确性）、4 项 P2（结构简化）、2 项 P3（可维护性）。

### P1 修复（语义正确性）
- **sortedValidDomains 函数名说谎** (`manifest.go`): 函数名含 "sorted" 但未排序，map 迭代顺序随机导致错误信息不稳定。补 `sort.Strings` 使输出确定。
- **context.Canceled 误设 StatusFinished** (`agent_run.go`): 两处 context.Canceled 路径错误设置 StatusFinished，与同文件 IsInterrupt 路径不一致，导致用户中断被误报为正常完成且无法 Resume。改用 StatusInterrupted，统一终端事件发射到 runLoop。

### P2 结构简化（4 项）
- **deprecatedHookAdapter 冗余锁** (`hooks.go`): 每个 Agent 拥有独立 adapter，不存在跨 Agent 共享，移除 blockedToolsMu 互斥锁及所有 Lock/Unlock 调用。
- **imageTokenEstimate 定义位置错误** (`token.go`/`compaction.go`): token 估算常量原定义在 compaction.go，主要消费者在 token.go。移至 token.go，compaction.go 跨文件引用。
- **executeParallel 冗余 panic recovery** (`executor.go`): Execute 自身已有 defer/recover，executeParallel 内的重复 recovery 永远不会被同一 panic 触发。移除冗余代码。
- **PermissionExtension 字段排列误导** (`permission/extension.go`): mu 字段位于首位暗示保护全部字段，实际仅保护 approver。重排字段并加注释明确保护范围。

### P3 可维护性（2 项）
- **中断路径静默丢弃 endTurn 错误** (`agent_run.go`): context.Canceled 路径的 endTurn 错误改用 slog.Debug 记录，便于运维排查 checkpoint 丢失。
- **runLoop 变量声明风格统一** (`agent_run.go`): finished/err 从循环内声明提升到循环外，与 finalOutput 风格一致。

### 测试更新
- `integration_test.go`: TestCancelDuringToolExecution 和 TestCancelThenRerun_SameAgent 的断言从 StatusFinished 改为 StatusInterrupted，匹配修正后的语义。

### 验证
- `go vet`: PASS | `golangci-lint`: 0 issues | `go test -race`: 全绿 | `go build`: PASS

### AI 参与级别
- L3（核心引擎语义修正 + 结构重构，含敏感路径 hooks.go/executor.go，需人工审阅合入）

---

## 2026-07-23: agentcore 核心引擎深度质量审阅修复（P0-P3 全量）

### 背景
对 agentcore（65 源文件 / ~16.7K 行）进行全量代码审阅，发现 6 项 P0（crash/数据损坏）、10 项 P1（功能 bug/安全风险）、17 项 P2（架构改进）、11 项 P3（技术债务）。本次一次性修复全部发现。

### P0 修复（crash 与数据损坏）
- **MessageBus close-vs-send 竞态** (`orchestrate.go`): Publish 改用 recover 防护 cancel-close 竞态 panic
- **串行 Execute 无 panic recovery** (`executor.go`): 添加 defer/recover 到 Execute 公共方法
- **并行 panic 结果语义错误** (`executor.go`): recover 时正确设置 Err + ToolCallID
- **State.Restore 别名污染** (`state.go`): Restore 深拷贝 messages 防止穿透 checkpoint 存储
- **非结构化压缩摘要迭代失效** (`compaction.go`): else 路径 summaryMsg 套 compactionSummaryPrefix
- **EventBus.On 返回 nil panic** (`event.go`): closed 时返回 no-op func 而非 nil

### P1 修复（功能 bug 与安全）
- **ineffectiveCompactions 永久熔断** (`compaction.go`): 添加 5 分钟时间衰减恢复机制
- **early-exit/handoff 不发 AgentEndEvent** (`agent_run.go`): AgentEndEvent 集中到 runLoop 统一发射
- **context.Canceled 跳过 AfterTurn** (`agent_run.go`): 补全 endTurn 调用维护 Before/After 配对
- **ChunkedEngine.isProtected 误判** (`context_engine_chunked.go`): 移除隐式内容启发式，仅认显式标记
- **intentCache 全局共享** (`handoff_context.go` + `agent.go`): 改为 per-Agent 实例缓存
- **messagesNoClone 竞态** (`state.go`): 返回切片头浅拷贝隔离底层数组
- **deprecatedHookAdapter.blockedTools 无锁** (`hooks.go`): 添加 sync.Mutex 保护
- **IsRetryable 死代码** (`errors.go`): 标记 Deprecated 并指向 IsRetryableError
- **filecheckpoint Restore TOCTOU** (`store.go`): 全程持锁与 RestoreAndTrim 一致
- **inheritRuntime 全信任转移** (`handoff.go`): 添加运行时安全审计日志

### P2 架构改进（17 项）
- UpdateFromResponse 实落地（`context_engine.go`）、Cache 文档修正（`cache/doc.go`）
- iface 抽象层限制文档化（`iface/doc.go`）、Budget 预检查（`budget.go`）
- 英文关键词过滤收紧（`reasoning_router.go`）、Message.Clone 深拷贝 CacheControl（`message.go`）
- 图片 token 度量统一（`token.go`）、concurrency/evidence/filecheckpoint doc 修正
- permission SetApprover 加锁（`permission/extension.go`）、Drain 超时可配（`event.go`）
- Pipeline FailOnHandlerError 开关（`pipeline_executor.go`）、插件冲突告警（`pluginsys/loader.go`）
- 深度偏移修正（`orchestrate.go`）、snapshot 失败日志（`filecheckpoint/extension.go`）

### P3 技术债务（11 项）
- 策略提示词无 system 时兜底注入、budget Duration 死代码清理
- Manifest 错误信息动态化、PluginManager.Plugins() 返回拷贝
- processed map 契约文档化、evidence/context.go 死代码移除

### 涉及敏感路径
- `agentcore/handoff.go`（inheritRuntime 安全审计日志）
- `agentcore/hooks.go`（deprecatedHookAdapter 并发锁）
- `agentcore/executor.go`（panic recovery）
- `agentcore/state.go`（Restore 深拷贝）
- `agentcore/compaction.go`（压缩摘要 prefix + 熔断恢复）

### 验证
- `go vet`: PASS | `golangci-lint`: 0 issues | `go test -race`: 全绿
- `go build ./...`: PASS（根模块 + tools 子模块）

### AI 参与级别
- L3（核心引擎多文件修复，含并发安全/错误处理/状态管理变更，需人工审阅合入）

---

## 2026-07-23: 删除 evidence 包悬空测试 TestContext_RoundTrip

### 背景
`go vet ./...` 失败：`agentcore/evidence/ledger_test.go` 的 `TestContext_RoundTrip` 引用了 `WithLedger`/`FromContext`，但这两个函数已在 `context.go` 重构中被主动删除（零调用死代码，EvidenceExtension 通过私有字段直接访问 ledger）。测试未同步删除，导致该包测试编译失败（`go test ./agentcore/evidence/...` build failed），CI/vet 拦截。深度评审 M15（`docs/review/agentcore-deep-review-2026-07-20.md`）已记录此事。

### 变更内容
- `agentcore/evidence/ledger_test.go`：删除悬空的 `TestContext_RoundTrip` 测试函数及其 `context` 导入（被测功能已不存在）

### 涉及敏感路径
- 无（仅删除遗留测试，不触碰 `agentcore/evidence` 生产代码或任何敏感路径）

### AI 参与级别
- L1（测试清理，无逻辑变更）

---

## 2026-07-23: 修复 TieredEngine 上下文压缩管线三个盲区

### 背景
用户在 MD→DOCX 转换时遇到 `3,171,658 tokens` vs `1,048,565 tokens` 上限的 400 错误。
审查上下文管理代码后发现三个结构性缺陷：
1. Snip/Prune 只处理 `RoleTool` 消息，大的 user/assistant 消息漏网
2. Force-fold 的摘要请求本身在上下文 3x 溢出时也会超出模型窗口
3. CJK token 估算偏低（chars/4 对中文只分配 ~0.75 tokens/字），ShouldCompact 不及时触发

### 变更内容
- `agentcore/token.go`：`EstimateTokens` 新增 CJK 校正（+0.75 tokens/CJK 字符），新增 `countCJKRunes`/`isCJK`（含 ASCII 快速路径）
- `agentcore/context_engine_tiered.go`：将 `snipToolResults` + `snipLargeMessages` 统一为单一 `snipMessages` 方法（复用 `SnipMessageContent`，单次 deepCopy），通过 `snipThresholdsForRole` 按角色分配 head/tail 预算
- `agentcore/compaction.go`：`CompactionParams` 新增 `ContextWindow` 字段，`runCompaction` 在构建摘要 prompt 前预检截断（唯一截断防线）；提取 `truncateToTokenBudget` 共享辅助函数
- `agentcore/context_engine.go`：构造 `CompactionParams` 时传递 `ContextWindow`
- 自适应 token→rune 转换：`truncateToTokenBudget` 根据每条消息的实际 token 密度计算截断长度，避免对 ASCII 过截断或对 CJK 欠截断

### 涉及敏感路径
- 无（仅修改上下文压缩引擎内部逻辑，不触碰 handoff/guardrails/permission/sandbox/auth）

### AI 参与级别
- L2（bug fix，需人工审阅压缩逻辑正确性）

---

## 2026-07-23: Agent 三合一 — Chat + Assistant + Router → UnifiedAgent

### 背景
原架构中 Chat Agent（对话/情感陪伴）、Assistant Agent（工具执行）和 Router（领域路由）
三者分离，集成模式下用户消息经过 Chat → transfer_to_assistant → 子 Agent 执行 →
返回 HandoffResult → Chat 解释结果，产生 3 倍延迟和上下文序列化损耗。

### 变更内容
- 新增 `UnifiedAgentConfig`（`domains/unified.go`），融合三者能力
- 删除运行时模式切换：移除 `MADY_SINGLE_AGENT`/`MADY_ROUTER_MODE` 环境变量
- `buildAgentConfig`（`cmd/mady/tui_session_config.go`）从三分支 switch 简化为单一路径
- `ProfessionalHandoffConfigs` 移除 assistant 目标，仅保留 patent/legal
- `domainFactoryMap` 的 chat/assistant 统一映射到 `UnifiedAgentConfig`
- 护栏等级统一为 `LevelLight`
- `domains/graph.go` 简化为 3 节点路由图（unified/patent/legal）
- 删除 `RouterConfig`/`RouterStep`/`RouterConfigWithRegistry` 等 6 个 Router 函数
- 删除 `IntegratedChatConfig`（已被 UnifiedAgentConfig 替代）

### 涉及敏感路径
- `domains/router.go`（AllowedSources 白名单新增 `"mady-agent"`）
- 护栏降级为 `LevelLight`（用户明确要求，未来通过人机协作/plan 模式替代）

### AI 参与级别
- L3（架构级重构，需人工审阅）

---

## 2026-07-23: 文档同步 — AGENTS.md / CLAUDE.md 与代码实际状态对齐

### 背景
上一轮文档同步（`3206acd`）后，25 个功能提交带来了 335 个文件变更（+43K 行），
包括 6 个全新专利分析子模块、编排内核硬化、CWD 即工作区行为变更等。
AGENTS.md 与 CLAUDE.md 的目录结构、代码统计和资源定位描述已严重滞后。

### 变更内容

**代码统计更新**
- AGENTS.md / CLAUDE.md：`940 源文件（630+310）/ ~207K 行` → `1081 源文件（732+349）/ ~240K 行`

**CLAUDE.md 目录结构修正**
- `agentcore/` 子目录：移除错误归属的 `doomloop/`、`evaluate/`、`tracing/`（实际为顶层独立包），
  新增 `iface/`（接口抽象层）
- `domains/` 子目录：新增 `claimdrafting/`（权利要求撰写）、`specdrafting/`（说明书撰写）、
  `enablement/`（26.3 充分公开判断）、`evidence/`（证据规则引擎）、`domainconfig/`（统一领域配置），
  更新 `reasoning/`（拓扑驱动泛化）、`sqlite/`（新增 case_index）
- 顶层目录：新增 `doomloop/`、`evaluate/`（含 benchmark/calibrate/cli）、`tracing/`
- `graph/`：补充 StateSchema/Reducer、NodePolicy、DegradationMark
- `memory/compiler/`：补充时间衰减置信度、质量加权、持久化
- `workflows/patent/`：补充无效宣告/侵权比对/复审请求
- `pkg/`：新增 `i18n/`（zh-CN/en-US），`util/` 补充沙箱配置
- `cmd/mady/`：新增 `trust-knowledge` 子命令
- `tools/` 源文件统计：`~85 源文件` → `65 源 + 20 测试`

**AGENTS.md 资源定位补充**
- 新增"CWD 即工作区"段落：`detectCaseFromCWD` 自动创建瞬态项目上下文，
  `applyPersistence` 移除案件关联门闩（cd3ae7d 行为变更）

**AGENTS.md 项目概览**
- 领域扩展层补充新增的 5 个子模块名称

### 涉及文件（3 个）

| 文件 | 变更类型 |
|------|---------|
| `AGENTS.md` | 修改：代码统计 + 资源定位补充 + 项目概览 |
| `Claude.md` | 修改：目录结构全面修正（agentcore/domains/顶层/pkg/cmd）+ 代码统计 |
| `docs/decisions/AI_CHANGELOG.md` | 本条记录 |

### 说明
敏感路径表与 `scripts/check-sensitive-paths.sh` 的 `SENSITIVE_PATHS` / `SENSITIVE_PATH_PREFIXES` 数组对比一致，无需更新。

## 2026-07-23: 四大功能模块断链修复 — inventiveness/claimdrafting/specdrafting 接线

### 背景
工作流系统优化后，四个核心专利功能模块（可实现性/创造性/权利要求书/说明书撰写）的图引擎和规则引擎代码已完整实现且单元测试全过，但**装配层（wiring）存在断链**——部分模块的工具/扩展从未注册到运行时 Agent，沦为死代码。

### 变更内容

**断链 1：inventiveness 工具未注册** — `domains/patent.go`（敏感路径）
- `PatentAgentConfig.ExtraTools` 漏注册 `NewInventivenessTool`，仅 `enablement` 被注册
- 修复：添加 `inventiveness.NewInventivenessTool(WithProvider(base.Provider))`，与 enablement 对称
- 影响：TUI 模式下 `evaluate_inventiveness` 工具此前完全不可达（Server 模式通过事件触发器可用）

**断链 2：claimdrafting 完全未接线（三重断链修复）**
- `domains/claimdrafting/provider_adapter.go`（**新增**）：`ProviderAdapter` 将 `agentcore.Provider` 适配为 `claimdrafting.Provider`（`Complete(prompt)→string`）
- `domains/claimdrafting/extension.go`（修改）：修复 `handleDraftClaims` 死字段——原代码直接 `NewClaimBuilder` 绕过 `e.drafter`，现改为优先使用 drafter（LLM 增强）并降级到 builder；新增 `Drafter()` getter
- `domains/patent.go`（修改）：新增 `globalClaimDraftingExt` + `SetupClaimDraftingExtension()`，注入到 `PatentAgentConfig` 和 `BuildProjectAgent`

**断链 3：specdrafting 完全未接线 + 替换旧版**
- `domains/patent.go`（修改）：新增 `globalSpecDraftingExt` + `SetupSpecDraftingExtension()`，注入到 `PatentAgentConfig` 和 `BuildProjectAgent`
- 移除 `patent.NewSpecificationTool()`（workflows/patent 简单版），由 `specdrafting.Extension`（12 节点 Pregel 图 + 16 条规则 + 评分器）替代
- `cmd/mady/framework.go`（修改）：启动期调用 `SetupClaimDraftingExtension` + `SetupSpecDraftingExtension`

### 断链根因
四模块均通过全局注入模式（`globalXxx` + `Setup*`）装配，与 `globalKnowledgeExt`/`globalDraftingRunner` 一致。但三个模块的 Setup 函数和注入逻辑在上一批优化中遗漏编写，导致代码虽完整却不可达。

### 测试
- `go build ./...` + `go build ./cmd/mady/...` 全部通过
- `go test ./domains/...` 全部通过（15 个包 0 失败）
- `go vet` + `gofmt` 干净

### 涉及文件（5 个）

| 文件 | 变更类型 |
|------|---------|
| `domains/claimdrafting/provider_adapter.go` | **新增**：agentcore.Provider 适配器 |
| `domains/claimdrafting/extension.go` | 修改：修复 drafter 死字段 + 新增 Drafter() getter |
| `domains/patent.go` | 修改：注册 inventiveness 工具 + 两个 extension 注入 + Setup 函数（敏感路径） |
| `cmd/mady/framework.go` | 修改：启动期调用两个 Setup 函数 |
| `docs/decisions/AI_CHANGELOG.md` | 本条记录 |

### 残留事项
~~`claimdrafting.LLMDrafter.DraftFromScratch` 的 LLM 结果解析为 `TODO`（drafter.go:55 `_ = result`），当前 LLM 路径仍降级到 builder。接线已完成，LLM 解析待后续实现。~~ **已实现**（见后续条目）

## 2026-07-23: claimdrafting LLM 结果解析实现

### 变更内容

**`domains/claimdrafting/drafter.go`**（重写）
- 实现 `parseClaimsFromLLM()` 解析器：将 LLM 生成的自由文本权利要求解析为结构化 `Claim` 对象
- 支持独立权利要求（"其特征在于"两段式）、从属权利要求（"根据权利要求"引用式）、多项从属（"或"连接多引用）
- 3 个解析测试 + 5 个异常格式降级测试全部通过
- `DraftFromScratch` 流程重构：builder 降级输出 → LLM 调用 → 解析成功则返回结构化结果 → 解析失败静默降级 builder
- prompt 增加技术领域提示 + 输出格式示例（约束 LLM 输出格式便于解析）
- `TechDomain` 现在正确传递：`handleDraftClaims` 为 `DraftInput.TechDomain` 设值，prompt 中包含技术领域提示

**测试**（`domains/claimdrafting/integration_test.go`）：
- `TestParseClaimsFromLLM_Basic` — 产品+方法独立权利要求 + 从属 + 多项从属全链路解析
- `TestParseClaimsFromLLM_Malformed` — 5 种异常格式识别
- `TestParseClaimsFromLLM_DomainCarryOver` — TechDomain 传递验证

### 涉及文件（4 个）

| 文件 | 变更类型 |
|------|---------|
| `domains/claimdrafting/drafter.go` | 重写：实现 LLM 结果解析 + 重构 DraftFromScratch 流程 |
| `domains/claimdrafting/extension.go` | 修改：handleDraftClaims 传递 TechDomain 给 DraftInput |
| `domains/claimdrafting/integration_test.go` | 修改：新增 3 个解析器测试 |
| `docs/decisions/AI_CHANGELOG.md` | 本条记录 |

## 2026-07-22: 工作流编排器优化第三批 — 工具注册 + CLI/TUI 入口 + 复审请求工作流

### 背景
第二批已完成无效宣告和侵权比对 Pregel 工作流的实现和测试，但缺口分析发现：
1. 新工具未注册到任何 Agent（完全不可达）
2. 无 CLI 子命令和 TUI 斜杠命令
3. 复审请求工作流仅有规则定义，无 Pregel 实现

本批完成全链路打通 + 复审工作流实现。

### 变更内容

**工具注册** — `domains/patent.go`（敏感路径）
- `PatentAgentConfig` 的 `ExtraTools` 注册 `NewInvalidationTool`、`NewInfringementTool`、`NewReexaminationTool`
- 三个新工具现可通过 Patent Agent 在 TUI/Server 模式下被 LLM 自动调用

**CLI 子命令** — `cmd/mady/patent.go`
- 新增 `mady patent invalidation` — 无效宣告分析
- 新增 `mady patent infringement` — 侵权比对分析（双参数）
- 新增 `mady patent reexamination` — 复审请求书起草

**TUI 斜杠命令** — `cmd/mady/slash_registry.go` + `cmd/mady/tui_session.go`
- 新增 `/invalidation <权利要求>` — 直接运行无效宣告 Pregel 图
- 新增 `/infringement <权利要求> | <被控方案>` — 侵权比对（管道符分隔双参数）
- 新增 `/reexamination <驳回决定书>` — 复审请求书起草
- `/patent` 帮助命令更新为列出全部 5 个专利快捷命令

**复审请求工作流** — `workflows/patent/reexamination.go`（新增，~380 行）
- Pregel 5 节点图：`parse_decision → classify_grounds → draft_request → rule_check → conclude → __end__`
- 驳回决定解析器：自动提取文号/日期/申请人/申请号/对比文件
- 6 种驳回理由识别：新颖性/创造性/充分公开/权利要求清楚/修改超范围/实用新型客体
- **实用新型特化**：自动检测专利类型，实用新型过滤创造性驳回理由
- 法律依据：专利法第41条 + 3 个月期限提醒
- 规则引擎集成：`ReexaminationRules()` + domain `"patent_reexamination"`
- `NewReexaminationTool` → `draft_reexamination_request` 工具

### 测试
- `workflows/patent/reexamination_test.go`：12 个测试用例（空输入/决定解析/理由识别/客体识别/类型检测/实用新型过滤/骨架起草/规则引擎/发明端到端/实用新型端到端）
- `go test -race -count=1 ./workflows/patent/... ./domains/... ./cmd/mady/...` 全部通过
- 全部 17 个包 0 失败

### 涉及文件（8 个）

| 文件 | 变更类型 |
|------|---------|
| `domains/patent.go` | 修改：注册 3 个新工具（敏感路径） |
| `cmd/mady/patent.go` | 修改：新增 3 个 CLI 子命令 |
| `cmd/mady/slash_registry.go` | 修改：注册 3 个斜杠命令 |
| `cmd/mady/tui_session.go` | 修改：新增 3 个 slash handler |
| `workflows/patent/reexamination.go` | **新增**：复审请求 Pregel 工作流 |
| `workflows/patent/reexamination_test.go` | **新增**：12 个测试 |
| `workflows/patent/tool.go` | 修改：新增 `NewReexaminationTool` |
| `docs/decisions/AI_CHANGELOG.md` | 本条记录 |

## 2026-07-22: 工作流编排器优化第二批 — 无效宣告 + 侵权比对 Pregel 工作流

### 背景
第一批完成了规则引擎场景化 + 知识系统深度集成（4 项），但专利全生命周期中两个核心领域工作流仍然缺失：
**无效宣告分析**和**侵权比对分析**。本次实施第二批（P0 优先级），补全这两个关键环节。

### 变更内容

**P0-1 无效宣告分析工作流** — `workflows/patent/invalidation.go`（新增）
- Pregel 6 节点图：`parse_patent → identify_grounds → [gather_evidence] → analyze_grounds → conclude → __end__`
- 5 种无效理由自动识别：A22.2 新颖性 / A22.3 创造性 / A26.3 充分公开 / A26.4 清楚支持 / A33 修改超范围
- 权利要求解析器：自动识别独立/从属权利要求、编号和类型
- `InvGraphOption` + `WithInvRetriever()` functional option，条件插入证据检索节点
- 规则引擎集成：使用 `InvalidationRules()` + domain `"patent_invalidation"` 校验分析完整性
- 逐理由独立分析框架（单独对比 / 三步法 / 充分公开 / 清楚支持 / 修改超范围），每项均有法律依据和要点提示

**P0-2 侵权比对分析工作流** — `workflows/patent/infringement.go`（新增）
- Pregel 6 节点图：`parse_claims → parse_product → full_coverage → equivalence → rule_check → conclude → __end__`
- 全面覆盖分析：技术特征分解 + 逐特征比对 + 字面侵权判定
- 等同侵权分析：手段/功能/效果三要素框架 + 禁止反悔/捐献规则限制提示
- 规则引擎集成：使用 `InfringementRules()` + domain `"patent_invalidation"` 校验分析完整性
- `InfGraphOption` functional option（预留扩展位）

**工具注册** — `workflows/patent/tool.go`
- `NewInvalidationTool(opts ...InvGraphOption)` → `analyze_patent_invalidation` 工具
- `NewInfringementTool(opts ...InfGraphOption)` → `analyze_patent_infringement` 工具

### 测试
- `workflows/patent/invalidation_test.go`：14 个测试用例（空输入/权利要求解析/理由识别/证据降级/检索注入/分析框架/端到端/截断辅助函数）
- `workflows/patent/infringement_test.go`：15 个测试用例（空输入/特征提取/全面覆盖全匹配-部分匹配/等同分析/规则引擎/端到端全匹配-部分匹配/辅助函数）
- `go test -race ./workflows/patent/...` 全部通过，无回归

### 设计决策
- **与第一批保持一致**：functional option DI + nil-safe 降级 + 条件节点插入 + 规则引擎校验
- **确定性骨架**：所有节点不调用 LLM，生成结构化分析骨架供代理师/律师审阅和填充
- **技术特征匹配**：采用 naive 子串 + 3-rune 重叠匹配，适合骨架分析阶段；真实侵权比对仍需人工判断
- **禁止反悔/捐献规则**：在等同分析中以提示性文案呈现，不做自动化判断（需审查历史档案）

## 2026-07-22: 工作流编排器优化第一批 — 规则引擎场景化 + 知识系统深度集成

### 背景
对照专利全生命周期评估工作流覆盖度后，识别出 9 项优化空间。本次实施第一批 4 个无依赖项，
聚焦规则引擎场景化和知识系统（风险扫描/判例检索/法条动态检索）与 Pregel 工作流的深度集成。

### 变更内容

**P2-8 规则引擎场景化** — `workflows/patent/rule_engine.go`
- 将单一的 `DefaultPatentRules()` 拆分为 6 个场景规则集：
  `NoveltyRules()` / `InventivenessRules()` / `InfringementRules()` /
  `InvalidationRules()` / `ReexaminationRules()` / `DisclosureRules()`
- 新增侵权规则（等同原则/禁止反悔/捐献规则）、无效规则（组合动机论证/公开日核实）、
  复审规则（复审理由范围/新证据关联性）
- 扩展 `synonymMap` 新增侵权/无效/复审领域术语
- `DefaultPatentRules()` 改为聚合上述 6 个场景集的并集（向后兼容）

**P1-3 风险扫描器集成** — `workflows/patent/analysis.go`
- 新增 `FeatureRiskScanner` 接口和 `WithRiskScanner()` functional option
- 新增 `risk_scan` Pregel 节点，在 `rule_check` 和 `conclude` 之间条件插入
- `concludeWithRulesNode` 增强：将风险报告嵌入最终分析报告
- 向后兼容：无 scanner 注入时管线行为与原有完全一致

**P1-4 判例检索接入知识库** — `workflows/legal/comparison.go`
- 新增 `CaseSearcher` 接口和 `WithCaseSearcher()` functional option
- `caseSearchNode` 从 `DegradationNotImplemented` 升级为工厂模式 `newCaseSearchNode(searcher)`
- `BuildComparisonGraph` / `BuildComparisonGraphWithReasoning` 均支持 opts 注入
- 向后兼容：无 searcher 时保持原有降级行为

**P2-9 OA 法条动态检索** — `workflows/patent/oa_response.go`
- 新增 `OARuleRetriever` 接口和 `WithOARuleRetriever()` functional option
- 新增 `rule_retrieval` Pregel 节点，在 `classify_rejection` 之后条件插入
- `draftResponseNode` 增强：动态法条嵌入答复书的"适用法条"章节
- 新增 `rejectionTypeToQuery` 映射函数（驳回类型→检索查询）
- 向后兼容：无 retriever 时使用硬编码模板法条

### 测试
- 全部新增测试通过（含 race 检测）：`go test -race ./workflows/patent/... ./workflows/legal/...`
- 向后兼容测试：无注入时与原有行为完全一致
- 新增测试覆盖：nil 注入降级、正常注入、错误降级、端到端管线 4 个场景 × 4 项

## 2026-07-22: 专利法第26.3条判断模块全面增强 — 对标 Wiki 知识库补全 10 项差距

### 背景
对照宝宸知识库 Wiki 中关于专利法第26.3条（充分公开）的全部规定（审查指南 + 司法解释 +
各技术领域特殊规则 + 大量案例），项目现有 `domains/enablement/` 模块存在 10 项差距。
本次一次性实施 P0/P1/P2 全部优先级修复，将 26.3 判断模块从"基础三步法"升级为
"领域自适应 + 六种情形 + 完整规则体系"。

### 变更内容

**P0 — 修补审查指南硬性规定缺口**

- **差距1（六种情形）**：`types.go` `EnablementJudgment` 新增 `MeansCannotSolve`（技术手段
  不能解决技术问题）和 `PartialMeansUnreal`（多手段方案某一手段不能实现）两个标志；
  `nodes.go` step3 prompt 从 4 种情形扩展为 6 种，并明确标注「能够实现 = 实现方案 +
  解决问题 + 产生效果 三者同时满足」；`patent-core.yaml` 判定规则从 4 条增至 6 条；
  `framework.go` 默认框架同步更新
- **差距3（司法解释）**：`patent-law-a26.3.yaml` 新增 `judicialInterpretations` 段，
  纳入授权确权规定2020第六条（公开不充分三种情形 + 26.3→26.4联动）、第九条（功能性特征）、
  第十条（补充实验数据）
- **差距5（清楚性增强）**：`types.go` `ClarityResult` 新增 `CoinedTerms`（自造词）和
  `ObviousErrors`（明显错误）字段；`nodes.go` step2 prompt 增加自造词检测和
  明显错误识别（唯一正确理解→不影响；多种解释→不符合26.3）
- **差距10（案例补充）**：`enablement_cases.json` 新增 5 个 fixture（手段不能解决/部分不能实现/
  自造词/化学马库什/生物保藏）；benchmark 新增 2 个评测用例（飞行汽车/多功能手环）

**P1 — 领域规则与规则体系增强**

- **差距2a/2b/2c/2d（领域规则）**：新增 `domain_rules.go`（210行），实现：
  - 领域自动检测（`DetectDomain`）：基于关键词匹配识别 chemical/biotech/tcm/computer/mechanical/electronic
  - 化学三要素规则：确认/制备/用途 + 马库什权利要求 + 第二医药用途 + 补充实验数据
  - 生物保藏制度：两个条件判定 + 序列表要求
  - 中药领域规则：正名要求 + 配比记载 + 可预测性判断
  - 计算机领域规则：流程图要求 + AI参数关联（2023修订）
  - 机械/电学领域规则：可预见性 + 附图可实施性
- **差距7（实验数据体系）**：`types.go` 新增 `ExperimentDataAssessment` 结构
  （四要素：样品/方法/结果/对应关系），结论节点输出实验数据有效性评估
- **差距4（判断标准细化）**：step3 prompt 增加 4 项细化标准——技术问题认定规则、
  无需过度实验标准（In re Wands 8因素）、明显夸大效果处理；step1 prompt 增加完整性三层要求
  和「完整 ≠ 面面俱到」原则
- **差距8（26.3/26.4联动）**：`EnablementResult` 新增 `SupportIssue` 和 `SupportWarnings` 字段；
  结论节点 prompt 增加联动评估指令

**P2 — 远期演进**

- **差距6（完整性内容质量）**：step1 prompt 从"章节存在性检查"升级为"内容充分性评估"
- **差距9（26.3/22.4界限）**：结论节点 prompt 和 SKILL.md 明确区分公开不充分与实用性
- **SKILL.md 全面重写**：判断标准从 4 项扩展为 8 项，新增实验数据规则、26.3/26.4区分、
  26.3/22.4界限、功能性描述边界等完整内容

### 涉及文件（11 个源文件 + 2 个数据文件 + 1 个技能文件 + 1 个文档）

| 文件 | 变更类型 |
|------|---------|
| `domains/enablement/types.go` | 修改：新增 5 个字段/结构 |
| `domains/enablement/nodes.go` | 修改：全步骤 prompt 重写 + 新字段解析 |
| `domains/enablement/graph.go` | 修改：新增 stateKeyDomain |
| `domains/enablement/framework.go` | 修改：默认框架同步 |
| `domains/enablement/domain_rules.go` | **新增**：领域检测 + 各领域规则文本 |
| `domains/enablement/enablement_test.go` | 修改：新增 15+ 测试用例 |
| `domains/enablement/testdata/enablement_cases.json` | 修改：新增 5 个 fixture |
| `domains/rules/data/articles/patent-law-a26.3.yaml` | 修改：6 种情形 + 司法解释 |
| `domains/rules/data/rules/patent-core.yaml` | 修改：判定规则 4→6 条 |
| `evaluate/benchmark/patent_exam_real_a26_3.go` | 修改：新增 2 个评测用例 |
| `skills/enablement/SKILL.md` | 修改：全面重写判断标准与规则 |
| `docs/decisions/AI_CHANGELOG.md` | 本条记录 |

### 验证
- `go build ./domains/enablement/... ./evaluate/... ./domains/rules/... ./server/...` 通过
- `go vet` 同上范围通过
- `go test ./domains/enablement/...` — 45+ 测试用例全部通过（含 15 个新增测试）
- `go test ./domains/rules/...` 通过
- `go test ./evaluate/...` 通过（benchmark 的 `TestEvalSuite_GoldenPerfect` 为改动前已有失败）

### 设计决策
- **领域检测策略**：采用关键词匹配而非 IPC 分类码，因为 disclosure 管线提取的 Features/Problems
  天然包含领域关键词，无需额外标注步骤。优先级 chemical > biotech > tcm > computer > electronic > mechanical
- **领域规则注入方式**：通过 prompt 追加而非硬编码判断，保持 LLM Agent 的灵活性，
  同时确保领域特殊规则被纳入考量
- **六种情形 vs 五种**：审查指南原文列举 5 种"无法实现"情形，项目中 `insufficient_data`
  对应第5种且语义清晰，故保留为独立标志。新增加的 `means_cannot_solve` 和 `partial_means_unrealizable`
  补全第3、4种情形，总计 6 个标志覆盖审查指南全部情形

## 2026-07-22: 实用新型/发明驳回复审请求工作流 — 产出实现规划文档（Spec 阶段）

### 背景
项目中专利文书自动化仅覆盖「审查意见答复（OA Response）」（`workflows/patent/oa_response.go`），
未覆盖「驳回复审请求」这一独立法律程序（审查员作出驳回决定后向复审和无效审理部门提复审）。
全仓无任何复审请求的可执行工作流。经确认用户场景为「正式的驳回复审请求书」后，
按 Spec-Driven 流程先行产出规划文档。

### 变更内容
- **新增** `docs/specs/reexamination-request/01-proposal.md`：完整提案文档，含背景、业务差异
  （复审请求 ≠ OA 答复）、目标、成功标准、设计方案（数据模型 / Pregel 图 / 工具签名 / 文档模板）、
  实用新型特化考量、文件清单、决策摘要、风险
- **更新** `docs/specs/README.md` Spec 索引表，追加复审请求工作流条目

## 2026-07-22: 知识管理系统审阅优化 — BuildProjectAgent 注入 + 测试补充

### 背景
全面审阅知识管理系统与智能体的集成链路，发现：
1. `BuildProjectAgent` 未注入 `globalKnowledgeExt`，导致项目级 Agent 缺少 `search_knowledge`/`search_laws`/`add_document` 工具
2. 重排器（Reranker）5 种实现零单元测试
3. `KnowledgeExtension` 核心入口缺少集成测试

### 变更内容
- **修复** `domains/patent.go` `BuildProjectAgent()`：添加 `globalKnowledgeExt` 注入（241-246 行），使项目级 Agent 获得知识检索工具
- **新增** `retrieval/rerank_test.go`：16 个测试用例覆盖 PositionReranker（空输入/位置加分/零权重/重排序）、DeduplicatingReranker（空/单条/去重/长签名）、ChainReranker（空链/顺序执行）、LegalReranker（空/权威提升/自定义层级/未知来源）、PatentReranker（空/权威提升/未来日期抑制/自定义等级/未知类型）
- **新增** `knowledge/extension_test.go`：20 个测试用例覆盖 Search 多路径（有后端/嵌入器/可写库/图谱/重排器/空查询/无结果）、BackendHook 图增强注入、工具暴露（search_laws/add_document/all）

### 验证
- `go build ./...` 通过
- `go vet ./knowledge/... ./retrieval/... ./memory/... ./domains/...` 通过
- `go test ./knowledge/... ./retrieval/... ./memory/...` — 12 个测试包、全部通过
- 完整编译无新诊断

### 设计决策
- **图引擎选型**：复用现有 Pregel（`graph` 包），图结构 `parse_decision → classify_grounds →
  analyze_claims → draft_request → [llm_enhance] → approval_gate`，与 OA 工作流同范式
- **实用新型特化**：实用新型初步审查不审查创造性，且高频驳回理由为「实用新型客体」
  （专利法第 2 条第 3 款），故独立设计 `um-subject-defense.md` 模板与 `GroundUMSubject` 类型分支，
  理由枚举不含 `inventiveness`
- **复用已有资产**：`domains/case_index.go` 的 `DocRejection`/`CaseStatusRejected`、
  `domains/legal_intent.go` 的 `CaseReexamination` 意图识别、OA 工作流的工具封装模式
- **任务粒度**：阶段 1+2 控制在 5 个文件内（2 新增 Go 文件 + 2 新增模板 + 1 行注册），符合
  AGENTS.md「单次改动 3-5 文件」约定；`domains/patent.go` 属敏感路径，注册改动需 L3 审阅

### 待确认项（[NEEDS CLARIFICATION]）
- Human Owner 待指派
- `domains/deadline_calculator.go` 是否已含复审 3 个月期限规则，影响阶段 4 案件闭环

### 验证结果
- 本条为 Spec 阶段文档产出，未涉及代码改动，无需构建/测试验证
- 文档结构与现有 `docs/specs/vector-retrieval/01-proposal.md` 格式对齐

---

## 2026-07-22: 知识库工具注入修正 — search_knowledge/search_laws 归属 Patent/Legal Agent

### 背景
知识库（9121 部法律法规 + 专利法条 + 判例 + 审查指南）的 `search_knowledge` / `search_laws` 工具
此前仅通过会话级 `extendConfig()` 注入到顶层 Chat Agent。HandoffDelegate 创建的子 Agent
（patent-agent、legal-advisor）不继承父 Agent 扩展，导致这些专业 Agent 虽然在 System Prompt
中声明可用 `search_knowledge / search_laws`，实际工具注册表中并不存在——LLM 要么调用
不存在的工具（报错），要么凭训练数据编造法条（幻觉）。

### 变更内容

**domains/patent.go**
- 新增 `globalKnowledgeExt` 全局变量 + `SetupKnowledgeExtension()` 注入函数（遵循 `globalDraftingRunner` / `globalPatentRetriever` 同一模式）
- `PatentAgentConfig` 中在 `injectDocTemplateTools` 之后追加 `globalKnowledgeExt` 到 `cfg.Extensions`

**domains/legal.go**
- `LegalAgentConfig` 中同样追加 `globalKnowledgeExt` 到 `cfg.Extensions`

**domains/assistant.go**
- 从 System Prompt 移除 `search_knowledge / search_laws` 声明（assistant-agent 不拥有知识库工具，法律/专利知识检索属于 patent-agent / legal-advisor 职责）

**cmd/mady/framework.go**
- `initReasoningAndTemplates` 中调用 `domains.SetupKnowledgeExtension(fc.KnowledgeExt)`，在 `wikistore` 延迟任务完成后注入

### 设计决策
- 采用全局注入模式而非函数签名变更：`PatentAgentConfig` 签名受 `domainFactoryMap` 约束（`func(agentcore.Config) agentcore.Config`），添加参数需要改动 domainFactoryMap 及所有调用方
- `KnowledgeExtension` 的 `Init()` / `Dispose()` 均为空操作，同一实例安全共享于多个 Agent
- 暂未从 `extendConfig` 移除 Chat Agent 的 KnowledgeExt 注入：单 Agent 模式（`MADY_SINGLE_AGENT=1`）仍依赖此路径；集成模式下 Chat Agent 拥有该工具不影响正确性（System Prompt 已指示路由到专业 Agent）

### 验证结果
- `go build ./...` — 通过
- `go test ./domains/...` — 通过



## 2026-07-23: 新增 domains/novelty 新颖性判断独立模块

### 背景
对照宝宸知识库 Wiki 中专利实务/新颖性目录的全部 22 个规则文件（覆盖 19 个知识域），
项目现有代码仅在 `domains/inventiveness/` 中完整实现了创造性（A22.3）判断，而新颖性（A22.2）
仅有 `domains/rules/data/articles/patent-law-a22.2.yaml` 的 4 步法条框架概述，
未实现为独立的 Pregel 子图、LLM 判断节点或可调用的 Agent 工具。

### 变更内容

**新增模块** — `domains/novelty/`（8 文件）

| 文件 | 行数 | 说明 |
|------|:---:|------|
| `doc.go` | 47 | 包文档：法条依据 + 判断框架 + 使用方式 |
| `types.go` | 135 | 输入/输出类型：9 结构体（NoveltyInput/Result + 4 子结果） + 6 StateKey 常量 |
| `framework.go` | 192 | ArticleFrameworkProvider 接口 + defaultA222Framework() 降级框架（~130 行 Markdown） |
| `graph.go` | 82 | 7 节点 Pregel 图拓扑（含条件边路由） |
| `nodes.go` | 970 | 5 LLM 节点（含完整 prompt）+ 6 个 JSON Schema + 5 个解析函数 + 辅助函数 |
| `tool.go` | 182 | evaluate_novelty 工具注册 + Pregel 图执行 |
| `novelty_test.go` | 530 | 12 节 ~42 个单元测试 |
| `integration_test.go` | 140 | 8 个集成测试（全图流/跳过传播/工具执行） |

**Pregel 图拓扑**（7 节点线性链 + 条件分支）：
- `load_input` → `prior_art_check` → `single_compare` → `conflict_check` → `grace_priority`
- → 条件边：`TechDomain` 非空时 → `special_domain` → `generate_conclusion`
- → `TechDomain` 为空时直连 → `generate_conclusion` → `__end__`

**判断框架覆盖**（19 个知识域）：
1. 现有技术定义（A22.5）+ "为公众所知"严格/宽松标准
2. 书面公开（出版物定义+流通渠道）+ 互联网公开（法律地位+时间认定）
3. 公开使用与默示保密义务 + 销售公开 + 口头公开
4. 单独对比原则 + 全部特征对比
5. 上下位概念 + 惯用手段直接置换
6. 数值范围 8 种判断情形
7. 四要素综合判断（领域/问题/方案/效果）
8. 性能/参数/用途/制备方法特征
9. 隐含公开 + 权利要求撰写方式影响
10. 抵触申请三要件 + 全文内容制 + 2008 修法变化
11. 宽限期三种情形 + 第三方独立公开穿透
12. 国际/本国优先权 + "相同主题"四要素
13. 化学领域特殊规则（通式化合物/立体异构体/晶体/组合物/制药用途）
14. 充分公开（可实施性）要求

**YAML 规则补充**：
- `domains/rules/data/articles/patent-law-a22.2.yaml`：4→8 步增强（新增公众所知/充分公开/抵触申请/宽限期/优先权步骤）
- `domains/rules/data/rules/novelty-rules.yaml`：新增 11 条规则（覆盖 8 个判断维度）
- `domains/rules/data/rules/patent-core.yaml`：补充 A22.5 现有技术定义规则

**Agent 集成**：
- `domains/patent.go`：注册 `evaluate_novelty` 工具到 Patent Agent 的 ExtraTools

### 测试统计
- `go test ./domains/novelty/`：~50 测试用例全部通过
- `go test -race ./domains/novelty/`：通过
- `golangci-lint run ./domains/novelty/`：0 issues
- `go build ./domains/`：通过

### 涉及敏感路径
- `domains/patent.go`（ExtraTools 注册——仅追加条目，不修改现有逻辑）

### AI 参与级别
- L2（新增功能模块，仅涉及 patent.go 敏感路径的追加注册）

---

> **职责边界**：本文件记录 **AI 参与的功能变更决策**——做了什么、为何做、潜在风险。
> **与 CHANGELOG.md 的分工**：`CHANGELOG.md` 记录对用户可见的版本变化（功能/修复/破坏性变更），
> 按 Keep a Changelog + 语义化版本组织。本文件记录 AI 决策上下文（背景、变更理由、验证结果），
> 供开发者和 AI 助手追溯"为什么这样改"。

## 2026-07-21: P0-1 Phase 3 — iface 消费侧迁移

### 背景
Batch 2/3 创建了 `agentcore/iface/` 接口包（AgentRunner/EventBus/LifecycleHook/Extension/Tracer）
和适配器（iface_adapter.go），但消费侧（guardrails, server 等）仍直接依赖 agentcore 具体类型。

### 变更内容

**iface 接口扩展**
- 新增 `iface/message.go`：`Message` 结构体和角色常量（RoleSystem/User/Assistant/Tool）
- 增强 `iface/event.go`：添加 `SimpleEvent` 类型和 `NewEvent()` 构造函数；`Event` 接口新增 `Payload() any` 方法
- 新增 `iface/context.go`：`AgentContext` 接口（`Input() string`, `Messages() []Message`）
- iface 上下文类型（`AgentRunContext`, `ModelCallContext`）增加 `Raw any` 字段，用于适配器保存原始 agentcore 指针

**适配器增强（agentcore/iface_adapter.go）**
- `eventBusAdapter.Emit()`：当 iface.Event 包含 payload 时，通过 `payloadEvent` 包装保留原始事件体
- `ifaceWrappedEvent.Payload()`：返回原始 agentcore.Event；对 `payloadEvent` 解包返回其 payload
- 新增 `ifaceLifecycleHookAdapter`：将 `iface.LifecycleHook` 转换为 `agentcore.LifecycleHook`（上下文类型转换 + Raw 传递）

**guardrails 迁移**
- `levels.go`：`New()` 返回 `iface.LifecycleHook`；struct 嵌入 `iface.BaseLifecycleHook`；AfterModelCall 签名使用 `*iface.AgentRunContext`/`*iface.ModelCallContext`，内部通过 `toAgentMCC()` 类型断言访问 agentcore 字段
- `citation_gate.go`：同样迁移到 `iface.LifecycleHook`，去掉直接 agentcore 依赖

**server 迁移**
- `server.go`：`eventBus` 字段类型从 `*agentcore.EventBus` 改为 `iface.EventBus`；`New()` 通过 `NewIFaceEventBus()` 包装
- `disclosure.go` / `disclosure_events.go`：事件总线接口和 InventivenessTrigger 迁移到 `iface.EventBus`
- `skills.go`：事件订阅和发射迁移到 `iface.EventType`/`iface.EventHandler`
- `chat.go`：agent.OnAll 保持 agentcore 类型（agent 内部事件总线不变）

**调用端适配**
- `domains/*.go` 和 `integration/*.go`：guardrails.New() 调用处添加 `agentcore.NewIFaceLifecycleHook()` 包装
- `cmd/mady/server.go`：consumer.OnEvent 通过 iface 适配器桥接
- 测试文件全面更新以使用 iface 类型

### 架构设计
- **类型断言适配器模式**：消费侧暴露 iface 接口，内部通过 `Raw` 字段 + 类型断言访问 agentcore 具体类型
- **payloadEvent 传递链**：iface.SimpleEvent → eventBusAdapter → payloadEvent → agentcore.EventBus → ifaceWrappedEvent → 消费者通过 Payload() 获取原始事件体
- **双向适配**：`NewIFaceLifecycleHook()` 和 `NewIFaceEventBus()` 提供 iface→agentcore 转换

### 验证结果
- `go build ./...` — 通过
- `go vet ./...` — 通过
- `go test -race ./agentcore/... ./guardrails/... ./server/...` — 全部通过

## 2026-07-21: TUI 文档与代码一致性修复

### 背景
对比分析 TUI 模块代码实现与 `.qoder/repowiki` 文档后发现 12 项不一致，其中 3 项为高严重度：
LAYERS.md 目录结构严重滞后（遗漏 32+ 文件）、TickInterval 默认值注释与实际值矛盾、
core.Every 移除决策未记录。

### 变更内容

**`tui/tui.go` — TickInterval 注释修正**
- 将注释从 "Defaults to 16ms (~60 fps)" 改为 "Defaults to 8ms (~125 fps)"
- 实际代码默认值为 `8 * time.Millisecond`（125fps），原注释声称 16ms（60fps）与实际不符
- 补充说明选择更高帧率的原因（确保流式输出流畅）及调优建议

**`tui/LAYERS.md` — 目录结构全面更新**
- core 包：补充遗漏的 5 个文件（cell.go, celldiff.go, cellparse.go, cellrender.go, sgr.go）
- terminal 包：补充 ansi.go
- component 包：补充遗漏的 19 个文件（editor_*.go 5 文件、judgment_view.go、review_gate.go、
  session_selector.go、command_center.go、skill_center.go、system_status.go、table.go、
  todo_panel.go、viewport.go、evidence_overlay.go、tool_card.go、syntax_langs.go、syntax_tokenizer.go）
- chat 包：补充遗漏的 11 个文件（chat_app_layout/stream/tool.go、chat_history_render*.go、
  chat_history_input/selection.go、clipboard.go、reasoning.go、state.go）
- stdio 包：补充 layout.go
- 根包：补充 tui_focus.go, tui_lifecycle.go, tui_input.go, tui_loop.go, tui_render.go
- 更新 Layer 表格描述，包含各包文件数量

**`tui/LAYERS.md` — 新增设计决策章节**
- Cell-Level Rendering Model：记录 core/cell*.go + sgr.go 的单元格级渲染子系统
- Editor Subsystem：记录 Editor 5 文件拆分架构（1577 行，按职责分组）
- core.Every Removal：记录 API 移除决策及 TUI.Every 替代方案
- ChatApp Multi-File Architecture：记录 ChatApp 14 文件拆分架构

**`tui/component/session_selector.go` — 添加文件头文档注释**
**`tui/component/skill_center.go` — 添加文件头文档注释**
**`tui/component/table.go` — 添加文件头文档注释**
**`tui/component/todo_panel.go` — 添加文件头文档注释**

**`tui/scripts/verify_layers.sh` — 新增 LAYERS.md 同步检查脚本**
- 提取 LAYERS.md 代码块中的 .go 文件名，与磁盘实际文件对比
- 检测遗漏文件（on disk but not listed）和幻影条目（listed but not on disk）
- 支持 `--diff` 模式输出 diff 格式
- 可集成到 CI 或 `make verify` 流程

**`.qoder/repowiki/.../用户界面 (tui).md` — TickInterval 描述修正**
- 将 "默认约 60fps" 改为 "默认 8ms（~125fps），确保流式输出流畅"

### 验证
- `go build ./tui/...` ✅ 通过
- `go vet ./tui/...` ✅ 通过
- `./tui/scripts/verify_layers.sh` ✅ 90 文件全部同步

### 风险
- 无代码逻辑变更，仅注释和文档更新
- LAYERS.md 变更不影响编译和运行时行为
- 验证脚本为新增工具，不影响现有流程

---

## 2026-07-21: 修复 TUI 运行时 stderr 泄漏 + 编辑器占位符复位

### 背景
TUI 进入 alternate screen 模式后，`os.Stderr` 与 `os.Stdout` 共享同一个终端设备，
任何向 stderr 的写入（`slog.Warn`/`log.Printf`/`fmt.Fprintf(os.Stderr,…)`）都会直接
出现在 TUI 显示区（尤其是游标所在的编辑器输入区），导致"警告信息溢出到输入区"的视觉污染。

同时，编辑器占位符在 `Idle()` 中被清空为 `""` 而非恢复原始值，
导致首次 Agent 运行后输入区永久丢失"输入消息…"提示文字。

### 变更内容

**`cmd/mady/tui.go` — stderr 重定向**
- 新增 `redirectStderrToFile(madyHome string) func()`：在 `app.Start()` 成功后
  将 `log.SetOutput`、`slog.SetDefault`、`os.Stderr` 三者重定向到
  `~/.mady/logs/mady.log`，并返回 cleanup 函数在 TUI exit 后恢复
- `slog` 级别降为 `LevelWarn`（TUI 运行时 Debug/Info 不写日志文件）
- 日志文件格式：`\n--- mady tui started at {time} ---\n` 分隔每次启动
- `runTui()` 调用时机：`app.Start()` 成功后立即执行，失败时的错误仍可输出到真实终端

**`tui/chat/chat_app.go` — 编辑器占位符恢复**
- `ChatApp` 新增 `defaultPlaceholder string` 字段，构造函数中初始化为空
- `newChatApp()` 设置 `defaultPlaceholder: "输入消息…（/ 查看命令）"`
- `Idle()` 使用 `a.defaultPlaceholder` 替代硬编码 `""`

### 验证
- `go build ./...` ✓
- `go vet ./cmd/mady/... ./tui/chat/...` ✓
- `go test ./tui/... -count=1` ✓（全部 9 子包通过）
- `go build ./cmd/mady/...` ✓

## 2026-07-21: Phase 3 工作流工具 + TUI 判断摘要条与浮层统一

### 背景
- 从开发计划中执行 Phase 3（P2 medium-term）和 TUI 3.x 剩余任务
- Phase 1/2 任务在之前的 Sprint 中已完成，计划文档已过期

### 变更内容

**Phase 3.1 — YAML 驱动 WorkflowManifest 加载**
- `domains/reasoning/manifest.go`: 新增 `WorkflowManifestStore.LoadDir()` 支持从 YAML 目录加载 manifests；`GlobalWorkflowStore()` 全局单例
- `domains/reasoning/handoff_integration.go`: `NewWorkflowRunner` 优先查找 YAML 加载的 manifest，回退内置默认值
- `cmd/mady/framework.go`: `loadWorkflowManifests()` 在启动时种子化 YAML 模板文件并加载

**Phase 3.2 — 说明书撰写工具**
- `workflows/patent/specification.go`: Pregel 5 节点图（技术领域→背景→发明内容→附图→实施方式→组装），含 `BuildSpecificationGraph()`
- `workflows/patent/tool.go`: 注册 `specification_drafter` 工具

**Phase 3.3 — 审查员模拟辩论**
- `workflows/patent/debate.go`: Pregel 3 轮辩论图，预置 4 类审查意见（新颖性/创造性/清楚性/支持性），含 `BuildDebateGraph()`

**TUI 3.1 — 主界面"当前判断"摘要条**
- `tui/chat/events.go`: 新增 `JudgmentSummary` 数据模型
- `tui/chat/chat_app.go`: `chatModel` 增加 `judgmentSummary` 字段 + `SetJudgmentSummary()` / `ClearJudgmentSummary()` 公共方法
- `tui/chat/chat_app_layout.go`: `updateJudgmentView()` 从 model 填充所有 JudgmentView 字段
- `tui/chat/chat_app_stream.go`: 事件 `onAgentStart` 清除摘要、`onApprovalPrompt` 从 ReviewGatePayload 填充、`onAgentInterrupt` 显示中断理由

**TUI 3.2 — 浮层四分类补齐**
- `tui/chat/chat_app.go`: 新增 `OpenSelectionOverlay` / `OpenReviewOverlay` / `OpenGateOverlay` / `OpenSystemOverlay` 四类构造方法
- `tui/chat_bridge.go`: 补全 `OverlayCatSelection` → `OverlaySelection` 映射

**TUI 3.3 — 状态语言统一**
- `tui/component/judgment_view.go`: `statusLabelFromStatus` 新增 `"analyzing"` → "分析中"
- `tui/chat/chat_app_layout.go`: 根据 `ActiveTools` 判断"分析中"状态

### 验证
- `go build ./...` ✓
- `go test -race ./...` ✓（全部通过）
- `cd tools && go test -race ./...` ✓
- `golangci-lint run` ✓（修复 9 个 lint 问题：gofmt/gosec/staticcheck/gocritic/unused）
- Pre-commit hooks 全通过（trailing-whitespace/end-of-file-fixer/gofmt/goimports/govet/golangci-lint/sensitive-paths/commitlint）
- 15 文件 +1388/-15 行，已推送至 origin/main

## 2026-07-21: Sprint 3 — 大文件拆分重构（R7/R8/R9）


## 2026-07-21: Code Review 跟进 — 三项改进实施

### 背景
- 提交 7b85ae7（Sprint 3）代码审查后提出三个改进建议：
  1. handleGetImages 使用类型化 imageInfo 结构体替代 map[string]any + 类型断言
  2. 内联 4 个 discoveryState 空委托方法
  3. 将 handleCdp/handleConsole 拆入独立的 browser_tool_debug.go

### 变更（6 文件）

**S1: imageInfo 结构体（tools/browser_tool_media.go）**
- 新增 imageInfo 结构体（index/src/alt/width/height/displayed），JSON 标签对齐 JS 输出
- handleGetImages 解析目标从 []map[string]any 改为 []imageInfo，消除 11 行类型断言
- 格式化循环改为直接访问结构体字段，类型安全性和可读性提升

**S2: 内联委托方法（mcp/discovery_state.go + mcp/discovery.go）**
- 删除 4 个纯转发方法：onResourceUpdated / onResourcesListChanged / onPromptsListChanged / onAsyncRefreshError
- 13 个调用点替换为直接 s.cfg.X(args) 调用+ nil 守卫
- discovery_state.go 缩减至 158 行，移除不再需要的 "context" import

**S3: browser_tool_debug.go（tools/）**
- 新建 tools/browser_tool_debug.go（73行），含 handleCdp + handleConsole
- tools/browser_tool_handlers.go 从 167→105 行，仅含 handleSnapshot + handleEvaluate

### 验证
- go build ./... — 通过（根模块 + tools 子模块）
- go vet ./... — 通过
- golangci-lint run — 0 issues（根模块 + tools）
- go test -race ./mcp/... — 通过（21.6s）
- cd tools && go test -race ./... — 全部通过


### 背景
- 持续推进文件规模治理，对项目中最大的非测试源文件进行拆分，降低认知负荷和合并冲突风险。
- 本次处理 3 个大文件：mcp/discovery.go（889→717行）、tools/browser_tool_handlers.go（817→167行）、agentcore/event.go（730→254行）。
- 纯重构，零行为变更。

### 变更（7 文件）

**R7: mcp/discovery.go 拆分**
- 新增 mcp/discovery_state.go（183行），提取所有 discoveryState 缓存/状态管理方法（cachedResources、storeResources、cachedResourceTemplates、cachedPrompts、markSubscribed、invalidateResource 等 17 个方法）。
- mcp/discovery.go 从 889 行缩减到 717 行（-19%），专注于发现协议的核心编排逻辑。

**R8: tools/browser_tool_handlers.go 拆分**
- 新增 tools/browser_tool_navigate.go（257行）：handleNavigate + handleBack（导航类处理函数）
- 新增 tools/browser_tool_interact.go（229行）：handleClick + handleType + handleScroll + handlePress + handleDialog（交互类处理函数）
- 新增 tools/browser_tool_media.go（200行）：handleScreenshot + handleVision + analyzeBrowserScreenshot + handleGetImages（媒体/视觉类处理函数）
- tools/browser_tool_handlers.go 从 817 行缩减到 167 行（-80%），仅保留 handleSnapshot + handleEvaluate + handleConsole + handleCdp

**R9: agentcore/event.go 拆分**
- 新增 agentcore/event_types.go（482行），提取所有事件类型定义、构造器、JSON 序列化和 error 辅助函数
- agentcore/event.go 从 730 行缩减到 254 行（-65%），仅保留 EventBus 实现

### 设计要点
- 按功能领域垂直拆分，文件以 browser_tool_<category>.go 命名，方便 IDE 模糊搜索
- 事件类型与 EventBus 实现分离：event_types.go 只定义数据结构和序列化，event.go 只负责发布/订阅编排
- 所有新文件保持 package 不变，不引入新的公共 API
- drainSentinel 保留在 event.go 中（紧耦合于 Drain 方法），不随事件类型迁移

### 验证
- go build ./... — 通过
- go vet ./... — 通过
- golangci-lint run — 0 issues（根模块 + tools 子模块）
- go test -race ./agentcore/... ./mcp/... — 全部通过（含 mcp 测试 20.7s）
- cd tools && go test -race ./... — 全部通过



## 2026-07-21: 补充“绞车带轴”专家盲测单案用例卡

### 背景
- 用户确认将已脱敏的 `绞车带轴` 案件纳入下一步专家盲测，并希望直接推进到可执行的准备物，而不是停留在“是否适合作为盲测案件”的讨论层面。
- 仓库中已有总纲性质的 `docs/design/p3-blind-test-plan.md`，但缺少一份能够直接发给组织者使用的“单案用例卡”，导致实际执行时仍需临场补任务书、评分表和单案通过线。

### 变更（3 文件）
- `docs/design/p3-blind-test-case-jiaoche-daizhou.md`
  - 新增“绞车带轴”单案盲测用例卡。
  - 固化了案件定位、输入材料清单、给专家的任务书、组织者操作说明、评分表、整体判定口径、观察记录项和单案通过标准。
  - 明确该案同时产出`发明`与`实用新型`两套权利要求书，用于检验双案型区分能力，而不是只测单一文稿生成。
- `docs/design/README.md`
  - 把 `p3-blind-test-plan.md` 与新建的单案用例卡加入设计索引，便于后续持续扩展更多盲测案件时统一检索。
- `docs/decisions/AI_CHANGELOG.md`
  - 追加本条记录，满足“变更即记录”的仓库约定。

### 设计要点
- **总方案 + 单案卡片分层**：总方案继续负责指标、方法学和流程共性；单案卡片负责某一案件的材料、任务书和执行细节，避免把 `p3-blind-test-plan.md` 膨胀成难以复用的长文档。
- **以“代理人可接手修改”为通过口径**：本案目标不是一键出最终稿，而是验证 Mady 能否生成有业务价值的初稿，这更贴近当前阶段的产品成熟度与人机协作定位。
- **把双案型区分作为显式评分维度**：因为该案要求同时输出`发明`和`实用新型`，若不单列评分维度，容易出现“两版文本实质相同但总分不低”的误判。

### 验证
- 文档内容已与 `docs/design/p3-blind-test-plan.md` 的 P3 总体方法保持一致。
- 新文档已纳入 `docs/design/README.md` 索引，可作为后续新增更多单案盲测卡的模板起点。

## 2026-07-21: 为“绞车带轴”案件补充 Mady 基线输出模板

### 背景
- 在完成 `90-任务书.md` 与 `99-评分要点.md` 后，用户继续推进该案的盲测准备，希望补齐一份可直接承接 `Mady` 生成结果的“基线输出模板”，用于 T2 盲化标注阶段作为被评审文本。
- 现有仓库仅有通用导出能力（如 `workflows/patent/export.go` 的 Markdown 文件头模式），没有面向单案盲测的基线输出模板，因此需要在案件目录内新增一份可直接填充和外发的 Markdown 骨架。

### 变更（2 文件）
- `/Users/xujian/projects/exam-papers/绞车带轴/95-Mady-基线输出模板.md`
  - 新增案件级基线输出模板，覆盖发明/实用新型两套权利要求书草案、区别点说明、不确定点与人工复核提示。
  - 在顶部增加元信息注释块，承接模型、运行入口、输入材料和操作人信息，但提醒外发给专家前可删除来源信息。
  - 模板中的内部自检提示已对齐本案最新口径：权利要求 1 优先围绕相交通孔与锁链式适配，不把定位柱默认当作独立权利要求必备特征。
- `docs/decisions/AI_CHANGELOG.md`
  - 追加本条记录，延续“变更即记录”的仓库约定。

### 设计要点
- **模板承接输出而非替代输出**：该文件是 `Mady` 基线产物的整理骨架，不直接预填具体权利要求文本，避免在还未真实运行时伪造结果。
- **一份文件兼顾内部与外发**：通过 HTML 注释和引用块承载内部元信息与复核提示，使同一份模板既可用于内部留档，也能轻量处理后发给专家盲评。
- **保持盲评友好结构**：正文按“发明 / 实用新型 / 区别点 / 不确定点”固定布局，便于多个案件、多个专家横向比对。

## 2026-07-21: 为“绞车带轴”案件补充 Mady 基线输出 v1 草案

### 背景
- 在案件目录中落地 `95-Mady-基线输出模板.md` 后，用户继续推进下一步，希望不只停留在空模板，而是直接形成一份首版基线答案，作为后续内部复核与专家盲评版裁剪的基础。
- 该案的评审口径已明确收敛：权利要求 1 优先围绕“相交通孔 + 锁链式适配”构造，`定位柱`不再默认视为独立权利要求必备特征，因此基线答案需要显式体现这一收敛策略。

### 变更（2 文件）
- `/Users/xujian/projects/exam-papers/绞车带轴/96-Mady-基线输出-v1.md`
  - 新增首版基线输出草案，包含发明/实用新型两套权利要求书草案、相对对比文件的区别点说明、以及不确定点与人工复核提示。
  - 发明与实用新型的独立权利要求均围绕杆状本体、第一通孔、第二通孔及其相交关系展开，并将锁链式捆绑带适配作为主保护点。
  - 将 `定位柱`、绳索式适配和整体绞车常规结构下沉到从属或扩展保护层次，避免再次偏回“多类型兼容一次性塞进独权”的旧口径。
- `docs/decisions/AI_CHANGELOG.md`
  - 追加本条记录，区分“模板文件”和“已填充的 v1 基线内容”。

### 设计要点
- **先给可复核的 v1，再做盲评裁剪**：当前文件保留内部元信息与复核提示，便于先做专业口径校准，再生成剥离来源信息的专家盲评版。
- **独权口径显式收敛**：首版答案不追求把所有实施方式都塞进权利要求 1，而是优先稳住锁链式适配的核心区别点。
- **保留不确定点而不伪装确定性**：对“链环分别穿设或卡接”的具体措辞强度、发明与实用新型的进一步区分空间等，明确列入人工复核项，避免把尚待校准的表述当成定稿事实。

## 2026-07-21: 为“绞车带轴”案件生成专家盲评版

### 背景
- 在形成 `96-Mady-基线输出-v1.md` 后，用户继续推进下一步，希望直接生成一份可以外发给专家的匿名评审稿。
- 盲评版需要剥离内部来源信息、模型痕迹和复核提示，同时尽量保留与内部版一致的正文结构，确保专家看到的是“待评审输出”，而不是“系统过程材料”。

### 变更（2 文件）
- `/Users/xujian/projects/exam-papers/绞车带轴/97-专家盲评版.md`
  - 新增专家盲评版正文，保留发明/实用新型权利要求书草案及两段区别点说明。
  - 移除了顶部元信息、`Mady` 标识、内部复核提示和不确定点说明，降低来源暴露风险。
  - 保持与 `96-Mady-基线输出-v1.md` 基本一致的正文内容，便于内部版与外发版逐项对照。
- `docs/decisions/AI_CHANGELOG.md`
  - 追加本条记录，区分“内部复核版”与“专家盲评版”的用途边界。

### 设计要点
- **内容不重写，只做盲评裁剪**：盲评版不再引入新的 substantive 改动，避免内部版与外发版因文本漂移导致评审结果不可追溯。
- **去来源、保正文**：核心策略是去掉元信息和过程说明，但保留专家需要评审的全部内容。
- **保留案件结构一致性**：标题与章节顺序保持稳定，方便后续多个案件统一组织专家评审材料。

## 2026-07-21: 为“绞车带轴”案件补充专家评分表

### 背景
- 在完成任务书、内部复核版、专家盲评版和评分要点后，该案件还缺少一份由专家直接填写的统一评分表。
- 若没有标准化表单，不同专家容易各写各的评语，导致后续汇总时难以对齐“整体判定、分项得分、重大缺陷、修改优先级”这几类关键信息。

### 变更（2 文件）
- `/Users/xujian/projects/exam-papers/绞车带轴/98-专家评分表.md`
  - 新增案件级专家评分表，配套 `90-任务书.md` 与 `97-专家盲评版.md` 使用。
  - 评分表包含基本信息、整体判定、8 项分项评分、关键问题判断、总分汇总和最终评语，口径与 `99-评分要点.md` 保持一致。
  - 在“关键问题判断”中单列权利要求 1 主保护点、扩展特征误入独权、双案型区分、重大缺陷等问题，降低专家评审结果的自由散射。
- `docs/decisions/AI_CHANGELOG.md`
  - 追加本条记录，标记该案件已从“材料包”补齐为“可执行盲测包”。

### 设计要点
- **评分表与评分要点分工明确**：`99-评分要点.md` 服务于组织者统一口径；`98-专家评分表.md` 服务于专家填写，两者互相配套但不混用。
- **先结构化采分，再写自由评语**：先收分项分数与关键判断，再留开放评语，能同时兼顾统计分析和专业反馈。
- **对齐当前独权口径**：评分表中显式检查“相交通孔 + 锁链式适配”主线，避免专家再被旧口径带回“定位柱必须进独权”的判断。

## 2026-07-20: 新增现有 TUI + 浮层优化方案设计稿

### 背景
- 用户希望把《中论》所强调的“审慎、平衡、先辨其据再断其行”体现在前端体验中，
  但在当前阶段明确不启动多栏工作台重构，而是先把现有单主视图 `TUI` 与 `Overlay`
  浮层体验整理成一份可直接落地的设计方案。
- 仓库内已有一份偏长期、多栏化方向的已归档设计稿 `docs/archive/tui-design-plan.md`，
  但它不适合作为当前迭代的直接实施依据，因此需要新增一份更贴近现状的收束版文档。

### 变更（2 文件）
- `docs/design/tui-overlay-optimization-v0.1.md`
  - 新增正式设计稿，明确现有 `TUI` 的优化方向仍以“单主视图 + 按需浮层”为核心，
    不引入 Sidebar / 多栏工作台重构。
  - 文档收敛了主界面职责、浮层四分类、证据详情/复核门/系统态三类核心浮层、
    键位规范、状态语言统一、分阶段落地边界与验收标准。
- `docs/design/README.md`
  - 将上述文档加入设计索引，便于后续在 `docs/design/` 下检索和持续维护。

### 验证
- 文档路径与索引结构已对齐当前 `docs/design/` 目录组织。
- 方案内容与当前“先稳主流程、不过早扩布局复杂度”的项目节奏一致；
  适合作为后续小步实施 `TUI` 体验优化的基线文档。

## 2026-07-21: 专利分析工作流检索真实化 + CLI 子命令 + TUI 斜杠命令

### 背景
- 用户反馈专利分析工作流（新颖性分析 / OA 答复）脱离实际使用场景：
  核心检索节点为占位实现，缺少直接调用的入口，TUI 中只能通过自然语言让 LLM 决定是否调用工具。
- 调研确认代码基础设施（Pregel 图、规则引擎、双轨检查器、disclosure 管线）本身完成度很高，
  但实际可用性被以下三个 gap 限制：① searchNode 是占位 ② 无直接 CLI 入口 ③ TUI 需斜杠命令快捷触发。

### 变更（7 文件）
- `workflows/patent/analysis.go`
  - 新增 `GraphOption` / `graphConfig` / `WithRetriever` 模式，对齐 disclosure/graph.go 的注入风格
  - 将 `searchNode` 从裸函数改为闭包工厂 `newSearchNode(retriever)`：
    - retriever 非 nil 时查询 `domain.DomainRetriever` 返回真实现有技术
    - retriever 为 nil 时返回占位文本（向后兼容）
  - 新增 `BuildNoveltyGraphWithRulesWithOpts(opts ...GraphOption)` 支持可选检索器注入
  - 新增 `BuildNoveltyGraphWithOpts` 对应无规则引擎版本的选项注入
  - 保留 `BuildNoveltyGraph` / `BuildNoveltyGraphWithRules` 作为无选项的向后兼容包装
- `workflows/patent/tool.go`
  - `NewPatentNoveltyTool(opts ...GraphOption)` 接受选项参数，传递给 `BuildNoveltyGraphWithRulesWithOpts`
- `workflows/patent/analysis_test.go`
  - `TestSearchNode` 改为调用 `newSearchNode(nil)` 闭包工厂
  - 新增 `TestSearchNode_WithRetriever` 验证 nil retriever 场景
- `domains/patent.go`
  - 新增 `globalPatentRetriever` + `SetupPatentRetriever(r)` + `GetPatentRetriever()`，遵循 `globalDraftingRunner` 的全局注入模式
  - `PatentAgentConfig` 中 `NewPatentNoveltyTool(WithRetriever(globalPatentRetriever))` 传递检索器
- `cmd/mady/framework.go`
  - `initReasoningAndTemplates` 中新增：从 `KnowledgeBackend` 构建 `PatentDomainRetriever` 并调用 `SetupPatentRetriever`
- `cmd/mady/patent.go`（新文件）
  - 新增 `mady patent novelty` 和 `mady patent oa` CLI 子命令，直接运行 Pregel 图输出 Markdown
- `cmd/mady/tui_session.go` + `cmd/mady/slash_registry.go`
  - 新增 `/novelty <描述>` 斜杠命令：直接触发新颖性分析 Pregel 图
  - 新增 `/oa <OA文本>` 斜杠命令：直接触发 OA 答复 Pregel 图
  - 新增 `/patent` 帮助命令
  - `main.go` 注册 `patent` 子命令并更新用法说明

### 设计决策
1. **检索注入沿用 disclosure 的 `GraphOption` 模式**，而非全局变量——保持架构一致性，函数签名零破坏
2. **CLI 子命令直接调用 Pregel 图**（方式二），而非包装成 Tool——更轻量，适合脚本/管道场景
3. **斜杠命令绕过 LLM 意图分类**——确定性触发工作流，避免"描述了需求但 LLM 选择不调用工具"的问题
4. **确定性优先**：所有新增入口都复用已有的确定性规则引擎，LLM 仅作为可选增强层

### 风险与限制
- CLI 模式下 `globalPatentRetriever` 为 nil（未运行 setupFrameworkContext），search 节点自动降级为占位结果。需要用 TUI/Server 模式才能享受真实检索
- `openKnowledgeBackend` 返回的是 `knowledge.KnowledgeBackend` 接口，断言为 `*ksqlite.SQLiteStore` 可能失败（数据库格式不匹配），此时静默跳过

### 验证
- `go build ./...` 全量编译通过
- `go vet ./...` 零警告
- `go test ./workflows/patent/...` — 51 个测试全通过
- `go test ./domains/...` — 所有子包通过
- CLI 验证：`go run ./cmd/mady/ patent novelty "一种智能窗户清洁装置"` 输出完整分析报告（含规则引擎检查）
- CLI 验证：`go run ./cmd/mady/ patent oa "审查员认为..."` 输出 OA 答复骨架
- 向后兼容：不传 `WithRetriever` 的 `NewPatentNoveltyTool()` 与旧行为一致

## 2026-07-21: 专利分析导出 + OA 答复 LLM 增强节点

### 背景
- P4：新颖性分析和 OA 答复结果缺少导出功能（CLI 仅输出到 stdout，TUI 仅显示为系统消息）
- P5：OA 答复书骨架为纯确定性逻辑，缺少实质性论证段落，代理师需要从头撰写论证

### 变更（5 文件）
- `workflows/patent/export.go`（新文件）
  - 新增 `ExportNoveltyReport(output)` / `SaveNoveltyReport(output, filePath)` — 新颖性分析报告 Markdown 导出
  - 新增 `ExportOAResponse(output)` / `SaveOAResponse(output, filePath)` — OA 答复书 Markdown 导出
  - 文件头包含生成时间和类型元数据注释
- `workflows/patent/oa_response.go`
  - 新增 `OAGraphOption` / `oaGraphConfig` / `WithOAProvider` 模式（与 analysis.go 的 GraphOption 对齐）
  - 新增 `newOAEnhanceNode(provider agentcore.Provider)` — LLM 增强节点闭包工厂：
    - 读取确定性骨架 + 原始 OA 文本，构建 prompt 调 LLM 生成实质性论证段落
    - LLM 失败时静默降级（保留确定性输出，不阻塞管线）
    - provider 为 nil 时返回 no-op 节点
  - 新增 `BuildOAResponseGraphWithOpts(opts ...OAGraphOption)` — 有 provider 时管线变为 `draft_response → llm_enhance → approval_gate`
  - 保留 `BuildOAResponseGraph()` 作为向后兼容包装
  - 新增状态键 `OAStateLLMEnhanced` 标记增强是否生效
- `workflows/patent/oa_response_tool.go`
  - `NewOAResponseTool(opts ...OAGraphOption)` 接受选项，传递给 `BuildOAResponseGraphWithOpts`
  - 修复编码乱码："权���要求" → "权利要求"
- `workflows/patent/oa_response_test.go`
  - 新增 `TestBuildOAResponseGraphWithOpts_NoProvider` — 无 provider 时图运行正常且无增强
  - 新增 `TestOAEnhanceNode_NoopOnNilProvider` — nil provider 的 no-op 行为验证
  - 新增 `TestOAEnhanceNode_WithNilProviderGraph` — 有无 opts（无 provider）输出一致
- `cmd/mady/patent.go`
  - `novelty` 和 `oa` 子命令新增 `-o <file>` 可选参数，写入 Markdown 文件
  - 参数解析重构为 `parseCLIArgs(args)` 统一处理 `-f` / `-o` / 直接文本

### 设计决策
1. **导出复用已有 Markdown 输出**：export.go 不重新渲染结构，只在已有输出上添加文件头元数据。避免与 disclosure/export.go 的 AnalysisReport 结构冲突
2. **OA LLM 增强节点遵循 disclosure 内联工厂模式**：创建独立 Agent 实例、MaxTurns=1、JSON Schema 可选、失败静默降级——与 noveltyNode / reportNode 一致
3. **OAGraphOption 与 GraphOption 分离**：虽然模式相同，但 OA 图的配置项（provider）与新颖性图的配置项（retriever）类型不同，各自定义避免 type confusion
4. **LLM 增强为可选**：不改变默认行为，无 provider 注入时管线行为与之前完全一致

### 风险与限制
- OA LLM 增强节点的 SystemPrompt 目前内联在 Go 代码中（与 disclosure 的做法一致），未使用 prompt-templates/ 目录中的 JSON 模板。后续可迁移到模板系统
- LLM 增强对 provider 可用性有隐式要求——provider 不可用时静默降级，用户可能不知道增强未生效（由 OAStateLLMEnhanced 标记，但 TUI 目前未展示此标记）
- 导出功能仅支持 Markdown 格式（不含 DOCX），因为 OA 和 Novelty 输出不是结构化对象

### 验证
- `go build ./...` 通过
- `go vet ./...` 零警告
- `go test -count=1 ./workflows/patent/...` — 54 个测试全通过（新增 3 个）
- CLI: `mady patent novelty -o /tmp/report.md "描述"` 写入带元数据的 Markdown 文件
- CLI: `mady patent oa -o /tmp/response.md "OA文本"` 同上

---

## 2026-07-21: Slash 命令结果持久化到 Session（深度集成）

### 背景
- P4 阶段 `/novelty` 和 `/oa` 斜杠命令已添加（通过 `PrintSystem()` 显示），但输出未持久化到 `session.AgentStore` JSONL
- 正常对话通过 `submitInput()` → `agent.Run()` → `agent.SaveState()` 自动持久化，但斜杠命令绕过 Agent 直接调用 Pregel 图，结果仅存于 ChatHistory 内存，TUI 重启后丢失
- 已有文件导出功能（`export.go`），但未覆盖 TUI 内会话恢复场景

### 变更（1 文件）
- `cmd/mady/tui_session.go`
  - 新增 `persistSlashMessages(inputLine, outputText string)` 方法：
    - 若 `agentStore == nil` 静默跳过（持久化未启用）
    - 通过 `agentStore.Load()` 获取当前线程已有消息，构造用户消息（`RoleUser`+原始命令行）和助手消息（`RoleAssistant`+分析结果）
    - 调用 `agentStore.Save()` 写入 JSONL，错误仅记录日志不阻断显示
  - `handleNoveltySlash`：获取 Pregel 输出后先调用 `persistSlashMessages` 再 `PrintSystem`
  - `handleOASlash`：同上

### 设计决策
1. **先持久化再显示**：即使 JSONL 写入失败也不影响用户立即看到结果
2. **复用现有 AgentStore API**：`Save()` 内部 `syncMessages()` 自动做增量追加，只需传递完整消息列表
3. **用户消息保存原始命令行**：用 `ctx.input`（如 `/novelty "发明..."`）而非提取后的 description，确保线程历史可追溯完整输入
4. **5 秒超时**：持久化操作用 `context.WithTimeout` 保护，不阻塞事件循环

### 风险与限制
- 当前 `/branch` 的 `snap.Messages` 加载后通过 `ChatHistory.Append` 重建，但 ChatHistory 的消息 ID 与 AgentStore 的消息 ID 不互通——重启后 slash 结果理论上在 JSONL 中存在，但 TUI 需要显式加载恢复（当前 `/branch` 和默认启动的恢复机制尚未覆盖此场景）
- 不影响正常 Agent 对话的持久化流程（`submitInput` → `agent.SaveState` 路径不变）

### 验证
- `go build ./...` 通过
- `go vet ./...` 零警告
- `go test ./cmd/mady/...` 全部通过
- 手动验证：TUI 中 `/novelty "一种方法"` 执行后，session JSONL 文件包含对应 user/assistant 消息条目

---

## 2026-07-20: disclosure 真实 dry-run 收口与 SQLite 可写性预检

### 背景
- 真实 DeepSeek 环境下，`POST /v1/disclosure/analyze` 已恢复可用，但继续执行手工 dry-run 时，
  `POST /v1/disclosure/analyze/{task_id}/review` 在默认 `~/.mady/workspace/approvals.db`
  上返回 500。
- 受控实例日志确认根因为 `approval/sqlite: save: attempt to write a readonly database (8)`；
  也即审批留痕问题暴露得过晚，启动阶段未能提前识别默认数据目录/SQLite 文件不可写。

### 变更（4 文件）
- `domains/sqlite/approval_store.go`
  - 在 `NewApprovalStore()` 完成 schema 初始化后追加真实写探针：
    通过事务内向 `approval_records` 插入一条探测记录并回滚，
    将只读数据库从“运行时 /review 才失败”前移为“启动期打开 store 即失败”。
- `cmd/mady/server.go`
  - 新增 `preflightWritableSQLitePath()`，在打开 `approvals.db` / `eval.db` 前先验证：
    父目录可创建、目录可写、已存在 DB 文件可读写。
  - 调整 server 启动日志口径，把 SQLite 相关失败统一表述为“不可写”，
    明确它只会降级审批留痕或评估持久化，不阻断主服务。
- `domains/sqlite/approval_store_test.go`
  - 新增只读 DB 初始化失败回归测试，锁定写探针行为。
- `cmd/mady/server_test.go`
  - 新增 SQLite 路径预检会自动创建父目录的最小测试。

### 验证
- 真实手工 dry-run：
  - 默认 `~/.mady` 环境：成功跑到 `awaiting_review`，但 `/review` 暴露只读 DB 问题并产生日志。
  - 项目内可写 `MADY_HOME=/Users/xujian/projects/Mady/.tmp/mady-home` 环境：
    `analyze -> awaiting_review -> reviewed` 全链路通过，`approvals.db` 成功落库
    `disclosure_review / adopted` 记录。
- 代码级回归：
  - 待本轮针对性测试与诊断一起收口。

---

## 2026-07-20: 全仓技术债务修复（补充：修复 agent_run.go nilness tautology）

### 变更（1 文件）
- `agentcore/agent_run.go:264` 中条件 `mcc.Err != nil && err == nil` 的 `err == nil`
  是 tautological（前一行 `err != nil` 已 return），简化为 `mcc.Err != nil`

### 验证
- `go build ./...` ✅ | `go vet ./...` ✅ | `golangci-lint run` 0 issues ✅
- `go test -race ./agentcore/...` 全绿 ✅

---

## 2026-07-20: 全仓技术债务修复（Code Review 阶段——5 项结构改进）

### 背景
全仓技术债务扫描 + 3 轮修复（15 项）完成后，执行 Code Review 技能评估。审阅发现 5 项可改进：
1. `cmd/mady/tui_session.go` 已超 1000 行（1058 → 1123）
2. `renderAllWithState` 快/慢路径循环体完全重复（detectToolGroup 已提取但未消除）
3. `distributeVerticalFill` / `distributeHorizontalFill` 80% 代码重复
4. `withStartupDiscoveryTimeout` 硬读 `os.Args[1]`
5. `distributeHorizontalFill` 用指针参数更新 maxHeight

### 变更（5 项，7 文件 + 2 新增）

**1. 拆分 tui_session.go（+2 文件）**
- `tui_session.go`：1123 行 → 327 行（struct + accessors + 所有 handle* handler + sidebar + approval store）
- **新增** `tui_session_config.go`：buildAgentConfig / applyPlanModeThinking / extendConfig / applyPersistence / buildMemoryExtension / injectMemoryExtension
- **新增** `tui_session_agent.go`：getCurrentAgent / agentStatus / markAgentInitializing / setAgentInitError / swapCurrentAgent / shutdownAgent / agentUnavailableMessage / initializeAgentAsync / rebuildAgent / submitInput / resumeIfInterrupted
- 三个文件各约 120-330 行，每个文件一个聚焦职责

**2. 合并 renderAllWithState 快/慢路径（`tui/chat/chat_history_render.go`）**
- 提取 `renderMessagesRange(msgs, start, ...)` 封装共用的循环体
- 快路径：splice clean prefix + `renderMessagesRange(firstDirtyIdx)`
- 慢路径：`renderMessagesRange(0)`
- 消除 ~80 行结构重复，两路径行为永不漂移
- 修复快路径缺少 `return` 导致慢路径续跑的 bug（原始代码因 `return` 存在未触发）

**3. 合并 distributeFill（`tui/layout/flex.go`）**
- 提取 `distributeFill(fillCount, totalWeight, containerSize, used, sizes, rendered, renderFn)` 核心算法
- `distributeVerticalFill` 和 `distributeHorizontalFill` 各约 5 行包装
- 消除 ~30 行重复，`distributeHorizontalFill` 的 maxHeight 改为返回值消除指针参数

**4. 修复 os.Args 隐式依赖（`cmd/mady/framework.go` / tui.go / server.go / acp.go）**
- `withStartupDiscoveryTimeout(ctx)` → `withStartupDiscoveryTimeout(ctx, cmdName)`
- 三处调用方显式传入子命令名（`"tui"` / `"serve"` / `"acp"`）

### 文件规模变化
| 文件 | 改前 | 改后 | 说明 |
|------|------|------|------|
| `tui_session.go` | 1123 | 327 | -71%：handler + sidebar + approval store |
| `tui_session_config.go` | — | 167 | 新增：config 构造 |
| `tui_session_agent.go` | — | 166 | 新增：agent 状态管理 |
| `chat_history_render.go` | 947 | 522 | -45%：提取 renderMessagesRange 消除循环体重复 |
| `flex.go` | 528 | 512 | -3%：合并 distributeFill 核心算法 |

### 验证
- `go build ./...` ✅ | `go vet ./...` ✅ | `golangci-lint run` 0 issues ✅
- `go test -race ./tui/... ./cmd/mady/...` 全绿 ✅

### 影响范围
- `cmd/mady/tui_session.go`（重写）+ `tui_session_config.go`（新增）+ `tui_session_agent.go`（新增）
- `tui/chat/chat_history_render.go`（提取 renderMessagesRange、修复快路径漏 return）
- `tui/layout/flex.go`（合并 distributeFill、maxHeight 返回值化）
- `cmd/mady/framework.go` / `tui.go` / `server.go` / `acp.go`（cmdName 参数化）

### 风险等级
- 低（纯结构提取与语义等价修复，validate 途径全部通过）

---

## 2026-07-20: 全仓技术债务修复（第三轮：detectToolGroup 提取 + 快慢路径去重）

### 背景
第二轮修复（3+2 文件）后，`renderAllWithState` 的快速/慢速路径仍包含两段完全相同的工具组检测逻辑（各 ~25 行），每次维护需同步修改两处。

### 变更（1 文件）

1. **`tui/chat/chat_history_render.go` — 提取 `detectToolGroup()`**
   - 新增 `detectToolGroup(msgs, i)` 方法，封装工具组/中间轮次检测逻辑
   - 快速路径原 25 行内联代码 → `h.detectToolGroup(msgs, i)` 调用
   - 慢速路径原 25 行内联代码同步替换
   - **效果**：消除 50 行完全相同的代码，两个路径各剩 5 行调用
   - **修复**：初始实现遗漏 `i==0 && end==len-1` 时 `midTurn` 默认 true 的语义（`foundPrev` 标记补丁）

### 设计要点
- `detectToolGroup` 仅做检测不涉及渲染：返回 `groupEnd` 和 `ok` 布尔值，渲染仍由 `renderToolGroup` 完成
- 行为等价性：原始 inline 代码 `midTurn` 在无前一条非工具消息时默认 true（不折叠），提取版通过 `foundPrev` 标记保留该语义
- 至此 `renderAllWithState` 中所有可提取的内联代码均已提取完毕：`renderToolGroup`（R3）、`renderMessageSeparator`（R3）、`detectToolGroup`（本轮）

### 验证
- `go build ./...` ✅ | `go vet ./...` ✅ | `golangci-lint run ./tui/...` 0 issues ✅ | `go test -race ./tui/...` 全绿 ✅

### 影响范围
- `tui/chat/chat_history_render.go`（~70 行重组 + 新增 35 行方法）

### 风险等级
- 低（纯代码提取 + 语义等价补丁，不改变运行时行为）

---

## 2026-07-20: 全仓技术债务修复（第二轮：快速修复 + 函数拆分 + 布局模板化）

### 背景
在第一轮修复（context 传播 / framework 拆分 / innerLoop 拆分）基础上继续推进剩余高价值项。

### 变更（4 文件）

1. **`tools/bash.go:263` — 临时文件创建错误日志**
   - `os.CreateTemp` 失败时原为 `_ =` 静默忽略，现改为捕获并 `fmt.Fprintf(os.Stderr, ...)` 输出警告
   - 不影响执行流程（后续 `if tempFile != nil` 仍是防御性检查）

2. **`a2a/server_jsonrpc.go:96` — JSONRPC 响应编码错误日志**
   - `json.NewEncoder(w).Encode(results)` 原为 `_ =` 忽略，现改为捕获并用 `slog.Default().Warn` 记录

3. **`cmd/mady/tui_session.go` — `buildAgentConfig()` 三路分支重构**
   - 137 行 → 提取 `extendConfig()`（公共扩展后缀，14 行重复 ×3 消除）+ `applyPlanModeThinking()`（计划模式 Thinking 覆盖）
   - 三个 switch 分支保持独立（IntegratedChat / Router / 单 Agent），公共后缀集中到 `extendConfig`

4. **`tui/layout/flex.go` — `renderVertical` / `renderHorizontal` 填充分配模板化**
   - 提取 `distributeVerticalFill()` 和 `distributeHorizontalFill()` 替代两处重复的 Fill 分配循环
   - 两函数共享相同算法骨架，维度差异（高/宽）通过参数区分；`distributeHorizontalFill` 额外跟踪 `maxHeight`

### 设计要点
- `extendConfig` 按"接收 config → 注入扩展 → 注入记忆/持久化 → 返回"路径设计，各 switch 分支在调用前后可自由追加特殊配置（如 planMode thinking + permission extension）
- Fill 分配提取为独立函数后，Flex 的主渲染流程更清晰地表达为：测量 → 分配 → 布局 → 输出

### 影响范围
- `tools/bash.go`（2 行）
- `a2a/server_jsonrpc.go`（1 行替换）
- `cmd/mady/tui_session.go`（~140 行重组）
- `tui/layout/flex.go`（~80 行新增 + 替换）

### 风险等级
- 低（纯代码重组 + 错误日志增强，不改变任何运行时行为或协议语义）

---

## 2026-07-20: 全仓技术债务扫描 + 三项高价值修复

### 背景
对全仓库 841 个 Go 源文件进行了系统性扫描（go vet / go build / golangci-lint / go test / 静态分析），评估技术债务分布。选择三项高价值修复执行。

### 变更（6 文件）

1. **`context.Background()` 传播修复（3 处）**
   - `tools/browser_tool_handlers.go:152`：`navigateHandler` 中 Lightpanda fallback 调用 `RunChromeFallbackCommand(context.Background(), ...)` → 改用现有参数 `ctx`（已在函数签名中）
   - `agentcore/handoff.go:288`：`inheritRuntime` 中 `extensions.Register(context.Background(), ...)` → 加 `ctx` 参数并由调用方 `executeDelegate(ctx)` 传入
   - `acp/server.go:261`：`recordPermissionDecision` 中 `context.WithTimeout(context.Background(), 5s)` → 加 `ctx` 参数并由 `handlePrompt(ctx)` 中的闭包传入；同步更新测试文件调用签名
   - 验证：`go build ./acp/... ./agentcore/... ./tools/...` + `go test -race` 全线通过

2. **`setupFrameworkContext()` 函数拆分（cmd/mady/framework.go）**
   - 370 行单函数 → 8 个单职责子函数 + 主干约 70 行控制流
   - 提取：`loadManifests`、`discoverSkills`、`discoverMCP`、`initWorkspace`、`buildBaseTools`、`initPlugins`、`initMemorySystem`、`initReasoningAndTemplates`
   - 验证：`go build ./cmd/mady/...` + `go vet` + `golangci-lint` + `go test -race ./cmd/mady/...`
   - 行为等价：纯段落式机械提取，无共享局部变量，验证构建/测试全部通过

3. **`runInnerLoop()` 核心循环拆分（agentcore/agent_run.go）**
   - 254 行单函数 → 提取 `callModelWithFallback`（Provider 调用 + Context Overflow 重试封装）和 `guardTruncation`（max_tokens 截断守卫）
   - 主干从 254 行降至约 220 行，两个提取点均为纯功能段落、无外层循环控制流捕获
   - 验证：`go test -race ./agentcore/...` + `go test -race ./...`（含竞态）全线通过

### 设计要点
- **纯机械提取 + 语义等价**：三项修复均不改变运行时逻辑，仅为代码组织优化
- `inheritRuntime` 和 `recordPermissionDecision` 的 `ctx` 参数使上下文传播链更完整，消除潜在的资源泄漏路径
- `setupFrameworkContext` 的子函数名按"动词+领域"命名（`loadManifests`、`initWorkspace`），对应文件中已有的 `loadWikiStore`、`resolveWikiRoot` 模式

### 影响范围
- `tools/browser_tool_handlers.go`（1 行）
- `agentcore/handoff.go`（3 行）
- `agentcore/agent_run.go`（~60 行新增 + 替换）
- `acp/server.go`（3 行）
- `acp/permission_recording_test.go`（4 行）
- `cmd/mady/framework.go`（~380 行重组）

### 风险等级
- 低（纯代码重组 + 简单上下文传播，不触及安全敏感路径，不改变运行时行为）

---

## 2026-07-20: 手工 dry-run 暴露并修复 provider/disclosure 启动兼容问题（进行中）

### 背景
在按 `docs/design/disclosure-internal-dry-run.md` 执行真实手工彩排时，服务可正常启动，`POST /v1/disclosure/analyze` 也能返回 `task_id`，但真实 DeepSeek 环境下 disclosure 管线在提取阶段暴露出两类兼容性问题：

1. **模型名占位值 `"default"` 未被解析**
   - disclosure 提取/报告节点等大量调用仍使用 `Model: "default"`
   - `chatcompat` provider 会原样把 `"default"` 发给 DeepSeek，导致 400：
     - `The supported API model names are deepseek-v4-pro or deepseek-v4-flash, but you passed default`

2. **DeepSeek Chat Completions 端点不接受 `response_format: json_schema`**
   - disclosure 提取节点使用 `NewJSONSchemaResponseFormat(...)`
   - 真实请求在提取阶段报 400：
     - `This response_format type is unavailable now`

同时，本地继续构建 `mady` 时还顺手暴露了两个与当前彩排无关、但会阻塞 `build-mady` 的现存编译残留：

- `cmd/mady/tui_session.go` 丢失 `handleInput()` 函数声明
- `cmd/mady/tui_session_config.go` 存在未使用/重复 import

### 已完成变更（6 文件）

1. **统一解析 `"default"` 模型占位值（`provider/chatcompat/chat.go`）**
   - 新增 `resolveModelName()`
   - 当请求模型为 `""` 或 `"default"` 时，按当前 provider 环境解析为实际模型：
     - DeepSeek → `deepseek-v4-flash`
     - Zhipu → `glm-5.2`
     - Kimi → `kimi-k2.6`
     - Generic → 使用 `MODEL`
   - 这样 disclosure / memory / inventiveness 等所有仍写 `"default"` 的路径都会自动落到真实模型名

2. **补 provider 回归测试（`provider/chatcompat/chat_test.go`）**
   - 验证 DeepSeek 环境下 `"default"` 会解析为 `deepseek-v4-flash`
   - 验证 Generic 环境下 `"default"` 会解析为 `MODEL`

3. **为 disclosure 添加 DeepSeek response_format 兼容开关（`disclosure/model_compat.go`）**
   - 新增 `supportsJSONSchemaResponseFormat()`
   - 当前策略：
     - `PROVIDER` 为空或 `deepseek` → 关闭 `json_schema response_format`
     - 其他 provider → 保持开启

4. **在 disclosure 提取/新颖性节点按 provider 条件启用 schema（`disclosure/extract.go`、`disclosure/novelty.go`）**
   - DeepSeek 下不再显式挂 `NewJSONSchemaResponseFormat(...)`
   - 继续依赖 prompt 约束输出 JSON，并由既有解析逻辑本地解析

5. **补 disclosure 兼容测试（`disclosure/model_compat_test.go`、`disclosure/extract_config_test.go`）**
   - 验证默认 DeepSeek 环境下不使用 `json_schema response_format`
   - 验证 Generic 环境下仍允许 schema

6. **修复 `cmd/mady` 现存编译残留**
   - `cmd/mady/tui_session.go`
     - 补回丢失的 `handleInput()` 函数声明
     - 清理两个未使用 import
   - `cmd/mady/tui_session_config.go`
     - 清理未使用 `log` import
     - 删除重复 `permission` import

### 验证
- `go test -count=1 ./provider/chatcompat` 通过
- `go test -count=1 ./disclosure ./server` 通过
- `go build ./cmd/mady` 通过
- `make build-mady` 通过
- 真实服务启动与 `POST /v1/disclosure/analyze` 通过，能够拿到 `task_id`

### 当前状态
- **已修复**：`default model` 真实 provider 400
- **已修复**：`cmd/mady` 当前构建残留
- **仍待收口**：真实 DeepSeek disclosure 提取阶段仍返回 `response_format type unavailable`
  - 当前单测表明 disclosure 配置层已不再主动挂 schema
  - 说明还存在一条更深的运行时路径，把 `ResponseFormat` 带回了 provider 请求
  - 下一步应继续向 `agentcore.Agent -> ProviderRequest -> chatcompat.buildRequest()` 实际运行时链路收缩定位

### 风险等级
- 中（真实 provider 兼容修复已部分落地，但手工 dry-run 尚未完全走通）

### 审查要求
- L2（真实集成问题修复 + 运行中问题继续定位）

## 2026-07-20: 补充 disclosure 内部 dry-run 准备包（操作清单 + 一键 gate）

### 背景
前序稳定化工作已经完成：

- 启动链路稳固
- disclosure happy-path smoke 已落地
- 审批留痕一致性测试已补齐
- build / race / lint / 任意目录启动已回归通过

下一步不再适合继续堆新功能，而应把“内部彩排”变成一个可直接执行的流程包，供后续真人试用或 P3 前演练复用。

### 变更（2 文件）

1. **新增内部 dry-run 文档（`docs/design/disclosure-internal-dry-run.md`）**
   - 明确内部彩排目标：
     - `analyze -> awaiting_review -> reviewed`
     - 审批留痕可回溯
     - Markdown 导出路径仍可用
   - 提供一套最小操作口径：
     - 启动 `mady serve`
     - `POST /v1/disclosure/analyze`
     - `GET /v1/disclosure/analyze/{task_id}`
     - `POST /v1/disclosure/analyze/{task_id}/review`
   - 区分自动化 gate 与手工彩排：
     - 自动化验证使用 `make test-dry-run-gate`
     - 导出验证统一复用 `make test-disclosure-smoke`
   - 补充完成标准、常见现象与建议节奏，便于后续真人试用前快速复用

2. **新增 dry-run gate（`Makefile`）**
   - 增加 `test-dry-run-gate` 目标
   - 组合执行：
     - `test-disclosure-smoke`
     - `test-approval-audit`
   - `help` 同步展示，作为 disclosure 内部彩排前的统一准入门槛

### 设计要点
- **文档与自动化配套**：不是只写一份操作说明，而是给出一条一键 gate，避免彩排前靠人工记忆流程。
- **优先复用已有验证资产**：导出和留痕不重新造测试，直接复用前两步已经补好的 smoke / audit。
- **服务于真人试用前的“彩排”**：目标是流程可演练，而不是新增产品能力。

### 验证
- `make test-dry-run-gate` 通过

### 影响范围
- `docs/design/disclosure-internal-dry-run.md`
- `Makefile`

### 风险等级
- 低（文档 + make 入口补充，不改产品逻辑）

### 审查要求
- L0（流程资产补充）

## 2026-07-20: 启动体验收尾（入口级 MCP discovery 超时 + eval store 降级初始化）

### 背景
在完成 build / race / lint / 任意目录启动回归后，启动日志仍暴露两个“非阻断但影响体感”的点：

1. **MCP discovery 启动预算偏长**
   - 默认总超时仍是 3 秒，即便 `tui` / `serve` 只是希望“尽快可用、MCP 最佳努力补齐”
   - 用户感知上会把这段等待误认为“启动还没好”

2. **eval.db 初始化告警语义过粗**
   - `runServer` 直接尝试打开 `eval.db`，未显式准备父目录
   - 打开失败时日志只说“评估数据不持久化”，但没有强调“主服务继续可用”

### 变更（5 文件）

1. **MCP discovery 支持 context 级 timeout override（`mcp/config_discovery.go`）**
   - 新增 `mcp.WithDiscoveryTimeout(ctx, timeout)`，允许调用方为单次启动传入 discovery 总超时覆盖值
   - `DiscoverMCPExtensions()` 读取顺序改为：
     - `ctx` override
     - `MADY_MCP_DISCOVERY_TIMEOUT_MS`
     - 默认 `3s`
   - 保留环境变量覆盖能力，但不再强依赖“改进程级 env”来区分入口场景

2. **为 `tui` / `serve` 自动注入更短启动预算（`cmd/mady/framework.go`）**
   - 新增 `withStartupDiscoveryTimeout(ctx)`
   - 当满足以下条件时，自动给 `setupFrameworkContext()` 注入 `1.5s` MCP discovery timeout：
     - 当前子命令是 `tui` 或 `serve`
     - 用户未显式设置 `MADY_MCP_DISCOVERY_TIMEOUT_MS`
   - 这样交互式入口优先“尽快可用”，而不是总是等待完整的 MCP auto-discovery 预算

3. **eval store 初始化抽成 helper（`cmd/mady/server.go`）**
   - 新增 `openEvalStore(evalDB string)`：
     - 先 `EnsureDir(filepath.Dir(evalDB))`
     - 再创建 `knowledge.NewEvalStore(...)`
   - 失败日志改为：
     - 明确指出是 `eval store` 不可用
     - 明确说明“仅禁用评估数据持久化，不影响主服务”

4. **补充 focused test（`mcp/config_discovery_test.go`）**
   - 新增 timeout 解析测试：
     - `ctx` override 优先于 env
     - env 回退生效
     - 默认值回退生效

5. **补充 eval store 初始化测试（`cmd/mady/server_test.go`）**
   - 新增 `TestOpenEvalStore_CreatesParentDir`
   - 验证 nested 父目录不存在时也能成功创建并产出 `eval.db`

### 验证
- `go test -count=1 ./cmd/mady ./mcp` 通过
- `go test -race -count=1 ./cmd/mady` 通过
- `make lint` 通过
- 从临时目录启动 `build/mady serve`：
  - MCP discovery 告警已从 `3s` 降为 `1.5s`
  - eval store 告警改为明确的“降级继续”语义

### 影响范围
- `mcp/config_discovery.go`
- `mcp/config_discovery_test.go`
- `cmd/mady/framework.go`
- `cmd/mady/server.go`
- `cmd/mady/server_test.go`

### 风险等级
- 低（启动期配置与日志优化，未改核心业务状态机）

### 审查要求
- L0（局部启动体验优化，可通过单测与启动日志验证）

## 2026-07-20: 修复 disclosure 状态轮询的并发竞态（深拷贝任务快照）

### 背景
在执行收口回归包时，`make test-race` 首次失败，定位到 `server` 包中新加的 happy-path smoke test：
- 后台 goroutine 在 `disclosureTaskManager.executeTask()` 内更新 `task.Progress` / `task.Result`
- 同时 HTTP 轮询接口 `handleDisclosureStatus()` 在持读锁期间仅做了浅拷贝，然后把共享指针直接交给 `encoding/json`

结果是 JSON 编码在锁外继续遍历 `Progress.NodesDone` 或 `Result` 内部字段时，可能与后台写入并发，触发 race detector。

### 变更（1 文件）

1. **为 disclosure 状态接口构建独立快照（`server/disclosure.go`）**
   - 新增 `cloneDisclosureProgress()`：
     - 深拷贝 `CurrentNode`
     - 复制 `NodesDone` 切片，避免与后台 append 共享底层数组
   - 新增 `cloneAnalysisReport()`：
     - 优先通过 JSON round-trip 深拷贝 `AnalysisReport`
     - round-trip 失败时回退为结构体浅拷贝，保证接口稳态可用
   - `handleDisclosureStatus()` 改为在 `task.mu.RLock()` 内构建响应快照：
     - `Progress` 使用 `cloneDisclosureProgress(task.Progress)`
     - `Result` 使用 `cloneAnalysisReport(task.Result)`
   - 锁外仅负责附加创造性分析结果并写回 HTTP 响应，不再暴露共享可变指针

### 设计要点
- **锁内取快照，锁外编码**：既保留状态接口的并发可读性，也避免把任务内部可变对象直接泄露给 JSON encoder。
- **优先修语义，不扩锁范围**：没有把整个 `writeJSON` 包进锁里，避免把网络输出与锁耦合。
- **以 race gate 驱动修复**：该问题不是纯理论隐患，而是已被 `go test -race` 真实捕获。

### 验证
- `go test -race -count=1 -run TestDisclosureHappyPathSmoke -v ./server` 通过
- `make test-race` 通过
- `make lint` 通过
- 从临时目录启动 `build/mady serve`，成功走完 `setupFrameworkContext()` 并监听 `:8080`

### 影响范围
- `server/disclosure.go`

### 风险等级
- 低（只调整状态轮询的响应快照构造，不改业务状态机）

### 审查要求
- L0（并发安全修复，局部可验证）

## 2026-07-20: 补齐审批留痕一致性测试（TUI / Server / ACP 三入口对齐）

### 背景
当前人工决策留痕已覆盖三条入口：
- TUI `/approve` `/reject` 走 `ApprovalGate.RecordDecision`
- Server disclosure `/review` 端点走 `domains.RecordApprovalDecision`
- ACP 工具授权回调走 `domains.RecordApprovalDecision`

其中 `domains.RecordApprovalDecision` 已提供统一的记录构造逻辑，但此前缺少对 **TUI 入口** 的 focused 测试，导致“三入口是否真的按同一语义落库”主要依赖代码阅读和零散测试，缺少一条可复跑的审计证据链。

### 变更（3 文件）

1. **新增 TUI 留痕测试（`cmd/mady/tui_session_approval_test.go`）**
   - `TestTUISessionRecordApprovalDecision_SoftInterruptUsesReviewTrigger`
     - 通过 `ApprovalGate.AfterModelCall` 造景软中断
     - 验证 TUI 记录使用 `trigger=review`
     - 验证 `OriginalOutput` 来自 gate 缓存的被审输出
     - 验证 `CaseID`、`Decision`、`State`、`ModifiedOutput`、`Feedback` 全部正确持久化
   - `TestTUISessionRecordApprovalDecision_HardInterruptUsesGateData`
     - 用真实 `agentcore.Agent` + 中断工具造景 `agent.Interrupted()`
     - 验证 disclosure 硬中断路径使用 `trigger=disclosure_review`
     - 验证 `OriginalOutput` 同时包含中断 reason 与结构化 `Data`
     - 验证 rejected 决策正确映射到 `StateRejected`

2. **新增一致性回归入口（`Makefile`）**
   - 增加 `test-approval-audit` 目标，统一执行：
     - `./domains`
     - `./server`
     - `./acp`
     - `./cmd/mady`
   - 用于一次性覆盖三条审批/授权留痕路径的 focused 回归。

3. **已有测试矩阵形成三入口闭环**
   - `domains/approval_test.go`：校验统一构造逻辑与状态映射
   - `server/disclosure_review_test.go`：校验 disclosure `/review` 路径
   - `acp/permission_recording_test.go`：校验 ACP 工具授权路径
   - 本次新增的 `cmd/mady/tui_session_approval_test.go` 补齐 TUI 路径，三者组合形成完整一致性证据。

### 设计要点
- **补测试，不改行为**：第三包目标是验证三入口语义一致，而不是重构留痕实现。
- **软/硬中断分开测**：TUI 同时存在 keyword soft-interrupt 与 disclosure hard-interrupt 两条记录路径，必须分别覆盖。
- **复用既有统一入口**：继续以 `domains.RecordApprovalDecision` / `DecisionToState` 为单一真源，避免引入新的构造分叉。

### 影响范围
- `cmd/mady/tui_session_approval_test.go`
- `Makefile`
- 既有 `domains` / `server` / `acp` 测试资产被统一纳入一个 focused 审计入口

### 风险等级
- 低（新增测试与命令入口，不改产品逻辑）

### 审查要求
- L0（测试资产补充，无安全影响）

## 2026-07-20: 新增 disclosure happy-path smoke test（analyze -> review -> export）

### 背景
当前项目已具备 disclosure 异步分析、`awaiting_review` 人工复核和 Markdown/DOCX 导出能力，也已有若干单点测试（如 review 端点校验、导出测试、disclosure e2e）。但缺少一条**轻量、可快速复跑**的最小 happy path 验收链，难以在“走顺流程、稳固基础”阶段持续确认主流程仍然可用。

### 变更（2 文件）

1. **新增 focused smoke test（`server/disclosure_smoke_test.go`）**
   - 新增 `TestDisclosureHappyPathSmoke`，通过 HTTP handler 串起完整最小链路：
     - `POST /v1/disclosure/analyze`
     - 轮询 `GET /v1/disclosure/analyze/{task_id}` 等待 `awaiting_review`
     - `POST /v1/disclosure/analyze/{task_id}/review`
     - 再次轮询状态确认 `reviewed`
     - 对返回报告执行 `disclosure.SaveReport(...md)` 导出
   - 测试内置 `disclosureSmokeProvider`，使用 stub JSON 响应驱动 disclosure Pregel 图，不依赖真实 LLM 或外部知识库。
   - 断言覆盖：
     - analyze 返回非空 `task_id`
     - review 前报告已生成但 `ReviewedByHuman=false`
     - review 后状态推进到 `reviewed` 且审批记录已写入 `ApprovalStore`
     - Markdown 导出成功，且已复核报告不再带“尚未经人工复核”警告

2. **新增 Makefile 入口（`Makefile`）**
   - 增加 `test-disclosure-smoke` 目标，执行：
     - `go test -count=1 -run TestDisclosureHappyPathSmoke ./server`
   - `help` 同步展示该命令，便于本地和后续 CI/人工回归直接复用。

### 设计要点
- **只测主干，不测全部分支**：目标是快速确认 happy path 活着，而不是替代全量 e2e。
- **低依赖**：使用 stub provider + 内存 ApprovalStore，避免真实模型、知识库、pandoc 等外部条件导致冒烟验证不稳定。
- **覆盖 review/export 衔接点**：既验证 server 的 `awaiting_review -> reviewed` 状态机，也顺带验证“复核后的报告”能被导出成最终交付物。

### 影响范围
- `server/disclosure_smoke_test.go`
- `Makefile`

### 风险等级
- 低（新增测试与命令入口，不改产品逻辑）

### 审查要求
- L0（测试资产补充，无安全影响）

## 2026-07-20: 稳固 TUI 启动链路（后台初始化 Agent + 显式状态 + 可调 discovery 超时）

### 背景
前一轮修复已经为 `mady tui` 启动窗口期补上 nil 防御，避免用户在 Agent 未就绪时立即输入导致 panic；但根因仍在：`cmd/mady/tui.go` 中 `app.Start()` 之后仍同步执行 `agentcore.New(s.buildAgentConfig())`，启动尾段依然被 Agent 创建与 MCP discovery 阻塞。结果是首帧虽已渲染，但用户只能得到“请稍候”的被动提示，状态不可见、失败原因不可见，且 `/mode` 等读 `currentAgent` 的路径仍散落裸访问。

### 变更（4 文件）

1. **TUI 启动改为后台初始化（`cmd/mady/tui.go`）**
   - `app.Start()` 成功后不再同步 `agentcore.New(...)`。
   - 改为调用 `tuiSession.initializeAgentAsync()` 在后台创建 Agent；主线程立即进入事件循环。
   - 退出时统一通过 `shutdownAgent()` 取回并关闭当前 Agent，避免与后台初始化并发冲突。

2. **统一 Agent 状态管理（`cmd/mady/tui_session.go`）**
   - 新增 `agentMu`、`agentInitInFlight`、`agentInitErr`、`shuttingDown` 字段，显式表示“初始化中 / 初始化失败 / 关闭中”。
   - 新增 helper：`getCurrentAgent()`、`agentStatus()`、`markAgentInitializing()`、`swapCurrentAgent()`、`shutdownAgent()`、`agentUnavailableMessage()`、`initializeAgentAsync()`。
   - `submitInput()` 与 `resumeIfInterrupted()` 改为先检查状态，再在 `runMu` 临界区内重读当前 Agent，避免 goroutine 持有被 rebuild/close 掉的实例。
   - `handleThinkingCommand`、`handleReviewCommandEx`、`handlePlanCommandEx`、`handleSettingsReset` 统一复用 `rebuildAgent()`，收拢重复的 `agentcore.New + Close + BindAgent` 逻辑。
   - `recordApprovalDecision()` 改为通过 `getCurrentAgent()` 读取中断状态，不再裸读 `s.currentAgent`。

3. **Slash 命令读取显式状态（`cmd/mady/slash_registry.go`）**
   - `/mode` 命令不再只判断 `currentAgent == nil`，而是区分“初始化中 / 初始化失败 / 尚未就绪”三种状态并给出对应提示。

4. **MCP discovery 超时改为可调（`mcp/config_discovery.go`）**
   - 抽取默认 discovery 总超时常量 `defaultDiscoveryTimeout = 3s`。
   - 新增 `MADY_MCP_DISCOVERY_TIMEOUT_MS` 环境变量读取逻辑；当值为正整数毫秒时覆盖默认超时。
   - discovery 超时警告改为引用实际生效的 timeout，便于后续 TUI/Server 按场景调优。

### 设计要点
- **先让首屏真的“活着”**：不是只把 panic 变成提示，而是把 Agent 创建完全移出启动主路径。
- **状态显式化**：启动期不再把“未就绪”与“失败”都折叠成 nil；交互层可以准确反馈当前阶段。
- **访问收口**：把 `currentAgent` 的直接读写收敛到 helper，后续若改成 `atomic.Pointer` 或更细粒度锁，不必全仓扫点替换。
- **向后兼容**：未改变领域配置、审批逻辑、MCP 发现顺序，只调整启动时序与状态暴露。

### 影响范围
- `cmd/mady/tui.go`
- `cmd/mady/tui_session.go`
- `cmd/mady/slash_registry.go`
- `mcp/config_discovery.go`

### 风险等级
- 低到中（主要是 TUI 启动时序调整；不触碰安全敏感路径）

### 审查要求
- L1（启动时序与并发状态管理改动，建议人工过一遍）

## 2026-07-20: 修复 TUI 输入框区域溢出（Flex 容器不裁剪总输出）

### 背景
用户报告 TUI 界面"输入框区域有大量溢出"。静态分析确认根因不在 Editor 本身（`Editor.Render` 已有 `maxRows` 硬截断，默认 8 行），而在装配输入框的垂直 Flex 容器：`tui/layout/flex.go` 的 `renderVertical` 对 Natural 子组件（header/autocomplete/editorFrame/footer/statusBar）按实际渲染行数累加，**从不裁剪总输出到终端高度**。当这些组件行数之和 ≥ 终端 rows 时，Fill 子组件（history）被压到 0，底部输入框与状态栏被挤出可视区域。

### 变更（5 文件，均通过 build/vet/lint(0 issues)/`go test -race ./...` 全量）

1. **新增 SizeShrinkable 布局策略（`tui/layout/layout.go`）**
   - `SizePolicy` 枚举新增 `SizeShrinkable`：取自然高度，但当容器总高度不足时可向下收缩（不低于 `Min`）。
   - 新增构造函数 `Shrinkable(c, min)`。

2. **Flex 按比例收缩 + 安全网裁剪（`tui/layout/flex.go`）**
   - `renderVertical` 第一遍测量 Shrinkable 子组件自然高度并计入 `used`。
   - 新增第三遍：当 `used > totalHeight` 时，收集所有 Shrinkable 子组件的 slack（`size - Min`），按比例分摊 overflow；整数除法残余用贪心逐行修正。每次收缩通过 `OnAllocate` 通知组件新目标高度并重渲染。辅助方法 `reallocateShrinkable(i, newSize, width, rendered, sizes)`。
   - 新增安全网：最终输出若仍超 `totalHeight`（不可压缩组件单独溢出，或 Shrinkable 组件未响应 OnAllocate），从**顶部**截断保留底部（输入框/状态栏为用户焦点），保证永不溢出终端。

3. **editorFrame 改为可收缩（`tui/chat/chat_app_layout.go`）**
   - `chatLayout` 新增 `editorMaxRows int64` 字段（基线行预算）。
   - 新增 `maxRowsSetter` 接口（`SetMaxRows(int64)`），供类型断言解耦。
   - `buildFlex`：①开头重置 editor 到基线 `maxRows`（防止上次收缩值粘连，保证自然高度测量准确）；②`editorFrame` 从 `Natural` 改为 `Shrinkable(ef, 3)`（min 3 = 上下边框 + ≥1 编辑行），`OnAllocate` 回调将 editor `SetMaxRows(h-2)` 以配合重渲染收缩。

4. **传基线配置（`tui/chat/chat_app.go`）**
   - `chatLayout` 构造新增 `editorMaxRows: cfg.EditorMaxRows`（来自 `newChatApp`，默认 8）。

5. **测试（`tui/layout/flex_test.go`）**
   - 新增 `shrinkComp` mock（支持 `SetMaxRows` 运行时截断）。
   - `TestFlexVerticalShrinkable`：单 Shrinkable 按比例收缩 + OnAllocate 被调用 + 总输出 == totalHeight。
   - `TestFlexVerticalShrinkableHitsMinThenSafetyNet`：收缩到 Min 后仍溢出 → 安全网从顶部裁剪，底部输入行可见。
   - `TestFlexVerticalShrinkableProportional`：两 Shrinkable 按 slack 比例分摊。

### 设计要点
- **向后兼容**：`SizeShrinkable` 是新增枚举值，现有 Natural/Fill/Fixed/Min/Max/Percent 行为完全不变；未标记 Shrinkable 的组件不参与收缩。
- **双保险**：主动收缩（editor 响应 OnAllocate 减小 maxRows）+ 兜底安全网（极端情况裁顶部），输入框与状态栏始终可见。
- **不持久化收缩**：`buildFlex` 每次重置基线，避免上一次收缩值污染下一次自然高度测量。
- **Autocomplete 暂未标 Shrinkable**：`component.Autocomplete` 无运行时可见行数控制接口，保持 Natural，由安全网兜底（候选项通常不多且短暂出现）；主要溢出来源（多行 editor 粘贴）已根治。

### 审查后修复（3 处）
- **安全网同步 `rects` 偏移**：安全网从顶部裁剪输出后，`f.rects[i].Row` 同步减去裁剪量，使 `ChildRect` 反映实际屏幕位置。修复 `chatLayout.editorTop`（用于鼠标坐标→editor 行转换）在极端溢出时记录裁剪前位置、导致鼠标点击/选区定位失效的 bug。新增测试断言 editor Row==1、header Row==-2（裁出顶部）。
- **注释勘误**：第三遍注释由"Fill clamped to 0"改为"clamped to their min guard of 1"（第二遍实际下界保护为 1）。
- **`recalcMaxRows` 测量一致性**：抽出 `resetEditorBaseline()` 方法，`buildFlex` 与 `recalcMaxRows` 开头共用，保证两处自然高度测量都基于基线 maxRows 而非上一帧收缩值。

### 影响范围
- TUI 布局层（`tui/layout/`）+ chat 装配层（`tui/chat/chat_app_layout.go`、`chat_app.go`）。
- 不触碰 Agent 运行时/安全敏感路径（敏感路径清单均未命中）。
- 水平布局 `renderHorizontal` 未改动（其高度由 `maxHeight` 决定，不涉及终端高度溢出）。

### 风险等级
- 低（纯 TUI 布局渲染，无安全影响；向后兼容新策略）

### 审查要求
- L0（TUI 布局修复，无安全影响）

## 2026-07-20: 修复 TUI 五处用户体验问题（鼠标选中/复制粘贴/Settings Esc退出/斜杠命令/步数上限）

### 背景
用户报告 TUI 交互存在 5 个体验问题：①MouseMode SGR 的 `?1002h` 按钮事件追踪捕获所有鼠标拖拽，阻止终端原生文本选中；②Cmd+C/Cmd+V 在 Kitty 键盘协议激活时被 TUI 拦截但无对应处理器；③Settings 面板（/settings 打开）无 Escape 退出键；④斜杠命令（如 `/help`）在 autocomplete 激活时因触发符 `"/"` 被重复追加变成 `//help` 而失效；⑤Chat Agent 最大轮次上限为 8（用户感知为"9步"），多工具调用场景经常中断。

### 变更（9 文件，均通过 build/vet/lint(0 issues)/test）

1. **鼠标选中文本（`tui/tui_input.go`）**
   - `enableMouse` 的 SGR 模式从 `?1002h`（button-event tracking）改为 `?1000h`（basic click tracking）。按钮事件追踪捕获所有鼠标拖拽→OS 原生选中失效；基础追踪仅报点击事件，拖拽透传至终端原生处理，用户可正常用鼠标选中文字。补注释说明此举为设计选择而非回退。

2. **Cmd+C/V 复制粘贴（4 文件）**
   - `tui/terminal/keybindings.go`：新增 `"tui.input.paste"` 绑定 → `["super+v", "ctrl+v", "meta+v"]`
   - `tui/component/editor.go`：新增 `onCopy`/`onPaste` 回调字段 + `OnCopy`/`OnPaste` setter
   - `tui/component/editor_edit.go`：`processKeys` 新增 `tui.input.copy`/`tui.input.paste` case，映射到 `handleCopy`（提取编辑器选中文本→onCopy 回调→剪贴板写入）/`handlePaste`（onPaste 回调→剪贴板读取→`insertRune` 逐字符注入）
   - `tui/chat/clipboard.go`：新增 `ReadFromClipboard()` 函数（OS 原生 pbpaste/xclip/powershell） + `readNative()` 内部实现
   - `tui/chat/chat_app.go`：`newChatApp` 中 `editor.OnCopy`/`editor.OnPaste` 绑定到 `CopyToClipboard`/`ReadFromClipboard`

3. **Settings 面板 Esc 退出（2 文件）**
   - `tui/component/settings.go`：新增 `onCancel` 回调 + `OnCancel` setter + `cancel()` 方法；`processKeys` 新增 `tui.select.cancel` case（Escape）
   - `cmd/mady/settings_panel.go`：`openSettings` 中 `settings.OnCancel` 绑定到 `s.app.CloseOverlay(ov)`

4. **斜杠命令双斜杠修复（2 文件）**
   - `tui/component/autocomplete.go`：`applyCurrent` 在 prepend trigger 前先 `strings.HasPrefix(replace, trigger.Trigger())` 检查——防止 `"/" + "/help"` 产生 `"//help"`
   - `tui/component/editor_edit.go`：`processKeys` 新增 `tui.input.tab` case——autocomplete 激活时 Tab 跳过编辑器的默认 `insertRune('\t')`，让 autocomplete 自行处理

5. **Chat Agent 轮次上限（`domains/chat.go`）**
   - `ChatAgentConfig`：`MaxTurns` 截断阈值从 `8` 提升至 `100`（含注释同步），`IntegratedChatConfig` 通过 `ChatAgentConfig(base)` 链式调用自动继承新值

### 影响范围
- TUI 交互层（tui_input.go / editor* / autocomplete / settings / clipboard / keybindings + chat_app + settings_panel），domains 层 1 处配置参数
- 不触碰 Agent 运行时/安全敏感路径（所有安全敏感路径清单均未命中）
- Agent 行为零影响（MaxTurns 仅是客户端配合 DoomLoop 的软限制，非熔断硬限制）

### 风险等级
- 低（TUI 交互与配置参数，无安全影响）

### 审查要求
- L0（TUI 交互修复 + 配置参数调整，无安全影响）

### 背景
用户报告在非项目目录启动 `mady tui` 时界面异常：左侧菜单可见（`▎ Mady`/`📂 会话`/`/cmd 命令中心` 等），右侧大面积空白、无输入框、最右侧一列垂直短线。gemma4-multimodal 看图确认症状。

### 根因
`tui/chat/chat_app_layout.go:140` 用 `layout.Natural(l.sidebar)` 把 sidebar 加入水平 Flex。但 `tui/layout/flex.go:267-271` 的 `renderHorizontal` 对 `SizeNatural` 的处理是 **占用全部父宽度**（代码注释："treat it as the full parent width for now"），导致同级的 `FillWeight(mainFlex, 1)` 被压到最小保护宽度 1 列（`flex.go:353-355 if w < 1 { w = 1 }`）。结果 sidebar 吞掉几乎所有宽度，主区域（header/history/editor/statusBar）被渲染到 **1 列宽**，每行只有 1 个字符——截图里"右侧垂直短线"即主区域 1 列渲染的字符。

触发条件：终端宽度 ≥96 列时启用 sidebar（`chat_app_layout.go:125 useSidebar := l.sidebar != nil && width >= 96`）。`app.SetSidebar(s.buildSidebar())` 在 `tui.go:238` 无条件调用，不分项目/非项目目录（用户感觉"非项目目录才出问题"是巧合——实际取决于终端窗口宽度，项目目录下用户窗口可能 <96 列）。

### 改动（2 文件 + 1 测试，均通过 vet/build/race/lint/test）
- `tui/chat/chat_app_layout.go:140`: `layout.Natural(l.sidebar)` → `layout.Fixed(l.sidebar, sidebarWidth)`。sidebar 固定占 24 列，mainFlex FillWeight 得到 `width - 24` 列。补注释说明 Natural 在水平 Flex 中会占满父宽度的陷阱。
- `tui/layout/flex_test.go`: 新增两个回归测试：
  - `TestFlexHorizontalNaturalStarvesFill`: 记录 gotcha——Natural(sidebar) 占满 100 列，Fill(main) 被压到最小 1 列（min-guard）
  - `TestFlexHorizontalFixedWithFill`: 验证正确用法——Fixed(sidebar,24) + FillWeight(main,1) 得到 24 + 76 的正确分配

### 影响范围
- 仅 TUI 布局层（`tui/chat` + `tui/layout`），不触碰 agentcore/产品逻辑/安全敏感路径。
- 修复所有 ≥96 列终端的 sidebar 布局，不改变 <96 列（无 sidebar）的行为。
- `tui/layout/flex.go` 未改动——Natural 占满宽度的行为是布局引擎既有契约（其他使用 Natural 的地方依赖此语义），本次只在调用点改用 Fixed。

### PTY 验证
200x50 PTY（`/tmp` 目录）启动捕获：sidebar（24 列 `▎ Mady`/`📂 会话`/`⚙ 模式`/`⌨ 快捷操作`）与主区域（header `mady · model=deepseek-v4-flash`、系统消息、`Agent 就绪`）并排正常渲染。

### 风险等级
- 低（TUI 布局修复，1 行产品代码改动 + 回归测试，不触碰安全敏感路径）。

### 审查要求
- L0（TUI 渲染修复，无安全影响）。

---

## 2026-07-20: 修复 TUI 启动窗口期 nil agent panic

### 背景
用户反馈 `mady tui` 启动后立即输入消息会 panic（`SIGSEGV addr=0x318`）。根因是启动序列的时序错位：`cmd/mady/tui.go` 中 `app.Start()` 先于 `agentcore.New(...)` 返回，TUI 事件循环已在另一 goroutine 运行并接收输入，回调 `submitInput` → `agent.Run` 解引用 nil receiver（`s.currentAgent` 尚未赋值）。窗口期实际长度约 3 秒（`buildAgentConfig` 内 MCP discovery 阻塞），用户必现触发。

### 改动（3 文件，均通过 vet/build/race/test/lint）
- `cmd/mady/tui_session.go`: `submitInput` 入口加 nil 防御——`s.currentAgent == nil` 时用 `app.PrintSystem` 提示"Agent 正在初始化，请稍候片刻再发送消息…"并 return，避免 goroutine 内 `agent.Run` nil deref。注释说明窗口期成因。
- `cmd/mady/slash_registry.go`: `/mode` 命令 Handler 同样加 nil 防御（窗口期内执行 slash 命令同样会 panic）。
- `cmd/mady/tui.go`: 启动首条提示从"输入消息开始对话"改为"正在初始化 Agent，请稍候…"，避免误导用户在 Agent 未就绪时输入；`app.Start()` 前补注释说明窗口期与 nil 防御位置。

### 影响范围
- 仅 `cmd/mady/` 三个文件，不触碰 agentcore/agent.go/agent_run.go（panic 现场但非根因）。
- 不改变"先 Start 再 New"的启动序列（保持首帧渲染不被阻塞的策略），只补防御。
- 其他已访问 `s.currentAgent` 的路径已有 nil 检查：`resumeIfInterrupted`（tui_session.go:430）、`tui_session.go:997`；reload/switch 等 `prev := s.currentAgent` 路径为用户主动操作，窗口期内不可达。

### 后续优化（未本次处理）
- MCP discovery 超时 3s（`mcp: discovery timed out after 3s`）导致窗口期过长，可考虑异步化或缩短超时。
- 更彻底的方案是把 agent 创建移到 `app.Start()` 之前，或用 atomic.Pointer 保护 `s.currentAgent` 读写。

### 风险等级
- 低（防御性 nil 检查 + 提示文案调整，未改产品逻辑，不触碰安全敏感路径）。

### 审查要求
- L0（TUI 健壮性修复，无安全影响）。

---

## 2026-07-23: 专利修改规则体系全面集成（Phases 1-3）

### 背景
宝宸知识库 Wiki 中存储了详尽的专利修改规则体系（专利法第33条 + 15条子规则 + 13个典型案例），
但智能体在执行撰写/OA答复/补正/复审/无效等任务时无法系统性地遵循这些规则。
此前项目中仅有 `patent-core.yaml` 中一条笼统的 `patent-a33-amendment` 规则。

### 变更内容

**Phase 1 — YAML 规则文件（3 文件）**
- `domains/rules/data/articles/patent-law-a33.yaml`：**新增**法条框架，三步判断法（依据合法性 → 直接毫无疑义确定 → 修改时机）
- `domains/rules/data/rules/amendment-rules.yaml`：**新增**15条详细修改规则，覆盖修改依据(5)、直接毫无疑义判定(6)、修改时机(3)、背景技术(1)，每规则附带原则、规则和参考案例
- `domains/rules/data/rules/patent-core.yaml`：**修改**已有 `patent-a33-amendment` 规则，增加交叉引用（指向 amendment-rules.yaml 和 validate_amendment 工具）
- `domains/rules/loader.go`：**修复** loadRuleFile 增加了对 `rules/` 子目录的递归读取支持（此前 rules/ 下的文件永远不会被加载）；**修复** `evidence-rules.yaml` 中未加引号的 YAML 映射值导致的反序列化失败

**Phase 2 — validate_amendment 工具（1 文件）**
- `domains/rules/engine.go`：**新增** `validate_amendment` 工具（三步检查：修改依据→A33判定→时机合规），`handleValidateAmendment` 方法，`formatAmendmentAnalysis` 格式化器，`formatArticleShort` 辅助函数，`AmendmentAnalysis`/`AmendmentRuleRef` 值类型
- `domains/rules/types.go`：**新增** `AmendmentRuleRef`、`AmendmentAnalysis` 结构体
- `domains/rules/engine.go`：**修改** `SystemPromptSuffix()` 增加 validate_amendment 工具说明

**Phase 3 — 编排方案 + amendment 模块（5 文件）**
- `domains/rules/data/orchestrations/oa-response.yaml`：**新增**审查意见答复事务编排（4 个发现阶段 + 4 条可用法条 + 5 节执行模板）
- `domains/rules/data/orchestrations/re-examination.yaml`：**新增**专利复审事务编排（4 个发现阶段 + 4 条可用法条 + 5 节执行模板）
- `domains/amendment/doc.go`：**新增**包文档（架构定位、检查流程、使用方式）
- `domains/amendment/types.go`：**新增**值类型（ModType、CheckInput、CheckResult、Violation）
- `domains/amendment/checker.go`：**新增**编译型规则检查器（`AmendmentChecker`），注册 3 条编译型规则（基本输入检查、被动修改OA检查），提供 `FormatCheckResult` 格式化函数
- `domains/amendment/checker_test.go`：**新增**8 个测试用例（无修改/有修改/被动无OA/被动有OA/无原始文件/空输入/说明书修改/格式化输出）

### 架构决策
1. **YAML 为主 + 编译型为辅**：修改超范围判断天然需要'本领域技术人员'视角，YAML 规则通过 LLM 判断最适合；编译型规则（`domains/amendment/`）负责边界明确的规则
2. **规则从不加载到能加载**：`loader.go` 原只从 `data/` 顶层读取规则 YAML，`rules/` 子目录下已有文件（patent-core.yaml、evidence-rules.yaml）从未被加载。新增 `rules/` 子目录读取 + Rule UnmarshalYAML 收集额外字段
3. **OA答复/复审作为独立编排方案**：与已有的 invalidation（无效宣告）编排方案并列，形成覆盖专利全生命周期的编排矩阵（4种事务类型）

### 影响范围
- 新增 9 个 YAML/Go 文件，修改 4 个既有文件
- 不触碰任何安全敏感路径
- 向后兼容：现有 `patent-a33-amendment` 规则保留并增强

### 验证
- `go build ./...` — 通过（根模块 + tools 子模块）
- `go test ./domains/rules/... ./domains/amendment/...` — 全部通过（33 tests）
- `make all` — 通过（vet + build + test）
- YAML 文件验证：9 个文件全部有效
- 真实数据加载验证：40 条规则 / 4 篇法条框架 / 2 个编排方案全部被加载



### 背景
远程 CI 自 `b52de24` 之后连续 8 次失败（`fix(agentcore): 深度审阅全面修复...` 起）。根因：`disclosure/e2e_docx_flow_test.go::Test_DOCX_to_Report_FullFlow` 第 7 步无条件调用 `SaveReport(report, "*.docx")`，其底层 `convertToDOCX` 依赖外部 `pandoc` 可执行文件；GitHub Actions 的 `macos-latest` runner 默认未安装 pandoc，`exec.Command("pandoc", ...)` 返回 `exec: "pandoc": executable file not found in $PATH`，整个测试 FAIL。`build-and-test` 矩阵的 `fail-fast` 默认策略随后取消 `ubuntu-latest/root` 作业，导致全红。

### 改动（1 文件，均通过 vet/race/lint/test）
- `disclosure/e2e_docx_flow_test.go`: 新增 `os/exec` import；DOCX 导出段（原 244-250 行）改为先 `exec.LookPath("pandoc")` 判定——不可用时 `t.Logf` 跳过、不触发 `t.Fatalf`，与同包 `export_test.go:110-114` 中 `TestSaveReport_DOCX` 的兼容写法一致；其余 MD 导出/正文断言逻辑完全保留。

### 影响范围
- 仅 `disclosure` 测试代码，不涉及导出/产品逻辑（`disclosure/export.go` 未动）。
- CI 侧无配套依赖变更（仍走 `actions/setup-go@v6`，未在 workflow 里 brew install pandoc）：本地有 pandoc 时仍验证完整链路，CI 无 pandoc 时只跳过 DOCX 导出子步骤。

### 风险等级
- 低（仅测试跳过逻辑，未改产品代码，不触碰安全敏感路径）。

### 审查要求
- L0（测试兼容性修复，无安全影响）。

---

## 2026-07-20: agentcore 深度审阅全面修复（43 项：2C/8H/18M/25L 全清）

### 背景
承接 agentcore 深度审阅报告（`docs/review/agentcore-deep-review-2026-07-20.md`），对全部 43 项发现按模块分 16 组修复。

### 改动（均通过 build/vet/lint(0 issues)/race/test）

**P0 安全熔断（planmode/pipeline/handoff/filecheckpoint/doomloop）**
- `planmode/readonly.go` 重写：移除 python/node/go/ruby/cargo 解释器的 readOnly 放行（`-c`/`-e` 执行任意代码绕过）；awk/sed fail-closed（内部 `>`/`w` 重定向绕过 `strings.Contains(">")` 检测）；go 改 subcommand 白名单（test/vet/list 放行，run/build 阻止）；新增引号感知的 stripQuoted/splitCommandChain 替换字符串 Contains 误判 [安全敏感]
- `pipeline_executor.go`: 抽取 executeStage 包 defer recover，StageHandler panic 不再拖垮 doomloop（参考 executor.go:320）
- `handoff.go`: executeDelegate/handleTransfer 加 DepthFromContext 限深（≥DefaultMaxDelegationDepth=8 返回 ErrDepthExceeded）+ WithDepth(ctx,+1)，防互相 delegate 栈溢出 [安全敏感]
- `filecheckpoint/store.go`: SnapshotFile/restoreFile 入口加 isPathSafe 校验；RestoreAndTrim 重写为全程持锁原子（消除并发丢 checkpoint）；extension.go bash 重定向路径提取 + BeforeTurn off-by-one 修复 [安全敏感]
- `doomloop/doomloop.go`: detector 遍历改为持 mu（OnSignal 锁外回调）；删除 totalToolCalls 死字段；新增 SignalError 结构化错误 + errors.As [安全敏感]

**P0 数据完整性 + 健壮性**
- `context_engine_tiered.go`: snipToolResults 改 []rune 切片（中文 UTF-8 不断裂）
- `compaction.go`: summary 失败时保留原文不再 ReplaceMessages 丢数据；删除 findCutPoint 死函数
- `evaluate/cli/cli.go`: runLive goroutine 加 defer recover；删除 any 类型擦除，新增 Completer 接口 + callProviderSimple 用具体 ProviderRequest/Response 类型（不再 fmt.Sprintf("%v") 把结构体格式化）
- `extension.go`: Register 第 N 个 Init 失败时逆序 Dispose 已成功者
- `budget.go`: AfterModelCall 累加后检查超限并 fireExceed 告警
- `reasoning_strategy.go`: 注入 hint 改用真克隆 slice（兑现"不原地修改"注释承诺）

**P1 一致性 + 健壮性**
- `tool_gen.go`: schemaCache 始终缓存 strict 版（与调用顺序无关）；tryCoerceValue int 字段 ParseInt 优先
- `plugin_manager.go`: retriever 注入前 copy input（不污染调用方 map）
- `permission.go`: Mode==Deny 时 readOnly 降级 Ask（不再自动放行）[安全敏感]
- `evidence`: 删除 BeforeModelCall 死代码；Ledger 改 RWMutex
- `evaluate`: extractJSONArrays 括号配平迭代（支持多块/字符串感知）；llm_judge Samples 有界并发；benchmark init 失败 panic（CI 不再假绿）；sortedMetricNames 加排序

**P2 死代码清理**
- 删除 NewHandoffError/NewGuardrailError（全仓零调用）；orchestrate Publish 加 DroppedMessages 计数；tracing/otel.go 删不可达分支

### 影响范围
agentcore/ 全子树（planmode/pipeline/handoff/filecheckpoint/doomloop/context_engine/compaction/evaluate/extension/budget/reasoning_strategy/tool_gen/plugin_manager/permission/evidence/orchestrate/tracing/errors）

### 风险等级
- 中（含 5 处安全敏感路径修复：planmode/handoff/filecheckpoint/doomloop/permission，需人工审阅）

### 审查要求
- L2（安全敏感路径：planmode/readonly.go、handoff.go、filecheckpoint/store.go、doomloop/doomloop.go、permission/permission.go）

### 验证
- `go build ./...` ✅ | `go vet ./...` ✅ | `golangci-lint run ./agentcore/...` 0 issues ✅ | `go test -race ./agentcore/...` 全绿 ✅ | 根模块 build/vet ✅


## 2026-07-20: agentcore 深度审阅（182 文件全量，2C/8H/18M/25L）

### 背景
对 agentcore/ 全量子树（基线 `bda2694`..`9f9846b`，增量 +10309/-679 行，约占 agentcore 1/3）进行逐文件精读质量评估，采用 8 阶段方法（基线验证→自动化扫描→历史回归→核心循环回归→三路并行精读→安全并发专项→汇总）。

### 产物
- **新增 `docs/review/agentcore-deep-review-2026-07-20.md`**：结构化审阅报告（执行摘要/基线/历史回归矩阵/按严重度清单/按模块发现/修复路线图 P0-P3/质量趋势/覆盖矩阵）

### 关键发现
- **整体评级：良好（B+）**。`golangci-lint` 0 issues、`go test -race` 全绿、vet 通过、TODO 清零；核心循环重构（agent_run.go 拆分 runLoop/runInnerLoop + failLoop 提取）质量高；hooks.go 修复 time.After timer 泄漏
- **2 Critical**：planmode 只读判定被 python/node/awk 等解释器绕过（`planmode/readonly.go:27-33`）；pipeline StageHandler.Execute 无 panic recover（`pipeline_executor.go:96`，对比 executor.go:320 工具路径有 recover）
- **8 High**：handoff 无递归限深（互相 delegate→栈溢出）、filecheckpoint isWithinRoot 死代码+零路径校验、doomloop detector 隐式串行假设、TieredEngine 中文 UTF-8 截断、evaluate/cli runLive 损坏且 0% 覆盖、extension.Register 部分失败泄漏、compaction summary 失败丢消息、planmode awk/sed 内部重定向绕过
- **历史回归**：P2-4(doc.go)/C5-C6(stream泄漏)/manifest GuardrailLevel/P0-13(context.Background 25→8处) **全部通过**；P0-11 部分残留（NewHandoffError/NewGuardrailError 全仓零调用）
- **未修改任何代码**，仅产出审阅报告。所有 Critical/High 经主会话亲自核实。修复路线图见报告第 7 节

## 2026-07-20: 创造性分析完全独立节点——三步法 Pregel 子图 + EventBus 异步触发

### 背景
disclosure 管线（技术交底书分析）完成新颖性初判后，**创造性评估完全缺失**。
原 `domains/reasoning/multi_hypothesis.go` 的多假设推理引擎是通用组件，
无法直接对接专利三步法（最接近现有技术→区别特征→技术启示）的结构化流程。

用户明确要求三条约束：
1. P0 选 B：Pipeline Executor 优先实现（已完成）
2. P1→P2 按顺序执行：先管线可运行，再评估消费，最后创造性分析
3. 创造性分析**完全独立**：不嵌入 disclosure 管线代码，通过 EventBus 事件驱动

### 变更

#### P2.1: 创造性分析 Pregel 子图（`domains/inventiveness/`）
- **新增 `domains/inventiveness/graph.go`**：独立的 Pregel 子图，不依赖 disclosure 包
  - 图拓扑：`load_input → step1_closest_prior_art → step2_distinguishing_features → step3_technical_suggestion → generate_conclusion → __end__`
  - 三步法严格对齐《专利审查指南》第二部分第三章 3.2.1.1：
    - Step 1：从现有技术证据中确定最接近的对比文件（Temperature=0.2，确定性任务）
    - Step 2：列举区别特征 + 重新确定实际解决的技术问题（Temperature=0.2）
    - Step 3：判断是否存在技术启示——多假设推理典型场景（Temperature=0.2）
  - 每步均为独立 LLM Agent 节点，输出结构化 JSON
  - `EvidenceCoverage == "none"` 时跳过全部步骤，设置 `Skipped=true`
- 定义本地值类型（`InventivenessInput`/`InventivenessResult`/`ThreeStepResult`/`EvidenceChunk`/`TechFeature`/`PFETriple`）避免 disclosure 包依赖——disclosure 有同名类型但独立维护

#### P2.2: EventBus 触发器（`server/disclosure_events.go`）
- **新增 `server/disclosure_events.go`**：
  - `DisclosureCompletedEvent`：实现 `agentcore.Event` 接口，由 `disclosureTaskManager` 在任务完成后自动发射
  - `InventivenessTrigger`：事件消费者，订阅 `disclosure_completed` 事件
    - 筛选条件：Report != nil && Err == "" && 有提取数据
    - 执行流程：构建 `InventivenessInput`（含特征/PFE三角/新颖性结论）→ 运行 Pregel 子图 → 通过回调储存结果
    - 异步、容错：子图失败仅记日志，不影响上游 disclosure 管线
    - 支持 `WithInventivenessResultHandler` 选项注入结果回调
- **修改 `server/disclosure.go`**：
  - `disclosureTaskManager` 新增 `eventBus` 字段
  - `newDisclosureTaskManager(eventBus)` 构造函数参数化
  - `initDisclosureManager` 传递 `s.eventBus`
  - `executeTask` 完成后调用 `emitCompleted` 发射事件
  - `DisclosureTaskStatus` 新增 `Inventiveness` 字段

#### P2.3: 结果汇总到 API
- **修改 `server/server.go`**：
  - `Server` 新增 `inventivenessResults` map（`*csync.Map[string, *inventiveness.InventivenessResult]`）
  - 新增 `SetInventivenessResult` / `GetInventivenessResult` 方法
  - 新增 `github.com/xujian519/mady/domains/inventiveness` 导入
- **修改 `server/disclosure.go`**：
  - `handleDisclosureStatus` 查询 `GetInventivenessResult` 并附加到响应
- **修改 `cmd/mady/server.go`**：
  - 在 `runServer` 中创建并启动 `server.NewInventivenessTrigger`
  - 注入 `srv.SetInventivenessResult` 作为结果回调

### 架构决策
- **EventBus 解耦而非图内嵌入**：将 inventiveness 分析与 disclosure 管线彻底分离。disclosure 管线结束时仅发射事件，不关心谁消费。触发器和子图可以独立测试、独立替换、或在不同部署环境中选择性启用。
- **值类型复制而非包依赖**：`domains/inventiveness` 定义独立的 `EvidenceChunk`/`TechFeature`/`PFETriple` 类型（与 disclosure 包的对应类型字段完全一致）。避免 disclosure→inventiveness 或 inventiveness→disclosure 的单向依赖。两者之间通过 server 包的桥接函数转换。
- **结果回调模式**：`InventivenessTrigger` 不直接持有 `*Server` 引用，通过 `WithInventivenessResultHandler` 回调注入。保持触发器可测试（注入 mock handler）且与 server 包的解耦。
- **`csync.Map` 而非 `sync.Map`**：复用既有并发安全 map 实现，与 disclosure 任务管理器一致的模式。

### 文件清单

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `domains/inventiveness/graph.go` | 新增 | 三步法 Pregel 子图（5 节点 + 5 种值类型） |
| `server/disclosure_events.go` | 新增 | DisclosureCompletedEvent + InventivenessTrigger |
| `server/disclosure.go` | 修改 | eventBus 集成 + emitCompleted + result 扩展 |
| `server/server.go` | 修改 | inventivenessResults map + Set/Get 方法 |
| `cmd/mady/server.go` | 修改 | 触发器装配 |
| `docs/decisions/AI_CHANGELOG.md` | 修改 | 本记录 |

### 验证
- `go build ./server/... ./domains/inventiveness/... ./cmd/mady/...` ✅
- `go build ./...` ✅（全项目无回归）
- `go vet ./server/... ./domains/inventiveness/...` ✅
- `go test -race -count=1 ./server/...` ✅（3.2s）
- `go test -race -count=1 ./agentcore/...` ✅

---

## 2026-07-20: EvalHook 数据接消费端——评估指标从"发了没人看"到可查询、可告警

### 背景
`knowledge/eval.go` 的 `EvalHook` 在 `AfterModelCall` 后发送 `evalResultEvent` 到事件总线，但 **没有任何消费者**。三个评估指标（Faithfulness、AnswerRelevancy、ContextPrecision）被计算、被发射、被遗忘——这是之前 `search_knowledge` 工具被注册但无人知晓调用的同模式第三次重演。

### 变更

#### 1. EvalStore 持久化层
- **新增 `knowledge/eval_store.go`**：`EvalStore` 读写独立 `eval.db` SQLite 数据库。自动迁移创建 `eval_results` 表（含 turn、question、answer、faithfulness 等字段 + created_at/faithfulness 索引）。提供 `Save`、`QueryByThreshold`、`QueryStats` 三个方法。
- **新增 `knowledge/eval_store_test.go`**：5 个测试覆盖 Save/Query round-trip、多行统计、空表统计、duration 解析、默认配置验证。

#### 2. EvalConsumer 事件消费者
- **新增 `knowledge/eval_consumer.go`**：`EvalConsumer` 实现 `agentcore.EventHandler`（回调式，直接注册到 `EventBus.OnAll`），过滤 `eval_result` 事件后执行：①持久化到 SQLite ②阈值检查（< AlertThreshold 打印 Warn）③极低忠实度（< 0.4）打印 Error 告警。

#### 3. 配置扩展
- **`knowledge/eval.go`**：`EvalConfig` 新增 `AlertThreshold`（默认 0.6）和 `AlertAction`（默认 "log"），`DefaultEvalConfig` 同步更新。

#### 4. 服务器集成
- **`cmd/mady/server.go`**：启动时打开 `~/.mady/eval.db`，创建 `EvalConsumer`，通过 `srv.OnAll(consumer.OnEvent)` 注册到 Server 事件总线。
- **`server/server.go`**：新增 `EventBus()` getter 方法。

#### 5. mady eval baseline CLI
- **`cmd/mady/eval.go`**：新增 `runEvalBaseline` 子命令，读取 eval.db 输出基线统计：总评估数、平均忠实度/相关度/精度、低忠实度率，并展示前 5 条低忠实度示例。与现有 `mady eval` 评估套件共用入口（`mady eval baseline [--since YYYY-MM-DD] [--until YYYY-MM-DD]`）。

### 架构决策
- **独立 eval.db**：不与 knowledge.db 共享数据库，避免 schema 耦合和迁移复杂性。eval.db 仅用于 EvalResult 持久化，生命周期独立于知识库。
- **回调式 EventHandler**：使用 `EventBus.OnAll`（同步回调）而非 `Subscribe`（channel），避免 goroutine 泄漏风险。回调式在事件量不大时（每次模型调用一次）性能足够。

### 验证
- `go test -race ./knowledge/...` ✅
- `go build ./cmd/mady/...` ✅
- `go build ./...` ✅
- `mady eval baseline` 可执行并输出正确格式

---

## 2026-07-20: Pipeline Executor 实现——插件系统真正可运行

### 背景
外部智能体审阅发现 `plugins/patent/novelty-analysis/`、`infringement-check/`、`oa-response/` 三个插件目录包含 `plugin.json` + `SKILL.md`（定义了完整的 pipeline 阶段），但 `agentcore.ScanPlugins` 仅被测试代码调用，**没有任何生产代码加载和执行它们**。这些插件处于"看起来已实现但从未运行"的危险状态。

### 变更

#### 1. Pipeline Executor 系统（P0）
- **新增 `agentcore/pipeline_handler.go`**：`StageHandler` 接口（`Name()` + `Execute()`）+ `StageHandlerRegistry`（类似 Atom 注册表模式）+ `PipelineState` 类型（key-value state）+ `StageError`/`InterruptStageError` 错误类型
- **新增 `agentcore/pipeline_executor.go`**：`PipelineExecutor` 编排器，按 `PluginManifest.Pipeline.Stages` 顺序遍历，Atom 阶段派发到 `StageHandlerRegistry`，Tool 阶段标记跳过。支持 `WithFailOnUnknown` 配置化。`finalizeState` 确保中断时仍写入执行元数据
- **新增 `agentcore/pipeline_stage_handlers.go`**：5 个内建 `StageHandler`：
  - `searchHandler`：通过 `Retriever` 接口（本地类型，解耦 `retrieval/domain` 循环依赖）执行 FTS5 检索
  - `extractHandler`：LLM Agent 结构化提取（JSON Schema 输出），支持 features/problems/effects 三种类型
  - `compareHandler`：LLM Agent 逐特征对比（单独对比原则），生成 claim_chart + diff_features
  - `reasoningHandler`：通用 LLM 推理（自由文本输出），自动从 state 构建上下文
  - `approvalGateHandler`：返回 `InterruptStageError` 触发人机交互（镜像 disclosure review_gate 模式）
- **新增 `agentcore/plugin_manager.go`**：`PluginManager` 封装插件发现 + 执行 + 提供 `RunPluginTool()`（注册 `run_plugin` Agent 工具）
- **新增 `agentcore/pipeline_executor_test.go`**：13 个单元测试覆盖：空 pipeline、未知 atom、FailOnUnknown、state 隔离、approval-gate 中断、tool stage 跳过、reasoning/extract handler、多阶段 pipeline、manifest 元数据、Handler 注册表查询、PipelineState getter/setter、JSON 提取

#### 2. cmd/mady 框架集成
- **`cmd/mady/framework.go`**：新增 `pluginToolExtension` 类型（单工具 Extension 适配器）+ 在 `setupFrameworkContext` 中扫描 `plugins/` 目录并加载插件管理器；发现插件时注册 `run_plugin` 工具到 BaseConfig

### 架构决策
- **StageHandler 与 Atom 分离**：`Atom` 接口保持纯 schema 契约（无 Execute），`StageHandler` 是独立的运行时执行器接口。这允许未来切换执行模型而不影响 Atom 定义。
- **本地 `Retriever` 接口**：`pipeline_stage_handlers.go` 定义 `Retriever`/`RetrieverQuery`/`RetrieverResults`/`RetrieverDocument` 本地类型，避免 `agentcore → retrieval/domain → knowledge → agentcore` 导入循环。
- **`repoDir/plugins/` + `~/.mady/plugins/` 双路径搜索**：支持开发时本地插件和部署后用户插件共存。
- **Tool-based stages 标记跳过**：Tool 阶段（如 `draft-amendments` 的 `write_file`）尚未实现，当前跳过并记录 warning。计划 P2 阶段接入工具框架。

### 验证
- `go build ./agentcore/...` ✅
- `go test -race ./agentcore/...` ✅（13 新增测试 + 所有既有测试）
- `go build ./cmd/mady/...` ✅
- `go vet ./agentcore/... ./cmd/mady/...` ✅
- `go build ./...` ✅（全项目无回归）

---

## 2026-07-19: 四大证据源全面接线修复

### 背景
此前对编排层地基和证据接入点的分析发现三个结构性问题：
1. disclosure 分析管线的 `retrieve_prior_art` 节点从未接入 `PatentDomainRetriever`，新颖性评估降级为纯 LLM 自身知识
2. `scholar_search`（学术论文检索）和 `search_knowledge`（知识库检索）虽然在 tools 层注册，但无 Agent 的 SystemPrompt 提及，LLM 不知道何时调用
3. `BuildProjectAgent` 案件模式搜索工具依赖 base extension 隐式提供，缺少显式声明

### 变更

#### 1. disclosure 管线接入 PatentDomainRetriever（P0）
- **`server/server.go`**：新增 `disclosureRetriever atomic.Pointer[domain.DomainRetriever]` 字段 + `SetDisclosureRetriever`/`getDisclosureRetriever` 方法
- **`server/disclosure.go`**：`submitTask`/`executeTask` 沿参数链传递 `domain.DomainRetriever`；`executeTask` 调用 `BuildDisclosureAnalysisGraphWithOpts(provider, WithRetriever(retriever))` 替代原 `BuildDisclosureAnalysisGraph(provider)`
- **`cmd/mady/server.go`**：从 `fc.KnowledgeBackend` 类型断言为 `*knowledge/sqlite.SQLiteStore`，构建 `retrieval/domain/sqlite.PatentDomainRetriever` 并注入 Server

#### 2. SystemPrompt 补充搜索工具指引（P1）
- **`domains/assistant.go`**：职责列表新增 `scholar_search` 和 `search_knowledge / search_laws`
- **`domains/patent.go`**：五步工作法第 2 步新增 `scholar_search` 和 `search_knowledge / search_laws`；第 4 步显式提到 `patent_lookup`
- **`domains/legal.go`**：可用工具列表新增 `scholar_search` 和 `search_knowledge / search_laws`，区分互联网检索与本地知识库检索
- **`domains/chat.go`**：IntegratedChatConfig 的路由描述中补充学术论文检索

#### 3. BuildProjectAgent 搜索工具探究
经代码追踪，`BuildProjectAgent` 的 Agent 实例已通过 `setupFrameworkContext` 的 `baseTools` extension 获得 `web_search`/`web_fetch`/`scholar_search`/`patent_lookup` 等搜索工具（base extension 的 `BuildTools` 无 `EnabledTools` 过滤），无需额外修改。

### 架构决策
- **disclosure 图构建模式从单参扩展为可选 option 链**：`BuildDisclosureAnalysisGraph(provider)` 仍然保持向后兼容（降级为无检索器），新代码路径调用 `BuildDisclosureAnalysisGraphWithOpts(provider, WithRetriever(...))` 激活检索
- **PatentDomainRetriever 依赖注入路径**：`frameworkContext.KnowledgeBackend` → 类型断言 `*SQLiteStore` → `NewPatentDomainRetriever(store)`，复用已在 `loadWikiStore` 中打开的数据库连接，避免二次打开
- **evidence_coverage 传递链路**：`retrieve_prior_art` 节点检测到 retriever != nil 后执行真实 FTS5 检索 → `evidence_coverage` 设为 `"patent_database"` → `check_novelty` 节点据此注入证据上下文至 LLM、而非纯 LLM 知识降级

### 验证
- `go build ./server/... ./cmd/mady/... ./domains/... ./disclosure/...` ✅
- `go test ./disclosure/... ./server/... ./domains/...` ✅
- `go test ./tools/... ./knowledge/... ./retrieval/... ./agentcore/...` ✅

---

## 2026-07-19: A2UI 模块审阅与完备化

### 背景
对 a2ui 包（Agent-to-UI 协议 v0.9.1 实现）进行"完整可运行"审阅，发现：
- 模块自身质量极高（100% 测试覆盖率、零 vet 警告），但 **完全未被运行时集成**（零外部调用者）
- `agui/converter.go` 默认分支错误地传递了完整 A2UIEvent 结构体而非仅信封数据

### 变更

#### 1. 模块自身改进（P1）
- **静默吞错修复**：`UpdateDataModel.UnmarshalJSON` 丢弃 `json.Unmarshal` 错误 → 显式检查
- **补齐 Builder 构造函数**：新增 `Video`、`AudioPlayer`、`Tabs`、`Modal`、`DateTimeInput`、`ChoicePicker` 6 个便捷构造函数
- **新增 `NewCheck` 构造函数**：简化客户端校验声明
- **100% 覆盖率保持**：新增 20+ 测试

#### 2. 运行时集成（P2）
- **新增 `agentcore.A2UIEvent` 事件类型**（EventType = "a2ui"）：承载序列化的 A2UI 信封，避免循环依赖
- **新增 `a2ui.ToAgentCoreEvent()` 桥接函数**：将 Envelope 转为 EventBus 兼容事件
- **修复 `agui/converter.go` 默认分支**：添加 `case *agentcore.A2UIEvent`，传递 `ev.Envelope` 而非整个结构体，保留原始时间戳
- **消除冗余序列化**：`ToAgentCoreEvent` 复用已有的 `envelopeToMap` 函数

#### 3. 测试完备化
- **agentcore A2UIEvent 单元测试**：创建、事件总线发射与接收
- **agui 转换器 A2UIEvent 测试**：验证 CUSTOM 事件名称/值正确性
- **端到端 SSE 流测试**（`TestE2E_A2UIStream`）：通过生命周期钩子发射 A2UIEvent，验证其在 AG-UI SSE 流中呈现为 `CUSTOM (name: "a2ui")` 事件

#### 4. 示例程序（P3）
- **新增 `example/a2ui-demo/`**：演示 Builder API、数据绑定、AG-UI/A2A 传输绑定

### 架构决策
- **事件桥接模式**：a2ui→agui 存在单向依赖（a2ui 导入 agui 的 CustomEvent 类型），agui 反向导入会形成循环依赖。采用 agentcore.A2UIEvent 作为中间事件类型，agui 转换器仅需识别事件名 "a2ui"，不依赖 a2ui 包
- **envelopeToMap 序列化**：将结构体转为泛型 map 后通过事件总线传输，保持 agentcore 对 a2ui 类型的零感知

### 验证
- `go build ./...` ✅
- `go test ./a2ui/...` ✅（100% 覆盖率）
- `go test ./agui/...` ✅（含新 A2UI e2e 测试）
- `go test ./agentcore/...` ✅（含新 A2UIEvent 测试）
- `go run ./example/a2ui-demo/` ✅

---

## 2026-07-21: 修复 pre-commit 钩子失效问题

### 背景
执行 `make verify` 或 `git commit` 时，golangci-lint hook 抛出 panic：
`package requires newer Go version go1.26 (application built with go1.25)`，
导致本地 pre-commit 门禁全线失效，开发者绕过 lint 检查提交代码。

### 根因
`$(go env GOPATH)/bin/golangci-lint`（v2.12.2）由 Go 1.25.12 构建，无法解析 Go 1.26 新增语法。
pre-commit 包装脚本 `scripts/precommit-golangci-lint.sh` 优先使用 GOPATH/bin 路径，
Homebrew 版本（Go 1.26.2 构建）正确但不被选中。

### 变更（4 文件，均通过 build/vet/lint/test-race/`pre-commit run --all-files`）

1. **修复 golangci-lint 版本兼容性**
   - 删除 GOPATH/bin 下旧版 golangci-lint（Go 1.25.12 构建），
     通过 `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
     重新安装（Go 1.26.5 构建），与系统 Go 版本匹配

2. **`graph/node_policy.go:27` — 修复 gocritic `deprecatedComment`**
   - `Deprecated:` 注释与正文之间补空行，满足 gocritic 规范要求

3. **`graph/node_policy_test.go:212` — 修复 misspell `cancelled`**
   - `cancelled` → `canceled`（美式拼写，与项目 lint 规范一致）

### 验证
- `make verify` ✅（lint + build + test-race，根模块 + tools 子模块全部通过）
- `pre-commit run --all-files` ✅（10 个 hook 全部绿色）
- `pre-commit run --hook-stage commit-msg` ✅（sensitive-paths gate + commitlint 正常）

---

# AI 决策变更日志

> 本文件记录 AI 协助开发过程中的关键决策。新决策追加在本文件顶部。
> 历史决策（2025-06 ~ 2026-07-17，共 108 条）
> 已归档至 `docs/decisions/archive/`，见下方[归档索引](#归档索引)。

---

## 归档索引

| 归档文件 | 时段 | 章节数 | 主要主题 |
|----------|------|--------|----------|
| [`2025-06-initial-review.md`](archive/2025-06-initial-review.md) | 2025-06-11 | 1 | 2025-06 初始代码质量审查 |
| [`2026-07-w11-handoff-docs.md`](archive/2026-07-w11-handoff-docs.md) | 2026-07-11 ~ 2026-07-12 | 12 | 2026-07-11~12 文档同步与 Invisible Handoff |
| [`2026-07-w13-eval-doomloop.md`](archive/2026-07-w13-eval-doomloop.md) | 2026-07-13 ~ 2026-07-14 | 45 | 2026-07-13~14 评估框架增强与 DoomLoop |
| [`2026-07-w15-citation-gate.md`](archive/2026-07-w15-citation-gate.md) | 2026-07-15 ~ 2026-07-17 | 50 | 2026-07-15~17 引用核验 Gate 实现 |

---

## 2026-07-19: 里程碑 4——原路线图剩余项收尾

### 背景
用户核对原路线图后发现 Sprint 1-3 的 10 项任务中有 5 项未执行。本里程碑完成剩余项。

### 变更

#### 1. AI_CHANGELOG.md 归档化（原 Sprint 3 #9）
- 主文件从 3154 行 / 265KB 降到 1383 行 / 77KB（**-57%**）
- 108 个历史章节（2025-06 ~ 2026-07-17）归档到 `docs/decisions/archive/` 下 4 个文件
- 主文件保留 07-18~07-19 近期决策 + 归档索引表

#### 2. Deprecated API 清理（原 Sprint 3 #10）
清理了 2 处**真正无调用方的死代码**：
- `tui/theme/style.go`：删除 14 个 deprecated 全局 Style 变量及 palette.go 同步代码
- `domains/reasoning/fact_blackboard.go`：删除 deprecated `SetPlan` 方法

**保留** 5 处仍被生产代码使用的 deprecated 字段（需先完成钩子迁移才能删除）。

#### 3. log → slog 迁移（原 Sprint 2 #5）
- 制定迁移方案文档 `docs/decisions/log-to-slog-migration-plan.md`
- 执行阶段 1+2：agentcore/tui-theme/memory/domains/knowledge/server/mcp/tools（14 文件，53 处）
- **非入口包 log.Printf 从 53 处降到 0 处**
- cmd/mady（27 处）按方案保留（入口包 Go 惯用法）

#### 4. 拆分 agent_run.go runLoop（原 Sprint 2 #6）
- `runLoop` 从 280 行 → 36 行（**-87%**）
- 提取 `runInnerLoop` 方法（254 行），行为完全等价
- 9 个 TestAgentRun* 测试 + race detector 全过

#### 5. browser_providers 覆盖率提升（原 Sprint 3 #8）
- 新增 27 个测试，**覆盖率从 4.4% 提升到 50.3%**
- 网络依赖测试用 `testing.Short()` 守护

### 验证
- `make all` ✅（根 + tools 子模块）
- `go test -race ./agentcore/...` ✅
- `pre-commit run --all-files` ✅

### 原路线图执行状态
所有 10 项任务现已全部处理完毕。

---

## 2026-07-19: 里程碑 3——超大函数拆分 + fmt.Errorf 债务复核

### 背景
原路线图里程碑 3 包含三项：
1. 拆分 `tools/browser_legacy.go:NewBrowserNavigateTool`（288 行）
2. 拆分 `agentcore/compaction.go:runCompaction`（235 行）
3. `fmt.Errorf` 分批加 `%w`（审查报告称 450 处未 wrap）

经深入分析，**第 2、3 项均不成立**，详见下文勘误。仅执行第 1 项。

### 变更：拆分 NewBrowserNavigateTool

#### 问题
`NewBrowserNavigateTool` 是返回 `*agentcore.Tool` 的工厂函数，核心是一个 288 行
的闭包（`Func` 字段）。闭包内 `switch session.backendType` 有两个巨大 case：
- `BackendLightpanda`（约 97 行）
- `BackendLocal, BackendCDP, BackendBrowserbase, ...`（约 84 行）

两个 case 的导航逻辑**80%+ 完全相同**：注入 stealth JS → 导航 →
等待 DOM interactive → 重新应用 stealth JS → 获取 title →
生成 snapshot → 更新 session 状态。差异仅：
- 错误前缀（"lightpanda navigation failed" vs "navigation failed"）
- Lightpanda 末尾有 12 行的 fallback 处理

#### 重构
提取共享 helper `chromedpNavigateAndSnapshot(session, parsedURL, navTimeout, navErrPrefix)`，
封装完整的 chromedp 导航流程（92 行）。两个 case 改为单行调用：
- `BackendLightpanda`：调用 helper + fallback 处理
- `BackendLocal/CDP/...`：仅调用 helper

#### 行为等价性
- helper 使用 `defer cancel()`，在函数返回时释放 timeoutCtx
- fallback 使用外层 ctx（非 timeoutCtx），不受影响
- 所有 chromedp 操作在 helper 返回前完成，cancel 时机等价于原代码

#### 效果
- `NewBrowserNavigateTool`：288 行 → 131 行（**-55%**）
- 新增 `chromedpNavigateAndSnapshot`：92 行（单一来源）
- 75 行重复导航逻辑从 2 处合并为 1 处
- 修改导航流程（如调整 stealth JS 应用时机）只需改 1 处

### 不执行的任务与勘误

#### ❌ 不拆分 runCompaction
`agentcore/compaction.go:runCompaction`（235 行）经评估**不适合拆分**：
- 该函数是**线性业务流程**（非重复代码），步骤间有大量局部变量共享
  （msgs, compressStart, compressEnd, summaryIdx, turnsToSummarize 等）
- 强行拆分需要在多个 helper 间传递 10+ 个参数，**降低可读性**
- 该函数是核心上下文压缩逻辑，**风险极高**（任何 bug 影响所有 agent 记忆系统）
- 拆分弊大于利，保持现状

#### ❌ fmt.Errorf 加 %w 不成立
审查报告称"450 处 fmt.Errorf 未用 %w（39%）"是**严重误判**：

经 Python 脚本精确分析（基于 AST 级别的参数类型推断）：
- 449 处未 wrap 的 `fmt.Errorf` 中，**真正应该 wrap 但没 wrap 的数量为 0**
- 63 处是**字面新建错误**（无参数，如 `fmt.Errorf("invalid token")`）
- 384 处是**带业务变量参数的新建错误**
  （如 `fmt.Errorf("session %q not found", sessionID)`，
  参数是 sessionID/name/statusCode 等，不是 err）
- 2 处边界情况（panic recover 的 `%v`、errStr 字符串变量）是合理设计

**关键认知**：`%w` 只用于**包裹另一个 error** 以保持错误链。
用 `%v`/`%q`/`%d` 打印业务变量是**完全正确**的 Go 惯用法。
审查报告的统计方式（"不含 %w 的 Errorf 数量"）技术上准确但语义误导。

Mady 的错误处理实际**非常健康**：
- 694 处 `fmt.Errorf` 正确使用了 `%w`
- 18 处 `errors.Is`、7 处 `errors.As`（数量适中，因为项目主要用 sentinel error）

### 验证
- `make all` ✅（根 + tools 子模块）
- `cd tools && go test ./...` ✅
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- gopls：0 errors

### 涉及文件
- `tools/browser_legacy.go`（重构）
- `docs/decisions/AI_CHANGELOG.md`（本记录）

### 影响范围
- ✅ 浏览器导航工具：代码重复消除，可维护性显著提升
- ✅ 错误处理认知勘误：避免未来基于误判的批量重构
- ❌ 不影响任何运行时行为（重构是行为保持的提取）

### 审查报告勘误（累计）
本次审查（里程碑 1-3）共发现原审查报告 **6 处误判**：
1. ~~"根模块与 tools 子模块循环依赖"~~ → 合理装配耦合（里程碑 2 已记录）
2. ~~"`agui/types.go:36` 312 行大函数"~~ → awk 统计脚本误判（里程碑 2 已记录）
3. ~~"`acp/protocol.go:30` 282 行大函数"~~ → 同上误判
4. ~~"retrieval/embedding.go HTTP client 无 Timeout"~~ → 实际已设置 30s（里程碑 1 已记录）
5. ~~"runCompaction 235 行需拆分"~~ → 线性业务流程，拆分弊大于利（本次记录）
6. ~~"450 处 fmt.Errorf 未 wrap 是债务"~~ → 真正应 wrap 的为 0 处（本次记录）

**结论**：Mady 的代码质量基线**远高于审查报告所暗示的水平**。
审查中的静态扫描数字（行数、百分比）有多处误判，但深度分析后确认实际债务极少。

---


## 2026-07-19: 里程碑 2——代码现代化与重复消除

### 背景
技术债务审查报告中原列"Sprint 2 第 4 项：解开 tools/browser_session.go 与
browser_providers 的循环依赖"。经深入分析，**该项基于审查误判**：
- `tools/browser_session.go` 与 `tools/browser_providers` 同属 tools 子模块内部，
  是正常的子包依赖（非跨模块循环）
- 根模块 ↔ tools 子模块的双向引用是合理的装配耦合
  （cmd/mady 和 domains 装配 tools 扩展，tools 实现 agentcore.Extension 接口）
- agentcore 不 import domains（仅测试文件引用），无架构层循环

因此里程碑 2 转向**低风险高价值**的两项债务清理。

### 变更

#### 1. `interface{}` → `any`（Go 1.26 现代化）
- 全仓库 18 处 `interface{}` 机械替换为 `any`
- 使用 `gofmt -r 'interface{} -> any'` 官方 rewrite 规则
- 包含 2 处注释中的 `interface{}` 引用
- **零运行时行为变更**（`any` 是 `interface{}` 的类型别名，编译后完全等价）

涉及文件（9 个）：
- `agentcore/evaluate/cli/cli.go`、`agentcore/evaluate/loader.go`
- `agentcore/evaluate/reflection.go`、`agentcore/evaluate/reflection_test.go`
- `agentcore/evaluate/tool_accuracy.go`
- `domains/reasoning/multi_hypothesis.go`
- `retrieval/model_rerank_test.go`
- `skill/frontmatter.go`、`tui/terminal/keybindings.go`

#### 2. `agui/converter.go` Convert 函数重复代码消除
- **问题**：Convert 函数 237 行，11 对 value/pointer case 完全对称重复，
  其中 5 对（HandoffStart/End、CompactionStart/End、AutoRetry）包含
  ~15 行的 CustomEvent 字面量构造，修改字段时需同步改两处
- **重构**：提取 5 个 helper 方法（`convertHandoffStart`/`convertHandoffEnd`/
  `convertCompactionStart`/`convertCompactionEnd`/`convertAutoRetry`），
  每对 pointer case 解引用后转发给 value helper
- **遵循已有模式**：与既有的 `convertMessageDelta`/`convertToolCallStart`/
  `convertToolCallEnd` 保持一致的单一职责（helper 只构造事件，
  `closeAll` 由 Convert 主函数调用）
- **效果**：Convert 函数从 237 行降到 156 行（-34%），
  CustomEvent 字段定义从 10 处降到 5 处

### 设计权衡
- **为何不提取所有 11 对 case**：剩余 6 对（AgentStart/End/Error、TurnStart/End、
  MessageDelta、ToolCallStart/End）要么已经是单行 return，要么已有 helper。
  强行提取会引入 6 个新方法但每个只有 1-2 行，收益为负。
- **为何 pointer case 不直接 type-switch 到统一接口**：各事件类型字段不同，
  无法用单一接口抽象；解引用转发是最小化、行为完全不变的方案。

### 验证
- `make all` ✅（根 + tools 子模块）
- `go test -count=1 -v ./agui/...` ✅（13 个测试全过）
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- gopls 诊断：0 errors（与重构前一致）

### 涉及文件
- `agentcore/evaluate/cli/cli.go`、`agentcore/evaluate/loader.go`
- `agentcore/evaluate/reflection.go`、`agentcore/evaluate/reflection_test.go`
- `agentcore/evaluate/tool_accuracy.go`
- `domains/reasoning/multi_hypothesis.go`
- `retrieval/model_rerank_test.go`
- `skill/frontmatter.go`
- `tui/terminal/keybindings.go`
- `agui/converter.go`

### 影响范围
- ✅ 代码现代化：Go 1.26 `any` 别名全面采用
- ✅ 可维护性：CustomEvent 字段修改从改 2 处变为改 1 处
- ✅ 可读性：Convert 函数职责更清晰（分发器 + helper 构造器）
- ❌ 不影响任何运行时行为（`any` 是别名，重构是行为保持的提取）

### 审查报告勘误
原审查报告中以下结论需更正：
1. ~~"根模块与 tools 子模块循环依赖"~~ → 实为合理的装配耦合（非病态循环）
2. ~~"`agui/types.go:36` 312 行大函数"~~ → 误判，types.go 是连续 struct 定义，
   awk 统计脚本将多个 struct 算成了一个"函数"
3. ~~"`acp/protocol.go:30` 282 行大函数"~~ → 同上误判
4. 真实的超大函数排行见本次审查的 Python 脚本输出（基于大括号深度匹配）

---


## 2026-07-19: 里程碑 1——本地开发反馈回路完整性修复

### 背景
全量技术债务审查发现：本地开发工具链与 CI matrix 存在**反馈回路不一致**。

CI（`.github/workflows/ci.yml`）通过 matrix 同时跑根模块和 `tools/` 子模块的
vet/build/test，但本地 `make` 与 `pre-commit` 都**漏掉 `tools/` 子模块**：
- `make all` / `make test` / `make build` / `make vet` 只跑 `go X ./...`（根模块）
- `.pre-commit-config.yaml` 的 `go-vet` hook 同样只覆盖根模块
- pre-commit 未集成 `golangci-lint`，开发者本地提交无法触发 errcheck/gosec/staticcheck 等 10 个 linter

开发者本地 `make all` 通过 → 推送 → CI 才发现 tools 子模块挂掉，反馈延迟 5-15 分钟，
且大量 lint 问题（未 wrap 错误、gosec 违规等）只能在 PR 阶段暴露。

### 变更
1. **Makefile**（`Makefile`）：
   - `all`/`build`/`test`/`test-race`/`test-short`/`test-verbose`/`vet` 七个 target
     全部追加 `cd tools && $(GO) X ./...` 步骤，与 CI matrix 行为对齐
   - `coverage` 保持仅根模块（与 CI codecov 上传路径一致）
   - `help` 输出顶部添加覆盖范围的显式说明
2. **pre-commit 配置**（`.pre-commit-config.yaml`）：
   - `go-vet` hook 改为 `go vet ./... && (cd tools && go vet ./...)`
   - 新增 `golangci-lint` hook（pre-commit 阶段），调用独立包装脚本
3. **新增包装脚本**（`scripts/precommit-golangci-lint.sh`）：
   - 对根模块 + tools 子模块分别运行 golangci-lint
   - 跨机器兼容：优先 `$(go env GOPATH)/bin/golangci-lint`，缺失回退 PATH
   - 未安装时非阻塞降级（提示 `make install-lint`），避免阻塞新贡献者

### 设计权衡
- **非阻塞降级 vs 强制安装**：选择前者。golangci-lint 不在 Go 工具链中，
  新贡献者首次 clone 不应被阻塞；CI 仍会强制执行。
- **抽独立脚本 vs 内联 YAML**：原计划内联 `bash -ec '...'`，
  但 YAML 对 `: `（冒号空格）敏感导致解析错误，脚本方式更可读可维护，
  与既有 `scripts/check-sensitive-paths.sh` 模式一致。

### 验证
- `make vet` ✅（根 + tools 均跑）
- `make build` ✅
- `make test` ✅（tools 子模块测试首次在本地可见）
- `make all` ✅
- `pre-commit run --all-files` ✅（9 个 hook 全过）
- `pre-commit run --all-files golangci-lint` ✅

### 涉及文件
- `Makefile`
- `.pre-commit-config.yaml`
- `scripts/precommit-golangci-lint.sh`（新增）

### 影响范围
- ✅ 本地开发：`make all` 与 CI 行为完全对齐，反馈从 PR 阶段提前到提交前
- ✅ 提交前 lint：errcheck/gosec/staticcheck 等 10 个 linter 在本地可触发
- ✅ 多模块一致性：根模块与 tools 子模块在所有本地工具链中等价对待
- ❌ 不影响任何运行时行为；不修改任何业务代码

### 后续里程碑
- **里程碑 2**（结构性）：解开 tools/browser_session.go 与 browser_providers 的循环依赖
- **里程碑 3**（持续治理）：log → slog 迁移、`fmt.Errorf` 加 `%w`、超大函数拆分

---


## 2026-07-18: 代码质量现代化——slices.Contains/min 替换 + 全面审阅报告

### 背景
对全仓库进行系统性代码质量审阅，包括 lint/vet/race/coverage/gopls
四个维度的静态分析扫描。

### 发现
- **Lint (golangci-lint 含 12 个 linter)**：0 issues ✅
- **go vet**：0 issues ✅
- **gopls**：0 errors, 0 warnings, 仅存 6 个现代化 hints
- **go build ./...**：clean ✅
- **go mod verify**：clean ✅
- **race**：go test -race ./... 全通过 ✅
- **覆盖率**：整体 ~59%，10 个包低于 50%
- **TODOs/FIXMEs/HACKs**：生产代码 0 处
- **time.After 泄漏风险**：0 处
- **panic 使用**：9 处，全部为确定性的编程错误/配置错误 sentinel panic

### 变更
- `a2a/ws.go`：手动循环 `for _, allowed := range s.allowedOrigins` →
  `slices.Contains(s.allowedOrigins, origin)`
- `mcp/client_reconnect.go`：`if x > max { x = max }` → `x = min(x, max)`

### 验证
- `go build ./...` ✅
- `go test -race -count=1 ./a2a/... ./mcp/...` ✅
- `pre-commit run --all-files` ✅

### 涉及文件
- `a2a/ws.go`、`mcp/client_reconnect.go`

### 质量评估总表

| 维度 | 评分 | 说明 |
|------|------|------|
| Lint | 🟢 0/0 | 12 个 linter + 全部 govet 检查，零问题 |
| 类型安全 | 🟢 0/0 | gopls 零错误零警告 |
| 竞态安全 | 🟢 0/0 | full `-race` 全绿 |
| 构建完整性 | 🟢 clean | go build + mod verify |
| 技术债务 | 🟢 极低 | 0 TODO/FIXME/HACK/BUG |
| 覆盖率 | 🟡 59% | 70% 目标，gap 在 TUI/协议层测试 |
| 现代化程度 | 🟢 Go 1.26 | 已应用 min/max/slices/Clear/range-int |
| 架构依赖 | 🟢 严格单向 | 8 层分层已校验 |
| 安全防护 | 🟢 all C/H fixed | Phase 7 全部 8C+16H 已修复 |

**结论：代码质量已达成熟生产级水平，下一阶段重心应转向 P3 专家盲测。**

---


## 2026-07-18: Phase 7 剩余 High 修复（H2 H3 H6）+ 安全性审核（H4 H5 H7）

### 背景
Phase 7 报告列出 18 个 High 问题，此前已修复 11 个（H1/H8-H14）。
剩余 7 个 High（H2-H7）集中于 a2a 和 mcp 包的并发安全问题。

### 变更
- **H2 (a2a/recordTask O(n²) 扫描)**：将单条目逐次淘汰的 while 循环改为
  batch collect → sort.Slice → 批量淘汰，复杂度从 O(k×n) 降为 O(n log n)。
- **H3 (a2a/handleResubscribe 浅拷贝)**：新增 `deepCopyEvent()` 对
  TaskUpdateEvent 的 Result/Artifact/Error 指针字段进行防御性深拷贝，
  在附带锁的 replay 构建过程中保证事件数据在锁释放后不会被意外共享。
- **H4 (a2a/锁顺序)**：经审阅确认安全——`recordTask` 释放 `ts.mu` 之后才获取
  `taskStatesMu`，不存在 ABBA 死锁路径。已在函数文档注释中注明锁顺序约定。
- **H5 (mcp/Close+readLoop 关闭竞争)**：审阅确认 `c.mu` 互斥锁防护了 pending
  通道的清理，readLoop 的通道关闭与 Close() 不会并发执行。Safe.
- **H6 (mcp/tryReconnect 协议版本校验)**：`initialize` 方法发送
  `protocolVersion` 给服务端但未检查服务端返回的版本。新增版本前缀匹配：
  年份不同时记录 `log.Printf` 警告，不阻塞连接（向后兼容旧服务器）。
- **H7 (mcp/callWithRetry TOCTOU)**：`writeMessage` 在 `c.mu` 解锁后调用
  `c.stdin.Write()`。若 `Close()` 在间隙被调用，Write 返回 `errClientClosed`
  并由外层 `callWithRetry` 的重试逻辑处理。误报风险低，文档已注明设计意图。

### 验证
- `go build ./a2a/... ./mcp/... && go build ./...` ✅
- `go test -race -count=1 ./a2a/...` ✅ (1.8s)
- `go test -race -count=1 ./mcp/...` ✅ (21.5s)
- `pre-commit run --all-files` ✅

### 涉及文件
- `a2a/taskstate.go`（recordTask O(n²)→O(n log n)、锁顺序注释）
- `a2a/server_jsonrpc.go`（handleResubscribe 深拷贝 + 防御性 replay）
- `a2a/types.go`（新增 deepCopyEvent 函数）
- `mcp/client.go`（initialize 协议版本核验）

### 备注
- H2-H7 经审阅修复后，Phase 7 全部 **18 个 High 问题中 16 个已修复/安全关闭**，
  H4/H5/H7 经审阅确认为安全或无实际风险。
- 2 个待定项（H2 的极端大量 task 场景 batch 排序开销、H3 的 `Message`/`Part`
  深层数组深拷贝）在当前使用场景下无实际影响，暂不处理。

---


## 2026-07-18: Phase 7 遗留 High 修复（H1 H11）+ 报告同步

### 背景
Phase 7 全量审阅报告（2026-07-14）标识 8 Critical + 18 High + 34 Medium 问题。
此前 `bda2694` / `6171e2e` 已修复全部 8 Critical 和 6 个 High，但报告状态列未同步；
且 H1（WebSocket token URL 日志泄漏）与 H11（ACP CWD 路径沙箱验证）仍未修复。

### 变更
- **H1 (a2a)**：新增 `RedactURL()` 函数（`a2a/middleware.go`），脱敏 token/apiKey 等敏感
  查询参数后输出 URL；`handleWebSocket` 处添加注释说明 query param 的已知风险交换。
  新增 `a2a/middleware_test.go`（5 个用例覆盖脱敏/非脱敏/空参数）。
- **H11 (acp)**：新增 `sanitizeCWD()` 函数，在 `handleNewSession` / `handleForkSession`
  中对 CWD 路径执行 `filepath.Clean` + `filepath.Abs`，防止 `../` 目录遍历。
- `docs/review/phase7-summary-and-roadmap.md`：全量同步——8 Critical 标注 ✅ + 修复 commit 引用；
  High 表新增状态列（11/18 已修复）；CI 门禁表 3 项🔴→✅；安全防护评级 ⭐⭐⭐→⭐⭐⭐⭐；
  总体评级从 ⭐⭐⭐⭐(4/5) 维持。

### 验证
- `go build ./acp/... ./a2a/...` ✅
- `go test -count=1 ./acp/... ./a2a/...` ✅（acp 0.011s / a2a 0.839s）
- `pre-commit run --all-files` ✅（含 sensitive-paths gate 和 commitlint）

### 涉及文件
- `acp/server.go`（sanitizeCWD 新增 + handleNewSession/handleForkSession 改用 sanitizeCWD）
- `a2a/middleware.go`（RedactURL + SensitiveQueryParams 新增）
- `a2a/middleware_test.go`（新建）
- `a2a/ws.go`（handleWebSocket 添加日志脱敏注释说明）
- `docs/review/phase7-summary-and-roadmap.md`（全表同步）

### 备注
- 剩余 7 个 High（H2-H7）未处理，主要是 a2a 锁顺序/竞争和 mcp 关闭竞争，需独立审查。
  从 roadmap 看 P3 盲测完成前不会启动这类协议层修复，符合"停止规则"。

---


## 2026-07-18: M1 门禁加固（4/4）：pre-commit 跨机器兼容 + commit-msg 敏感路径门禁

### 背景
`.pre-commit-config.yaml` 的 `go-imports` hook 硬编码 `/Users/xujian/go/bin/goimports`，
换机器/CI 即失效；同时 sensitive-paths 检查此前仅在 `--ci-base` 模式（GitHub Actions）
和无参数模式（HEAD commit）下运行，**本地 `git commit` 阶段没有任何门禁**，
导致开发者可以在本地静默提交"AI 参与 + 敏感路径"的违规组合，CI 才会拦截——反馈链路过长。

### 变更
- `.pre-commit-config.yaml`：
  - `go-imports` entry 改为动态查找：优先用 `go env GOPATH/bin/goimports`，
    缺失时回退 `PATH` 中的 `goimports`，跨机器/CI 通用
  - 新增 `check-sensitive-paths` hook（`stages: [commit-msg]`），
    调用 `scripts/check-sensitive-paths.sh --msg-file` 读取本次提交消息文件
- `scripts/check-sensitive-paths.sh`：新增 `--msg-file <path>` 参数，
  供 commit-msg 钩子读取 pre-commit 传入的提交消息文件；
  与 `--ci-base` 互斥；空参数/文件不存在/未知参数均显式报错退出

### 验证
- `bash -n scripts/check-sensitive-paths.sh` ✅
- 边界用例 4 项全部按预期：互斥分支 exit=1、未知参数 exit=1、
  缺值 exit=1、msg 文件不存在 exit=1
- `pre-commit run --hook-stage commit-msg --commit-msg-filename <tmp>` ✅
  （`sensitive paths gate ... Passed`、`commitlint ... Passed`）
- 关键阻塞场景：构造"AI Co-authored-by + 暂存 guardrails/levels.go"，
  commit-msg 钩子返回 exit=1 并打印完整阻塞提示文案 ✅
- `pre-commit run --all-files` ✅（trailing-whitespace / end-of-file 顺手清理
  docs/TOOL_CONTRACT.md、docs/design/citation-verification-gate.md、
  docs/evaluation-baseline-invalidation-p2b.json 三处历史格式遗留）

### 涉及文件
- `.pre-commit-config.yaml`、`scripts/check-sensitive-paths.sh`
- （格式顺手清理）`docs/TOOL_CONTRACT.md`、`docs/design/citation-verification-gate.md`、
  `docs/evaluation-baseline-invalidation-p2b.json`

### 备注
- 本地需执行 `pre-commit install --hook-type commit-msg` 注册 commit-msg 钩子
  （`.git/hooks/commit-msg` 不在版本控制内，新克隆需手动安装）
- CI 端 `.github/workflows/ai-code-quality.yml` 仍使用 `--ci-base` 模式，
  与本次新增 `--msg-file` 模式互不影响，无需改动

---


## 2026-07-18: M1 门禁加固（3/4）：清理无效 -short flag 与 forbidigo 死配置

### 背景
- CI race 步骤 `go test -race -count=1 -short ./...` 中的 `-short` 是纯无效 flag：
  全库 0 处 `testing.Short()` 调用，没有任何测试会因此在 CI 被跳过
- `.golangci.yml` 存在针对 `cmd/mady/main.go` 的 forbidigo 排除规则，
  但 forbidigo 未出现在 `linters.enable` 列表中，规则永远不会生效，属死配置

### 变更
- `.github/workflows/ci.yml`：race 步骤命令去掉 `-short`，
  改为 `go test -race -count=1 ./...`（Makefile 的 `test-short` target 保留不动）
- `.golangci.yml`：删除 forbidigo 排除规则（原第 93-95 行）

### 验证
- `golangci-lint run ./...`（根模块）✅ 0 issues
- `cd tools && golangci-lint run ./...`（tools 子模块）✅ 0 issues
- `go build ./... && go vet ./...` ✅

### 涉及文件
- `.github/workflows/ci.yml`、`.golangci.yml`

---


## 2026-07-18: M1 门禁加固（2/4）：codecov 配置合并并启用覆盖率门槛

### 背景
仓库根目录同时存在 `codecov.yml`（project target 30%/threshold 5%）与 `.codecov.yml`
（50%/2%）两份冲突配置，Codecov 实际生效项不确定；且 CI 覆盖率上传步骤
`fail_ci_if_error: false`，上传失败静默通过，覆盖率门禁形同虚设。
当前覆盖率基线约 61.4%，采取渐进收紧策略。

### 变更
- 删除 `.codecov.yml`，配置统一合并进 `codecov.yml`：
  - project target 55%、threshold 2%（低于当前基线，留出缓冲渐进收紧）
  - patch target 50%
  - ignore 取两文件并集：`example/**`、`benchmark/**`、`integration/**`、
    `**/*_test.go`、`**/doc.go`、`vendor/**`
  - 保留原有 precision(2)/round(down)/comment 配置，range 调整为 `55...80`
- `.github/workflows/ci.yml`：覆盖率上传步骤 `fail_ci_if_error` 由 `false` 改为 `true`，
  上传失败将直接使 CI 失败

### 验证
- `go build ./... && go vet ./...` ✅
- pre-commit `check-yaml` 通过（提交时自动校验 codecov.yml / ci.yml 语法）

### 涉及文件
- `codecov.yml`、`.codecov.yml`（删除）、`.github/workflows/ci.yml`

---


## 2026-07-18: M1 门禁加固（1/4）：integration e2e 测试接入 CI

### 背景
`integration/` 下的 e2e 测试（build tag `integration`，本地 21 个用例全绿）此前只能手动跑，
CI 无任何门禁，回归无法被自动拦截。

### 变更
- `Makefile`：新增 `test-integration` target（`go test -tags integration -count=1 ./integration/...`），
  同步加入 `.PHONY` 与 help 文本（Test 段落）
- `.github/workflows/ci.yml`：新增 `integration` job（位于 build-and-test 之后），
  ubuntu-latest + checkout@v7 + setup-go@v6（`env.GO_VERSION`），
  执行 `go test -tags integration -count=1 -timeout 10m ./integration/...`

### 验证
- `go test -tags integration -count=1 ./integration/...` ✅（26 个 PASS，含子测试）
- `go build ./... && go vet ./...` ✅

### 涉及文件
- `Makefile`、`.github/workflows/ci.yml`

---


## 2026-07-18: TUI 质量审查修复落地（P1 主题检测 + P2 测试覆盖 + 支撑构造函数）

### 背景
对 Mady `tui/` 模块进行全面质量审查后，识别出 1 个 P1 功能缺陷、1 个 P2 结构性短板，
以及变更日志与代码状态不一致的问题。本次集中完成这些修复的落地工作。

### 修复内容

1. **P1 修复深色终端主题检测**
   - `tui/theme/global.go`：`DefaultSemanticForTerminal()` 在检测到非浅色终端时返回 `DefaultMadyDark()`
   - 新增 `tui/theme/global_test.go`：覆盖 COLORFGBG 缺失、异常、深色（0–7）与浅色（8+）分支，
     并验证 `SetSemanticTheme`/`CurrentPalette` 全局状态同步
   - 测试通过保存/恢复 `atomicPalette` 避免全局状态交叉污染

2. **P2 提升 TUI 测试覆盖率**
   - 重写 `tui/agentadapter/adapter_test.go`：删除空回调，改为通过真实事件总线 emit agentcore 事件，
     等待异步分发 drain 后断言 chat 事件字段，覆盖 agent start、turn end、message delta、agent error
   - 新增 `tui/terminal/keybindings_test.go`：覆盖 Register、Matches、SetUserBindings、
     冲突检测、All、LoadUserBindingsJSON（含非法 token 与空输入）
   - 新增 `tui/component/settings_test.go`：覆盖空列表、Get/SetValue、导航、左右循环、
     OnChange、OnSubmit、渲染
   - 新增 `tui/component/loader_test.go`：覆盖 Start/Stop 幂等、IsRunning、SetMessage、
     SetStyle、渲染、CancellableLoader abort

3. **支撑性改动：agentcore 事件构造函数**
   - `agentcore/event.go`：为外部测试和调用方新增 `NewAgentStartEvent`、`NewAgentEndEvent`、
     `NewAgentErrorEvent`、`NewAgentInterruptEvent`、`NewSkillLoadedEvent`、`NewTurnStartEvent`、
     `NewTurnEndEvent`、`NewMessageDeltaEvent`、`NewToolCallStartEvent`、`NewToolCallEndEvent`、
     `NewHandoffStartEvent`、`NewHandoffEndEvent`、`NewCompactionStartEvent`、
     `NewCompactionEndEvent`、`NewAutoRetryEvent`（共 15 个）
   - 所有构造函数自动设置正确的 `baseEvent.Kind`，解决外部包无法构造可路由事件的问题

4. **文档与注释同步**
   - `tui/component/domain.go`：修正注释，明确 `agentcore.Message.Metadata["domain"]` 的 JSON
     解码链路尚未实现，避免维护者误以为该链路已完工

5. **误报澄清**
   - `agentcore/evaluate/metrics_test.go:TestSetCitationVerifierConcurrent` 经 10 次 `-race` 实测
     稳定通过，无 `send on closed channel` 风险；其 `sync.WaitGroup` 用法正确，无需改动

### 验证
- `go build ./...` ✅（根模块）
- `go test -race ./tui/theme/ ./tui/component/ ./tui/terminal/ ./tui/agentadapter/ ./agentcore/ ./agentcore/evaluate/` ✅
- `golangci-lint run ./tui/... ./agentcore/...` ✅ 0 issues
- 覆盖率数字待全量跑批后重新测量

### 涉及文件
- `tui/theme/global.go`、`tui/theme/global_test.go`
- `tui/agentadapter/adapter_test.go`
- `tui/terminal/keybindings_test.go`
- `tui/component/settings_test.go`、`tui/component/loader_test.go`
- `agentcore/event.go`
- `tui/component/domain.go`
---


## 2026-07-18: 规范合规全面修复（P0/P1/P2/P3 共 9 项）

### 背景
对照 AGENTS.md / CLAUDE.md / SECURITY.md / CONTRIBUTING.md / tone-style-guide /
chat-assistant-architecture 全套规范审阅项目，发现 4 项 P0、2 项 P1、3 项 P2/P3 真实违规，
无任何"误报"。本次集中修复全部 9 项。

### P0：真实违规（4 项）

1. **敏感路径清单 6 份不一致**：`scripts/check-sensitive-paths.sh` 仅 10 条，
   AGENTS.md/CLAUDE.md 各 18 条，SECURITY.md 12 条，GO-DEVELOPMENT-STANDARDS.md 9 条，
   CI 实际只检测 10 条路径——8 个文档承诺的敏感路径门禁缺口
   - 修复：脚本扩展为 18 条权威源（补 `tools/vision.go` + 7 个上层边界文件），
     4 份管理文档同步为 20 条（脚本 18 + 目录级 `agentcore/permission/`、`guardrails/guardian/`）
   - 在每份文档显式声明"脚本为权威源"，避免未来漂移
2. **`tui/chat` 违规导入 `agentcore`**：LAYERS.md 第 21 行明确禁令，
   `chat_history.go:16` 实际 import agentcore（使用 DomainMessage 类型）
   - 修复：将 `agentcore/message_domain.go` 整体下沉到 `tui/component/domain.go`
     （该文件本就是孤儿——所有引用方都在 TUI 内部，agentcore/evidence 包有独立的同名类型）
3. **`tui/component` 3 个 card 组件违规导入 `agentcore`**：LAYERS.md Layer 4 仅允许依赖 Layer 0–2
   - 修复：随 P0-2 一次性消除——DomainMessage 迁移到本包后，evidence_card/approval_card/conclusion_card
     均去掉 agentcore import，改用同包类型
4. **删除原 `agentcore/message_domain.go`**：无任何外部消费者（agentcore/evidence 包定义独立的
   EvidenceDirection/EvidenceSpan，与 DomainMessage 无关）

### P1：规范执行偏差（2 项）

1. **`server.New` 导出符号无注释**（GO-DEVELOPMENT-STANDARDS.md §10.1 要求每个导出符号必须注释）
   - 修复：补中文注释（SSEKeepAlive 已有注释，原审查误报）
2. **GO-DEVELOPMENT-STANDARDS.md §10.1 与 §10.4 内部矛盾**：§10.1 示例用英文，§10.4 要求中文
   - 修复：§10.1 显式约定"注释使用中文，但首句以英文符号名开头以兼容 godoc/golint"，
     示例改为中文版本
3. **CI 未强制 Conventional Commits**：仅本地 pre-commit 钩子，贡献者未装钩子可绕过
   - 修复：新增 `commitlint.config.js`（放宽 header-max-length 到 120 以兼容中文 subject，
     关闭 subject-case），在 `.github/workflows/ci.yml` 增加 commitlint job（wagoid/commitlint-github-action@v6）

### P2：架构与路径规范（2 项）

1. **cmd/mady 3 处 `./` 相对路径兜底**：framework.go:224、server.go:58、tui.go:81
   违反 AGENTS.md "禁止新增 ./ 相对路径默认值"
   - 修复：兜底分支改用 `util.ResolveDataDir()`，与 `MadyHome()` 三级解析保持一致
   - 注：这些分支理论上不可达（MadyHome 最终回退已调 filepath.Abs），
     修复目的在于统一治理、防止未来扩张
2. **`agentcore/evaluate` 反向依赖 `guardrails`**：metrics.go:260 调用 `guardrails.VerifyCitations`，
   违反"上层不得反向依赖下层"约束
   - 修复：依赖注入模式——在 evaluate 包定义本地 `CitationValidityReport` +
     `CitationVerifier` 函数变量 + `SetCitationVerifier` 注入 API，evaluate 包零 guardrails 引用
   - 注入点：`cmd/mady/eval.go`（顶层装配层）通过 `citationVerifierAdapter` 注入；
     `scripts/replay_citation_metrics/main.go` 同步注入；
     `metrics_test.go` 通过 t.Cleanup 在测试中注入并恢复
   - 测试场景例外：`evaluate/benchmark/live_*_test.go` 的 `provider/chatcompat`、
     `domains/reasoning` 反向引用属合理的测试集成场景（需真实 LLM 才能跑），保留现状

### P3：文档一致性（1 项）

1. **`tui/layout` 在 LAYERS.md 缺定义**：CLAUDE.md:102 标注"Layer 8"错误，
   实际 `tui/layout` 仅依赖 `tui/core`（Layer 0）
   - 修复：LAYERS.md 表格补 `tui/layout` 行（Layer 0 扩展），Directory Structure 补 layout/ 子树，
     CLAUDE.md 描述改为"Layer 0 扩展：布局原语（仅依赖 core）"

### 合规修复复审纠偏（P1-1 ~ P2-2）

对本次合规修复进行第二轮质量复审，发现并修复 7 项遗留问题（3 P0 + 4 P1/P2）：

- **P1-1 脚本扩展为目录前缀匹配**：`scripts/check-sensitive-paths.sh` 原仅 `grep -Fx` 精确匹配，
  无法覆盖 `agentcore/permission/` 与 `guardrails/guardian/` 目录下新增文件。
  新增 `SENSITIVE_PATH_PREFIXES` 数组 + `grep -F` 前缀匹配循环。
- **P1-2 `currentCitationVerifier` 并发安全**：原为裸 `var CitationVerifier`，
  `mady eval --workers N` 并发调用 `Compute()` 存在 data race。
  改为 `atomic.Pointer[CitationVerifier]`，`SetCitationVerifier` 走原子 Store，
  新增 `TestSetCitationVerifierConcurrent` 用确定性 barrier 协调（writer 与 readers
  通过 channel 握手，保证每轮切换的 verifier 都被所有 readers 观察到），断言两个
  可区分 verifier（score=0.0 与 0.5）均被实际观察到——而非仅检查 `score ∈ [0,1]`
  （后者是恒真的假阳性断言）或靠 goroutine 调度运气（后者 flaky）。
  （`go test -race -count=5` 通过）。`metrics_test.go` 改用公开 API `SetCitationVerifier`
  而非直接 var 赋值。两个测试均标注 `// 不可加 t.Parallel()` 以防未来误用。
- **P1-3 commitlint CI 触发口径**：`.github/workflows/ci.yml` 原 `on: [push, pull_request]`
  导致 push 到 main 时会 lint 单个 squash commit（语义无意义）。
  改为 `if: github.event_name == 'pull_request'` 仅 PR 触发，并补注释说明覆盖范围。
- **P1-4 README 示例硬编码 `"./sessions"`**：README.md:380 仍示范旧反模式。
  改为展示 `util.ResolveDataDir("sessions")` + error handling 正确范式。
- **P2-1 `ResolveDataDir` 错误被静默吞掉**：`cmd/mady/{framework,server,tui}.go` 三处
  `sessionDir, _ = util.ResolveDataDir(...)` 用 `_, _` 丢弃 error。
  改为捕获并 `log.Printf` / `fmt.Fprintf(os.Stderr, ...)` 输出，便于兜底分支异常时定位。
- **P2-2 过期"修复："注释前缀**：`cmd/mady/server.go:69` 残留 `// 修复：使用 fc.WorkspaceDir...`
  合并到 P2-1 同一 edit 中清理为现在时态描述。

### 验证（复审后全量重跑）
- `go build ./...` / `go vet ./...`：OK
- `cd tools && go build ./... && go vet ./...`：OK
- `golangci-lint run ./...`（根 + tools）：0 issues
- `go test -race -count=1 ./... 2>&1 | grep -E "^(ok|FAIL|---)"`：70 包全部 `ok`，无 FAIL
- `cd tools && go test -race -count=1 ./...`：2 包全部 `ok`
- 敏感路径清单 5 份文档条目数一致：脚本 18（权威源）、4 份管理文档 20（含目录级 2 条）

> **注**：本次修复声称"全量测试通过"时曾出现失误——首次跑 `go test -race ./...`
> 输出被 `tail -80` 截断，掩盖了工作区中其它无关改动（`tui/agentadapter/adapter_test.go`、
> `tui/theme/global.go` 等）导致的测试失败。本次 commit 前，已通过 `git stash`
> 隔离所有无关改动，本次修复涉及包的测试结果见上。

### 涉及文件（本次修复 24 个 = 21 改 + 2 新增 + 1 删除）
**新增**：`tui/component/domain.go`、`commitlint.config.js`
**删除**：`agentcore/message_domain.go`
**修改**：
- `scripts/check-sensitive-paths.sh`、`SECURITY.md`、`AGENTS.md`、`CLAUDE.md`、`docs/GO-DEVELOPMENT-STANDARDS.md`、`tui/LAYERS.md`、`README.md`
- `tui/chat/chat_history.go`、`tui/component/{evidence,conclusion,approval}_card.go`
- `server/server.go`
- `.github/workflows/ci.yml`
- `cmd/mady/{framework,server,tui,eval}.go`
- `agentcore/evaluate/metrics.go`、`agentcore/evaluate/metrics_test.go`
- `scripts/replay_citation_metrics/main.go`

> **隔离记录**：commit 前工作区另含 5 个无关改动（`agentcore/event.go`、
> `tui/agentadapter/adapter_test.go`、`tui/theme/global.go`、
> `tui/component/input_test.go`、`tui/component/selectlist_test.go`），
> 对应上方"TUI 质量审查后续修复"条目。已 `git stash` 保留，本次修复 commit 不含这些改动。

---


## 2026-07-18: Code Review 第二轮修复（5 项 P0/P1 + 1 项测试质量）

### 背景
`/review` 对未提交改动 + 既有代码扫描后报告 8 项问题（4 P0 + 2 P1 + 2 P2）。逐项核查后发现 **3 项误报**（审查员看的是旧版本），**5 项真问题 + 1 项测试质量建议**。用户选择"全部 8 项修复 + 先修问题再一起提交"。

### 误报核查（3 项）
| # | 审查声称 | 实际状态 | 结论 |
|---|---|---|---|
| 1 | `audit.go` key 派生用 `copy` | 实际已是 `sha256.Sum256([]byte(keyStr))` | 误报（旧版本） |
| 2 | `claim_drafting.go:43` 空输出丢 claims | 已有 `else { state[StateKeyOutput] = claims }` 分支 | 误报（旧版本） |
| 3 | `legal.go:118` 注释引用不存在方法 | 当前文件只有 114 行，该注释不存在 | 误报（旧版本） |

### 真实修复（5 项）
1. **P0** `domains/audit.go` Encryptor 伪加密 → 真 AES-256-GCM 实现
   - 新增 imports：`crypto/aes`、`crypto/cipher`、`crypto/rand`、`encoding/base64`、`io`
   - `Protect`：用 AES-256-GCM 加密，返回 `base64(nonce || ciphertext)`
   - `Reveal`：base64 解码 → AES-GCM 解密；任何失败 fail-open 返回原文（兼容迁移期间未加密数据）
   - 修复 gocritic `appendAssign` 警告：`append(nonce, sealed...)` → `make + 双 append`
   - 验证：Encryptor 无任何调用方，纯占位实现升级，零连锁影响
2. **P0** `disclosure/claim_drafting.go:34,38` Pregel 节点错误返回 `nil` → `state`
   - 两处 `return nil, fmt.Errorf(...)` → `return state, fmt.Errorf(...)`
   - 与 `pregelAgentNode` 等其他节点的 `state, err` 契约对齐，避免后续 Pregel 加入 error-bridge 时 NPE
3. **P0** `disclosure/claim_drafting.go` 孤儿权利要求防护
   - 新增节点入口 guard：`len(ext.PFETriples) == 0` 时直接返回 `state, err`
   - 原问题：当 `Problems` 为空且 `Features` 非空时，`linkPFETriples` 不会回填，会生成 `DependsOn:1` 但权利要求 1 不存在的孤儿，违反专利法第 26 条第 4 款引用清楚性要求
4. **P1** `guardrails/citation_table.go:69` 商标法总条数 75 → 73
   - 核对 CNIPA 公布的《商标法》（2019 年第四次修正）原文，最后一条是第七十三条（非七十五条）
   - 注释同步更新："共八章 75 条" → "共八章 73 条"
   - 影响：条号 74、75 不再被误判为合法引用，S1 存在性核验防线恢复
5. **P1** `disclosure/graph.go:271` 图拓扑注释过时
   - `generate_report → review_gate → __end__` → `... → review_gate → draft_claims → __end__`
   - 与实际拓扑（line 334-336）对齐，避免新接入方误以为 review_gate 是终止节点

### 测试质量补充（1 项）
- `workflows/patent/oa_response_test.go:87-90` `TestParseOANode_InventivenessRejection`
  - 原仅 `t.Logf` 无 assert → 改为 `if rejectionType != string(rules.OaInventiveness) { t.Errorf(...) }`
  - 该测试原本等于无断言，等于不测；现在真正验证"创造性 → OaInventiveness"的映射

### 验证
- `go build ./...` / `go vet ./...`：OK
- `goimports -l` / `gofmt -l`：clean
- `golangci-lint run ./domains/... ./disclosure/... ./guardrails/... ./workflows/...`：0 issues
- `go test -race -count=1`：domains / disclosure / guardrails / workflows/patent 全部 PASS

### 涉及文件（5 个）
`domains/audit.go`、`disclosure/claim_drafting.go`、`disclosure/graph.go`、`guardrails/citation_table.go`、`workflows/patent/oa_response_test.go`

---


## 2026-07-18: P2-5 大文件拆分（4 个 >1000 行文件 → 20 个聚焦文件）

### 背景
全量技术债务扫描后，发现 4 个超过 1000 行的文件集中了多个不相关的职责：
`mcp/client.go` (1005)、`mcp/http.go` (1023)、`a2a/server.go` (1027)、`server/server.go` (1324)。
单文件职责过重影响可读性、增量编译效率、code review 质量，且新增功能时容易产生 merge conflict。
按"小炸弹不是大炸弹"原则，分 4 个 phase 顺序拆分（按风险升序：mcp → a2a → server）。

### 拆分明细

| Phase | 原文件 (行数) | 拆分后文件 | commit |
|---|---|---|---|
| 1 | `mcp/client.go` (1005) | `client.go` + `stdio_extension.go` + `client_jsonrpc.go` + `client_readloop.go` + `client_reconnect.go` (5 文件) | `cbeec0d` |
| 2 | `mcp/http.go` (1023) | `http.go` + `http_jsonrpc.go` + `http_sse.go` + `http_session.go` (4 文件) | `23baa2c` |
| 3 | `a2a/server.go` (1027) | `server.go` + `server_options.go` + `server_taskstate.go` + `server_jsonrpc.go` + `server_card.go` (5 文件) | `aafc6ec` |
| 4 | `server/server.go` (1324) | `server.go` + `types.go` + `pool.go` + `chat.go` + `thread.go` + `skills.go` (6 文件) | `6ee1dd3` |

### 拆分原则
- **同包内拆分**：每个新文件保持 `package` 不变，Go 天然支持同包跨文件可见 unexported 符号
- **按职责聚类**：每个新文件聚焦单一主题（jsonrpc / sse / session / reconnect / thread / skills / pool / types）
- **零外部 API 变更**：导出符号在原 package 内重组，签名不变；调用方（cmd/mady, example/, tools/）零影响
- **测试零修改**：所有 `_test.go` 在 same package，可见 unexported 符号，无需调整
- **每个 phase 一 commit**：便于独立 revert 与 code review

### 收益
- 4379 行 → 20 文件，每文件 80-510 行，单文件平均行数下降 ~70%
- HTTP MCP 客户端的 jsonrpc / sse / session 三个子问题彻底分离，后续维护可独立修改
- server/server.go 的 use-after-free 防护逻辑（pool.go）独立成文件，安全审计边界更清晰
- 增量编译：修改 chat 逻辑不再触发 thread/skills 的重新编译

### 关键陷阱记录
1. **Write 工具偶发"假成功"**：Phase 2/3/4 多次遇到 Write 报"Wrote file successfully"但文件实际未创建，build 时报符号 undefined。**对策**：每写一个文件立即 `ls -la` 验证
2. **goimports 路径**：本机 PATH 不含 `$(go env GOPATH)/bin`，`.pre-commit-config.yaml` 保留绝对路径 `/Users/xujian/go/bin/goimports`，不要改为 PATH 查找（会让 hook 在本机立即失败）
3. **gosec G204 豁免**：`mcp/client.go` 拆分后 `exec.Command` 迁移到 `client_reconnect.go`，`.golangci.yml` 的豁免规则需同步扩展为 `mcp/client\.go|mcp/client_reconnect\.go`

### 验证
每个 phase 完成后均跑全套验证（根 + tools 子模块）：
- `go build ./...`：OK
- `go vet ./...`：OK
- `goimports -l` + `gofmt -l`：clean
- `golangci-lint run ./...`：0 issues
- `go test -race -count=1 ./...`：全部 PASS，无 DATA RACE

---


## 2026-07-18: 全量技术债务清理（P0 真实 Bug + P1 Lint 债务 + P2 结构性改进）

### 背景
对 Mady（858 文件 / ~185K 行 / go.work 多模块）执行全量技术债务扫描，发现 7/16 REVIEW
后的 Tier 3 提交引入了大量未跑 lint 的新代码，lint 从 4 issues 涨到 35 issues。本次
集中清理 3 项真实 Bug + 5 类 lint 债务 + 4 项结构性改进，根模块与 tools 子模块均达到
`golangci-lint run ./...` 0 issues。

### P0：真实 Bug（3 项）

| # | 文件 | 问题 | 修复 |
|---|------|------|------|
| A | `tools/vision.go:244` | 运算符优先级歧义：`len>=2 && II \|\| MM` 等价于 `(len>=2 && II) \|\| (MM)`，当 `len<2` 且首字节为 'M' 时 `data[1]` 越界 panic | 改为 `if len(data) >= 2 { if magic := string(data[:2]); magic == "II" \|\| magic == "MM" {...}}`，补 7 个回归用例（含单字节 'M'/'I' 不 panic） |
| B | `mcp/install.go:304` | MCP 配置文件权限 0o644 允许同组/其他用户读取（配置含 token、命令执行白名单） | `0o644` → `0o600` |
| C | `a2a/push.go` | **SSRF TOCTOU 漏洞**：`validateWebhookURL` 提前 `net.LookupHost` 查私网 IP，但 `http.Client.Do(req)` 内部会再次 DNS 解析，DNS rebinding 攻击可在两次查询间切换 IP 绕过防护 | 新增 `ssrfSafeDialer.DialContext`：自行 `LookupIPAddr` 后选首个公网 IP 用 `net.JoinHostPort` 重组地址直连，**完全绕开 Transport 内部二次解析**；`validateWebhookURL` 移除 `LookupHost` 调用（私网检查下沉到 dialer）。补 4 个 SSRF 测试（dialer 拒私网/放行、URL 校验、端到端 Notify） |

`.golangci.yml` 新增白名单：`a2a/push\.go` 的 G704，附注释说明运行时防护消除 taint。

### P1：Lint 债务批量清理（31 项）

| 类别 | 数量 | 文件 |
|---|---|---|
| gofmt | 10 | `a2a/server.go`、`agentcore/cache/{policy,stats}.go`、`mcp/{client,http}.go`、`memory/compiler/rule_engine_bridge.go`、`server/server.go`、`tools/browser.go`、`workflows/autoresearch/{contract,heartbeat}.go`（一次 `gofmt -w .` 修复） |
| QF1012（`WriteString+Sprintf` → `Fprintf`） | 15 | `domains/style.go` x9、`prompt/loader.go` x4、`domains/doctmpl/loader.go` x2 |
| errcheck | 1 | `provider/adapter/session.go:65` `io.Copy` 改 `_, _ = io.Copy`（goroutine stderr 复制，错误无法返回） |
| gocritic ifElseChain → switch | 3 | `a2a/agent_handler.go:255`、`tui/component/editor_edit.go:34,42` |
| misspell | 1 | `session/session_store.go:181` `serialises` → `serializes` |
| SA9009 | 1 | `domains/style.go:19` 注释含 `//go:embed` 文字被误判，重写注释消除 |

### P2：结构性改进（3 项）

| # | 文件 | 改动 |
|---|---|---|
| 1 | `tools/browser_tool_handlers.go:117` | SPA 渲染缓冲 1s sleep 保留（无对应集成测试可校准），强化注释说明经验值来源及修改前置条件 |
| 2 | `memory/sqlite_store.go:526` | `_ = json.Unmarshal(...)` 保留（与同函数 `time.Parse` 一致的 best-effort 降级策略），加注释说明 metadata 列无 schema 约束、损坏时降级为 nil 以避免单条记录阻塞整次查询 |
| 3 | `knowledge/loader/law_index_test.go:135` | `os.UserHomeDir()` + `/.mady` 硬拼接 → `util.MadyHome()`，对齐 AGENTS.md 的"任意 cwd 启动 / `$MADY_HOME` 覆盖"原则；并加 `t.Setenv("MADY_HOME", t.TempDir())` 隔离副作用（`MadyHome()` 会 `EnsureDir` 创建目录）。其余两处 skip（`patent_retriever_test.go`、`stage2_wiring_test.go`）经审查为合理的 corpus/env 门控，保留 |

> 备注：`.pre-commit-config.yaml` 的 goimports 绝对路径硬编码问题在评估后**未修改**——本机 PATH 不含 `$(go env GOPATH)/bin`，改为 PATH 查找会让 hook 在本机立即失败；AGENTS.md 已有"换机器需重装 goimports 并调整该路径"的警告，保持现状更稳。

### 已审查的非问题（避免误改）
- 32 处 `_ =` 忽略错误中 28 处为合理 defer cleanup；4 处 a2ui `json.Unmarshal` 注释明确合约
- 9 处 `time.Sleep(1s)` 中 `process.go:416` 进程轮询合理，浏览器自动化 8 处合理
- 所有 `MustXxx` panic = Go 惯例；全库无 `recover()` 吞 panic 反模式
- TODO/FIXME/HACK 标记为 0；无循环依赖（8 层洋葱架构）
- 二进制产物（mady/acp-server/cli-chat 等）在 `.gitignore` 中，未入库

### 未做（待评估）
- **4 个大文件拆分**（`server/server.go` 1325 行 / `a2a/server.go` 1031 行 / `mcp/http.go` 1024 行 / `mcp/client.go` 1006 行）：风险较高，需配套测试，标记为 P2-5 单独评估
- Phase 4 backlog（multi_hypothesis 双雄辩论等 6 项）已在 `docs/decisions/phase4-backlog.md` 规划

### 验证
- `go build ./...` ✅（根 + tools 子模块）
- `go vet ./...` ✅（根 + tools 子模块，零警告）
- `golangci-lint run ./...` ✅ **0 issues**（根 + tools 子模块）
- `go test -race ./...` ✅ 全部 60+ 包通过（根 + tools），无数据竞争
- `gofmt -l .` ✅ clean（根 + tools）

### 涉及文件（共 21 个）
`.golangci.yml`、`a2a/a2a_test.go`、`a2a/agent_handler.go`、
`a2a/push.go`、`a2a/server.go`、`agentcore/cache/{policy,stats}.go`、
`domains/doctmpl/loader.go`、`domains/style.go`、`docs/decisions/AI_CHANGELOG.md`、
`knowledge/loader/law_index_test.go`、`mcp/{client,http,install}.go`、
`memory/compiler/rule_engine_bridge.go`、`memory/sqlite_store.go`、`prompt/loader.go`、
`provider/adapter/session.go`、`server/server.go`、`session/session_store.go`、
`tools/browser.go`、`tools/browser_tool_handlers.go`、`tools/vision.go`、
`tools/vision_test.go`、`tui/component/editor_edit.go`、
`workflows/autoresearch/{contract,heartbeat}.go`


## 2026-07-18: Open Design 思路引入 —— Tier 3 实现（最终阶段）

### 背景
完成 Open Design 思路引入的第三阶段（高影响力/高复杂度），覆盖所有剩余 3 项特性。
至此，全部 8 个思路已分三个 Tier 完整引入 Mady。

### 本次实现（Tier 3 — 高影响力 / 高复杂度）

| # | 特性 | 改动 |
|---|------|------|
| 6 | **文档模板库** | `domains/doctmpl/`：`DocTemplate` 类型 + `LoadDocTemplates` / `LoadDocTemplatesFromFS` / `ResolveDoc` / `FindDocByCategory` / `DocIndex` API。`doc-templates/` 含 12 个 Markdown 模板（claims 3 / specification 4 / oa-response 3 / disclosure 2），使用 `{{variable}}` 占位符语法。`go:embed` 嵌为二进制内置模板，用户可在 `$MADY_HOME/doc-templates/` 覆盖或新增 |
| 7 | **Pipeline Atoms** | `agentcore/atom.go`：`Atom` 接口（Name/Description/Category/InputSchema/OutputSchema）+ 5 个具体原子操作（search / extract / compare / reasoning / approval-gate）+ 全局注册表（RegisterAtom / LookupAtom / ListAtoms / ListAtomsByCategory / AtomIndex）。`agentcore/plugin.go`：`PluginStage` 新增 `Atom` 字段，校验阶段验证原子引用有效性；3 个 plugin.json 已更新使用 atom 引用替代裸 tool 名 |
| 8 | **Agent 适配器模式** | `provider/adapter/`：`AgentAdapter` 接口（Name/Description/Detect/Spawn/Capabilities）+ `AgentSession` 接口（Send/Stream/Close）+ `AgentCapabilities` / `SpawnConfig` / `StreamChunk` 类型 + 全局注册表（RegisterAdapter / LookupAdapter / ListAdapters / DetectAll / AdapterIndex）。`claude.go`（Claude Code 适配器）+ `codex.go`（Codex CLI 适配器），均通过 `cliSession`（stdin/stdout 双向通信）实现 AgentSession |

### 技术细节

**文档模板库（借鉴 OD 的 template system）**：
- Markdown + YAML frontmatter 格式，`extractFrontmatter` 解析器
- `LoadDocTemplatesFromFS` 统一 fs.FS 接口，支持 embed.FS 和 os.DirFS
- `ResolveDoc` 使用 `strings.ReplaceAll("{{"+key+"}}", value)` 替换变量
- `DocIndex` 按 category 分组输出可读索引，适合 CLI 帮助

**Pipeline Atoms（借鉴 OD 的可组合 pipeline 原语）**：
- 5 个原子覆盖完整专利工作流：搜索 → 提取 → 对比 → 推理 → 审批
- `PluginStage.Atom` 引用注册表原子，实现从"粗粒度工具名"到"语义原子"的升级
- 向后兼容：Tool 字段仍有效（当 Atom 为空时）；两者共存时 atom 提供语义约束
- 原子通过 init() 自动注册，包导入即可使用

**Agent 适配器模式（借鉴 OD 的 agent composability）**：
- `detect → spawn → stream → capabilities` 四步统一接口
- `cliSession` 共享实现适用于所有 CLI-based 编码助手（Claude/Codex/Cursor/Copilot）
- `DetectAll` 运行时发现已安装的编码助手及其可用性
- `AdapterIndex` 生成终端诊断表格（✓/✗ 状态）
- mock 实现（mockAdapter/mockSession）便于单元测试

### 不影响
- 所有新增特性为纯新增，不修改任何已有生产代码路径
- Pipeline Atoms 通过 PluginStage.Atom 可选引用，不改变已有 plugin.json 的行为
- doc-templates 为独立目录，不影响已有文件
- 不涉及安全红线路径

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部 82 包通过，含新增 3 个测试文件，18+ 新用例）
- `go vet ./...` ✅（零警告）
- `tools/` 子模块 build/test/vet ✅
- 项目全量无新增 lint 问题


## 2026-07-18: Open Design 思路引入 —— Tier 2 实现

### 本次实现（Tier 2 — 中等影响力）

| # | 特性 | 改动 |
|---|------|------|
| 4 | **结构化 DOCUMENT_STYLE.md** | `domains/style.go`：`DocumentStyle` 类型（YAML 风格指南，包含 tone/voice/anti_patterns/disclaimers/citation/output_conventions），`SystemPrompt()` 方法生成注入文本。`styles/` 目录含 4 个默认风格（patent-standard / legal-standard / chat-friendly / assistant-neutral） |
| 5 | **专利工作流插件系统原型** | `agentcore/plugin.go`：`PluginManifest` 类型 + `ValidatePlugin` + `ScanPlugins` + `LoadPlugin`。`plugins/patent/` 含 3 个原型插件（novelty-analysis / infringement-check / oa-response），每个插件 = `plugin.json` + `SKILL.md`，pipeline 由可组合 stages 构成 |

### 技术细节

**DocumentStyle（借鉴 OD 的 DESIGN.md）**：
- YAML 格式，9 个结构化章节替代原有纯 prose `tone-style-guide.md`
- `SystemPrompt()` 自动生成 agent 注入文本，按领域（patent/legal/chat/assistant）可切换
- 反模式（anti_patterns）标注 severity（block/warn）便于自动化检测

**Plugin System（借鉴 OD 的 open-design.json + pipeline）**：
- 插件 = `plugin.json`（manifest）+ `SKILL.md`（agent contract）
- Pipeline 由可组合 stages 构成，每个 stage = id + tool + description
- 复用现有 `ValidateManifest` 校验框架（name regex / domain / guardrail_level）
- `ScanPlugins` 自动发现 `plugins/` 目录下的所有 `plugin.json`
- 3 个原型覆盖核心专利场景：新颖性分析（5 stages）、侵权比对（7 stages）、OA 答复（6 stages）

### 不影响
- 不涉及安全红线路径
- 原有 `tone-style-guide.md` 保留为人可读参考文档
- 插件与现有 SKILL.md 技能系统并行运作

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部通过，含新增 9 个测试文件，30+ 新用例）
- `go vet ./...` ✅


## 2026-07-18: 引入 Open Design 项目思路 —— 三阶段 Tier 1 实现

### 背景
深度研究 [open-design](https://github.com/nexu-io/open-design)（79K+ Stars）项目，
识别出 8 个可引入思路并按影响力排序。详见 plan.md 完整分析报告。

### 本次实现（Tier 1 — 高影响力 / 低风险）

| # | 特性 | 改动 |
|---|------|------|
| 1 | **MCP Install CLI** | `mcp/install.go` + `cmd/mady/mcp_install.go`：`mady mcp-install <agent>` 一键将 Mady 的 ACP 服务端接入 Claude Code / Codex / Cursor / Gemini CLI / GitHub Copilot 等编码 Agent |
| 2 | **SKILL.md 扩展字段** | `skill/skill.go` 新增 `MadyExtension` 结构体（mode/guardrail_level/approval_required/inputs/example_prompt/capabilities/handoff_allowed），`skill/frontmatter.go` 新增 YAML 解析，4 个现有 SKILL.md 已添加 `mady:` 扩展段 |
| 3 | **提示词模板库** | `prompt/templates/` 目录含 20 个 curated 模板（检索/分析/撰写/OA/交底书/法律），通过 `//go:embed` 编译进二进制；`prompt/loader.go` 提供 `LoadPrompts` / `LoadPromptsFromFS` / `ResolvePrompt` / `FindPromptByTrigger` / `FindPromptByName` API，`prompt/store.go` 提供 `PromptStore` 运行时缓存与覆盖机制 |

### 技术细节
- MCP Install 复用已有 `MCPServerConfig` / `MCPConfigFile` 类型，支持 `--list` / `--print` / `--uninstall`
- SKILL.md `mady:` 扩展使用 `gopkg.in/yaml.v3` 解析，向后兼容（无扩展字段的旧 SKILL.md 不受影响）
- 提示词模板使用 `{{variable}}` 语法，与现有 `prompt.Template` 渲染器独立并存

### 不影响
- 不涉及安全红线路径（不修改 `agentcore/handoff.go`、`guardrails/levels.go`、`tools/bash.go` 等）
- `tools/` 子模块无变更，多模块工作区规则无违反

### 验证
- `go build ./...` ✅
- `go test -race ./...` ✅（全部通过，含新增 5 个测试文件）
- `go vet ./...` ✅


## 2026-07-18: Code Review 全部 6 个问题修复

### 修复清单

| 优先级 | 问题 | 改动 |
|--------|------|------|
| 🔴 P1 | `formatEnhancedReport` silient 吞 baseline 错误 | 改用 `fmt.Fprintf(os.Stderr, ...)` 输出读文件/解析JSON的警告信息 |
| 🟡 P2 | `ReasoningStrategyRouter` 每次 BeforeModelCall 原地修改系统消息 | 改为 `cp := msg; cp.Content += hint; mcc.Request.Messages[i] = cp` 先拷贝再赋值 |
| 🟡 P2 | 4 份完全相同的 DoomLoop 配置拷贝 | 提取 `domains/lifecycle.go` 中的 `defaultDoomLoopHook()`，4 个领域统一调用；消除 4 处 `doomloop` 子包 import |
| 🟡 P2 | ChatAgent 收到"逐步推理"策略提示不适合聊天 | 创建 `chatSelector` 并将 `StrategyHintInjection = false`，保留 effort/budget 调整但不注入系统提示 |
| 🟢 P3 | `SuppressPersist` 位置与内容修改分离 | 合并为 `mcc.Response.SuppressPersist = g.config.Level >= LevelStrict` 紧跟在 `Content` 赋值之后 |
| 🟢 P3 | 集成测试 signal 收集 5 处重复模板代码 | 提取 `signalCapture` + `newDoomloopWithCapture` + `runStubAgent` 辅助函数，5 个测试各减少约 30 行重复代码 |

### 额外改进
- 集成测试新增 `signalCapture.requireDetected` / `requireNone` 方法，统一信号断言逻辑
- 新增 `runStubAgent` 辅助函数消除测试中重复的 `agentcore.New` + `defer agent.Close()` + `agent.Run` 样板代码
- 4 个领域文件移除不再需要的 `"github.com/xujian519/mady/agentcore/doomloop"` import


## 2026-07-18: ReasoningStrategyRouter 接入 4 个领域 Agent

### 改动
- **domains/assistant.go**：在 DoomLoop 之后接入 `ReasoningStrategyRouter`，
  通用助理场景默认使用 StepByStep → StructuredAnalysis → VerifiedThinking
  三级复杂度策略
- **domains/chat.go**：同助理模式接入，聊天场景也能在复杂问题时获得结构化推理支持
- **domains/patent.go**：专利分析场景接入，默认策略映射适配专利审查/三性分析需求
- **domains/legal.go**：法律分析场景接入，策略提示涵盖三段论推理和法律适用框架

### 接线模式
每个领域在 Lifecycle 链中的位置：
```
DoomLoop (安全第一: 死循环检测)
  → ReasoningStrategyRouter (优化: effort/budget + strategy hint)
  → CitationGate (引用核验)
  → Guardrails (内容安全)
  → Psychological / Tools (扩展)
```
`ReasoningStrategyRouter` 使用默认配置（`NewDefaultClassifier` +
`NewDefaultStrategySelector`），策略映射为 Low→StepByStep、
Medium→StructuredAnalysis、High→VerifiedThinking。领域可通过注入自定义
`ComplexityClassifier` 或 `StrategySelector` 调整行为。


## 2026-07-18: DoomLoop 集成测试——7 个端到端测试覆盖 5 个探测器和领域接线

### 改动
- **integration/doomloop_e2e_test.go**（新增）：7 个集成测试用例，覆盖：
  - `TestDoomLoopE2E_ToolCallLoop` — mock provider 返回相同工具调用 3 次以上，
    验证 ToolCallLoop 信号正确发射
  - `TestDoomLoopE2E_TextRepetition` — provider 返回重复文本 + 持续工具调用，
    使 Agent 保持循环，验证 TextRepetition 信号正确发射
  - `TestDoomLoopE2E_CircuitBreaker` — provider 返回超出熔断器上限的工具调用，
    验证 CircuitBreaker 信号正确发射
  - `TestDoomLoopE2E_EmptyResult` — 注册返回空结果的工具，验证 EmptyResult
    信号正确发射
  - `TestDoomLoopE2E_NormalOperation` — provider 返回多样化响应（不同文本/
    不同工具），验证无误报信号
  - `TestDoomLoopE2E_DomainLifecycleChain` — 验证 DoomLoop 与自定义
    LifecycleHook 正确组合为 LifecycleChain，BeforeAgentRun 按序调用
  - `TestDoomLoopE2E_DomainConfigAssistant` — 使用 `domains.AssistantAgentConfig`
    构建完整领域 Agent，验证 DoomLoop 接线不导致构造或运行错误

### 测试设计要点
- 使用 `//go:build integration` 构建标签，遵循既有集成测试约定
- doomLoopProvider 结构体封装 mock LLM 响应模式，通过 contentFn/toolCallsFn
  函数字段灵活控制每轮调用返回的内容
- 注意到文本重复探测器的关键约束：Agent 在无工具调用的文本响应后立即退出内层
  循环，因此 TextRepetition 测试必须让 provider 同时返回重复文本和工具调用，
  使 Agent 持续迭代以积累探测器历史


## 2026-07-18: 评估框架增强收尾——新度量注册 + `--format enhanced` CLI

### 改动
- **suite.go**：`DefaultEvaluator()` 新增 `ToolAccuracy` 和 `WorkflowQuality`
  两个度量，跑批时自动覆盖工具调用准确性和工作流执行质量
- **cli/cli.go**：新增 `FormatEnhanced` 输出格式；`formatEnhancedReport()`
  调用 `BuildEnhancedReport` + `FormatEnhancedReport`，支持指标分解、
  百分位分布、最差/最佳用例展示；支持 `BaselineFile` 从 JSON 文件加载
  前次结果做趋势对比（退化/改善用例高亮）
- **evaluator.go**：`BatchReport` 和 `CaseResult` 添加 JSON tags，确保
  baseline JSON 文件可被正确反序列化
- **cmd/mady/eval.go**：新增 `--format enhanced` 和 `--baseline <文件>` 标志
- **main.go**：帮助文本增加 enhanced 格式示例

### 用法示例

```bash
# 增强报告输出（含指标分解 + 百分位 + 最差/最佳用例）
mady eval --format enhanced --suite p2a --mode static

# 带 baseline 的趋势对比
mady eval --format json --suite p2a --mode static -o baseline.json
mady eval --format enhanced --baseline baseline.json --suite p2a --mode static
```


## 2026-07-18: 引用核验 P2a——CitationSource 知识源抽象 + S2 wiki 法条索引（82 条全覆盖）

### 背景
设计方案（docs/design/citation-verification-gate.md）§5 决策二规划了
三层核验源降级：S1 内嵌静态表（P1b 落地）→ S2 知识库法条索引 → S3 联网
法条库。P1b 的 S1 静态表仅覆盖 30 条手工精校条目，核验大量落 Unknown
放行。P2a 落地 S2：运行时从 wiki 拆分法条（`~/.mady/knowledge/wiki/
法律法规/法律/专利法-2020-拆分-*.md`）的 H3 标题（### 第X条 <标题>）
构建「条号 → 主题关键词」索引，专利法 2020 全 82 条覆盖。

### 改动
- `guardrails/citation_source.go`（新增）：`CitationSource` 接口
  （Topics/MaxArticle 两方法）+ S1 静态表适配（DefaultCitationSource）+
  `CompositeCitationSource`（关键词并集去重 primary 在前、上限取 primary
  非零优先）+ `CitationSourceFuncs` 函数适配器（knowledge/loader 不
  import guardrails，装配侧 cmd/mady 组合注入，依赖倒置）
- `guardrails/citation_gate.go`：CitationGateConfig 新增 Source 字段 +
  WithCitationSource 选项；VerifyCitationsWithSource 导出（nil 源退回
  S1，VerifyCitations 行为零改动）；verifyOne 参数化知识源。
  **交叉匹配仍只查 S1 精校词**（crossMatchTopics 不走注入源）——S2 自动
  标题词只参与本条自证，不参与张冠李戴判定，误报防线 #1 的延伸
- `knowledge/loader/law_index.go`（新增）：BuildLawArticleIndex 遍历
  拆分文件（排除目录/实施细则/part 文件），lawcite.ParseChineseNumber
  解析条号，标题按「与/及/和/、」切分子短语（≥2 字去重）。
  **v1 只索引专利法-2020**：实施细则-2023 因 2001/2010/2023 条号漂移
  （考试答案按旧口径引用，用 2023 主题核验必误报）暂缓，留 P3+ 版本感知
- `pkg/lawcite`：导出 ParseChineseNumber（chineseToArabic 包装，供索引器）
- `scripts/smoke_citation_gate`：lint 收口（exitAfterDefer 抽 runSmoke
  函数、两处 G703 #nosec 说明）

### 验收（硬性）
- S1 默认源回放（go run ./scripts/replay_citation_gate）：三层 93 题
  真实幻觉全命中、误报 0 ✅（verifyOne 参数化后行为等价）
- S1+S2 复合源回放（knowledge/loader/law_index_replay_test.go，
  TestCompositeSource_Replay）：L0 TP 3 / L1 TP 2 / L3 TP 4，三层误报
  均 0 ✅——S2 自证未掩盖任何已知幻觉
- 真实 wiki 索引断言：82 条全覆盖、第 9 条含「先申请原则」、第 22 条含
  「创造性」✅

- **影响范围**: guardrails 2 改 2 新 + knowledge/loader 1 新 2 测 +
  lawcite 1 改 + scripts 1 改
- **风险等级**: 低（未注入 Source 时 Gate 行为与 P1b 逐字节等价；
  S2 词不参与交叉匹配，复合源经回放实测零误报）
- **审查要求**: L1（guardrails/ 改动对照 docs/chat-assistant-architecture.md）
- **验证**: go vet/build 双模块 ✅ | golangci-lint run 0 issues ✅ |
  go test -race ./... 全绿 ✅ | 双重回放验收 ✅（见上）


## 2026-07-18: 从 XiaoNuo Agent 引入评估框架增强、死循环检测和推理策略编排

### 背景
对 XiaoNuo Agent（TypeScript Bun 单体仓库，34 个 @nuo/* 包）做深度分析后，
识别出三大值得引入 Mady 的能力：评估框架增强、DoomLoop 死循环检测器、
推理策略编排系统。按优先级依次实施。

### 改动清单

#### Plan 3：评估框架增强（agentcore/evaluate/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `evaluate/cli/cli.go` | **新增** | CLI 评估引擎：RunCLI / FormatResult / OutputResult，支持 static 和 live 模式，table/json/markdown 三种输出格式 |
| `cmd/mady/main.go` | **修改** | 注册 `mady eval` 子命令 |
| `cmd/mady/eval.go` | **新增** | eval 子命令实现，flag 解析（--suite/--domain/--case/--format/--mode/--model 等） |
| `evaluate/tool_accuracy.go` + `_test.go` | **新增** | 工具调用准确性度量：三维度评分（工具选择 + 参数准确 + 调用顺序），12 个测试 |
| `evaluate/workflow_quality.go` + `_test.go` | **新增** | 工作流执行质量度量：步骤完成度 + 顺序 + 精确性，支持 Pipeline/Parallel/Router 模式，11 个测试 |
| `evaluate/reflection.go` + `_test.go` | **新增** | Reflection 自我反思质量评估 + RubricJudge 可定制多准则 LLM 评分，16 个测试 |
| `evaluate/loader.go` + `_test.go` | **新增** | JSON 夹具加载系统：支持单文件/数组/原生数组格式，目录递归扫描，7 个测试 |
| `evaluate/testdata/tool_accuracy_fixtures.json` | **新增** | 工具准确性评估示例夹具 |
| `evaluate/report_enhanced.go` + `_test.go` | **新增** | 增强报告：指标分解（均值/最值/标准差/通过率）、百分位分布、趋势对比（baseline diff），10 个测试 |
| `evaluate/eval_integration_test.go` | **新增** | 集成测试：全流水线测试（加载→评估→报告），4 个测试 |

#### Plan 2：DoomLoop 死循环检测器（agentcore/doomloop/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `doomloop/doc.go` | **新增** | 包文档，6 种检测器概览 |
| `doomloop/doomloop.go` + `_test.go` | **新增** | 核心实现：ToolCallLoop（重复工具调用）、TextRepetition（重复文本）、Cycle（A→B→A→B 循环）、EmptyResult（空结果）、CircuitBreaker（总迭代次数）、CompactionBreaker（重复压缩摘要）。25 个测试，实现 agentcore.LifecycleHook 接口 |

#### Plan 1：推理策略编排系统（agentcore/）

| 文件 | 类型 | 说明 |
|------|------|------|
| `reasoning_strategy.go` + `_test.go` | **新增** | 策略选择器（6 种策略：step_by_step/structured_analysis/debate/tree_of_thoughts/verified_thinking/first_principles）、框架步骤系统（3 级复杂度→不同框架）、策略提示注入（BeforeModelCall 自动追加系统提示）。11 个测试 |

### 技术决策

1. **CLI 子包隔离**：将 CLI 引擎放在 `evaluate/cli` 子包而非 `evaluate` 包内，
   避免 benchmark 导入冲突（import cycle）。
2. **Metric 接口兼容**：所有新度量实现现有的 `Metric` 接口（`Name() + Compute()`），
   无需修改 Evaluator 核心。
3. **DoomLoop 通过 LifecycleHook 接入**：不影响 Agent 运行时的核心循环，
   检测器通过标准钩子机制注入，可独立启用/关闭。
4. **推理策略提示不侵入核心提示**：策略提示通过 `BeforeModelCall` 在每次调用前
   追加到 system message 末尾，不影响原始提示结构。

### 验证

- `go build ./...` ✅
- `go test ./agentcore/doomloop/` ✅（25 个测试）
- `go test ./agentcore/evaluate/...` ✅（全部 78+ 测试，含 7 个新测试文件的 60+ 测试）
- `go test ./agentcore/ -run "TestReasoning..."` ✅（11 个测试）
- `go build ./cmd/mady/` ✅
- `golangci-lint run ./...` 0 issues ✅（提交前收口：测试 nil Context →
  context.TODO、报告落盘权限 0644→0600、string 比较改 bytes.Equal、
  嵌入字段选择器简化、gofmt 全仓、main.go eval 错误返回检查）

- **影响范围**: agentcore/evaluate 10 新（含 cli 子包与 testdata）+
  agentcore/doomloop 2 新 + agentcore/reasoning_strategy 1 新 1 测 +
  cmd/mady 1 新 1 改
- **风险等级**: 低（均为新增包/钩子/子命令，默认 Agent 配置未改动；
  DoomLoop 与推理策略经 LifecycleHook 可选接入）
- **审查要求**: L1
- **验证**: 见上 ✅


## 2026-07-18: fix(tools) computer_use schema 畸形致 oMLX 500 + 引用核验端到端冒烟

### 背景
P1b 域接线后的端到端冒烟（domains.PatentAgentConfig 完整 hook 链 + oMLX
真实生成）首轮即 provider 500。curl 分层诊断（裸请求 ✅ / 简单工具 ✅ /
完整产品配置 ❌）定位到 1971fea 启用的 computer_use 工具 schema 畸形——
TUI assistant / patent agent 走 oMLX 等 OpenAI 兼容端点必现。

### 改动
- fix(tools)：`computerUseSchema()` 的 `"required": ["action"]` 误置于
  `properties` 内部（变成名为 "required" 的非法属性定义，值为数组），
  端点序列化该畸形 schema 时 500；移正为与 properties 平级的顶层键
  （对照 bash/browser 等全部工具写法一致，grep 确认为独例）。
  `TestComputerUseSchema` 补两条防回归断言（properties 内不得含
  "required" 键；顶层 required == [action]）——既有测试只断言字段
  存在故漏检
- feat(scripts)：`scripts/smoke_citation_gate` 端到端冒烟工具——完整
  PatentAgentConfig hook 链（CitationGate + Strict 护栏 + ApprovalGate）
  + oMLX 单题跑批（SMOKE_CASE / SMOKE_MAX_TOKENS / SMOKE_MAX_TURNS 可配）；
  SMOKE_FILE 离线模式对任意文本跑 guardrails.VerifyCitations 出判定报告

### 冒烟结论（patent_exam_2008_a31_02 单题）
- 链路通畅：修复后 1m27s 生成 2059 字，完整 hook 链无异常
- Gate 行为正确：本次生成引用 1 条且 Valid → 正确地未标注（非失灵）
- 幻觉命中实景（对 v0.8 L0 缓存真实幻觉答案离线核验）：4 条引用
  1 Valid / 2 Unknown / 1 Suspect，提示文案精确指出「专利法第47条第1款」
  用途"分案申请"与本条主题（无效宣告效力）不一致、更接近细则第42条

- **影响范围**: tools/ 2 文件改 + scripts/ 1 新增
- **风险等级**: 低（schema 修复经全部同类工具对照为独例）
- **审查要求**: L1
- **验证**: tools 模块 vet/build/test/lint 0 issues ✅ | 冒烟端到端 ✅ |
  离线核验演示 ✅


## 2026-07-18: 启用 computer_use 桌面控制工具（Assistant + Patent Agent）

### 背景
用户需要在专利检索与下载场景中控制本地桌面浏览器，操作 CNIPA/Google Patents 等网站。
`computer_use` 工具已在 `tools/` 中完整实现（macOS/Linux/Windows 三平台后端 + 安全拦截 + 测试），
但此前在所有领域 Agent 中均被禁用。

### 改动
- `domains/assistant.go`：`ExtensionConfig` 新增 `ComputerUse: true`，从 `DisableTools` 移除 `tools.ToolComputerUse`
- `domains/patent.go`：同上
- Legal Agent 保持原有禁用状态（暂无浏览器操作场景）
- 默认审批模式为 `COMPUTER_USE_APPROVAL=none`（仅拦截危险操作，不额外提示）

### 影响范围
- `domains/assistant.go`（+1 行 / -1 行）
- `domains/patent.go`（+1 行 / -1 行）

### 审查要求
- L1（简单配置变更，不涉及敏感路径）
- `computer_use` 已在 SECURITY.md 中列为需用户授权的敏感工具，用户已知悉


## 2026-07-18: 修复 PlanCompiler 边连接 bug——多假设子图被完全绕过

### 背景
`TestDrafting_WorkflowTool` 集成测试写入失败：步骤 4（StrategyMultiHypothesis）未产出任何内容，
步骤 5 的链式节点却消耗了本属于步骤 4 的 LLM 调用。调试发现 Pregel 执行仅 4 次 LLM 调用
（应 5 次），步骤 4 的多假设子图被完全绕过。

### 根因
`CompilePlanToGraph` 中，步骤间的边连接使用 `terminal`（子图最后一个节点）而非 `entry`
（子图入口节点）：
```go
// 原代码：
g.AddEdge(prevTerminal, terminal)  // terminal = rejectName（多假设子图末端）
// 应改为：
g.AddEdge(prevTerminal, stepEntry) // stepEntry = aThink（多假设子图入口）
```
对于 `StrategyChain` 和 `StrategyReact`，入口 == 末端（单节点或 think→observe），
原错误被掩盖。但对 `StrategyMultiHypothesis`，`terminal = rejectName` 而 `entry = aThink`，
前一步直接连到子图末端的 rejection 节点，整个多假设子图从不激活。

### 改动
- `domains/reasoning/plan_compiler.go`：引入 `stepEntry` 和 `stepTerminal` 变量，
  在 switch 分支中分别记录每个步骤的入口和终止节点。边连接改为
  `g.AddEdge(prevTerminal, stepEntry)`，`prevTerminal = stepTerminal`。
  将 `if i == 0 { entryName = ... }` 移出 switch 分支统一在循环体处理。

### 影响范围
- `domains/reasoning/plan_compiler.go`（`CompilePlanToGraph` 方法重写步骤间边连接逻辑）

### 风险等级
低（修正逻辑正确性；既有 graph 测试全部通过；4 个 drafting 集成测试全部通过；
全部 26 个集成测试回归绿色）

### 验证
- `go build ./...` ✅
- `go test ./graph/...` 52 测试全部通过 ✅
- `go test ./domains/reasoning/...` 全部通过 ✅
- `go test -tags integration ./integration/...` 26 集成测试全部通过 ✅
- `TestDrafting_WorkflowTool` 修复后 5 步全部输出、所有 LLM 调用正确 ✅

### Code Review 收尾修复

1. **`buildChainStep` 返回类型统一**：改为返回 `(string, string, error)`（entry == terminal 相同），
   使 `CompilePlanToGraph` 的 switch 四个分支全部用一致的 `(entry, term, err)` 模式，
   消除 StrategyChain 分支需单独赋值的视觉差异。
2. **提取 `injectDraftingTool` helper**：消除 `globalDraftingRunner` nil 守卫在
   `PatentAgentConfig` 和 `BuildProjectAgent` 两处的重复，各从 5 行压缩为 1 行调用。


## 2026-07-18: 知识系统全面优化（5 阶段）

### 背景
知识系统（knowledge.db 6.5GB / 8 万文档 / 14 万分块 / 21 万知识图谱节点）已建设完毕，
但向量检索链路因环境变量未注入进程而被禁用，实际仅使用 FTS-only（BM25 关键词搜索），
浪费了预先投入的 144K BGE-M3 向量索引和 RRF 融合架构。

### Phase 0 — 环境激活
- **`.envrc`**（新建）：direnv 自动加载 `.env`（`dotenv` 指令）
- **`cmd/mady/main.go`**：导入 `github.com/joho/godotenv/autoload`，Go 进程自动读取 `.env`
- **依赖**: 新增 `github.com/joho/godotenv v1.5.1`
- **效果**: OMLX_API_KEY / KNOWLEDGE_RERANK 等环境变量自动注入进程，向量检索链路激活

### Phase 1 — laws-full.db FTS5 索引
- **`~/.mady/knowledge/laws-full-local.db`**：复制 laws-full.db（152MB, 9121 部法规）为本地独立文件并添加 FTS5 trigram 索引（law_fts 虚拟表）
- **`knowledge/sqlite/store.go`**：`OpenLawsDB()` 自动检测 law_fts FTS5 表；`SearchLaws()` 双路搜索——3+ 字符用 FTS5 BM25 排序，短查询/无 FTS 时回退 LIKE
- **`cmd/mady/knowledge.go`**：优先加载 `laws-full-local.db`（FTS5 版），回退 `laws-full.db`（原始版）
- **效果**: 法律搜索从 LIKE 模糊匹配升级为 BM25 排序，相关度大幅提升

### Phase 2 — SQL 回退向量搜索并行化
- **`knowledge/sqlite/store.go`**：`VectorSearch()` 的 SQL fallback 路径从单线程 2000 行/批次扫描改为并行 goroutine（同 CPU 核数），每个 worker 维护 min-heap
- **新增 `vectorSearchSQLParallel()`**：并行 SQL batch 扫描 + 结果合并
- **效果**: SQL 回退搜索从 **13.8 秒降至 188ms**（73 倍提速）

### Phase 3 — EvalHook 默认启用
- **`knowledge/eval.go`**：`DefaultEvalConfig()` 中 `Enabled: false → true`
- **`knowledge/extension.go`**：`NewExtension()` 自动创建 EvalHook；`BackendHook()` 通过 `AppendLifecycle` 将 EvalHook 与后端检索钩子组合
- **效果**: 每次模型调用后自动评估 Faithfulness / AnswerRelevancy / ContextPrecision 并发送到事件总线

### Phase 4 — 启动诊断增强
- **`knowledge/sqlite/store.go`**：新增 `Stats()` 方法，返回文档/分块/向量/维度/内存统计
- **`knowledge/graph/store.go`**：新增 `NodeTypeCounts()` 方法，按节点类型统计
- **`cmd/mady/knowledge.go`**：启动时输出详细诊断（文档/分块/向量数、图谱节点类型分布）
- **效果**: 启动日志从 "active" 一句扩展为完整的知识库统计报告

### Phase 5 — Benchmark 回归套件
- **`Makefile`**：新增 `bench-knowledge` 目标
- **`docs/performance-baseline.md`**（新建）：性能基线文档
- **效果**: 每次知识系统变更后可一键运行基准测试并对比回归

### 性能基线（M4 Pro, 2026-07-18）

| 操作 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| FTS 全文搜索 | 44ms | **11ms** | 4× |
| 向量检索 (内存索引) | 31ms | **18ms** | 1.7× |
| 端到端检索 (FTS+向量+RRF) | 44ms | **30ms** | 1.5× |
| SQL 回退向量搜索 | **13.8s** | **188ms** | 73× |
| 向量预加载 (144K) | 391ms | **269ms** | 1.5× |
| 图谱增强 + 法规 FTS5 | 未测量 | 实时 | — |

### 影响范围
- `cmd/mady/main.go`, `cmd/mady/knowledge.go`, `Makefile`, `.envrc`, `go.mod`, `go.sum`
- `knowledge/eval.go`, `knowledge/extension.go`, `knowledge/ext_test.go`
- `knowledge/sqlite/store.go`, `knowledge/graph/store.go`
- `docs/performance-baseline.md`, `docs/decisions/AI_CHANGELOG.md`

### 风险等级
- 低（不修改现有 API 签名，测试全绿）

### 审查要求
- L1

### 验证
- `go build ./...` ✅ | `go test -count=1 ./knowledge/... ./retrieval/... ./memory/...` ✅ | `make bench-knowledge` ✅


## 2026-07-18: MCP 发现超时优化

### 背景
`mady tui` 启动缓慢（15-20s），经分析发现主因是 `~/.claude.json` 中 9 个 MCP 服务器
并发发现导致 10 秒超时等待。其中 3 个服务器（zai-mcp-server 通过 npx 启动 >15s、
jina-ai-mcp-server 和 tolaria 模块路径不存在）拖慢了整个发现流程。

### 改动
- **`~/.claude.json`**：移除 3 个失效/慢速 MCP 服务器（zai-mcp-server、jina-ai-mcp-server、tolaria），
  保留 6 个正常运行的服务（codegraph、professional-router、gemma4-multimodal、web-reader、web-search-prime、zread）
- **`mcp/config_discovery.go`**：MCP 发现总超时从 10s 缩短至 **3s**，
  即使个别服务器偶发延迟也不会阻塞启动流程

### 效果
| 指标 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| TUI 首帧渲染 | ~15-20s | **~4s** | ≈75% |
| Agent 就绪 | ~18s | **~6s** | ≈67% |

### 影响范围
- `mcp/config_discovery.go`
- `~/.claude.json`（用户外部配置，非仓库文件）

### 风险等级
- 低（不修改现有 API，测试全绿）

### 审查要求
- L1

### 验证
- `go build ./...` ✅ | `go test ./mcp/...` ✅

---

## 2026-07-20: Code Review 反馈修复 — TUI SIGINT / Paste 异步 / 测试覆盖

- **date**: 2026-07-20
- **scope**: tui, domains
- **summary**: 针对代码审查报告的 8 项问题全部修复：①去掉 `ctrl+c` 键绑定避免截获 SIGINT；②Paste 改为异步 Cmd 模式（`onPaste` 返回 `core.Cmd`，通过 `pastePendingCmd` 传递到 TUI 事件循环执行，clipboard 读取在 goroutine 中完成）；③新增 `insertText` 批量插入方法（单次 undo 快照 + 单次 onChange，避免大文本逐 rune 插入性能问题）；④新增 7 个单元测试（handleCopy、handlePaste 异步 Cmd 流程、insertText 单行/多行/替换选择、autocomplete 双触发守卫、trigger 正常前置）；⑤复制通知从截断文本改为字符计数（`📋 已复制（N 字符）`）；⑥确认 `MaxToolCalls` 不在本改动范围内（`agentcore/budget.go` 预算控制）。
- **rationale**: 代码审查发现 2 个严重问题（SIGINT 截获、paste 阻塞事件循环）和 3 个重要问题（无测试、大文本逐 rune 插入性能、goroutine 线程安全），逐一根因修复。
- **files**: tui/terminal/keybindings.go, tui/component/editor.go, tui/component/editor_edit.go, tui/component/editor_test.go, tui/component/autocomplete_test.go, tui/chat/chat_app.go
- **commit**: (工作区未提交)

---

## 2026-07-20: 鼠标文字选中的流畅度修复 — ?1000h → ?1002h

- **date**: 2026-07-20
- **scope**: tui
- **summary**: 鼠标选中文字从 `?1000h`（basic click tracking）切回 `?1002h`（button-event tracking）。Editor 和 ChatHistory 已有完整的 `handleMouse` 拖选实现（press/motion/release），但 `?1000h` 不上报 motion 事件导致内置选中失效，而终端 OS 原生选中在追踪模式下半死不活。`?1002h` 让 TUI 内置选择机制完整运作（ANSI 蓝底高亮 + ⌘+C 复制/右键复制），终端原生选中被禁用但 TUI 内选中体验流畅。
- **rationale**: 用户反馈 `?1000h` 下鼠标选文"非常不流畅"。根因是终端知道鼠标追踪开着但只给 press 事件，原生选择响应迟钝。TUI 已有完整的内置选择实现，只需恢复 motion 事件即可。
- **files**: tui/tui_input.go, tui/chat/chat_app_layout.go
- **commit**: (工作区未提交)

---

## 2026-07-20: Phase 1 Review Gate 浮层实现

- **date**: 2026-07-20
- **scope**: tui, agentcore
- **summary**: 实现 TUI Overlay 优化方案 Phase 1：复核门（Review Gate）浮层。新建 `tui/component/review_gate.go`，提供证据清单（展开/折叠）、检查项打勾、置信度进度条、Pass/Back/Block 三按钮操作面板，支持键盘导航（Tab 切区 + ↓/↑ 遍历 + Space 切换 + p/b/f 快捷键）。`agentcore/event.go` 的 `ApprovalPromptEvent` 新增 `Data` 字段（向后兼容，nil 默认），经 adapter 映射到 `ApprovalPromptChatEvent.Data`。`ChatApp` 新增 `OpenReviewGate(ReviewGateData)` / `CloseReviewGate()` 方法，在 `onApprovalPrompt` 检测到结构化数据时自动打开浮层。三个新键绑定 `tui.review.pass`(p) / `tui.review.back`(b) / `tui.review.block`(f) 注册到 KeybindingsManager。全量 12 个测试覆盖渲染、导航、交互、空状态、焦点指示器，全部通过（`go test ./tui/component/... ./tui/chat/... ./tui/agentadapter/...`）。
- **rationale**: 设计稿 `docs/design/tui-overlay-optimization-v0.1.md` 将 Review Gate 列为 P1 优先级——在没有复核门浮层时，用户只能通过历史消息中的静态审批卡片做决策，无法快速浏览证据摘要、检查清单和风险项。浮层方案让用户在审批决策时保持上下文连续，无需滚动翻阅历史。
- **files**: tui/component/review_gate.go (new), tui/component/review_gate_test.go (new), tui/component/domain.go, tui/chat/events.go, tui/chat/chat_app.go, tui/chat/chat_app_stream.go, tui/agentadapter/adapter.go, agentcore/event.go, tui/terminal/keybindings.go, docs/design/tui-overlay-implementation-plan.md
- **commit**: (工作区未提交)

---

## 2026-07-20: Phase 2 主界面改造 — Judgment View

- **date**: 2026-07-20
- **scope**: tui
- **summary**: 实现 TUI Overlay 优化方案 Phase 2：主界面 Judgment View（当前判断视图）。新建 `tui/component/judgment_view.go`，在主界面顶部渲染阶段/状态标签行、判断摘要、置信度条（三色：低/中/高）、待确认项（最多 3 条）、上下文摘要（最多 4 条）、动作提示行。三种渲染模式：collapsed（idle/streaming 仅状态行）、normal（done/ready 追加判断行）、expanded（awaiting_review/blocked 全量展开）。修改 `chatLayout.buildFlex()` 在 header 与 history 之间插入 judgmentView（空时自动隐藏）；新增 `ChatApp.SetJudgment/SetPhase/SetMode/SetJudgmentPending/SetJudgmentContext/SetJudgmentConfidence` 公开方法；`onAgentStart/onMessageDelta/onAgentEnd/onAgentError/onApprovalPrompt` 事件处理器自动调用 `updateJudgmentView()` 同步状态。11 个单元测试覆盖渲染、空状态、置信度标签、截断校验、三模式验证、降级标记、Setter 完整性。
- **rationale**: 设计稿将 Judgment View 列为 P2 优先级——没有判断视图时，主界面偏"聊天流承载"，用户无法一眼看懂当前阶段、判断和待确认项。判断视图让主界面从"聊天记录"提升为"当前判断页"，在 awaiting_review 模式下全量展开辅助复核决策，idle/streaming 模式折叠减少视觉负担。
- **files**: tui/component/judgment_view.go (new), tui/component/judgment_view_test.go (new), tui/chat/chat_app_layout.go, tui/chat/chat_app.go, tui/chat/chat_app_stream.go
- **commit**: (工作区未提交)

---

## 2026-07-20: Phase 3 浮层分类统一 — OverlayCategory

- **date**: 2026-07-20
- **scope**: tui
- **summary**: 实现 TUI Overlay 优化方案 Phase 3：浮层分类统一。新增 `tui/overlay.go` 中 `OverlayCategory` 类型和 `Overlay.Category` 字段（4 类：Selection 选择型 / Review 审阅型 / Gate 复核型 / System 系统型），`DefaultOverlaySize()` 分类默认尺寸函数。`chat_app.go` 新增 `OverlayCatSelection/Review/Gate/System` 常量，`overlayHandle` 新增 `category` 字段和 `OverlayCategory()` 访问器，`OverlayOpts.Category` 可选字段。`chat_bridge.go` 中 `PushOverlay` 通过类型断言 `interface{ OverlayCategory() int }` 从 `overlayHandle` 传播分类到 `tui.Overlay`。现有覆盖层已对应归类：KeyHelp→Review、ReviewGate→Gate、Settings→Review。
- **rationale**: 设计稿指出浮层缺少清晰分工，不同类型的内容容易混成"都可以弹出来"。分类统一后，后续可以按类施加一致的行为规则（尺寸、关闭规则、焦点策略），为 Phase 4 系统态浮层做好准备。
- **files**: tui/overlay.go, tui/chat/chat_app.go, tui/chat_bridge.go, cmd/mady/settings_panel.go
- **commit**: (工作区未提交)

---

## 2026-07-20: 代码审查修复 — 6 项改进

- **date**: 2026-07-20
- **scope**: tui, agentadapter
- **summary**: 根据代码审查报告修复 6 项问题。① P0: `map[string]any` 数据边界 → 定义 `chat.ReviewGatePayload` 结构体，将解析逻辑移至 `agentadapter.parseReviewGateData()`，`openReviewGateFromData` 改为接收类型安全的 `*ReviewGatePayload`。② P1: 函数职责归位 → `openReviewGateFromData` 和 `submitApprovalCommand` 从 `chat_app_stream.go` 移至 `chat_app.go`（与 `OpenReviewGate` 相邻）。③ P1: 合并 7 个重复 setter → 删除 `SetJudgment/SetPhase/SetMode/SetJudgmentPending/SetJudgmentContext/SetJudgmentConfidence`，保留 `JudgmentView()` 访问器 + 新增单入口 `UpdateJudgmentView()`。④ P2: 死字段 `labelOverride` → 新增 `JudgmentView.SetStatusLabel()` 暴露为受控 API。⑤ P2: `matches()` fallback → 添加注释说明测试用途。⑥ P3: `OverlayCategory` 传播验证 → 新增 `TestTuiAppHostPropagatesCategory`，覆盖 5 种分类路径。所有构建和测试通过。
- **rationale**: 代码审查发现 `map[string]any` 已固化为从 agentcore 到 TUI 的数据通道模式，80 行手工拆包代码位于错误层且类型不安全。修复后数据契约由 `ReviewGatePayload` 编译器保证，解析边界收束到 adapter 的单一路径。
- **files**: tui/chat/events.go, tui/chat/chat_app.go, tui/chat/chat_app_stream.go, tui/agentadapter/adapter.go, tui/component/judgment_view.go, tui/component/judgment_view_test.go, tui/component/review_gate.go, tui/chat_bridge.go, tui/chat_bridge_test.go
- **commit**: (工作区未提交)

---
## 2026-07-20: Phase 4 — 系统态浮层 (System Status Overlay)

- **date**: 2026-07-20
- **scope**: tui/component, tui/chat, tui/terminal
- **summary**: 新增 SystemStatus 系统态浮层组件，用于展示系统运行条件（模式/降级原因、最近事件、当前影响），透明但不扰民。变更包括：① 新增 `tui/component/system_status.go` — SystemStatus 组件（Focusable Component，支持 mode/events/impacts 三区渲染、[l] 详细日志 / [Esc] 返回键位、dirty-flag 缓存）。② 新增 `tui/component/system_status_test.go` — 11 项测试覆盖渲染、事件截断、空影响隐藏、焦点、键位处理、缓存失效。③ 扩展 `tui/chat/chat_app.go` — 新增 `SystemStatusData` 结构体、`systemStatusOverlay` 字段、`OpenSystemStatus`/`CloseSystemStatus` 方法（锁定序同 `ToggleKeyHelp` 模式）。④ 扩展 `tui/terminal/keybindings.go` — 注册 `tui.system.open` 键位绑定（默认 `s`）。⑤ 改造 `tui/chat/chat_app_layout.go` — `updateJudgmentView()` 设置 action hints 包含 `[s] 系统态`；layout.Update 中拦截 expanded 状态下的 `s` 键触发 OpenSystemStatus。⑥ 扩展 `tui/component/judgment_view.go` — 新增 `Mode()` 和 `IsExpanded()` 公共访问器。
- **rationale**: 设计稿要求降级/阻塞/异常状态必须真实可见但不侵占主界面注意力。系统态浮层作为 Phase 3 分类的 `OverlaySystem` 类实现，默认尺寸 50/40，通过 judgmentView 动作行的 `[s]` 入口在 expanded 模式下可一键打开。最近事件最多显示 3 条，无影响时自动隐藏"当前影响"区域。
- **files**: tui/component/system_status.go, tui/component/system_status_test.go, tui/chat/chat_app.go, tui/chat/chat_app_layout.go, tui/component/judgment_view.go, tui/terminal/keybindings.go
- **commit**: (工作区未提交)

---
## 2026-07-21: Sprint 1 文件规模治理 — R1/R2/R6

- **date**: 2026-07-21
- **scope**: tools, agentcore, tui
- **summary**: 执行文件规模治理和超长函数拆分 Sprint 1，完成三项重构：
  - **R1**: `tools/browser_legacy.go` 工具构造函数去重（1131→~930 行），提取 `toolConfig`/`toolParams`/`touchSession`/`chromedpBackends`/`isChromedpBackend`/`refPrefixNormalized`/`refLookupErr`/`refLookupWithFallback`/`chromedpEval` 等 9 个辅助函数和数据结构，合并 chromedp 后端 dispatch 重复分支。
  - **R2**: `agentcore/agent_run.go` `runInnerLoop` 拆分（237→~75 行编排），提取 `runPreTurn`/`runModelTurn`/`runAfterModelCall` 三个方法。
  - **R6**: `tui/chat/chat_app.go` `newChatApp` 拆分（159→~25 行编排），提取 7 个构造函数（`applyChatDefaults`/`newChatHistoryWithConfig`/`newChatEditor`/`newChatHeader`/`newChatAutocomplete`/`newChatLayout`/`bindChatEditorEvents`）。
- **rationale**: 三个文件均包含超过 150 行的单一函数/构造函数，违反"一个函数只做一件事"原则，不利于单元测试和后续维护。
- **verification**: `go build ./...` + `go vet ./...` + `golangci-lint` (0 issues) + `go test -race ./agentcore/... ./tui/chat/...` 全绿，`tools/` 子模块独立 `go build ./... && go vet ./...` 通过。
- **files**: tools/browser_legacy.go, agentcore/agent_run.go, tui/chat/chat_app.go

---
## 2026-07-21: Sprint 2 文件规模治理 — R3/R4/R5

- **date**: 2026-07-21
- **scope**: tui/component, acp, tui/chat
- **summary**: 执行文件规模治理和超长函数拆分 Sprint 2，完成三项重构：
  - **R3**: `tui/component/syntax.go` `tokenize` 拆分（200→~40 行编排），提取 6 个辅助方法（`tokenizeComment`/`tokenizeLineComment`/`tokenizeString`/`tokenizeNumber`/`tokenizeIdent`/`tokenizePunct`），各自处理一种词法类别。
  - **R4**: `acp/server.go` 方法按职责分组，添加 9 个分组注释头（Server lifecycle / Outbound requests / Permission / File operations / Response helpers / Request dispatch / Method handlers / State builders / Utilities），无行为改动。
  - **R5**: `tui/chat/chat_history_render.go` 将 4 个选区业务方法（`isSelectionEmptyLocked`/`GetSelectedText`/`ClearSelection`/`getSelectedTextLocked`）提取到独立文件 `chat_history_selection.go`，渲染逻辑与选区逻辑分离。
- **rationale**: 延续 Sprint 1 文件规模治理计划，三个文件仍然包含超长函数或职责混杂的方法集合。
- **verification**: `go build ./...` + `go vet ./...` + `golangci-lint` (0 issues) + `go test -race ./tui/component/... ./tui/chat/... ./acp/...` 全绿，`tools/` 子模块独立验证通过。
- **files**: tui/component/syntax.go, acp/server.go, tui/chat/chat_history_render.go, tui/chat/chat_history_selection.go

---
## 2026-07-21: 全量质量审阅修复

- **date**: 2026-07-21
- **scope**: docs, agentcore, cmd, pkg/agentconfig, tools
- **summary**: 根据 2026-07-21 质量审阅报告，修复 5 个发现 + 记录 2 个已知风险。

  **修复项：**
  - **docs/design/p3-blind-test-case-jiaoche-daizhou.md**: 替换硬编码本机路径为 `{案件包目录}` 占位符，消除外发给组织者时的路径暴露风险。
  - **agentcore/agent_run.go**: 恢复 `context.Canceled` 处理分支的意图注释（`// User interrupted — emit clean end event instead of cryptic error.`），该注释在 Sprint 1 重构 `runInnerLoop` 提取 `runModelTurn` 时丢失。
  - **cmd/mady/framework.go**: `KnowledgeBackend` 类型断言失败或为 nil 时添加 `slog.Debug` 日志，避免运维人员排查时"检索器静默未启用"。
  - **pkg/agentconfig/provider.go**: `ResolveContextWindow` 中 `glm-5` 前缀拆分为 `glm-5.` 和 `glm-5v` 两条，防止未来 `glm-50`/`glm-500` 等模型误匹配 1M 上下文窗口。
  - **tools/browser_tool_debug.go**: 文件头部添加注释说明 `RequireActiveSession` 定义于 `browser_session.go`，降低跨文件阅读时的认知负担。

  **记录为已知风险：**
  - **段注释同步风险**: `acp/server.go` 和 `tools/browser_legacy.go` 中使用的职责分组段注释（`// --- Server lifecycle ---` 风格）提供了良好的结构导航，但新增/删除函数时需要同步更新。标记为已知维护负担，建议代码审查时核对。
  - **syntax.go 纯重排验证**: `tui/component/syntax.go` 的 340 行变更为纯提取式重构（`tokenize` 拆为 6 个子方法），无行为改动。已在 Sprint 2 CHANGELOG 中记录验证结果（全绿），但缺少针对每个子方法的独立单元测试。建议后续添加 `tokenizeComment`/`tokenizeString` 等子方法的单元测试以覆盖边界条件。
- **rationale**: 质量审阅发现的中等和轻微问题应在合并前修复，保持代码库的一致质量水位。
- **verification**: `go build ./...` + `go vet ./...` + `go test ./agentcore/... ./cmd/mady/... ./pkg/agentconfig/... ./tools/...` 全绿，`tools/` 子模块独立验证通过。
- **files**: docs/design/p3-blind-test-case-jiaoche-daizhou.md, agentcore/agent_run.go, cmd/mady/framework.go, pkg/agentconfig/provider.go, tools/browser_tool_debug.go, docs/decisions/AI_CHANGELOG.md

---
## 2026-07-21: TUI 四轮优化 Sprint — 启动瘦身/状态机/错误回显/系统面板

- **date**: 2026-07-21
- **scope**: cmd/mady, tui/chat
- **summary**: 四轮迭代持续优化 TUI 启动体感、状态管理、错误处理和系统可观测性。

  **Sprint 1 — 启动瘦身（4 新文件 + 3 修改）**
  - `cmd/mady/tui_deferred.go`: 新增 `DeferredInit` 结构体，支持 9 类初始化任务（wiki/store/rules/skills/mcp/workspace/tools/plugins/memory/reasoning）后台并发执行，`StartAll()` 聚合错误。
  - `cmd/mady/tui_storage.go` + `tui_storage_test.go`: 新增 storage 探针（session 目录/settings/approval DB 三种预检），启动时显示降级标签。
  - `cmd/mady/framework.go`: `setupFrameworkContext` 拆分为同步路径（Provider/MadyHome/BaseConfig/Manifests）与延迟路径（`registerDeferredTasks`），新增 `executeSyncRemaining` 供 serve/acp 使用。
  - `cmd/mady/tui.go` + `tui_session.go` + `tui_session_config.go`（修改）: 探针结果注入状态栏、系统消息显式告知存储不可用；`extendConfig` 按需读取 RuleEngine/WikiStore。

  **Sprint 2 — 显式状态机（4 修改 + 测试）**
  - `tui/chat/state.go`: `AppState` 新增 `StateInitializing` / `StateFailed`（共 7 状态）；`Transition()` 支持新状态；`evtAgentReady` 事件。
  - `tui/chat/chat_app.go`: `chatModel` 增加 `state AppState` 字段（初始化为 `StateInitializing`），`MarkAgentReady()` / `MarkAgentFailed()` 方法；`Running` / `StreamID` / `ActiveTools` 标记为 deprecated 兼容 shim。
  - `tui/chat/chat_app_stream.go` + `chat_app_tool.go`: 事件处理器（onAgentStart/onMessageDelta/onAgentEnd/onToolStart/onToolEnd/onApprovalPrompt/onCompactionStart 等）全部通过 `Transition()` 驱动状态机；补全遗漏的 `updateJudgmentView()` 调用。
  - `tui/chat/chat_app_layout.go`: `updateJudgmentView()` 从 FSM state 派生状态，新增 `buildSystemStatusData()`。
  - `cmd/mady/tui_session_agent.go`: `initializeAgentAsync` 成功→`MarkAgentReady()`，panic→`MarkAgentFailed()`。
  - `state_test.go` + `chat_app_test.go`: FSM 集成测试覆盖全生命周期（Initializing→Ready→Streaming→ToolRunning→AwaitingConfirm→Compacting→Failed）。

  **Sprint 3 — 统一错误出口（1 新文件 + 1 修改）**
  - `cmd/mady/tui_session_agent.go`: 新增 `ErrorSeverity` 枚举（RunFailure/PostProcessFailure/Degradation）、`showUserError()` 统一回显；`submitInput`/`resumeIfInterrupted`/`rebuildAgent` 中用 `showUserError` 替代 `log.Printf`；`rebuildAgent` 增加 panic recover。
  - `cmd/mady/tui_session_agent_test.go`: 新增严重度标签、错误回显、panic recover 测试。

  **Sprint 4 — 系统态面板（2 修改）**
  - `tui/chat/chat_app_layout.go`: `s` 键触发 `buildSystemStatusData(app, mode)`，展示 FSM 状态、活跃工具、判断摘要、审批状态、持久化状态、上下文窗口；`stateLevel()` 色阶辅助。
  - `tui/chat/chat_app_test.go`: 新增系统面板数据绑定测试。

- **rationale**: 四轮 Sprint 目标连贯——先让启动不再阻塞首帧（Sprint 1），再用显式状态机替代布尔字段漂移（Sprint 2），然后将静默日志错误转化为用户可见提示（Sprint 3），最后通过系统面板为调试和运维提供运行时快照（Sprint 4）。逐轮验证，每轮均通过 `go build ./...` + `go vet ./...` + `go test -race ./tui/... ./cmd/mady/...` + `cd tools && go build ./... && go vet ./...`。
- **verification**: `go build ./...` ✓ | `go vet ./tui/... ./cmd/mady/...` 0 警告 ✓ | `go test -race ./tui/... ./cmd/mady/...` 10/10 包通过 ✓ | `cd tools && go build ./... && go vet ./...` ✓
- **files**:
  - Sprint 1: cmd/mady/tui_deferred.go (new), cmd/mady/tui_storage.go (new), cmd/mady/tui_storage_test.go (new), cmd/mady/framework.go, cmd/mady/tui.go, cmd/mady/tui_session.go, cmd/mady/tui_session_config.go
  - Sprint 2: tui/chat/state.go, tui/chat/state_test.go, tui/chat/chat_app.go, tui/chat/chat_app_stream.go, tui/chat/chat_app_tool.go, tui/chat/chat_app_layout.go, tui/chat/chat_app_test.go, cmd/mady/tui_session_agent.go
  - Sprint 3: cmd/mady/tui_session_agent.go, cmd/mady/tui_session_agent_test.go (new)
  - Sprint 4: tui/chat/chat_app_layout.go, tui/chat/chat_app_test.go

---

## 2026-07-21: TUI 欢迎页面 — PrintWelcome 结构化启动信息

### 背景
此前 TUI 启动后仅显示一行 `app.PrintSystem("Mady 已就绪")`，缺少品牌识别和命令速查引导。参考 Claude Code 欢迎页的极简风格（无全屏 Splash、不抢占编辑器焦点），为 Mady TUI 新增结构化欢迎信息。

### 变更（2 文件）

1. **`tui/chat/chat_app.go`**：新增 `PrintWelcome(provider, model, mode, project string)` 方法
   - 格式：居中 `◈ Mady ◈` 品牌标题 + `中观智能体 · β` 副标题 + `─── 快速命令 ───` 分隔线 + 8 条核心命令双列布局 + `─── 当前上下文 ───` 分隔线 + 提供方/模型/模式/案件摘要 + `💡 输入 / 查看命令...` 引导提示
   - 宽度自适应：≥60 列双列命令布局，<60 列回退单列
   - 复用 `ChatHistory` 的 `RoleSystem` 渲染管线（`▌` 前缀 + SystemStyle 颜色），零新增组件
   - 所有颜色使用 `theme.CurrentPalette()` 中已有语义 Token（Accent/Dim/Border/Text）
   - 新增辅助函数 `centeredLabel()` 生成居中 `─── 标签 ───` 分隔线
2. **`cmd/mady/tui.go`**：替换 `PrintSystem("Mady 已就绪")` 为 `PrintWelcome(s.providerName, s.normalModel, modeLabel, projectLabel)`
   - `projectLabel` 来源于 `s.currentProject.Alias`（nil 时显示"无"）
   - 命令列表基于验证真实存在的斜杠命令：`/help`、`/case`、`/theme`、`/plan`、`/clear`、`/review`、`/thinking`、`/settings`

### 中观哲学融入
不出现"中观"文字说教，通过设计语言传达：
- **对称中心**：`◈ Mady ◈` 居中排版
- **菱形 ◈ 符号**：上下尖角隐喻"中"字上下竖线，横平竖直隐喻"口"字
- **冷色克制**：使用现有 Mady Dark 冷蓝品牌色
- **呼吸式间距**：各区块间 1 行空白
- **极简信息**：仅展示"够用"的信息量，无版本号、commit hash、Logo ASCII art

### 验证
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test -race ./tui/...` ✅（9 子包全绿）
- `make lint` ✅（gofmt + vet 0 issues）


---

## 2026-07-21: 新增护航评估 GuardrailsMetric + AdoptionRateMetric

### 背景
评估框架（agentcore/evaluate/）已有 14 种 Metric、CI 门禁和定时 Live 评估，
但缺少护航评估和 ApprovalRecord 接入。需要扩展评估体系覆盖护栏行为验证和
人工审批采纳率统计。

### 变更内容

**A1/A2: 创建 guardrails_eval.go — GuardrailsTestCase + GuardrailsMetric**
- 新增 `GuardrailsTestCase` 类型（ID / GuardrailLevel / Input / Context / ShouldFlag / MinFlagCount / ExpectedAction）
- 新增 `GuardrailsMetric` 实现 `Metric` 接口（Name="guardrails_accuracy"），
  Compute 比较实际护栏动作与期望动作是否一致
- 新增 `EvaluateGuardrailsBatch()` 批量评估函数，接受 mock guardrail 函数
- 新增 `NewGuardrailsMetric()` 工厂函数，默认阈值 0.8

**A3: 创建 guardrails_eval_test.go — 护栏评估测试**
- 使用 mock guardrail（关键词匹配，不依赖外部 LLM）验证：
  - Light 等级不拦截正常对话、拦截有害内容
  - Strict 等级拦截违规内容、放行正常内容
  - 误报率计算与阈值验证
  - GuardrailsMetric 在标准 Evaluator 中正常运行
  - 多等级边界行为（blocked/risk/approval  keywords 在不同等级下的正确响应）

**B1: 创建 adoption.go — AdoptionRateMetric + ApprovalRecord 接口**
- 定义 `ApprovalRecord` 接口（Decision() 返回 "adopted"/"modified"/"rejected"）
- 定义 `ApprovalRecordFunc` 适配器类型，适配已有结构体（如 domains.ApprovalRecord）
- 新增 `AdoptionRateMetric` 实现 `Metric` 接口（Name="adoption_rate"），
  Compute 返回 (Adopted + Modified) / Total，无数据时返回 1.0
- 新增 `Record(decision)` 方法累计计数
- 新增 `FromApprovalRecords(records)` 批量导入工厂函数
- 导出 FullyAdopted / Accepted / RejectedRate / Total 辅助方法

**B2: 创建 adoption_test.go — 采纳率指标测试**
- 覆盖全采纳 / 部分采纳 / 全部拒绝 / 无数据四种场景
- Record 方法累计计数与未知决策静默忽略
- FromApprovalRecords 批量导入与空切片
- AdoptionRateMetric 在 Evaluator.Evaluate 和 EvaluateBatch 中运行
- ApprovalRecordFunc 适配器验证
- 累加模式（复用同一 metric 实例多批次记录）
- prediction/reference 参数被忽略的语义验证

### 设计决策
1. **evaluate 包不自建 guardrails/domains 依赖**：使用接口 + mock 替代真实依赖，
   符合分层隔离原则（agentcore/evaluate 不得反向引用扩展层）
2. **GuardrailsMetric.Compute 比较字符串而非调用真实护栏**：Metric 接口契约要求
   确定性 (prediction, reference) → float64；护栏系统调用由 EvaluateGuardrailsBatch
   在外部编排
3. **AdoptionRateMetric 无数据时返回 1.0**：尚无采纳记录时视为满分——表示尚未出现
   负面反馈，与 GuardrailFalseNegativeRate 的零数据返回 0 不同，因为采纳率缺数据
   不代表问题
4. **ApprovalRecord 定义为接口而非结构体**：避免 evaluate 包与 domains 包的循环依赖；
   通过 ApprovalRecordFunc 适配器桥接

### 影响范围
- `agentcore/evaluate/guardrails_eval.go`（新增，43 行）
- `agentcore/evaluate/guardrails_eval_test.go`（新增，350 行）
- `agentcore/evaluate/adoption.go`（新增，160 行）
- `agentcore/evaluate/adoption_test.go`（新增，290 行）
- 不修改任何既有文件

### 验证
- `go build ./agentcore/evaluate/...` ✅
- `go vet ./agentcore/evaluate/...` ✅
- `go test -race ./agentcore/evaluate/...` ✅（全部测试通过，含新老测试 60+）
- `go build ./...` ✅（全项目无回归）
- `ExampleAdoptionRateMetric` 示例测试通过 ✅

---

## 2026-07-21: Batch 6 — 联邦式 Agent 网络 + i18n 多语言框架

### 背景
架构评审最后一批 P3 长期愿景项。A2A 协议实现完整（Google 兼容，25 源文件），
但缺少 Agent 注册目录和健康检查心跳池，无法构成联邦网络。i18n 方面 ~2300 行
中文硬编码，无任何国际化基础设施。

### 变更内容

**P3-2: 联邦式 Agent 网络（6 源文件，~550 行）**
- `a2a/registry/registry.go` — 内存 Agent 注册表（Registration 结构体 +
  Registry 类型，含 bySkill 二级索引），线程安全（sync.RWMutex）
- `a2a/registry/registry_test.go` — 11 个测试（Register/Deregister/Get/List/
  ListByCapability/ListBySkill/Count + 并发安全 + 深拷贝隔离）
- `a2a/registry/doc.go` — 包文档与使用示例
- `a2a/pool/pool.go` — 健康检查心跳池（Pool 类型 + DefaultCheckFunc），
  可配置 interval/timeout/ttl，连续失败自动摘除
- `a2a/pool/pool_test.go` — 10 个测试（Join/Leave/Alive + 存活/死亡检测 +
  自动摘除 + Start/Stop 生命周期 + 并发安全）
- `a2a/pool/doc.go` — 包文档与使用示例
- 零新外部依赖（仅标准库 sync/net/http/context/time）

**P3-3: i18n 多语言框架（8 源文件 + 4 翻译文件，~380 行）**
- `pkg/i18n/locale.go` — Locale 类型（zh-CN/en-US）+ ParseLocale
- `pkg/i18n/catalog.go` — 线程安全 Catalog：New/Add/T/LoadYAML/LoadDir +
  全局快捷方式（Global/SetGlobal/T），Fallback 链：key 不存在→返回 key、
  当前语言缺翻译→回退 zh-CN
- `pkg/i18n/catalog_test.go` — 14 个测试（基本翻译/格式化/fallback/
  LoadYAML/并发安全）
- `pkg/i18n/doc.go` — 包文档与使用示例
- `pkg/i18n/translations/zh-CN/guardrails.yaml` — 9 条护栏中文翻译
- `pkg/i18n/translations/en-US/guardrails.yaml` — 9 条护栏英文翻译
- `pkg/i18n/translations/zh-CN/common.yaml` — 8 条通用中文翻译
- `pkg/i18n/translations/en-US/common.yaml` — 8 条通用英文翻译
- `guardrails/disclaimer.go` — 新增 3 个 i18n 感知函数（Disclaimer/ShortDisclaimer/
  LevelTag），旧常量保留并标记 Deprecated

### 设计决策
1. **bySkill 二级索引**：registry 通过 `map[string]map[string]*Registration`
   实现 ListBySkill O(1) 查询，覆盖旧注册时自动清理索引
2. **Pool 可配置性**：interval/timeout/ttl 链式设置，DefaultCheckFunc 3s 超时
   适配合伙人网络环境
3. **i18n Fallback 链**：key 不存在→返回 key 本身（开发期可见）→当前语言缺
   翻译→回退 zh-CN，逐步迁移不影响已有代码
4. **零外部 i18n 依赖**：使用 gopkg.in/yaml.v3（已有依赖）+ 标准库实现，
   不引入 golang.org/x/text
5. **增量落地策略**：首批落地 guardrails 文案（面向用户、最关键），后续可扩展
   到 System Prompt、规则引擎等

### 验证
- `make verify`（lint + 架构边界 + build + race test）全量通过 ✅
- lint: 0 issues ✅，架构边界: 14/14 通过 ✅
- a2a/registry 11 个测试全绿 ✅
- a2a/pool 10 个测试全绿 ✅
- pkg/i18n 14 个测试全绿 ✅
- guardrails 测试全绿 ✅（含 guardian 子包）
- 全项目 80+ 包竞态测试通过 ✅

---

## 2026-07-21: P1-1 物理移动 + P0-1 Phase 3 iface 消费侧迁移

### 背景
架构评审遗留的两个待决项集中处理。doomloop/evaluate/tracing 三个子包在 Batch 2
已完成逻辑解耦，但物理上仍嵌在 agentcore/ 下。iface 接口层在 Batch 2/3 创建，
但 guardrails/server 等消费者仍直接依赖 agentcore 具体类型。

### 变更内容

**P1-1: doomloop/evaluate/tracing 物理移动到顶层目录（~36 文件变动）**
- 创建 `doomloop/`、`evaluate/`（含 benchmark/cli 子包）、`tracing/` 顶层目录
- 从 `agentcore/doomloop/`、`agentcore/evaluate/`、`agentcore/tracing/`复制全部文件
- 更新 import 路径：`agentcore/doomloop` → `doomloop`、`agentcore/evaluate` → `evaluate`、
  `agentcore/tracing` → `tracing`（涉及 13 个内部文件 + 7 个外部引用方）
- 更新 Makefile eval 目标和 2 个 CI workflow 中的路径
- 删除 3 个旧目录
- 三个包仍依赖 agentcore（叶节点包），agentcore 不反向依赖它们

**P0-1 Phase 3: iface 接口扩展 + guardrails/server 消费侧迁移（19 文件变动）**
- `agentcore/iface/message.go` — 新增 Message 结构体 + 4 个角色常量（system/user/assistant/tool）
- `agentcore/iface/context.go` — 新增 AgentContext 接口（Input()/Messages()）
- `agentcore/iface/types.go` — Event 接口新增 `Payload() any` 方法
- `agentcore/iface/event.go` — 新增 SimpleEvent + NewEvent() 构造函数
- `agentcore/iface/lifecycle.go` — AgentRunContext/ModelCallContext 新增 `Raw any` 字段
- `agentcore/iface_adapter.go` — Emit() 支持 payload 传递；新增 ifaceLifecycleHookAdapter
- `guardrails/levels.go` — 嵌入 iface.BaseLifecycleHook，返回 iface.LifecycleHook，
  参数类型改为 iface 类型，内部通过 `Raw` 字段类型断言访问 agentcore 字段
- `guardrails/citation_gate.go` — 同上迁移
- `server/server.go` — eventBus 字段类型改为 iface.EventBus
- `server/disclosure.go`、`disclosure_events.go`、`skills.go` — 事件系统迁移到 iface
- `domains/*.go`（5 文件）— guardrails.New() 包装为 agentcore.NewIFaceLifecycleHook()
- 测试文件更新：guardrails_test.go、citation_gate_test.go、server_test.go 等

### 设计决策
1. **类型断言适配器模式而非全量重写**：消费侧只依赖 iface 接口，通过 `Raw` 字段 +
   类型断言访问 agentcore 具体类型。避免全量重写，保留对 agentcore 深度字段的访问。
2. **payloadEvent 传递链**：iface.SimpleEvent → eventBusAdapter.Emit() → payloadEvent →
   agentcore.EventBus → ifaceWrappedEvent → 消费者通过 Payload() 获取原始事件体。
   实现 iface 事件到 agentcore 事件的无损传递。
3. **渐进式迁移**：先迁移 guardrails（LifecycleHook 消费者）和 server（EventBus 消费者），
   memory/knowledge/domains 等其他消费者的迁移留待后续。

### 验证
- `make verify`（lint + 架构边界 + build + race test）全量通过 ✅
- lint: 0 issues ✅，架构边界: 14/14 通过 ✅
- doomloop/evaluate/tracing 三个顶层包构建和竞态测试通过 ✅
- guardrails 测试全绿 ✅（含 guardian 子包）
- server 测试 3.25s 通过 ✅
- agentcore/iface 测试全绿 ✅
- 全项目 80+ 包竞态测试通过 ✅
- tools 子模块独立测试通过 ✅

### 背景
架构评审 P2-3（声明式领域配置）和 P3-1（评估体系持续化）集中推进。当前领域 Agent
通过 Go 工厂函数硬编码配置，新领域注册需修改 Go 代码。评估框架已有 CI 门禁和
定时 Live 评估，缺少护栏行为验证和采纳率统计。

### 变更内容

**P2-3: 声明式领域配置（7 源文件 + 6 测试数据）**
- `domains/domainconfig/config.go` — DomainConfig 结构体（YAML/JSON 双标签），
  支持：元数据(name/domain/description)、安全配置(guardrail_level/knowledge_domain)、
  模型参数(model/temperature/max_tokens)、执行配置(engine/max_turns)、
  工具/技能列表、系统提示词(内联/文件路径)、附加配置(extra)
- `domains/domainconfig/load.go` — LoadConfig（单文件 YAML/JSON 自动识别）、
  LoadConfigs（目录批量扫描）、DefaultConfigDir（`$MADY_HOME/domains/`）
- `domains/domainconfig/doc.go` — 包文档与使用示例
- `domains/domainconfig/testdata/patent.yaml`、`legal.yaml`、`chat.yaml`、
  `assistant.yaml`、`patent.json` — 各领域配置示例
- `domains/domainconfig/testdata/invalid/empty_name.yaml`、`empty_domain.yaml` —
  校验失败测试数据
- `domains/domainconfig/config_test.go` — 11 个测试（YAML 加载/JSON 加载/
  文件不存在/批量加载/Validate 边界/序列化一致性）

**P3-1: 评估体系持续化（4 源文件）**
- `agentcore/evaluate/guardrails_eval.go` — GuardrailsTestCase + GuardrailsMetric
  （Name="guardrails_accuracy"）+ EvaluateGuardrailsBatch 批量评估
- `agentcore/evaluate/guardrails_eval_test.go` — 5 组测试：Light 放行正常/拦截违规、
  Strict 拦截违规/放行正常、误报率阈值、多等级边界、Evaluator 集成
- `agentcore/evaluate/adoption.go` — ApprovalRecord 接口 + ApprovalRecordFunc 适配器
  + AdoptionRateMetric（Name="adoption_rate"）+ Record/FromApprovalRecords/辅助方法
- `agentcore/evaluate/adoption_test.go` — 4 种场景测试（全采纳/部分采纳/全拒绝/无数据）
  + Record 累计 + 批量导入 + Evaluator 集成 + 适配器 + Example

### 设计决策
1. **DomainConfig 与 Manifest 分层**：Manifest 保持轻量元数据（go:embed 编译），
   DomainConfig 提供完整运行时配置。新领域可放 YAML 文件注册，无需 Go 代码
2. **零侵入向后兼容**：不修改现有 Go 工厂函数（PatentAgentConfig 等仍照常工作），
   DomainConfig 通过额外目录加载，按需合并
3. **evaluate 不自建 guardrails/domains 依赖**：使用接口 + mock + 适配器模式
   避免循环依赖，符合分层隔离原则
4. **AdoptionRateMetric 无数据返回 1.0**：尚无采纳记录视为满分，与
   GuardrailsMetric 的零数据处理策略不同，因为缺采纳数据不代表问题

### 验证
- `make verify`（lint + 架构边界 + build + race test）全量通过 ✅
- lint: 0 issues ✅，架构边界: 14/14 通过 ✅
- `domains/domainconfig` 11 个测试全绿 ✅
- `agentcore/evaluate` 60+ 测试全绿 ✅（含新老测试）
- 全项目 80+ 包竞态测试通过 ✅
- tools 子模块独立测试通过 ✅

## 2026-07-22: 修复 CI 检查失败 — mod tidy + go-arch-lint 格式升级

### 背景
CI 检查（GitHub Actions）持续失败，涉及两个 job：
1. **mod-tidy**：`tui/` 子模块缺少对根模块 `github.com/xujian519/mady` 的 require 指令
2. **check-arch**：`.go-arch-lint.yml` 使用已废弃的 `packages` 格式，不兼容 go-arch-lint v1.13+

### 变更内容

**tui/go.mod**
- 执行 `go mod tidy`，补充缺失的根模块依赖及间接依赖
- 新增 `tui/go.sum`

**.go-arch-lint.yml** — 从旧格式迁移到新格式
- `packages`（`name`/`path`/`rules`/`forbidden`）→ `components`（`**` glob）+ `deps`（`mayDependOn`/`anyProjectDeps`）
- 新增 40+ 组件定义覆盖所有 Go 包目录
- 维护 10 个受限组件的架构规则，转换为正向白名单
- `graph` 组件添加 `agentcore` 豁免（测试文件类型构造需要）
- 未受限组件使用 `anyProjectDeps: true`

### 验证
- `make verify` 全量通过（lint + build + race test）✅
- `go-arch-lint check` 通过（v1.16.0 + v1.12.0 双版本验证）✅
- `go mod tidy -diff` 通过 ✅

### 风险说明
- 黑名单→白名单转换可能遗漏预期外的依赖路径，但 CI 此前从未正确执行 check-arch

---

## 2026-07-23: workflows/autoresearch 全量审阅修复

### 背景
对 `workflows/autoresearch` 包进行 8 维度全量代码审阅，发现 16 项需修复问题
（P0×4、P1×4、P2×4、P3×4），涵盖并发安全、状态机守卫、测试覆盖、文档一致性、
集成缺口。详见审阅报告。

### 修复清单

**P0 — 严重问题：**

| # | 问题 | 改动 |
|---|------|------|
| 1 | 全包无锁导致并发不安全 | `ResearchContract` 和 `Heartbeat` 新增 embedded `sync.Mutex` + `json:"-"`，所有公共方法加 `Lock()/defer Unlock()` |
| 2 | 状态机无非法转换保护 | `Start/Pause/Resume/Complete/Abort` 入口增加状态守卫；`Abort` 支持 `running/paused`；`Complete` 支持 `running/paused`；`Pause` 只允许 `running`；`Resume` 只允许 `paused`；`Start` 只允许 `idle` |
| 3 | `Abort` 向 `SuccessCriteria` 追加条目阻塞 `AllCriteriaMet` | 移除 `Abort` 的副作用，新增 `AbortReason string` 独立字段；`AllCriteriaMet` 在 `StatusAborted` 时直接返回 `false` |
| 4 | `IsExpired` 在未启动时误判（`time.Since(zeroTime) ≈ 292y`） | 入口检查 `c.StartedAt.IsZero()` 直接返回 `false` |

**P1 — 代码质量：**

| # | 问题 | 改动 |
|---|------|------|
| 1 | 默认值 `MaxDuration=0` 与注释"最多30分钟"矛盾 | `NewResearchContract` 中 `MaxDuration` 改为 `30 * time.Minute` |
| 2 | `Domain` 无校验 | 声明 `validDomains` 集合，构造时检查，非法值归一化为 `"general"` |
| 3 | `PausedAt/CompletedAt` 空指针恐慌风险 | 新增 `PausedAtTime() (time.Time, bool)` / `CompletedAtTime() (time.Time, bool)` 安全访问器 |
| 4 | `Evidence` 无限追加 | `AddEvidence` 追加后检查上限（`MaxRounds` 或硬上限 100），超出时淘汰最旧条目 |
| 5 | 测试覆盖率缺口（Abort/AddEvidence/时长过期 0%） | 新增 11 个测试用例覆盖所有缺口 |

**P2 — 集成与文档：**

| # | 问题 | 改动 |
|---|------|------|
| 1 | `workflows/doc.go` 未列出 autoresearch 子包 | 在 Sub-packages 章节追加 `workflows/autoresearch` 条目及说明 |
| 2 | `SinceLastBeat` 文档未说明不判断过期 | 补充注释：过期判定请使用 `Check()` 后读取 `IsStale` |
| 3 | 脆弱测试 `time.Sleep` | `TestHeartbeat` 中移除 `time.Sleep(time.Millisecond)`（`Timeout=1ns` 已足够触发过期） |
| 4 | `AI_CHANGELOG.md` 缺少创建决策 | 本条目即为该包的完整创建决策和审阅修复记录 |

### 涉及文件
- `workflows/autoresearch/contract.go` — 重写：新增 mutex、状态守卫、AbortReason、默认值修正、Evidence 上限、空指针访问器、Domain 校验
- `workflows/autoresearch/heartbeat.go` — 新增 mutex + SinceLastBeat 注释完善
- `workflows/autoresearch/autoresearch_test.go` — 新增 11 个测试用例：TestAbort、TestAbortBlocksAllCriteriaMet、TestIsExpiredBeforeStart、TestTimeBasedExpiry、TestAddEvidence、TestEvidenceCap、TestIllegalStateTransitions、TestPausedAtCompletedAtAccessors、TestHeartbeatFullPath、TestNewContractDefaults、TestDomainValidation
- `workflows/doc.go` — 追加 autoresearch 子包条目

### 验证
- `go build ./workflows/autoresearch/...` ✅
- `go vet ./workflows/autoresearch/...` ✅
- `go test -v -race -count=1 ./workflows/autoresearch/...` ✅（16 测试全绿）
- `go test -race -count=5 ./workflows/autoresearch/...` ✅（5 次无 flaky）
- 覆盖率：0% → **~93%**（Abort/AddEvidence/IsExpired/SinceLastBeat 从 0% 全覆盖）

---

## 2026-07-23: workflows/autoresearch 第二轮修复（P2/P3 剩余项）

### 背景
第一轮修复完成 P0/P1 全部 8 个问题后，继续处理审阅报告中的 P2/P3 剩余项：
Heartbeat-Contract 编译器级耦合、持久化接口层、架构定位文档、可观测性埋点。

### 修复清单

| # | 问题 | 改动 |
|---|------|------|
| P2-3 | Heartbeat-Contract 松耦合（字符串 ContractID 无编译器保证） | 新增 `CreateHeartbeat(interval, timeout)` 方法，ContractID 自动从 `c.ID` 派生；新增 `ContractID()` 访问器 |
| P2-1 | 持久化层缺失（纯内存，进程重启数据丢失） | 新增 `persist.go`：`ResearchStore` 接口（SaveContract/LoadContract/SaveHeartbeat/ListActive/DeleteContract）+ 深拷贝隔离的 `InMemoryResearchStore` 实现 |
| P2-2 | 包归属说明不清晰 | `doc.go` 新增架构定位章节，明确 `autoresearch` 是"多轮次元级状态管理器"，与其他 workflow 是层次关系 |
| P3-3 | 全包零可观测性 | 在 `Start/Pause/Resume/Complete/Abort/AdvanceRound/AddEvidence/Beat/Check` 所有关键生命周期点添加 `slog.Info/Warn/Debug` 日志，关键事件含结构化字段（contract_id/round/duration/reason） |

### 涉及文件
- `workflows/autoresearch/contract.go` — 新增 `CreateHeartbeat`、`ContractID`、`truncateString`；全部生命周期方法加 slog 日志
- `workflows/autoresearch/heartbeat.go` — `Beat`/`Check` 加 slog 日志（stale 时 WARN 含 timeout 和距上次心跳时长）
- `workflows/autoresearch/persist.go` **（新增）** — ResearchStore 接口 + InMemoryResearchStore（线程安全 + 深拷贝隔离）
- `workflows/autoresearch/persist_test.go` **（新增）** — 8 个测试：SaveAndLoad、NotFound、ListActive、ListActive_Empty、SaveHeartbeat、DeleteContract、DeepCopyEvidence、ConcurrentSafety
- `workflows/autoresearch/autoresearch_test.go` **（扩展）** — 新增 5 个测试：TestContractIDAccessor、TestCreateHeartbeat、TestTruncateString、TestEvidenceTrimmedLogging、TestDeepCopyAllFields
- `workflows/autoresearch/doc.go` — 架构定位说明

### 验证
- `go build ./workflows/autoresearch/...` ✅
- `go vet ./workflows/autoresearch/...` ✅
- `go test -race -count=1 ./workflows/autoresearch/...` ✅（**28 测试全绿**）
- `go test -race -count=5 ./workflows/autoresearch/...` ✅（5 次无 flaky）
- `go build ./...` ✅（全项目无回归）
- 覆盖率：93% → **100.0%**（全部 31 个函数 100% 覆盖）

## 2026-07-24: IPC 审查标准卡片知识集成（P2-2）

### 背景
将 Obsidian 知识库中约 138 张审查标准卡片的关键内容提取为结构化数据，纳入 Mady 知识库。

### 修改清单
- `knowledge/standards/doc.go` — 包文档
- `knowledge/standards/ipc-standards.go` — IPC 审查标准主模块（含 LoadStandards/FindByIPCSection/FindByArticle/FindByIPCDetail/Search/FormatAsContext）
- `knowledge/standards/ipc-standards_test.go` — 9 个测试用例
- `knowledge/standards/ipc-standards.yaml` — 138 条审查标准结构化数据（嵌入二进制）
- `domains/rules/data/ipc-standards/ipc-standards.yaml` — 数据文件副本
- `domains/reasoning/types.go` — 新增 RuleSourceIPC 常量
- `domains/reasoning/rule_retrieval.go` — 新增 IPCStandardSource 接口、queryIPC 方法
- `domains/reasoning/ipc_source.go` — IPCStandardAdapter 实现

### 数据规模
- 138 条标准，覆盖 IPC 大类 A/B/C/D/E/F/G/H
- 法律条款覆盖：A22.2(35), A22.3(64), A23(3), A26.3(25), A26.4(9), A33(2)
- 117/138 (85%) 含审查要点，43/138 (31%) 含实务提示
- 全部 9 项测试通过

## 2026-07-24: 斜杠命令系统交互改进（P0三阶段 + code-review修复）

### 背景
Mady TUI 斜杠命令系统与主流产品（Claude Code、VS Code Command Palette）相比存在交互差距：命令中心不显示当前状态、无内联参数补全、未知命令只有纯文本提示。

### 修改清单
- `cmd/mady/settings_panel.go` — buildCommandItems 填充 Status 字段；openCommandCenter 支持变参预填搜索；新增 resolveCommandStatus
- `cmd/mady/slash_registry.go` — 新增 ArgSuggestion 类型、SlashArgProvider + filterSuggestions；SlashCommand 新增 Args 字段；为 theme/plan/review/thinking/settings 填充 Args
- `cmd/mady/tui.go` — 注册 SlashArgProvider 到 Providers
- `cmd/mady/tui_session.go` — handleSubmit 未知命令时打开命令中心预填搜索
- `tui/component/autocomplete.go` — Refresh 支持 FullInputProvider 接口
- `tui/component/command_center.go` — 新增 SetFilter 方法
- `tui/core/component.go` — 新增 FullInputProvider 接口

### Code Review
5 维度（line-by-line/cross-file/removed-behavior/Go-pitfalls/altitude-conventions）覆盖评审，修复 4 项 bug：尾随空格阻断补全、token 忽略无前缀过滤、多空格 cmdName 含尾随空格、重复代码提取 filterSuggestions
