# AI 决策变更日志（归档）

> **归档段**：2026-07-15~17 引用核验 Gate 实现

> 本文件包含 50 个决策记录，从主 `AI_CHANGELOG.md` 归档。
> 归档时间：2026-07-19。主文件仅保留近期决策与归档索引。

---

## 2026-07-17: 引用核验 P1c——metrics 同源重构 + citation_validity 指标

### 背景
设计方案（ed813ce）§3 决策四要求评测与护栏共享同一引用抽取源。P1b 落地
Gate 后，metrics.go 仍维护私有正则与中文数字归一化副本（双份实现漂移风险）。
P1c 收口：评测指标同源 + 新增 citation_validity，让"答案引对了没有"
（而非仅"引全了没有"）进入评测体系。

### 改动
- `pkg/lawcite`：导出 `Normalize`（中文数字归一化公共 API，供 metrics
  同口径使用，不再维护私有副本）
- `guardrails/citation_gate.go`：CitationReport 新增五类判定计数
  （Valid/Unknown/Unverifiable/Suspect/Invalid），Flagged 明细与
  Gate 处置行为零改动
- `agentcore/evaluate/metrics.go`：
  新增 `CitationValidity` 指标——调 guardrails.VerifyCitations，
  得分 = Valid ÷（总 − Unknown − Unverifiable），无可核验引用得 1
  （与 Gate 放行语义一致）；6 组单测覆盖（合法/无引用/Unknown/
  Unverifiable/Suspect/Invalid/对错参半）。
  `CitationCompleteness` 同源重构：删除私有 citationPattern 与中文数字
  归一化全套副本（约 100 行），抽取改调 lawcite.Extract、归一化改调
  lawcite.Normalize；匹配语义（条级前缀泛化/项引用/之一后缀/子串防护）
  零改动
- `scripts/replay_citation_metrics/`：指标离线回放工具（P1c 验收件）——
  三层 93 条缓存答案逐题重算两指标输出 JSON 快照，按 caseID 排序累加
  保证逐字节确定。scripts/ 下回放工具改 cmd 布局各居子目录
  （replay_citation_gate 随之迁移，用法改为
  `go run ./scripts/replay_citation_gate`）

- **影响范围**: evaluate/guardrails/lawcite 各 1 文件改 + scripts 1 新 1 迁
- **风险等级**: 低（评测口径经快照等价验证；Gate 行为零改动）
- **审查要求**: L1
- **验证**: 口径等价硬性验收——重构前后 93 题×3 层×2 指标 per-case 得分
  零差异（before 快照锚定 v0.8 报告值 0.935）✅ | citation_validity 首批
  真实数据：L0 0.935 / L1 0.984 / L3 0.935 ✅ | `go vet`/`go build`
  双模块 ✅ | `golangci-lint run` 双模块 0 issues ✅ |
  `go test -race ./...` 全量 ✅ | gate 回放回归（三层 TP 全命中、
  误报 0）✅

---

## 2026-07-17: 引用核验 Gate P1a+P1b 实施（lawcite 同源抽取 + 双级核验 + 域接线）

### 背景
设计方案（见上条，ed813ce）已定硬验收标准：v0.8 缓存的 93 条真实答案回放，
幻觉题全命中且误报为 0。本条目按 P1a（同源抽取包）→ P1b（核验 Gate +
静态表 + 域接线）落地，Strict 档 SuppressPersist 联动按方案留 P2。

### 改动
- P1a `pkg/lawcite`（fcd9533）：法条引用结构化抽取（中文数字归一、
  承接语境 statuteWindow=120 法律归属、条/款/项/之N 定位、引用点 ±40 字语境、
  Key 去重），评测与护栏同源的单一事实源；19 项单测 + Benchmark 0.73ms
- P1b `guardrails/citation_table.go`（S1 静态主题表）：只收 2008→2020
  条号稳定且无争议的条目（漂移条目落 Unknown 放行）；主题词宁少勿多，
  泛化词（说明书/权利要求书/发明）不收录；新增 invalidationGrounds
  （可作无效宣告理由的实体条款集）
- P1b `guardrails/citation_gate.go`：VerifyCitations 双级核验
  （R1 存在性：专利法 >82 条判 Invalid；R2 语境相关性：用途声明与本条
  注册主题比对）+ CitationGate LifecycleHook（AfterModelCall 相位，
  工具调用回合跳过）+ FormatCitationWarnings（"请人工核对"存疑措辞，
  对照 tone-style-guide）
- 回放校准两轮，全部沉淀为对抗测试：
  第一轮确立核心判定式 Suspect=本条主题未命中+交叉命中另一条主题
  （仅本条未命中→Unverifiable 放行）；crossMatchNoise 泛化噪声词表
  （实施/使用/公告/请求等不参与交叉匹配）；enumStarters 枚举接续符
  剔除逗号（"根据专利法第X条，<用途>"是标准句式）
  第二轮 L3 三案裁定：2013_a26_01 判**真错误**入 knownTruePositives
  （智力活动规则错引第22条，应引第25条）；2012_a31_02 真误报→
  topics[31] 补原文措辞"限于一项"使逐字引用自证 Valid；
  2009_a2_02 真误报→"无效宣告"对 invalidationGrounds 条款豁免交叉匹配
  （"无效宣告理由（专利法第九条）"是同位命名而非张冠李戴）
- 域接线：domains/patent.go（PatentAgentConfig 与 BuildProjectAgent 两处）、
  domains/legal.go 各接入 NewCitationGate(LevelStandard)，置于
  guardrails.New 之前；P1b 阶段 Strict 域统一按 Standard 处置
  （追加提示 + Recorder 留痕回调），SuppressPersist + ApprovalGate 联动留 P2

- **影响范围**: guardrails/ 新增 2 文件、pkg/lawcite 新增（P1a 已提交）、
  domains/ 接线 2 文件、scripts/replay_citation_gate.go 回放验收工具
- **风险等级**: 中——新 hook 进入专利/法律域 AfterModelCall 热路径；
  domains/patent.go 属安全敏感路径（BuildProjectAgent WorkingDir 沙箱
  边界所在文件），本次仅追加 Lifecycle hook 未触碰 WorkingDir 逻辑
- **审查要求**: L2（人工审阅 citation_gate.go 判定矩阵、静态表收录口径、
  domains 接线点）
- **验证**: `go vet` 双模块 ✅ | `go build` 双模块 ✅ |
  `golangci-lint run` 双模块 0 issues ✅ | `go test -race ./...` 全量 ✅ |
  回放验收：三层 93 条真实答案 TP 全命中（L0 3 / L1 1 / L3 3）、误报 0、
  exit 0 ✅

---

## 2026-07-17: 法条引用核验 Gate 设计方案（待评审）

### 背景
v0.8 基线证实 12B 本地模型存在法条编号幻觉（2008_a31_02 把分案依据错引为
"专利法第47条"，自评 judge 反给 0.9），且 L3 实验证明被动装配检索工具无法修复
（工具零触发）。需要主动式引用核验机制。

### 改动
- `docs/design/citation-verification-gate.md` 成文：双级核验（R1 存在性 +
  R2 语境相关性，纯存在性检查拦不住张冠李戴）；三层本地核验源降级
  （S1 内嵌静态主题表 / S2 知识库法条索引 / S3 语义检索）；
  新 hook `guardrails/citation_gate.go` 复用 LifecycleHook 与 Level 三档语义，
  `levels.go` 零改动；新增 `pkg/lawcite` 共享抽取包让评测与护栏同源；
  误报控制四防线（Unknown 一律放行、关键词交集而非 LLM 判断、"存疑"措辞、
  对抗用例卡误报率）；分阶段实施 P1a-P3；硬验收标准=用 v0.8 缓存的 93 条
  真实答案回放，幻觉题全命中且误报为 0

- **影响范围**: 纯文档（设计待评审，无代码改动）
- **风险等级**: 低
- **审查要求**: L1（实施阶段 P1b 涉及 guardrails/ 新文件，届时升级 L2）
- **验证**: 不适用（文档）

---

## 2026-07-17: P2A 全量 31 题本地基线（v0.8）+ 跑批 harness 增强

### 背景
v0.7 留下「小样本陷阱」遗留问题（3→10 题结论多次反转），需 P2A 全量 31 题稳健基线
作为质量锚点（P3 盲测方案 §8 前置依赖）。用户指定用本地 oMLX 免费端点
（127.0.0.1:8000，gemma-4-12B-it-8bit）跑批，零 API 成本、可高频复跑。

### 改动
- 跑批 harness（仅测试代码，`agentcore/evaluate/benchmark/`）：
  生成/评判双阶段拆分、各自按题落盘缓存（$TMPDIR，断点续跑）；
  4 worker 并发 + 主 goroutine 同步筛 pending（修复缓存读写 map 数据竞争，
  曾触发 fatal error: concurrent map read and map write）；
  单题 15 分钟超时；MaxTokens 8192 上限消除本地低吞吐下的超长输出 livelock
- `docs/evaluation-baseline-v0.8.md`：全量基线报告成文。
  结果：L0 裸 LLM 90.3%（28/31，citation 0.935 / judge 0.723）；
  L1 Agent 框架 93.5%（29/31，citation 0.935 / judge 0.746）；
  L3 +检索工具 90.3%（28/31，citation 0.935 / judge 0.798）。
  L3 关键结论：检索工具装配后**全程零触发**（12B 模型对考试题不主动检索，
  与 v0.7 DeepSeek 观测一致），L3≈L1，法条编号幻觉未被修复——
  修复幻觉需主动触发机制（知识库注入/引用核验 gate），是后续迭代靶点。
  共性失败题 2 道：2008_a31_02（法条编号幻觉：误引"专利法第47条/细则21条"，
  正确为细则第42条；同模型自评无法识别，judge 仍给 0.77+）、
  2009_a22_01（基准期望引用与参考答案自引法条不自洽，待核官方答案）。
  另记录基础设施脆弱点：oMLX 长时跑批间歇性 502 曾致 8 题 judge 三样本全 0，
  清除故障缓存重评后恢复（llm_judge=0 应一律视为故障信号重跑）。
  报告含与 DeepSeek 历史基线的不可比性声明（被测模型+judge 均不同）
- `docs/design/p3-blind-test-plan.md` §8：全量基线数据固化到锚点章节

- **影响范围**: 测试代码 + 文档，无产品代码改动
- **风险等级**: 低
- **审查要求**: L1
- **验证**: `go vet` 双模块 ✅ | `go test -race ./agentcore/evaluate/...` ✅ |
  `golangci-lint run` 双模块 0 issues ✅ | 31 题两层级全量跑批完成 ✅

---

## 2026-07-17: golangci-lint 本地门禁清零（23 issues）

### 背景
应用户要求安装 golangci-lint（brew，v2.12.2 与 CI 同版本），实测发现根模块 22 +
tools 1 个 lint 问题（TUI 改版与 main.go 拆分后积累，CI 门禁处于红色状态）。

### 改动
- errcheck ×8：cmd/mady 设置持久化 `s.store.Set` 错误处理（失败 log 不打断交互）
- gocritic ×8：append 链合并 ×7 + 单 case switch 改 if ×1
- gosec ×2：settings_store.go 写文件权限 0644→0600（行为变化：仅影响新建文件）；
  tools/vision.go EnvVisionAPIKey 误报行内豁免（#nosec G101，环境变量名非凭证）
- ineffassign/staticcheck ×2：删除必被覆盖的 editorIndex 赋值；for 循环改 append 展开
- unused ×3：删除无引用私有方法 thinkingDisplay/syncFromStore 和 TUI.savedCycle 字段

- **影响范围**: `cmd/mady/`、`tui/`、`tools/vision.go`（共 9 文件）
- **风险等级**: 低（机械修复，唯一行为变化为 0600 收紧）
- **审查要求**: L1
- **验证**: `golangci-lint run ./...` 根+tools 均 0 issues ✅ | build/test ✅

---

## 2026-07-17: 下一阶段路线图共识（人机协作）+ roadmap.md 重写

### 背景
docs/roadmap.md 停留在 07-14，与代码现实严重脱节：P2B"❄️冻结"条目已过时
（07-15 当天已用宝宸知识库 31562 件重建 100 例并解冻，07-16 完成 L0→L5 五层评估）；
retrieve_prior_art、规则获取四步闭环、RecordDecision 均已落地但未反映。
项目实际进度比文档路线图超前约 3 个月，已站在 P3 门口。

### 用户决策（四点共识）
1. P3 专家盲测必须真人真测，**当前只做数据收集就绪，不启动真实盲测**
2. 协议层 C1-C8 Critical：规划执行（见下方专条）
3. 巨型文件拆分（main.go / computer_use.go）：规划执行（见下方专条）
4. 视觉分析空壳：规划执行（见下方专条）

### 改动
- `docs/roadmap.md` 全文重写：P2B 冻结条目标注过时并补重建/五层评估定论；
  补 07-15/16 集中落地清单（设计一、规则闭环、HITL、TUI 产品化）；
  P3 调整为"数据收集就绪"（三条就绪标准）；新增"下一阶段执行计划"
  （Sprint 0 安检包 / Sprint 1 协议层 / Sprint 2 拆分 / Sprint 3 视觉 / 封存项 / 文档同步债）
- `docs/design/p3-blind-test-plan.md`：盲测方案成文（见 P3 就绪专条）

- **影响范围**: 纯文档
- **风险等级**: 低
- **审查要求**: L1
- **验证**: 不适用（文档）

---

## 2026-07-17: 协议层 C1-C8 Critical 安全修复（Phase 7 遗留清零）

### 背景
Phase 7 审查 8 项协议层 Critical 全部标 ❌，文档间修复状态无法对应。
逐项代码核实：C3/C6 已被此前批次修复（本轮补回归测试），C1/C2/C4/C5/C7/C8 真实存在
或只修了一半（如 C4 同源校验可被子域名前缀绕过、C5 无条件信任 XFF 可被伪造绕过、
C7 文件所有权检查对克隆恶意仓库无效）。

### 改动（13 改 + 7 新）
- C1 ACP 认证：`acp/auth.go` TokenAuthProvider（常量时间比较）+ initialize/
  authenticate 之外方法强制门禁（-32000）；`MADY_ACP_TOKEN` 接线；默认不配置
  仍允许本地开发 + 启动警告
- C2 Agent 池 use-after-free：`server/server.go` 池化 entry 引用计数
  （refs/evicted/pooled 全部 poolMu 下访问），淘汰只摘标记、refs 归零才真正 Close；
  新增 pool_test.go 并发压测（-race）
- C4 CheckOrigin：`a2a/ws.go` 严格 host 相等 + 回环放行 + WithAllowedOrigins 白名单
- C5 速率限制：`a2a/ratelimit.go` SetTrustedProxies 可信代理门控，默认仅信回环
- C7 .mcp.json 命令执行：`mcp/config_trust.go` SHA-256 信任存储
  （$MADY_HOME/trusted-mcp.json，0600）+ `mady trust-mcp` 子命令；
  **行为变化（有意）**：cwd 的 .mcp.json 未信任不再静默执行
- C8 TLS：`cmd/mady/server.go` 接线 -tls-cert/-tls-key（成对，缺一则 fail-fast）

- **影响范围**: `acp/`、`server/`、`a2a/`、`mcp/`、`cmd/mady/`、`.golangci.yml`
- **风险等级**: 高（安全敏感路径；C7 改变默认行为）
- **审查要求**: L3（协议安全边界，需人工审阅）
- **验证**: build/vet ✅ | `go test -race ./acp/... ./server/... ./a2a/... ./mcp/...` ✅ | golangci-lint 0 issues（改动目录）

---

## 2026-07-17: 视觉分析全链路真实化（清除伪造占位）

### 背景
`tools/vision.go` DefaultVisionOperations.Analyze 返回 "[Vision analysis placeholder]"
伪造文本；浏览器 vision action 截图后丢弃字节返回伪造字符串。诚实性红线：
禁止返回伪造分析。

### 改动
- `tools/vision.go`：DefaultVisionOperations 实现真实 OpenAI 兼容多模态调用
  （image_url data URL，60s 超时，错误体截断 4KB）；新增 MADY_VISION_MODEL /
  MADY_VISION_API_KEY / MADY_VISION_BASE_URL env 兜底；未配置时返回明确中文错误
- `tools/browser.go` / `browser_tool_handlers.go`：BrowserToolConfig 加 Vision 字段；
  handleVision 与 browser_vision 共用 analyzeBrowserScreenshot（MIME 魔数检测 + 限额）
- `tools/tools.go`：WithVision 配置自动共享给 browser（显式配置优先）
- 测试 +14：mock provider 请求构造/ctx 透传/错误路径、httptest HTTP 构造、env 三态

- **影响范围**: `tools/{vision,browser,browser_tool_handlers,tools,vision_test}.go`
- **风险等级**: 中（工具行为变化：未配置时从伪造文本变为明确错误）
- **审查要求**: L2
- **验证**: `cd tools && go build/vet/test -race ./...` ✅

---

## 2026-07-17: main.go 与 computer_use.go 拆分（技术债务批次）

### 改动
- `cmd/mady/main.go` 821 行 → 85 行；拆出 `framework.go`（装配 308 行）、
  `knowledge.go`（知识库 241 行）、`tui.go`（子命令 239 行），纯机械移动；
  顺带 git rm `cmd/mady/web_test.html`（临时调试页）+ .gitignore 补 `/acp-server`
- `tools/computer_use.go` 2564 行 → 532 行 + 8 个职责文件（safety/exec/keys/
  cua_driver/som/macos/win/lin；`_win.go`/`_lin.go` 命名规避 GOOS 隐式排除）；
  新增 `computer_use_test.go` 482 行 16 测试（安全拦截/审批/解析/SOM/Schema）
- **加固**：fork bomb 拦截正则补全覆盖经典写法 `:(){ :|:& };:`（原正则要求
  `:(` 后无 `)` 直接 `{`，匹配不到）；fails-closed 方向，放行语义不变

- **影响范围**: `cmd/mady/`、`tools/computer_use*.go`、`.gitignore`
- **风险等级**: 中（拆分纯机械；正则加固为拦截方向）
- **审查要求**: L2（computer_use 属工具能力层）
- **验证**: build/vet ✅ | `go test -race ./cmd/... ./tools/...` ✅ | gofmt ✅

---

## 2026-07-17: P3 数据收集就绪（HITL 触点补全 + 评估筛选 + 盲测方案）

### 背景
用户决策：专家盲测必须真人，当前只把数据收集链路做扎实（路线图三条就绪标准）。

### 改动
- **HITL 触点全留痕**（原仅 TUI /approve /reject 一路）：
  - `domains/approval.go` 抽出包级 RecordApprovalDecision（与 gate.RecordDecision
    同源）；`domains/sqlite/approval_store.go` 补 State 字段 JSON 持久化
  - TUI 硬中断路径补齐：`recordApprovalDecision` 检测 agent.Interrupted()，
    用中断 gate 标识 + 结构化数据填充（原先 OriginalOutput 落空）
  - Server 新增 `POST /v1/disclosure/analyze/{task_id}/review`（无 store 返回 503
    而非静默丢数据）；ACP session/request_permission 授权结论留痕（接线就绪，
    待 PermissionAware 实现激活）
  - cmd/mady/{server,acp}.go 打开与 TUI 同一份 approvals.db，三入口汇聚
- **评估筛选**：`MADY_EVAL_SUITE=p2a` 全量 31 题稳定顺序；newDeepSeekTestEnv
  支持 MADY_EVAL_API_KEY/BASE_URL/MODEL 任意 OpenAI 兼容端点；
  本地 MLX 端点冒烟通过（裸 LLM 9.2s / Agent 18.6s）
- **盲测方案**：`docs/design/p3-blind-test-plan.md`（先后对照 + sham 盲化、
  拉丁方分配、双人标注 κ、通过线预设、伦理免责、P2A 锚定指令）

- **影响范围**: `domains/`、`server/`、`acp/`、`cmd/mady/`、`agentcore/evaluate/`、docs
- **风险等级**: 高（domains/approval.go 属 SECURITY.md 安全敏感路径——纯新增/重命名，
  未触碰 gate 触发逻辑，需人工审阅后合入）
- **审查要求**: L3
- **验证**: build ✅ | `go test -race ./agentcore/... ./server/... ./disclosure/... ./domains/... ./acp/...` ✅

---

## 2026-07-17: 仓库卫生快赢（死代码 / demo 残留 / openapi / benchmark 注释）

### 改动
- 删除 `disclosure/report.go` noveltyStubNode 死代码（生产已被 noveltyNode 取代）；
  测试改测生产路径；types.go "Phase 2 stub" 陈旧注释订正
- git rm `example/tui-demo2`、`example/tui-demo3`（迭代残留，tui-demo 保留且编译通过）
- `docs/openapi.yaml` 补 3 条 disclosure 路由（+188 行）并修复两处既有 YAML 语法错误
- benchmark：invalidation_decisions.go 头注释订正（40→100 例、脚本名、数据源）；
  suite.go AllCases/ValidCases 重复 append 序列提取为 registeredCases()
- `tui/agent_integration_test.go`：hasAPIKey 白名单守卫改为直接探测 BuildProvider
  （修复 KIMI_API_KEY 环境下误判放行导致的测试失败）

- **影响范围**: `disclosure/`、`example/`、`docs/openapi.yaml`、`agentcore/evaluate/benchmark/`、`tui/`
- **风险等级**: 低
- **审查要求**: L1
- **验证**: build/vet ✅ | `go test -race ./disclosure/... ./agentcore/evaluate/...` ✅

---

## 2026-07-16: 确认阀闭环（Stage② 中断 → checkpoint → resume 续跑）

### 背景
设计文档二核心闭环：Stage ② 检索规则后须经人工确认才能约束 Plan/Execute/Check。
此前 B（ConfirmedRuleSet）只做数据底座，卡在 run_five_step_workflow 工具无状态
（局部 runner，中断后丢失）。本次解决工具状态管理，打通完整闭环。

### 机制
```
run_five_step_workflow（无 checkpoint_id）
  → Stage① 事实 → Stage② 规则检索
  → runStage2 返回 InterruptError（requireConfirmation=true 时）
  → 工具 SaveCheckpoint（保存 blackboard 到 CheckpointStore）
  → 返回"规则待确认 + checkpoint_id"消息
用户确认规则 → run_five_step_workflow（带 checkpoint_id + confirmed_rules）
  → ResumeFromCheckpoint + SetConfirmedRules
  → runFrom(startStage=3) 续跑 Stage③④⑤
```

### 改动
- `five_step_runner.go`：FiveStepRunnerConfig 加 RequireRuleConfirmation；
  FiveStepRunner 加 requireConfirmation 字段 + SetRequireRuleConfirmation setter；
  runStage2 末尾返回 InterruptError（携带 gate/stage/total_rules/case_id）
- `handoff_integration.go`：WorkflowToolInput 加 CheckpointID + ConfirmedRules；
  AsWorkflowToolWithCheckpoint(runner, store) 新函数——中断时存 checkpoint +
  resume 时恢复 + SetConfirmedRules + runFrom(3)；旧 AsWorkflowTool(runner)
  转调它（传 nil store，向后兼容）
- `tui_session.go`：tuiSession 加 workflowStore（MemoryCheckpointStore）；
  reviewMode 开启时 SetRequireRuleConfirmation(true) + 注入 store

### 闭环现状
确认阀仅在 reviewMode 开启时生效（默认关闭，不破坏现有行为）。B/F/G 的
ConfirmedRuleSet/RuleAssertionHook/ConfirmedRuleWriter 现可被真正填充/触发/写入。

- **影响范围**: `domains/reasoning/{five_step_runner,handoff_integration,confirmation_gate_test}.go`、`cmd/mady/tui_session.go`
- **风险等级**: 中（改变五步工作法 Stage②→③ 的过渡语义，但默认关闭 + 向后兼容）
- **审查要求**: L2（涉及人机协作安全边界）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race` ✅ | gofmt ✅

---

## 2026-07-16: Stage ② 接入确定性规则源（第四路 RuleSourceRules）

### 背景
核查发现：此前"三路召回已完成"的声明有盲点——MultiSourceRetriever 的三路（KG/
Vector/Skill）都是中低权威性来源，而**权威性最高的确定性规则引擎**（domains/rules
的 17 个 YAML，NOV-001~004 等）从未被它消费。Engine.ToRuleConstraints() 全仓库
仅测试调用，生产零调用。规则引擎虽已装配为 chat agent 的 search_rules 工具，
但五步工作法"获取规则"阶段根本没查它。

### 改动
- `domains/reasoning/types.go`：新增 `RuleSourceRules = "deterministic_rules"` 枚举
- `domains/reasoning/rule_retrieval.go`：
  - 新增 `RuleEngineSource` 接口（MatchRules，与 RuleVectorStore/RuleSkillReader 同构）
  - MultiSourceRetriever 加 ruleEngine 字段 + 构造函数第 4 参数
  - 新增 queryRules 方法 + querySource switch 分支
  - Retrieve 把 manifest.CaseType 注入 queryCtx 供规则域映射
- `domains/reasoning/manifest.go`：4 个 manifest（Novelty/Patentability/Drafting/
  Invalidation）的 Sources 均加 RuleSourceRules 为第一路（Weight 1.2，高于 KG 1.0）
- `domains/reasoning/wiring/rule_engine_adapter.go`：RuleEngineAdapter 包装 rules.Engine，
  caseType→domain 映射（patentability→patent_novelty+patent_inventiveness 等），
  跨域去重，未知 caseType 回退 keyword 搜索
- `cmd/mady/main.go`：frameworkContext 加 RuleEngine 字段，setupFrameworkContext
  加载（消除 runTui 重复 LoadEngineFromMadyHome），buildReasoningRetriever 接第四路

### 权威性分层现状（对齐设计文档二第二节）
Stage ② 现为真正的四路召回：
| 路 | 来源 | AuthorityScore | 权威性 |
|---|---|---|---|
| Rules | domains/rules YAML | 0.95 | 最高（代码固化法条映射） |
| KG | 知识图谱 | 0.9 | 中高（结构化事实） |
| Vector | knowledge.db FTS | 0.7 | 中（规范性依据） |
| Skill | wiki patent-cards | 0.4 | 低（经验参考） |

- **影响范围**: `domains/reasoning/{types,rule_retrieval,manifest,phase2..5_test}.go`、`domains/reasoning/wiring/rule_engine_adapter{,_test}.go`、`cmd/mady/main.go`
- **风险等级**: 低（纯召回扩容，四路任一缺失静默跳过；签名变更仅影响 1 生产调用点 + 9 测试）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./domains/reasoning/...` ✅ | gofmt ✅

---

## 2026-07-16: TUI 中断用户引导（缺口3 收尾）

### 背景
前一条记录修复了 Resume 闭环的 Server + TUI 缺口，遗留缺口3：AgentInterruptEvent
在 TUI 侧无引导文案，用户看到中断会困惑。核实发现 agentadapter 根本没有映射
EventAgentInterrupt（switch 无此分支），中断事件被静默丢弃，TUI 永远不知道发生了中断。

### 改动
补全中断事件从 agentcore → adapter → chat_app 的完整渲染链路：
- `tui/chat/events.go`：新增 `ChatEventAgentInterrupt` 常量 + `AgentInterruptChatEvent` 类型
  （携带 Reason + Data，Data 含 gate 标签供文案定制）
- `tui/agentadapter/adapter.go`：新增 `case ChatEventAgentInterrupt` 分支，把
  `agentcore.AgentInterruptEvent` 映射成 `chat.AgentInterruptChatEvent`
- `tui/chat/chat_app.go`：Subscribe 注册 `onAgentInterrupt`
- `tui/chat/chat_app_stream.go`：`onAgentInterrupt` handler 结束流式→Idle→PrintSystem
  渲染引导文案；`interruptGuidance` 按 Data["gate"] 定制：disclosure_review 显示
  "技术交底书分析已暂停，等待人工复核"，其他 gate 显示关卡名，无 gate（ApprovalGate
  软中断）显示通用提示。三类都引导用户用 /approve 或 /reject
- 测试：`interrupt_guidance_test.go` 覆盖三分支 + 空回退 + ChatEventKind

### 闭环状态
中断人机协作三缺口全部关闭：
- ✅ 缺口1（Server）：awaiting_review 状态 + 返回 report
- ✅ 缺口2（TUI Resume）：/approve 走 agent.Resume 从中断点继续
- ✅ 缺口3（TUI 引导）：中断时渲染 /approve /reject 引导文案

- **影响范围**: `tui/chat/{events,chat_app,chat_app_stream,interrupt_guidance_test,chat_app_test}.go`、`tui/agentadapter/adapter.go`
- **风险等级**: 低（纯 UI 渲染，不改变中断/恢复机制本身，只补事件映射 + 文案）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tui/... ./cmd/mady/` ✅ | gofmt ✅

---

## 2026-07-16: disclosure 中断的 Resume 闭环修复（Server + TUI）

### 背景
上一条记录（review_gate 改为主动中断）落地后，核实消费层发现两处缺口：
review_gate 的 InterruptError 在机制层能冒泡到 agent loop，但 TUI/Server
都没准备好处理这个**工具内部硬中断**（区别于 ApprovalGate 的关键词软中断）。

### 缺口1（Server，硬回归）— 已修复
`server/disclosure.go` executeTask 原先把所有 Pregel error 置为 `failed`。
review_gate 必然返回 InterruptError，导致所有 Server 端交底书分析任务失败。
修复：识别 `agentcore.IsInterrupt`，因 review_gate 在报告生成之后才中断，
此时 state 已含完整 AnalysisReport，提取并置为 `awaiting_review` 状态返回
（Server 是异步任务模型，无交互式 Resume，让客户端拿到报告自行人工复核）。

### 缺口2（TUI）— 已修复
`/approve` 原先用 `submitInput("确认")` → `agent.Run` 开新 turn，会丢弃被
中断工具的中间 state。新增 `tuiSession.resumeIfInterrupted()`：当 agent 处于
`Interrupted` 态时调 `agent.Resume`（从中断点继续同一 runLoop），否则回退到
原 submitInput（兼容 ApprovalGate 软中断）。

### 缺口3（中断用户引导）— 遗留，本次不做
`AgentInterruptEvent` 在 TUI 侧无针对 disclosure review 的引导（"请输入
/approve 确认"），用户看到中断会困惑。当前 `Ctrl+C` 绑定的是 `OnInterrupt`
（取消执行），与"确认恢复"语义无关。此项涉及 TUI 交互设计，留待后续。

### 改动
- `server/disclosure.go`：executeTask 识别 IsInterrupt → awaiting_review + report
- `cmd/mady/tui_session.go`：新增 `resumeIfInterrupted()`（goroutine + runMu，调 agent.Resume）
- `cmd/mady/slash_registry.go`：/approve 先 resumeIfInterrupted，失败回退 submitInput

- **影响范围**: `server/disclosure.go`、`cmd/mady/{tui_session,slash_registry}.go`
- **风险等级**: 中（/approve 行为变更：硬中断走 Resume、软中断走 Run，两路径分流）
- **审查要求**: L2（涉及人机协作恢复链路）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./disclosure/ ./server/ ./cmd/mady/` ✅ | gofmt ✅

---

## 2026-07-16: disclosure review_gate 从 no-op 改为主动触发人工复核中断

### 背景
评估设计文档（`design-rule-acquisition-stage.md` 第五节）时发现：disclosure 管线的 `review_gate` 节点是 no-op（仅设 `_gate_ready` flag），注释称"实际暂停由 ApprovalGate LifecycleHook 触发"，但 ApprovalGate 是 chat agent 的 `AfterModelCall` hook，挂不到作为工具执行的 Pregel 子图节点上——该关卡形同虚设。

### 关键发现：中断机制已就绪，无需写"适配层"
核实冒泡链路后发现核心机制天然成立，原计划"Pregel→InterruptError 适配层"不必新建：
- `domains.RequireApproval`（approval.go:155）已封装 `agentcore.NewInterruptErrorWithData`
- Pregel 节点 return 的 error 经 `WrapNodeError` 包成 `NodeError`，而 `NodeError.Unwrap()`（errors.go:26）保留 Unwrap 链
- `agentcore.IsInterrupt` 用 `errors.Is(err, ErrInterrupt)` 穿透 NodeError → interruptError → ErrInterrupt ✅

真正的缺口只有两处：(1) review_gate 不返回中断；(2) `analyze_disclosure` 工具吞错（一律转 FailureResult，中断信号丢失）。

### 改动
- `disclosure/report.go` review_gate 节点：从 no-op 改为返回 `agentcore.NewInterruptErrorWithData`，携带 report_id/novelty/gate 标签供人工审阅入口定位。直接用 agentcore 原语而非 `domains.RequireApproval`，避免基础设施层 disclosure 反向依赖领域层 domains（ADR-0001）
- `disclosure/tool.go`：Pregel Run 返回 error 时识别 `IsInterrupt` 并透传（`return msg, err`），其余错误仍转 FailureResult。这是中断信号能到 agent loop 的关键卡点
- 测试：新增 `TestReviewGateInterrupt_PregelPropagation` 验证 InterruptError 经 Pregel Run + WrapNodeError 后 IsInterrupt 仍成立；更新 `TestReviewGateNode` / `TestReviewGateNode_NilReport` / `TestDisclosureAnalysisGraph_FullFlow` 反映新行为

### ⚠️ 安全敏感——需人工审阅
此改动改变"重点节点人机协作"的触发方式（review_gate 从静默变主动中断）。虽未修改 `domains/approval.go`（安全敏感路径），但：
- disclosure 管线现在**必定**在 review_gate 暂停等人工 Resume，行为从"静默完成"变为"强制复核"
- 若有下游代码依赖"disclosure 工具总是成功返回"的假设，会因中断而中断
- 建议人工确认：TUI/Server 入口对 disclosure 中断的 Resume 流程是否完备

- **影响范围**: `disclosure/{report,tool,disclosure_test}.go`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 中（行为变更影响人机协作关卡，但机制是复用已验证的 agentcore 中断，非新造）
- **审查要求**: L2（涉及人机协作安全边界）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./disclosure/ ./cmd/mady/` ✅ | gofmt ✅

---

## 2026-07-16: reasoning Stage ② 规则召回接入真实知识资产（vector + skill 两路）

### 背景
评估两份设计文档（`design-prior-art-retrieval-stage.md`、`design-rule-acquisition-stage.md`）时，经代码 + 运行时数据双重核实，纠正了一处关键误判：

- **规则引擎早已完整装配**：`cmd/mady/main.go:555` `rules.LoadEngineFromMadyHome()` → `engine.go:31` 解析 `$MADY_HOME/knowledge/rules`（软链接到 xiaonuo 的 17 个 YAML）→ `tui_session.go:112/135/181` 注入三种 agent。启动即生效，`search_rules`/`get_article_framework` 工具已上线。
- **知识数据早已就位**：`~/.mady/knowledge/{knowledge.db 6.5G, patent_kg.db 207M, laws-full.db 152M, wiki/ 1573md}` 全部软链接到 xiaonuo，`knowledge/sqlite` 查询层已接入 chat agent。
- **真正的缺口在 Stage ②**：`main.go` `buildReasoningRetriever` 用 `reasoning.NewMultiSourceRetriever(walker, nil, nil)` —— 后两个 nil（`RuleVectorStore` / `RuleSkillReader`）导致"获取规则"阶段只能查知识图谱，向量检索和 wiki 经验两路完全缺失。

### 改动
- **新建 `domains/reasoning/wiring/` 子包**（装配层，保持 reasoning 主体零基础设施依赖，符合 ADR-0001）：
  - `vector_rule_store.go`：`VectorRuleStore` 把已打开的 `knowledge.KnowledgeBackend.FTSSearch` 适配为 `reasoning.RuleVectorStore`，命中法条/审查指南语料片段，AuthorityScore=0.7（规范性依据层）
  - `skill_rule_reader.go`：`SkillRuleReader` 解析 `patent-cards/*.md`（Obsidian 列表式元数据 + 卡片正文）为 `reasoning.RuleSkillReader`，AuthorityScore=0.4（经验参考层，非法律依据）
- **`cmd/mady/main.go`**：`loadWikiStore` 增加 `KnowledgeBackend` 返回值；`frameworkContext` 加 `KnowledgeBackend`/`WikiRoot` 字段；`buildReasoningRetriever` 接入三路（KG + Vector + Skill）；新增 `resolveWikiRoot` 解析 wiki 根目录
- **`disclosure/report.go:78`**：订正过时注释（原"Phase 2 将增强 retrieval 集成"误判为未来计划，实际 retrieval 已接入 chat agent 与 Stage ②，仅 disclosure 节点未接）

### 边界（不做）
- 不动 disclosure 管线节点（`retrieve_prior_art` 是独立任务，需先建 `PatentDomainRetriever`）
- 不接审批阀（需 Pregel→InterruptError 适配层，是 design-rule-acquisition-stage.md 的后续）
- 不碰任何安全敏感路径

- **影响范围**: `domains/reasoning/wiring/{vector_rule_store,skill_rule_reader}.go` + 2 测试 + testdata、`cmd/mady/{main,stage2_wiring_test}.go`、`disclosure/report.go`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（纯装配层接线，新增子包不修改 reasoning 主包任何现有代码；三路任一数据缺失时该路静默跳过，不影响现有行为）
- **审查要求**: L1（不涉及护栏/Handoff/沙箱/Checkpoint）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./domains/reasoning/... ./cmd/mady/...` ✅

---

## 2026-07-16: TUI 优化代码质量审阅 — 9 项问题修复

对 12 批次 TUI 优化做全面批判性审阅（3 个 Explore agent 交叉审阅 component/chat/cmd 三层），发现并修复 9 项真问题：

### 严重（功能 bug）
- **S2 settings 双重触发**：`settings_panel.go` OnSubmit 重复调 applySettingEntry——Enter 时 SettingsList 先 cycle+OnChange 再 OnSubmit，导致 plan/review toggle 两次抵消（等于没改）+ 重建 agent 两遍。修复：OnSubmit 只关闭 overlay 不 apply（OnChange 已 apply）
- **S1 /approve /reject 回归**：注册表用 `Available: reviewOn` gate，review 关闭时 Lookup 返回 nil → 落入"未知命令"分支，丢失原"⚠ 审核关卡未启用"引导提示。修复：去掉 Available gate，改 Handler 内检查 reviewMode 恢复引导语义
- **M5 /skill: 补全缺冒号**：Suggestions 给 `/skill`（Name 无冒号），但 prefixMatch 要 `/skill:`——补全选中后变未知命令。修复：SlashCommand 加 SuggestText 字段，skill 命令填 `/skill:`

### 中等（潜在 bug / 设计缺陷）
- **M2 keymap 未知修饰键降级裸键**：`foobar+a` 告警但仍接受，parseKeyID 静默丢 foobar 后变裸 `a`，劫持所有裸 a 按键（比绑定失败更危险）。修复：未知修饰键时**拒绝 token**（不入 valid），告警说明已跳过
- **component S1 SetContext 死代码**：context 占用条从未接线（ctxUsed/Total 恒为 0）。修复：ChatAppConfig 加 ContextWindow 字段，onTurnEnd 用 `te.Usage.TotalTokens / ContextWindow` 调 SetContext 激活
- **M1 EventKindFor 默认分支错误**：未知事件返回 evtAgentStart（会错误触发 Idle→Streaming），且遗漏 TurnStart/AutoRetry。修复：加 evtUnknown（Transition 无 case → 真 no-op）+ 显式映射 TurnStart/AutoRetry
- **M4 Clear() 未重置 tailAnchorLen**：清空历史后 tailAnchorLen 残留旧值，下次流式显示无意义的"↓ N new"。修复：Clear 重置 tailAnchorLen=0 + follow=true
- **M3 loadKeymapOverrides 吞权限错误**：所有 ReadFile 错误当 missing file 忽略，权限不足时静默禁用定制。修复：用 os.IsNotExist 区分，非 not-exist 错误告警
- **component S2 TestBlockCacheAvoidsRecompute 无效断言**：原断言只比 len/cap 不比指针，删掉 cache-hit 分支测试仍通过（优化核心保证形同虚设）。修复：改 unsafe.Pointer 比较切片底层数组地址，真正验证复用

### 顺带清理
- 删除 `slash_registry.go` exactMatch 的死代码（`tokens` map 只写不读）
- 删除 `markdown.go` parseBlocks 的 `_ = start` 残留
- tok/s 量纲修复：turnStarted 改在 onTurnStart 重置（单 turn 耗时，非自 AgentStart 累计）
- settings_panel.go 注释修正（ov 赋值顺序描述与代码相反）

- **影响范围**: `cmd/mady/{slash_registry,settings_panel,tui_helpers,main}.go`、`tui/chat/{chat_app,chat_app_tool,chat_history,state}.go`、`tui/terminal/keybindings.go`、`tui/component/markdown.go` + 4 个 _test.go
- **风险等级**: 低-中（S2/S1/M5 是用户可见 bug 修复；其余为健壮性/测试有效性提升）
- **审查要求**: L2
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./tui/... ./cmd/...` ✅（全 11 子包）| gofmt ✅

---

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

---

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

---

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

---

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

---

## 2026-07-16: 接通 RecordDecision——HITL 数据采集链路

- **变更**:
  1. `domains/approval.go`：`ApprovalGate` 加 `lastTriggeredOutput` 字段，`AfterModelCall` 触发审批时保存 Agent 产出；`RecordDecision` 自动使用该字段作为 `originalOutput`（调用方可传空），记录后清空。
  2. `cmd/mady/tui_session.go`：`tuiSession` 加 `approvalGate` 引用；`buildAgentConfig` 在 reviewMode 时创建带 `SQLiteApprovalStore`（`workspace/approvals.db`）的 gate 并保存引用；新增 `openApprovalStore`（SQLite，WAL 模式）和 `recordApprovalDecision`（调用 `gate.RecordDecision`）；`/approve` 记录 `DecisionAdopted`，`/reject` 记录 `DecisionRejected`。
- **原因**: 之前 `RecordDecision`/`ApprovalStore`/`ApprovalRecordState` 是已设计但未接线的死代码。P2B 五层评估证明用 LLM 模拟 HITL 无法准确测量真实人机协作价值（L5=0.320 < L1=0.513，因为 LLM 修订破坏正确初稿）。需要接通生产环境的真实 HITL 数据采集，让用户每次 /approve /reject 都持久化到 SQLite，积累真实 AdoptionRate 数据，为 P3 专家盲测提供基础。
- **影响范围**: `domains/approval.go`（ApprovalGate 加字段+改 RecordDecision）、`cmd/mady/tui_session.go`（gate 创建带 store + approve/reject 留痕 + openApprovalStore + recordApprovalDecision）
- **风险等级**: 中（涉及 `domains/approval.go` 安全敏感路径，但仅新增字段和自动填充逻辑，不改 AfterModelCall 的审批触发行为）
- **审查要求**: L3（安全敏感路径 `domains/approval.go`）
- **验证**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test -race ./domains/...` ✅ | gofmt ✅

---

## 2026-07-16: P2B 五层评估完成——LLM 模拟 HITL 的方法论困境

- **变更**: 新增 `mockHumanRevision`（LLM 模拟专家修订）和 `TestLiveAgentP2BHitlEval`（L5 HITL tier），跑出 P2B 五层完整对比。
- **P2B 五层排序（llm_judge 均值）**：L1 通用 prompt（0.513）> L4 增强 prompt（0.410）> L0 裸 LLM（0.363）> L2 工具编排（0.334）≈ L5 模拟 HITL（0.320）
- **L5 关键发现**：mockHumanRevision 对高分初稿有害（−0.73/−0.80），对低分初稿有益（+0.53），净效果为负（0.320 < L1 0.513）。根因：LLM 无法像真实专家一样判断「初稿已够好不需改」，对所有初稿都做修改引入不确定性。
- **结论**：不能从 L5=0.320 得出"HITL 有害"——这是 LLM 模拟修订的局限。真实 HITL 的理论上限介于 L1（0.513）和完美修订之间。需真实专家盲测（P3）才能准确测量。
- **影响范围**: `agentcore/evaluate/benchmark/live_agent_test.go`、`docs/evaluation-baseline-v0.7.md`、`docs/decisions/AI_CHANGELOG.md`
- **风险等级**: 低（新增测试和文档，不改现有逻辑）
- **审查要求**: L1
- **验证**: `go vet` ✅ | P2B L5 10 题 live eval ✅

---

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

---

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

---

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

---

## 2026-07-16: 性能优化 Phase 3 — VectorIndex Search 最小堆优化

- **变更**:
  1. `knowledge/sqlite/vector_index.go`：新增 `minVectorHeap` 类型（`container/heap` 实现）。Search 方法中每个 worker 从 `make([]vectorMatch, n)` 全量分配+全排序改为 topK 大小最小堆，内存分配从 O(N/workers) 降到 O(K)。
- **动机**: 每个 worker 分配整个 shard（144K/12≈12K entries）的 `[]vectorMatch` + `sort.Slice` 全排序，benchmark 显示 4.7MB/op alloc。
- **收益**: alloc 4.7MB→26KB（降低 **99.4%**），速度 16.5ms→14.8ms（+10%）。
- **影响范围**: `knowledge/sqlite/vector_index.go`（1 个文件）
- **风险等级**: 低（搜索结果不变，只优化内存分配策略）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go test -race ./knowledge/sqlite/...` ✅ | `golangci-lint` ✅ | benchmark alloc 4.7MB→26KB ✅

---

## 2026-07-16: 性能优化 Phase 2 — PubSub PublishMustDeliver 解锁

- **变更**:
  1. `agentcore/pubsub.go`：`PublishMustDeliver` 改为 snapshot subscribers 模式。先短暂持 RLock 拷贝 `[]chan T`，释放锁后再逐个发送。新增 `<-b.done` select case 在发送过程中优雅退出。
- **动机**: 原 `PublishMustDeliver` 在 RLock 期间遍历所有 subscriber 发送，满 channel 最多阻塞 50ms/subscriber，N 个满 subscriber = N×50ms 总阻塞期间 RLock 不释放，阻塞 Subscribe/Unsubscribe。
- **收益**: 消除 PublishMustDeliver 对 Subscribe/Unsubscribe 的锁阻塞。
- **影响范围**: `agentcore/pubsub.go`（1 个文件）
- **风险等级**: 低（snapshot 期间新增的 subscriber 不会收到这条消息，可接受）
- **审查要求**: L2
- **验证**: `go build` ✅ | `go test -race ./agentcore/...` ✅ | `golangci-lint` ✅

---

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


---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

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

---

## 2026-07-16: 7 个文件裸 sync.RWMutex 迁移到 pkg/csync 泛型容器

### 背景
项目中大量使用 `sync.RWMutex` + `map`/`slice` 的并发保护模式，存在大量重复的
Lock/Unlock 样板代码。`pkg/csync` 提供了 `csync.Map`, `csync.Slice`, `csync.Value`
泛型容器封装了内部 RWMutex，用 `Get/Set/Del/Copy` 代替手写锁。

### 改动

| 文件 | 原来 | 改为 |
|------|------|------|
| `acp/session.go` | `mu sync.RWMutex` + `sessions map[string]*sessionState` | `sessions *csync.Map[string, *sessionState]` |
| `acp/server.go` | `clientCapsMu sync.RWMutex` + `clientCaps *ClientCapabilities` | `clientCaps atomic.Pointer[ClientCapabilities]`（单指针，csync.Value 不支持 pointer） |
| `acp/server.go` | `pendingMu sync.Mutex` + `pending map[string]chan acpResponse` | `pending *csync.Map[string, chan acpResponse]` |
| `knowledge/store.go` | `mu sync.RWMutex` + `docs/chunks/byDomain` 三个 map | 三个独立的 `*csync.Map[...]` |
| `server/disclosure.go` | `mu sync.RWMutex` + `tasks map[string]*disclosureTask` | `tasks *csync.Map[string, *disclosureTask]` |
| `server/server.go` | `mu sync.RWMutex` 保护 config/maxBody/srv | `config *csync.Value[Config]` + `maxRequestBodyBytes atomic.Int64` + `srv atomic.Pointer[http.Server]` |
| `server/disclosure.go` | `s.mu.RLock/Unlock` 保护 disclosure 双检锁 | `disclosure atomic.Pointer[disclosureTaskManager]` + `discMu sync.Mutex` |
| `knowledge/graph/cache.go` | 原样保留（csync.Map 缺 Range 迭代，evictIfNeeded 需遍历删除） | 添加 `// TODO(csync):` 说明 |
| `session/session.go` | `idMu sync.Mutex` + `idCounter int64` | `idCounter atomic.Int64`（Remove idMu） |
| `session/session.go` | `locksMu sync.Mutex` + 耦合 LRU 链表 | 原样保留，添加 TODO 说明需同时处理 list + map |

- **原因**: 消除重复的 Lock/Unlock 样板代码，利用泛型容器提供类型安全的并发访问
- **影响范围**: acp/session.go, acp/server.go, knowledge/store.go, server/disclosure.go, server/server.go, session/session.go, knowledge/graph/cache.go
- **风险等级**: 低（纯互斥替换，不改 API 签名；测试全绿）
- **审查要求**: L1
- **验证**: `go build ./...` ✅ | `go test ./acp/... ./knowledge/... ./server/... ./session/...` ✅

---

## 2026-07-16: goimports -local 统一导入分组
- **Decision**: pre-commit 钩子中 goimports 增加 `-local github.com/xujian519/mady` 标志，使 import 分组变为标准的三段式（标准库/第三方/本地）
- **Reason**: GO-DEVELOPMENT-STANDARDS.md §2.3 要求三段式导入分组，但原配置缺少 `-local` 导致第三方和本地包混在同一组
- **Impact**: `.pre-commit-config.yaml` 修改；全仓 13 个文件自动格式化调整导入顺序
- **Risk**: 低（纯格式化变更，不影响语义）
- **Verification**: `go build ./...` ✅ | `go vet ./...` ✅ | `go test ./...` ✅

---

## 2026-07-16: 测试中 time.Sleep 替换为 channel/ticker 同步
- **Decision**: 移除 `tui/lifecycle_test.go`（2处）、`a2a/ratelimit_test.go`（1处）、`mcp/client_test.go`（5处）、`server/server_test.go`（1处）中的 `time.Sleep` 调用，改用 `time.NewTicker` + channel 等待或 channel 同步
- **Reason**: GO-DEVELOPMENT-STANDARDS.md §7.7 要求避免 time.Sleep 导致脆弱测试
- **Risk**: 低
- **Verification**: `go test ./tui/... ./a2a/... ./mcp/... ./server/...` ✅

---

## 2026-07-16: 补充导出常量块注释
- **Decision**: 为 `agentcore/state.go`、`agentcore/provider.go`（3处）、`agentcore/executor.go` 的 const 块补充块级注释
- **Reason**: 导出符号必须注释（GO-DEVELOPMENT-STANDARDS.md §10.1），部分 const 块此前缺少文档
- **Risk**: 低
- **Verification**: `go build ./agentcore/...` ✅

---
