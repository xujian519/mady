# AI 决策变更日志

## 2026-07-16: TUI 模块优化 Phase B/C/D — 批次 7-12 全部落地

### Batch 7（P1-3）ToolCard 共享渲染组件
- 新增 `tui/component/tool_card.go`：`RenderToolCard(cfg, theme, width)` 把 RoleTool 的 bar+title+status+collapsed-summary 渲染抽成可复用组件，diff 正文走 `NewMarkdown` 复用现有 ```diff 围栏着色
- `tui/chat/chat_history_render.go` RoleTool 分支改走 `RenderToolCard`（事件流不变，仍两条消息），`ToolCardTheme` 从 ChatHistoryTheme 桥接样式保持视觉等价
- 测试：`tool_card_test.go`（bar 颜色启发、collapsed 摘要截断、diff 正文追加）

### Batch 8（P1-4）stick-to-bottom 提示 + StatusBar tok/s 指标
- `tui/component/statusbar.go`：加 `SetUsage(prompt, completion, tokPerSec)` + `SetContext(used, total)`；Render 在 elapsed 后显示 `⚡ 1.2k tok/s`，右侧加 10 格上下文占用条（绿/橙/红 按负载）
- `tui/chat/chat_history.go/.go`：加 `tailAnchorLen` 字段；用户上滑时冻结锚点，新内容到达显示 `↓ N new — End to follow`，FollowTail 回底部清除
- `tui/chat/chat_app_tool.go` `onTurnEnd`：累加 `Usage`、按 turn 耗时算 tok/s 转发 StatusBar（修复原注释谎称"StatusBar 自己订阅"的断层）；`onAgentStart` 记 turn 起始时间
- 测试：`TestChatHistoryStickToBottomHint`

### Batch 9（P1-5）Slash 命令注册表（消除双源真相）
- 新增 `cmd/mady/slash_registry.go`：`Registry` + `SlashCommand{Name/Aliases/Desc/Match/Available/Handler}`，Lookup 短路匹配（前缀命令优先），`Suggestions()` 统一生成补全
- `handleSubmit` 从两段式 switch 改为 `slashReg.Lookup`；`buildSlashRegistry()` 注册全部 18 个命令（含 /mode 多域 gate、/approve /reject 审核 gate）
- 删除冗余 `slash_suggestions.go`（被注册表的 Suggestions 取代，消除与 handleSubmit 的双源漂移）
- 测试：`slash_registry_test.go`（精确/前缀/别名匹配、Available gate、Suggestions 可见性）

### Batch 10（P2-7）接入 settings 组件（孤岛资产启用样板）
- `tui/chat/chat_app.go`：加 public `OpenOverlay(content, OverlayOpts)` + `CloseOverlay(OverlayRef)`（复用 overlayHandle，锁外调 host 避免死锁）
- 新增 `cmd/mady/settings_panel.go`：`openSettings()` 构造 SettingEntry（theme/plan/review/thinking），Box 包裹 + OpenOverlay 推送，OnChange 实时生效、OnSubmit 关闭
- 注册 `/settings` 命令到 slash_registry

### Batch 11（P2-6）显式状态机 + 整帧快照测试
- 新增 `tui/chat/state.go`：`AppState`（idle/streaming/tool-running/awaiting-confirm/compacting）+ 纯函数 `Transition(state, event)` + `EventKindFor(ChatEvent)`，渐进式 FSM（当前作 spec+测试靶，未来可让 handler 委托）
- 测试：`state_test.go`（22 条表驱动转移 + EventKind 映射 + String）、`chat_app_frame_test.go`（整帧结构断言：header/history/loader/editor border/statusBar 全在场，行数 ~24）

### Batch 12（P2-8）键位配置文件化 keymap.json
- `tui/terminal/keybindings.go`：加 `LoadUserBindingsJSON([]byte) (warnings, err)`，解析 `{"tui.editor.x": ["ctrl+a"]}`，校验 token 形状（空 name/未知修饰键告警但保留），空 payload 清除覆盖
- `cmd/mady/tui_helpers.go`：`loadKeymapOverrides(madyHome, km)` 从 `~/.mady/keymap.json` 加载；`main.go` runTui 在 app 构造后应用到 `app.Keybindings()`
- 测试：`keybindings_json_test.go`（有效应用/未知修饰键告警/空 token 跳过/畸形 JSON 报错/空清除）

- **影响范围**: `tui/component/{statusbar,tool_card}.go`、`tui/chat/{chat_app,chat_history,chat_history_render,chat_app_tool,chat_app_stream,state}.go`、`tui/terminal/keybindings.go`、`cmd/mady/{slash_registry,settings_panel,tui_helpers,main,tui_session}.go`（+若干 _test.go）
- **风险等级**: 中（P1-3/P1-4 改渲染与事件转发，P1-5 改命令分发，但均有测试覆盖；P2 批次为新增/渐进式，低风险）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tui/... ./cmd/...` ✅（全 11 子包）| tools 子模块 `go build` ✅ | gofmt ✅

## 2026-07-16: TUI 模块优化 Phase B / P0-1 — 流式 Markdown 增量解析（11× 提速）

- **问题**：流式 Pending 消息每个 token delta 都触发整段 `renderMarkdown(source, width)` 全量重解析（O(N)），长回复累积成 O(N²)。`chat_history.go:349` 的"连续相同 delta 去重"只挡重复 token，没挡增长型 token。
- **方案**（ChatHistory 层 block 缓存，Markdown 保持纯函数）：
  1. `tui/component/markdown.go`：从单遍 `renderMarkdown` 拆出 `parseBlocks(src) []Block`（切片器，保持完全相同的块边界判定）+ `renderBlock(b, width, theme) []string`（单块渲染）；`renderMarkdown` 变为两者组合（行为等价）
  2. 新增 `BlockCache` + `RenderMarkdownIncremental(src, width, theme, cache)`：按 (blockRaw, blockKind, closed, width) 缓存每块的渲染行，只重渲染变更块
  3. `tui/chat/chat_history.go`：`cachedMessage` 加 `blockCache` 字段；`renderMessage` 加 `mdCache` 参数
  4. `tui/chat/chat_history_render.go`：Pending 助手消息走 `RenderMarkdownIncremental`（复用 block 缓存），非 Pending 走原 `NewMarkdown` 全量路径
- **安全网**：先加 `TestRenderMarkdownEquivalenceGolden`（捕获重构前全块类型输出为 golden）+ `TestBlockCacheMatchesFreshRender`（增量输出 == 全量输出），再做等价重构；`TestChatHistoryStreamingDeltaReusesBlockCache` 验证缓存复用
- **实测性能**（`BenchmarkChatHistoryStreamAppend`，200 delta 流式渲染）：优化前 3,261,499 ns/op / 49,549 allocs → 优化后 292,925 ns/op / 3,005 allocs —— **11.1× 提速、12.0× 省内存、16.5× 少分配**
- **影响范围**: `tui/component/markdown.go`、`tui/component/markdown_equiv_test.go`(新)、`tui/chat/chat_history.go`、`tui/chat/chat_history_render.go`、`tui/chat/chat_history_test.go`
- **风险等级**: 中（渲染路径重构，但 golden 等价测试 + 增量一致性测试全覆盖；非 Pending 路径完全不变）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tui/...` ✅（全 9 子包）| gofmt ✅ | benchmark 11× 提升 ✅

## 2026-07-16: TUI 模块优化 Phase A — 五个超大文件机械拆分（Batch 1-5）

- **变更**（纯机械拆分，不改逻辑，同包分文件零 export 摩擦）：
  1. `tui/tui.go` (1051行) → 6 文件：`tui.go`(类型+构造+Children+accessor, 269) / `tui_lifecycle.go`(Start/Stop/Tick/Every, 209) / `tui_loop.go`(eventLoop, 45) / `tui_input.go`(processMsg/Cmd/mouse, 279) / `tui_render.go`(RequestRender/renderFrame/normalizeLine, 171) / `tui_focus.go`(focus+overlay 栈, 131)
  2. `tui/chat/chat_history.go` (1321行) → 3 文件：`chat_history.go`(类型+Append/Patch/Delta+msgCache, 458) / `chat_history_render.go`(Render/renderAll/renderMessage+selection, 526) / `chat_history_input.go`(Update/handleMouse/scroll/click-toggle, 371)
  3. `tui/chat/chat_app.go` (1169行) → 4 文件：`chat_app.go`(类型+构造+Print*/Busy/Idle+overlayHandle, 463) / `chat_app_stream.go`(onEditorSubmit/onAgent*/onMessageDelta, 95) / `chat_app_tool.go`(onTool*/onHandoff*/onTurn*/extractToolDiff, 301) / `chat_app_layout.go`(chatLayout+Update+doCopy, 363)
  4. `tui/component/editor.go` (1343行) → 5 文件：`editor.go`(类型+构造+SetValue/Select+Focusable+Update, 346) / `editor_render.go`(Render+handleMouse+hitTest, 324) / `editor_edit.go`(processKeys+editing 原语, 415) / `editor_killring.go`(kill-ring+yank, 126) / `editor_history.go`(undo/redo+input history, 182)
  5. `cmd/mady/main.go` (1057行) → 5 文件：`main.go`(入口+setupFrameworkContext+知识库加载, 680) / `server.go`(runServer, 90) / `acp.go`(runAcp, 40) / `slash_suggestions.go`(buildSlashSuggestions, 46) / `tui_helpers.go`(thinking/project/format 辅助, 265)
- **保留的不变量**：(1) `onMessageDelta` 的 StreamID 临界区不拆；(2) `ToggleKeyHelp` 锁内捕获 ref/锁外调 host 的反死锁模式；(3) ChatHistory 的 msgCache/invalidateMessageLocked 增量缓存框架；(4) 同包白盒测试（renderFrame/processMsg/sendMsgSafe/onMessageDelta）零影响
- **修复**：拆分 chat_history_render.go 时一处 `applySelectionHighlightLocked` 末尾参数误抄为 `width`（应为 `lineWidth`），被 `TestSelectionHighlightKeepsVisibleWidthStable` 等 3 个测试捕获并修正
- **影响范围**: `tui/`、`tui/chat/`、`tui/component/`、`cmd/mady/`（仅文件重组，无语义变更）
- **风险等级**: 低（机械移动，测试全覆盖验证语义等价）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tui/... ./cmd/...` ✅（全 9 子包通过）| gofmt ✅

## 2026-07-16: 全量质量审阅 v0.3.0 — 9 维度全覆盖（29 检查点通过，2 修复）

- **变更**:
  1. `docs/decisions/REVIEW_REPORT_2026-07-16.md`：新增全量审阅报告
  2. `tools/vision.go` + `tools/tools.go`：VisionToolConfig 补全 Sandbox 字段传播 + resolvePath → resolvePathSandboxed（沙箱绕过修复）
  3. `domains/approval.go`：NewApprovalGate 签名改为 variadic opts 以适配已有调用
- **审阅范围**: 阶段 0（基线通过）→ 阶段 1（六大自动化扫描）→ 阶段 2（16 CRITICAL 历史回归全部修复）→ 阶段 3（安全红线/并发/v0.3.0 新模块/架构/措辞/测试）
- **结果**: 29 检查点通过，2 个安全问题发现并修复。详细报告见 `docs/decisions/REVIEW_REPORT_2026-07-16.md`
- **影响范围**: `tools/vision.go`, `tools/tools.go`, `domains/approval.go`（修改后验证通过）
- **风险等级**: 低（修复沙箱安全边界 + 接口兼容性）
- **审查要求**: L3（安全敏感路径 `tools/vision.go` 含沙箱）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tools/... ./domains/...` ✅ | `golangci-lint run` ✅

## 2026-07-16: 接通 RecordDecision——HITL 数据采集链路

- **变更**:
  1. `domains/approval.go`：`ApprovalGate` 加 `lastTriggeredOutput` 字段，`AfterModelCall` 触发审批时保存 Agent 产出；`RecordDecision` 自动使用该字段作为 `originalOutput`（调用方可传空），记录后清空。
  2. `cmd/mady/tui_session.go`：`tuiSession` 加 `approvalGate` 引用；`buildAgentConfig` 在 reviewMode 时创建带 `SQLiteApprovalStore`（`workspace/approvals.db`）的 gate 并保存引用；新增 `openApprovalStore`（SQLite，WAL 模式）和 `recordApprovalDecision`（调用 `gate.RecordDecision`）；`/approve` 记录 `DecisionAdopted`，`/reject` 记录 `DecisionRejected`。
- **原因**: 之前 `RecordDecision`/`ApprovalStore`/`ApprovalRecordState` 是已设计但未接线的死代码。P2B 五层评估证明用 LLM 模拟 HITL 无法准确测量真实人机协作价值（L5=0.320 < L1=0.513，因为 LLM 修订破坏正确初稿）。需要接通生产环境的真实 HITL 数据采集，让用户每次 /approve /reject 都持久化到 SQLite，积累真实 AdoptionRate 数据，为 P3 专家盲测提供基础。
- **影响范围**: `domains/approval.go`（ApprovalGate 加字段+改 RecordDecision）、`cmd/mady/tui_session.go`（gate 创建带 store + approve/reject 留痕 + openApprovalStore + recordApprovalDecision）
- **风险等级**: 中（涉及 `domains/approval.go` 安全敏感路径，但仅新增字段和自动填充逻辑，不改 AfterModelCall 的审批触发行为）
- **审查要求**: L3（安全敏感路径 `domains/approval.go`）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./domains/...` ✅ | gofmt ✅

## 2026-07-16: P2B 五层评估完成——LLM 模拟 HITL 的方法论困境

- **变更**: 新增 `mockHumanRevision`（LLM 模拟专家修订）和 `TestLiveAgentP2BHitlEval`（L5 HITL tier），跑出 P2B 五层完整对比。
- **P2B 五层排序（llm_judge 均值）**：L1 通用 prompt（0.513）> L4 增强 prompt（0.410）> L0 裸 LLM（0.363）> L2 工具编排（0.334）≈ L5 模拟 HITL（0.320）
- **L5 关键发现**：mockHumanRevision 对高分初稿有害（−0.73/−0.80），对低分初稿有益（+0.53），净效果为负（0.320 < L1 0.513）。根因：LLM 无法像真实专家一样判断「初稿已够好不需改」，对所有初稿都做修改引入不确定性。
- **结论**：不能从 L5=0.320 得出"HITL 有害"——这是 LLM 模拟修订的局限。真实 HITL 的理论上限介于 L1（0.513）和完美修订之间。需真实专家盲测（P3）才能准确测量。
- **影响范围**: `agentcore/evaluate/benchmark/live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（新增测试和文档，不改现有逻辑）
- **审查要求**: L1
- **验证**: `go vet` ✅ | P2B L5 10 题 live eval ✅

## 2026-07-16: 性能优化 Phase 6 — P2 次要优化（nextMemoryID atomic + CosineSimilarity float32 + APIEmbedder 连接池）

- **变更**:
  1. `memory/store.go`：`nextMemoryID` 从 `idMu sync.Mutex` + `idCounter int64` 改为 `atomic.Int64.Add(1)`，消除全局锁
  2. `retrieval/embedding.go`：`CosineSimilarity` 和 `DotProduct` 从 float64 逐元素转换改为 float32 原生运算，仅在最终 sqrt 时转 float64，减少 1024 维向量约 3072 次类型转换
  3. `retrieval/embedding.go`：`NewAPIEmbedder` 从 `&http.Client{}` 改为配置 Transport（MaxIdleConns:100, MaxIdleConnsPerHost:20, IdleConnTimeout:90s, Timeout:30s），启用 HTTP keep-alive 连接复用
- **跳过 6d (AgentRun sync.Pool)**: Run 方法中对象（AgentRunContext/ProviderRequest/messages）生命周期复杂，跨 lifecycle 钩子和 goroutine，不适合 sync.Pool
- **影响范围**: `memory/store.go`、`retrieval/embedding.go`（2 个文件）
- **风险等级**: 低
- **审查要求**: L1
- **验证**: `go build` ✅ | `go test -race ./memory/... ./retrieval/...` ✅ | `golangci-lint` ✅

## 2026-07-16: 性能优化 Phase 5 — Memory 向量检索修复（P0-1/P0-2）

- **变更**:
  1. `memory/sqlite_store.go`：新增 `embedder retrieval.Embedder` 字段 + `WithSQLiteEmbedder()` Option。`Remember` 从硬编码 `NULL` 改为有 embedder 时自动生成 embedding 写入；`RememberBatch` 同步自动填充。`Recall` 从纯 `keywordScore` 改为：有 queryVec + entry embedding 时用 `retrieval.CosineSimilarity` 向量搜索，否则 fallback 到 keywordScore。
  2. `memory/store.go`：InMemoryStore 同步改动——新增 `embedder` 字段 + `WithEmbedder()` Option。`Remember`/`RememberBatch`/`Recall` 同 SQLite 版本逻辑一致。
  3. `memory/sqlite_store_test.go`：新增 `mockEmbedder`（字符 hash 向量）、4 个新测试覆盖 Remember/RememberBatch/Recall 向量路径 + nil embedder fallback。
- **根因**: `Remember` SQL 中 embedding 列硬编码 `NULL`；schema 有 `embedding BLOB` 列但从未使用。`Recall` 退化为 keywordScore 暴力匹配，O(N×content_len) 重复 tokenize + 词频 map。
- **向后兼容**: ✅ nil embedder → 现有 keywordScore 行为，零破坏
- **影响范围**: `memory/sqlite_store.go`、`memory/store.go`、`memory/sqlite_store_test.go`（3 个文件）
- **风险等级**: 低（纯代码层修复，Memory 系统尚未集成到生产入口，运行时无影响）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go test -race ./memory/...` ✅（19/19 通过） | `golangci-lint` ✅

## 2026-07-16: P2B 四层评估定论——通用 prompt + 自主推理最优

- **变更**: 新增 `ManifestToSystemPrompt`（manifest steps → 结构化 prompt）和 `TestLiveAgentP2BPromptAugmentedEval`（L4 增强 prompt 评估），跑出 P2B 四层完整对比。
- **P2B 四层排序（llm_judge 均值）**：L1 通用 prompt（0.513）> L4 增强 prompt（0.410）> L0 裸 LLM（0.363）> L2 工具编排（0.334）
- **两个假设验证**：
  - ✅ "prompt 引导 > 工具编排"：L4（0.410）> L2（0.334）
  - ❌ "增强 prompt > 通用 prompt"：L4（0.410）< L1（0.513）
- **深层结论**：对 LLM Agent，最简单的通用 prompt + 自主推理（L1）反而最好。过多的结构约束（工具编排 L2 或增强 prompt L4）都是干扰。manifest/metrics/orchestration 的价值应转向评估和审计，而非推理引导。
- **影响范围**: `domains/reasoning/manifest_prompt.go`（新）、`agentcore/evaluate/benchmark/live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（新增函数和测试，不改现有逻辑）
- **审查要求**: L2
- **验证**: `go build/vet/test` ✅ | P2B L4 10 题 live eval ✅

## 2026-07-16: 性能优化 Phase 3 — VectorIndex Search 最小堆优化

- **变更**:
  1. `knowledge/sqlite/vector_index.go`：新增 `minVectorHeap` 类型（`container/heap` 实现）。Search 方法中每个 worker 从 `make([]vectorMatch, n)` 全量分配+全排序改为 topK 大小最小堆，内存分配从 O(N/workers) 降到 O(K)。
- **动机**: 每个 worker 分配整个 shard（144K/12≈12K entries）的 `[]vectorMatch` + `sort.Slice` 全排序，benchmark 显示 4.7MB/op alloc。
- **收益**: alloc 4.7MB→26KB（降低 **99.4%**），速度 16.5ms→14.8ms（+10%）。
- **影响范围**: `knowledge/sqlite/vector_index.go`（1 个文件）
- **风险等级**: 低（搜索结果不变，只优化内存分配策略）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go test -race ./knowledge/sqlite/...` ✅ | `golangci-lint` ✅ | benchmark alloc 4.7MB→26KB ✅

## 2026-07-16: 性能优化 Phase 2 — PubSub PublishMustDeliver 解锁

- **变更**:
  1. `agentcore/pubsub.go`：`PublishMustDeliver` 改为 snapshot subscribers 模式。先短暂持 RLock 拷贝 `[]chan T`，释放锁后再逐个发送。新增 `<-b.done` select case 在发送过程中优雅退出。
- **动机**: 原 `PublishMustDeliver` 在 RLock 期间遍历所有 subscriber 发送，满 channel 最多阻塞 50ms/subscriber，N 个满 subscriber = N×50ms 总阻塞期间 RLock 不释放，阻塞 Subscribe/Unsubscribe。
- **收益**: 消除 PublishMustDeliver 对 Subscribe/Unsubscribe 的锁阻塞。
- **影响范围**: `agentcore/pubsub.go`（1 个文件）
- **风险等级**: 低（snapshot 期间新增的 subscriber 不会收到这条消息，可接受）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go test -race ./agentcore/...` ✅ | `golangci-lint` ✅

## 2026-07-16: 性能优化 Phase 1 — Session Lock O(1) LRU 改造

- **变更**:
  1. `session/session.go`：FileStore 锁缓存从 `lockOrder []string`（O(N) LRU）改为 `container/list` 双向链表（O(1) LRU）。`touchLock` 方法删除，LRU touch 内联为 `lockList.MoveToFront(elem)`。`lockCleanup` 同步清理 list entry。
  2. 新增 `lockEntry` 结构体（id + mu），`locks` 从 `map[string]*sync.RWMutex` 改为 `map[string]*list.Element`。
  3. 新增 `container/list` import。
- **动机**: `sessionLock` 是所有 session 操作的入口，全局 `locksMu` 串行化 + `touchLock` O(N) 线性扫描 `lockOrder` 切片（找到→删除→append），session 数增长后每次锁操作线性增长，成为并发瓶颈。
- **收益**: LRU touch/evict 从 O(N)→O(1)，持锁时间大幅缩短，降低锁争用。
- **影响范围**: `session/session.go`（1 个文件）
- **风险等级**: 中（sessionLock 属安全敏感路径，影响所有 session 操作的并发安全）
- **审查要求**: L3（sessionLock 是 session 生命周期核心）
- **验证**: `go build` ✅ | `go test -race ./session/...` ✅ | `golangci-lint` ✅


## 2026-07-16: LLMNodeBuilder 修复 + P2B 五步工具评估定论

- **变更**:
  1. `domains/reasoning/llm_node_builder.go`（新）：实现 `LLMNodeBuilder`，每个 PlanStep 真正调用 LLM 做分析，结果累积到 blackboard。修复了 `noopNodeBuilder`（唯一实现，只输出步骤名不调 LLM）导致五步工具是空框架的根因。
  2. `domains/reasoning/five_step_runner.go`：`formatResult` 输出实际分析内容（`### 分析过程`），不再只输出步骤名+JSON 元数据。移除 JSON dump 降低噪声。
  3. `domains/reasoning/handoff_integration.go`：`NewWorkflowRunner` 注入 `LLMNodeBuilder`（生产环境不再用 noop）。
  4. 清除 macOS 真实缓存路径（`os.TempDir()`=`/var/folders/.../T/` 而非 `/tmp/`）后重跑 P2B L2 10 题。
- **P2B L2 实测（LLMNodeBuilder，10题）**：llm_judge 均值 **0.334**，远低于 L1 的 0.513（−0.179）。6/10 题下降。
- **核心架构发现**：外部编排的分步推理（PlanStep→Pregel→5次LLM调用）不如 Agent 内部自主多轮推理（agent.Run）。五步工具把分析拆成 5 个独立 LLM 调用，破坏了推理连贯性；L1 让 Agent 整体端到端推理更优。3 题时测到的 0.700 是小样本偏差。
- **影响范围**: `domains/reasoning/llm_node_builder.go`（新）、`five_step_runner.go`、`handoff_integration.go`、`phase1_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 中（修改五步推理核心引擎，但 noop 仍可作为 fallback；phase1 测试已适配）
- **审查要求**: L3（涉及推理引擎架构变更）
- **验证**: `go build/vet/test` ✅ | reasoning 全量 test ✅ | P2B L2 3题+10题 live eval ✅

## 2026-07-15: 重建 P2B 真实案件基准 + 首次真实案件三层评估

- **变更**:
  1. 从 `/Users/xujian/projects/宝宸知识库_Raw/无效复审决定`（31562 件 MD 格式真实无效决定书）提取 100 条含完整案件事实的 TestCase，替换之前 40 条空壳数据。提取脚本 `scripts/extract_invalidation_cases.py`，修正了旧版正则锚点错误（旧版用 `独立权利要求1：` 但真实文档用 `权利要求书如下：`）。
  2. P2B 解冻：`suite.go ValidCases()` 重新加入 `InvalidationDecisionCases`，`live_deepseek_test.go` 更新冻结注释为 REBUILT。
  3. 新增 P2B Agent 评估测试：`TestLiveAgentP2BBaselineEval`（L1 无工具）、`TestLiveAgentP2BWorkflowEval`（L2 + invalidation manifest）。`TestLiveDeepSeekInvalidationEval` 加 `MADY_EVAL_CASES` 支持限量。
  4. 跑出 P2B 三层 10 题评估（稳定 judge），数据填入 `docs/evaluation-baseline-v0.7.md`。
- **P2B 三层实测**：L0 judge 0.363 → L1 Agent 0.513（+0.150，Agent 框架在真实案件上有显著增益）→ L2 invalidation manifest 0.407（−0.107，manifest 步骤设计需改进，与 P2A 结论一致）。
- **数据质量对比**：Input 平均 94→562 字符，权利要求非空 0%→70%，结论分布 34/5/1→42/33/25。
- **影响范围**: `scripts/extract_invalidation_cases.py`（新）、`agentcore/evaluate/benchmark/invalidation_decisions.json`、`suite.go`、`live_deepseek_test.go`、`live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（数据重建 + 测试新增；不改生产运行时逻辑）
- **审查要求**: L2
- **验证**: `go build/vet/test` ✅ | `make eval` ✅ | P2B L0/L1/L2 live eval ✅

## 2026-07-15: 建立稳定可靠的产品能力评估基线（L1/L2/L3 三层 10 题）

- **变更**: 在修复 judge 方差后，用稳定 judge（3-sample 中位数 + temperature 0.01）重跑 L1/L2/L3 三层各 10 题建立第一个可靠基线，更新 `docs/evaluation-baseline-v0.7.md` 的实时评估章节与关键发现。
- **稳定基线数据（llm_judge 均值）**：
  - **L1 Agent 框架**：0.700（PassRate 100%）
  - **L2 +五步推理**：0.700（PassRate 100%）—— **推翻方差噪声期「L2 有害 0.548」的错误结论**
  - **L3 +检索工具**：0.730（PassRate 100%）—— **修正方差噪声期「L3 双刃剑 0.658」的结论为「微弱正增益」**
- **关键发现**：
  1. L1=L2=0.700：五步推理工具在考试题上中性（信息完备的分析题不需要程序性流程），非「有害」。
  2. L3=0.730 微弱领先：`2018_a2_01`（保护客体）显著受益（0.60→0.80），但 patent/scholar 检索调用仍为 0。
  3. 稳定 judge 推翻了方差噪声期的全部工具效果结论，验证了「judge 方差是可靠评估前提」的判断。
- **原因**: judge 方差修复（两轮重复差异 0.000）后，需重跑获得可靠基线，作为后续优化的对照基准。
- **影响范围**: `docs/evaluation-baseline-v0.7.md`（实时评估数据全量替换为稳定 judge 结果 + 关键发现重写 + 下一步调整）
- **风险等级**: 低（仅文档更新）
- **审查要求**: L1

## 2026-07-15: 修复 LLM-as-judge 方差（temperature 修复 + 3-sample 中位数）

- **变更**:
  1. `agentcore/evaluate/llm_judge.go`：新增 `Samples` 字段，`Compute` 改为多次独立调用取中位数（`median` 辅助函数，比均值更抗离群值）。`computeOnce` 提取单次评分逻辑。Temperature 默认从 0（被 chatcompat 省略，导致非确定性）改为 0.01（通过 `>0` 检查，近似确定性）。
  2. `agentcore/evaluate/benchmark/suite.go`：`LiveEvaluator` 默认 `Samples=3`（`MADY_JUDGE_SAMPLES` 可调），`Samples=0` 保持单次向后兼容。
  3. `agentcore/evaluate/llm_judge_test.go`：新增 `TestMedian`（5 子用例）、`TestLLMJudge_SamplesTakesMedian`（3-sample 中位数验证）、`TestLLMJudge_SamplesDefaultSingleShot`（默认单次向后兼容）。
- **方差根源**：五轮 L2 实验发现同题 judge 分数跨轮波动达 0.71（`2012_a31_02` 从 0.88 到 0.17），使任何 ±0.05 的工具改进无法被可靠测量。根因有二：(a) `Temperature=0` 被 chatcompat 的 `>0` 检查跳过，judge 实际在非确定性 temperature 下运行；(b) 单次评分无统计降噪。
- **验证结果**：两轮 L1 重复实验（同 3 题），修复后两轮 judge 分数完全一致（差异 0.000），对比修复前同题跨轮波动 0.71。judge 方差已被彻底消除。
- **代价**：每题 judge 调用从 1 次增至 3 次（API 成本 ×3），`MADY_JUDGE_SAMPLES=1` 可降回单次。
- **影响范围**: `agentcore/evaluate/llm_judge.go`、`agentcore/evaluate/llm_judge_test.go`、`agentcore/evaluate/benchmark/suite.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（`Samples=0` 向后兼容；temperature 0.01 近似确定性；median 对单次调用是 no-op）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go vet` ✅ | evaluate 全量 test ✅ | 两轮 L1 live eval 方差验证 ✅（差异 0.000）

## 2026-07-15: 补全 drafting/invalidation manifest + 五轮 L2 实验定论

- **变更**:
  1. `domains/reasoning/manifest.go` 新增 `defaultDraftingManifest()`（5 步权利要求撰写）和 `defaultInvalidationManifest()`（5 步无效宣告分析），注册到 `DefaultManifests()`。步骤设计参考 Athena `task_1_4_write_claims.md`（从属权利要求四类型/A-B-C 保护范围策略）和 XiaoNuo `invalidity_checker.yaml`（4 步 SOP + 证据组合 4 方案 + 逐条独立论证约束）。
  2. `domains/reasoning/phase3_test.go` 补充 drafting/invalidation manifest 的断言（步骤数、multi_hypothesis 策略、RequireAllRulesUsed 约束）。
  3. `agentcore/evaluate/benchmark/live_agent_test.go` `caseTypeFromExamID` 修正：所有 P2A 法条统一映射 `patentability`（分析模板），A31 不再映射 drafting。原因：实验证明考试题是分析判断题（非完整程序任务），drafting manifest 的完整撰写流程偏离考点。
- **五轮 L2 实验最终结论**：五步工具在 P2A 考试题上始终无法稳定超越 L1（五轮均值 0.622/0.623/0.575/0.548 < L1 的 0.665）。但根因不是工具无用，而是 **LLM-as-judge 方差过大**（同一题跨轮次波动达 0.71），使任何 ±0.05 的工具改进效果无法被可靠测量。
- **核心教训**：(1) manifest 为真实案件设计，不能直接用于考试题（考试考分析，不考完整程序）；(2) LLM-as-judge 方差是当前评估方法的最大瓶颈，必须先解决（多次评分取均值/调整 rubric/交叉验证）才能可靠测量任何工具改进。
- **保留的代码**：drafting/invalidation manifest 保留——对真实案件场景（用户真的要撰写权利要求/提起无效宣告）有实务价值，只是不用于考试评估。
- **影响范围**: `domains/reasoning/manifest.go`、`domains/reasoning/phase3_test.go`、`agentcore/evaluate/benchmark/live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（新增 manifest + 测试断言；caseType 映射修正；不改生产运行时逻辑）
- **审查要求**: L2
- **验证**: `go build ./domains/reasoning/...` ✅ | `go vet` ✅ | manifest 测试 ✅ | `TestAgentWiringSmoke` ✅ | L2 五轮 live eval ✅

## 2026-07-15: 代码异味审查与修复（P0+P1+P2 批次）

- **变更**: 全项目代码异味审查后执行 7 项修复，覆盖参数爆炸、重复代码、超大函数、错误处理一致性。
- **具体修复**:
  1. **P0 `runCompaction` 参数爆炸**: 13 参数 → `CompactionParams` 结构体（`agentcore/compaction.go`），移除 2 个未使用参数（`compressionBaseURL/compressionAPIKey`）
  2. **P0 浏览器会话检查重复**: 24 处相同 3 行模式 → `RequireActiveSession()` helper（`tools/browser_session.go`）
  3. **P0 `runTui` 超大函数**: 771 行 / 138 分支 → 提取 `tuiSession` 结构体 + 15 个方法到 `cmd/mady/tui_session.go`（672 行），`runTui` 本体降至 ~130 行
  4. **P0 `runLoop` 超大函数**: 提取 `failLoop()` + `endTurn()` helpers，消除 10 处重复错误模式 + 4 处重复 turn-end 模式
  5. **P1 `computer_use.go` 工具定义**: 提取 `computerUseDescription()` + `computerUseSchema()` 从 `NewComputerUseTool` 中
  6. **P2 `mcp/client.go` 错误包装**: 5 处裸 `return err` 加 `fmt.Errorf("mcp ...: %w", err)` 上下文
  7. **P2 清理废弃 `components/` 包**: 删除已标注 Deprecated 且无引用的 RAG 接口包
- **验证**: `go build ./...` + `go test ./...`（根模块 63 包）+ `cd tools && go build && go test`（2 包）全通过
- **影响范围**: `agentcore/compaction.go`、`agentcore/agent_run.go`、`agentcore/context_engine.go`、`cmd/mady/main.go`、`cmd/mady/tui_session.go`（新增）、`tools/browser.go`、`tools/browser_tool.go`、`tools/browser_session.go`、`tools/computer_use.go`、`mcp/client.go`、`components/`（删除）
- **风险等级**: 中（涉及 `agentcore/agent_run.go` 核心运行循环，但提取的 helpers 保持原有控制流不变）
- **审查要求**: L2（核心运行循环改动）
- **未完成（后续批次）**: SemanticTheme 拆分为子结构体（41 字段）、ContextEngine 接口拆分（13 方法）、computer_use.go 按平台拆分

## 2026-07-15: 修复 L2 五步工具 caseType 硬编码（实测效果有限，如实记录）

- **变更**:
  1. 新增 `caseTypeFromExamID(caseID)`：从 P2A case ID 的法条标记（a2/a22/a26/a31/a33/r42）推断推理 CaseType（→patentability/drafting/invalidation/general_legal），取代之前对所有题固定 `CaseNoveltySearch` 的做法。
  2. 新增 `toolFactory` 类型和 `runAgentLiveEvalWithFactory`：支持按 case 动态构造工具集，使 L2 测试能为每道题构造 caseType 匹配的 `FiveStepRunner`。原 `runAgentLiveEval` 改为对 factory 的包装（传 nil），L1/L3 行为不变。
  3. `TestLiveAgentWithWorkflowEval` 改用 factory 模式。
- **实测结果（10 题）**：全部均值 0.622→0.623（+0.002），A22 题均值 0.633→0.656（+0.022），PassRate 维持 90%。**逻辑正确但整体效果有限**。
- **根因（caseType 不是唯一瓶颈）**：(1) `DefaultManifests()` 只有 novelty_search/patentability 两个模板，A31→drafting 等映射因无 manifest 退化为单步 fallback；(2) `2018_a2_01`（保护客体）无论 novelty_search 还是 patentability 都 FAIL；(3) LLM-as-judge 方差大（同映射下个别题 ±0.20 波动）。
- **保留修复的理由**：按法条推断 caseType 在逻辑上比一刀切更正确（A22 题微弱受益），且为未来补全 manifest 模板奠定路由基础。
- **影响范围**: `agentcore/evaluate/benchmark/live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（仅评估测试代码，不改生产代码；L1/L3 行为不变）
- **审查要求**: L2
- **验证**: `go vet ./agentcore/evaluate/benchmark/...` ✅ | `TestAgentWiringSmoke` ✅ | L2 10 题重跑 ✅（均值 0.623）

## 2026-07-15: 产品能力评估 10 题实测——小样本结论修正 + 三层完整诊断

- **变更**: 将 L1/L2/L3 三层从 3 题扩到 10 题（相同种子 20241201），获得稳健的产品能力基线。10 题数据**修正了 3 题小样本的多项结论**，已同步更新 `docs/evaluation-baseline-v0.7.md`。
- **10 题实测结果（llm_judge 均值）**：
  - **L1 Agent 框架**：0.665（PassRate 100%）—— 从 3 题的 0.833 回归真实水平
  - **L2 +五步推理**：0.622（PassRate 90%，FAIL `2018_a2_01`）—— **修正：3 题时 0.911 > L1 被判为「稳定增益」，10 题后 0.622 < L1 的 0.665，6/10 题下降**
  - **L3 +检索工具**：0.658（PassRate 90%，FAIL `2007_a22_01`）—— **修正：3 题时 0.761 被判为「工具过载」，10 题后揭示是「双刃剑」**
- **关键诊断**：
  1. **三层均值接近但方差极大**：L1=0.665/L2=0.622/L3=0.658，均值中性掩盖了工具效果的题型强相关。同一工具在不同题上效果天差地别（L3 的 `2018_a2_01` +0.53 vs `2007_a22_01` −0.47）。
  2. **五步工具 caseType 硬编码是 L2 根因**：`NewWorkflowRunner` 固定 `CaseNoveltySearch`，对非新颖性题（保护客体 A2）框架错配致崩（−0.20）。
  3. **L3 检索工具双刃剑**：对信息不足的题大幅提升（`2018_a2_01` +0.33、`2007_a31_02` +0.27），对信息完备的题严重干扰（`2007_a22_01` −0.40）。可观测性显示 `web_search` 高频调用（14-16 次/题）、`patent_lookup` 部分触发（0-3 次）、`scholar_search` 始终 0 次。
  4. **小样本陷阱实证**：3→10 题结论多次反转，验证了路线图停止规则「Golden Set 不能说明质量差异 → 不换模型/Prompt」的必要性。
- **下一步优先级**：(1) 修复五步工具 caseType 硬编码；(2) 检索工具精准触发（移除始终 0 调用的 scholar_search）；(3) 扩到全量 31 题。
- **原因**: 用户要求扩到 10 题验证稳定性。结果证实了扩样本的必要性——3 题的乐观结论被 10 题推翻，避免了在错误方向上优化。
- **影响范围**: `docs/evaluation-baseline-v0.7.md`（三层 10 题数据 + 趋势修正 + 诊断 + 下一步）、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（仅文档更新，无代码改动）
- **审查要求**: L1

## 2026-07-15: 产品能力评估实测——L0→L1→L2 梯度验证 + L3 工具过载诊断

- **变更**:
  1. 从 `~/.zshrc` 加载 `DEEPSEEK_API_KEY`（仅注入环境变量，不落盘、不入仓），对 L0/L1/L2/L3 四层各跑 3 道 P2A 真题（固定种子 20241201，共享相同题目，通过率可横向对比），实测数据已填入 `docs/evaluation-baseline-v0.7.md`。
  2. L3 测试修复：`TestLiveAgentWithPatentToolsEval` 的 `PatentToolConfig` 从空配置改为 `tools.PatentToolConfigDefaults()`，正确读取 `NUO_PATENT_PATH` 环境变量解析本地 nuo-patent 构建。
- **实测结果（llm_judge 均值）**：
  - **L0 裸 LLM**：0.533（PassRate 66.7%）
  - **L1 Agent 框架（无工具）**：0.833（PassRate 100%）—— Agent 多轮生成较裸 LLM 单轮回复增益 +0.300
  - **L2 +五步推理**：0.911（PassRate 100%）—— 结构化五步工具增益 +0.078
  - **L3 +检索工具**：0.761（PassRate 100%）—— 反降，工具调用可观测性诊断出根因：Agent 对 patent_lookup/scholar_search 调用 0 次，BuildTools 装配的 14 个工具中 Agent 选择了 read/grep/ls 等通用文件工具，注意力被分散
- **关键诊断**：
  - L0→L1→L2 的递进在全部 3 题上一致（均值单调上升），初步证明 Mady 产品能力的可量化价值。
  - L3 暴露两个问题：(a) `tools.BuildTools` 一次性装配 14 个工具导致工具过载；(b) P2A 考试真题题干已含全部信息，无法体现检索工具价值，需设计「需外部检索」的专属评估场景。
  - 工具调用可观测性（countingTool）是定位 L3 问题的决定性手段——没有逐工具计数，0.761 只是模糊的「分数下降」信号。
- **原因**: 用户要求用项目内 API key 跑出真实数据。实测首次量化了 Mady 产品能力相比裸 LLM 的增益，并验证了评估基础设施的有效性。
- **影响范围**: `agentcore/evaluate/benchmark/live_agent_test.go`（L3 配置修复）、`docs/evaluation-baseline-v0.7.md`（实测数据填充）、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（仅测试配置修复与文档更新；API key 仅注入环境变量未落盘）
- **审查要求**: L2
- **验证**: L0/L1/L2/L3 四层 live eval 全部跑通 ✅ | `go vet ./agentcore/evaluate/benchmark/...` ✅

## 2026-07-15: 建立产品能力评估三层对比基线（评估质量提升 阶段2-3）

- **变更**:
  1. **新建 `agentcore/evaluate/benchmark/live_agent_test.go`**：将 live evaluation 的 RunFunc 从「裸 `Provider.Complete`」升级为「完整 `agentcore.Agent` runtime」，首次让评估测出 Mady 产品能力而非模型裸能力。包含：
     - `agentRunFunc`：每个 case 构造独立 Agent（避免跨 case 状态污染），MaxTurns=20，装配可选工具，通过 `agent.Run(ctx, input)` 返回最终答案文本。
     - **三层对比测试**（共享 P2A 用例 + 固定种子 20241201，通过率可直接横向对比）：`TestLiveAgentBaselineEval`（Agent 无工具，校验框架无退化）、`TestLiveAgentWithWorkflowEval`（+`run_five_step_workflow` 五步推理工具，retriever nil 走优雅降级）、`TestLiveAgentWithPatentToolsEval`（+`patent_lookup`/`patent_legal`/`scholar_search` 检索工具，受 `MADY_EVAL_PATENT_TOOLS=1` 额外门控）。
     - **工具调用可观测性**（阶段3）：`toolCallCounter` 通过 `atomic.Int64` 包装每个工具的 Func，记录每题工具调用次数，区分「工具未被调用」与「工具结果未被有效利用」两种失败模式。
     - **离线装配链路 smoke test** `TestAgentWiringSmoke`（无 API key 门控，CI 可运行）：用 `stubProvider` 验证三层装配（Config 构造、workflow 工具注入、patent 工具装配、countingTool 计数）端到端可用。
  2. **新建 `docs/evaluation-baseline-v0.7.md`**：记录三层产品能力评估方法论（L0 裸 LLM / L1 Agent 框架 / L2 +五步推理 / L3 +检索工具）、静态评估结果、待填实时数据表格、用户运行操作指南。
- **原因**: v0.6 审阅发现 live eval 直接调裸 `Provider.Complete`，不装 Tools、不走 Agent runtime，32.5% 通过率测的是 DeepSeek 裸读题能力，与 Mady 核心价值（知识检索+五步推理+工具）完全脱节。优化 Prompt 提升的是模型能力而非产品能力。v0.7 让评估首次对齐产品价值，三层对比能定位增益来源或暴露集成断点。
- **影响范围**: `agentcore/evaluate/benchmark/live_agent_test.go`（新）、`docs/evaluation-baseline-v0.7.md`（新）
- **风险等级**: 低（仅新增测试文件与文档；不改生产代码；live test 受 `MADY_LIVE_EVAL=1` 门控，CI 自动跳过；离线 smoke test 用 stub provider 无网络调用）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./agentcore/evaluate/...` ✅ | `make eval` ✅（GoldenPerfect 等全绿）| `TestAgentWiringSmoke` 三层装配链路 ✅

## 2026-07-15: 冻结 P2B 空壳数据集，修正评估口径（评估质量提升 阶段1）

- **变更**:
  1. `agentcore/evaluate/benchmark/suite.go` 新增 `ValidCases()` 函数，返回排除冻结的 P2B（`InvalidationDecisionCases`）后的有效用例集；`AllCases()` 保持不变（仍含 P2B，供静态 CI 门禁 `RunStatic` 校验全部注册用例的结构完整性）。
  2. `agentcore/evaluate/benchmark/live_deepseek_test.go` 的 `TestLiveDeepSeekInvalidationEval` 顶部增加 `P2B FROZEN` 注释，记录冻结原因（空壳输入 40/40、退化分布 34/5/1、评估口径为裸 LLM）；保留测试代码与缓存以备数据重建后复用。
  3. `docs/evaluation-baseline-v0.6.md` 修正 P2B 结论分布（误记 16/14/10 → 实际 34/5/1），新增「⚠️ P2B 已冻结」说明章节，记录空壳输入与退化分布两个根本性缺陷及冻结处置。
  4. `docs/roadmap.md` P2B 里程碑状态由 ✅ 改为 ✅→❄️ 冻结，追加冻结原因与下一阶段基准说明。
- **原因**: 开发进度审阅中发现 P2B 的 40 条无效决定书 TestCase 的 Input「独立权利要求1/主要证据/请求理由」三个字段全部为空（40/40），实际结论分布严重失衡（全部无效 34 / 部分无效 5 / 维持有效 1，文档曾误记为 16/14/10）。在此数据集上优化 Prompt/模型得到的提升是虚假信号（换组数据即归零），且当前 live eval 直接调裸 `Provider.Complete` 不走 Agent runtime，32.5% 通过率测的是 DeepSeek 裸读空壳题目的猜测能力而非 Mady 产品能力。冻结 P2B 消除虚假信号，下一阶段以 P2A（31 道真题，数据质量良好）为唯一有效 live evaluation 基准。
- **影响范围**: `agentcore/evaluate/benchmark/suite.go`、`agentcore/evaluate/benchmark/live_deepseek_test.go`（仅注释）、`docs/evaluation-baseline-v0.6.md`、`docs/roadmap.md`
- **风险等级**: 低（`ValidCases()` 为新增函数不改变现有行为；`AllCases()` 与静态门禁保持不变；live test 仅加注释）
- **审查要求**: L2
- **验证**: `go build ./agentcore/evaluate/...` ✅ | `go vet ./agentcore/evaluate/...` ✅ | `go test -race ./agentcore/evaluate/...` ✅ | `make eval` ✅（GoldenPerfect/Degraded/CaseIntegrity/DefaultEvaluator 全绿）

## 2026-07-15: Go 规范开发文档制定 + 全仓库合规修复（4 批次）

- **变更**:
  1. 产出 `docs/GO-DEVELOPMENT-STANDARDS.md`（13 章），整合 Go 业界最佳实践与 Mady 实际代码模式
  2. 对照规范进行全仓库审阅，产出两份审计报告：`docs/review/2026-07-15-standards-review.md` + `docs/review/2026-07-15-security-sensitive-paths-audit.md`
  3. **批次 1（P0）并发安全 + 错误忽略**：server/disclosure.go goroutine 加 recover；browser_session.go ticker 加 stopCh+recover；browser_lightpanda.go `%v`→`%w`；21 处 json.Marshal 错误检查；2 处 json.Encode 错误检查；conn.Write 错误检查；3 处全局状态改为注入方式（browser.go/browser_advanced.go/browser_supervisor.go）；agentcore/mcp 结构化错误推广（NewRetryableError/NewFatalError）
  4. **批次 2（P0）零测试覆盖 + 签名**：protocol/jsonrpc 7 个测试用例；workflows/patent+legal 工具构造测试；domains/reasoning/collector 4 个 Collector + 工具函数测试；integration/ 包签名 `package integration`→`package integration_test`；5 处 time.Sleep(>100ms)→channel/sync 替换；4 个关键文件导出符号注释（agentcore/event.go 17 个、server/stream_events.go 23 个、server/server.go 12 个、mcp/client.go 7 个）
  5. **批次 3（P1）并发 safety net + context**：10 个 goroutine 添加 panic recovery（mcp/discovery.go 6 个、mcp/tools_refresh.go、tui/theme/watch.go、a2a/server.go 2 个、acp/server.go）；tools/browser_advanced.go `os.Exit(0)`→`close(ShutdownCh)`；22 处 context.Background() 传播替换（memory/sqlite_store.go、domains/sqlite/approval_store.go、domains/reasoning/sqlite/checkpoint_store.go、tools/browser_session.go）；acp 测试套件 133 子测试 + 48.7% 覆盖率
  6. **批次 4（P2）长期改进**：22 个模块 doc.go；13 个大文件添加 TODO(refactor) 注释；14 处 time.After→time.NewTimer；8 个接口方法添加 context.Context 参数（vision/git/patch/edit/ls/delete/grep/find/read）；`interface{}`→`any` 迁移；domains/router.go AllowedSources 白名单一致性修复；tools/bash.go 临时文件清理 goroutine 改进
- **原因**: 对全仓库进行系统性 Go 规范审阅后的合规修复，覆盖所有 P0/P1/P2 发现项
- **影响范围**: 69 个文件，+514/-151 行变更；6 个新测试文件；22 个新 doc.go
- **风险等级**: 中（涉及安全敏感路径 guardrails/levels.go 等，审计确认无安全问题）
- **审查要求**: L2+
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` ✅ | `go test -race ./tools/...` ✅

## 2026-07-15: 修复 Medium/Low 技术债务 22 项（分 5 个 WP）

- **变更**:
  1. **WP7 性能与随机性**：`a2a/client.go`/`a2a/server.go` 重试循环将 `time.After` 替换为 `time.NewTimer` 避免泄漏；`tools/browser_session.go`/`tools/web_fetch.go` 浏览器指纹改用 `crypto/rand`（新增 `tools/rand.go` 的 `cryptoIntn`）；`agentcore/retry.go` 的 `applyFullJitter` 改用 `crypto/rand`。
  2. **WP8 数据一致性与并发**：`agentcore/provider.go` 新增 `CallConfig.Equal` 与配套辅助函数；`session/agent_store.go` 移除 `reflect.DeepEqual`，改用 `Equal` 与逐字段比较；`domains/approval.go` 的 `MemoryApprovalStore` 将 `mu` 升级为 `sync.RWMutex` 并在 `List` 中使用 `RLock`；新增 `agentcore/provider_test.go`。
  3. **WP9 代码质量与常量**：删除 `disclosure/graph.go` 未使用的 `j`；`workflows/legal/comparison.go` 将未使用的 `query` 嵌入占位 `cases` 字符串；`domains/reasoning/planner.go` 将 `maxFactsInPrompt` 常量改为基于 `contextWindow` 的动态 `factsInPromptLimit`；更新 `Makefile` 中 `eval` 目标的注释以准确反映其仅运行 benchmark。
  4. **WP10 可移植性与安全**：`knowledge/loader/main_test.go`/`wiki_test.go` 将硬编码 `/tmp/wiki_test` 改为 `TestMain` 创建的临时目录；`Makefile` 将 `install-lint` 的 golangci-lint 版本提取为 `GOLANGCI_LINT_VERSION` 变量；`tools/tools.go` 新增 `ComputerUseConfig` 字段并传入 `NewComputerUseTool`；`domains/project.go` 的 `ValidateProjectPath` 增加 `filepath.EvalSymlinks` 解析并拒绝损坏符号链接，新增对应单元测试。
  5. **WP11 可扩展性与生成代码**：`agentcore/manifest.go` 暴露 `RegisterValidDomain`/`RegisterValidGuardrailLevel` 并加锁保护 `validDomains`/`validGuardrailLevels`；`guardrails/levels.go` 暴露 `RegisterLevel`/`RegisteredLevel` 并在注册时同步到 agentcore 校验表；`domains/reasoning/planner.go` 更新 ReAct/MultiHypothesis 分支注释；`agentcore/evaluate/benchmark/invalidation_decisions.go` 将 `init()` 中的 `panic` 改为 `stderr` 日志记录，避免生成数据损坏时整个程序崩溃。
- **原因**: 全仓库技术债务扫描后续阶段识别出 Medium/Low 风险点，涉及性能泄漏、密码学安全随机源、数据一致性、并发锁粒度、代码质量、可移植性、扩展性与生成代码健壮性。
- **影响范围**: `a2a/client.go`/`server.go`、`tools/rand.go`/`browser_session.go`/`web_fetch.go`/`tools.go`、`agentcore/retry.go`/`provider.go`/`provider_test.go`/`manifest.go`、`session/agent_store.go`、`domains/approval.go`、`disclosure/graph.go`、`workflows/legal/comparison.go`、`domains/reasoning/planner.go`、`domains/project.go`/`project_test.go`、`knowledge/loader/main_test.go`/`wiki_test.go`、`Makefile`、`guardrails/levels.go`、`agentcore/evaluate/benchmark/invalidation_decisions.go`。
- **风险等级**: 中（涉及安全敏感路径 `guardrails/levels.go`、`domains/project.go`、`agentcore/manifest.go`）
- **审查要求**: L2+
- **验证**: `go build ./...` ✅ | `cd tools && go build ./...` ✅ | `go test -race ./...` ✅ | `cd tools && go test -race ./...` ✅ | `go vet ./...` ✅

## 2026-07-15: 技术债务修复质量审阅与回归修复

- **变更**:
  1. 恢复 `domains/approval_test.go` 中被误删的既有测试集（keyword trigger、SkipIfNoTools、message build/truncate、default config、RequireApproval、MemoryApprovalStore、RecordDecision 等），并保留新增 `State` 字段的测试。
  2. 完善 `session/agent_store.go` 的 `messagesEqual`：在 Role/Content/ToolCalls 基础上补充比较 `ID`、`ToolCallID`、`Name`、`Type`、`InvocationID`、`CacheControl` 及 `Metadata`/`Blocks` 复杂字段，避免消息前缀判断遗漏差异。
  3. 修复 `agentcore/provider.go` 的 `responseFormatEqual`：将 `JSONSchema` 指针相等比较改为结构体深度比较（含 `Schema` map），确保 `CallConfig.Equal` 能正确识别相同配置。
  4. 修复 `knowledge/eval.go` 的 `evalResultEvent`：在通过 `EmitEvent` 发送时设置 `at: time.Now()`，避免事件时间为零值。
  5. 安装 `golangci-lint` v2.12.2 并修复其报出的 4 个问题：`agentcore/tool_gen.go`/`tool_gen_test.go` gofmt 格式化；`agentcore/tool_gen.go` 将 `reflect.Ptr` 改为 `reflect.Pointer`；`knowledge/fileindex/extension_test.go` 将 `cancelled` 改为 `canceled`；`tools/bash.go` 去除 `killProcessTree` 的空错误分支。
- **原因**: 对 WP1-WP11 全量修复进行质量审阅时发现回归/不完整点（测试覆盖丢失、相等性比较遗漏字段、事件时间未初始化），并在补装 lint 后发现既有代码的格式/拼写/静态分析问题，一并修复以达到提交标准。
- **影响范围**: `domains/approval_test.go`、`session/agent_store.go`、`agentcore/provider.go`、`knowledge/eval.go`、`agentcore/tool_gen.go`/`tool_gen_test.go`、`knowledge/fileindex/extension_test.go`、`tools/bash.go`。
- **风险等级**: 低
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `cd tools && go build ./...` ✅ | `go test -race ./...` ✅ | `cd tools && go test -race ./...` ✅ | `go vet ./...` ✅ | `make lint` ✅

## 2026-07-15: 修复 Critical/High 技术债务 19 项（分 6 个 WP）

- **变更**:
  1. **WP1 并发崩溃安全**：`graph/pregel.go` 节点 goroutine 增加 panic recover；`mcp/client.go` readLoop 增加 panic recover 并记录 unmarshal 错误。
  2. **WP2 进程安全与库 API**：`tools/bash.go` 加固 `killProcessTree`（校验 PID、处理错误、幂等）；`pkg/agentconfig/provider.go` 移除 `log.Fatal`，`BuildProvider` 改为返回 `(Provider, error)`，更新所有调用方（`cmd/mady/main.go`、`example/acp-server`、`tui/agent_integration_test.go`）。
  3. **WP3 工具可移植性与网络错误**：`tools/patent_search.go` 默认路径改为 `NUO_PATENT_PATH`/`nuo-patent`，校验所有 `json.Unmarshal` 与 `MkdirAll` 错误；`a2a/ws.go` 所有 WebSocket 写操作检查错误并在失败时关闭连接；`tools/browser_tool.go` 关键 `chromedp.Run` 错误记录/传播；`tools/computer_use.go` 使用 `os.TempDir()` 替换 `/tmp`，移除未使用的 `raw`。
  4. **WP4 领域层与 graph 解耦**：`domains/reasoning` 新增 `GraphBuilder` 接口与 `PregelNode/PregelState/PregelEdgeRouter` 类型别名；`BuildMultiHypothesisSubgraph`/`buildChainStep`/`buildReActStep` 全部传播图构建错误。
  5. **WP5 上下文与资源泄漏**：`mcp/client.go` `Close()` 强制 kill 后仍给短超时等待并记录日志，重连失败时异步等待带超时；`acp/session.go` `CreateSession`/`RestoreSession`/`ForkSession`/`loadPersistedSessions` 增加 `ctx` 参数；`acp/server.go` 将请求 `ctx` 传入认证与会话 handler。
  6. **WP6 CLI 默认值与数据一致性**：`cmd/mady/main.go` 显式检查 `fs.Parse` 错误，硬编码端点/模型集中到 `pkg/agentconfig` 常量；`domains/approval.go` `RecordDecision` 根据决策设置 `State`；`knowledge/eval.go` 实现 `LogResults` 事件发送；`session/session.go` 分支复制标签错误返回而非忽略。
- **原因**: 全仓库技术债务扫描识别出 7 项 Critical 与 12 项 High 风险点，涉及并发崩溃、进程安全、库 API、领域层依赖倒置、上下文传播、CLI 默认值与数据一致性。
- **影响范围**: `graph/pregel.go`/`pregel_test.go`、`mcp/client.go`、`tools/bash.go`/`bash_test.go`、`pkg/agentconfig/provider.go`/`defaults.go`/`provider_test.go`、`cmd/mady/main.go`、`example/acp-server/main.go`、`tui/agent_integration_test.go`、`tools/patent_search.go`、`a2a/ws.go`、`tools/browser_tool.go`、`tools/computer_use.go`、`domains/reasoning/graph.go`/`plan_compiler.go`/`multi_hypothesis.go`/`graph_test.go`、`acp/session.go`/`server.go`、`domains/approval.go`/`approval_test.go`、`knowledge/eval.go`、`session/session.go`。
- **风险等级**: 高（跨多个核心模块，含 API 签名变更）
- **审查要求**: L2+
- **验证**: `go build ./...` ✅ | `cd tools && go build ./...` ✅ | `go test -race ./...` ✅ | `cd tools && go test -race ./...` ✅ | `go vet ./...` ✅

## 2026-07-15: search_project_files / read_project_file 工具支持无 /case 降级模式

- **变更**:
  1. `knowledge/fileindex/extension.go`：当 `FileIndex` 为 nil 时，`read_project_file` 降级为直接文件系统读取（使用 `FileReader`），`search_project_files` 降级为 `WalkDir` 文件名/路径子串搜索（`searchFallback`）。降级搜索跳过隐藏目录和 `node_modules`/`vendor`，按匹配质量（精确/前缀/包含/路径）分层评分和排序，支持 context 取消，传播 WalkDir 错误。在 `ExtensionConfig` 新增 `FallbackDir` 字段，`Extension` 新增 `SetFallbackDir`/`workingDir` 方法。
  2. `knowledge/fileindex/extension_test.go`：新增 10 个 `searchFallback` 单元测试，覆盖匹配、无匹配、大小写、隐藏目录跳过、node_modules 跳过、maxResults 截断、评分排序、空目录、ctx 取消、目录缺失。
  3. `cmd/mady/main.go`：`NewExtension` 传入 `FallbackDir: fc.BaseConfig.ProjectDir`；`/case off` 时调用 `SetFallbackDir` 重置。
- **原因**: 用户反馈使用 `read_project_file` / `search_project_files` 前必须先执行 `/case`，流程繁琐。`FileReader.ReadProjectFile` 本身不依赖 `FileIndex`，完全可以独立工作。降级模式消除了这个不必要的障碍。
- **影响范围**: `knowledge/fileindex/extension.go`、`knowledge/fileindex/extension_test.go`、`cmd/mady/main.go`
- **风险等级**: 低（有 FileIndex 时行为完全不变；降级模式复用现有 `FileReader` + `FileReader.resolvePath` 沙箱）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go test ./knowledge/fileindex/... -count=1` ✅（28 测试全通过）

## 2026-07-14: 修复 MCP 发现超时后 Close() 阻塞导致 tui/acp 无法启动

- **变更**:
  1. `mcp/client.go`：`Close()` 关闭 stdout/stderr pipes 以唤醒 readLoop/captureStderr；强制 kill 后给 `cmd.Wait()` 增加 2 秒上限，避免 npx/npm exec 派生的孙子进程持有 pipes 时无限阻塞；使用进程组（`Setpgid` + `kill -pgid`）清理整个进程树。
  2. `mcp/process_unix.go` / `mcp/process_windows.go`：新增平台特定的 `setProcessGroup` 与 `killProcessTree` 辅助函数。
  3. `mcp/config_discovery.go`：`DiscoverMCPExtensions` 的 `wg.Wait()` 改为与 `discCtx.Done()` 竞争，超时后记录警告并返回，不再被单个 `Close()` 阻塞。
  4. `cmd/mady/main.go`：`setupFrameworkContext` 接收 `ctx`，并传给 `DiscoverMCPExtensions`，使用户 Ctrl+C 可取消启动流程。
- **原因**: 此前 10 秒总超时只能取消 `initialize` 调用，但超时后 `createExtension` 内部 `Close()` 在清理 npx/npm exec 派生的孙子进程时会因 `cmd.Wait()` 被持有 pipes 的孤儿进程阻塞而永不返回，导致 `wg.Wait()` 挂死，`setupFrameworkContext` 无法完成，TUI/ACP 启动卡住。
- **影响范围**: `mcp/client.go`、`mcp/process_unix.go`、`mcp/process_windows.go`、`mcp/config_discovery.go`、`cmd/mady/main.go`
- **风险等级**: 中（修改 MCP client 生命周期与进程清理逻辑）
- **审查要求**: L2
- **验证**: `go test -race ./mcp/...` ✅ | `go vet ./cmd/mady ./mcp` ✅ | `mady acp` 不再永久阻塞 ✅

## 2026-07-14: 本地部署 mady 并接入 Zed ACP；修复 MCP 发现阻塞启动

- **变更**:
  1. 本地构建并部署 `mady` 到 `/usr/local/bin/mady`（同时保留 `/opt/homebrew/bin/mady` 副本），使其在任意 cwd 下可用
  2. 新增 wrapper 脚本 `/opt/homebrew/bin/mady-acp-zed`：从 `~/.mady/env` 加载环境变量、检查 LLM API key、设置 `MADY_SKIP_MCP_DISCOVERY=1` 后启动 `mady acp`
  3. 在 `~/.config/zed/settings.json` 的 `agent_servers` 中添加 `Mady` custom server，命令指向 wrapper 脚本
  4. 修改 `mcp/config_discovery.go`：
     - `DiscoverMCPExtensions` 支持 `MADY_SKIP_MCP_DISCOVERY=1` 完全跳过发现
     - `DiscoverMCPExtensions` 改为并行创建 extension，并增加 10 秒总超时，避免单个 hung MCP server 阻塞 mady 启动
     - `createStdioExtension` 默认设置 15 秒 `RequestTimeout`，避免无响应 stdio server 永久阻塞
- **原因**: 用户要求在任何项目使用 `mady tui` 启动 TUI，并将 ACP 接入 Zed；实际测试发现本地 `~/.claude.json` 中的多个 MCP server 在串行初始化时无响应，导致 `mady tui` / `mady acp` 启动被挂起（甚至触发 OOM/SIGKILL）
- **影响范围**: `mcp/config_discovery.go`、本地二进制 `/usr/local/bin/mady`、wrapper `/opt/homebrew/bin/mady-acp-zed`、Zed 配置 `~/.config/zed/settings.json`
- **风险等级**: 中（修改 MCP 发现流程，影响 tui/serve/acp 的 MCP 加载行为）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `cd tools && go build ./...` ✅ | `go test ./mcp/... -count=1` ✅ | `go test -count=1 ./...` ⚠️（仅 `tui/terminal` 的 `TestTerminalSupportsKittyKeyboard_Detection/apple_terminal` 因环境差异失败，与本次改动无关）| `mady --help` ✅ | `/opt/homebrew/bin/mady-acp-zed` 响应 ACP initialize ✅ | `mady tui` 在 `/tmp` 初始化成功 ✅

## 2026-07-14: 修复 CI 中 tui 集成测试因缺少 API key 失败

- **变更**:
  1. 在 `tui/agent_integration_test.go` 新增 `hasAPIKey()` 辅助函数，检测 `API_KEY`、`DEEPSEEK_API_KEY`、`ZHIPU_API_KEY`、`KIMI_CODE_API_KEY`、`KIMI_API_KEY`、`OPENAI_API_KEY` 等环境变量
  2. 在 `TestAgentRunInTUISession` 开头增加无 API key 时 `t.Skip`，避免 CI 环境（无真实 LLM key）触发 `agentconfig.BuildProvider()` 的 `log.Fatal`
- **原因**: GitHub Actions 最新一次 push CI（run 29336497361）在 `test (root, ubuntu-latest)` 的 `go test` 步骤失败，错误为 `API_KEY (or provider-specific env var) is required`；该测试是集成测试，不应在缺少外部凭证的 CI 环境中强制运行
- **影响范围**: `tui/agent_integration_test.go`
- **风险等级**: 低（仅调整测试跳过逻辑，未改动业务代码）
- **审查要求**: L1
- **验证**: `env -u ... go test ./tui -run TestAgentRunInTUISession` ✅（正确 SKIP）| `go test -race ./tui -count=1` ✅ | `go vet ./tui` ✅ | `gofmt -l` ✅

## 格式

```
## YYYY-MM-DD: 标题

- **变更**: 做了什么
- **原因**: 为什么做
- **影响范围**: 涉及哪些包/文件
- **风险等级**: 低/中/高
- **审查要求**: L1-L4
```

## 2026-07-14: 修复 CitationCompleteness 中文数字与阿拉伯数字不匹配问题

- **变更**:
  1. 在 `agentcore/evaluate/metrics.go` 中重写 `CitationCompleteness.Compute`：新增中文数字转阿拉伯数字归一化、结构化法条引用提取（`第X条` / `第X条第Y款`）、概括匹配（`第X条` 可命中 `第X条第Y款`），并保留对非数字引用（如 `CN123`）的字符串匹配兼容
  2. 在 `agentcore/evaluate/evaluate_test.go` 新增三个测试覆盖中文数字匹配、子串误匹配规避、款级概括匹配
  3. 使用已有缓存重新运行 `TestLiveDeepSeekInvalidationEval`，验证修复效果：P2B 无效决定书基线通过率从 15.0%（6/40）提升至 32.5%（13/40），`citation_completeness` 从 0.287 提升至 0.775，`llm_judge` 从 0.381 微升至 0.408
  4. 更新 `docs/evaluation-baseline-v0.6.md`，新增修复后基线与修复详情章节
- **原因**: P2B 基线分析显示 `citation_completeness` 仅 0.287，主因是模型输出常用汉字数字（如「第二十二条第三款」），而 `RequiredCitations` 使用阿拉伯数字（如「第22条第3款」），导致字面匹配失败；同时简单子串匹配会把「第2条」误判为「第22条」的子串命中
- **影响范围**: `agentcore/evaluate/metrics.go`、`agentcore/evaluate/evaluate_test.go`、`docs/evaluation-baseline-v0.6.md`
- **风险等级**: 中（修改核心评估指标 `CitationCompleteness`，影响所有使用该指标的 benchmark 与 live eval 结果）
- **审查要求**: L2
- **验证**: `go test -v -run TestCitation ./agentcore/evaluate/...` ✅ | `go test -race ./agentcore/evaluate/...` ✅ | `go vet ./...` ✅ | `make eval` ✅ | `MADY_LIVE_EVAL=1 ... TestLiveDeepSeekInvalidationEval` ✅（40 题，13/40 通过，citation_completeness 0.775，llm_judge 0.408）

## 2026-07-14: 执行 P2B — 构建真实无效决定书 Golden Set 第二层并建立 LiveEval 基线

- **变更**:
  1. 从本地数据 `/Users/xujian/Downloads/专利无效数据`（202601-202604 四个 zip，共 2009 件无效宣告请求审查决定书 docx）中，按发明/实用新型/外观设计 × 全部无效/部分无效/维持有效 的配额筛选出 40 件典型案例
  2. 新建 `agentcore/evaluate/benchmark/invalidation_decisions.go`，将 40 件决定书转化为 `evaluate.TestCase` 格式（ID：`invalidation_decision_001` ~ `invalidation_decision_040`）
  3. 更新 `agentcore/evaluate/benchmark/suite.go`，将 `InvalidationDecisionCases` 注册到 `AllCases()`
  4. 新增 `TestLiveDeepSeekInvalidationEval`，使用 DeepSeek 对全部 40 道无效决定书案例进行实时评估
  5. 保存 40 道模型输出缓存到 `docs/evaluation-baseline-invalidation-p2b.json`
  6. 新建 `docs/evaluation-baseline-v0.6.md`，记录 P2B 基线：通过率 15.0%（6/40），`citation_completeness` 0.287，`llm_judge` 0.381
- **原因**: 你指出脱敏案件难获取，建议改用真实专利复审/无效决定书作为第二层评估数据；本地 2009 件决定书提供了充足覆盖，需要结构化提取、接入 Golden Benchmark 并建立 LLM 基线
- **影响范围**: `agentcore/evaluate/benchmark/invalidation_decisions.go`（新）、`agentcore/evaluate/benchmark/suite.go`、`agentcore/evaluate/benchmark/live_deepseek_test.go`、`docs/evaluation-baseline-invalidation-p2b.json`（新）、`docs/evaluation-baseline-v0.6.md`（新）
- **风险等级**: 低（仅新增 benchmark 数据集与测试；未改变现有评估逻辑）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go test -race ./agentcore/evaluate/...` ✅ | `make eval` ✅ | `MADY_LIVE_EVAL=1 ... TestLiveDeepSeekInvalidationEval` ✅（40 题，DeepSeek 6/40 通过，citation_completeness 0.287，llm_judge 0.381）

## 2026-07-14: 执行 P2A — Golden Set 第一层建设完成

- **变更**:
  1. 确认 `agentcore/evaluate/benchmark/` 已集成 31 道公开专利考试真题（A2/A22/A26/A31/A33/R42 六组），作为 Golden Set 第一层
  2. 运行 `make eval` 验证静态评估门禁：`TestEvalSuite_GoldenPerfect` / `Degraded` / `CaseIntegrity` / `DefaultEvaluator` 全绿
  3. 运行 `MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...`，使用 DeepSeek 对随机 3 道真题建立 LLM 基线：通过率 66.7%（2/3），`citation_completeness` 1.0，`llm_judge` 平均 0.456
  4. 更新 `docs/roadmap.md`：将 P2A 标记为完成，并记录关键数据
  5. 新建 `docs/evaluation-baseline-v0.5.md`，记录 Golden Set 第一层构成与 LiveEval 基线
- **原因**: 路线图要求 10-12 月首先完成 Golden Set 第一层；项目已集成 31 道真题，但缺少阶段确认、LiveEval 基线记录与文档更新
- **影响范围**: `docs/roadmap.md`、`docs/evaluation-baseline-v0.5.md`（新），未修改代码
- **风险等级**: 低（仅文档更新与验证；代码未变更）
- **审查要求**: L1
- **验证**: `make eval` ✅ | `go test -race ./agentcore/evaluate/benchmark/...` ✅ | `MADY_LIVE_EVAL=1 ... TestLiveDeepSeekEval` ✅



- **变更**:
  1. 在 `tui/chat/chat_app.go` 的 `chatLayout.Update` 中为 Escape 键增加退出逻辑：自动补全未激活时按下 Escape → 退出
  2. 在 `chatLayout.Update` 的 Ctrl+C 处理中增加退出分支：无选中文本且 Agent 未运行时按下 Ctrl+C → 退出
  3. 保留原有行为：有选中文本时 Ctrl+C/Cmd+C 仍优先复制；Agent 运行时 Ctrl+C 仍优先中断
- **原因**: 用户在上次修复后仍反馈 TUI 无法输入、Ctrl+C 无法退出。深入分析发现：原始终端模式下 ISIG 被清除，Ctrl+C 以 0x03 到达而非 SIGINT；`chatLayout.Update` 的 Ctrl+C 处理仅包含复制和中断分支，没有退出分支；Escape 键在自动补全未激活时被完全忽略；Editor.OnCancel 虽注册了退出回调但从未在任何代码路径被触发
- **影响范围**: `tui/chat/chat_app.go`
- **风险等级**: 中（TUI 退出行为变更，直接影响用户交互流程）
- **审查要求**: L1
- **验证**: `make all` ✅ | `go test -race ./tui/...` ✅ | 已重新编译并安装到 `/usr/local/bin/mady`

## 2026-07-13: 引入 LLM Rubric Judge 与语义相似度指标，替换纯 token 重叠评估

- **变更**:
  1. 新增 `agentcore/evaluate/llm_judge.go`，实现 `LLMJudge` 和 `SemanticSimilarity` 两种指标：
     - `LLMJudge` 使用 LLM 按 rubric 三个维度（conclusion / reasoning / citation）打分，输出结构化 JSON 并取平均，避免纯 token 重叠对长篇主观实务题的严苛误判
     - `SemanticSimilarity` 使用 LLM 判断预测答案与参考答案在语义上是否等价，忽略表达方式和篇幅差异
  2. 新增 `agentcore/evaluate/llm_judge_test.go`，覆盖 JSON rubric、markdown 代码块、百分比、分数等解析场景
  3. 更新 `agentcore/evaluate/benchmark/suite.go`，新增 `LiveEvaluator(judge, model)` 函数，使用 `CitationCompleteness` + `LLMJudge` 作为 live evaluation 的默认指标组合；保留 `DefaultEvaluator()` 用于静态 GoldenPerfect CI 门控
  5. 修复 review 反馈：修正 `truncateForJudge` 头部按字节截断导致中文文本 UTF-8 损坏的 bug；更新 `MaxTokens` 注释与默认值一致；用 `rand.New(rand.NewSource(seed))` 替换已弃用的 `rand.Seed`；`gofmt` 格式化
- **原因**: 用户要求将纯 token 重叠指标（F1 / KeywordRecall / JudgeConsistency）改为基于 LLM 评判的 rubric 评分或语义相似度指标；原指标在长篇主观实务题上严重失真（F1 precision 低、KeywordRecall 受措辞差异影响、JudgeConsistency 二值门控过严），而 LLM 能从法律结论、推理过程和法条引用维度更准确地评估答案质量
- **影响范围**: `agentcore/evaluate/llm_judge.go`（新）、`agentcore/evaluate/llm_judge_test.go`（新）、`agentcore/evaluate/benchmark/suite.go`、`agentcore/evaluate/benchmark/live_deepseek_test.go`
- **风险等级**: 低（新增指标和 evaluator 可选使用；不影响 GoldenPerfect CI 门控；live test 仍受 `MADY_LIVE_EVAL=1` 控制）
- **审查要求**: L1
- **验证**: `make all` ✅ | `go test -race ./agentcore/evaluate/...` ✅ | `MADY_LIVE_EVAL=1 go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...` ✅（随机 3 题，DeepSeek 2/3 通过，LLM judge 聚合平均 0.533，citation_completeness 1.000）



## 2026-07-13: 引入 DeepSeek 真实模型评估测试

- **变更**:
  1. 新增 `agentcore/evaluate/benchmark/live_deepseek_test.go`，在 `DEEPSEEK_API_KEY` 环境变量存在时，可随机抽取 3 道真实专利考试真题调用 DeepSeek API 进行 live evaluation
  2. 系统提示词采用项目五步工作法（① 收集事实 → ② 检索规则 → ③ 制定计划 → ④ 执行推理 → ⑤ 校验结论），引导模型按结构化流程作答
  3. 新增 `PatentExamRealCases()` 辅助函数，聚合全部 31 道按法条归类的真实专利考试真题 case
  4. 支持 `/tmp/mady_deepseek_eval.json` 缓存，中断后可重新运行继续完成剩余 case
- **原因**: 用户要求用真实模型和本项目五步工作法验证黄金测试集；静态 `TestEvalSuite_GoldenPerfect` 只能验证 metrics 链和门控逻辑，live evaluation 才能检验真实 LLM 在长篇专利实务题上的表现
- **影响范围**: `agentcore/evaluate/benchmark/live_deepseek_test.go`（新）
- **风险等级**: 低（仅在显式运行 `-run TestLiveDeepSeekEval` 且 API key 存在时执行；正常 CI 中自动跳过，不影响现有门禁）
- **审查要求**: L1
- **验证**: `go test -v -timeout 30m -run TestLiveDeepSeekEval ./agentcore/evaluate/benchmark/...` ✅（随机 3 题，DeepSeek-V3 0/3 通过当前严格门控，平均得分 0.091 / 0.351 / 0.335，F1 与 keyword_recall 仍偏低，说明严格 token 重叠指标对长篇主观实务题非常严苛）

## 2026-07-13: 黄金测试集扩展 — 2007-2019 年专代实务真题按专利法条款归类

- **变更**:
  1. 借鉴 XiaoNuo Agent 项目已整理的 31 个 2007-2019 年专利代理人资格考试《专利代理实务》真题 case，将其转化为 Mady `evaluate.TestCase` 格式，按专利法/实施细则核心条款归类为 6 组：
     - `PatentExamRealA2Cases`：专利法第二条（保护客体）相关 3 case（2012、2018、含实用新型保护客体的题目）
     - `PatentExamRealA22Cases`：专利法第二十二条（新颖性/创造性/实用性）相关 15 case
     - `PatentExamRealA26Cases`：专利法第二十六条（充分公开/支持/清楚）相关 3 case
     - `PatentExamRealA31Cases`：专利法第三十一条（单一性/合案/分案）相关 8 case
     - `PatentExamRealA33Cases`：专利法第三十三条（修改不得超范围）相关 1 case
     - `PatentExamRealR42Cases`：专利法实施细则第四十二条（分案申请程序）相关 1 case
  2. 新增 `agentcore/evaluate/benchmark/patent_exam_real_a2.go`、`a22.go`、`a26.go`、`a31.go`、`a33.go`、`r42.go` 6 个文件，ID 统一为 `patent_exam_<年份>_<条款>_<序号>`，便于按法条筛选和统计
  3. 删除旧的 `agentcore/evaluate/benchmark/patent_exam_real_2007.go`，2007 年 case 已按所属法条分散到上述 6 组中
  4. 更新 `agentcore/evaluate/benchmark/suite.go` 的 `AllCases()`，注册上述 6 个新变量
  5. 新增临时转换脚本 `convert_xiaonuo.py`（未入仓，位于 `/var/folders/.../exam_papers_text/`），用于将 XiaoNuo Agent JSON case 格式批量转为 Go 结构体，并自动校验 `RequiredCitations` 必须出现在 `Expected` 中
- **原因**: 用户要求将全部可用年份真题加入黄金测试集，并按专利法条款归类；XiaoNuo Agent 项目已人工整理并审核 2007-2019 年共 31 个真题 case，直接复用可避免重复 OCR 和答案整理，快速提升 benchmark 覆盖度
- **影响范围**: `agentcore/evaluate/benchmark/`（新增 6 文件、删除 1 文件、修改 suite.go）
- **风险等级**: 低（仅测试数据集变更，不改变评估逻辑；已通过 GoldenPerfect 门控）
- **审查要求**: L1
- **验证**: `go test ./agentcore/evaluate/benchmark/...` ✅ | `go test -race ./agentcore/evaluate/benchmark/...` ✅ | `go vet ./agentcore/evaluate/benchmark/...` ✅ | `gofmt -w agentcore/evaluate/benchmark/` ✅

## 2026-07-13: 引入 2007 年专代实务真题作为黄金测试集

- **变更**:
  1. 新增 `agentcore/evaluate/benchmark/patent_exam_real_2007.go`，从 2007 年全国专利代理人资格考试《专利代理实务》卷三真题及官方参考答案中抽取 4 道子任务，转换为 `evaluate.TestCase`：
     - 无效实务题：修改后的独立权利要求 1（`patent_exam_2007_1b`）
     - 无效实务题：无效期间专利文件修改的有关规定（`patent_exam_2007_1c`）
     - 撰写实务题：发明专利申请的独立权利要求 1（`patent_exam_2007_2a`）
     - 撰写实务题：独立权利要求合案申请理由（`patent_exam_2007_2b`）
  2. 在 `agentcore/evaluate/benchmark/suite.go` 的 `AllCases()` 中注册 `PatentExamReal2007Cases`
- **原因**: 现有 `PatentExamCases` 为模拟题，注释已注明待真题可用性确认后替换；2007 年真题及参考答案已本地可用，可作为权威、可复现的 Agent 评测基准
- **影响范围**: `agentcore/evaluate/benchmark/patent_exam_real_2007.go`（新）、`agentcore/evaluate/benchmark/suite.go`
- **风险等级**: 低（仅新增测试数据，不改变现有评估逻辑；已通过 `TestEvalSuite_GoldenPerfect` 等全部门控）
- **审查要求**: L1
- **验证**: `go test ./agentcore/evaluate/benchmark/...` ✅

## 2026-07-13: TUI 阶段 1-4 代码质量审查与修复

- **变更**:
  1. 修复 `tui/tui.go` cell 级 diff 渲染路径对 Raw 行的遗漏：当 `RowCellDiff.RawContent` 非空时，原循环只处理 `Segments`，导致 Raw 行变化时终端收不到任何输出；现改为先 `[0m` 重置 SGR、再整行重写 Raw 内容
  2. 修复 `tui/chat/chat_history.go` 中 `AppendDeltaWithKind` 新消息 ID 生成顺序：原代码先用 `h.msgIDSeq+1` 构造 ID 再递增，可能导致两个紧接调用在 `time.Now().UnixNano()` 相同时得到相同 ID；现改为先递增 `msgIDSeq` 再构造 ID，与 `Append` 保持一致
  3. 新增 `tui/celldiff_integration_test.go` 中 `TestRenderFrameCellDiffRawRow`，验证 Raw 行变化时 `TUI.renderFrame` 会输出新内容
  4. 新增 `tui/chat/chat_history_test.go` 中 `TestChatHistoryAppendDeltaGeneratesUniqueIDs`，验证连续 `AppendDelta("", ...)` 生成唯一 ID
- **原因**: 阶段 1-4 已完整落地，进入整体 review 时发现两处可触发实际缺陷的断链（Raw 行 diff 不渲染、极端情况下新消息 ID 冲突），需在进入后续阶段前补齐
- **影响范围**: `tui/tui.go`、`tui/chat/chat_history.go`、`tui/celldiff_integration_test.go`、`tui/chat/chat_history_test.go`
- **风险等级**: 中（修复点均位于 TUI 核心路径，但已新增测试覆盖并跑通全量 `-race`）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` ✅（根模块与 `tools/` 子模块）| `golangci-lint` 未安装

## 2026-07-13: TUI 通用 Viewport 组件

- **变更**:
  1. 新增 `tui/component/viewport.go` 的 `Viewport` 组件：基于 `[]string` 内容缓冲提供可滚动视口，支持 `MaxRows` 可见高度、tail-follow 模式、向上/向下滚动、绝对偏移滚动、`^ N more lines` 指示器（可自定义渲染函数）和宽度补齐
  2. Viewport 内部采用与 `ChatHistory` 一致的偏移语义：offset 为从底部向上滚动的行数，0 表示显示尾部；`ScrollBy` 正数向上、负数向下；`ScrollTo` 为绝对偏移；`FollowTail` 回到底部并重新启用自动跟随
  3. 提供线程安全的 getter/setter（`SetContent`/`SetMaxRows`/`SetIndicator`/`SetIndicatorFn`），渲染时不持有锁，且 `Invalidate` 为无操作（不保留派生缓存）
  4. 新增 `tui/component/viewport_test.go` 覆盖：无裁剪渲染、尾部裁剪、向上/向下滚动、偏移 clamp、`FollowTail`、指示器、动态调整 `MaxRows`、宽度补齐、自定义 indicator 函数、追加内容后自动跟随尾部
- **原因**: 阶段 1-3 已分别完成 ChatHistory 增量缓存、声明式布局层和 cell 级 diff；阶段 4 提取一个通用的 `Viewport` 容器，使日志、列表、帮助文本等长内容场景无需重复实现滚动/裁剪逻辑，为未来替换 `ChatHistory` 内嵌视口或构建多面板布局做准备。考虑到 `ChatHistory` 已有自洽的缓存+视口逻辑和大量选区/鼠标坐标依赖，本次不直接替换，避免一次性改动过大
- **影响范围**: `tui/component/viewport.go`（新）、`tui/component/viewport_test.go`（新）
- **风险等级**: 低（新组件独立，不替换现有路径；已通过全量 `-race` 测试）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` ✅（根模块与 `tools/` 子模块）| `golangci-lint` 未安装

## 2026-07-13: TUI cell 级 diff

- **变更**:
  1. 新增 `tui/core` 的 cell 级 diff：在 `RowDiff` 行级 diff 之上增加 `Segment`/`RowCellDiff` 与 `DiffCells`/`DiffFrame`，将每行中第一个不同列到最后一个不同列识别为最小重写段，跨行宽度变化时自动追加尾部清除（`ClearTail` + `TailStart`）
  2. 新增 `SerializeRowSegment`：在段起始处重置 SGR、按 cell 样式优化输出，并在段末将终端 SGR 状态过渡回未变更后缀的样式，避免样式泄漏到未重写区域
  3. `tui/tui.go` 的差分渲染路径从“整行擦除+重写”改为“按段移动光标+重写+按需清除尾部”，CSI 2026 同步输出、光标管理和 DECAWM 控制保持不变
  4. 宽字符边界保护：diff 段边界若落在 continuation cell，自动扩展到 primary cell，避免只重写宽字符右半
  5. 新增 `tui/core/celldiff_test.go` 覆盖无变化、单 cell 变化、前缀/后缀变化、新行缩短、宽字符边界、raw 行回退等场景；新增 `tui/celldiff_integration_test.go` 通过 `VirtualTerminal` 验证 `TUI.renderFrame` 实际只输出变化段
- **原因**: 阶段 1 ChatHistory 增量缓存减少组件层渲染，阶段 2 声明式布局减少布局计算，阶段 3 cell 级 diff 进一步降低终端输出带宽；对于流式 token、光标闪烁、spinner 等场景，行内大部分 cell 不变，重写整行浪费明显
- **影响范围**: `tui/core/celldiff.go`、`tui/core/cellrender.go`、`tui/tui.go`、`tui/core/celldiff_test.go`（新）、`tui/celldiff_integration_test.go`（新）
- **风险等级**: 中（渲染路径核心变更，SGR 状态管理与光标移动需严格正确；已通过 `VirtualTerminal` 集成测试和全量 `-race` 测试）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` ✅（根模块与 `tools/` 子模块）| `golangci-lint` 未安装

## 2026-07-13: TUI 声明式布局层（Flex）

- **变更**:
  1. 新增 `tui/layout` 包：`Flex` 容器、`Child` 配置、`Direction`/`SizePolicy` 枚举、辅助构造器（`Natural`/`Fixed`/`Fill`/`FillWeight`/`Percent`/`Min`/`Max`），以及 `BoundsProvider` 接口
  2. 在 `tui/core` 中新增可选 `Sizer` 接口，让组件在不生成完整渲染输出的情况下声明自然高度，避免布局测量时重复渲染
  3. `Flex` 支持 `DirectionVertical`（chatLayout 主场景）和 `DirectionHorizontal`（基础实现），支持 `SizeNatural`/`Fixed`/`Min`/`Max`/`Fill`/`Percent` 策略；`Fill` 子项可通过 `OnAllocate` 回调在分配空间后同步设置自身 `MaxRows`
  4. 改造 `chatLayout.Render`：从手工计算 header/history/ac/loader/editor/footer/statusBar 行数改为 `layout.NewFlex(DirectionVertical)` 声明式组装；新增 `editorFrame` 包装组件统一处理 editor 上下边框；保留 `editorTop`/`headerHeight` 计算以兼容 `MouseMsg` 坐标转换
  5. 新增 `tui/layout/flex_test.go` 与 `tui/chat/chat_app_test.go` 中 `TestChatLayoutUsesFlex`/`TestChatLayoutEditorTopAfterResize`，覆盖自然堆叠、Fill 分配、矩形查询、resize 后坐标更新
- **原因**: 原 `chatLayout.Render` 手工累加各组件行数并计算剩余空间，逻辑硬编码、难以扩展；引入声明式布局层后可复用到 future Viewport/面板/弹窗等场景，并为后续 cell 级 diff 和 Viewport 抽象提供统一的布局语义
- **影响范围**: `tui/layout/layout.go`（新）、`tui/layout/flex.go`（新）、`tui/layout/flex_test.go`（新）、`tui/core/component.go`、`tui/chat/chat_app.go`、`tui/chat/chat_app_test.go`
- **风险等级**: 中（`chatLayout` 是 TUI 主渲染路径，mouse/选区/复制坐标依赖 `editorTop`；已添加测试验证，但水平方向为简化实现，未覆盖复杂场景）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` ✅（根模块与 `tools/` 子模块）| `golangci-lint` 未安装

## 2026-07-13: 修复 knowledge/fileindex/store.go 语法错误

- **变更**: 补全 `store.go` 中缺失的 `import` 块闭合 `)` 与 `const` 块闭合 `)`，恢复该包可编译
- **原因**: 该文件存在语法错误导致 `go build ./...` 失败，阻塞 TUI 阶段 2 验证；修复后不影响任何业务逻辑
- **影响范围**: `knowledge/fileindex/store.go`
- **风险等级**: 低（纯语法修复，无行为变更）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go test -race ./knowledge/fileindex` ✅

## 2026-07-13: ChatHistory 增量渲染缓存

- **变更**:
  1. 在 `ChatHistory` 中引入按消息 ID 缓存的 `msgCache`（`map[string]cachedMessage`），缓存每个消息在固定宽度下渲染出的行；width 变化、主题变化、reasoning 渲染器变化或清空历史时整表失效
  2. `Append` 只让新增消息未缓存，后续 `Render` 只渲染新消息；`PatchMessage`/`AppendDelta`/`Finalize` 仅失效对应消息缓存；`tryToggleThinkingAtLineLocked` 切换单个消息/思考段时只失效该消息缓存；工具组折叠/展开时清空整表（影响多消息）
  3. 修正 `Append`/`AppendDelta` 自动生成消息 ID：原实现仅用 `time.Now().UnixNano()`，在紧密连续调用时可能产生重复 ID，导致缓存冲突；新增 `msgIDSeq` 单调序列号，ID 格式改为 `msg-{nanosec}-{seq}`
  4. 新增 `renderCount` 测试计数器与 `TestChatHistoryIncrementalCache`/`TestChatHistoryCacheProducesIdenticalOutput` 两个测试，验证增量缓存只渲染变化消息且输出不变
- **原因**: 原 `ChatHistory.Append` 直接将整个 `cachedAll` 标记为 dirty，长对话或流式输出时每帧都重新渲染所有历史消息，CPU 开销随消息数线性增长；增量缓存将常见操作（追加、流式 delta、单消息 patch）的渲染复杂度从 O(n) 降到 O(1)
- **影响范围**: `tui/chat/chat_history.go`、`tui/chat/chat_history_test.go`
- **风险等级**: 中（涉及消息缓存失效与消息 ID 生成变更；选择区、工具组折叠、reasoning 显示状态均已在失效路径中处理，但新增缓存状态可能引入遗漏失效场景）
- **审查要求**: L2

## 2026-07-13: TUI 复制功能修复 — Kitty flag 8 + 右键复制

- **变更**:
  1. **Kitty flag 8 开启**：`NewProcessTerminal()` 默认 `kittyFlags` 从 `1`（仅 disambiguate）改为 `1 | 8`（report all keys），使得 `Cmd+C` 作为 CSI u 序列 `\x1b[99;9u`（ModSuper）到达，可区分于 `Ctrl+C`（ModCtrl）；`main.go` TUI 入口同步显式设置 `KittyKeyboardFlags: 1 | 8`
  2. **右键复制**：`chatLayout.Update()` 中 `MouseRelease + Button==2` 分支触发 `doCopy(l)`，复用现有选区/剪贴板基础设施
  3. **Alacritty 支持**：`TerminalSupportsKittyKeyboard()` 新增 `ALACRITTY_WINDOW_ID` 和 `TERM=alacritty` 检测（Alacritty 0.13.0+ 支持 Kitty 协议）
- **原因**: (1) 无法使用 ⌘+C 复制 — Kitty flag 1 不足以区分 Cmd 和 Ctrl 修饰键；(2) 鼠标右键无法复制 — SGR 鼠标已正确解码 Button 2，但 layout 层未响应
- **影响范围**: `cmd/mady/main.go`、`tui/chat/chat_app.go`、`tui/terminal/terminal.go`
- **风险等级**: 低（flag 8 对不支持 Kitty 协议的终端安全忽略；右键事件与左键互斥；21 项测试全部通过）
- **审查要求**: L1

## 2026-07-13: Sandbox 全面修复与 Cwd 感知（ProjectDir 字段）

- **变更**:
  1. **Sandbox 默认值修复**：`ExtensionConfig.SandboxEnabled` 注释修正为"Default is false; domain factories must set true explicitly"（原注释声称默认 true 但 Go bool 零值=false，注释与代码矛盾）
  2. **只读工具沙箱注入**：ls/grep/find/glob/view 的 ToolConfig 新增 `Sandbox WorkingDirSandbox` 字段，BuildTools 统一注入；Func 内改用 `resolvePathSandboxed` 替代 `resolveReadPath`，启用沙箱时拒绝逃逸路径
  3. **Bash 工具沙箱字段**：BashToolConfig 新增 `Sandbox` 字段并经 BuildTools 注入（bash 本质无法做命令级沙箱，但配置一致性已保证）
  4. **Cwd 感知**：`agentcore.Config` 新增 `ProjectDir string` 字段（语义：用户当前项目文件夹 = os.Getwd()），与 `WorkspaceDir`（应用数据目录 = ~/.mady/workspace）分离。`setupFrameworkContext` 获取 cwd 注入 `BaseConfig.ProjectDir`；`applyPersistence` 案件模式覆盖为 `RootPath`
  5. **领域工厂适配**：`AssistantAgentConfig` WorkingDir 改用 `base.ProjectDir`（回退 WorkspaceDir），显式 `SandboxEnabled=true`；`PatentAgentConfig` 补充 tools extension（此前完全没有文件工具），WorkingDir 用 ProjectDir，`SandboxEnabled=true`；`BuildProjectAgent` 设置 `cfg.ProjectDir = rec.RootPath`
- **原因**: (1) 默认 Agent 不感知 cwd，工具 WorkingDir 指向 ~/.mady/workspace 而非用户项目目录；(2) SandboxEnabled 默认 false 导致沙箱形同虚设，read/write/edit 只打 warning 就放行；(3) 只读工具（ls/grep/find/glob/view）完全无沙箱字段，绝对路径可绕过 cwd 限制；(4) PatentAgentConfig 没有文件工具
- **影响范围**: agentcore/agent.go, tools/tools.go, tools/bash.go, tools/ls.go, tools/grep.go, tools/find.go, tools/glob.go, tools/view.go, domains/assistant.go, domains/patent.go, cmd/mady/main.go
- **风险等级**: 中（涉及 tools/path.go 安全边界路径，但未修改 resolvePathSandboxed 本身；SandboxEnabled 从 false→true 改变了默认行为，可能影响未显式设置的调用方）
- **审查要求**: L3

## 2026-07-13: 评估闭环与记忆自学习方案评审报告

- **变更**: 新建决策文档 `docs/decisions/eval-memory-plan-review-2026-07-13.md`（276行），对《评估闭环模块》《记忆自学习模块》《整体阶段划分》三部分方案进行代码级评审。结论：理念合格，落地需大改。识别 4 处重大脱节（向量检索已完成、评估框架已存在、memory 缺持久化、Checkpoint 概念混淆），提炼 7 项真正有价值的缺失工作，给出修正后的 A→B→C→D 四阶段落地路线（含 A5/A6 持久化基础设施前置）
- **原因**: 方案基于过时项目快照，照原样执行将产生两套并行系统（EvalCase vs TestCase、LawyerPreference vs MemoryEntry），且遗漏 memory/StageCheckpoint 缺持久化的关键风险
- **影响范围**: docs/decisions/eval-memory-plan-review-2026-07-13.md(新)；后续阶段 A-D 的实现将涉及 agentcore/evaluate、memory、domains/reasoning、domains/approval、guardrails、workflows/patent 等包
- **风险等级**: 低（仅文档变更，无代码改动）
- **审查要求**: L2

## 2026-07-13: 阶段 A5 — MemoryStore SQLite 持久化后端

- **变更**:
  1. **新建 `memory/sqlite_store.go`**(~380行)：`SQLiteMemoryStore` 类型，实现 `MemoryStore` 接口，数据持久化到 SQLite（WAL 模式）。Schema 含 memories 表（15 列）+ 2 个索引（layer/scope）。检索策略与 `InMemoryStore` 一致：关键词匹配 + 复合评分（语义+新鲜度+重要性），复用 `keywordScore`/`recencyScore`/`estimateImportance` 等包级函数。Embedding 以 BLOB 存储（little-endian float32），供未来向量检索升级。Metadata 以 JSON 序列化。支持 `SQLiteOption` 函数式配置（`WithSQLiteScoringConfig`/`WithSQLiteClock`）
  2. **新建 `memory/sqlite_store_test.go`**：15 个测试覆盖 Remember/Get/RememberBatch/Update/Forget/ForgetAll/Recall/RecallWithBudget/List/Prune/Stats/Persistence（关闭重开数据不丢失）/Concurrency（20 goroutine 并发写 + 10 并发读）/EmbeddingRoundTrip/空内容拒绝
- **原因**: `InMemoryStore`（memory/store.go:14）是 Phase 1 纯内存实现，重启后数据丢失。方案 Tier 2 用户偏好需要跨重启持久化，Tier 1 案件记忆预热也依赖持久化后端。此改动为 B2/C2 的前置基础设施
- **影响范围**: memory/sqlite_store.go(新), memory/sqlite_store_test.go(新)
- **风险等级**: 低（新建文件，不修改任何现有代码；`MemoryStore` 接口和 `InMemoryStore` 保持不变；`Manager` 通过接口注入，无需改动）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ memory 包全绿（含 15 个新测试 + 原有测试）

## 2026-07-13: 阶段 A6 — CheckpointStore SQLite 持久化后端

- **变更**:
  1. **新建 `domains/reasoning/sqlite/checkpoint_store.go`**(~150行)：`SQLiteCheckpointStore` 类型，实现 `reasoning.CheckpointStore` 接口（Save/Load/Delete）。Schema 含 stage_checkpoints 表（checkpoint_id PK + case_id/case_type/current_stage 索引列 + data JSON 列）+ case_id 索引。复用 `reasoning.MarshalCheckpoint`/`UnmarshalCheckpoint` 做 JSON 序列化。额外提供 `ListByCase` 方法按案件查询所有检查点。子包设计遵循依赖倒置：domain 层不导入 `database/sql`
  2. **新建 `domains/reasoning/sqlite/checkpoint_store_test.go`**：6 个测试覆盖 Save+Load/LoadNotFound/Delete/SaveReplace（同 ID 覆盖）/ListByCase/Persistence（关闭重开数据不丢失）
- **原因**: `MemoryCheckpointStore`（domains/reasoning/checkpoint.go:36）只有内存实现，重启后丢失。方案 Tier 1 案件记忆预热（B2）依赖 `ResumeFromCheckpoint` 从持久化 `CheckpointStore` 恢复。此改动为 B2 的前置基础设施
- **影响范围**: domains/reasoning/sqlite/checkpoint_store.go(新), domains/reasoning/sqlite/checkpoint_store_test.go(新)
- **风险等级**: 低（新建子包，不修改任何现有代码；`CheckpointStore` 接口和 `MemoryCheckpointStore` 保持不变）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 6 个测试全绿

## 2026-07-13: 阶段 A2 — Golden Benchmark 第一层（专利代理人考试模拟题）

- **变更**:
  1. **修改 `agentcore/evaluate/evaluator.go`**：`TestCase` 结构体新增 `Domain string` 字段，用于按领域（专利/法律/通用）筛选用例
  2. **新建 `agentcore/evaluate/benchmark/patent_exam.go`**：10 道模拟专利代理人考试题，覆盖新颖性判断(001)、创造性分析(002)、权利要求保护范围(003)、OA答复(004)、等同侵权(005)、无效宣告(006)、可专利性客体(007)、先用权(008)、从属权利要求审查(009)、上位概念侵权(010)。每题含 ID/Domain/Input/Expected/RequiredCitations。提供 `CaseCount()` 和 `CasesByDomain(domain)` 辅助函数
- **原因**: 评估闭环需要 Golden Benchmark 作为回归基准。方案第一层用考试真题，但版权风险高，先以 MVP 质量模拟题建仓（10 题），后续扩展至 50-100 题并经领域专家审核
- **影响范围**: agentcore/evaluate/evaluator.go(修改), agentcore/evaluate/benchmark/patent_exam.go(新)
- **风险等级**: 低（仅新增数据集 + 非破坏性字段扩展）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ evaluate 包全绿

## 2026-07-13: 阶段 A3 — 评估指标设计（Judge 一致性 + 护栏漏报率 + 人工采纳率）

- **变更**:
  1. **新建 `agentcore/evaluate/judge_metrics.go`**：
     - `JudgeConsistency` Metric：实现 `Metric` 接口，包装可选 `JudgeFunc`（Phase 3 启发式/Phase 4+ LLM 裁判）。无 JudgeFunc 时用 `keywordOverlap` 启发式（ExtractKeywords 提取关键词，≥60% 重叠判为一致）
     - `GuardrailFalseNegativeRate` 聚合指标：跨用例统计 TotalHighRisk/FlaggedHighRisk，`Rate()` 返回漏报率，`Score()` 返回 1-Rate
     - `AdoptionRate` 聚合指标：统计 Adopted/Modified/Rejected，`FullyAdopted()`/`Accepted()`/`RejectedRate()` 方法
     - 后两者是跨用例聚合指标，不实现 `Metric` 接口（单用例评分）
  2. **新建 `agentcore/evaluate/judge_metrics_test.go`**：TestJudgeConsistency_Heuristic（3 子测试：high_overlap/low_overlap/empty_reference）/ TestJudgeConsistency_CustomJudge / TestJudgeConsistency_Name / TestGuardrailFalseNegativeRate / TestAdoptionRate
- **原因**: 现有 `Metric` 实现只有 ExactMatch/F1Score/KeywordRecall/CitationCompleteness/LengthScore，缺少方案要求的 Judge 一致性、护栏漏报率、人工采纳率三项关键指标
- **影响范围**: agentcore/evaluate/judge_metrics.go(新), agentcore/evaluate/judge_metrics_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ evaluate 包全绿（含 5 个新测试）

## 2026-07-13: 阶段 A4 — CI 化评估门禁

- **变更**:
  1. **新建 `agentcore/evaluate/benchmark/suite.go`**：提供 `DefaultEvaluator()`（F1Score + KeywordRecall + CitationCompleteness + JudgeConsistency 四指标，阈值 0.7）、`AllCases()`（聚合所有 benchmark 用例）、`CasesByDomain(domain)`（按领域过滤）、`RunSuite(ctx, runFunc)`（活跃评估入口）、`RunStatic(predictions)`（CI 静态评估入口）
  2. **新建 `agentcore/evaluate/benchmark/suite_test.go`**：4 个 CI 门禁测试 — `TestEvalSuite_GoldenPerfect`（完美预测 PassRate=1.0，验证 Metric 链路完整性）、`TestEvalSuite_Degraded`（空预测 PassRate=0，负向控制）、`TestEvalSuite_CaseIntegrity`（用例格式校验：ID/Input/Expected/Domain 非空 + ID 唯一）、`TestEvalSuite_DefaultEvaluator`（Evaluator 配置校验）
  3. **修改 `agentcore/evaluate/benchmark/patent_exam.go`**：删除 `CaseCount()` 和 `CasesByDomain()`，统一到 `suite.go` 中基于 `AllCases()` 的实现（未来新增领域自动包含）
  4. **修改 `Makefile`**：新增 `eval` 和 `eval-race` target，运行 benchmark CI 门禁测试
- **原因**: 评估闭环需要 CI 门禁。Prompt/Rule/Skill 变更时，`make eval` 验证 Metric 链路完整性、用例格式正确性、完美/降级预测的通过/失败行为。静态评估模式（`EvaluateStatic`）无需 LLM API，CI 友好
- **影响范围**: agentcore/evaluate/benchmark/suite.go(新), suite_test.go(新), patent_exam.go(修改), Makefile(修改)
- **风险等级**: 低（新建文件 + 非破坏性重构；`CaseCount`/`CasesByDomain` 语义不变，只是移到 suite.go 并改为基于 AllCases()）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ evaluate + benchmark 全绿 | `make eval` ✅

## 2026-07-13: 阶段 B1 — ApprovalGate 结构化留痕机制

- **变更**:
  1. **修改 `domains/approval.go`**：
     - 新增 `ApprovalDecision` 类型（`DecisionAdopted`/`DecisionModified`/`DecisionRejected`）
     - 新增 `ApprovalRecord` 结构体（ID/SessionID/CaseID/Timestamp/TriggerKeyword/OriginalOutput/Decision/ModifiedOutput/Feedback），记录单次审批的完整信息
     - 新增 `ApprovalStore` 接口（Save/List/ListByCase），供 TUI `/review` 和评估闭环消费
     - 新增 `MemoryApprovalStore` 内存实现（sync.Mutex + slice）
     - `ApprovalGate` 新增 `store ApprovalStore` 字段
     - 新增 `WithApprovalStore(store)` 函数式配置选项
     - 新增 `RecordDecision()` 方法 — 供 TUI /review handler 在用户做出决策后调用，自动创建并持久化 ApprovalRecord。无 store 时为静默 no-op
  2. **修改 `domains/approval_test.go`**：新增 5 个测试 — `TestMemoryApprovalStore_SaveAndList`（多 session/case 交叉保存+查询）、`TestMemoryApprovalStore_Empty`（空查询）、`TestApprovalGate_RecordDecision`（完整决策记录+字段校验）、`TestApprovalGate_RecordDecision_NoStore`（无 store 时 no-op）、`TestApprovalGate_WithApprovalStore`（store 注入校验）
- **原因**: ApprovalGate.AfterModelCall 仅注入 steering message，无结构化留痕。审批记录是 AdoptionRate 指标（A3）和第二层 Golden Benchmark 转化（C1）的数据来源，缺此则评估闭环和回归用例转化均无数据
- **影响范围**: domains/approval.go(修改), domains/approval_test.go(修改)
- **风险等级**: 低（非破坏性扩展；ApprovalGate 现有行为不变，store 为可选注入）
- **审查要求**: L2（涉及 `domains/approval.go` 安全敏感路径，但仅新增不改已有逻辑）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ domains 包全绿（含 5 个新测试 + 原有 7 个测试）

## 2026-07-13: 阶段 B2 — Tier 1 案件记忆预热

- **变更**:
  1. **新建 `domains/reasoning/case_summary.go`**：`CaseSummary` 结构体（CaseID/CaseType/TechnicalField/CurrentStage/FactCount/WorkflowID/CreatedAt/UpdatedAt）+ `ExtractCaseSummary(cp *StageCheckpoint) CaseSummary` 函数（从 StageCheckpoint + FactBlackboard 提取关键信息）+ `String()` 方法（格式化为关键词密集的可读文本，便于记忆检索）
  2. **新建 `domains/reasoning/case_summary_test.go`**：3 个测试 — `TestExtractCaseSummary_WithBlackboard`（含 blackboard 的完整提取+字段校验）、`TestExtractCaseSummary_NilBlackboard`（nil blackboard 降级处理）、`TestCaseSummary_String`（文本格式校验）
  3. **新建 `memory/preheat.go`**：`PreheatCaseMemory(ctx, store, scope, caseID, summary)` 函数 — 将案件摘要作为高重要性 LongTerm 层 MemoryEntry 存入，metadata 含 `type=case_preheat` + `case_id`。memory 包不依赖 domains/reasoning（依赖倒置），由调用者负责生成 summary 字符串
  4. **新建 `memory/preheat_test.go`**：3 个测试 — `TestPreheatCaseMemory`（存储+字段校验+metadata 校验）、`TestPreheatCaseMemory_EmptySummary`（空摘要拒绝）、`TestPreheatCaseMemory_Recallable`（存储后可通过 Recall 检索到）
- **原因**: 案件恢复时 Agent 需要"记住"之前的案件上下文（CaseID/类型/技术领域/阶段/事实）。B2 从持久化的 StageCheckpoint 提取摘要并存入 MemoryStore，使 Agent 在新会话中能通过记忆检索恢复上下文。依赖 A5（SQLiteMemoryStore）和 A6（SQLiteCheckpointStore）作为前置基础设施
- **影响范围**: domains/reasoning/case_summary.go(新), case_summary_test.go(新), memory/preheat.go(新), preheat_test.go(新)
- **风险等级**: 低（新建文件，不修改任何现有代码；memory 包不新增对 domains 的依赖）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ reasoning + memory 包全绿（6 个新测试）

## 2026-07-13: 阶段 B3 — LLM 裁判 Metric + 抽样人工校准

- **变更**:
  1. **新建 `agentcore/evaluate/llm_judge.go`**：
     - `LLMJudgeCaller` 接口（`JudgeConsistency(prediction, reference) (bool, error)`）— 最小 LLM 调用抽象，不耦合 agentcore.Provider
     - `NewLLMJudgeFunc(caller LLMJudgeCaller) JudgeFunc` — 包装为 JudgeConsistency 使用的 JudgeFunc，LLM 错误时保守降级为 disagree(false)
     - `CalibrationSample` 结构体（CaseID/Prediction/Reference/Score/Reason）
     - `CollectCalibrationSamples(report, predictions, cases, rate, threshold)` — 三优先级抽样：failed（全部）→ borderline（阈值±0.1 全部）→ passing（按 rate 随机）。结果按分数升序排列，优先低分
  2. **新建 `agentcore/evaluate/llm_judge_test.go`**：8 个测试 — NewLLMJudgeFunc Agree/Disagree/Error、LLMJudgeConsistency+Caller、CollectCalibrationSamples FailedCase/Borderline/NilReport/ZeroRate
- **原因**: A3 的 JudgeConsistency 只有启发式 keywordOverlap，Phase 4+ 需要 LLM 裁判能力。抽样人工校准用于持续校准 LLM 裁判的准确性（false negative/false positive 检测）。LLMJudgeCaller 接口解耦 evaluate 与 agentcore，由调用者适配 Provider
- **影响范围**: agentcore/evaluate/llm_judge.go(新), llm_judge_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码；LLM 错误时保守降级，不会产生误通过）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ evaluate 包全绿（8 个新测试 + 原有测试）

## 2026-07-13: 阶段 C1 — Golden Benchmark 第二层回归用例转化

- **变更**:
  1. **新建 `domains/regression.go`**：`ApprovalToTestCase(record ApprovalRecord, domain string) evaluate.TestCase` — 将 DecisionModified 的审批记录转化为 TestCase（OriginalOutput→Input, ModifiedOutput→Expected）+ `ApprovalToRegressionCandidates(records, domain) []evaluate.TestCase` — 批量过滤+转化，跳过非 Modified 和空 ModifiedOutput 的记录
  2. **新建 `domains/regression_test.go`**：3 个测试 — 单条转化+字段校验、批量过滤（5 条记录→2 条候选）、空输入
- **原因**: B1 的 ApprovalGate 留痕中，DecisionModified 记录隐含人工质量标准。C1 半自动将这些记录转化为回归用例，构建 Golden Benchmark 第二层（脱敏真实案例）。人工仍需审核后才加入正式数据集
- **影响范围**: domains/regression.go(新), domains/regression_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ domains 包全绿（3 个新测试）

## 2026-07-13: 阶段 C2 — Tier 2 用户偏好持久化

- **变更**:
  1. **新建 `memory/preference.go`**：`UserPreference` 结构体（Key/Value/Category: style|citation|format|domain）+ `SaveUserPreference(ctx, store, scope, pref)` 存入 LayerUser 层（metadata 含 type=preference/category/key）+ `LoadUserPreferences(ctx, store, scope, category)` 按类别检索（空类别=全部）
  2. **新建 `memory/preference_test.go`**：5 个测试 — Save 基本功能+空值拒绝+默认类别、Load 全部+按类别过滤
- **原因**: Tier 2 用户偏好需要跨会话持久化。基于 A5 的 SQLiteMemoryStore + LayerUser 层，用户偏好（写作风格/引用格式/输出格式）在重启后保留。配合 MemoryScope.UserID 实现多用户隔离
- **影响范围**: memory/preference.go(新), memory/preference_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码；依赖 A5 的 SQLiteMemoryStore / InMemoryStore）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ memory 包全绿（5 个新测试）

## 2026-07-13: 阶段 D1 — Tier 3 规则蒸馏候选框架

- **变更**:
  1. **新建 `memory/compiler/rule_bridge.go`**：
     - `CandidateStatus` 类型（draft/reviewed/approved/rejected）
     - `RuleCandidate` 结构体（StrategyID/Description/Guidance/SuccessRate/Samples/DraftRuleText/Status/HumanApproved/ShadowPassed/ReviewerNote/CreatedAt/ReviewedAt）— 从高成功率策略蒸馏出的候选规则
     - `PromotionGateConfig` + `DefaultPromotionGateConfig()`（≥5 样本/≥80% 成功率/必须人工批准/必须影子评估）
     - `RuleCandidateExtractor`（`ExtractFromCompiler(c *Compiler) []RuleCandidate`）— 遍历策略，按阈值筛选高成功率策略生成候选
     - `RulePromotionGate`（`Evaluate(c RuleCandidate) PromotionResult`）— 检查所有晋升要求，返回 Ready + 未满足原因列表
     - `MarkHumanApproval(approved, note)` / `MarkShadowResult(passed)` — 候选状态管理方法。**人工批准是唯一设置 HumanApproved 的途径，无法通过任何 extractor 或 gate 逻辑自动设置**
  2. **新建 `memory/compiler/rule_bridge_test.go`**：11 个测试 — ExtractFromCompiler（筛选+默认阈值）/ EmptyCompiler / Extractor 默认值 / Gate Ready / Gate NotReady（4 项全不满足）/ Gate 部分满足 / MarkHumanApproval / MarkHumanRejection / MarkShadowResult / DefaultPromotionGateConfig
- **原因**: Tier 3 规则蒸馏从 Compiler 的策略统计中提取高成功率策略，作为规则引擎候选规则。**技术预研点**：compiler 的 Strategy.Guidance 是提示策略文本，rule_engine 的 CheckRule 是结构化法律检查规则，两者无直接映射。D1 只建立候选提取+晋升门控框架，不实现 Guidance→CheckRule 的自动转换（需独立技术预研）。安全约束：Tier 3 永不全自动晋升，必须人工审核 + 影子评估
- **影响范围**: memory/compiler/rule_bridge.go(新), rule_bridge_test.go(新)
- **风险等级**: 低（新建文件，不修改任何现有代码；晋升门控强制人工批准，无自动晋升路径）
- **审查要求**: L3（涉及 Tier 3 规则蒸馏安全边界，虽然代码本身是框架性质，但晋升门控逻辑需人工审阅确认安全性）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ compiler 包全绿（11 个新测试 + 14 个原有测试）

## 2026-07-13: 阶段 D2 — 人工审查队列 + 晋升前影子评估

- **变更**:
  1. **新建 `memory/compiler/review_queue.go`**：
     - `ShadowEvalResult` 结构体（Passed/Score/Detail/RunAt）— 影子评估结果
     - `ShadowEvalFunc` 类型 — 由外部注入的评估函数（避免 compiler → evaluate 循环依赖），调用者负责桥接到 `benchmark.RunStatic`
     - `ReviewQueue` 结构体（sync.Mutex + 候选切片 + shadowFn）— 线程安全的审查队列
     - `Enqueue`（仅接受 Draft 状态候选）/ `Dequeue`（FIFO）/ `Pending` / `List`（快照不消费）
     - `RunShadowEval(c *RuleCandidate)` — 调用注入的 ShadowEvalFunc 并标记结果，未配置时返回错误
     - `ReviewSession(c, approved, note)` — 一站式审查流程：影子评估 → 人工批准 → 晋升门控检查 → 返回 PromotionResult
     - `DrainApproved()` — 批量取出已批准候选并从队列移除
  2. **新建 `memory/compiler/review_queue_test.go`**：11 个测试 — EnqueueAndDequeue / SkipNonDraft / List / RunShadowEval 成功/错误/未配置 / ReviewSession 批准/拒绝/影子失败 / DrainApproved / EmptyDequeue
- **原因**: D1 建立了候选提取和晋升门控框架，但缺少人工审查的流程编排。D2 提供审查队列（FIFO 管理待审候选）和影子评估机制（晋升前验证候选规则不会导致回归）。ShadowEvalFunc 通过依赖注入避免 compiler → evaluate 循环依赖
- **影响范围**: memory/compiler/review_queue.go(新), review_queue_test.go(新)
- **风险等级**: 低（新建文件，不修改任何现有代码；影子评估函数由外部注入，compiler 包无直接依赖 evaluate）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ compiler 包全绿（11 个新测试 + 25 个已有测试）

## 2026-07-13: 阶段 D3 — 规则晋升流程 + 审计日志

- **变更**:
  1. **新建 `memory/compiler/promoter.go`**：
     - `RuleRegistrar` 回调类型 — 由外部实现，将已批准候选注册到目标规则系统（如 `workflows/patent.RuleEngine`）。**调用者负责 RuleCandidate → CheckRule 的转换**（D1 标记的技术预研点，Guidance 文本 → 结构化法律规则无自动映射）
     - `PromotionLog` 结构体（CandidateID/StrategyID/SuccessRate/Samples/PromotedAt/Note）— 审计追踪
     - `RulePromoter` 结构体 — 编排最终晋升流程：门控检查 → 注册器调用 → 审计日志记录
     - `Promote(c)` — 单候选晋升，门控未通过或注册失败均返回错误
     - `PromoteBatch(queue)` — 从 ReviewQueue 批量晋升，单个失败不阻塞后续，返回成功数 + 错误列表
     - `Logs()` — 审计日志快照
     - `PromoteFromCompiler(c, queue, minSamples, minSuccessRate)` — 便捷管线：提取候选 → 入队（供人工审查流程使用）
  2. **新建 `memory/compiler/promoter_test.go`**：8 个测试 — Promote 成功/门控拒绝/注册器错误 / PromoteBatch 全成功/部分失败 / 默认值 / PromoteFromCompiler / PromotionLog 字段校验
- **原因**: D2 的审查队列完成了候选审查+影子评估，D3 完成最后的晋升注册环节。晋升门控在注册前再检查一次（defense-in-depth），注册器通过回调注入避免 compiler → patent 包的跨层依赖。审计日志满足 Tier 3 安全约束的可追溯要求
- **影响范围**: memory/compiler/promoter.go(新), promoter_test.go(新)
- **风险等级**: 低（新建文件；晋升门控强制人工批准+影子评估，无自动晋升路径；注册器回调由外部实现）
- **审查要求**: L3（涉及 Tier 3 规则晋升安全边界，晋升流程和审计日志需人工审阅）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全量 68 个包全绿（compiler 包 43 个测试）

## 2026-07-13: 向量检索落地阶段1 — SQLite backend 接线（FTS + Vector RRF 融合）

- **变更**:
  1. **新建 `knowledge/backend_hook.go`**(~130行)：`BackendRetrievalHook` 类型，嵌入 `BaseLifecycleHook`，实现 `BeforeModelCall` 调用 `KnowledgeExtension.search()` 走 `backendSearch`（FTS + Embed+VectorSearch → RRF 融合）；自实现 `buildContextBlock` + `injectContext`（复刻 `retrieval/agent.go` 的上下文格式化和注入逻辑）
  2. **修改 `knowledge/extension.go`**：新增 `BackendHook(cfg) agentcore.LifecycleHook` 方法，`backend==nil` 时返回 nil，否则返回 `NewBackendRetrievalHook`
  3. **修改 `cmd/mady/main.go`**：新增 `buildEmbedder()`（读 OMLX_BASE_URL/OMLX_API_KEY/OMLX_EMBED_MODEL 构建 `APIEmbedder`）、`loadKnowledgeBackend(madyHome)`（读 KNOWLEDGE_DB_DIR → `sqlite.NewSQLiteStore` 只读打开 knowledge.db）；改造 `loadWikiStore` 为优先 SQLite backend（buildEmbedder → loadKnowledgeBackend → NewExtension(nil,...) → WithBackend → BackendHook），回退 WIKI_PATH 内存库
  4. **新建 `knowledge/backend_hook_test.go`**：7 个测试覆盖 nil guard / context 注入 / 空查询跳过 / 无结果跳过 / nil mcc 安全 / FTS+Vector RRF 双通道融合
- **原因**: 向量检索算法层（APIEmbedder/SQLiteStore/RRFFuser/backendSearch）已实现但生产链路完全未接线，`WithBackend` 全项目零 caller，知识检索生产关闭。此改动完成阶段1接线，让 Agent 运行时自动从 81K 文档/144K chunks 的 knowledge.db 执行混合检索
- **影响范围**: knowledge/backend_hook.go(新), knowledge/extension.go, cmd/mady/main.go, knowledge/backend_hook_test.go(新)
- **环境变量**: OMLX_BASE_URL(默认 http://127.0.0.1:8000/v1) / OMLX_API_KEY / OMLX_EMBED_MODEL(默认 bge-m3-mlx-8bit) / KNOWLEDGE_DB_DIR(默认 ~/.mady/knowledge)
- **降级策略**: OMLX_API_KEY 未设置 → embedder=nil → SQLite backend 不可用 → 回退 WIKI_PATH 内存搜索 → 无 wiki 则知识检索关闭
- **风险等级**: 低（新建文件 + 非破坏性修改；SQLiteStore 只读模式；embedder/backend 均为可选注入，未设置时不改变原有行为）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 60+ 包全绿 | 端到端 `mady serve` 确认 `knowledge: SQLite backend active` ✅

## 2026-07-13: 向量检索落地阶段2 — 暴力查询优化 + Cross-encoder 重排

- **变更**:
  1. **新建 `knowledge/sqlite/vector_index.go`**(~150行)：`VectorIndex` 类型，启动时一次性 `SELECT chunk_id, document_id, vector FROM embeddings` 全量加载 144K 向量到连续 `[]float32`（`unsafe.Slice` 零拷贝 BLOB→float32）；`Search(queryVec, topK)` 并行 goroutine 分片计算点积（利用归一化跳过除法），合并排序取 Top-K
  2. **修改 `knowledge/sqlite/store.go`**：新增 `vecIndex *VectorIndex` 字段 + `PreloadVectors() error` + `HasVectorIndex() bool`；`VectorSearch` 开头检查 `vecIndex != nil` 走 `vectorSearchInMemory` 快速路径，否则回退 SQL 批量读取
  3. **新建 `retrieval/model_rerank.go`**(~200行)：`QueryReranker` 接口（扩展 `Reranker`，新增 `RerankWithQuery(ctx, query, results)`）；`ModelReranker` 类型调 Cohere 兼容 `/v1/rerank` 端点（oMLX Qwen3-Reranker-4B），支持 `MaxDocuments` 截断 + `TopN` 限制 + 降级（API 错误返回原结果）
  4. **修改 `knowledge/extension.go`**：`KnowledgeExtension` 新增 `queryReranker` 字段 + `WithReranker()` 方法；`backendSearch` 在 RRF 融合后检查 reranker：融合 candidateK 个候选 → rerank → 截取 topK
  5. **修改 `cmd/mady/main.go`**：`loadKnowledgeBackend` 中调用 `store.PreloadVectors()`；新增 `buildReranker()`（读 KNOWLEDGE_RERANK/OMLX_RERANK_MODEL）；`loadWikiStore` 中 `ext.WithReranker(reranker)` 接入
  6. **新建 `retrieval/model_rerank_test.go`**：8 个测试覆盖 no-op / 空输入 / 重排序 / API 错误降级 / MaxDocuments 截断 / TopN 限制 / 接口实现
  7. **修改 `knowledge/backend_hook_test.go`**：新增 `TestBackendHook_RerankerApplied` 验证 reranker 在 BeforeModelCall 中被正确调用且重排序生效
- **原因**: 阶段1接线后 VectorSearch 走 SQL 批量读取（144K 向量 ~3.7s），无法满足 <50ms 性能预算；同时启发式 reranker 无 query 语义信息，Top-5 精度不足
- **影响范围**: knowledge/sqlite/vector_index.go(新), knowledge/sqlite/store.go, retrieval/model_rerank.go(新), knowledge/extension.go, cmd/mady/main.go, retrieval/model_rerank_test.go(新), knowledge/backend_hook_test.go
- **环境变量**: 新增 OMLX_RERANK_MODEL(默认 Qwen3-Reranker-4B-4bit-MLX) / KNOWLEDGE_RERANK(默认 off，设为 on 启用)
- **降级策略**: PreloadVectors 失败 → 回退 SQL 批量 VectorSearch；KNOWLEDGE_RERANK=off → 跳过 reranker，直接 RRF topK；rerank API 错误 → 返回原 RRF 结果
- **风险等级**: 中（向量全量加载 ~560MB 内存；reranker 增加 ~200ms 延迟但可关闭）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ knowledge+retrieval 全绿

## 2026-07-13: 向量检索落地阶段2 T2.5 — Benchmark 基线

- **变更**:
  1. **新建 `knowledge/sqlite/bench_test.go`**：底层性能 benchmark（6 项）— PreloadVectorIndex(251ms) / FTSSearch(10.3ms) / VectorIndexSearch(14.5ms 纯计算) / VectorSearchInMemory(15.2ms 含IO) / VectorSearchSQL(1,328ms 对比基线) / GetChunk(5.2μs)
  2. **新建 `knowledge/bench_test.go`**（package knowledge_test）：端到端 benchmark — BackendSearch(29.8ms, FTS+Embed+Vector+RRF) / RRFFusion(4.6μs)；`benchEmbedder` 类型（预计算向量，不依赖 oMLX）
  3. **修改 `knowledge/sqlite/store.go`**：新增 `SampleVector()` 导出方法（从 embeddings 表取一条向量供 benchmark 使用）
  4. **修改 `knowledge/extension.go`**：新增 `Search()` 导出方法（委托 `search()`，供 external test 包调用）
  5. **新建 `docs/specs/vector-retrieval/benchmark-baseline.md`**：完整基线文档，含性能预算对比（全部达标）、耗时分解、并行效率分析、后续优化方向
- **原因**: 需要量化各检索路径性能，验证性能预算（VectorSearch<50ms / 端到端<500ms），建立优化前后的对比基线
- **关键数据**: 内存版 vs SQL 版 87x 加速；预加载 251ms 在 17 次查询后摊销；端到端 29.8ms 远低于 500ms 预算；M4 Pro 14核并行效率 ~14x
- **影响范围**: knowledge/sqlite/store.go, knowledge/sqlite/bench_test.go(新), knowledge/extension.go, knowledge/bench_test.go(新), docs/specs/vector-retrieval/benchmark-baseline.md(新)
- **风险等级**: 低（benchmark 测试文件 + 2 个导出方法，不改变运行时行为）
- **审查要求**: L1
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿 | 8 项 benchmark 全部产出数据

## 2026-07-13: 向量检索落地阶段3 — WritableStore + 三路 RRF + add_document 工具

- **变更**:
  1. **新建 `knowledge/sqlite/writable.go`**(~310行)：`WritableStore` 类型，读写模式打开 user.db（WAL）；`OpenWritable(path, embedder, knowledgeDBPath)` 建表（documents/chunks/embeddings/docs_fts，同 knowledge.db schema）+ 路径冲突检测（拒绝指向 knowledge.db）；`AddDocument(ctx, docID, title, content)` 分块（`retrieval.ChunkDocument`）→ 批量 Embed(batch=32) → 事务写入（delete 旧 + insert 新）；`Search(ctx, query, topK)` FTS+Vector RRF 融合；`float32ToBytes`/`vecNorm`/`hashString` 辅助函数
  2. **新建 `knowledge/sqlite/writable_test.go`**：11 个测试覆盖创建/FTS命中/无匹配/替换/路径冲突/nil embedder/空docID/并发写/schema幂等/hash/BLOB往返
  3. **修改 `knowledge/extension.go`**：新增 `WritableBackend` 接口（`Search` + `AddDocument`，领域层不 import sqlite）；`KnowledgeExtension` 新增 `writable` 字段 + `WithWritableStore()` 方法；`backendSearch` 新增第三路（user.db Search）参与 RRF 融合；`Tools()` 条件性暴露 `add_document` 工具（writable!=nil 时）；新增 `handleAddDocument` 方法
  4. **修改 `cmd/mady/main.go`**：`loadKnowledgeBackend` 改为返回 `(KnowledgeBackend, string)` 附带 knowledgeDBPath；新增 `openWritableStore(madyHome, embedder, knowledgeDBPath)`（读 USER_DB_PATH → `sqlite.OpenWritable` → 路径冲突检测 → 自动建目录）；`loadWikiStore` 中注入 `ext.WithWritableStore(ws)`
  5. **新建 `knowledge/ext_writable_test.go`**（package knowledge_test）：4 个集成测试 — add_document 工具暴露条件 / add_document→search 端到端命中 / 三路 RRF 融合（mockBackend + realWritable）/ 参数校验
- **原因**: 阶段1-2 完成了 knowledge.db 的只读检索（FTS+Vector RRF+Rerank），但用户无法向知识库添加自有文档。阶段3 新增独立 user.db（同构 schema，WAL 模式），通过 `add_document` 工具写入用户文档，检索时三路 RRF 融合（knowledge FTS + knowledge Vector + user Search），实现用户文档与权威知识库的混合检索
- **影响范围**: knowledge/sqlite/writable.go(新), knowledge/sqlite/writable_test.go(新), knowledge/extension.go, cmd/mady/main.go, knowledge/ext_writable_test.go(新)
- **环境变量**: 新增 USER_DB_PATH(默认 $MADY_HOME/knowledge/user.db)
- **安全**: user.db 路径冲突检测（拒绝指向 knowledge.db）；WAL 模式 + sync.Mutex 单写者；参数化查询防注入；embedder=nil 时 WritableStore 不初始化
- **降级策略**: embedder=nil → WritableStore 不初始化（无 add_document 工具，三路退化为两路）；OpenWritable 失败 → 打印警告继续（不影响 knowledge.db 检索）；user Search 失败 → 跳过该路，用 knowledge FTS+Vector 两路继续 RRF
- **风险等级**: 中（新增写入路径 + 新增工具；user.db 与 knowledge.db 物理隔离 + 路径冲突检测缓解污染风险）
- **审查要求**: L3（安全敏感：writable.go 新增写入沙箱边界）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全部包全绿（含 15 个新测试）
  1. 删除 `agentcore/manifests/chat.json`（embed 源）和 `manifests/chat.json`（根目录用户参考示例）
  2. 更新 `agentcore/manifest_test.go`：5 个测试的硬编码 manifest 数量从 4→3 / 5→4；ExternalOverride 测试从覆盖 chat-agent 改为覆盖 assistant-agent
- **原因**: 提交 `6837337`（Chat Agent 与意图识别深度融合）后，chat-agent 由 `IntegratedChatConfig` 统一动态构建（`domains/chat.go:71`），`ProfessionalHandoffConfigs` 已明确排除 chat（`domains/router.go:80`）。chat.json 作为独立 manifest 已多余，导致启动日志显示不必要的路由项
- **影响范围**: agentcore/manifests/、manifests/、agentcore/manifest_test.go（不影响代码层面的 ChatAgentConfig/IntegratedChatConfig/DomainChat 常量/分类器枚举）
- **风险等级**: 低（集成模式不依赖 chat manifest；Router 模式的 chatHandoff 在代码中硬编码，不依赖 manifest）
- **审查要求**: L1

## 2026-07-13: TUI 案件上下文接入（/case + /deadline 命令族）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `currentProject`/`currentProjectMeta` 变量；buildCfg 的 applyPersistence 扩展为注入案件 WorkspaceDir + SystemPrompt 上下文段；新增 /case 命令族（list/info/off/<关键词>切换），按 ProjectID 或 Alias 模糊匹配；新增 /deadline 命令显示当前案件期限；新增 formatProjectContext/formatProjectInfo 辅助函数；slashSuggestions 添加 /case 和 /deadline
- **原因**: 评审文档阶段2核心——让 TUI 用户能选择/切换案件，Agent 运行时感知案件上下文（工作目录、领域、期限）。ProjectRegistry 已就绪，只需 TUI 层接入
- **影响范围**: cmd/mady/main.go（1 个文件，约 130 行新增）
- **风险等级**: 低（复用已测试的 ProjectRegistry API，不涉及安全敏感路径；WorkspaceDir 注入使用 RootPath 字段，已有 sandbox 保护）
- **审查要求**: L1

## 2026-07-13: TUI /export 对话导出

- **变更**: **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数，将 ChatHistory 格式化为 Markdown（含案件信息、角色标签、时间戳）；slashSuggestions 新增 /export
- **原因**: 律师需要导出对话记录作为工作文档，评审文档 3.3 建议
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（只读导出，不涉及安全敏感路径）
- **审查要求**: L1

## 2026-07-13: TUI /review 审核关卡 + /export 对话导出

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `reviewMode` 变量；applyPersistence 中当 reviewMode=true 时注入 `domains.NewApprovalGate(domains.DefaultApprovalConfig())` 到 LifecycleChain；新增 `/review` 命令切换审核关卡开关（重建 Agent + 更新状态栏）；slashSuggestions 新增 /review
  2. **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数（Markdown 格式含案件信息+角色标签+时间戳）
- **原因**: 评审文档 3.2（/review 审批）和 3.3（/export 导出）。ApprovalGate 是"提醒式"审批（通过 Agent.Steer 注入审批提示，非同步阻塞），适合作为 TUI 开关命令
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（ApprovalGate 是已有已测试的 LifecycleHook；/export 是只读文件写入）
- **审查要求**: L1

## 2026-07-13: TUI reasoning 五阶段推理工具接入（阶段3.1）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `domains/reasoning` 包导入；applyPersistence 中当 currentProject 不为 nil 时，调用 `reasoning.NewWorkflowRunner()` 创建 FiveStepRunner 并通过 `reasoning.AsWorkflowTool()` 注入为 agentcore.Tool（retriever=nil/llm=nil 的 MVP 模式：有默认模板+L1校验，无知识库检索+L2/L3 LLM校验）；新增 `mapMatterTypeToCaseType()` 辅助函数（8种事项类型模糊匹配→CaseType 枚举）；/case 切换成功后提示推理工具已启用
- **原因**: 评审文档 /sources 建议建立在虚构的 ExecutionResult 上，但 reasoning 包的 Plan/CheckReport/UsedFacts/UsedRules 真实存在。之前 FiveStepRunner 零生产 caller，完整五阶段编排未接入 Agent 运行时。此改动让 TUI Agent 能在案件上下文中调用深度可验证推理
- **影响范围**: cmd/mady/main.go（1 个文件，约 35 行新增）
- **关键复用**: `reasoning.AsWorkflowTool()`（handoff_integration.go:41，已有完整 Tool 适配器）+ `reasoning.NewWorkflowRunner()`（handoff_integration.go:91，已有预配置工厂）
- **风险等级**: 低（复用已有适配器，不修改 reasoning 包源码；Tool 注入是 append 不是覆盖）
- **审查要求**: L1

## 2026-07-13: TUI 会话持久化（JSONL 自动保存 + 分支）

- **变更**:
  1. **`cmd/mady/main.go`**: buildCfg 前创建 FileStore + AgentStore + MemoryCheckpointSaver（优先级：$SESSION_DIR > $MADY_HOME/sessions > ./sessions）；buildCfg 闭包内新增 applyPersistence 辅助函数，每个模式分支（集成/路由/单Agent）统一注入 Store + Checkpoint；OnSubmit goroutine 中 Agent.Run 完成后自动调用 SaveState（用 context.Background() 确保中断后仍可保存）；/new 和 /clear 创建新 ThreadID（tui-{timestamp}）；/branch 实现真正的分支功能（BranchThread + UI 消息恢复）；/save 显示会话保存路径和线程数；slashSuggestions 中 /branch 和 /save 描述更新
- **原因**: 评审文档 P1 阻断项——TUI 之前纯内存模式，重启丢失对话，/save /branch 均提示不支持。复用 serve 模式的 session.FileStore + AgentStore 持久化方案
- **影响范围**: cmd/mady/main.go（1 个文件，约 80 行新增）
- **风险等级**: 低（复用已测试的 session 包，不涉及安全敏感路径；CheckpointSaver 为内存态不持久化，Store 为磁盘 JSONL）
- **审查要求**: L1

## 2026-07-13: TUI 状态栏常驻 + Handoff 文案中文化

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `statusBarModeLabel()` 辅助函数，生成中文友好的状态栏模式标签（集成/多域路由/🧠 计划 + 推理级别）；初始化时设置状态栏（之前完全缺失）；/thinking 命令后更新状态栏（之前不更新）；/plan 命令统一使用 statusBarModeLabel
  2. **`tui/chat/chat_app.go`**: UpdateStatusBar 格式从 `provider=X model=X mode=X` 简化为 `X/X · 模式标签`；onHandoffStart/onHandoffEnd 文案中文化（"handoff"→"已切换至"、"done"→"已完成"、"handoff failed"→"交接失败"）
- **原因**: 评审文档建议 1.2（/thinking/mode 状态栏常驻）和 1.3（Handoff 显示简化）。状态栏之前初始化时为空，/thinking 不更新；Handoff 英文文案对律师不友好
- **影响范围**: cmd/mady/main.go, tui/chat/chat_app.go
- **风险等级**: 低（UI 文案+状态栏显示逻辑，不涉及安全敏感路径）
- **审查要求**: L1

## 2026-07-12: 文档全面同步 — 552 文件/134K 行/新增 domains/rules + knowledge/sqlite + retrieval/domain

- **变更**:
  1. **CLAUDE.md**: 代码统计（517→552 文件，352→376 非测试，165→176 测试，~126K→~134K 行）；目录结构新增 domains/rules/、knowledge/sqlite/、retrieval/domain/、tools/browser_providers/、pkg/agentconfig/、benchmark/、integration/；agentcore 文件数修正（88+27→75+40，含子包拆分）；依赖列表更新（+modernc.org/sqlite +gopkg.in/yaml.v3）；架构图基础设施层补 knowledge/retrieval/benchmark/integration
  2. **README.md**: 发展路线新增 SQLite 知识库 + RRF 混合检索、YAML 规则引擎 + OA 解析 + 反套话引擎、五步工作法；知识管理段落补充 SQLite 只读取层和 RRF；推理引擎段落补充 domains/rules（OA解析/反套话/法律意图）；扩展表格新增规则引擎行
  3. **CHANGELOG.md**: [0.3.0] 新增 10 项 Added（SQLite 读取层、RRF 混合检索、YAML 规则引擎、OA 解析、反套话引擎、法律意图检测、五步工作法、pkg/agentconfig、browser_providers）
  4. **CONTRIBUTING.md**: 目录结构新增 domains/rules、knowledge/sqlite、retrieval/domain、tools/browser_providers、benchmark、integration、pkg/agentconfig；架构图基础设施层补 benchmark/integration
  5. **docs/knowledge.md**: 架构图补充 KnowledgeBackend + RRF Fuser；新增 SQLite 只读取层段落（3 个数据库表 + RRF 公式）
  6. **docs/adr/0001**: 基础设施层补充 knowledge/retrieval/benchmark/integration；依赖说明补充 modernc.org/sqlite
  7. **docs/chat-assistant-architecture.md**: 新增「v0.3.0 后续迭代（已完成）」10 项
  8. **AGENTS.md**: 核心分层描述更新（+domains/rules +memory +disclosure +ACP）；新增文件数/行数统计
- **原因**: 文档再次滞后于代码进度（代码已 552 文件/~134K 行，文档仍记 517 文件/~126K 行；v0.3.0 新增的 domains/rules + knowledge/sqlite + RRF 混合检索 + OA 解析 + 反套话引擎 + 五步工作法在多份文档中缺失）
- **影响范围**: CLAUDE.md, README.md, CHANGELOG.md, CONTRIBUTING.md, AGENTS.md, docs/knowledge.md, docs/adr/0001, docs/chat-assistant-architecture.md, docs/decisions/AI_CHANGELOG.md
- **风险等级**: 低（纯文档变更，不涉及代码逻辑）
- **审查要求**: L1

## 2026-07-12: XiaoNuno专利能力移植 — OA解析/反套话引擎/法律意图检测

- **变更**:
  1. **新增 `domains/rules/oa_parser.go`**: 审查意见解析器（从XiaoNuo legal-bus/src/rules/oa-parser.ts移植）。纯规则零LLM，3个提取函数：`DetectOaRejectionType`(7组关键词匹配novelty/inventiveness/clarity/support/disclosure/scope/formal)、`ExtractCitations`(正则提取CN/US/WO/EP/JP/KR专利文献号)、`ExtractAffectedClaims`(正则提取权利要求号+范围展开)；入口`ParseOfficeAction`+`FormatOaSummary`
  2. **新增 `domains/rules/slop_engine.go`**: 反AI套话引擎（从XiaoNuo slop-engine.ts 452行完整移植）。三层架构：Layer1短语级(42条正则替换规则，7个分组filler/qualifier/meta/intimacy/subjectless/search/advisory)、Layer2结构级(6种缺陷检测empty_three_step/fake_comparison/binary_turn/reason_pile/passive_voice/oa_formula)、Layer3评分级(50分制5维directness/evidence/rhythm/practicality/concision+8项快检)；入口`AnalyzeSlop`+`FormatSlopAnalysis`
  3. **新增 `domains/legal_intent.go`**: 法律意图细分检测器（从XiaoNuo LegalIntentDetector.ts 270行移植）。`@legal`显式触发+15组关键词→CaseType映射(复用reasoning.CaseType 12种)、专利语境门控(14个信号词)、子串去重(utf8.RuneCountInString)；入口`DetectLegalIntent`+`SelectRunMode`。独立函数，不修改现有ClassifyIntent路由
  4. **修改 `domains/rules/engine.go`**: RulesExtension.Tools()新增2个ReadOnly工具：`parse_office_action`(审查意见解析)、`analyze_slop`(反套话分析)
- **原因**: Mady基础框架完整但缺专利文书规则解析层。XiaoNuo的纯规则解析器从BCIP codex-patent-domain(Rust)移植，天然适合Go重写，零LLM开销
- **影响范围**: domains/rules/oa_parser.go(新), domains/rules/oa_parser_test.go(新), domains/rules/slop_engine.go(新), domains/rules/slop_engine_test.go(新), domains/legal_intent.go(新), domains/legal_intent_test.go(新), domains/rules/engine.go(修改)
- **风险等级**: 低（6个新文件+1个文件追加工具，不修改现有路由/classifier/安全路径）
- **审查要求**: L2

## 2026-07-12: ACP 知识系统集成修复

- **变更**:
  1. **`acp/server_app.go`**: `RunOptions` 新增 `Lifecycle agentcore.LifecycleHook` 字段；`buildAgentConfig` 将其注入 `agentcore.Config.Lifecycle`，使 ACP 创建/重建的 Agent 能携带知识检索等生命周期钩子
  2. **`cmd/mady/main.go`**: `runAcp()` 改为调用 `setupFrameworkContext()`（与 `runTui`/`runServer` 对齐），将 `fc.WikiHook` 通过 `RunOptions.Lifecycle` 传入 ACP 服务器
- **原因**: ACP 入口（`mady acp`）此前完全跳过了 `setupFrameworkContext()`，不加载 Wiki 知识库、不注入 RAG 检索钩子，导致 ACP 用户（如 Zed 编辑器）无法使用知识系统；TUI 和 Serve 已正确集成
- **影响范围**: acp/server_app.go, cmd/mady/main.go
- **风险等级**: 低（新增可选字段，nil 时不改变原有行为；已有测试全部通过）
- **审查要求**: L2

## 2026-07-12: 阶段4 — YAML规则引擎 (domains/rules/)

- **变更**:
  1. **新增 `domains/rules/types.go`**: Go类型系统，覆盖4种YAML格式 — Rule/Check（规则文件）、ArticleFramework/ArticleStep（法条框架）、Orchestration/DiscoveryStage/ExecutionTemplate（事务编排）、ReflectionDomain（反思指示词）；Check使用自定义`UnmarshalYAML`两遍解码：已知字段填充结构体，未知字段保存在`Extra map[string]any`供消费者解释
  2. **新增 `domains/rules/loader.go`**: `LoadFromDir(dir)` 从目录加载全部YAML文件，自动分类（顶层规则文件/articles/*/orchestrations/*/reflection-indicators.yaml），构建索引（rulesByDomain/rulesBySeverity/ruleIndex）
  3. **新增 `domains/rules/engine.go`**: `Engine`查询引擎（AllRules/RuleByID/RulesByDomain/RulesBySeverity/Article/Orchestration/ReflectionIndicators/SearchRules/ToRuleConstraints）+ `RulesExtension`实现agentcore.Extension（ToolProvider+SystemPromptProvider+TransformContextProvider）；暴露3个工具：search_rules、get_article_framework、get_orchestration；ToRuleConstraints将规则转换为reasoning.RuleConstraint供推理框架使用
  4. **新增 `domains/rules/engine_test.go`**: 10个测试覆盖全部功能（加载/Extra字段/域查询/严重度查询/ID查询/搜索/法条框架/编排/反思指示词/RuleConstraint转换）
  5. **依赖**: 添加 `gopkg.in/yaml.v3` v3.0.1（已在go.sum中间接存在，现提升为直接依赖）
- **原因**: XiaoNuo的规则数据（novelty/inventiveness/disclosure/claims/amendment/response 6个顶层规则文件 + 8个法条框架 + 2个事务编排 + 反思指示词）是专利法律推理的核心知识资产，需要在Mady中以Extension机制集成，供Agent通过工具查询规则、法条判断框架和事务编排方案
- **影响范围**: go.mod, go.sum, domains/rules/types.go, domains/rules/loader.go, domains/rules/engine.go, domains/rules/engine_test.go
- **风险等级**: 低（纯新增包，不修改任何现有文件）
- **审查要求**: L2

## 2026-07-12: 代码审查修复 — Context传播/错误处理/FTS5转义/LIKE转义

- **变更**:
  1. **Context传播** (`knowledge/extension.go`): `search`/`backendSearch`/`memorySearch` 方法签名增加 `context.Context` 参数；`handleSearch`/`Provide` 传递调用者ctx；`backendSearch` 中 `e.embedder.Embed` 从 `context.Background()` 改为 `ctx`，支持用户中断时取消嵌入API调用
  2. **NewSQLiteStore错误处理** (`knowledge/sqlite/store.go`): 添加 `db.Ping()` 验证连通性；维度检测查询失败时返回error而非静默回退到dim=1024
  3. **VectorSearch rows.Err()** (`knowledge/sqlite/store.go`): 内层 `rows.Next()` 循环后添加 `rows.Err()` 检查，避免DB错误导致静默返回部分结果
  4. **FTS5引号转义** (`knowledge/sqlite/store.go`): `strconv.Quote(query)` 替换为 FTS5 兼容的双引号包裹+内部双引号加倍（`"` → `""`），避免反斜杠转义导致查询异常
  5. **SearchLaws LIKE转义** (`knowledge/sqlite/store.go`): 转义 `%`→`\%`、`_`→`\_`、`\`→`\\`，添加 `ESCAPE '\'` 子句，确保关键词字面匹配
  6. **backendSearch错误日志** (`knowledge/extension.go`): FTS/Vector/Embed 错误从静默吞没改为 `log.Printf` 记录，便于诊断持续性故障
- **原因**: 代码审查发现6个问题（2中等+4低），涉及context传播缺失、错误静默吞没、SQL注入风险（非安全注入但语义错误）
- **影响范围**: knowledge/extension.go, knowledge/sqlite/store.go
- **风险等级**: 低（修复内部实现细节，不改变公开API）
- **审查要求**: L2

## 2026-07-12: 引入 XiaoNuo 知识系统数据资产 + SQLite 读取层 + RRF 混合检索

- **变更**:
  1. **数据资产引入**: 在 `~/.mady/knowledge/` 下创建符号链接，引入 XiaoNuo Agent 的知识数据（knowledge.db 6.5GB 含81K文档/144K分块/215K图谱节点/144K嵌入向量；laws-full.db 152MB 含9121条法律；patent_kg.db 207MB；ipc-classification/ 6.8MB；wiki/ 17MB；rules/ 76KB）
  2. **SQLite 依赖**: 添加 `modernc.org/sqlite` v1.53.0（纯Go无CGO），更新 go.mod
  3. **SQLite 读取层** (`knowledge/sqlite/store.go`, 419行): `SQLiteStore` 支持只读打开 knowledge.db/laws-full.db/patent_kg.db；`FTSSearch` 利用 FTS5 trigram + BM25 评分；`VectorSearch` 批量读取 BLOB float32 嵌入向量计算余弦相似度；`LoadGraph` 从 kg_nodes/kg_edges 批量加载到内存 GraphStore；`SearchLaws` LIKE 搜索法律库
  4. **RRF 融合检索器** (`retrieval/hybrid.go`): `RRFFuser` 实现 Reciprocal Rank Fusion 算法（k=60），融合 FTS 和向量搜索结果，score-agnostic 只看排名位置
  5. **Extension 集成 SQLite 后端** (`knowledge/extension.go`): 新增 `KnowledgeBackend` 接口（`FTSSearch`/`VectorSearch`）；`WithBackend()` setter 注入 SQLiteStore + Embedder；`search()` 方法优先走 SQLite 后端（FTS+Vector RRF 融合），降级到内存关键词搜索；`handleSearch`/`Provide` 统一调用 `search()` 分发
  6. **测试**: `knowledge/sqlite/store_test.go`（FTS/Graph/Laws 3测试全过）；`retrieval/hybrid_test.go`（RRF 4测试全过）
- **原因**: Mady 原有知识库仅2篇种子文档，无法支撑专利/法律专业领域智能体；XiaoNuo Agent 的数据模型（GraphNode/GraphEdge/节点类型/关系类型/权威度权重）与 Mady 完全对齐，嵌入向量格式兼容（BGE-M3 1024维 float32 LE），可直接复用
- **影响范围**: go.mod, go.sum, knowledge/sqlite/store.go, knowledge/sqlite/store_test.go, retrieval/hybrid.go, retrieval/hybrid_test.go, knowledge/extension.go
- **风险等级**: 低（新增文件+非破坏性修改，现有功能通过 WithBackend 可选注入，不影响默认行为）
- **审查要求**: L2（新增 SQLite 依赖和数据访问层，需确认只读模式和路径安全）

## 2026-07-11: 文档全面同步实际开发进度

- **变更**:
  1. **CLAUDE.md**: 代码统计（419→517 文件，283→352 非测试，136→165 测试，~108K→~126K 行）；目录结构新增 disclosure/memory/agentcore 子包/guardrails/guardian/；架构概要扩展层 10+→35+；新增 Invisible Handoff + IntegratedChatConfig 描述
  2. **CHANGELOG.md**: 版本顺序修正（0.3.0→0.2.0→0.1.0）；补充 0.3.0 缺失特性（Embed Manifest、MADY_HOME、Invisible Handoff、Reasonix 9 包、四级压缩、Permission/Guardian/PlanMode/Evidence/FileCheckpoint/MemoryCompiler/Tracing/Evaluate）；添加 [0.3.0] 链接
  3. **README.md**: 发展路线更新（下季度项中已实现的标记为当前）；架构图补充 memory/；manifest 说明改为 embed + ~/.mady/manifests/；扩展表格新增 8 个 opt-in 扩展包（Evidence/FileCheckpoint/Permission/PlanMode/Guardian/Evaluate/Tracing/Memory）；工具数 40+→35
  4. **SECURITY.md**: 护栏描述修正为实际行为（关键词屏蔽+免责声明+审批门，非"仅免责声明"）；新增 Guardian AI 熔断器 + Permission 权限门控描述；新增安全敏感路径表（12 条路径）；版本表 0.1.x→0.x.x
  5. **docs/chat-assistant-architecture.md**: 后续迭代补充 Invisible Handoff / Embed Manifest / Reasonix 包；下季度候选项更新
  6. **docs/manifest-guide.md**: 文件位置改为 embed + $MADY_HOME/manifests/；启动方式更新
  7. **docs/adr/0001**: TUI 7 层→8 层；基础设施层补充 disclosure/memory/filequeue/fuzzy
  8. **CONTRIBUTING.md**: 目录结构新增 disclosure/memory/filequeue/fuzzy；架构图工具层 10+→35，基础设施层补充新模块
- **原因**: 文档全面滞后于代码实际进度（代码已 517 文件/~126K 行，文档仍记 419 文件/~108K 行；v0.3.0 新增的 12 项特性在多份文档中缺失或描述不足）
- **影响范围**: CLAUDE.md, CHANGELOG.md, README.md, SECURITY.md, docs/chat-assistant-architecture.md, docs/manifest-guide.md, docs/adr/0001-use-layered-architecture.md, CONTRIBUTING.md
- **风险等级**: 低（纯文档变更，不涉及代码逻辑）
- **审查要求**: L1

## 2026-07-11: Chat Agent 与意图识别模块深度融合（Invisible Handoff + IntegratedChatConfig）

- **变更**:
  1. `agentcore/handoff.go`：`HandoffConfig` 新增 `Invisible bool` 字段；`executeDelegate` 中 `Invisible=true` 时不再将子 Agent 事件总线转发到父 Agent
  2. `agentcore/event.go`：`HandoffStartEvent` / `HandoffEndEvent` 新增 `Invisible bool` 字段
  3. `domains/router.go`：提取 `ProfessionalHandoffConfigs()` 共享函数；`AllowedSources` 白名单增加 `"chat-agent"`
  4. `domains/chat.go`：新增 `IntegratedChatConfig(base)` 工厂函数，注册 `ProfessionalHandoffConfigs` 为 Invisible Handoff，SystemPrompt 融合路由指令与对话能力；`ChatAgentConfig` 保持纯聊天向后兼容
  5. `tui/chat/events.go`：`HandoffStartChatEvent` / `HandoffEndChatEvent` 新增 `Invisible bool` 字段
  6. `tui/agentadapter/adapter.go`：透传 `Invisible` 标志
  7. `tui/chat/chat_app.go`：`onToolStart`/`onToolEnd` 跳过 `transfer_to_*` 工具显示；`onHandoffStart`/`onHandoffEnd` 跳过 `Invisible` 交接公告
  8. `cmd/mady/main.go`：新增 `useIntegratedMode`（`MADY_ROUTER_MODE=1` 回退到传统 Router 模式，`MADY_SINGLE_AGENT=1` 回退到单 Agent 模式）；集成模式使用 `IntegratedChatConfig` 作为默认 Agent

- **原因**: Chat Agent 功能单一且意图识别交接过程在 TUI 中可见（`transfer_to_*` 工具调用 + handoff 系统消息 + 子 Agent 实时输出流），影响用户体验。深度融合后 Chat Agent 成为统一对话界面，内部通过 Invisible Handoff 无缝委派专业任务。

- **影响范围**: agentcore/handoff.go, agentcore/event.go, domains/router.go, domains/chat.go, tui/chat/events.go, tui/agentadapter/adapter.go, tui/chat/chat_app.go, cmd/mady/main.go

- **风险等级**: 中（触及 `agentcore/handoff.go` 的安全敏感路径 — HandoffConfig 结构体和 executeDelegate 事件总线逻辑，但 AllowedSources 白名单校验不变，仅新增 Invisible 控制字段）

- **审查要求**: L3（handoff 白名单扩展 + 入口模式切换逻辑需审阅）

## 2026-07-11: 让 mady 在任意工作目录开箱即用（embed manifest + MADY_HOME 统一路径层）

- **变更**:
  1. `pkg/util/paths.go`（新增）：统一路径解析层 `MadyHome()` / `EnsureDir()` / `ResolveDataDir()`，优先级 `$MADY_HOME` > `~/.mady`
  2. `agentcore/embedded_manifests.go`（新增）+ `agentcore/manifests/*.json`（从仓库根 `manifests/` 迁入）：4 个领域 manifest 通过 `go:embed` 编进二进制，任意目录可用
  3. `agentcore/manifest_loader.go`：重构出 `ScanManifestsFromFS(fs.FS)`，新增 `LoadManifests(userDir)` 实现「内置 embed + 外部目录覆盖/新增」合并语义
  4. `cmd/mady/main.go`：`setupFrameworkContext()` 统一走 `util.MadyHome()`，消除 5 处 cwd 相对路径依赖（manifest/workspace/session/AgentStore cwd）；修掉 `main.go:581` 硬编码 `./workspace` 绕过 `WORKSPACE_DIR` 的隐蔽 bug
  5. `agentcore/agent.go` Config 新增 `WorkspaceDir` 字段；`domains/assistant.go` 读取 `base.WorkspaceDir` 替代硬编码 `./workspace`，经 Router 工厂链透传
  6. `Makefile` 新增 `install` target（默认 `PREFIX=~/.local`）
  7. 文档同步：`.env.example` 清理死变量（`KNOWLEDGE_DIR`/`SKILL_DIR`单数）、新增 `MADY_HOME` 说明；`AGENTS.md` 补「资源定位」gotcha
- **原因**: 修复"从非项目根目录启动 `mady tui` 静默降级为裸 LLM 对话"的根因——manifest 扫描依赖相对路径 `./manifests`，目录不存在时 `ScanManifests` 返回 nil 导致 `useMultiDomain=false`，全部领域 agent 能力丢失
- **影响范围**: pkg/util, agentcore(manifest_loader/agent/embedded_manifests), cmd/mady, domains/assistant, Makefile, .env.example, AGENTS.md
- **风险等级**: 中（触及安全敏感路径 `agentcore/manifest_loader.go` 的 Manifest 校验规则，但未改校验逻辑，仅重构加载入口 + 加 embed；`domains/assistant.go` WorkingDir 透传影响工具沙箱边界）
- **审查要求**: L3

## 2026-07-11: 引入 Reasonix 高价值特性 — Phase 0-2 实施

- **变更**: 基于 Reasonix 分析报告，为 Mady 引入 9 个新特性包，全部以 opt-in Extension 模式接入，零侵入现有代码路径：
  1. **Phase 0.1 Tool ReadOnly** (`agentcore/tool.go`): Tool 结构新增 `ReadOnly` 字段 + `DynamicReadOnly` 回调 + `ToolReadOnly()` 辅助函数；`tools/tools.go` 标记 12 个只读工具
  2. **Phase 0.2 Evidence Ledger** (`agentcore/evidence/`): Receipt/Ledger/查询方法/context 注入/Extension 自动注册，追踪每个 turn 的工具调用证据
  3. **Phase 0.3 File Checkpoint** (`agentcore/filecheckpoint/`): Store/Snapshot/Restore + BeforeHook 自动快照写入工具，支持按 turn 回退文件状态
  4. **Phase 1.1 Guardian AI** (`guardrails/guardian/`): AI 安全审查子 Agent，熔断器，三档审查级别，Middleware 集成，fail-closed
  5. **Phase 1.2 Permission System** (`agentcore/permission/`): Allow/Ask/Deny 三态决策 + 规则解析（glob/command prefix）+ Approver 接口 + Middleware
  6. **Phase 1.3 Plan Mode** (`agentcore/planmode/`): 计划模式工具门控，bash 命令安全分类器（read-only/write），LifecycleHook 集成
  7. **Phase 2.1 Tiered Compaction** (`agentcore/context_engine_tiered.go`): 四级渐进式压缩管线（snip→prune→force-fold），注册为 "tiered" ContextEngine
  8. **Phase 2.2 Memory Compiler** (`memory/compiler/`): 策略学习型记忆扩展，ε-greedy 探索，执行轨迹追踪，质量分级 + 置信度衰减，5 个预置专利/法律策略
- **原因**: 系统性提升 Agent 安全性、上下文管理效率、和学习能力，借鉴 Reasonix 工程实践
- **影响范围**: agentcore/{tool.go, evidence/, filecheckpoint/, permission/, planmode/, context_engine_tiered.go, context_engine.go, context_engine_test.go}, tools/tools.go, guardrails/guardian/, memory/compiler/
- **安全敏感**: 是（涉及 Permission 门控、Guardian 审查、Plan Mode 工具门控、文件系统操作）
- **验证**: go build ✅ | go test -race ✅ 全部通过
- **风险等级**: 中（新功能均为 opt-in，不影响现有代码路径）
- **审查要求**: L3

## 2026-07-11: 修复三个 CRITICAL 并发安全问题

- **变更**:
  1. `domains/agent_pool.go` GetOrCreate 消除 defer+手动 Unlock 混合模式导致的 double-unlock panic，改为显式 Lock/Unlock + 锁外批量 Close
  2. `domains/reasoning/fact_blackboard.go` 为 FactBlackboard 添加 sync.RWMutex 保护所有字段，写方法检查 Locked 并 panic，MarshalJSON/UnmarshalJSON 加锁
  3. `domains/project.go` 提取 StatusActive/StatusArchived/StatusUnreachable 常量替换硬编码字符串
- **原因**: 消除运行时 panic 风险和并发数据竞争
- **影响范围**: domains/agent_pool.go, domains/reasoning/fact_blackboard.go, domains/project.go
- **风险等级**: 中（涉及安全敏感路径 agent_pool 和并发同步）
- **审查要求**: L3

## 2026-07-11: 全面代码质量审查修复 — 16 CRITICAL + 45 MAJOR + lint清零

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

## 2025-06-11: 初始化代码质量全面审查报告

- **变更**: 完成 Mady 项目首次全面代码质量审查，覆盖 484 个文件的 6 大维度
- **原因**: 系统性识别性能瓶颈、安全漏洞、架构合规性问题，支撑智能体高效调用
- **审查结果**: 审查报告已输出至 `docs/decisions/REVIEW_REPORT_2025-06-11.md`
- **风险等级**: 中（大量安全/性能问题需修复）
- **审查要求**: L2

## 2026-07-13: 向量检索端到端验证修复 — Dimensions 修正 + Extension 注册暴露工具

- **变更**:
  1. **修正 `retrieval/embedding.go` `Dimensions()` 方法**：bge-m3 系列模型未在已知列表中，default case 返回 1536 导致 WritableStore schema 建为 1536 维，与实际 1024 维向量不匹配（`vector dim mismatch: got 1024, want 1536`）。添加 `strings.Contains(strings.ToLower(e.Model), "bge-m3") → return 1024` 判断
  2. **Extension 注册到 `cfg.Extensions` 暴露工具**：`loadWikiStore` 新增第三个返回值 `agentcore.Extension`（KnowledgeExtension），`frameworkContext` 新增 `KnowledgeExt` 字段，`buildCfg` 3 分支（集成/路由/单Agent）+ `runServer` + `runAcp` 均注入 `cfg.Extensions`。此前 Extension 只返回了 BackendHook（LifecycleHook），`Tools()` 方法从未被调用，`search_knowledge` 和 `add_document` 工具未暴露
  3. **`acp/server_app.go`**：`RunOptions` 新增 `Extensions []agentcore.Extension` 字段，`buildAgentConfig` 传递到 `agentcore.Config.Extensions`
  4. **`cmd/mady/main.go`**：新增 `extSlice()` 辅助函数（nil 安全的单 Extension → slice 转换）
- **原因**: 端到端测试发现两个问题 — (1) user.db 向量搜索维度不匹配 (2) add_document 工具未被 agent 识别
- **影响范围**: `retrieval/embedding.go`、`cmd/mady/main.go`、`acp/server_app.go`
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿 | 端到端：`mady serve` + oMLX → add_document 写入 → search_knowledge 检索命中 → 日志零报错
- **风险等级**: L2（Dimensions 修正影响所有 APIEmbedder 调用方；Extension 注册改变 agent 工具集）
- **审查要求**: L2

## 2026-07-13: 代码审查修复 — 跨数据库 chunk ID 冲突 + buildReranker 空值检查

- **变更**:
  1. **修复 `knowledge/sqlite/writable.go` chunk ID 冲突**：`ftsSearch` 和 `getChunk` 中的 `ID: strconv.Itoa(id)` 改为 `ID: "u:" + strconv.Itoa(id)`。knowledge.db 和 user.db 是独立的 SQLite 数据库，各自的 AUTOINCREMENT 序列都从 1 开始。`RRFFuser.Fuse`（`retrieval/hybrid.go:44`）用 `r.ID` 字符串去重，两个数据库的相同数字 ID 会被误判为同一 chunk，导致 RRF 分数错误累积和结果静默丢失
  2. **修复 `cmd/mady/main.go` `buildReranker` 空值检查**：文档字符串声明"OMLX_API_KEY 未设置返回 nil"，但代码未检查空值。添加 `if apiKey == "" { return nil }` 使实现与文档一致
  3. **新增回归测试 `TestExtension_CrossDBIDNoCollision`**：模拟 knowledge.db 返回数字 ID "1" + user.db 也有 chunk ID 1，验证两者在 RRF 融合后均独立出现（不被错误合并）
- **原因**: 代码审查（task review）发现三路 RRF 融合中的跨数据库 ID 冲突 bug — 当 user.db 配置启用时（`OMLX_API_KEY` 已设置 + `add_document` 被调用），搜索结果会静默损坏
- **影响范围**: `knowledge/sqlite/writable.go`（2处 ID 前缀）、`cmd/mady/main.go`（buildReranker 空值检查）、`knowledge/ext_writable_test.go`（新增回归测试）
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 全绿（含新测试 `TestExtension_CrossDBIDNoCollision`）
- **风险等级**: L2（chunk ID 格式变更影响 RRF 去重行为，但仅限 user.db 路径；knowledge.db 路径不变）
- **审查要求**: L2

## 2026-07-14: 全面修复 P1/P2 Golden Benchmark 与 CitationCompleteness 代码质量问题

- **变更**:
  1. **重构 `CitationCompleteness` 法条匹配逻辑**（`agentcore/evaluate/metrics.go`）：
     - 将 `normalizeChineseNumerals` 改为仅对 `第X条/款/项/章/节/点/部分` 结构中的中文数字归一化，避免误伤普通文本（如"三天"、"二十二项任务"）
     - 扩展 `citationPattern` 支持 `第X条第Y款第Z项` 与 `第X条之一/二/三` 等复杂引用
     - 新增 `citationSetMatches` 概括匹配：required `第X条` 可命中 pred `第X条第Y款`，required `第X条第Y款` 可命中 pred `第X条第Y款第Z项`
     - 保留 `CitationAwareMetric` 接口与 `WithCitations` 机制，evaluator 中通过类型安全方式注入 per-case RequiredCitations，避免修改原始 metric 实例
  2. **将 P2 无效决定书 benchmark 数据迁出到 JSON**（`agentcore/evaluate/benchmark/invalidation_decisions.go` + `invalidation_decisions.json`）：
     - 原 `invalidation_decisions.go` 为 40 个硬编码 case 的 293 行文件，现改为通过 `go:embed invalidation_decisions.json` 加载
     - 数据文件保持与 Go 结构完全一致的 JSON 格式，便于工具链生成与校验
  3. **修复 P2 数据错误**（`invalidation_decisions.json`）：
     - case `invalidation_decision_004`：结论与核心理由矛盾，已按原始 docx 更正为"部分无效"及合议组认定
     - case `invalidation_decision_039`：`Expected` 从段落标题"3.3 关于独立权利要求9"替换为真实"三、决定"内容
     - case `invalidation_decision_040`：补充缺失的专利号 `202020860338.5`
  4. **合并 `live_deepseek_test.go` 重复代码**：
     - 新增 `deepSeekTestEnv` 与 `newDeepSeekTestEnv` 统一读取环境变量、构造 provider
     - 新增 `randomCases` 固定随机种子（`20241201`），保证专利考试真题抽样可复现
     - 新增 `runLiveEval` 公共 helper：缓存加载、批量调用、input→prediction 映射、报告输出
     - 将原 `TestLiveDeepSeekEval` 和 `TestLiveDeepSeekInvalidationEval` 简化为对公共 helper 的调用
  5. **统一 `RequiredCitations` 法条格式**：40 个无效决定书法条全部规范为阿拉伯数字格式（如"专利法第22条第3款"），共 62 条引用，集中在创造性（22.3）、清楚（26.4）、说明书支持（26.3）、新颖性（22.2）、决定程序（46.1）、优先权（29.1）
  6. **新增/更新测试**（`agentcore/evaluate/evaluate_test.go`）：
     - `TestCitationCompletenessChineseNumerals`：中文数字与阿拉伯数字互配
     - `TestCitationCompletenessNoSubstringMismatch`：防"第2条"误匹配"第22条第3款"
     - `TestCitationCompletenessParagraphGeneralization`："第22条"匹配"第22条第3款"
     - `TestCitationCompletenessItemReference`："第22条第3款"匹配"第22条第3款第2项"
     - `TestCitationCompletenessSuffix`："第10条"匹配"第10条之一"
     - `TestCitationCompletenessContextProtection`：普通中文数字（无"第...条"结构）不误归一化
- **原因**: P2 无效决定书基线（15.0% 通过）远低于 P1（66.7%），审阅发现 `CitationCompleteness` 仅做简单 `strings.Contains` 无法匹配中文数字法条、存在子串误匹配，且 40 个 case 硬编码在单文件、数据有误、测试代码重复。修复后基线提升至 32.5% 通过（6/40 → 13/40），`citation_completeness` 从 0.287 提升至 0.775
- **影响范围**: `agentcore/evaluate/metrics.go`, `agentcore/evaluate/evaluate_test.go`, `agentcore/evaluate/evaluator.go`, `agentcore/evaluate/benchmark/invalidation_decisions.go`, `agentcore/evaluate/benchmark/invalidation_decisions.json`（新增）, `agentcore/evaluate/benchmark/live_deepseek_test.go`
- **风险等级**: 低（评估与测试代码，不影响生产运行时路径；仅修改数据/指标/测试结构）
- **审查要求**: L2（涉及评估指标行为与 Golden Benchmark 数据质量，需审阅指标语义是否正确）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./...` 全绿 ✅ | `make eval` ✅ | `golangci-lint` 未运行（网络超时无法安装 v2.12.2）
