# Mady 全仓代码审阅终审报告

- 计划文档：`docs/code-review-plan.md`
- 计划启动：2026-08-25；审阅收官：2026-11-13
- 执行档位：**保守档**（只做无行为变化的清理；不拆大文件、不动架构、不升级依赖）
- 覆盖范围：根模块 Go、`tui/` 独立子模块（Go）、`desktop/` 独立子模块（Go 后端 + React/TS 前端）
- 日卡：M01–M36 + 阶段 7 专项（M37-1～M37-6、M38、M39、M40、M41）全部完成

## 一、结论摘要

1. **审阅覆盖率 100%**：M01–M36 与阶段 7 专项（M37-1～M37-6 吞错专项、M38 宽字符口径 fix、M39 忽略 error 收敛、M40 口径修正 + 导出补注、M41 可疑忽略 fix）逐卡完成，进度表可追溯，无跳卡、无延期遗留整卡。
2. **行为不变**：阶段 1–6 全部提交为 `refactor` / `docs` 类型，未混入 `feat` / `fix`；阶段 7 的 `fix` 卡仅 M38（渲染口径，用户批准）与 M41（新增告警观测点 + 等价解析改写，用户批准），其余为 `refactor`（零行为变更）。每卡跑 build / vet / lint / arch / doc / layers / debt / test 门禁。
3. **红线零触碰**：敏感路径权威源（18 文件 + 2 前缀）零改动（M37-3 唯一命中站点按规范跳过、仅登记）；A2A/A2UI/AGUI/ACP/Server/MCP 协议面与工具 `inputSchema` 零改动。
4. **四项指标中「未注释导出」93 → 0、「静默吞错」380 → 1、「被忽略 error」24 → 0 三项达标**；「裸 fmt」未达计划目标，原因见 §四。
5. **发现并登记的真 P0 行为缺陷：0 项**。审阅中登记的复杂度问题（长函数、大文件）按保守档只记录不拆，清单见 §五。

## 二、指标对比（`scripts/scan_go_smells.py`）

根模块（非测试 Go 文件，2026-11-13 实测；扫描器口径 1050 个 Go 源文件 / 31 目录）：

| 指标 | 基线 2026-08-25 | 终态 2026-11-13 | 计划目标 | 结论 |
| --- | --- | --- | --- | --- |
| 裸 `fmt.Print*`（库代码调试残留） | 66 | 65 | < 30 | ❌ 未达成（见 §四-1） |
| 被忽略的 error（`_ = <expr>` 等） | 24 | **0** | ≤ 10 | ✅ 达成（M39 收敛 + M40 口径修正 + M41 逐条判定，见 §四-2） |
| 静默吞错无注释 | 380 | **1** | 显著下降 | ✅ 下降 99.7%（见 §四-3） |
| 未注释包级导出 | 93 | **0** | ≤ 20 | ✅ 达成 |
| >120 行单函数 | 30 | 34 | 不强制（保守档） | 记录（见 §五） |
| TODO/FIXME/HACK | 6 | 3 | ≤ 6 | ✅ 达成（均为 `TODO:` 标准标注，已逐条核实） |
| 敏感路径 | 18 文件 + 2 前缀 | 零改动 | 零改动 | ✅ 达成 |

子模块与前端（终态）：

| 范围 | 文件数 | 裸 fmt | 忽略 err | 吞错 | TODO | 未注释导出 | >120 行函数 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `tui/`（Go） | 131 | 0（OSC 52 序列已标 `scan-ignore`） | 0 | 0 | 0（「TODO panel」为面板名，M40 已排除） | 0 | 10 |
| `desktop/`（Go 后端） | 22 | 0 | 0 | 0 | 0 | 0 | 0 |
| `desktop/frontend`（TS/TSX） | 102 | — | — | 0（空 catch 0） | 0 | 73 → **0** | 0（>400 行文件 5 个，见 §五） |

前端另核：`console.log/debug` 调试残留 **0**、`@ts-ignore` **0**、`any` 7 处（全为 Wails `window` 桥接，合理）、`eslint-disable` 3 处（均带意图说明）。

## 三、分阶段成果

- **阶段 1 内核层（M01–M05）**：agentcore / graph / doomloop / session / store 全覆盖，横切卡收敛裸 fmt、忽略 err、未注释导出。
- **阶段 2 领域层（M06–M15）**：domains 下 15 个业务模块（撰写、新颖性、侵权、证据、对照图、工作流等）逐模块审阅，共约 370 文件。
- **阶段 3 基础设施层（M16–M22）**：knowledge / memory / retrieval / disclosure / guardrails / pkg 等，其中 guardrails 红线条目单独处置。
- **阶段 4 接口层（M23–M27）**：a2a / a2ui / agui / acp / server / mcp / bootstrap。协议面卡仅做注释治理，事件与协议结构零改动（含 `config_trust` 红线）。
- **阶段 5 工具/入口（M28–M31）**：tools 三大族（browser / desktop / 内置工具）与 `cmd/mady`；`inputSchema` 零改动。
- **阶段 6 端/测试/脚本（M32–M36）**：evaluate / example / integration / scripts；`tui/` 基础层与应用层；`desktop/` Go 后端与 React 前端；本报告。
- **阶段 7 专项收敛（M37-1～M37-6、M38、M39、M40、M41）**：按本报告遗留清单分 6 批处理静默吞错（domains / 协议面 / 内核 / tools / cmd / 周边目录），共 307 处站点，逐处判定「有意降级（补意图注释）」或「疑似漏错（不改代码、登记）」；M38 单独修 `tui/` 宽字符渲染口径，收口 17 个预存失败用例；M39 收敛被忽略 error 23 → 8，M40 修正扫描口径并补导出注释，M41 将 10 处可疑忽略逐条判定清零。

## 四、未达成指标的原因分析

1. **裸 fmt 65 处**：全部集中在 `cmd/`（65/65），为 CLI 子命令的正常标准输出（`mady --help`、子命令结果打印），非库代码调试残留。基线注记已说明「cmd 65 处为 subcmd 正常输出」，故此目标自始不可达。**建议**：把 `cmd/**` 在 lint 规则中显式豁免，或制定 CLI 输出规范后关闭该指标。
2. **忽略 err 0 处**（目标 ≤10，已达成并清零）：M39 将 23 处收敛至 8（`mcp` 12 处 Close 合并为 `closePipes` helper、`tools` 8 处 Wait 改为带 `//nolint:gosec // G104: cleanup-only` 的显式忽略、`domains`/`example`/`agentcore` 各 1～2 处补意图注释）；M40 修正扫描口径后，`cmd` 的 11 处 stderr 写入（`_, _ = fmt.Fprintf(stderr, ...)`，进程即将非零退出、无可补救）不再计入，同时新识别 10 处真实可疑忽略（见 §五 P2-9）；M41 逐条判定后 10 → 0——6 处改为 `slog.Warn`（内存汇总写入、案件工作区创建），2 处 `Sscanf` 改 `strconv.Atoi` 显式处理失败，2 处（`crypto/rand.Read`、BOM `Discard`）按库语义补 `scan-ignore` 豁免。
3. **静默吞错 1 处**：M37 六卡按目录分批（domains 61 / 协议面 58 / 内核 34 / tools 61 / cmd 43 / 周边 50）逐处判定并补注 307 处，终态仅剩 `agentcore/permission/rule.go` 1 处——该文件属敏感路径（permission 前缀），按红线未改动，需人工确认后补注或改传播。

## 五、遗留清单（按建议优先级）

**P0 行为缺陷**：0 项。

**P1 建议专项（不改行为，但影响可维护性）**

1. **静默吞错专项收敛**：✅ 已完成（M37-1～M37-6，307 处补注、零行为变更）。剩 1 处待人工处理：
   - `agentcore/permission/rule.go:97`——敏感路径（permission 前缀），按红线未改动；需人工确认该降级是否应改为传播错误。
2. **忽略 err 收敛**：✅ 已完成（M39 23 → 8，M40 口径修正后 10，M41 逐条判定后 **0**）。M39 处理的 `mcp`/`tools`/`domains`/`example`/`agentcore` 均归零；M40 识别出的 10 处真实可疑忽略由 M41 收口：6 处补 `slog.Warn`、2 处改 `strconv.Atoi`、2 处按库语义 `scan-ignore` 豁免（见 P2-9）。
3. **长函数**（>120 行）44 处：根模块 34、`tui/` 10。其中 `tui/` 已逐条登记（如 `layout/flex.go renderHorizontal`、`theme/palette.go BuildPalette`、`overlay_render.go composeOverlays`、`tui_render.go renderFrame`、`component/review_gate.go renderLocked` 等）。
4. **大文件拆解候选**（保守档仅记录）：
   - Go：`domains/figure/generate/check.go` 978 行、`example/cli-chat/main.go` 970、`domains/evidence/statutory.go` 848、`domains/figure/generate/dot.go` 822、`domains/figure/generate/tool.go` 773、`evaluate/benchmark/patent_exam_real_a22.go` 619。
   - 前端：`src/components/ProjectTree.tsx` 879 行、`src/components/chat/ChatView.tsx` 822、`src/lib/backend.ts` 569、`src/components/chat/Composer.tsx` 473、`src/components/SettingsPanel.tsx` 448。

**P2 一致性（低优先）**

5. 未注释导出：✅ 已清零（M40 补 `domains/patent.go` 的 `BuildProjectAgent` 注释），93 → 0。
6. **`domains/plantask/extension.go` 错误映射过粗**（M37-3 登记）：把活动会话读取失败统一映射为 `ErrNoActiveSession`，会掩盖 IO/权限错误；改契约属行为变化，保守档仅加注释，另开 fix 卡。
7. 前端 7 处 `any` 全在 Wails `window.go` / `window.runtime` 桥接，已收敛在 `src/lib/wails.ts` 与 `src/lib/backend.ts`；后续可在 `src/types/wails.d.ts` 用全局声明替代。
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

终态事实源校验：`src=1220 test=632`、子命令 17、manifests 3、`LAYERS.md` 与 `tui/` 同步（131 文件）、事实源 61 条。前端 `tsc` 0 error、`eslint` 0 issue、vitest 105 用例全绿（91 + 14）。

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
4. 大文件拆解另立计划（需行为等价重构 + 完整测试护栏），不建议并入保守档。
5. 把本报告 §二 的指标表固化进 CI 趋势看板，防止指标反弹（重点防吞错注释被回退、未注释导出新增）。
