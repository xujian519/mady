# AI 决策变更日志（归档）

> **归档段**：2026-07-13~14 评估框架增强与 DoomLoop

> 本文件包含 45 个决策记录，从主 `AI_CHANGELOG.md` 归档。
> 归档时间：2026-07-19。主文件仅保留近期决策与归档索引。

---

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

---

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

---

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

---

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

---

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

---

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

---

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



---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

## 2026-07-13: 修复 knowledge/fileindex/store.go 语法错误

- **变更**: 补全 `store.go` 中缺失的 `import` 块闭合 `)` 与 `const` 块闭合 `)`，恢复该包可编译
- **原因**: 该文件存在语法错误导致 `go build ./...` 失败，阻塞 TUI 阶段 2 验证；修复后不影响任何业务逻辑
- **影响范围**: `knowledge/fileindex/store.go`
- **风险等级**: 低（纯语法修复，无行为变更）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go test -race ./knowledge/fileindex` ✅

---

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

---

## 2026-07-13: TUI 复制功能修复 — Kitty flag 8 + 右键复制

- **变更**:
  1. **Kitty flag 8 开启**：`NewProcessTerminal()` 默认 `kittyFlags` 从 `1`（仅 disambiguate）改为 `1 | 8`（report all keys），使得 `Cmd+C` 作为 CSI u 序列 `\x1b[99;9u`（ModSuper）到达，可区分于 `Ctrl+C`（ModCtrl）；`main.go` TUI 入口同步显式设置 `KittyKeyboardFlags: 1 | 8`
  2. **右键复制**：`chatLayout.Update()` 中 `MouseRelease + Button==2` 分支触发 `doCopy(l)`，复用现有选区/剪贴板基础设施
  3. **Alacritty 支持**：`TerminalSupportsKittyKeyboard()` 新增 `ALACRITTY_WINDOW_ID` 和 `TERM=alacritty` 检测（Alacritty 0.13.0+ 支持 Kitty 协议）
- **原因**: (1) 无法使用 ⌘+C 复制 — Kitty flag 1 不足以区分 Cmd 和 Ctrl 修饰键；(2) 鼠标右键无法复制 — SGR 鼠标已正确解码 Button 2，但 layout 层未响应
- **影响范围**: `cmd/mady/main.go`、`tui/chat/chat_app.go`、`tui/terminal/terminal.go`
- **风险等级**: 低（flag 8 对不支持 Kitty 协议的终端安全忽略；右键事件与左键互斥；21 项测试全部通过）
- **审查要求**: L1

---

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

---

## 2026-07-13: 评估闭环与记忆自学习方案评审报告

- **变更**: 新建决策文档 `docs/decisions/eval-memory-plan-review-2026-07-13.md`（276行），对《评估闭环模块》《记忆自学习模块》《整体阶段划分》三部分方案进行代码级评审。结论：理念合格，落地需大改。识别 4 处重大脱节（向量检索已完成、评估框架已存在、memory 缺持久化、Checkpoint 概念混淆），提炼 7 项真正有价值的缺失工作，给出修正后的 A→B→C→D 四阶段落地路线（含 A5/A6 持久化基础设施前置）
- **原因**: 方案基于过时项目快照，照原样执行将产生两套并行系统（EvalCase vs TestCase、LawyerPreference vs MemoryEntry），且遗漏 memory/StageCheckpoint 缺持久化的关键风险
- **影响范围**: docs/decisions/eval-memory-plan-review-2026-07-13.md(新)；后续阶段 A-D 的实现将涉及 agentcore/evaluate、memory、domains/reasoning、domains/approval、guardrails、workflows/patent 等包
- **风险等级**: 低（仅文档变更，无代码改动）
- **审查要求**: L2

---

## 2026-07-13: 阶段 A5 — MemoryStore SQLite 持久化后端

- **变更**:
  1. **新建 `memory/sqlite_store.go`**(~380行)：`SQLiteMemoryStore` 类型，实现 `MemoryStore` 接口，数据持久化到 SQLite（WAL 模式）。Schema 含 memories 表（15 列）+ 2 个索引（layer/scope）。检索策略与 `InMemoryStore` 一致：关键词匹配 + 复合评分（语义+新鲜度+重要性），复用 `keywordScore`/`recencyScore`/`estimateImportance` 等包级函数。Embedding 以 BLOB 存储（little-endian float32），供未来向量检索升级。Metadata 以 JSON 序列化。支持 `SQLiteOption` 函数式配置（`WithSQLiteScoringConfig`/`WithSQLiteClock`）
  2. **新建 `memory/sqlite_store_test.go`**：15 个测试覆盖 Remember/Get/RememberBatch/Update/Forget/ForgetAll/Recall/RecallWithBudget/List/Prune/Stats/Persistence（关闭重开数据不丢失）/Concurrency（20 goroutine 并发写 + 10 并发读）/EmbeddingRoundTrip/空内容拒绝
- **原因**: `InMemoryStore`（memory/store.go:14）是 Phase 1 纯内存实现，重启后数据丢失。方案 Tier 2 用户偏好需要跨重启持久化，Tier 1 案件记忆预热也依赖持久化后端。此改动为 B2/C2 的前置基础设施
- **影响范围**: memory/sqlite_store.go(新), memory/sqlite_store_test.go(新)
- **风险等级**: 低（新建文件，不修改任何现有代码；`MemoryStore` 接口和 `InMemoryStore` 保持不变；`Manager` 通过接口注入，无需改动）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ memory 包全绿（含 15 个新测试 + 原有测试）

---

## 2026-07-13: 阶段 A6 — CheckpointStore SQLite 持久化后端

- **变更**:
  1. **新建 `domains/reasoning/sqlite/checkpoint_store.go`**(~150行)：`SQLiteCheckpointStore` 类型，实现 `reasoning.CheckpointStore` 接口（Save/Load/Delete）。Schema 含 stage_checkpoints 表（checkpoint_id PK + case_id/case_type/current_stage 索引列 + data JSON 列）+ case_id 索引。复用 `reasoning.MarshalCheckpoint`/`UnmarshalCheckpoint` 做 JSON 序列化。额外提供 `ListByCase` 方法按案件查询所有检查点。子包设计遵循依赖倒置：domain 层不导入 `database/sql`
  2. **新建 `domains/reasoning/sqlite/checkpoint_store_test.go`**：6 个测试覆盖 Save+Load/LoadNotFound/Delete/SaveReplace（同 ID 覆盖）/ListByCase/Persistence（关闭重开数据不丢失）
- **原因**: `MemoryCheckpointStore`（domains/reasoning/checkpoint.go:36）只有内存实现，重启后丢失。方案 Tier 1 案件记忆预热（B2）依赖 `ResumeFromCheckpoint` 从持久化 `CheckpointStore` 恢复。此改动为 B2 的前置基础设施
- **影响范围**: domains/reasoning/sqlite/checkpoint_store.go(新), domains/reasoning/sqlite/checkpoint_store_test.go(新)
- **风险等级**: 低（新建子包，不修改任何现有代码；`CheckpointStore` 接口和 `MemoryCheckpointStore` 保持不变）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ 6 个测试全绿

---

## 2026-07-13: 阶段 A2 — Golden Benchmark 第一层（专利代理人考试模拟题）

- **变更**:
  1. **修改 `agentcore/evaluate/evaluator.go`**：`TestCase` 结构体新增 `Domain string` 字段，用于按领域（专利/法律/通用）筛选用例
  2. **新建 `agentcore/evaluate/benchmark/patent_exam.go`**：10 道模拟专利代理人考试题，覆盖新颖性判断(001)、创造性分析(002)、权利要求保护范围(003)、OA答复(004)、等同侵权(005)、无效宣告(006)、可专利性客体(007)、先用权(008)、从属权利要求审查(009)、上位概念侵权(010)。每题含 ID/Domain/Input/Expected/RequiredCitations。提供 `CaseCount()` 和 `CasesByDomain(domain)` 辅助函数
- **原因**: 评估闭环需要 Golden Benchmark 作为回归基准。方案第一层用考试真题，但版权风险高，先以 MVP 质量模拟题建仓（10 题），后续扩展至 50-100 题并经领域专家审核
- **影响范围**: agentcore/evaluate/evaluator.go(修改), agentcore/evaluate/benchmark/patent_exam.go(新)
- **风险等级**: 低（仅新增数据集 + 非破坏性字段扩展）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ evaluate 包全绿

---

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

---

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

---

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

---

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

---

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

---

## 2026-07-13: 阶段 C1 — Golden Benchmark 第二层回归用例转化

- **变更**:
  1. **新建 `domains/regression.go`**：`ApprovalToTestCase(record ApprovalRecord, domain string) evaluate.TestCase` — 将 DecisionModified 的审批记录转化为 TestCase（OriginalOutput→Input, ModifiedOutput→Expected）+ `ApprovalToRegressionCandidates(records, domain) []evaluate.TestCase` — 批量过滤+转化，跳过非 Modified 和空 ModifiedOutput 的记录
  2. **新建 `domains/regression_test.go`**：3 个测试 — 单条转化+字段校验、批量过滤（5 条记录→2 条候选）、空输入
- **原因**: B1 的 ApprovalGate 留痕中，DecisionModified 记录隐含人工质量标准。C1 半自动将这些记录转化为回归用例，构建 Golden Benchmark 第二层（脱敏真实案例）。人工仍需审核后才加入正式数据集
- **影响范围**: domains/regression.go(新), domains/regression_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ domains 包全绿（3 个新测试）

---

## 2026-07-13: 阶段 C2 — Tier 2 用户偏好持久化

- **变更**:
  1. **新建 `memory/preference.go`**：`UserPreference` 结构体（Key/Value/Category: style|citation|format|domain）+ `SaveUserPreference(ctx, store, scope, pref)` 存入 LayerUser 层（metadata 含 type=preference/category/key）+ `LoadUserPreferences(ctx, store, scope, category)` 按类别检索（空类别=全部）
  2. **新建 `memory/preference_test.go`**：5 个测试 — Save 基本功能+空值拒绝+默认类别、Load 全部+按类别过滤
- **原因**: Tier 2 用户偏好需要跨会话持久化。基于 A5 的 SQLiteMemoryStore + LayerUser 层，用户偏好（写作风格/引用格式/输出格式）在重启后保留。配合 MemoryScope.UserID 实现多用户隔离
- **影响范围**: memory/preference.go(新), memory/preference_test.go(新)
- **风险等级**: 低（新建文件，不修改现有代码；依赖 A5 的 SQLiteMemoryStore / InMemoryStore）
- **审查要求**: L2
- **验证**: go build ✅ | go vet ✅ | go test -race ✅ memory 包全绿（5 个新测试）

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

## 2026-07-13: TUI 案件上下文接入（/case + /deadline 命令族）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `currentProject`/`currentProjectMeta` 变量；buildCfg 的 applyPersistence 扩展为注入案件 WorkspaceDir + SystemPrompt 上下文段；新增 /case 命令族（list/info/off/<关键词>切换），按 ProjectID 或 Alias 模糊匹配；新增 /deadline 命令显示当前案件期限；新增 formatProjectContext/formatProjectInfo 辅助函数；slashSuggestions 添加 /case 和 /deadline
- **原因**: 评审文档阶段2核心——让 TUI 用户能选择/切换案件，Agent 运行时感知案件上下文（工作目录、领域、期限）。ProjectRegistry 已就绪，只需 TUI 层接入
- **影响范围**: cmd/mady/main.go（1 个文件，约 130 行新增）
- **风险等级**: 低（复用已测试的 ProjectRegistry API，不涉及安全敏感路径；WorkspaceDir 注入使用 RootPath 字段，已有 sandbox 保护）
- **审查要求**: L1

---

## 2026-07-13: TUI /export 对话导出

- **变更**: **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数，将 ChatHistory 格式化为 Markdown（含案件信息、角色标签、时间戳）；slashSuggestions 新增 /export
- **原因**: 律师需要导出对话记录作为工作文档，评审文档 3.3 建议
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（只读导出，不涉及安全敏感路径）
- **审查要求**: L1

---

## 2026-07-13: TUI /review 审核关卡 + /export 对话导出

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `reviewMode` 变量；applyPersistence 中当 reviewMode=true 时注入 `domains.NewApprovalGate(domains.DefaultApprovalConfig())` 到 LifecycleChain；新增 `/review` 命令切换审核关卡开关（重建 Agent + 更新状态栏）；slashSuggestions 新增 /review
  2. **`cmd/mady/main.go`**: 新增 `/export` 命令（默认导出到 $MADY_HOME/exports/，支持自定义路径）；新增 `formatExportMarkdown` 辅助函数（Markdown 格式含案件信息+角色标签+时间戳）
- **原因**: 评审文档 3.2（/review 审批）和 3.3（/export 导出）。ApprovalGate 是"提醒式"审批（通过 Agent.Steer 注入审批提示，非同步阻塞），适合作为 TUI 开关命令
- **影响范围**: cmd/mady/main.go
- **风险等级**: 低（ApprovalGate 是已有已测试的 LifecycleHook；/export 是只读文件写入）
- **审查要求**: L1

---

## 2026-07-13: TUI reasoning 五阶段推理工具接入（阶段3.1）

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `domains/reasoning` 包导入；applyPersistence 中当 currentProject 不为 nil 时，调用 `reasoning.NewWorkflowRunner()` 创建 FiveStepRunner 并通过 `reasoning.AsWorkflowTool()` 注入为 agentcore.Tool（retriever=nil/llm=nil 的 MVP 模式：有默认模板+L1校验，无知识库检索+L2/L3 LLM校验）；新增 `mapMatterTypeToCaseType()` 辅助函数（8种事项类型模糊匹配→CaseType 枚举）；/case 切换成功后提示推理工具已启用
- **原因**: 评审文档 /sources 建议建立在虚构的 ExecutionResult 上，但 reasoning 包的 Plan/CheckReport/UsedFacts/UsedRules 真实存在。之前 FiveStepRunner 零生产 caller，完整五阶段编排未接入 Agent 运行时。此改动让 TUI Agent 能在案件上下文中调用深度可验证推理
- **影响范围**: cmd/mady/main.go（1 个文件，约 35 行新增）
- **关键复用**: `reasoning.AsWorkflowTool()`（handoff_integration.go:41，已有完整 Tool 适配器）+ `reasoning.NewWorkflowRunner()`（handoff_integration.go:91，已有预配置工厂）
- **风险等级**: 低（复用已有适配器，不修改 reasoning 包源码；Tool 注入是 append 不是覆盖）
- **审查要求**: L1

---

## 2026-07-13: TUI 会话持久化（JSONL 自动保存 + 分支）

- **变更**:
  1. **`cmd/mady/main.go`**: buildCfg 前创建 FileStore + AgentStore + MemoryCheckpointSaver（优先级：$SESSION_DIR > $MADY_HOME/sessions > ./sessions）；buildCfg 闭包内新增 applyPersistence 辅助函数，每个模式分支（集成/路由/单Agent）统一注入 Store + Checkpoint；OnSubmit goroutine 中 Agent.Run 完成后自动调用 SaveState（用 context.Background() 确保中断后仍可保存）；/new 和 /clear 创建新 ThreadID（tui-{timestamp}）；/branch 实现真正的分支功能（BranchThread + UI 消息恢复）；/save 显示会话保存路径和线程数；slashSuggestions 中 /branch 和 /save 描述更新
- **原因**: 评审文档 P1 阻断项——TUI 之前纯内存模式，重启丢失对话，/save /branch 均提示不支持。复用 serve 模式的 session.FileStore + AgentStore 持久化方案
- **影响范围**: cmd/mady/main.go（1 个文件，约 80 行新增）
- **风险等级**: 低（复用已测试的 session 包，不涉及安全敏感路径；CheckpointSaver 为内存态不持久化，Store 为磁盘 JSONL）
- **审查要求**: L1

---

## 2026-07-13: TUI 状态栏常驻 + Handoff 文案中文化

- **变更**:
  1. **`cmd/mady/main.go`**: 新增 `statusBarModeLabel()` 辅助函数，生成中文友好的状态栏模式标签（集成/多域路由/🧠 计划 + 推理级别）；初始化时设置状态栏（之前完全缺失）；/thinking 命令后更新状态栏（之前不更新）；/plan 命令统一使用 statusBarModeLabel
  2. **`tui/chat/chat_app.go`**: UpdateStatusBar 格式从 `provider=X model=X mode=X` 简化为 `X/X · 模式标签`；onHandoffStart/onHandoffEnd 文案中文化（"handoff"→"已切换至"、"done"→"已完成"、"handoff failed"→"交接失败"）
- **原因**: 评审文档建议 1.2（/thinking/mode 状态栏常驻）和 1.3（Handoff 显示简化）。状态栏之前初始化时为空，/thinking 不更新；Handoff 英文文案对律师不友好
- **影响范围**: cmd/mady/main.go, tui/chat/chat_app.go
- **风险等级**: 低（UI 文案+状态栏显示逻辑，不涉及安全敏感路径）
- **审查要求**: L1

---

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

---

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

---

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

---
