# Mady 债务 ↔ Issue 映射报告

- 生成日期：2026-09-14
- 关联仓库：`xujian519/mady`
- 依据：`docs/` 全量审计文档（90+ 份）+ 代码静态扫描（golangci-lint v2.12.2 / grep / 分块脚本）+ 导入依赖核验
- 状态：**执行中** —— 第 1 批（DEBT-01–05 + DEBT-57）已建 issue #58–#63，其余待建，见 §10.2
- 编号事实源：`docs/plans/debt-issues.json`（唯一事实源，索引区块由 `scripts/debtlink` 生成）
- 范围：P0–P3 全量，含 code-review-plan 日卡跟踪

---

## 一、核心结论（先看这一段）

**1. 仓库当前没有任何工程 issue。**
`gh api` 查得的 57 条实为 dependabot 自动 PR（标题形如 `deps(deps): bump ...`），
`gh issue list` 返回空。项目累积的全部议题、技术债务、异味代码，**只存在于 `docs/` 的
审计报告里，零条进入 issue 跟踪**。这意味着：无负责人、无状态流转、无跨版本追踪、
无外部贡献者可见性。

**2. 文档债务清单已大面积腐化——这是比债务本身更严重的问题。**
本次对文档中的 **6 条 P0** 逐条回代码核验，**5 条已实际修复但文档仍标记为未修复**：

| 文档声称的 P0 | 核验结果 | 证据 |
| --- | --- | --- |
| AGUI 「`agent.Run` 失败不发 `RUN_ERROR`」 | ❌ 已修复 | [agui/handler.go:189](agui/handler.go#L189) 已发射 `EventRunError` |
| `graph/pregel.go` 裸 map 合并、依赖迭代顺序 | ❌ 已修复 | [graph/state_schema.go:101](graph/state_schema.go#L101) `sort.Strings` 显式确定性 |
| `agentcore/pipeline_executor.go` 工具阶段静默跳过 | ❌ 条目失效 | 该文件已不存在，且全仓无 `FailOnUnknown` 符号 |
| goroutine panic 无隔离（executor/pubsub/pregel 三处） | ❌ 已修复 | 三处均衡有 `recover()` + 日志 |
| `guardrails/deferred_persist` 队列未接生产消费端 | ❌ 已修复 | [domains/assemble.go:122](domains/assemble.go#L122) 注入 + [guardrails/levels.go:138](guardrails/levels.go#L138) `Store` |
| LLM 出站零 PII 脱敏（25 条路径） | ⚠️ 已过期，但**仍在 4 处真实漏脱敏** | 主链路已接线（`bootstrap/setup.go:168`），desktop/TUI 绕过见 DEBT-01 |

**若直接照文档建 issue，会产生至少 5 条假 issue。** `docs/` 作为唯一事实源已不可信——
建 issue 必须同步建立「文档条目 → 代码」的核验动作，否则 issue 本身很快就腐化。

> **核验的价值在此显形**：PII 这一条，旧报告说「25 条路径全部未脱敏」（错，已修 21 条），
> 而若直接采信「已修复」也会漏掉真正的漏洞——**desktop 全域与 TUI provider 切换点至今
> 完全无脱敏**。两个方向都错，只有回代码核验才能得到正确结论（见 DEBT-01）。

**3. 去重核验后，有效债务 61 条**：P0 ×5（DEBT-01–05）/ P1 ×26（DEBT-06–31）/
P2 ×12 聚合（DEBT-32–43）/ P3 ×12 聚合（DEBT-44–55）+ 日卡跟踪 ×5（DEBT-56a–56e）
+ 已知限制 ×1（DEBT-57）。

**4. 存在一条贯穿性根因**：`agentcore` 是 God Package（75 个非测试源文件、27 个 interface），
共享值类型（`Message`/`Tool`/`Role`/`Event`/`StateSnapshot`）全部住在最顶层，
导致 **72 个包 import 它**，而抽象契约 `agentcore/iface` 只有 **2 个包**在用且仅有 1 个
interface。CLAUDE.md 宣称的「8 层严格单向依赖」在代码上不成立（12 个基础设施包、45 个
文件反向依赖 agentcore）。**其余架构类 issue 在 DEBT-02 解决前都是治标。**

---

## 二、现状基线

### 2.1 Issue 体系现状

| 项目 | 现状 |
| --- | --- |
| 真实 issue 数 | **0** |
| dependabot PR | 57 条（5 open / 52 closed），命名 `deps(deps): bump ...` |
| 已定义 label | `bug` `documentation` `duplicate` `enhancement` `good first issue` `help wanted` `invalid` `question` `wontfix`（**全部为 GitHub 默认，零自定义**） |
| milestone | 无 |
| 分支保护 | `docs/review/2026-07-21-spec-system-comprehensive-analysis.md:172-190` 标注「待核验」——**若未开启，全部 CI 门禁可被绕过** |

### 2.2 债务承载现状（全部在 docs/）

| 承载位置 | 内容 |
| --- | --- |
| `docs/code-review-plan.md` | 全仓审阅日卡计划，37 卡，M01–M13 已完成，M14–M36 未开始 |
| `docs/roadmap.md` | 模块成熟度 + 封存项（❄️ P3 通过前不启动）+ 停止规则 |
| `docs/known-limitations.md` | 唯一官方已知限制（1 条） |
| `docs/review/` | 51 份报告：full-audit 系列、分阶段审计、模块专项审阅 |
| `docs/reviews/` | 2 份：agent-runtime / tech-debt-fix |
| `docs/code-health/` | 1 份扫描报告 |
| `docs/refactoring/` | CHECKLIST.md（77 项，0/77）+ optimization-plan.md |
| `docs/architecture/` | 编排器重构 roadmap/tasks/checklist（状态 Proposed） |
| `docs/plans/` | 11 份专项计划 |
| `docs/decisions/` | phase4-backlog、各方案文档 |

**共性缺陷**：无状态流转、无负责人、无优先级出口、跨报告重复登记（同一问题在 3–5 份
报告里各写一遍，互不引用），且**已修复条目不会被回写标记**——这正是 §一.2 腐化的机制。

---

## 三、调研方法

| 路径 | 手段 | 产出 |
| --- | --- | --- |
| A 文档层 | 通读 `docs/` 全部审计/计划/决策文档，提取未闭环条目 | 61 条候选 |
| B 代码层 | `golangci-lint`（项目配置 + 严格临时配置）、`grep` 模式扫描、文件/函数分块统计 | 7 类共 ~60 处异味 |
| C 架构层 | `go list` 依赖核验、逐包 import 统计（`--exclude='*_test.go'`）、命名一致性比对 | 12 项结构性问题 |
| **D 核验** | **对候选中的每条 P0/P1 回代码逐条验证** | **推翻 5 条 P0，修正 2 条** |

> 方法说明：A/B/C 由独立调研路径并行产出后合并去重；D 为本报告新增的强制步骤——
> 没有 D，A 的产出不可直接使用（见 §一.2）。

---

## 四、建议的 Label 与编号体系

项目现有 label 全是 GitHub 默认，无一可用。建议先建以下 label（`gh label create`）：

**类型维度**（前缀 `debt/`）

| Label | 含义 | 颜色建议 |
| --- | --- | --- |
| `debt/security` | 安全红线、隐私、权限 | `b60205` |
| `debt/arch` | 分层、依赖方向、模块边界 | `5319e7` |
| `debt/privacy` | PII、数据隔离 | `d93f0b` |
| `debt/quality` | 复杂度、重复、命名、错误处理 | `fbca04` |
| `debt/dead-code` | 死代码、孤儿导出 | `c2e0c6` |
| `debt/test` | 测试缺口、覆盖率 | `0e8a16` |
| `debt/docs` | 文档漂移、规范一致性 | `0075ca` |
| `debt/protocol` | A2A/A2UI/AGUI/ACP/Server/MCP 协议面 | `1d76db` |

**优先级维度**：`P0` `P1` `P2` `P3`（对应现有文档分级，勿另立标准）

**状态维度**：`tracking`（跟踪型，不直接对应一次代码改动）、`blocked`（等待人工决策）

**编号**：本报告 ID `DEBT-01`…`DEBT-57` 写入 issue 正文首行，便于文档 ↔ issue 双向引用。
**编号一经分配不得复用**——`DEBT-06` 曾被误分配给 §9.4 已知限制条目，已改派 `DEBT-57`，
故 `DEBT-57` 在编号序列之外，属正常现象。
不占用 GitHub issue number（那是自动分配的）。

### 4.1 编号 ↔ Issue 索引（由 `scripts/debtlink` 自动生成，勿手工编辑）

> 事实源：`docs/plans/debt-issues.json`。改动后运行
> `go run scripts/debtlink/main.go -apply` 重新生成本区块；`make debt-check` 会校验其时效性。

<!-- debt-index:start -->
| 编号 | 优先级 | 标题 | Issue | 状态 |
| --- | :---: | --- | :---: | :---: |
| `DEBT-01` | P0 | desktop 全域 + TUI provider 切换点绕过 PII 脱敏 | [#58](https://github.com/xujian519/mady/issues/58) | 已建 |
| `DEBT-02` | P0 | agentcore God Package 与分层反向依赖（根因） | [#59](https://github.com/xujian519/mady/issues/59) | 已建 |
| `DEBT-03` | P0 | agentcore/iface 契约降级，文档与代码不符 | [#60](https://github.com/xujian519/mady/issues/60) | 已建 |
| `DEBT-04` | P0 | domains ↔ guardrails 目录级双向依赖 | [#61](https://github.com/xujian519/mady/issues/61) | 已建 |
| `DEBT-05` | P0 | pregel 节点 panic 被降级为日志，图继续执行且不标记失败 | [#62](https://github.com/xujian519/mady/issues/62) | 已建 |
| `DEBT-06` | P1 | domains 根包 3 个别名垫片（142 行纯转发） | — | 待建 |
| `DEBT-07` | P1 | acp → bootstrap 反向依赖 | — | 待建 |
| `DEBT-08` | P1 | graph 内部测试反向依赖 agentcore | — | 待建 |
| `DEBT-09` | P1 | tools/ 与 domains/ 两个巨型根包，职责混杂 | — | 待建 |
| `DEBT-10` | P1 | 三份 provider→默认模型映射，常量散落 8 处 | — | 待建 |
| `DEBT-11` | P1 | inventiveness LLM 综合判断可绕过 AND 公式 | — | 待建 |
| `DEBT-12` | P1 | psychological 仅实现 30–35% 声称能力 | — | 待建 |
| `DEBT-13` | P1 | specdrafting/claimdrafting P1 ×14 未闭环 | — | 待建 |
| `DEBT-14` | P1 | evidence YAML 15 条规则是否真进引擎待确认 | — | 待建 |
| `DEBT-15` | P1 | memory.SearchAllLayers 调用侧传空 scope | — | 待建 |
| `DEBT-16` | P1 | MCP config trust TOCTOU 时序窗口 | — | 待建 |
| `DEBT-17` | P1 | executor-full-review 14 项未闭环 | — | 待建 |
| `DEBT-18` | P1 | agentcore-deep-review 遗留 7 项 | — | 待建 |
| `DEBT-19` | P1 | agent-runtime-review P1 ×13 | — | 待建 |
| `DEBT-20` | P1 | //nolint 抑制文化：432 处，复杂度门禁形同虚设 | — | 待建 |
| `DEBT-21` | P1 | 高复杂度函数热点（gocyclo > 40） | — | 待建 |
| `DEBT-22` | P1 | memory 与各 domain 规则引擎成对复制粘贴 | — | 待建 |
| `DEBT-23` | P1 | 孤儿包 domains/chemistry + domains/figure（697 行） | — | 待建 |
| `DEBT-24` | P1 | 12 处 //nolint:unused：生产符号靠测试续命 | — | 待建 |
| `DEBT-25` | P1 | 安全红线相关路径测试缺口 | — | 待建 |
| `DEBT-26` | P1 | 模块级覆盖率薄弱 | — | 待建 |
| `DEBT-27` | P1 | AI_CHANGELOG 格式合规 | — | 待建 |
| `DEBT-28` | P1 | 文档计数漂移、死引用、目录引用错误 | — | 待建 |
| `DEBT-29` | P1 | 15 个包未在 CLAUDE.md 出现 | — | 待建 |
| `DEBT-30` | P1 | ADR 体系不一致 | — | 待建 |
| `DEBT-31` | P1 | docs/architecture/orchestrator-refactor-roadmap.md 整体失效 | — | 待建 |
| `DEBT-32` | P2 | cmd/mady 入口包膨胀 | — | 待建 |
| `DEBT-33` | P2 | 15+ 全局可变单例 | — | 待建 |
| `DEBT-34` | P2 | 前端构建产物被 git 跟踪 | — | 待建 |
| `DEBT-35` | P2 | 命名与类型一致性批量治理 | — | 待建 |
| `DEBT-36` | P2 | 并发原语三份实现 | — | 待建 |
| `DEBT-37` | P2 | 路径与魔法数硬编码收敛 | — | 待建 |
| `DEBT-38` | P2 | TUI 规范符合度与实现缺口 | — | 待建 |
| `DEBT-39` | P2 | TUI 全量审阅 P0-1 + P1 ×12 | — | 待建 |
| `DEBT-40` | P2 | 错误处理与文案规范批量治理 | — | 待建 |
| `DEBT-41` | P2 | 重复代码清理剩余项 | — | 待建 |
| `DEBT-42` | P2 | tools/ 4 种模式骨架代码返回 nil | — | 待建 |
| `DEBT-43` | P2 | 编排/checkpoint 遗留 P1 | — | 待建 |
| `DEBT-44` | P3 | 全仓重构 CHECKLIST（77 项，0/77） | — | 待建 |
| `DEBT-45` | P3 | optimization-plan 全景 | — | 待建 |
| `DEBT-46` | P3 | 编排器重构 roadmap Phase 0–5 | — | 待建 |
| `DEBT-47` | P3 | 知识库增强计划 | — | 待建 |
| `DEBT-48` | P3 | 专利能力超越考试大纲 Phase 1–2 | — | 待建 |
| `DEBT-49` | P3 | reasonix 引入计划 | — | 待建 |
| `DEBT-50` | P3 | YunXi → Mady 升级任务清单 | — | 待建 |
| `DEBT-51` | P3 | log → slog 迁移 | — | 待建 |
| `DEBT-52` | P3 | 桌面端 W2/W3/W4 波次 | — | 待建 |
| `DEBT-53` | P3 | TUI 四周 Sprint + 差距修复方案 | — | 待建 |
| `DEBT-54` | P3 | 规范体系缺口 | — | 待建 |
| `DEBT-55` | P3 | Phase 4 Backlog 未勾选项 | — | 待建 |
| `DEBT-56a` | P3 | 日卡阶段 2 收尾：domains 层剩余 | — | 待建 |
| `DEBT-56b` | P3 | 日卡阶段 3：基础设施层 | — | 待建 |
| `DEBT-56c` | P3 | 日卡阶段 4：接口层 | — | 待建 |
| `DEBT-56d` | P3 | 日卡阶段 5：工具与入口 | — | 待建 |
| `DEBT-56e` | P3 | 日卡阶段 6：端/测试/脚本 + 终审报告 | — | 待建 |
| `DEBT-57` | P2 | 桌面端 A2UI Action 入站处理未实现 | [#63](https://github.com/xujian519/mady/issues/63) | 已建 |
<!-- debt-index:end -->

---

## 五、Issue 清单 P0 —— 阻塞级（5 条）

> 核验状态图例：✅ 已回代码核实 / ⚠️ 部分核实 / 🔁 跨报告重复出现

### DEBT-01 🔁 【隐私】desktop 全域 + TUI provider 切换点绕过 PII 脱敏

- **状态**：✅ 已核实（核验完成，**结论与旧报告不同**）
- **已接线部分**：`sanitizer.New` 在全仓**非测试代码中仅被调用 1 次**——
  [bootstrap/setup.go:168](bootstrap/setup.go#L168)。它覆盖 `bootstrap.Setup` 入口族
  （`tui` 首启 / `serve` / `acp`），这些入口下的主循环、memory 抽取器、guardian、
  vision、claimdrafting、reasoning 全部被包住 ✅
- **真实漏脱敏路径（4 条，均为生产入口）**：

  | # | 位置 | 成因 | 影响 |
  | --- | --- | --- | --- |
  | 1 | [desktop/app.go:143](desktop/app.go#L143) | 裸调 `agentconfig.BuildProvider()` → `:161` `fc.Provider`、`:166` `NewBaseConfig` | **整个 desktop 应用零脱敏**（含 memory/reasoning/vision/主 Agent） |
  | 2 | [desktop/app_settings.go:155](desktop/app_settings.go#L155) | 设置面板切 provider 时裸调 `BuildProvider()` → `:178` `a.fc.BaseConfig.Provider` | desktop 运行中切换后，主 Agent 与 server 均用裸 provider |
  | 3 | [cmd/mady/tui.go:181](cmd/mady/tui.go#L181) | 启动恢复持久化的非默认 provider，`BuildProviderFor` 裸调 | TUI 命中该分支后 `buildAgentConfig` 全链裸 provider |
  | 4 | [cmd/mady/tui_session_provider.go:17](cmd/mady/tui_session_provider.go#L17) / [:142](cmd/mady/tui_session_provider.go#L142) | `/provider` 命令、`/settings reset` 裸调 `BuildProviderFor` | TUI 运行中切换 provider 即脱离脱敏 |

- **根因**：脱敏包装挂在**调用点**（`setup.go`）而非**构造点**（`agentconfig.BuildProviderFor`）。
  凡不经 `bootstrap.Setup` 的入口，一律拿到裸 provider
- **建议修复（改动很小）**：把 `sanitizer.New` 从 `bootstrap/setup.go:168` 下沉到
  [bootstrap/agentconfig/provider.go:43](bootstrap/agentconfig/provider.go#L43)
  `BuildProviderFor` 的返回处，一次性覆盖 desktop/TUI/example 全部构造点
  —— **前置确认**：`provider/sanitizer` 导入 `agentcore`，需按分层规范确认
  `bootstrap/agentconfig` 依赖该包是否合规（⚠️ 未验证）
- **附带**：`tools/vision.go:304→188` `visionOperationsFromEnv()` 自建 HTTP 直发 LLM，
  **完全绕过 `agentcore.Provider` 接口**——当前入口都注入了 Provider 故走不到，属休眠路径
- **脱敏规则本身也不足**：[provider/sanitizer/patterns.go:38-77](provider/sanitizer/patterns.go#L38-L77)
  仅 4 条正则（身份证 18 位 / 手机号 / 银行卡 16-19 位 / 邮箱）。`patterns.go:33-36` 注释
  显式声明**故意不匹配**中文姓名与地址。**专利场景高敏感类型全部遗漏**：申请人/发明人姓名、
  地址、专利申请号、代理机构代码、统一社会信用代码、**未公开技术方案正文**
  （后者无法用正则识别，属机制固有盲区）
- **次要观察**：`patterns.go:60` 银行卡规则 `\b\d{16,19}\b` 会误伤任何 16-19 位纯数字串
  （专利申请号片段、大额数值），响应侧会还原故功能无损，但增加 token 并可能干扰模型理解
- **来源**：`docs/review/53-full-audit-security.md:21-68`、`57-full-audit-summary.md:33`
  （原条目已过期，本报告为其修正版）
- **label**：`debt/privacy` `debt/security` `P0`
- **验收标准**：全仓 `grep -rn "BuildProvider\|chatcompat.New" --include="*.go" | grep -v _test.go`
  返回的每个构造点均产出已包装 provider；新增一条针对 desktop 入口的脱敏回归测试

### DEBT-02 🔁 【架构】`agentcore` God Package 与分层反向依赖（根因）

- **状态**：✅ 已核实
- **证据**（本次实测，非测试代码）：
  - import `agentcore` 根包的包数：**72**
  - import 抽象契约 `agentcore/iface` 的包数：**2**（`agentcore`、`guardrails`）
  - `agentcore/iface/` 全包**仅 1 个 interface**（`lifecycle.go:54 LifecycleHook`）
  - `agentcore` 根包 75 个非测试源文件、27 个 interface 定义
  - 基础设施层反向依赖 agentcore：**12 个包 / 45 个文件** ——
    `knowledge`(9) `disclosure`(8) `memory`(7) `session`(5) `evaluate`(5) `doomloop`(3)
    `prompt`(2) `retrieval`(2) `store`(1) `intent`(1) `tracing`(1) `psychological`(1)
  - 干净未被污染：`graph` `skill` `fuzzy`（0 文件）
- **根因**：`Message`/`Tool`/`Role`/`Event`/`StateSnapshot`/`TokenUsage` 等**纯值类型**
  住在最顶层 `agentcore`，任何需要这些类型的包都被迫拉起整个 agent 运行时
- **危害**：改 `Message` 牵动 72 个包；想复用 `session`/`prompt` 必须拖进整个运行时；
  `iface` 永远无法成为真契约
- **来源**：`docs/review/50-full-audit-arch-summary.md:23-100,102-133,152-181`、`57:34`
- **label**：`debt/arch` `P0`
- **建议动作**：抽出零依赖叶子包 `agentcore/kernel`（纯值类型），`agentcore` 只保留引擎行为。
  **这一步做不完，§五其余架构项都是治标。**

### DEBT-03 【架构】`agentcore/iface` 契约降级，文档与代码不符

- **状态**：✅ 已核实
- **证据**：`agentcore/iface/doc.go:27-30` 自陈 `Extension`/`ToolProvider`/
  `ContextProvider`/`Store`/`ChatProvider`/`AgentContext` 「**从未被实现**，已于
  2026-07-26 移除」；`doc.go:9-13` 记录 `Event`/`EventBus` 已于 2026-08-11 移除。
  仅存通道本身有损：`TurnInfo.ToolCount` **恒为 0**、`BeforeMessagePersist` 不携带
  `Message`、`BeforeCompactionPersist` 丢弃 `[]Message` 返回值（`doc.go:38-50`）
- **额外心智负担**：存在**两个同名 `iface` 包** —— `agentcore/iface`（1 接口）与
  `domains/iface`（0 接口，仅 4 个 DTO）
- **文档不符**：CLAUDE.md 描述 iface 为「接口抽象层（Agent/Extension/Lifecycle/
  Provider/Event 契约）」——实际只剩 1 个
- **label**：`debt/arch` `debt/docs` `P0`
- **建议动作**：二选一，**不要维持现状** ——
  (a) 补全为无损契约；或 (b) 删包 + 如实修正 CLAUDE.md。
  同时给 `domains/iface` 改名消除同名

### DEBT-04 【架构】`domains` ↔ `guardrails` 目录级双向依赖

- **状态**：✅ 已核实
- **证据**：`domains` → `guardrails` 5 处（`unified.go:7` `legal.go:8`
  `citation_wiring.go:10` `patent.go:32` `assemble.go:9`）；
  `guardrails` → `domains` 反向 1 处（`guardrails/outputgate.go:8` →
  `domains/provenance`）
- **危害**：Go 编译器不报错（不同 package，非 import cycle），但 `guardrails`
  **再也无法脱离 `domains` 单独测试或复用**。而 `guardrails` 又被 `bootstrap/`
  （4 文件）与 `cmd/mady/` 依赖——**被启动装配层依赖的安全组件，被领域层反向绑死**
- **同类方向倒置**：`guardrails/guardian/guardian.go:13` → `prompt`，
  护栏依赖基础设施层提示词库——改 prompt 模板可能影响安全审查行为
- **label**：`debt/arch` `debt/security` `P0`
- **建议动作**：把 `domains/provenance` 下沉为顶层独立包，`guardrails` 只依赖它

### DEBT-05 【并发】`pregel` 节点 panic 被降级为日志，图继续执行且不标记失败

- **状态**：✅ 已核实（**这是核验中发现的替代问题**）
- **证据**：`graph/pregel.go:294-299` 的 `recover()` 只 `slog.Error` 记录，
  **不写入 `errs` map**；而 `pregel.go:322-326` 仅遍历 `errs` 判定失败。
  即：节点 panic → 该节点结果缺失 → `mergeWithSchema` 以残缺状态继续 → 图正常返回
- **危害**：比原文档声称的「无隔离」更隐蔽——panic 不再杀死进程，但**失败被静默吞掉**，
  产出的是看似成功的残缺结果。违反项目「fail-closed」元规则
- **来源**：`docs/refactoring/CHECKLIST.md` P0 项（原描述为「panic 无隔离」，描述已过期）
- **label**：`debt/quality` `P0`
- **建议动作**：recover 分支写入 `errs[nodeName] = fmt.Errorf("node panicked: %v", r)`

---

## 六、Issue 清单 P1 —— 重要级（26 条）

### 架构与依赖

**DEBT-06 【架构】`domains` 根包 3 个别名垫片（142 行纯转发）**
`domains/audit_alias.go`(50) / `caseindex_alias.go`(46) / `project_alias.go`(46)。
后果：同一 API 有两个公开面（`domains.AuditLogger` vs `audit.AuditLogger`），
grep/IDE 跳转指向不同位置。根因见 `project_alias.go:16`——TUI 是独立模块，
依赖旧路径，**跨模块耦合锁死了内部重构**。✅已核实｜`debt/arch`

**DEBT-07 【架构】`acp` → `bootstrap` 反向依赖**
`acp/server_app.go:11` import `bootstrap/agentconfig`。`acp` 属外部接口层，
`bootstrap` 属启动装配层。对照：`server` 做对了（`server/` 无任何 bootstrap 依赖）。
影响：`acp` 无法独立于 bootstrap 装配使用。✅已核实｜`debt/arch`

**DEBT-08 【架构】`graph` 内部测试反向依赖 `agentcore`**
`graph/graph_test.go:10`、`graph/pregel_test.go:8`（均为 `package graph` 内部测试）。
最底层的图引擎，其测试必须拉起整个 agent 运行时——说明 graph API 未定义出
「图使用者」可满足的窄接口。而 `agentcore/worker/executor.go:28` 又依赖 `graph`，
构成测试态循环。✅已核实｜`debt/arch` `debt/test`

**DEBT-09 【架构】`tools/` 与 `domains/` 两个巨型根包，职责混杂**
`tools/` 根包 61 个非测试文件，其中 **23 个是浏览器子系统**，同包还混着文件系统工具、
网络搜索、文档转换、代码执行、专利检索。`tools/browserproviders/` 子包的存在说明
拆分做了一步就停了。`domains/` 根包 37 文件，至少混 4 类职责（案件管理 11 文件 /
Agent 装配 / 风格配置 / 沙箱回归）。✅已核实｜`debt/arch`

**DEBT-10 【架构】三份 provider→默认模型映射，常量散落 8 处**
三处功能重叠但 `default:` 分支行为不同的实现：
`provider/chatcompat/chat.go:395`（多一层 `"default"` 特判）、
`bootstrap/agentconfig/provider.go:125`、`bootstrap/agentconfig/models.go:91`。
模型名 `"deepseek-v4-flash"` 硬编码 4 处、`"deepseek"` 3 处。
**加一个新 provider 要改 8 个地方，漏一处即静默降级回 deepseek。**
✅已核实（`provider/chatcompat/chat.go:537` 亦硬编码默认模型）｜`debt/quality`

### 领域逻辑正确性

**DEBT-11 【领域】`inventiveness` LLM 综合判断可绕过 AND 公式**
`domains/inventiveness/node_conclusion.go:178-181`、`nodes.go:786-797`。
`docs/decisions/phase4-backlog.md:29`（R6-F01，未勾选）｜`debt/quality` `P1`

**DEBT-12 【领域】`psychological` 仅实现 30–35% 声称能力**
`psychological/engine.go:229-238`（calming 公式反向）、`config.go`
（`SkipDistortionDetection` 永久 true）、`hook.go`（整文件死代码）。
3 处注释重复声明「TODO placeholder」。backlog R9-F1/F2/F4 均未勾选｜`debt/dead-code` `debt/quality`

**DEBT-13 【领域】`specdrafting`/`claimdrafting` P1 ×14 未闭环**
`domains/specdrafting` Pregel 节点忽略 LLM provider、`LLMDrafter.Draft()` 错误静默吞、
claimdrafting 法律依据引用错误 ×3（`domain-chemical`/`scope-equivalents-coverage`/
`domain-utility-model`）、`parseClaimsFromLLM` 全有或全无策略。
来源 `docs/review/spec-claim-drafting-review-2026-07-23.md:81-311`｜`debt/quality`

**DEBT-14 【领域】`evidence` YAML 15 条规则是否真进引擎待确认**
`EvidenceRulesYAML` 已移到 `domains/evidence/embed.go` 并在
`bootstrap/setup_assembly.go:102` 加载，**但未确认是否注册进 `RuleIndex`**。
来源 `docs/review/52-full-audit-domain-logic.md:31-56`｜⚠️待核验｜`debt/quality`

### 安全

**DEBT-15 【安全】`memory.SearchAllLayers` 调用侧传空 scope**
`cmd/mady/tui_session_inspect.go:165` 传 `memory.MemoryScope{}`；`AsFilter` 对空
scope 不施加任何过滤。**对照**：`cmd/mady/tui_session_config.go:41` 是正规写法
（UserID/SessionID/AgentID/ProjectID 全填）、`memory/types.go:37 IsEmpty()` 的存在
说明隔离是刻意设计。同类：`desktop/memory_app.go:85` 的 `Remember` 也传空 scope。
✅已核实｜`debt/privacy`（影响面限于本地 TUI/桌面）｜`docs/review/53-full-audit-security.md:72-121`

**DEBT-16 【安全】MCP config trust TOCTOU 时序窗口**
`isConfigTrusted` 与 `createExtension` 之间存在校验窗口（`mcp/config_trust.go`，
敏感路径文件）。`docs/review/53-full-audit-security.md:151-181`｜`debt/security`

### 并发与资源

**DEBT-17 【并发】`executor-full-review` 14 项未闭环**
`ExecuteAll` 不检查 ctx 取消；`pool.Acquire` 失败致 goroutine 泄漏；`InvokeTool`
绕过生命周期钩子。来源 `docs/review/executor-full-review-2026-07-23.md:48-244`｜`debt/quality`

**DEBT-18 【并发】`agentcore-deep-review` 遗留 7 项**
planmode awk 绕过；pipeline 缺 panic 隔离；handoff 缺递归限深；filecheckpoint 沙箱
校验缺失；doomloop 隐式串行；TieredEngine 按字节截断中文；awk/sed 重定向绕过。
来源 `docs/review/agentcore-deep-review-2026-07-20.md:109-172`｜`debt/quality` `debt/security`

**DEBT-19 【并发】`agent-runtime-review` P1 ×13**
重点：`handoff.go` `inheritRuntime` 全信任能力转移（**仅 WARN 日志，安全红线**）、
`extension.go:81-138` `Register` 无回滚、`context_builder_default.go` `InjectMode`
未使用、`state.go:58-80` 与 `reasoning_strategy.go:256-259` 浅拷贝、
`budget.go:233-252` 事前检查缺失、`steering.go:33` 无界队列。
来源 `docs/reviews/agent-runtime-review-20260723.md`｜`debt/quality` `debt/security`

### 代码质量

**DEBT-20 【质量】`//nolint` 抑制文化：432 处，复杂度门禁形同虚设**
实测分布：`gosec` 212 / **`gocognit` 107** / `revive` 37 / `dupl` 29 /
`gosec,noctx` 22 / `errchkjson` 20 / `unused` 12 / `staticcheck` 12。✅已核实
- 项目 `.golangci.yml` 下 `golangci-lint run ./...` 输出 **0 issues**——不是干净，
  是被兜住。用严格临时配置（`gocognit:25` / `gocyclo:20` / `dupl:80`）重扫，
  根模块立刻暴露 18 处重复 + ~57 高认知复杂度 + ~50 高圈复杂度；tui 模块 8 + 40
- 另有 77 处 `//nolint` 无原因说明；`.golangci.yml` 未启用 `revive:exported`
- 来源 `docs/review/51-full-audit-code-quality.md:68-145`｜`debt/quality`

**DEBT-21 【质量】高复杂度函数热点（gocyclo > 40）**
`tui/chat/chat_app_keys.go:26 dispatchKey` **52** ·
`tui/theme/json.go:129 applyColorKey` **49** ·
`tools/piagent/schema.go:33 schemaNode` **46** ·
`tools/browser_tool_navigate.go:19 handleNavigate` **44** ·
`domains/reasoning/plan_compiler.go:132 CompilePlanToGraph` **43** ·
`cmd/mady/tui.go:60 runTui` **42**。
另 16 个函数 >150 行。建议表驱动化 / 拆子函数。✅已核实｜`debt/quality`

**DEBT-22 【质量】`memory` 与各 domain 规则引擎成对复制粘贴**
- `memory/store.go:242-259` ↔ `memory/sqlite_store.go:303-320`：`RecallWithBudget`
  两实现逐行相同
- `domains/claimdrafting/rules_domain.go:16-41` ↔ `:67-92` 同构；
  `domains/specdrafting/rules_domain.go:15-32` ↔ `:36-53` 完全同样模式——
  **两个 domain 的领域规则引擎是复制粘贴**
- `mcp/discovery_client.go:135/163/179` 三连重复（可泛型化）
- 已显式豁免的 `//nolint:dupl` 29 处（理由均为 "intentionally duplicated"）——
  属**技术债登记簿**，建议周期复核
✅已核实｜`debt/quality`

### 死代码

**DEBT-23 【死代码】孤儿包 `domains/chemistry` + `domains/figure`（697 行）**
`domains/chemistry/text.go`(213) + `domains/figure/consistency.go`(484)，
全仓**零外部引用**（本次实测：`CheckFigureConsistency`/`ExtractChemEntities` 等
导出符号在包外引用数均为 0，仅各自 `_test.go` 引用）。注释自称「移植自 Sati」——
典型移植后未接线。✅已核实｜`debt/dead-code`
**建议动作**：接入调用链或删除（二选一，勿继续搁置）

**DEBT-24 【死代码】12 处 `//nolint:unused`：生产符号靠测试续命**
`cmd/mady/framework.go:70,77,84,101`、`tui_storage.go:229`、`tui_deferred.go:13`、
`domains/reasoning/planner.go:373`、`multi_hypothesis.go:36,284`、
`agentcore/compaction_budget.go:32,35,90`——理由全部是 "used in xxx_test.go"。
✅已核实｜`debt/dead-code`

### 测试

**DEBT-25 【测试】安全红线相关路径测试缺口**
`agentcore/permission` 7 源 / 1 测（**0.14**）、`guardrails/guardian` 6 源 / 1 测
（**0.17**）、`guardrails` 整体 23 源 / 8 测（0.35）。权限门控与 AI 安全审查
是安全红线，应优先补测。来源 `docs/review/54-full-audit-test-coverage.md:144-241`｜`debt/test`

**DEBT-26 【测试】模块级覆盖率薄弱**
`bootstrap` 1.9% · `cmd/mady` 18.1% · `domains/audit` 0% ·
子包热点：`tools/desktop` 13 源/1 测(0.08)、`pkg/ocr` 13/1(0.08)、
`domains/infringement` 11/1(0.09)、`domains/specdrafting` 14/2(0.14)。
另：tui/ 零 `errors.Is/As` 使用；4 处无保护 `io.ReadAll`
（`tools/browser_camofox.go:320,409`、`tools/browserproviders/browser_use.go:58,104`）；
46 处 `time.Sleep`。｜`debt/test`

### 文档

**DEBT-27 【文档】AI_CHANGELOG 格式合规**
`docs/decisions/ai-changelog/INDEX.json` 实测 `total=429` 条。旧报告称「223 条
0% 合规」数字已过期，但**格式字段命中率问题待重新核验**（Decision/Reason/Risk/
Human Owner/Spec）。本次另发现 **INDEX.json 存在 1 条重复条目**
（`2026-09-06.md:3` 出现两次）——changelog 脚本幂等性缺陷。⚠️待重新核验｜`debt/docs`

**DEBT-28 【文档】文档计数漂移、死引用、目录引用错误**
`AGENTS.md`/`CLAUDE.md` 声称文件数 vs 实际漂移（旧报告 1229 vs 1405，本次实测
非测试 `.go` 文件 **1111**、全量 `.go` **1686**——**三个数字互不一致，需一次性
重新对齐**）；`README.md:215` 死引用 `chat-agent`；`README.md:219` vs
`manifest-guide.md:8` 数量矛盾；`domains/domainconfig/` 应为 `domains/config/`。
来源 `docs/review/55-full-audit-docs-consistency.md:88-134`｜`debt/docs`

**DEBT-29 【文档】15 个包未在 CLAUDE.md 出现**
本次逐个核实，以下目录**存在但 CLAUDE.md 零提及**：
`domains/amendment` `domains/analysiskit` `domains/audit` `domains/caseindex`
`domains/chemistry` `domains/figure` `domains/rulekit` `domains/search`
`domains/slop` `domains/deadline` `knowledge/patenttools` `invariant`
`internal/intentrules` `bootstrap/agentconfig`（`internal/intentrules` 被
`domains/legal_intent.go` 与 `intent/keyword.go` 同时引用，是未文档化的共享规则包）。
同时 CLAUDE.md 描述的部分结构已不存在（iface 的 4 类契约）。✅已核实｜`debt/docs`

**DEBT-30 【文档】ADR 体系不一致**
ADR-0001 写「6 层」vs 实际 8 层；编号缺口 004/005 缺失；ADR-0006/0007 部分实现。
ADR-0003 已正确改为 Deferred（下次评估 2026-10-31）。｜`debt/docs`

**DEBT-31 【文档】`docs/architecture/orchestrator-refactor-roadmap.md` 整体失效**
状态仍为 Proposed，但引用的 `agentcore/pipeline_executor.go` **已不存在**、
`FailOnUnknown` 符号**全仓不存在**、其列的两个 P0 均已修复（见 §一.2）。
`orchestrator-refactor-checklist.md` 全部未勾选。**需整体重评或归档**。✅已核实｜`debt/docs`

---

## 七、Issue 清单 P2 —— 改进级（12 条聚合）

> P2/P3 采用**聚合型 issue**（一个 issue 含多个子项 checklist），避免上百条噪声 issue。

**DEBT-32 【结构】`cmd/mady` 入口包膨胀** 39 个源文件。建议提取到 `internal/` 或
bootstrap。来源 `56a-structural-organization.md:66-79`｜`debt/arch`

**DEBT-33 【结构】15+ 全局可变单例** `domains/patent.go` 12 个 `global*`、
`domains/prompt_store.go:12`、`tools/browser_manager.go:11`——
`domainFactoryMap` 回调签名受限。｜`debt/arch`

**DEBT-34 【结构】前端构建产物被 git 跟踪** `desktop/dist`、`node_modules`、
`test-results`、`wailsjs`。来源 `56a-structural-organization.md:158-176`、
`desktop-next-development-plan.md:25`（W4-T2）｜`debt/quality`

**DEBT-35 【结构】命名与类型一致性批量治理**
① `Severity` 类型重复定义 2 处（`domains/workflows/patent/rule_engine.go:24` 用类型
别名 `= rules.Severity` 缝合，`domains/rules/types.go:10` 为源）；
② 5 套规则引擎命名（`Engine`/`DefaultEngine`/`RuleEngine`/`DesignRuleEngine`/`Slop*`）；
③ 3 套 Scorer 同名不同构、**无公共接口**（`ClaimScorer`/`InfringementScorer`/`SpecScorer`
构造签名互不相同，无法统一调度）；④ 3 套 Router；⑤ Mutex 命名 80% 用 `mu` 而非
`xxxMu`（~70 处）。｜`debt/quality`

**DEBT-36 【结构】并发原语三份实现** `pkg/csync`（11 处消费）、
`tui/internal/csync/slice.go:12`（**显式复制**，注释说明为独立子模块避环）、
`agentcore/concurrency/`（151 行，唯一消费者 `agentcore/executor.go:12`——一个包服务一个文件）。
tui 的复制有正当理由，但暴露「共享原语无独立发布单元」。｜`debt/arch`

**DEBT-37 【结构】路径与魔法数硬编码收敛**
① `.mady` 目录名 **4 处绕过 `pkg/util.MadyHome()`**：`cmd/mady/tui_storage.go:126`、
`acp/server_app.go:191,193`（后者是相对路径）、`mcp/config_trust.go:78-80`（**自行复刻
MADY_HOME 逻辑**）、`pkg/ocr/ocr.go:40`。`pkg/util/paths.go:10` 注释**点名了违规者**——
已知未修；
② 上下文窗口 `128000` 在 `agui/converter.go:29,47` **同文件重复硬编码**；
③ `8192` 三处无共享常量；④ 端口 `8080`/`8000` 各 5 处硬编码。｜`debt/quality`

**DEBT-38 【测试】TUI 规范符合度与实现缺口** P0 ×4 + P1 ×7：
`tui/stdio/` 目录**不存在**但规范仍列为核心层；`Overlay` 缺 `Modal` 字段；
19 个组件无测试文件；规范附录 C 与 `LAYERS.md` 全面过时；ChatEvent 数量三处不一致。
来源 `tui-spec-compliance-review-2026-07-31.md:68-229`｜`debt/docs` `debt/test`

**DEBT-39 【质量】TUI 全量审阅 P0-1 + P1 ×12**
P0-1 `WindowSizeMsg` 防抖自续循环（常驻 100ms 定时器 + 组件收不到 resize）；
P1-1 `tui/terminal/detect.go:781-792` VTE 阈值 `82000` **恒假** → VTE 终端永远拿不到
KKP；P1-2 `keys.go:381-388` Kitty PUA 映射错位（**Enter↔Tab**）；
P1-4 `theme_registry.go:249-255` 主题被静默覆盖；P1-6 光标永久隐藏；
P1-7 `editor_edit.go:325-341` onChange 未调用；P1-8 Ctrl+Shift+Z 无法 redo；
P1-12 `session_selector.go:347,400,409,422` 4 处无 context/done 的 goroutine。
来源 `tui-full-audit-2026-08-09.md:26-94`｜`debt/quality`

**DEBT-40 【质量】错误处理与文案规范批量治理**
① **44 处大写开头错误信息**（Go 约定小写）——`desktop/app_files.go`(25)、
`app_settings.go`(3)、`app.go`(2)、`templates.go`(2)、`project.go`(4)、
`tools/bash.go`(2)、`domains/approval.go`；② `a2a/jsonrpc.go:11,18`、`server.go:240,251`、
`middleware.go:67`、`ratelimit.go:174` 共 6+ 处 `_ = json.NewEncoder(w).Encode(...)`
**HTTP 响应写失败被静默吞掉**；③ `revive:stutter` ~300、`goconst` 剩余 621。
来源 `56-full-audit-simplicity.md:23-133`、`scan-report-2026-07-27.md:157-165`｜`debt/quality`

**DEBT-41 【质量】重复代码清理剩余项**
`mcp/client.go` ↔ `mcp/http.go` 3 方法拷贝（~40 行）；三入口权限策略复制
（`cmd/mady/acp.go:39-50`、`cmd/mady/server.go:107-118`、`desktop/app.go:280-291`，~30 行）。
（`domains/approval` 与 `browser_supervisor` 的重复已消除 ✅）
来源 `56-full-audit-simplicity.md` ｜`debt/quality`

**DEBT-42 【质量】`tools/` 4 种模式骨架代码返回 nil**
来源 `docs/refactoring/optimization-plan.md:569-576`｜`debt/dead-code`

**DEBT-43 【质量】编排/checkpoint 遗留 P1**
`domains/workflows/patent/analysis.go:149-170`（占位符 + nil error）、
`domains/workflows/legal/comparison.go:82-87`（硬编码占位符）、
`cmd/mady/tui_session_config.go:188`（评审模式误用 `MemoryCheckpointStore`）。
来源 `orchestrator-refactor-roadmap.md:26-28`｜`debt/quality`

---

## 八、Issue 清单 P3 + 跟踪型（12 条聚合 + 5 条日卡跟踪）

**DEBT-44 【跟踪】全仓重构 CHECKLIST（77 项，0/77）**
`docs/refactoring/CHECKLIST.md:144-149`。含 goroutine 安全类 3 项
（其中 panic 隔离已修复，见 DEBT-05；剩余项需重评）。
**建议**：先重新核验 77 项现状再决定是否逐条建 issue｜`tracking`

**DEBT-45 【跟踪】optimization-plan 全景**
`docs/refactoring/optimization-plan.md:21-22,607-616`——P1-A Linter 增强、
P1-B Worktree 清理（**可释放 470MB**）、P2/P3 未开始｜`tracking`

**DEBT-46 【跟踪】编排器重构 roadmap Phase 0–5**
状态 Proposed，但**已部分失效**（见 DEBT-31）。建议先重评再启动｜`tracking` `blocked`

**DEBT-47 【跟踪】知识库增强计划**
`docs/plans/knowledge-enhancement-plan.md:488,746-773`——`query_writing_patterns`
已上线；`knowledge/loader/guideline_loader.go` 等 P1 文件待建｜`tracking`

**DEBT-48 【跟踪】专利能力超越考试大纲 Phase 1–2**
`docs/decisions/patent-beyond-exam-roadmap.md:41-44`——P0 审查员角色模拟器、
案件决策回溯链；P1 判例库接入、多方案权衡矩阵｜`tracking`

**DEBT-49 【跟踪】reasonix 引入计划**（FileSnap / TurnCheckpoint / 逐调用审批决策）
`docs/decisions/reasonix-implementation-plan.md`｜`tracking`

**DEBT-50 【跟踪】YunXi → Mady 升级任务清单**
`docs/decisions/YunXi-Mady-升级任务清单.md:8-146`——P0-2 技能 Include 展开、
P0-1 统一意图分类器；P1-1 宪法规则引擎、P1-2 子 Agent 角色 XML｜`tracking`

**DEBT-51 【跟踪】`log` → `slog` 迁移**
`docs/decisions/log-to-slog-migration-plan.md:3` 状态「方案待评审」。
关联证据：`agentcore/executor.go:357` 仍在用 `log.Printf`（全仓 `log.Fatal` 8 处、
6 处在示例程序）｜`debt/quality` `tracking`

**DEBT-52 【跟踪】桌面端 W2/W3/W4 波次**
`docs/plans/desktop-next-development-plan.md:18-32`——TodoDock、Token 全量补齐、
组件测试环境、Zustand 切分、TanStack Query 接管、暗色三态、HIG 走查、前端 i18n 评估｜`tracking`

**DEBT-53 【跟踪】TUI 四周 Sprint + 差距修复方案**
`tui-four-week-optimization-sprint-plan.md:4`（待实施）、
`tui-gap-implementation-plan.md:23,33,111,176`（全部复选框未勾选）｜`tracking`

**DEBT-54 【文档】规范体系缺口**
① GitHub 分支保护**未确认**——若未开启则所有 CI 门禁可被绕过（标「待核验」）；
② 数据处理与隐私规范缺失（`docs/data-privacy-standards.md` 存在但未纳入门禁）；
③ 测试策略文档缺失；④ **敏感路径清单 4 处复制漂移**（权威源为
`scripts/check-sensitive-paths.sh`）。来源 `2026-07-21-spec-system-comprehensive-analysis.md:172-190`｜`debt/docs`

**DEBT-55 【跟踪】Phase 4 Backlog 未勾选项**
`docs/decisions/phase4-backlog.md:25-41`：#3 evidence 3400 行死代码（暂缓）、
#4 侵权证明标准三重矛盾、#7 inventiveness 非对称 override、#8 enablement 9 个
fixtures 断言缺失、#15 compaction 压缩前 redact（`agentcore/compaction.go:462`）、
#19 enablement 五场景 vs 六场景不一致｜`tracking`

### 日卡跟踪（依据 `docs/code-review-plan.md`，M01–M13 已完成）

按**阶段聚合**为 5 条跟踪型 issue，而非一卡一 issue：

| ID | 标题 | 覆盖卡 | 规模 |
| --- | --- | --- | --- |
| DEBT-56a | 日卡阶段 2 收尾：domains 层剩余 | M14 `writing`+`doctmpl`、M15 domains 根包+sqlite+case_* | 289 文件剩余 |
| DEBT-56b | 日卡阶段 3：基础设施层 | M16–M22（knowledge/memory/retrieval/disclosure/guardrails/pkg 等） | 7 卡 |
| DEBT-56c | 日卡阶段 4：接口层 | M23–M27（a2a/a2ui+agui/acp+server/mcp/bootstrap） | 5 卡，含协议面红线 |
| DEBT-56d | 日卡阶段 5：工具与入口 | M28–M31（tools 拆 3 卡 + cmd/mady） | 4 卡 |
| DEBT-56e | 日卡阶段 6：端/测试/脚本 + 终审报告 | M32–M36 + `docs/code-review-report.md` | 5 卡 |

**量化目标**（`docs/code-review-plan.md` §一.4）：裸 `fmt` 调试残留 66 → <30；
被忽略 error 24 → ≤10；静默吞错无注释 380 处下降；未注释包级导出 93 → ≤20；
>120 行单函数 30 处下降。**建议把这 5 个指标写进跟踪 issue 的验收标准。**

---

## 九、明确不建 Issue 的项（已修复 / 已封存）

> 这些条目在文档中仍以「未修复」出现，**照抄建 issue 会产生假 issue**。

### 9.1 已修复（文档未回写）

| 文档条目 | 实际状态 | 证据 |
| --- | --- | --- |
| AGUI `RUN_ERROR` 不发射 | ✅ 已修复 | `agui/handler.go:189` |
| pregel 裸 map 合并 | ✅ 已修复 | `graph/state_schema.go:101` |
| `pipeline_executor.go` 静默跳过 | ✅ 条目失效 | 文件不存在，无 `FailOnUnknown` |
| goroutine panic 无隔离 ×3 | ✅ 已修复 | 三处均有 `recover()` |
| `deferred_persist` 未接线 | ✅ 已修复 | `domains/assemble.go:122` + `levels.go:138` |
| `resumeIfInterrupted` ABBA 死锁 | ✅ 已修复 | `cmd/mady/tui_session_agent.go:230-249,303-330` 锁序已一致 |
| `_ = recover()` 静默吞 panic | ✅ 已修复 | `tui/tui_loop.go:27` 已 `slog.Warn` |
| `enablement` 混用 In re Wands | ✅ 已修复 | 全仓 grep `Wands` 0 命中 |
| eval benchmark `init()` panic | ✅ 已修复 | 现为 `log.Printf` 告警 |
| 编排递归深度限防 R10-F1 | ✅ 已修复 | `DefaultMaxOrchestrationDepth=8` |
| phase1 的 5 个历史 P0 | ✅ 全部闭环 | `docs/review/2026-07-25-phase1-p0-triage.md:171-186` |
| `domains/citation` 目录 | ✅ 已删除 | 测试覆盖率条目一并关闭 |
| ADR-0003 状态 | ✅ 已改 Deferred | 下次评估 2026-10-31 |

> **遗留小债（P3，可并入 DEBT-27）**：
> `guardrails/deferred_persist.go:10-17` 的接线状态注释**已过期**（日期 2026-07-18），
> 仍在向读者声称「尚未接入 production 消费端」「消息直接丢弃」——本次核验中被它误导过一次。

### 9.2 已封存（`docs/roadmap.md:74-92`，P3 通过前不启动）

不建 issue，仅登记：DomainTrademark 第二垂直 · Memory 生产集成（默认关闭）·
阶段 D 规则蒸馏 · YAML 声明式工作流收尾 · ACP Rebuildable 完整实现。

### 9.3 等待人工决策（不建 issue，建 `blocked` 标记或留档）

- **启动真实盲测**（`roadmap.md` ⏸️，共识：真人真测，当前只做数据收集）
- **`go.work` 结构修复 T4.3**（`docs/plans/architecture-debt-fix-plan.md:124-130`
  明确「未经人工确认不执行」，需人工 Owner）
- **C1/C2/W1/W2/W4 架构债 Phase 1–5**（同上文件，Brooks-Lint 健康分 50/100，零执行）

### 9.4 官方已知限制（保留 `docs/known-limitations.md`，可同步建 1 条 issue）

**DEBT-57 【协议】桌面端 A2UI Action 入站处理未实现**
`server/desktop.go` `SendAction()` 投递的
A2UI action 到达事件总线后无消费端——agent 侧未注册 `EventA2UI` 入站处理器。
协议集成分两阶段，当前完成第一阶段（Agent→UI），第二阶段（UI→Agent）规划中。
此为 `docs/known-limitations.md` 唯一正式条目，**建议建 issue 以获得可见性**。

---

## 十、创建与关联操作方案

### 10.1 前置：建立 label（一次性）

```bash
# 类型（8 个）
gh label create "debt/security"  --color b60205 --description "安全红线、权限"
gh label create "debt/arch"      --color 5319e7 --description "分层、依赖方向、模块边界"
gh label create "debt/privacy"   --color d93f0b --description "PII、数据隔离"
gh label create "debt/quality"   --color fbca04 --description "复杂度、重复、命名、错误处理"
gh label create "debt/dead-code" --color c2e0c6 --description "死代码、孤儿导出"
gh label create "debt/test"      --color 0e8a16 --description "测试缺口、覆盖率"
gh label create "debt/docs"      --color 0075ca --description "文档漂移、规范一致性"
gh label create "debt/protocol"  --color 1d76db --description "A2A/A2UI/AGUI/ACP/Server/MCP"
# 优先级（4 个）
for p in P0 P1 P2 P3; do
  gh label create "$p" --color "$( [ $p = P0 ] && echo b60205 || ([ $p = P1 ] && echo d93f0b || ([ $p = P2 ] && echo fbca04 || echo c2e0c6)) )" --description "优先级 $p"
done
# 状态（2 个）
gh label create "tracking" --color ededed --description "跟踪型，不对应单次代码改动"
gh label create "blocked"  --color 000000 --description "等待人工决策"
```

### 10.2 批量创建（建议分批，勿一次性倒入）

| 批次 | 内容 | 条数 | 建议时机 |
| --- | --- | --- | --- |
| 第 1 批 | P0（DEBT-01–05）+ DEBT-57（§9.4 已知限制） | 6 | **立即**——阻塞级，且 DEBT-02 是根因 |
| 第 2 批 | P1 安全/隐私/并发（DEBT-15/16/17/18/19） | 5 | 立即 |
| 第 3 批 | P1 架构与质量（DEBT-06–14、20–31） | 21 | 立即 |
| 第 4 批 | P2 聚合（DEBT-32–43） | 12 | 第 1–3 批有进展后 |
| 第 5 批 | P3 + 跟踪型（DEBT-44–56e） | 17 | 需要时 |

### 10.3 关联方式（issue ↔ 文档 双向可追溯）

**issue 正文模板**（每条）：

```markdown
DEBT-XX · [标题]

## 现状
（已验证的代码事实，附 file:line）

## 影响
（具体危害，拒绝泛泛而谈）

## 证据
- 代码：`path/file.go:123`
- 文档：`docs/review/xxx.md:45`

## 建议动作
（可执行的一步或几步）

## 验收标准
（可验证，例如「golangci-lint 严格配置下 gocognit 告警数下降 N」）

---
来源报告：... ｜ 核验状态：✅已核实 / ⚠️待核验
```

**反向关联**：在 `docs/code-review-plan.md` 等文档的对应条目后追加 `→ #<issue>`，
让文档重新成为可导航的索引（而非事实源）。**建议由脚本自动回写，避免二次漂移。**

**提交关联**：修复合入时用 `Fixes #<n>` / `Refs #<n>`（Conventional Commits 的
footer 位置），使 issue 随提交自动关闭。

### 10.4 建议的里程碑

当前无 milestone。建议建 3 个：
`P0 止血`（DEBT-01–05 + §9.4）· `分层收敛`（DEBT-02 拆解的后续架构项）·
`债务清理 Q4`（P1/P2 聚合项）

---

## 十一、风险与建议执行顺序

### 风险

| 风险 | 说明 | 缓解 |
| --- | --- | --- |
| **假 issue** | 文档条目未核验就建 issue，会产出至少 5 条无效 issue（§9.1） | 本报告已内置核验步骤；建 issue 前逐条复核 file:line |
| **issue 反向腐化** | issue 建完同样会漂移，重演 docs/ 的今天 | ① issue 正文写「验收标准」；② 关闭时回写文档；③ 每季度重核 P0/P1 |
| **范围失控** | 60 条一次全部落地，无人力承接 | 分批（§10.2），第 1–3 批共 32 条已足够一个季度 |
| **DEBT-02 是深水区** | 抽 `agentcore/kernel` 会触及 72 个包的 import | 单独排期、单独 PR、先做 `Message`/`Role` 两个最热类型试点 |
| **公开仓库** | 所有 issue 对外可见，含未公开的安全缺口描述 | DEBT-15/16/19 等安全项描述**避免给出可直接利用的细节**，或改私有 issue（GitHub 无私有 issue，可考虑仅在本仓 security advisory 中记录） |

### 建议执行顺序（前三步）

1. **先建 label + 第 1 批 6 条 P0 issue**（半天）——让阻塞项第一次获得可见性
2. **同步建立文档回写机制**（脚本把 `→ #issue` 写回 `docs/`）——否则这次映射
   半年后同样失效，等于白做
3. **启动 DEBT-02 的试点拆解**（`Message` + `Role` 下沉 `agentcore/kernel`）——
   它是 §五/§六 中 8 条架构 issue 的共同根因，其余架构修复在它之前都是治标

---

## 附录 A：来源文档 → Issue 对照索引

| 来源文档 | 覆盖 Issue |
| --- | --- |
| `docs/review/57-full-audit-summary.md` | DEBT-01/15/16/19/20/25/27/28 |
| `docs/review/53-full-audit-security.md` | DEBT-01/15/16 |
| `docs/review/50-full-audit-arch-summary.md` | DEBT-02 |
| `docs/review/51-full-audit-code-quality.md` | DEBT-20/40 |
| `docs/review/52-full-audit-domain-logic.md` | DEBT-11/12/14 |
| `docs/review/54-full-audit-test-coverage.md` | DEBT-25/26 |
| `docs/review/55-full-audit-docs-consistency.md` | DEBT-28/30 |
| `docs/review/56a-structural-organization.md` | DEBT-32/33/34/35 |
| `docs/review/56-full-audit-simplicity.md` | DEBT-41/35 |
| `docs/review/tui-full-audit-2026-08-09.md` | DEBT-39 |
| `docs/review/tui-spec-compliance-review-2026-07-31.md` | DEBT-38 |
| `docs/review/spec-claim-drafting-review-2026-07-23.md` | DEBT-13 |
| `docs/review/agentcore-deep-review-2026-07-20.md` | DEBT-18 |
| `docs/review/executor-full-review-2026-07-23.md` | DEBT-17 |
| `docs/reviews/agent-runtime-review-20260723.md` | DEBT-19 |
| `docs/architecture/orchestrator-refactor-roadmap.md` | DEBT-31/43/46 |
| `docs/decisions/phase4-backlog.md` | DEBT-11/12/55 |
| `docs/decisions/a2ui-agui-review-report.md` | DEBT-02（AGUI 部分已修复） |
| `docs/plans/architecture-debt-fix-plan.md` | §9.3（人工决策） |
| `docs/refactoring/CHECKLIST.md` | DEBT-05/44 |
| `docs/refactoring/optimization-plan.md` | DEBT-42/45 |
| `docs/code-review-plan.md` | DEBT-56a–56e |
| `docs/roadmap.md` | §9.2 封存项 |
| `docs/known-limitations.md` | §9.4 |

## 附录 B：本次核验方法（可复现）

```bash
# 1. issue 现状
gh api "repos/xujian519/mady/issues?state=all&per_page=100" \
  --jq '.[] | select(.pull_request == null) | .number'   # → 空

# 2. agentcore 耦合度
grep -rl 'xujian519/mady/agentcore"' --include="*.go" . | grep -v _test.go \
  | xargs -n1 dirname | sort -u | wc -l                  # → 72

# 3. iface 契约残存
grep -rn "interface {" agentcore/iface/                  # → 仅 1 个

# 4. nolint 抑制规模
grep -rn "//nolint" --include="*.go" . | grep -v _test.go | wc -l   # → 432

# 5. 孤儿包引用核验
grep -rn "CheckFigureConsistency\|ExtractChemEntities" --include="*.go" . \
  | grep -v "^./domains/chemistry/\|^./domains/figure/"  # → 空

# 6. 严格门禁重扫（暴露被 nolint 掩盖的告警）
golangci-lint run ./... --no-config \
  --enable=gocognit,gocyclo,dupl --settings.gocognit.min-complexity=25
```

---

**下一步**：请确认本报告的分类、优先级、聚合粒度与 §五/§六 的核验状态。
确认后我按 §10.2 第 1 批创建 P0 issue，并同步实现 §10.3 的文档回写脚本。
