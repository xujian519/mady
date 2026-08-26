# Mady 全仓代码审阅日卡计划（保守档）

- 计划日期：2026-08-25
- 执行深度：**保守档**（用户确认）——只做无行为变化的清理，不拆大文件、不动架构、不升级依赖
- 范围：根模块 Go 源文件（935 个非测试 `.go`，约 28 万行）+ `tui/`、`desktop/` 独立子模块
- 周期：约 6–8 周，约 30 张日卡，每天一张（人工触发）
- 执行技能：`mady-daily-code-review`（本地 `.claude/skills/`），本计划文档为"家"（权威定义处）

## 一、目标与成功标准

**目标**：对全部代码做一轮全覆盖的「审阅 + 精炼」。审阅先行、精炼后置，**只做无行为变化的清理**，保持全部功能不变。

**成功标准（可量化验收）**：
1. **审阅覆盖率 100%**：全部日卡完成，本进度表可逐卡追溯
2. **每卡门禁全绿**：`make verify`（lint + check-arch + doc-check + verify-layers + build + test-race）；涉 `tui/`/`desktop/` 单独 `cd <模块> && go build ./... && go test -race ./...`；涉协议面卡过全量 `go test ./...`
3. **行为不变**：全部提交为 `refactor`/`chore`/`docs` 类，无 `feat`/`fix` 混入；全量测试保持全绿
4. **指标目标**（基线见 §六）：裸 `fmt` 库代码调试残留 66 处 → <30；被忽略 error 24 处 → ≤10；静默吞错无注释 380 处显著下降（补意图注释）；未注释包级导出 93 处 → ≤20；>120 行单函数 30 处显著下降
5. **产出终审报告** `docs/code-review-report.md`：指标对比、遗留清单、未来专项建议（含大文件拆解候选）

**明确排除（保守档边界）**：不拆大文件（仅记录"待拆建议"）、不动架构分层、不升级依赖、不改工具 `inputSchema`（破坏 LLM replay fixture）、事件面/协议面零改动（A2A/A2UI/AGUI/ACP/Server/MCP）、`tui/`/`desktop/` 只做行为不变清理。

## 二、阶段划分

| 阶段 | 周期 | 内容 | 卡数 |
|---|---|---|---|
| 阶段 1 | 第 1–2 周 | 内核层：agentcore / graph / doomloop / session / store | 5 |
| 阶段 2 | 第 3–6 周 | 领域层：domains 各业务模块（最大，289 文件/52.6K 行） | 10 |
| 阶段 3 | 第 5–7 周 | 基础设施层：knowledge / memory / retrieval / disclosure / guardrails / pkg 等 | 7 |
| 阶段 4 | 第 7 周 | 接口层：a2a / a2ui / agui / acp / server / mcp / bootstrap | 5 |
| 阶段 5 | 第 8 周 | 工具/入口：tools / cmd/mady | 5 |
| 阶段 6 | 第 8–9 周 | 端/测试/脚本：evaluate / example / scripts / tui / desktop + 终审报告 | 5 |

**合计约 37 张日卡**，每天一张（人工触发）；每周五留 30 分钟周复盘（卡数、指标变化、下周取卡）。

## 三、每日执行格式（一张"日卡" ≈ 2–3 小时）

1. **取卡**：按进度表优先级取未完成卡（大文件多、主链路模块优先）
2. **审阅**：通读该模块全部文件，按 §四 Go 审阅清单逐项核查，产出审阅记录（发现分级：P0 行为缺陷 / P1 复杂度 / P2 一致性 / P3 风格）
3. **精炼**：仅执行无行为变化的清理——死代码/孤儿导出删除、命名一致性、重复逻辑合并、错误处理收窄（`_ = <expr>` → 检查/注释）、注释治理（删赘注/补必要意图注释）、内联简化。调用 `code-simplifier` 技能执行
4. **验证**：`make verify` 全绿（lint/check-arch/doc-check/verify-layers/build/test-race）；涉 `tui/`/`desktop/` 单独验证；涉协议面跑 `go test ./...` 全绿
5. **提交**：Conventional Commits（`refactor(<scope>): …`），一个关注点一个提交；**P0 发现不混入精炼提交**，记录后另开 fix 卡；AI 参与追加 `Co-authored-by` + `go run scripts/changelog/main.go` 记录 `docs/decisions/ai-changelog/`
6. **记录**：更新本进度表（状态 + 发现摘要 + 指标计数）

**Go 审阅清单（每卡必查，见 §四）**。

## 四、Mady Go 审阅清单（每卡必查）

> 面向 Go + Mady 项目规范（`docs/GO-DEVELOPMENT-STANDARDS.md` §0 硬约束、AGENTS.md 规范元规则、敏感路径权威源 `scripts/check-sensitive-paths.sh`）。

- [ ] 1. **死代码 / 未使用导出 / 孤儿文件**（`golangci-lint` staticcheck U1000 + 零消费导出 grep；注意区分"零消费但属公共 API"的保留导出）
- [ ] 2. **被忽略的 error**（`_ = <expr>`、`if x, _ := ...`）——GO-STANDARDS §0.1#1「所有 error 必须检查」
- [ ] 3. **静默吞错无意图注释**（`if err != nil { return zero }` 降级但无注释）——补 fail-safe 意图注释；`return err` 传播不算
- [ ] 4. **裸 `fmt.Print`/`fmt.Println` 调试残留**（库代码应走结构化 `log`/`tracing`/`pkg/i18n`；main 包 CLI 输出豁免）
- [ ] 5. **嵌套内联 + 超长函数 + 过深嵌套**（单函数 >120 行、高圈复杂度）——只记录待拆建议，不拆
- [ ] 6. **重复代码可提取、冗余抽象**；禁 `common/`/`utils/`/`base/` 通用包（doc-check 包名守卫）
- [ ] 7. **命名与注释**（`xxxMu` RWMutex 命名、导出符号必须中文注释、三组 import 排序、错误信息小写开头不加标点）
- [ ] 8. **项目规范一致性**（分层单向依赖、禁 dot import、首个参数 `ctx context.Context`、goroutine 生命周期）
- [ ] 9. **TODO/FIXME/HACK 标注核实**（逐条判断是否还成立；已解决的删除）
- [ ] 10. **测试与注释是否随精炼同步更新**（改签名/删符号须同步测试）
- [ ] 11. **Mady 特有红线（横切）**：敏感路径改动禁止（`check-sensitive-paths.sh` 权威源 18 文件 + 2 前缀）；工具 `inputSchema`/事件面/协议面零改动；`go.work` 三模块独立验证

## 五、日卡清单

### 阶段 1 — 内核层（W1–W2）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M01 | agentcore/ 核心（agent_run/reasoning_*/skill_extension/iface） | 130 文件/21.4K 行 | ✅ 2026-08-25 |
| M02 | agentcore/ 子包（concurrency/evidence/filecheckpoint/permission/planmode/tasklist/worker） | 同上 | ✅ 2026-08-26 |
| M03 | agentcore/ manifests + 其余 + graph/ | graph 9 文件/1.8K 行 | ⬜ |
| M04 | doomloop/ + session/ + store/ | 3 + 9 + 3 文件 | ⬜ |
| M05 | 内核层横切 | 裸 fmt/忽略 err/未注释导出收敛 | ⬜ |

### 阶段 2 — 领域层（W3–W6）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M06 | domains/claimdrafting + config | 撰写模块 | ⬜ |
| M07 | domains/specdrafting | 说明书撰写 | ⬜ |
| M08 | domains/enablement + inventiveness | 26.3 + 创造性图引擎 | ⬜ |
| M09 | domains/novelty + infringement | 新颖性 + 侵权比对 | ⬜ |
| M10 | domains/evidence + ipc + checker | 证据规则 + 分类 + 撰写检查 | ⬜ |
| M11 | domains/claimchart + provisions + provenance | 对照图 + 法条 + 溯源 | ⬜ |
| M12 | domains/workflows + plantask + workercontract | 领域工作流 | ⬜ |
| M13 | domains/rules + reasoning（含 sqlite/wiring） | 规则引擎 + 推理 | ⬜ |
| M14 | domains/writing + doctmpl | 撰写质量 + 模板 | ⬜ |
| M15 | domains 根包 + sqlite + case_* | 根包 289 文件剩余 | ⬜ |

### 阶段 3 — 基础设施层（W5–W7）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M16 | knowledge/（fileindex/graph/knowledgeinit/loader/risk/sqlite） | 53 文件/9.7K 行 | ⬜ |
| M17 | memory/ + compiler | 25 文件/4.9K 行 | ⬜ |
| M18 | retrieval/（domain/sqlite/nuopatent/browser） | 19 文件/3.7K 行 | ⬜ |
| M19 | disclosure/ | 20 文件/3.9K 行 | ⬜ |
| M20 | guardrails/（红线条目单独） | 23 文件/3.1K 行 | ⬜ |
| M21 | pkg/ + provider/ + prompt/ + skill/ + fuzzy/ + intent/ + tracing/ + psychological/ | 小模块合卡 | ⬜ |
| M22 | 基础设施层横切 | 裸 fmt/忽略 err/未注释导出收敛 | ⬜ |

### 阶段 4 — 接口层（W7）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M23 | a2a/ | 26 文件/5.4K 行（协议面红线） | ⬜ |
| M24 | a2ui/ + agui/ | 20 文件/3.2K 行（协议面红线） | ⬜ |
| M25 | acp/ + server/ | 32 文件/6.7K 行（含红线） | ⬜ |
| M26 | mcp/ | 25 文件/4.6K 行（含 config_trust 红线） | ⬜ |
| M27 | bootstrap/ | 16 文件/2.8K 行 | ⬜ |

### 阶段 5 — 工具/入口（W8）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M28 | tools/ browser_* + browserproviders | 85 文件/18.8K 行拆 3 卡 | ⬜ |
| M29 | tools/ desktop + computer_use* | 同上 | ⬜ |
| M30 | tools/ 内置工具族 + tools.go 注册 | 同上 | ⬜ |
| M31 | cmd/mady/ | 42 文件/7.7K 行 | ⬜ |

### 阶段 6 — 端/测试/脚本（W8–W9）

| 卡 | 模块 | 规模/热点 | 状态 |
|---|---|---|---|
| M32 | evaluate/ + example/ + integration/ + scripts/ | 32+11+脚本 | ⬜ |
| M33 | tui/（独立子模块）A | 分层 Elm 架构 | ⬜ |
| M34 | tui/（独立子模块）B | 同上 | ⬜ |
| M35 | desktop/（独立子模块 Go 后端） | Wails | ⬜ |
| M36 | desktop/（React 前端）+ 终审报告 | docs/code-review-report.md | ⬜ |

## 六、基线（2026-08-25 实测）

来源：`scripts/scan_go_smells.py`（根模块非测试 Go 文件，默认排除 `_test.go`）。
基线为扫描工具 2026-08-25 修复后实测（fixme 统计注释行、静默吞错含 `if err := ...; err != nil` 形式）。

| 指标 | 基线值 | 目标 | 备注 |
|---|---|---|---|
| 根模块非测试 `.go` 文件 | 929 | — | CLAUDE.md 的 1093 含 tui/desktop 与未跟踪文件 |
| 裸 `fmt.Print*`（库代码调试残留） | 66 | <30 | main 包 CLI 输出豁免；cmd 65 处为 subcmd 正常输出 |
| 被忽略的 error（`_ = <expr>` 等） | 24 | ≤10 | GO-STANDARDS §0.1#1 |
| 静默吞错无注释（降级无意图注释） | 380 | 显著下降 | 含 `if err := ...` 形式；补 fail-safe 意图注释 |
| 未注释包级导出（函数/类型/变量） | 93 | ≤20 | tools 74 为主；方法不计 |
| >120 行单函数 | 30 | 不强制下降（保守档） | 记录待拆建议 |
| TODO/FIXME/HACK | 6 | ≤6 | 真实标注：psychological_config.go 3 + psychological 3；逐条核实 |
| 敏感路径 | 18 文件 + 2 前缀 | 零改动 | 权威源 `check-sensitive-paths.sh` |

**更新基线**：每张卡验证后跑 `python3 scripts/scan_go_smells.py`，把更新后计数追加到进度表对应卡行。

## 七、风险与护栏

- **inputSchema 红线**：任何工具 `inputSchema` 改动都会使 LLM replay fixture 失配——精炼时禁止改动，说明性文字只放工具顶层 description
- **事件面/协议面红线**：AgentEvent/gateway frames/A2A/A2UI/AGUI/ACP/Server/MCP 事件零改动
- **敏感路径红线**：`scripts/check-sensitive-paths.sh` 权威源（18 文件 + 2 前缀）；AI 参与 + 敏感路径组合触发 pre-commit gate 阻塞；涉及须人工审阅
- **P0 分流**：审阅发现的行为缺陷只登记，单独开 fix 卡处理，不混入精炼提交
- **提交纪律**：一个关注点一个提交；每日至少 1 个 commit；禁一次性大杂烩提交；Conventional Commits（`refactor(<scope>): …`）
- **AI 参与记录**：每卡提交追加 `Co-authored-by` + `go run scripts/changelog/main.go` 追加 `docs/decisions/ai-changelog/`
- **多模块验证**：`tui/`、`desktop/` 为独立 `go.mod`，改动须各自 `cd <模块> && go build ./... && go test -race ./...`
- **周复盘**：每周五核对指标计数变化，异常（门禁红、指标反弹）当周修复

## 八、执行方式

- **触发**：用户每天发「执行今天的日卡」/「跑今天的卡」，由本地技能 `mady-daily-code-review` 按本进度表取未完成卡执行
- **人工兜底**：无自动调度；每日卡由用户确认开始，P0 与敏感路径变更必须人工审阅

## 九、日卡执行记录

### M01（2026-08-25）agentcore/ 核心 ✅

- **审阅范围**：agentcore 顶层 74 非测试 `.go` + `iface/` 2 文件（子包 `concurrency/evidence/filecheckpoint/permission/planmode/tasklist/worker/manifests` 留 M02）
- **发现分级**：
  - P0 行为缺陷：0 项
  - P1 复杂度：2 项（`ToolDomains` 未接线占位，注释已说明待另立 spec，保留；`executeToolCalls` 136 行待拆，保守档不拆）
  - P2 一致性：2 项（`reflection.go` config 无锁读取与 SetConfig 写不一致——涉并发语义，保守档记录不处理；`appendLifecycleHook` 直通包装器可收敛，但调用点含 `handoff.go` 敏感路径，跳过）
  - P3 风格：13 项已收敛（见精炼）
- **精炼**：3 个 refactor 提交，全部无行为变化
  - `7de0d28` 收敛重复逻辑与死分支（messagesReadOnly 委托 / compressionProvider 死分支 / 温度冗余条件 / sortedSegments 提取）
  - `1903a93` 库代码日志统一走 slog（event_logger/pubsub/lifecycle）
  - `db24461` 补 fail-safe 降级与导出注释、修正 config 默认注释
- **门禁**：`make verify` 全绿（lint/check-arch/doc-check/verify-layers/build/test-race，含 tui/desktop 三模块）
- **指标重扫**（`python3 scripts/scan_go_smells.py agentcore agentcore/iface`，文件 132）：
  - 静默吞错无注释：35 → **29**
  - 未注释导出符号：1 → **0**
  - 被忽略 error：2 处（`_ = failLoop`）已全部补意图注释（扫描器按 `_ =` 模式机械计数不变）
  - 裸 fmt / TODO：0；>120 行单函数：2（记录待拆，不拆）

### M02（2026-08-26）agentcore/ 子包 ✅

- **审阅范围**：`concurrency`（1）/ `evidence`（7）/ `filecheckpoint`（3）/ `permission`（敏感路径前缀，按规范跳过只登记）/ `planmode`（3）/ `tasklist`（8）/ `worker`（7）非测试 `.go`
- **发现分级**：
  - P0 行为缺陷：0 项
  - P2 一致性：tasklist Store/FileStore 方法重复双行英文注释（生成残留）；evidence Ledger 只读查询用 `Lock()` 与 ledger.go 读写锁语义不一致
  - P3 风格：filecheckpoint 循环尾冗余 `continue`、手写前缀比较；planmode Decide bash 嵌套 if、stripQuoted inSingle 冗余分支；evidence commandMatches 相等判断被 HasPrefix 覆盖；worker executor.go 末尾冗余 `var _` 编译时验证
  - 登记不处理：worker 包 `RegisterDefaultWorkers`/`EnsureWorker`/`Catalog.Verify`/`Registry.RegisterOrUpdate`/`IsActivated`/`VerifyAll` 非测试零消费，属公共 API 按规范保留
- **精炼**：3 个 refactor 提交，全部无行为变化
  - `1df2804` tasklist 去除重复双行注释
  - `d16af7d` evidence Ledger 只读查询 RLock + 冗余匹配条件
  - `c67822d` filecheckpoint/planmode/worker 冗余代码清理
- **门禁**：`make verify` 全绿（lint/check-arch/doc-check/verify-layers/build/test-race，含 tui/desktop 三模块）
- **指标重扫**（全仓）：裸 fmt 66 / 忽略 err 24 / 吞错无注释 374 / TODO 6 / 未注释导出 92 / >120 行函数 30——与 M01 后基线持平（本卡改动均为注释与锁语义收敛，不触发扫描器计数变化）
