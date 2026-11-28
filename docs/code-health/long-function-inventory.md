# 长函数清单（>120 行）

> 定位：M45（`docs/code-review-plan.md` 阶段 8）产物。保守档**只登记、不拆解**——
> 拆解属行为等价重构，需完整测试护栏，另立计划。
>
> 口径：**单函数 >120 行**（含 `func` 声明行；`*_test.go` 不计）。数据源即机器验证：
>
> ```bash
> python3 scripts/scan_go_smells.py --list-long-funcs            # 根模块
> python3 scripts/scan_go_smells.py tui --list-long-funcs        # tui 子模块
> python3 scripts/scan_go_smells.py desktop --list-long-funcs    # desktop 子模块
> ```
>
> 当前计数（2026-11-28 实测）：根模块 **34** + `tui/` **10** = **44 处**；`desktop/` 为 0
> （其 `third_party/` 由扫描器排除）。本表是快照，新增/消失以后续扫描输出为准。

## 1. 分级规则

| 级别 | 判据 | 处置 |
| --- | --- | --- |
| **P1** | 纯逻辑/解析/遍历，无 `//nolint:gocognit`，可切分为独立函数并用表格驱动测试锁定行为 | 优先排期（另立 refactor 卡） |
| **P2** | 编排/装配/状态机，切分需引入中间结构或拆阶段，改动面跨方法 | 有专项需求时拆，拆前补特征测试 |
| **P3** | 工具构造器（`New*Tool`）、渲染分发、协议面或敏感路径，代码内已 `//nolint:gocognit` 声明拆分收益低 | 保留；仅在功能变更时顺带评估 |

**P3 中的 14 处带 `//nolint:gocognit`**（根模块 7 + `tui/` 7）：豁免是显式例外
（AGENTS.md 元规则「例外显式」），理由写在代码行尾注释；本表不对其做任何代码改动。

分布：P1 **5** 处 / P2 **15** 处 / P3 **24** 处。

## 2. P1 — 优先拆解（5 处）

| 行数 | 位置 | 函数 | 拆分方向 |
| ---: | --- | --- | --- |
| 232 | `domains/figure/generate/check_scan.go:202` | `scanSVG` | 按 SVG 元素/属性类型拆表驱动 handler（`scanElement` 映射表），共享游标与结果累加器 |
| 145 | `domains/figure/generate/check_path.go:33` | `parsePath` | 按 path 命令字母（M/L/C/Z…）拆子函数，游标与回调抽成小结构 |
| 140 | `knowledge/loader/wiki.go:112` | `importDirectory` | 拆「遍历过滤 / 单文件导入 / 统计累加」三段，目录遍历与导入解耦 |
| 124 | `domains/evidence/date.go:270` | `extractDateFromText` | 按日期格式族拆候选提取器 + 统一打分，格式表驱动 |
| 123 | `tui/terminal/key_decode.go:126` | `decodeKittyU` | 纯函数，按 CSI-u 字段解析拆小函数，用表格驱动测试逐个锁定 |

## 3. P2 — 有条件拆解（15 处）

| 行数 | 位置 | 函数 | 拆分方向 |
| ---: | --- | --- | --- |
| 268 | `cmd/mady/tui.go:60` | `runTui` | 拆「参数/配置装配」与「事件循环启动」两段 |
| 233 | `cmd/mady/slash_commands.go:294` | `registerPlanTaskCommands` | 注册项按领域拆 `registerPlanTask*` 子函数，主函数只留编排 |
| 190 | `cmd/mady/server.go:83` | `runServer` | 服务装配与路由/中间件注册分离 |
| 164 | `domains/reasoning/plan_compiler.go:132` | `CompilePlanToGraph` | 节点装配按节点类型拆，边构建与校验独立 |
| 153 | `mcp/config_discovery.go:82` | `DiscoverMCPExtensions` | 多来源发现各自成函数，合并与去重单列 |
| 146 | `mcp/client_reconnect.go:44` | `tryReconnect` | 重连策略（退避/上限）与状态推进分离 |
| 141 | `provider/chatcompat/responses.go:318` | `readResponsesStream` | SSE 逐事件解析拆 `parseResponseEvent`，流生命周期单独处理 |
| 137 | `agentcore/agent_run_tool.go:88` | `executeToolCalls` | M01 已登记：按「准备 / 执行 / 结果归并」拆（内核主链路，需回归测试） |
| 129 | `cmd/mady/session_panel.go:28` | `openSessionSelector` | 面板数据加载与渲染分离 |
| 129 | `knowledge/fileindex/store.go:151` | `Refresh` | 拆「扫描 / 差异判定 / 写库 / 清理」四阶段 |
| 129 | `tools/piagent/schema.go:33` | `schemaNode` | JSON Schema 递归按类型分派（带 nolint，但纯逻辑、可表格驱动测试） |
| 128 | `disclosure/export.go:58` | `buildMarkdownReport` | 按报告章节拆 render 函数，拼装层只负责顺序 |
| 125 | `domains/figure/generate/submission.go:109` | `BuildSubmissionPage` | 页面元素逐块组装，校验与拼装分离 |
| 134 | `tui/tui_render.go:37` | `renderFrame` | 帧合成按「背景 / 主区 / 覆盖层 / 状态栏」拆阶段 |
| 150 | `tui/overlay_render.go:21` | `composeOverlays` | 叠加层定位与裁剪分离（带 nolint，涉及宽字符口径，须跑 `tui/` 全量测试） |

## 4. P3 — 保留（24 处）

| 行数 | 位置 | 函数 | 保留理由 |
| ---: | --- | --- | --- |
| 397 | `example/cli-chat/main.go:29` | `main` | 示例程序的交互循环，拆分收益低 |
| 209 | `tools/browser_tool_navigate.go:19` | `handleNavigate` | 工具编排 + 平台分支，已 `nolint:gocognit` |
| 164 | `agentcore/worker/catalog.go:77` | `DefaultWorkers` | 纯数据清单装配 |
| 164 | `tools/execute_code.go:103` | `NewExecuteCodeTool` | 工具构造器，已 `nolint:gocognit` |
| 157 | `domains/patent.go:307` | `PatentAgentConfig` | **敏感路径**（`domains/patent.go`，动态 WorkingDir），禁止非审阅改动 |
| 156 | `domains/rules/engine.go:232` | `RulesExtension.Tools` | 工具清单装配 |
| 152 | `tools/desktop/computer_use_cua_driver.go:32` | `cuaDriverCapture` | 平台后端分支，已 `nolint:gocognit` |
| 147 | `disclosure/report.go:62` | `assessNoveltyFromState` | **敏感路径**（`disclosure/report.go`，review_gate 主动中断） |
| 147 | `tools/pandoc.go:51` | `NewPandocTool` | 工具构造器，已 `nolint:gocognit` |
| 140 | `agui/handler.go:57` | `handleRun` | 协议面（AGUI 事件/帧），保守档零改动 |
| 133 | `tools/read.go:64` | `NewReadTool` | 工具构造器，已 `nolint:gocognit` |
| 132 | `bootstrap/init_reasoning.go:41` | `InitReasoningAndTemplates` | 启动装配顺序敏感，拆开不降低风险 |
| 130 | `domains/case_extension.go:61` | `CaseExtension.Tools` | 工具清单装配 |
| 128 | `domains/evidence/tool_statutory.go:27` | `newStatutoryTool` | 工具构造器（含 schema 字面量） |
| 123 | `bootstrap/init_knowledge.go:25` | `LoadWikiStore` | 启动装配，已 `nolint:gocognit` |
| 122 | `tools/tools.go:274` | `BuildTools` | **敏感路径**（`tools/tools.go`，工具能力门控） |
| 121 | `domains/claimdrafting/rules.go:28` | `RegisterDefaultRules` | 规则注册表（逐条登记，拆开反而割裂可读性） |
| 194 | `tui/theme/palette.go:45` | `BuildPalette` | 渲染/主题构造，已 `nolint:gocognit` |
| 141 | `tui/component/review_gate.go:420` | `ReviewGate.renderLocked` | 渲染状态机，已 `nolint:gocognit` |
| 129 | `tui/chat/chat_history_input.go:77` | `ChatHistory.handleMouse` | 事件分发状态机，已 `nolint:gocognit` |
| 129 | `tui/layout/flex.go:348` | `Flex.renderHorizontal` | 布局渲染，已 `nolint:gocognit` |
| 128 | `tui/chat/chat_app_keys.go:26` | `chatLayout.dispatchKey` | 按键分发状态机，已 `nolint:gocognit` |
| 124 | `tui/component/session_selector.go:465` | `SessionSelector.Render` | 渲染分发 |
| 122 | `tui/component/keyhelp.go:140` | `KeyHelp.Render` | 渲染分发，已 `nolint:gocognit` |

## 5. 拆解时的护栏（另立计划时适用）

1. **先补测试再拆**：P1/P2 目标函数若无直接测试，先写特征测试（表驱动优先）固定当前行为，
   再拆；`tui/` 改动须 `cd tui && go test -race -count=1 ./...` 全绿。
2. **行为等价**：拆解提交为 `refactor`，不得混入 `fix`/`feat`；每卡 3-5 个文件。
3. **红线不变**：协议面（a2a/a2ui/agui/acp/server/mcp）只允许重构不改帧结构；敏感路径
   （`scripts/check-sensitive-paths.sh`）不因拆解而改动语义。
4. **口径回归**：每次拆解后跑 `python3 scripts/scan_go_smells.py`，本表计数应下降；
   若函数被拆到 120 行以下但文件更大，属预期（本指标只看单函数）。

## 6. 相关文档

- `scripts/scan_go_smells.py` — 计数与明细再生源（`--list-long-funcs`）
- `docs/code-review-plan.md` — 阶段 8 登记与「保守档只记录不拆」边界
- `docs/code-review-report.md` — §五 P1-3 遗留项引用
