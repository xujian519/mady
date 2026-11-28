# Mady 全仓代码审阅终审报告

- 计划文档：`docs/code-review-plan.md`
- 计划启动：2026-08-25；审阅收官：2026-11-13
- 执行档位：**保守档**（只做无行为变化的清理；不拆大文件、不动架构、不升级依赖）——M63 按用户指令单独立「依赖升级卡」，范围限定同主版本 patch/minor，大版本迁移仍排除在外
- 覆盖范围：根模块 Go、`tui/` 独立子模块（Go）、`desktop/` 独立子模块（Go 后端 + React/TS 前端）
- 日卡：M01–M36 + 阶段 7 专项（M37-1～M37-6、M38、M39、M40、M41）全部完成

## 一、结论摘要

1. **审阅覆盖率 100%**：M01–M36 与阶段 7 专项（M37-1～M37-6 吞错专项、M38 宽字符口径 fix、M39 忽略 error 收敛、M40 口径修正 + 导出补注、M41 可疑忽略 fix）逐卡完成，进度表可追溯，无跳卡、无延期遗留整卡。
2. **行为不变**：阶段 1–6 全部提交为 `refactor` / `docs` 类型，未混入 `feat` / `fix`；阶段 7 的 `fix` 卡仅 M38（渲染口径，用户批准）与 M41（新增告警观测点 + 等价解析改写，用户批准），其余为 `refactor`（零行为变更）。每卡跑 build / vet / lint / arch / doc / layers / debt / test 门禁。
3. **红线零触碰**：敏感路径权威源（18 文件 + 2 前缀）零改动（M37-3 唯一命中站点按规范跳过、仅登记）；A2A/A2UI/AGUI/ACP/Server/MCP 协议面与工具 `inputSchema` 零改动。
4. **四项指标全部达标**：「未注释导出」93 → 0、「静默吞错」380 → 0、「被忽略 error」24 → 0、「裸 fmt」66 → 0（M44 口径收口：CLI 输出层单列 `cli_fmt` 179 处显式可见，规范见 `docs/cli-output-standards.md`）。
5. **发现并登记的真 P0 行为缺陷：0 项**。审阅中登记的复杂度问题（长函数、大文件）按保守档只记录不拆，清单见 §五。

## 二、指标对比（`scripts/scan_go_smells.py`）

根模块（非测试 Go 文件，2026-11-13 实测；扫描器口径 1050 个 Go 源文件 / 31 目录）：

| 指标 | 基线 2026-08-25 | 终态 2026-11-13 | 计划目标 | 结论 |
| --- | --- | --- | --- | --- |
| 裸 `fmt.Print*`（库代码调试残留） | 66 | **0** | < 30 | ✅ 达成（M44 口径收口，CLI 输出层单列 `cli_fmt` 179，见 §四-1） |
| 被忽略的 error（`_ = <expr>` 等） | 24 | **0** | ≤ 10 | ✅ 达成（M39 收敛 + M40 口径修正 + M41 逐条判定，见 §四-2） |
| 静默吞错无注释 | 380 | **0** | 显著下降 | ✅ 下降 100%（见 §四-3） |
| 未注释包级导出 | 93 | **0** | ≤ 20 | ✅ 达成 |
| >120 行单函数 | 30 | **35**（+ `tui/` 10 = 45） | 不强制（保守档） | 逐条登记见 `docs/code-health/long-function-inventory.md`（§五-3） |
| TODO/FIXME/HACK | 6 | 3 | ≤ 6 | ✅ 达成（均为 `TODO:` 标准标注，已逐条核实） |
| 敏感路径 | 18 文件 + 2 前缀 | 零改动 | 零改动 | ✅ 达成 |

子模块与前端（终态）：

| 范围 | 文件数 | 裸 fmt | 忽略 err | 吞错 | TODO | 未注释导出 | >120 行函数 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `tui/`（Go） | 131 | 0（OSC 52 序列已标 `scan-ignore`） | 0 | 0 | 0（「TODO panel」为面板名，M40 已排除） | 0 | 10 |
| `desktop/`（Go 后端） | 22 | 0 | 0 | 0 | 0 | 0 | 0 |
| `desktop/frontend`（TS/TSX） | 102 | — | — | 0（空 catch 0） | 0 | 73 → **0** | 0（>400 行文件 5 个，见 §五） |

前端另核：`console.log/debug` 调试残留 **0**、`@ts-ignore` **0**、`any` 7 处（全为 Wails `window` 桥接，合理）、`eslint-disable` 3 处（均带意图说明）。M54 新增 ProjectTree 组件测试（5 例）与 ChatView 纯函数测试（17 例），M57 再补 ChatView 组件级测试（4 例），M58 补 Composer 测试（6 例），M59 补 SettingsPanel 测试（5 例），前端用例 105 → **142**。

## 三、分阶段成果

- **阶段 1 内核层（M01–M05）**：agentcore / graph / doomloop / session / store 全覆盖，横切卡收敛裸 fmt、忽略 err、未注释导出。
- **阶段 2 领域层（M06–M15）**：domains 下 15 个业务模块（撰写、新颖性、侵权、证据、对照图、工作流等）逐模块审阅，共约 370 文件。
- **阶段 3 基础设施层（M16–M22）**：knowledge / memory / retrieval / disclosure / guardrails / pkg 等，其中 guardrails 红线条目单独处置。
- **阶段 4 接口层（M23–M27）**：a2a / a2ui / agui / acp / server / mcp / bootstrap。协议面卡仅做注释治理，事件与协议结构零改动（含 `config_trust` 红线）。
- **阶段 5 工具/入口（M28–M31）**：tools 三大族（browser / desktop / 内置工具）与 `cmd/mady`；`inputSchema` 零改动。
- **阶段 6 端/测试/脚本（M32–M36）**：evaluate / example / integration / scripts；`tui/` 基础层与应用层；`desktop/` Go 后端与 React 前端；本报告。
- **阶段 7 专项收敛（M37-1～M37-6、M38、M39、M40、M41）**：按本报告遗留清单分 6 批处理静默吞错（domains / 协议面 / 内核 / tools / cmd / 周边目录），共 307 处站点，逐处判定「有意降级（补意图注释）」或「疑似漏错（不改代码、登记）」；M38 单独修 `tui/` 宽字符渲染口径，收口 17 个预存失败用例；M39 收敛被忽略 error 23 → 8，M40 修正扫描口径并补导出注释，M41 将 10 处可疑忽略逐条判定清零。
- **阶段 9 协议面专项（M66–M70）**：阶段 4 对协议面只做了注释治理、未建契约护栏，故单独立项。M66 已完成契约基线取证（`docs/code-health/protocol-contract-baseline.md`：七层帧结构 / 事件类型 / 错误码 / SSE-WS 格式 / instrumentation + 实测覆盖矩阵，`bootstrap` 3.4% 为最弱）；M67 因「单卡 3-5 文件」约束拆为两卡：M67-1（AGUI + A2UI golden fixture）锁定 23 个事件名、22 类事件 JSON 字段、`msg_`/`thinking_` ID 规则、SSE 帧格式与 A2UI 版本/四信封/JSONL 分帧；M67-2（A2A + ACP + server + MCP）锁定 A2A 端点与 HTTP/WS 各 8 方法、10 个错误码、6 个任务状态与 SSE 帧（无 `event:` 行），ACP 11 方法、5 个错误码文案与 `session/update` 通知帧，server 8 个流 schema 常量、39 条路由表与 `/api/v1` 版本化响应头，MCP 11 个 RPC 方法与 5 个 `mcp_*` 事件名——两卡均已完成；M68 已把 `agui/converter.go` 的 `Converter.Convert`（194 行）按事件族拆为分发 + `convertXxx` 组装函数（帧结构与事件序列零改动，M67-1 fixture 原样通过，`long_funcs` 35 → 34）；M69 已补测弱覆盖层：`bootstrap/agentconfig` 46.2% → 80.8%、`mcp` 72.1% → 74.8%、`bootstrap` 3.4% → 7.5%（PlantaskBridge Replan 闭环），剩余启动装配类覆盖需集成环境；M70 已产出 `docs/protocol-overview.md`（七层地图 + 契约六类 + 变更流程与禁止事项 + 2 处按实现固化偏差）与 `docs/protocol-reference.md`（逐层线协议参考 §1–§7 + fixture 索引 §8），阶段 9 收口。

## 四、未达成指标的原因分析

1. **裸 fmt 66 → 0（M44 口径收口）**：原 65 处全部集中在 `cmd/mady/subcmd`（8 文件），是 CLI 子命令的正常标准输出，非库代码调试残留；基线注记只覆盖 subcmd，漏算了 `cmd/mady/main.go`、`cmd/export-benchmark/main.go` 与 `example/*/main.go`。M44 新增 `docs/cli-output-standards.md` 作为 CLI 输出层的规范家（判定判据、stdout/stderr 通道约定、豁免理由），并让 `scripts/scan_go_smells.py` 把 `cmd/` 与 `package main` 的命中单独计入 `cli_fmt`（当前 179：cmd 96 + example 83），库代码「裸 fmt」指标归 **0**，`--fail` 仍只拦库代码。**指标关闭。**
2. **忽略 err 0 处**（目标 ≤10，已达成并清零）：M39 将 23 处收敛至 8（`mcp` 12 处 Close 合并为 `closePipes` helper、`tools` 8 处 Wait 改为带 `//nolint:gosec // G104: cleanup-only` 的显式忽略、`domains`/`example`/`agentcore` 各 1～2 处补意图注释）；M40 修正扫描口径后，`cmd` 的 11 处 stderr 写入（`_, _ = fmt.Fprintf(stderr, ...)`，进程即将非零退出、无可补救）不再计入，同时新识别 10 处真实可疑忽略（见 §五 P2-9）；M41 逐条判定后 10 → 0——6 处改为 `slog.Warn`（内存汇总写入、案件工作区创建），2 处 `Sscanf` 改 `strconv.Atoi` 显式处理失败，2 处（`crypto/rand.Read`、BOM `Discard`）按库语义补 `scan-ignore` 豁免。
3. **静默吞错 0 处**：M37 六卡按目录分批（domains 61 / 协议面 58 / 内核 34 / tools 61 / cmd 43 / 周边 50）逐处判定并补注 307 处；剩余 1 处 `agentcore/permission/rule.go`（敏感路径）由 M43 补注 fail-open 边界；M44 复核发现 M42 改写后的 `agentcore/plantask/extension.go` 映射分支被计入 1 处（M43 记录称「1 → 0」仅覆盖 `rule.go`），补一行意图注释后归零——零行为变更。

## 五、遗留清单（按建议优先级）

**P0 行为缺陷**：0 项。

**P1 建议专项（不改行为，但影响可维护性）**

1. **静默吞错专项收敛**：✅ 已完成（M37-1～M37-6，307 处补注、零行为变更）。剩 1 处已由 M43 确认：
   - `agentcore/permission/rule.go:97`——敏感路径（permission 前缀）。M43 取证确认为**理论风险、当前不可利用**（生产规则均工具粒度、不解析 args），仅补意图注释说明 fail-open 边界，零行为变更；经用户人工确认后放行。M44 复核补注 `agentcore/plantask/extension.go` 的哨兵映射分支，全仓吞错归 **0**。
2. **忽略 err 收敛**：✅ 已完成（M39 23 → 8，M40 口径修正后 10，M41 逐条判定后 **0**）。M39 处理的 `mcp`/`tools`/`domains`/`example`/`agentcore` 均归零；M40 识别出的 10 处真实可疑忽略由 M41 收口：6 处补 `slog.Warn`、2 处改 `strconv.Atoi`、2 处按库语义 `scan-ignore` 豁免（见 P2-9）。
3. **长函数**（>120 行）44 处：根模块 34、`tui/` 10。M45 已逐条登记到 `docs/code-health/long-function-inventory.md`（分级 P1 5 / P2 15 / P3 24，含拆分方向与保留理由；14 处带 `//nolint:gocognit` 显式豁免）。口径修正：扫描器原 `i - func_start > 120` 漏计「恰好 121 行」的函数，M45 改为含声明行计数（34 → 35）；M68 拆分 `agui/converter.go` 的 `Converter.Convert` 后根模块 35 → 34，基线同步棘轮下压。
4. **大文件拆解候选**（保守档仅记录）：
   - Go：~~`domains/figure/generate/check.go` 978 行~~ ✅ M47 已拆分（`check.go` 174 + `check_scan.go` 489 + `check_path.go` 213 + `check_rules.go` 123，纯搬迁零行为变更）；~~`example/cli-chat/main.go` 970 行~~ ✅ M49 已拆分（`main.go` 440 保留主循环与 hotkeyRouter + `config.go` 272 + `thinking.go` 150 + `helpers.go` 88 + `tools.go` 71）；~~`domains/evidence/statutory.go` 848 行~~ ✅ M50 已拆分（`statutory.go` 439 模型与编排 + `statutory_kinds.go` 333 种类贡献 + `statutory_acquisition.go` 64 取得方式贡献）；~~`domains/figure/generate/dot.go` 822 行~~ ✅ M51 已拆分（`dot.go` 371 类型/选项/校验 + `dot_build.go` 287 四类图构建 + `dot_render.go` 92 页面与头部 + `dot_templates.go` 85 内置模板）；~~`domains/figure/generate/tool.go` 773 行~~ ✅ M52 已拆分（`tool.go` 196 工具定义与入口 + `tool_schema.go` 258 入参 schema + `tool_run.go` 218 执行与渲染 + `tool_path.go` 114 输出路径解析）；~~`evaluate/benchmark/patent_exam_real_a22.go` 619 行~~ → M53 决定**保留单文件**（该目录惯例是「一法条一文件」，a2/a26/a31/a33/r42 均同构；按年份拆会破坏惯例并需引入聚合变量），仅在数据集内补 2007–2011 / 2014–2015 / 2016–2019 三段年份注释，零结构改动
   - 前端：~~`src/components/ProjectTree.tsx` 879 行~~ ✅ M55 已拆分（`ProjectTree.tsx` 593 主组件 + `projectTree/tree.ts` 177 节点模型与 reducer + `projectTree/TreeNodeRow.tsx` 121 行渲染），M54 的 5 例组件测试继续兜底；~~`src/components/chat/ChatView.tsx` 822 行~~ ✅ M56 已拆分（`ChatView.tsx` 743 主组件与自动滚动 hook + `chat/transcript.ts` 95 历史映射/格式化/高度估算，17 例测试随函数迁至新模块且不再需要 pdfjs mock）；~~`src/lib/backend.ts` 569 行~~ ✅ M60 已拆分（`backend.ts` 15 行聚合入口 + `backend/runtime.ts` 133 声明解析/运行态 + `backend/conversation.ts` 247 会话与记忆 + `backend/workspace.ts` 202 文件/技能/MCP/知识库/项目，导出面不变）；~~`src/components/SettingsPanel.tsx` 448 行~~ ✅ M62 已拆分（`SettingsPanel.tsx` 377 主组件 + `settings/AppearanceSection.tsx` 59 + `settings/AboutSection.tsx` 81，分区改为纯展示组件、状态仍由主组件持有），M64 续拆（「AI 服务」分区迁 `settings/AIServiceSection.tsx` 97，主组件 330 行），M65 再拆（左侧导航迁 `settings/SettingsNav.tsx` 51，主组件 304 行）。

> 前端 5 个大文件至此全部处置：ProjectTree（M55）、ChatView（M56）、backend.ts（M60）、Composer（M61）、SettingsPanel（M62）；M54/M57/M58/M59 的 37 例前端测试为这些拆分提供了护栏。

**P2 一致性（低优先）**

5. 未注释导出：✅ 已清零（M40 补 `domains/patent.go` 的 `BuildProjectAgent` 注释），93 → 0。
6. ~~**`domains/plantask/extension.go` 错误映射过粗**（M37-3 登记）~~ ✅ M42 已修复：新增 `ErrSessionNotFound` 哨兵，仅「未找到」映射为 `ErrNoActiveSession`，IO/权限/解析错误原样上抛（路径原记 `domains/` 有误，实际位于 `agentcore/plantask/`）。
7. ~~前端 Wails 桥接的 `any`~~ ✅ M71 已收口：`window.runtime` / `window.go` / `window.__mady` 三个全局与 `useAutoScroll` 的 virtualizer 参数共 7 处 `any` 全部消除，全局声明集中到 `src/types/wails.d.ts`（`MadyTestHooks` + `Window` 增强），`listenToWailsEvent` 改为泛型载荷 `T`。剩余 1 处 `AguiEventPayload = any` 已由 M72 收口：新增 `src/agui-bridge/payloads.ts` 建立 20 个事件的载荷映射（字段对齐 Go 侧 `json` 标签），`HANDLERS` 改为映射类型使「新增事件忘接线」变成编译错误。**前端 `any` 已归零**。
8. ~~`tui/` TODO 8 处与裸 fmt 1 处为误报~~ ✅ M40 已修扫描器：TODO 仅识别 `TODO:` / `TODO(owner)` 标注形态（`TODO panel` 名词短语不再计入），OSC 52 剪贴板写入用行内 `// scan-ignore: bare_fmt — <原因>` 显式豁免；tui 两项均归零。
9. ~~**10 处真实可疑忽略**（M40 口径修正后暴露，建议另开 fix 卡逐条判定是否应传播）~~ ✅ M41 已逐条判定并清零：
   - `domains/case_ext_register.go:43`、`domains/case_ext_sync.go:117`、`:195`——案件已落库/文档已记录，工作区创建失败不回滚整次操作，改为 `slog.Warn`
   - `memory/extension.go:169`、`:171`——汇总 goroutine 无调用方接收错误，改为逐条 `slog.Warn`，长期记忆丢失不再静默
   - `server/middleware.go:40`——Go 1.24+ `crypto/rand.Read` 不返回错误（失败即 panic），原「拒绝请求」建议不成立，补 `scan-ignore` 说明
   - `tools/browser_agent_browser.go:156`、`tools/browser_lightpanda.go:152`、`domains/claimchart/tool.go:427`、`knowledge/fileindex/reader_spreadsheet.go:59`——`fmt.Sscanf` 两处改 `strconv.Atoi` 显式处理失败；会话 close 与 BOM `Discard` 两处为最佳努力清理，补 `scan-ignore` 说明

## 六、验证方式与门禁记录

每卡执行（根模块）：

```bash
make check-arch doc-check verify-layers debt-check
go build ./... && go vet ./... && golangci-lint run ./...
go test -race ./...
```

子模块与前端：

```bash
cd tui && go build ./... && go vet ./... && golangci-lint run ./... && go test -race ./...
cd desktop && go build ./... && go vet ./... && golangci-lint run ./... && go test -race ./...
cd desktop/frontend && npx tsc --noEmit && npx eslint src/ && npm test
```

终态事实源校验（2026-11-28 复核）：`src=1236 test=644`、子命令 17、manifests 3、`LAYERS.md` 与 `tui/` 同步、事实源 61 条。前端 `tsc` 0 error、`eslint` 0 issue、vitest **142** 用例全绿（91 + 51，含 M54/M57/M58/M59 新增 37 例）。

**预存失败收口情况**：`tui/` 的 17 个渲染失败用例已由 M38 全部收口（`cd tui && go test -race -count=1 ./...` 全绿）；根模块唯一残留失败为 `knowledge/sqlite` 的 `TestSearchLaws`，依赖本机 `~/.mady/knowledge/laws-full.db`，属环境依赖、与本轮审阅无关。

阶段 7 附加验证口径（吞错专项每卡执行）：

```bash
python3 scripts/scan_go_smells.py <模块目录>   # 吞错计数归零
git diff --stat                                  # deletions 必须为 0，新增行全为注释
```

## 七、后续建议

1. ~~一张「吞错收敛」专项卡（M37）~~ ✅ 已完成：307 处补注，吞错 380 → 1；剩余 1 处敏感路径待人工确认（§五-1）。
2. ~~一张「宽字符渲染口径」fix 卡（M38）~~ ✅ 已完成：边框字形固定单列 + 测试入口固定口径，`tui/` 全量全绿。
3. ~~忽略 err 收敛小卡（M39）~~ ✅ 已完成：23 → 8（M40 口径修正后 10 处真实忽略）。
3b. ~~**可疑忽略 fix 卡**（建议下一张）~~ ✅ 已完成（M41）：10 处逐条判定并清零，6 处改为 `slog.Warn`、2 处改 `strconv.Atoi`、2 处按库语义 `scan-ignore` 豁免。
4. ~~大文件拆解另立计划~~ ✅ 已由阶段 8（M47–M65）执行完毕：Go 5 个候选拆分 / 1 个保留（M53），前端 5 个候选全部拆分；拆分前先补特征测试（M48/M54/M57–M59），零行为变更。
5. ~~把本报告 §二 的指标表固化进 CI 趋势看板，防止指标反弹（重点防吞错注释被回退、未注释导出新增）~~ ✅ M46 已完成：基线单源 `docs/code-health/metrics-baseline.json`，`scripts/check_metrics_trend.py` 高于基线即失败、低于基线可棘轮下压、基线缺失 fail-closed；接入 `make verify` 与 CI `metrics-trend` job，并有 3 个红绿负控制用例（`scripts/gatechecks/`）。
6. ~~依赖升级另立一卡~~ ✅ M63 已完成：Go 根模块 6 项直接依赖升到同主版本最新 patch/minor，前端 26 项在 semver 范围内升级，三模块与前端门禁全绿；`chromedp`（0.x minor 破坏性区间）与 `onnxruntime_purego`（与本地原生库耦合）登记暂缓，大版本迁移（React 19 / vite 8 / vitest 5 / TS 7）不在本轮范围。同时补齐 `.github/dependabot.yml` 的前端 `npm` 生态条目——此前只有根模块 gomod 被每周扫描。
7. ~~**协议面契约无护栏**~~ ✅ 阶段 9 已收口（M66–M70）。M66 完成七层契约基线取证；M67 起把事件类型、JSON-RPC 方法、错误码、SSE/WS 帧固化为 golden fixture，任何契约漂移测试即红；M70 补齐按层协议文档与契约变更流程。协议层内部重构（M68）与弱覆盖补测（M69）不得改变任何帧结构。
8. ~~**工具 `inputSchema` 无护栏**~~ ✅ M74 已收口：`tools/schema_contract_test.go` + `tools/testdata/tool_schemas.golden.json` 固化 31 个内置工具的 `Parameters`，字段改名/类型变化/required 增删即红；更新入口 `go test ./tools -run TestContractToolSchemas -count=1 -args -update`。
9. ~~**TUI 关键卡片无整卡渲染护栏**~~ ✅ M75 已收口：`tui/component/render_golden_test.go` + `testdata/render_golden.txt` 固化 7 个工具卡/审批卡整卡输出（剥 ANSI、裁行尾空格），并断言每行宽度不超目标宽度；更新入口 `go test ./component -run TestRenderGolden -count=1 -args -update`。
10. ~~**TUI 对话转录无整段渲染护栏**~~ ✅ M76 已收口：`tui/chat/render_golden_test.go` + `testdata/render_golden.txt` 固化 7 个转录用例（CJK 换行、表格列宽、流式光标、工具行、窄宽代码块、错误信息），同样断言每行宽度与单换行结尾；更新入口 `go test ./chat -run TestRenderGoldenTranscript -count=1 -args -update`。
11. ~~**输入区与任务面板无渲染护栏**~~ ✅ M77 已收口：复用 `component/golden_test.go` 基建新增 5 个 golden（输入区占位符/CJK 多行/长折行，任务面板四态混合/窄宽截断）。遗留 P2：M75 的 `render_golden_test.go` 与既有 `TestGoldenToolCard`/`TestGoldenApprovalCard` 重叠，待清理卡合并。
