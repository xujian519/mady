# Mady 全仓代码审阅终审报告

- 计划文档：`docs/code-review-plan.md`
- 计划启动：2026-08-25；审阅收官：2026-11-13
- 执行档位：**保守档**（只做无行为变化的清理；不拆大文件、不动架构、不升级依赖）
- 覆盖范围：根模块 Go、`tui/` 独立子模块（Go）、`desktop/` 独立子模块（Go 后端 + React/TS 前端）
- 日卡：M01–M36 全部完成（36 张）

## 一、结论摘要

1. **审阅覆盖率 100%**：M01–M36 逐卡完成，进度表可追溯，无跳卡、无延期遗留整卡。
2. **行为不变**：全部提交为 `refactor` / `docs` 类型，未混入 `feat` / `fix`；每卡跑 build / vet / lint / arch / doc / layers / debt / test 门禁。
3. **红线零触碰**：敏感路径权威源（18 文件 + 2 前缀）零改动；A2A/A2UI/AGUI/ACP/Server/MCP 协议面与工具 `inputSchema` 零改动。
4. **四项指标中「未注释导出」明确达标**（93 → 1），「静默吞错」显著下降（380 → 306），「裸 fmt」「忽略 err」未达计划目标，原因见 §四。
5. **发现并登记的真 P0 行为缺陷：0 项**。审阅中登记的复杂度问题（长函数、大文件）按保守档只记录不拆，清单见 §五。

## 二、指标对比（`scripts/scan_go_smells.py`）

根模块（非测试 Go 文件，2026-11-13 实测；扫描器口径 1050 个 Go 源文件 / 31 目录）：

| 指标 | 基线 2026-08-25 | 终态 2026-11-13 | 计划目标 | 结论 |
| --- | --- | --- | --- | --- |
| 裸 `fmt.Print*`（库代码调试残留） | 66 | 65 | < 30 | ❌ 未达成（见 §四-1） |
| 被忽略的 error（`_ = <expr>` 等） | 24 | 23 | ≤ 10 | ❌ 未达成（见 §四-2） |
| 静默吞错无注释 | 380 | **306** | 显著下降 | ✅ 下降 19.5%（见 §四-3） |
| 未注释包级导出 | 93 | **1** | ≤ 20 | ✅ 达成 |
| >120 行单函数 | 30 | 34 | 不强制（保守档） | 记录（见 §五） |
| TODO/FIXME/HACK | 6 | 6 | ≤ 6 | ✅ 达成（均为真实标注，已逐条核实） |
| 敏感路径 | 18 文件 + 2 前缀 | 零改动 | 零改动 | ✅ 达成 |

子模块与前端（终态）：

| 范围 | 文件数 | 裸 fmt | 忽略 err | 吞错 | TODO | 未注释导出 | >120 行函数 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `tui/`（Go） | 131 | 1（误报，OSC 52 剪贴板序列） | 0 | 0 | 8（全为「TODO panel」面板名） | 0 | 10 |
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

## 四、未达成指标的原因分析

1. **裸 fmt 65 处**：全部集中在 `cmd/`（65/65），为 CLI 子命令的正常标准输出（`mady --help`、子命令结果打印），非库代码调试残留。基线注记已说明「cmd 65 处为 subcmd 正常输出」，故此目标自始不可达。**建议**：把 `cmd/**` 在 lint 规则中显式豁免，或制定 CLI 输出规范后关闭该指标。
2. **忽略 err 23 处**（目标 ≤10）：分布 tools 8、cmd 8、mcp 3、agentcore 2、domains 1、example 1。多数为 `defer resp.Body.Close()` 与 `os.Remove` 之类的可接受忽略，但工具层 8 处值得逐条确认是否应上报。**建议**：单开一张小卡按目录收敛到 ≤10。
3. **静默吞错 306 处**：逐卡补注按各卡自身范围执行，终态仍有存量集中区：domains 61、cmd 43、a2a 35、agentcore 31、tools 31、mcp 14、example 13、knowledge 12、pkg 10、server 10、bootstrap 9、skill 6、session 3、retrieval 2。这些多为协议/边界处的「降级返回」，但缺意图注释会让后续维护者误判为漏处理错误。

## 五、遗留清单（按建议优先级）

**P0 行为缺陷**：0 项。

**P1 建议专项（不改行为，但影响可维护性）**

1. **静默吞错专项收敛**：按目录分批（建议顺序 domains → a2a → agentcore → tools → cmd），补 fail-safe 意图注释或改为传播错误。总量 306。
2. **忽略 err 收敛**：23 → ≤10，重点在 `tools/`。
3. **长函数**（>120 行）44 处：根模块 34、`tui/` 10。其中 `tui/` 已逐条登记（如 `layout/flex.go renderHorizontal`、`theme/palette.go BuildPalette`、`overlay_render.go composeOverlays`、`tui_render.go renderFrame`、`component/review_gate.go renderLocked` 等）。
4. **大文件拆解候选**（保守档仅记录）：
   - Go：`domains/figure/generate/check.go` 978 行、`example/cli-chat/main.go` 970、`domains/evidence/statutory.go` 848、`domains/figure/generate/dot.go` 822、`domains/figure/generate/tool.go` 773、`evaluate/benchmark/patent_exam_real_a22.go` 619。
   - 前端：`src/components/ProjectTree.tsx` 879 行、`src/components/chat/ChatView.tsx` 822、`src/lib/backend.ts` 569、`src/components/chat/Composer.tsx` 473、`src/components/SettingsPanel.tsx` 448。

**P2 一致性（低优先）**

5. 未注释导出仅剩 1 处：`domains/patent.go` 的 `BuildProjectAgent`。
6. 前端 7 处 `any` 全在 Wails `window.go` / `window.runtime` 桥接，已收敛在 `src/lib/wails.ts` 与 `src/lib/backend.ts`；后续可在 `src/types/wails.d.ts` 用全局声明替代。
7. `tui/` 的 TODO 8 处与裸 fmt 1 处均为误报（面板名 / OSC 52 剪贴板序列），可在扫描器中加白名单以免误读。

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

**已知预存失败**（与本轮审阅无关，非本计划引入）：`tui/` 根包 `lifecycle_test.go` 与 `tui/component` 部分渲染测试因 Unicode 框线字符（`─╭╮╰╯═║█░`）宽度被计算为 2 而失败，stash 后仍复现。建议单开 fix 卡统一宽度口径（`go-runewidth` 配置或测试期望值）。

## 七、后续建议

1. 一张「吞错与忽略 err 收敛」专项卡（M37），按 §五-P1 的目录顺序推进，预计把 306 压到 100 以内。
2. 一张「宽字符渲染口径」fix 卡，解决 `tui/` 预存测试失败，避免长期红灯掩盖新问题。
3. 大文件拆解另立计划（需行为等价重构 + 完整测试护栏），不建议并入保守档。
4. 把本报告 §二 的指标表固化进 CI 趋势看板，防止指标反弹。
