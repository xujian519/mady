# Mady 开发路线图

> 更新日期：2026-07-17 | 代码基线：`2595584`
>
> 基于 Manus AI 路线图审阅 + 项目代码深度验证 + 7-9 月执行反馈 + 07-17 人机协作路线图共识

---

## 一、产品定位

**Mady = 证据驱动的专利案件工作台**

**北极星**：每个专业工作成果中，经人工确认、可定位到权威来源且无需结构性重做的证据化结论数量 ÷ 专业人员投入时间。

**首要用户旅程**：技术交底书 → 技术特征树 → 现有技术证据包 → 新颖性初评 → 人工复核报告 → 导出

## 二、模块成熟度（代码验证后修正）

| 模块 | 路线图原评分 | 修正评分 | 关键事实 |
|------|:---------:|:------:|------|
| 知识/检索 | 3.5 | **5.0** | FTS5 trigram BM25 + 144K 向量并行 + 三路 RRF + cross-encoder |
| 评估框架 | 2.0 | **5.0** | 6 套专利法条基准 + LLM Judge + RAG 评估 |
| 推理引擎 | — | **5.0** | FactBlackboard + Syllogism + 多假设 + 五阶段引擎 |
| Agent 运行时 | 4.0 | **4.5** | 生命周期/事件/压缩/Handoff/流式/结构化输出 |
| TUI | 3.5 | **4.5** | 8 层 Elm 架构，斜杠命令完整 |
| 工程 CI/CD | 3.0 | **4.0** | golangci-lint v2 + 多平台矩阵 + Codecov + eval 门禁 |
| 工作流 | 4.0 | **4.0** | Pipeline/Parallel/Router + PER 模式 |
| 状态持久化 | 3.0 | **4.0** | SQLiteCheckpoint + SQLiteMemory + SQLiteApproval |
| 护栏安全 | 1.5 | **3.5** | 两层防御 + 熔断器 + fails-closed |
| 人机审批 | 2.0 | **3.0** | ApprovalState 状态机 + SQLite 持久化 + /approve /reject + RecordDecision 留痕 |
| 专业工作流 | 2.5 | **3.5** | disclosure 11 节点管线：retrieve_prior_art 真实检索 + LLM 新颖性 + review_gate 主动中断 |
| 证据/引用 | 2.0 | **3.0** | EvidenceSpan + ClaimBinding + ConflictDetector + Receipt/Ledger + EvidenceGroundedness 指标 |
| 产品可用性 | 1.5 | **2.5** | DOCX/Markdown 导出 + TUI 审批流程 + 品牌主题/命令系统/设置持久化 |

## 三、7-9 月执行成果（2026.07.14 - 2026.09.30）

### P0 止血（第 1-2 周）✅

- CI 修复：`go mod tidy` / `go vet` / `go build` / `go test` 全绿
- lint 清零：修复 11 个 golangci-lint v2.12.2 问题
- 分支保护：开启 PR 审查 + 状态检查 + 禁止直接推送
- 评估基线：41 个 Golden Case，6 套法条基准，静态回放 1.0
- 功能冻结：README 收敛为首要用户旅程，明确不做事项

### P1A 证据底座（第 3-6 周）✅

- **EvidenceSpan**：证据实体（文档版本、页码、原文哈希、引用方向）+ ClaimBinding + ConflictDetector
- **Case 实体**：CaseStage 状态机（7 阶段）+ ProjectRecord 业务字段（CaseType/FilingNumber/Jurisdiction/Confidentiality）
- **Approval 状态机**：drafted → pending → approved/modified/rejected/canceled + 终止态隔离
- **SQLiteApprovalStore**：WAL 模式持久化 + Save/List/ListByCase/Delete
- **统一存储契约**：store.CaseStore 接口（CaseID/RunID/Migrate/Version），3 个 store 实现

### P1B 闭环集成（第 7-9 周）✅

- **新颖性初判节点**：LLM 驱动（JSON Schema 输出）+ 启发式回退 + sync.Once 缓存
- **证据包裹生成器**：特征 → EvidenceSpan 映射（PriorArtStatus → 证据方向）
- **端到端测试**：证据构建 + JSON 提取 + Schema 完整性

### P1C 打磨（第 10-11 周）✅

- **DOCX/Markdown 导出**：6 章节报告 + pandoc 转换 + 免责声明
- **CI 评估门禁**：eval-benchmark job + 回归检测
- **TUI 审批增强**：/approve /reject 接入 Agent 执行 + 审核状态显示

### 代码审查修复

max effort 审查（10 角度 × 10 并行）发现 15 项，全部修复。详见提交 `5a75779`。

## 四、10-12 月执行与展望

### P2A：Golden Set 第一层 ✅（10 月，第 1-2 周）

- **31 道公开专利考试真题**已按法条归类并集成到 `agentcore/evaluate/benchmark/`（A2/A22/A26/A31/A33/R42 六组）
- **静态评估全绿**：`TestEvalSuite_GoldenPerfect` / `CaseIntegrity` / `DefaultEvaluator` 均通过，41 个 Golden Case 保持 PassRate=1.0
- **LiveEval 基线**：使用 DeepSeek（`deepseek-chat`）对随机 3 道真题评估，通过率 66.7%（2/3），`citation_completeness` 1.0，`llm_judge` 平均 0.456；完整报告见 `docs/evaluation-baseline-v0.5.md`

### P2B：Golden Set 第二层 ✅（10-11 月，已重建并完成五层评估）

- 你指出脱敏案件难获取，改为使用**真实专利复审/无效决定书**作为第二层数据
- 从本地数据 `/Users/xujian/Downloads/专利无效数据`（202601-202604 四个 zip，共 2009 件无效宣告请求审查决定书 docx）中，按发明/实用新型/外观设计 × 全部无效/部分无效/维持有效 的配额筛选出 **40 件典型案例**
- 转化为 `agentcore/evaluate/benchmark/invalidation_decisions.go`（40 个 `TestCase`），并注册到 `AllCases()`
- **LiveEval 基线**：使用 DeepSeek（`deepseek-chat`）对全部 40 道无效决定书案例评估，通过率 **15.0%（6/40）**，`citation_completeness` 平均 **0.287**，`llm_judge` 平均 **0.381**；完整报告见 `docs/evaluation-baseline-v0.6.md`
- 低分原因：① `RequiredCitations` 与模型输出的法条格式（阿拉伯数字 vs 汉字数字）不完全匹配；② 部分 `Expected` 提取偏短或偏程序性，导致 LLM 评判偏低

> **✅ 2026-07-17 更正（原"❄️ 冻结"条目已过时）**：P2B 曾于 2026-07-15 因"空壳输入 + 退化分布"冻结，
> **同日已用宝宸知识库 31562 件原始 MD 重建 100 条完整字段案例并解冻**（`agentcore/evaluate/benchmark/invalidation_decisions.json`），
> 07-16 完成 L0→L5 五层评估并定论：**通用 prompt + 自主推理最优；LLM 模拟 HITL 存在方法论困境，P3 必须真人**。
> **当前有效 live 基准 = P2A（31 道真题）**。详见 `docs/evaluation-baseline-v0.7.md`。

### 07-15/16 集中落地（路线图外增量，全部 ✅）

- **设计一：现有技术检索阶段**——`retrieve_prior_art` 节点接 DomainRetriever（`dd6556f`）+ PatentReranker（`bdc5298`）+ PatentDomainRetriever（`42bae20`）+ check_novelty 注入 evidence
- **获取规则设计四步全链路**——Stage② 四路召回（vector + skill + 确定性规则源）→ 确认阀中断→checkpoint→resume（`8016235`）→ ConfirmedRuleSet 消费 Plan/Execute/Check（`49ce45e` + `7e4e18f`）→ ConfirmedRuleWriter Wiki 双向回写（`77dc0a6`）+ RuleComplianceCompleteness/EvidenceGroundedness 指标（`c28e78f`）
- **HITL 数据链路**——RecordDecision 接通，/approve /reject 留痕到 SQLite（`20e96e2`）
- **P2B L0→L5 五层评估**（`967554c`）+ LLMNodeBuilder（`c5dd4bd`）+ 六阶段性能优化（`694733b`）
- **TUI 产品化**——品牌主题、命令系统、设置持久化、专业卡片、流式渲染提速 11×（`f645f03` 等）

### P3：专家盲测（11-12 月）→ 调整为"数据收集就绪"

**目标**：盲测 10 个案件，测量人工采纳率/修改率/拒绝率。

> **2026-07-17 共识调整**：专家盲测必须真人真测，**当前阶段只做数据收集就绪，不启动真实盲测**。
> 就绪标准：① 所有 HITL 触点（TUI /approve /reject、Server 审批端点、disclosure review_gate 续跑）全部留痕到 SQLite；
> ② P2A 全量 31 题 live 基线作为质量锚点；③ 盲测方案（案件来源/评价表/度量定义）成文备用。

## 五、下一阶段执行计划（2026-07-17 人机协作共识）

> 决策记录：P3 缓行只收集数据；协议层 Critical、巨型文件拆分、视觉空壳三项规划执行。

### Sprint 0：安检包（约 2-3 天）

| # | 事项 | 动机 |
|---|------|------|
| S1 | `agentcore/filecheckpoint` 覆盖率 33% → 补测试（Checkpoint 属安全红线） | 盲测前安全敏感区不带病 |
| S2 | `guardrails/` 测试仅 8 个函数 → 补强 | 同上 |
| S3 | CI 覆盖率纳入 `tools/` 子模块并重新生成 coverage.out（当前仅 24/73 个包的陈旧数据） | 覆盖率度量失真 |
| S4 | 快赢清理：`noveltyStubNode` 死代码、example/tui-demo2/3 残留收敛、根目录二进制入 .gitignore、openapi.yaml 补 3 条 disclosure 路由 | 仓库卫生 |

### Sprint 1：协议层 Critical 修复（Phase 7 遗留，C1-C8）

| # | 位置 | 问题 | 修复方向 |
|---|------|------|---------|
| C1 | `acp/server.go:399-410` | handleAuthenticate 空操作 | 实现真实 token 校验 |
| C2 | `server/server.go:813-838` | Agent 池 use-after-free | 引用计数或生命周期收敛 |
| C3 | `acp/server.go:362-396` | ClientCapabilities 无条件接受 | 能力声明白名单/降级 |
| C4 | `a2a/ws.go:17-23` | CheckOrigin 通配符 | Origin 白名单配置 |
| C5 | `a2a/ratelimit.go:90` | 反代后速率限制失效 | X-Forwarded-For 可信代理解析 |
| C6 | `mcp/http.go` 6 处 | io.ReadAll 无大小限制 | LimitReader 上限 |
| C7 | `mcp/config_discovery.go:58-76` | $PWD/.mcp.json 命令执行 | 配置文件所有权/确认校验 |
| C8 | `server/server.go:206-212` | 无 TLS | TLS 选项 + 反代部署文档 |

### Sprint 2：巨型文件拆分

| # | 文件 | 现状 | 方向 |
|---|------|------|------|
| F1 | `cmd/mady/main.go` | 装配上帝文件，38 次改动热点 | 按子命令/装配职责拆分 |
| F2 | `tools/computer_use.go` | 2564 行、零测试 | 按平台/职责拆分 + 补测试 |
| F3 | `a2a/server.go` (1728) / `tools/browser.go` (1321) / `server/server.go` (1222) / `mcp/http.go` (1026) / `mcp/client.go` (1008) | 自带 TODO(refactor) | 随改动就近拆分，不专门立项 |

### Sprint 3：视觉分析空壳处理

- `tools/vision.go:72` + `tools/browser_tool_handlers.go:611`：截图后返回占位文本，从未接视觉模型
- 方向：接入真实多模态模型（复用 provider 层）；无可用视觉模型时返回明确"暂不支持"错误，**禁止返回伪造分析**

### 封存项（P3 通过前不启动）

- DomainTrademark 第二垂直（停止规则 gate）
- Memory 生产集成（Phase 2 LLM 自动提取，当前默认关闭）
- 阶段 D 规则蒸馏（等真实 HITL 数据喂料）
- YAML 声明式工作流收尾（`workflows/patent_novelty.yaml` 孤儿文件）
- ACP Rebuildable 完整实现（SetSessionMode/Model 静默降级）
- TUI 显式状态机 handler 委托

### 文档同步债（随本轮执行一并清算）

- README 发展路线过时（Q4 计划已完成未更新）
- `docs/specs/README.md` 向量检索"待 Sign-off"与代码已上线矛盾
- `docs/design/tui-design-plan.md` Phase 1-6 未执行，5 个待决策问题悬置 → **已归档至 `docs/archive/tui-design-plan.md`（2026-07-18）**，部分 Phase 4 标记汇总至 `docs/decisions/phase4-backlog.md`
- `docs/evaluation-baseline-v0.7.md` §关键设计决策与重建后数据冲突未回改

## 六、停止规则

> **main 不绿 → 不加新功能**
>
> **Golden Set 不能说明质量差异 → 不换模型/Prompt**
>
> **首个专利工作流未通过专家盲测 → 不启动第二垂直**
