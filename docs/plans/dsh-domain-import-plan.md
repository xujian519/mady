# DeepSeek Harness 三域能力引入 Mady 总体方案

> 目标仓库：`/Users/xujian/projects/deepseek-harness`（TypeScript / Cordis 插件体系）
> 承载仓库：Mady（Go / Extension + LifecycleHook 体系）
> 范围：`packages/patent/`（16 包）、`packages/document/`（5 包）、`packages/legal/`（3 包）
> 制订日期：2026-10-28
> 配套清单：`docs/plans/dsh-domain-import-tasks.md`

---

## 一、结论摘要

1. **两仓大量资产同源，不是从零移植。** dsh 的 `writing-patterns` 10 份语料、`doc-style` 4 套样式、
   `patent-rule/assets/rules/patent/mady-gap-rules.yaml` 均明确标注来自 Mady（"Mady Document Style"、
   "Mady 规则资产并入"）。因此本方案不重复引入这些资产，只做双向漂移校验。
2. **真正增量集中在 dsh 自行新写的部分**，共 8 个能力块：
   附图生成引擎、CNIPA 出件（DOCX + 断言）、期限（送达推定 + 节假日顺延）、费用测算、
   逐条可核验法条索引（来源 + 核验日期 + 未转录不放行）、交付件登记与质量门、
   输出门禁（post-execute / 前置闸门 / 单调 deny）、持久多智能体团队。
3. **法律域与文档域的增量是"可核验性"与"交付闭环"，不是推理能力。** Mady 的证据三性、知识库检索、
   模板渲染、写作风格均已存在且部分更强；缺的是民诉法第 66 条证据分类、司法解释/指导案例独立检索、
   法条引用的来源与核验日期、Office→PDF、交付件质量门并列呈现。
4. **建议按 W-1 前置 spike + 6 个实施波次推进，总计约 163 人日（AI 协作口径；含不确定性区间 143–183）**，
   每波独立可验收、可回滚。零覆盖项（费用、TRIZ、出件、附图生成）与 fail-closed 项（法条核验来源、期限顺延）优先。
5. **本方案已经过一次逐项实测评审**（`docs/plans/dsh-domain-import-review.md`）：确认 8 项假设成立、
   修正 5 项阻断问题。其中 FreeCAD 结构图（原 W4-F3）已移出实施波次，改为「先研究能力、再决定立项」。

**不建议做的事**：不把 dsh 的 `patent-core/src/atoms`（StageProvider/StageHandler 原子流水线）搬进来。
Mady 的 `graph/`（DAG + Pregel，含 StateSchema/Reducer、NodePolicy、DegradationMark）是更成熟的执行引擎，
atoms 属于 dsh 侧的轻量替代品；只吸收其 26 个 handler 的业务语义，不引入其运行时。

---

## 二、调研事实：两仓能力底账

### 2.1 规模

| 域 | 包数 | 非测试 TS 行数 | 附带资产 |
|---|---:|---:|---|
| patent | 16 | ~95,000 | 27 份规则 YAML、4 份法条索引、节假日/费用索引、10 份撰写语料、13 套交付模板、CNIPA 出件模板与 Python 引擎 |
| document | 5 | ~7,300 | 17 份模板、4 套样式 YAML |
| legal | 3 | ~5,400 | 7,910 份法规条号索引 YAML（仅 17 份含条文转录，其余 `text: null`）；专利法转录 37 条（82 条范围内）+ 实施条例 45 条 + 审查指南 68 节点 |

### 2.2 Mady 侧已有能力（不重复建设）

| 能力 | Mady 落点 | 成熟度 |
|---|---|---|
| 专利撰写（权利要求/说明书） | `domains/claimdrafting`（4.2K 行）、`domains/specdrafting`（2.6K 行） | 强，含规则引擎 + Pregel + 评分 |
| 新颖性/创造性/充分公开/侵权 | `domains/novelty`、`inventiveness`、`enablement`、`infringement` | 强，四域各 1.9–2.4K 行 |
| 证据判定（三性/举证责任/冲突） | `domains/evidence`（3.0K 行，7 工具，82 测试） | 强，但为专利取证 15 类，非民诉法 66 条 |
| 推理与图引擎 | `domains/reasoning`（5.6K 行）、`graph/` | 强于 dsh |
| 规则引擎 + OA 解析 | `domains/rules`（2.2K 行）、`domains/rulekit` | 强，缺门禁化 |
| 法条引用解析 | `pkg/lawcite`（条/款/项/之N） | 强，缺来源与核验日期 |
| 知识库 FTS5 | `knowledge/sqlite`（1.8K 行，laws-full.db 9,121 部法规） | 强，偏专利；司法解释/指导案例无独立检索 |
| 文档模板 | `domains/doctmpl`（2.2K 行，22 模板，MD/DOCX/HTML/PDF） | 强，缺交付件登记与质量门 |
| 写作模式 | `domains/writing`（10 语料 + SkillCompiler） | 与 dsh 同源 |
| 期限计算 | `domains/deadline`（395 行） | 薄，缺送达推定与节假日顺延 |
| 编排骨架 | `agentcore/worker`、`agentcore/tasklist`、`domains/provisions` | 中，缺消息总线与依赖 DAG |

### 2.3 dsh 侧真正增量（本方案的核心标的）

| # | 能力块 | dsh 证据 | Mady 现状 |
|---|---|---|---|
| A | 附图生成与合规 | `patent-tools/src/figure/` 42 文件 + 17 个 figure 工具文件（Graphviz / viz-js WASM / FreeCAD / Inkscape / SVG 后处理 / 引线字框 / 渲染核验） | 仅 `domains/figure` 一致性核验，无生成；**源基线部分完成，见 §2.4** |
| B | CNIPA 出件 | `patent-filing`（五段合卷 DOCX、模板反解体例、`assertions` 含编号/禁用词/禁用色/字号） | `doctmpl` 可分篇渲染，无合卷与体例断言 |
| C | 期限 | `patent-deadline`（发文日 + 15 日推定送达 + 国务院节假日顺延 + fail-loud） | 仅按申请日推算，无送达与顺延 |
| D | 费用 | `patent-fees`（费种/减缴/年费分档/滞纳金，金额未转录即拒绝出价） | 无 |
| E | 可核验法条索引 | `patent-law` / `legal-law`（逐条 `text` + `sourceDoc` + `verifiedOn` + `maxArticle`，未转录不判 Valid） | `citation_table` 仅条号→主题词，无来源与日期 |
| F | 交付件登记与质量门 | `document-deliver` + `patent-document/verify-deliverable` + 13 套交付模板（SKILL.md + HTML + checklist + citation-log + conventions） | 无 |
| G | 输出门禁 | `patent-rule` 的 `RuleOutputGate`（接 post-execute）、`delivery-gate`、`structural-gate`、单调 deny、交付前置闸门 | `rulekit` 有引擎，无门禁接线与前置闸门 |
| H | 多智能体团队 | `patent-teams`（队长-成员、可续聊子代理、依赖任务、邮箱、事件驱动调度器、锁） | `worker` + `tasklist` 静态，无消息总线 |
| I | 民用法律证据分类 | `legal-evidence`（民诉法 66 条八类法定证据 × 三性确定性评分） | 专利取证 15 类，与 66 条不对应 |
| J | 司法解释/指导案例检索 | `legal-knowledge`（案例/法规/司法解释/指导案例四类 + FTS5 + 内存降级 + install） | 仅法规 + 专利判例 |
| K | Office→PDF | `office-to-pdf`（LibreOffice 原生或 WASM） | 无 |
| L | TRIZ | `methodology`（40 原理 + 39×39 矩阵 + 5 分离原理） | 无 |
| M | 文献四源 | `tool-literature`（arXiv/OpenAlex/Semantic Scholar/Crossref + 按主机限速 + LRU） | 三源，缺 OpenAlex 与 `paper_download` |
| N | 索引资产格式 | `patent-index-asset`（YAML 映射 + 严格字段 + 来源/核验字段） | 无统一格式，各模块自解析 |
| O | 审查模拟对抗 | `patent-core/src/adversary`（评审/分歧/隔离检查/报告） | `inventiveness` 有审查模拟，需逐项核对后补差 |

### 2.4 附图引擎的源基线（重要：dsh 侧并非全部完成）

以 2026-10-28 的 dsh master 为准（记忆中 2026-09-30 的状态已过时）：

**已合入**：dot/Graphviz 渲染、SVG 后处理、引线不穿数字、毫米格式化与边界统一、断线（break lines）、
跨图一致性核对、图 vs spec 比对、渲染门禁（`figureFailOnRenderFindings` 默认关闭，需显式开启）、
CNIPA 页码「第N页 共M页」、FreeCAD 剖切视图、hatch 几何。

**未完成或受限**：
- FreeCAD 结构线稿默认关闭（`structureFigureEnabled=true` 才启用），未安装 `freecadcmd` 时返回 `setup_required`；
- CI 用 pinned AppImage（FreeCAD **1.1.4**）提取安装，因为 Ubuntu 包 0.21 低于渲染器所需 1.1 API；
- FreeCAD 自带 Python 在空 `LANG`/`LC_ALL` 下对中文标注抛 `UnicodeEncodeError`（需 UTF-8 locale）；
- 已有技术债记录：多职责大文件、渲染预算曾无 Config 出口等。

**对 Mady 的含义**：W4 只移植「已合入且可验证」的部分；FreeCAD 结构图先做能力研究（W-1 的 T-R-01），
研究结论出来前不承诺交付。禁止把 dsh 未完成的 P1 项当作移植前提。

---

## 三、移植原则

1. **接口对等，不逐行翻译。** dsh 是 TS + Cordis `ctx` 服务；Mady 是 Go + `Extension` / `LifecycleHook` /
   `iface` 契约。每个包先产出一张「服务契约 → Go 接口」映射，再实现。禁止把 Cordis 的依赖注入形态硬搬。
2. **资产逐字直搬，代码重写。** YAML / HTML / CSS / DOCX 模板 / Python 引擎可原样复制；随附来源注释
   （`sourceDoc` / `verifiedOn` / 官方文号）必须保留。每类资产加一个「资产指纹」测试（SHA-256 清单），
   防止后续手改导致与来源漂移。
3. **fail-closed 是一等公民。** 抄袭 dsh 的四条硬语义并写成负控制测试：
   - 条文未转录 → 判 `Unverified`，绝不判 `Valid`；
   - 费用条目金额为 `null` → 拒绝给合计，不按 0 或猜测单价算；
   - 日历缺该年份 → 报错，不按"非休假日"静默处理；
   - 出件断言不通过 → 不出件，不降级为警告。
4. **工具契约二分。** 采用 dsh 的「规范值（可无损 JSON）+ 纯 `render` 函数产出模型可见 prose」，
   对齐 Mady 既有 `docs/TOOL_CONTRACT.md`；工具结果同时写入证据账本（`agentcore/evidence`）。
5. **门禁走 LifecycleHook。** dsh 的 `tools/post-execute` → Mady `AfterToolExecution`；
   交付前闸门 → `BeforeToolExecution`（检查本会话是否已成功执行 `rule_check` / `law_verify`）。
   `deny` 单调不可被后续 hook 翻转。
6. **来源可核验优先于覆盖率。** 宁可只转录 37 条关键专利法条，也不把 7,910 份骨架说成已核验。
   骨架只用于「条号存在性」判定，必须与条文核验分开报告。
7. **每波独立提交、独立验收。** 遵循 `AGENTS.md` 的 Spec-Driven 四阶段与 AI_CHANGELOG 追加要求；
   触及敏感路径（`guardrails/`、`domains/router.go`、`domains/patent.go`、`guardrails/citation_*.go`）
   必须在 PR 模板勾选红线并跑 `scripts/check-sensitive-paths.sh`。
8. **区分两层门禁。** Mady 已有 `scripts/gatechecks/check-tone-words.py`（CI 层，校验仓库文案）；
   dsh 的规则门禁是运行时 `AfterToolExecution`（校验模型输出）。两者互补，不互相替代，也不合并实现。
9. **先研究、后立项。** 涉及外部重型依赖（FreeCAD、LibreOffice）的能力，先产出能力边界报告与失败模式，
   再决定是否进入实施波次；不把「能调用」当作「能交付」。

---

## 四、24 包全景映射

处置代号：**直搬**＝资产原样复制；**增强**＝在既有模块上补齐；**重写**＝新增 Go 模块实现同等契约；
**跳过**＝已等价或不应引入。

### 4.1 patent（16 包）

| dsh 包 | 处置 | Mady 落点 | 引入内容 | 人日 |
|---|---|---|---|---:|
| `patent-index-asset` | 重写 | `pkg/indexasset/` | YAML 映射读取、严格字段读取器、来源记录字段、加载错误类型 | 2 |
| `patent-law` | 增强 + 直搬 | `pkg/lawcite/`、`guardrails/citation_*.go` | 4 份索引 YAML（专利法 37 条转录、实施条例 45 条、审查指南 68 节点、修订沿革）、`verifiedOn`/`sourceDoc`/`maxArticle` 判定、`law_verify` 工具、未转录不放行 | 5 |
| `legal-law` | 直搬 + 增强 | `pkg/lawcite/`、`knowledge/legal/` | 7,910 份条号骨架（仅存在性，其中 17 份带条文）、`legal_law_verify` 工具、精校子集标注 | 4 |
| `patent-fees` | 重写 + 直搬 | `domains/fees/` | `cn-fees.yaml`、费种/计数基准/减缴/年费分档/滞纳金、null 拒算、`patent_fees` 工具 | 6 |
| `patent-deadline` | 增强 + 直搬 | `domains/deadline/` | `cn-holidays.yaml`（2025/2026，含文号）、发文日 + 15 日推定送达、指定期限、届满日顺延、fail-loud、`patent_deadlines` 工具 | 6 |
| `patent-rule` | 增强 + 直搬 | `domains/rules/`、`domains/rulekit/` | 27 份规则 YAML schema 适配、`RuleOutputGate`→`AfterToolExecution`、`delivery-gate`/`structural-gate`→`BeforeToolExecution`、EVI-011 单调 deny | 10 |
| `patent-document` | 重写 + 直搬 | `domains/deliver/`、`skills/patent/templates/` | 13 套交付模板（SKILL.md + template.html + example.html + checklist + citation-log + conventions + tokens.css + manifest.json）、HTML 模板渲染通道、段落编号 `[NNNN]`、`brandInjector`、`htmlScan`、`checkDocumentCompliance` 四条规则、`verify_deliverable`（路径一致性/时序/交付清单）、Chrome PDF | 15 |
| `patent-filing` | 重写 + 直搬 | `domains/filing/` | CNIPA 五段合卷 DOCX、`申请文件.json` spec、模板反解体例、Python 引擎（build/style/render_figures/verify）编排、断言 | 10 |
| `patent-tools` | 选择性重写 | `tools/`、`domains/*/` | 55 个工具文件按逐项处置表（见 `dsh-domain-import-tasks.md` 附录 A）补齐，采用「规范值 + render」契约；figure 部分只移植已合入项（§2.4） | 10 |
| `patent-core` | 选择性吸收 | `domains/triz/`、`domains/audit/` 等 | TRIZ、adversary 审查模拟、analysis-report、技术问题四检验、claim-coverage/单一性、draft 图文说明/表题、response/plan；**跳过 `atoms`**，四个图子图（citation-check/enablement/inventiveness/novelty）Mady 已有对应，仅做语义比对 | 15 |
| `patent-teams` | 重写 | `domains/teams/` | 队长-成员、可续聊子代理、依赖任务 DAG、邮箱、事件驱动调度器、团队锁、状态视图；**形态取决于 W-1 的团队模型 ADR，估算 ±40%** | 15 |
| `patent-workflow` | 增强 | `domains/plantask/`、`domains/workflows/patent/` | 灵活计划层、plantask 审批接缝、五类会话事件写入 | 4 |
| `patent-data` | 增强 | `retrieval/domain/nuopatent/`、`browser/` | 结果 LRU 缓存、结构化元数据映射、CNIPR 全文/法律状态通道 | 5 |
| `patent-knowledge` | 增强 | `knowledge/sqlite/`、`knowledge/patenttools/` | wiki 卡片关键词查询、IPC 查表、图谱查询补齐、install 引导 | 3 |
| `writing-patterns` | 跳过 | `domains/writing/` | 同源；仅做漂移校验（10 份语料字段一致性测试） | 0 |
| `tool-literature` | 增强 | `tools/scholar_search.go` | OpenAlex 源、`paper_download`、按主机限速（arXiv 3s / S2 1s）、LRU GET 缓存 | 3 |
| `methodology` | 重写 | `domains/triz/` | 40 原理、39×39 矩阵、5 分离原理、`triz` 工具 + 系统提示词段 | 3 |

### 4.2 document（5 包）

| dsh 包 | 处置 | Mady 落点 | 引入内容 | 人日 |
|---|---|---|---|---:|
| `doc-style` | 跳过（对齐字段） | `domains/config/style.go`、`styles/` | 同源；补齐 dsh 的结构/引用约定投影（`projection.ts` 三投影） | 1 |
| `doc-template` | 增强 + 直搬 | `domains/doctmpl/` | 17 份模板去重并入、类型化变量模式、残余占位符与警告、Markdown/HTML/DOCX 三渲染器对齐 | 3 |
| `docx-kit` | 跳过（校验） | `domains/doctmpl/renderer_docx.go` | 已具备纯 Go OOXML；补 `extractDocxText` 的页眉页脚与条目/解压字节预算 | 1 |
| `office-to-pdf` | 重写 | `tools/office_pdf.go` | 双通道：LibreOffice 原生优先 → `pandoc + typst` 兜底 → 两者皆缺时结构化拒绝；缺失字体名返回 | 3 |
| `document-deliver` | 重写 | `domains/deliver/` | `document_deliver` 工具、成品登记（路径 + hash + 格式）、确定性核验与模型自述并列 | 4 |
| `document-deliver` | 见 4.1 | — | — | — |

### 4.3 legal（3 包）

| dsh 包 | 处置 | Mady 落点 | 引入内容 | 人日 |
|---|---|---|---|---:|
| `legal-evidence` | 增强 | `domains/evidence/` | 民诉法 66 条八类法定证据模型、三性确定性评分（关联/合法/真实）、`evaluate_legal_evidence` 工具；与既有 15 类**并行两维**（法定种类 ∥ 取证场景），不做互相覆盖的映射 | 5 |
| `legal-knowledge` | 增强 | `knowledge/sqlite/`、`knowledge/legal/` | 案例/法规/司法解释/指导案例四类检索、FTS5 + 内存降级、`legal_knowledge_install` | 6 |
| `legal-law` | 见 4.1 | — | — | — |

**合计工作量：约 163 人日**（W-1 3.5 + W0 3 + W1 36 + W2 35 + W3 11 + W4 26 + W5 48），
另含可选的 W6 桌面呈现 8 人日；FreeCAD 结构图（原 F3）与 W5 团队实施的 ±40% 不确定性不计入。

**估算方法**：按「任务复杂度 × 同类模块历史实现速度」类比（Mady 既有模块的非测试代码量 / 测试函数数），
并对含外部依赖或跨模块重构的任务标注不确定性；整体置信区间 ±30%（约 114–212 人日）。

---

## 五、分波实施计划

波次依赖：**W-1 → W0 → (W1 ∥ W3) → W2 → W5**；**W4 独立可并行**（仅依赖 W0 的资产规范与 W2 的交付通道）。

### W-1 前置 spike（3.5 人日）

**目的**：在投入实施前，先解决两个「结论会改变方案」的问题，并建立环境探测基线。

1. **T-R-01 FreeCAD 能力研究（2d）**：产出能力边界报告——本机/macOS 的可安装路径（AppImage 仅 Linux，
   macOS 需 cask/官方 .dmg）、渲染器所需 FreeCAD 1.1 API 的版本约束、`freecadcmd` 非交互入口、
   UTF-8 locale 对中文标注的影响、冷启动与并发缓存问题、TechDraw/剖切/阴影/hatch 的能力范围与失败模式。
   结论用于决定是否立项「结构图」。
2. **T-R-02 团队模型 ADR（1d）**：对比 Mady `agentcore/worker` + `tasklist` + `domains/provisions`
   与 dsh `patent-teams`（队长-成员、邮箱、依赖 DAG、调度器、锁），给出「扩展 worker」还是「新增 domains/teams」的选择与理由。
3. **T-R-03 环境探测脚本（0.5d）**：`scripts/check-domain-deps`，输出 python3/python-docx/dot/inkscape/
   freecadcmd/soffice/pandoc/typst 的可用性矩阵，供每个任务开工前复核。

### W0 基座与资产落位（3 人日）

**目的**：先把「来源可核验的资产格式」和「资产目录约定」立起来，后续所有波次复用。

产出：
- `pkg/indexasset/`：`LoadMapping` / `StrictFields` / `SourceRecord{sourceDoc, verifiedOn, maxArticle}` / `LoadError`。
- 资产目录约定：
  - 法条索引 → `data/law/<law>.yaml`（`text` / `sourceDoc` / `verifiedOn` / `topics` / `paragraphs`）
  - 费用索引 → `data/fees/cn-fees.yaml`
  - 工作日历 → `data/calendar/cn-holidays.yaml`
  - 规则包 → `domains/rules/data/rules/`（沿用现状）
- `scripts/asset-fingerprint`：生成 `data/*.sha256` 清单，CI 校验资产未被手改。
- 三份格式文档：索引资产格式、来源字段语义、fail-closed 判定表。

验收：`go test ./pkg/indexasset/...`；指纹脚本对现有 4 份资产生成清单并通过 CI。

### W1 可核验性（36 人日）

**目的**：把「引用/金额/期限」三类事实做到可核验、可拒绝。这是专利/法律业务的信任基座。

1. **法条核验升级（T-W1-01，5d）**
   - `pkg/lawcite` 增加 `Resolve(statute, article, para, item)`：返回 `{Text, SourceDoc, VerifiedOn, Status}`。
   - `guardrails/citation_table.go` 的 91 条表与 dsh 4 份索引合并：条文级以索引为准，主题词级保留表。
   - 新增四值 `Resolution.Status`（`Verified` / `ExistOnly` / `OutOfRange` / `Unverified`）；
     `guardrails` 既有五值 `Verdict`（Valid/Unknown/Unverifiable/Suspect/Invalid）保持不变，
     由 Gate 映射：Verified→Valid、ExistOnly→Unverifiable、OutOfRange→Invalid、Unverified→Unknown。
     `Valid` 唯一来源是 `Verified`（要求 `text` 与 `verifiedOn` 同时存在）。
   - 新增 `law_verify` 工具，与 `guardrails.NewCitationGate` 共用同一解析器。
2. **法规骨架并入（T-W1-02，4d）**
   - 7,910 份 YAML 转入 `data/law/skeleton/`，仅用于条号存在性；报告字段区分「条文已核验 / 仅条号存在 / 不存在」。
   - 专利法已转录 37 条、实施条例 45 条、审查指南 68 节点为精校子集，优先命中；专利法其余 45 条仅有条号。
3. **费用测算（T-W1-03，6d）**
   - `domains/fees`：`Quote(case{type, claims, pages, priorities, triggers, reduction})`。
   - 金额 `null` → 返回 `Refused{reason}`；滞纳金按月百分比与 `maxMonths` 计算。
   - 金标用例：发明申请费 900、减缴后 135、年费第 1–3 年分档、超 10 项权利要求每项 150 等（以索引为准）。
4. **期限增强（T-W1-04，6d）**
   - `domains/deadline` 增加 `AddServiceTerm(publishDate)`（发文日 + 15 日推定送达）、`ExtendToWorkday`。
   - 日历缺年份 → `ErrCalendarMissing`；调休上班日视为工作日。
   - 回归用例：2026 春节、国庆、跨年、调休周日。
5. **门禁接线（T-W1-05，15d，分两批）**
   - `domains/rules/gate`：`OutputGate`（`AfterToolExecution`，关键词/结构分析，`warn`/`block`）、
     `StructuralGate`（章节/要素）、`EvidenceGuard`（EVI-011 单调 deny）。
   - **批 A**：dsh 规则包中 action=block 与结构性规则（实测 44 条规则、200+ check 块，block 32 处），
     适配到 Mady schema：`category_detection`→`structural_analysis`、`regex antiPatterns`→`keyword_blocklist`、
     `requiredElements`→`structural_analysis`；**未知 `check.type` 一律报错，不静默忽略**。
   - **批 B**：warn/log 提示级规则（warn 99、log 51），批量转换并按命名空间分组。
   - `DeliveryGate` 不在本波接线：交付工具在 W2 才存在，避免循环依赖（见评审 P0-5），归入 W2 验收。
   - 新增「门禁不可翻转」测试：deny 后任何后续 hook 无法改回。

验收：已转录 37 条逐条测试 + 未转录 45 条「必须非 Verified」负控制；费用 15 个金标用例；期限 12 个日期用例；门禁 5 个翻转攻击测试。

### W2 交付闭环（35 人日）

**目的**：让「产出物」可交付、可自检、可追溯到来源。

1. **13 套交付模板（T-W2-01，15d）**
   - 资产直搬至 `skills/patent/templates/<name>/`（`SKILL.md`、`assets/template.html`、`example.html`、`references/{checklist,citation-log,conventions}.md`、`tokens.css`、`manifest.json`）。
   - **新增 HTML 模板渲染通道**：Mady `doctmpl` 现有 HTML 渲染是 Markdown→HTML 转换（goldmark），
     不能直接吃 dsh 的 HTML 模板；需实现「原生 HTML 模板 + 槽位填充」路径，再交 `renderer_pdf_chrome.go` 出 PDF。
   - **段落编号器**：`[NNNN]` 四位编号，成文与验收共用同一匹配式（Mady 当前无此能力）。
   - **品牌注入**（logo/字号/页眉页脚）与 `htmlScan`（HTML 结构扫描）。
   - **`Compliance`**：移植 `checkDocumentCompliance` 四条规则——`section-numbering`（中文数码章节编号递增不重复）、
     `internal-section`（对外件不得含内部工作章节）、`internal-body`（不得含待办/核验记录/成功率等）、
     `evidence-cross-reference`（证据编号交叉核对），并按 `external-filing` / `internal-record` 分流。
   - **`VerifyDeliverable`**：判定案件主路径与交付副本字节一致、渲染产物晚于输入（mtime）、产出交付清单；
     **注意这不是模板结构断言**（旧方案此处写错，已按评审 P0-1 修正）。
2. **CNIPA 出件（T-W2-02，10d）**
   - 复用 `patent-filing/assets/engine/*.py`（Python 3 可用，Mady `tools/execute_code.go` 已有执行通道）。
   - `domains/filing`：`Compose(spec{abstract, claims, specification, figures}) → .docx`；体例从 `申请文件模板.docx` 反解。
   - 断言：5 节、`[NNNN]` 编号连续、`forbid_text` 禁用词、禁用彩色、字号上限、`claim_pattern`。
   - Go 侧只做编排与断言，排版交给既有 Python 引擎，避免重写 OOXML 排版。
3. **交付件登记（T-W2-03，4d）**
   - `domains/deliver` 增加 `Register(path, format, selfCheck)`，记录 hash、时间、格式、模型自述与确定性核验结论两列。
   - 会话日志写入 `deliver/registered` 事件；不新增 host 写 RPC。
4. **模板并入（T-W2-04，3d）**：17 份 dsh 模板与 Mady 22 份去重，冲突以「dsh 有来源字段」为准合并。
5. **Office→PDF（T-W2-05，3d）**：`tools/office_pdf.go` 双通道——
   LibreOffice 原生优先；缺失时走 `pandoc + typst`（docx→typst→pdf），需实测中文字体与表格保真度；
   两者皆缺时结构化拒绝并给出安装指引，不静默失败。本机已确认 LibreOffice 缺失、pandoc 与 typst 可用。

验收：13 套模板各跑一遍渲染 + verify；出件样例通过全部断言；故意注入禁用词必须失败。

### W3 法律域（11 人日）

1. **民诉法 66 条证据模型（T-W3-01，5d）**
   - `domains/evidence/statutory.go`：八类（书证/物证/视听资料/电子数据/证人证言/当事人陈述/鉴定意见/勘验笔录）× 三性规则。
   - **二维模型，不做映射表**：`EvidenceKind`（66 条法定种类）与 `AcquisitionContext`（Mady 15 类取证场景）
     是正交维度——「互联网公开」可对应电子数据，但其证明力判断依赖公开时间与可及性，与法定种类无关。
     三性评分分别从两维取规则，互不覆盖。
2. **四类司法检索（T-W3-02，6d）**
   - `knowledge/legal`：`SearchCases` / `SearchRegulations` / `SearchJudicialInterpretations` / `SearchGuidingCases`。
   - 复用 `knowledge/sqlite` FTS5 连接；无库时内存降级；`legal_knowledge_install` 从源库拷贝并可选裁剪 FTS。
   - 指导案例支持「指导案例第 N 号」检索。

验收：四类检索各 5 个用例；无库降级路径测试；66 条八类各 1 个评分用例。

### W4 生成能力：附图引擎（26 人日，F3 移出）

**只移植 dsh master 已合入且可验证的部分**（见 §2.4）。分期交付，每期可独立使用：

| 期 | 内容 | dsh 对应 | 人日 |
|---|---|---|---:|
| F1 | 矢量图构建 + Graphviz/dot 渲染 + SVG 视口/后处理 | `dot-builder`、`graphviz-renderer`、`svg-viewport`、`structure-svg-postprocess` | 8 |
| F2 | 流程图/框图/时序图/剖面图 + 引线穿字与字框检测 + 渲染核验 | `plot-diagram`、`section-diagram`、`sequence-diagram`、`leader-line`、`glyph-box`、`render-check-*` | 10 |
| F4 | 图文一致性与交付（图号、标记、摘要附图、跨图一致性、提交页） | `figure-set-consistency`、`figure-comparison`、`submission-page`、`verify-patent-figure` | 8 |

策略：F1/F2 用 Go 原生（纯计算 + 调 `dot` 二进制，本机已确认 dot 可用），F4 复用 `domains/figure` 现有核验逻辑。

**F3（FreeCAD 结构图）不入本波**：本机 FreeCAD 缺失，且 dsh 侧结构图默认关闭、依赖 FreeCAD 1.1 API，
CI 用 pinned AppImage 仅支持 Linux x86_64。按「先研究、后立项」原则，由 W-1 的 T-R-01 产出能力边界报告，
再决定是否立项；立项前不估算、不承诺。

验收：每期用 3 个真实案例的附图输入跑渲染 + 核验；净距分档与引线穿字检查作为门禁。

### W5 协作与外围（48 人日）

1. **patent-teams（15d）**：形态取决于 W-1 的团队模型 ADR；在 `agentcore/worker` + `tasklist` 上补邮箱、
   依赖 DAG、事件驱动调度、团队锁、状态视图、可续聊子代理。估算 ±40%，ADR 后重估。
2. **patent-core 增量（15d，可拆）**：TRIZ（3d）、adversary 审查模拟（4d）、analysis-report（2d）、技术问题四检验（2d）、claim-coverage/单一性（2d）、response/plan（2d）。
3. **patent-tools 补齐（10d）**：见任务清单附录 A 中标注「缺」与「部分」的项。
4. **patent-data/knowledge 增强（5d）**：LRU 缓存、CNIPR 通道、wiki 卡片查询、install 引导。
5. **tool-literature（3d）**：OpenAlex + `paper_download` + 限速/LRU。

### W6（可选）桌面呈现

`ui-document-studio` 的交付工作室（交付物列表 + 两列质量门）在 Mady `desktop/`（Wails + React）中复刻。
**默认不做**，仅在 W2 交付登记模型稳定后按需评估。

---

## 六、关键接口设计

### 6.1 法条核验（W1）

```go
// pkg/lawcite
type Resolution struct {
    Statute    Statute
    Article    int
    Paragraph  int
    Item       int
    Text       string      // 条文原文；骨架条目为空
    SourceDoc  string      // 官方文书
    VerifiedOn string      // 转录日期
    Status     VerifyStatus // Verified | ExistOnly | OutOfRange | Unverified
}

// Status 判定优先级：OutOfRange > Unverified(无索引) > ExistOnly(骨架) > Verified(有 text + verifiedOn)
```

### 6.2 费用（W1）

```go
// domains/fees
type Query struct {
    PatentType PatentType // invention | utility | design
    Claims     int
    Pages      int
    Priorities int
    Triggers   []Trigger // application | substantive-exam | reexam | invalidation | annuity
    AnnuityYear int
    Reduction  Reduction // none | individual | entity
    LateMonths int
}
type Quote struct {
    Items   []Item
    Total   int
    Refused []Refusal // 金额 null 或超表达范围的条目
}
```

### 6.3 交付件（W2）

```go
// domains/deliver
type Deliverable struct {
    Path       string
    Format     string // html | pdf | docx | md
    SHA256     string
    SelfCheck  string            // 模型自述
    Checks     []DeterministicCheck // 结构断言 / 占位符 / 引用一致性
    Verified   bool              // 全部确定性检查通过
}
```

### 6.4 门禁（W1 批 A/B；DeliveryGate 在 W2）

```go
// domains/rules/gate，全部实现 iface.LifecycleHook
type OutputGate struct{ /* ... */ }      // AfterToolExecution：结果文本 → 规则评估 → warn/block
type StructuralGate struct{ /* ... */ }  // AfterToolExecution：章节/要素结构分析
type EvidenceGuard struct{ /* ... */ }   // EVI-011：单调 deny，不可被后续 hook 翻转
// W2 接线：
type DeliveryGate struct{ /* ... */ }    // BeforeToolExecution：交付工具前检查本会话前置调用记录
```

跨轮状态（如「本会话是否已成功执行 rule_check」）需 hook 按 `SessionID` 自维护，
`AgentRunContext` 本身不携带历史调用列表。

### 6.5 证据二维模型（W3）

```go
// domains/evidence/statutory.go
type EvidenceKind string // 书证|物证|视听资料|电子数据|证人证言|当事人陈述|鉴定意见|勘验笔录

type StatutoryEvidence struct {
    Kind        EvidenceKind        // 法定种类（民诉法 66 条，正交轴 1）
    Acquisition AcquisitionContext  // 取证场景（Mady 15 类，正交轴 2）
    Relevance   Score               // 三性：各自从两轴取规则
    Legality    Score
    Authenticity Score
}
```

---

## 七、风险与取舍

| 风险 | 影响 | 应对 |
|---|---|---|
| FreeCAD 本机缺失、仅 Linux AppImage 有 CI 路径 | 结构图无法交付 | 已移出 W4；W-1 先做能力研究再立项 |
| dsh 附图引擎源基线未全部完成 | 移植到未完成能力 | §2.4 明确只移植已合入项；未完成项不作为前提 |
| FreeCAD 自带 Python 在空 locale 下中文标注抛 `UnicodeEncodeError` | 中文附图失败 | 渲染子进程强制 UTF-8 locale（`LANG`/`LC_ALL`/`LC_CTYPE`） |
| LibreOffice 本机缺失 | Office→PDF 不可验收 | 双通道：LibreOffice → pandoc+typst → 结构化拒绝；验收覆盖 pandoc 版式抽查 |
| 7,910 份法条骨架被误当已核验 | 法律风险 | 状态机区分 `ExistOnly`；报告与 UI 用不同措辞；负控制测试 |
| `laws-full.db` 规模（9,121 部）来自既有文档，未实测 | 排期偏差 | W0 阶段实测并更新基线 |
| 规则 YAML schema 与 Mady 不一致 | 规则静默不生效 | 资产加载期严格校验 + 一条「未识别 check.type 即报错」的 fail-closed |
| patent-teams 与 agentcore/worker 语义重叠 | 两套团队模型 | 先写 ADR 决定「扩展 worker」还是「新增 teams」，再实施 |
| 工具数量膨胀（35→90+） | 模型选择困难、上下文膨胀 | 工具按领域分组挂载，非全量注入；沿用 manifest 机制按 preset 装配 |
| 资产手改导致与官方来源漂移 | 核验失效 | 资产指纹清单 + CI 校验 |

---

## 八、与既有规范的衔接

- **AI_CHANGELOG**：每波结束按 `docs/decisions/ai-changelog/` 规范追加（先读 `INDEX.json`，再用 `scripts/changelog/main.go`）。
- **Spec-Driven**：W2（出件）、W4（附图）属于新功能，需先出 proposal → spec → design → tasks，
  本文档作为 plan 层输入。
- **敏感路径**：W1 触及 `guardrails/citation_gate.go`、`guardrails/citation_table.go`；
  W5 触及 `agentcore/hooks.go`、`domains/router.go`，均需红线勾选与静态检查。
- **测试**：每波必须包含负控制测试（故意错误必须被拦）与金标用例（确定值必须一致），
  沿用 `make verify`（lint + build + race）。
- **措辞**：新增面向用户文案遵循 `docs/tone-style-guide.md`（不绝对化、带置信度、拒绝时给替代方案）。
- **环境前置**：每个任务开工前跑 `scripts/check-domain-deps`（python3/python-docx/dot/inkscape/
  freecadcmd/soffice/pandoc/typst 可用性矩阵），缺失依赖不得静默降级。
- **评审记录**：本方案已按 `docs/plans/dsh-domain-import-review.md` 的 5 项 P0 与 5 项 P1 修订；
  后续变更须在评审文档追加变更记录，不覆盖原评审结论。

---

## 九、门禁与状态（2026-10-28 更新）

| 波次 | 状态 | 提交 | 说明 |
|---|---|---|---|
| W-1 | ✅ 完成 | 1b1dd92 | ADR 0008 已批准；FreeCAD 结论为暂不立项 |
| W0 | ✅ 完成 | 98e1828 | `pkg/indexasset` 覆盖率 95.5%；资产指纹脚本落地 |
| W1 | ✅ 完成 | 6a2e7ed · b0bd250 · 65da87c · b8bec75 · 8d244bb · ec93acd · 174d5f8 | T-W1-01～05 完成；门禁接入 107 条 |
| W2 | ◐ 进行中 | 27b0085 · 21eaec1 · 47cd499 | T-W2-01（交付模板）、T-W2-02（CNIPA 出件）、T-W2-03（交付件登记）、T-W2-04（模板去重）完成；T-W2-05 待做 |

### 批 A / 批 B 规则接入状态

- **来源实测**：dsh `patent-rule` 共 158 条规则（此前按单缩进统计为 44 条，偏少），其中可确定性判定 130 条。
- **已接入 107 条**：
  - 批 A 96 条：block 级 32、结构性（structural_analysis）61、手工 3（PAT-RISK-001 / PAT-APPROVAL-001 / PAT-ABS-001）；
  - 批 B 11 条：warn/review 级非结构性关键词规则（LEGAL-DISC-001、LEGAL-LIMIT-001、LEGAL-JURIS-001、LEGAL-ABS-001、CON-COMP-0103、CON-COMP-0105、EX-SRC-001、H-CLA-002、IPC-GEN-INV-002、EX-INV-007、INV-EVIDENCE-001），**只提示不阻断**。
- **未接入 23 条**：log 级规则（Mady 无 log 动作，降级为 warn 会改变语义）与需要 LLM 判断的规则。
- **批 B 误报观察点**（部署后需复核）：
  - `CON-COMP-0103` 的「根据法律规定」会命中法律文书的常见泛称；
  - `IPC-GEN-INV-002` 的「事后诸葛亮」没有否定语境豁免，判断句中提及即命中；
  - 两条均为 warn 级，只追加提示；OutputGate 对 warn 设 5 条上限，避免刷屏。
- **误报抽检**：`TestWarnRulesFalsePositiveSamples` 锁定「精确引用 / 否定语境 / 引文豁免」三类不误报，并记录上述两条已知命中。
- **生成与校验**：`go run ./scripts/rulescan --src <dsh 规则目录> --emit domains/rules/data/rules/gate` 机械转换；全部规则通过 `ValidateGateRules`；`TestStructuralPatternRegex` 验证正则模式（如 `专利法[^。；\n]{0,12}第\s*(?:二十二|22)\s*条`）确实命中。
- **反向验证**：移除生成文件后 `TestBundledGateRulesAllConsumable` 变红（只剩 3 条），还原后恢复；破坏 `keyword_blocklist` 判定后 OutputGate 与 StructuralGate 两个测试变红。

### 门禁语义差异（与 dsh 不同，需知悉）

1. **block 的时机**：dsh 的 post-execute block 是硬拦截；Mady 的 `AfterToolExecution` 在工具已执行后运行，只能标注「不可作为交付依据」，真正的拦截在 `StructuralGate`（`BeforeToolExecution`）。
2. **结构性规则的执行点**：`structural_analysis` 不在 OutputGate 生效——它对任意工具结果全文都会报「缺要素」，只对交付制品文本判定。`StructuralGate` 已实现（`BeforeToolExecution`，按 `TextArgs` 取正文），但**尚未接线**：现有交付工具 `deliver_register` 的入参是文件路径、不含正文，配 entries 无从判起；需要一个把模板正文暴露为入参的渲染工具，或在出件链路上直接调用引擎（见 T-W2-03 状态节的遗留项）。
3. **review 级**：Mady 的 OutputGate 同样按标注处理，审批仍由既有 ApprovalGate 在模型输出层负责。
4. **骨架约束**：结构性规则不参与法条核验的状态判定，`ExistOnly` 仍不会被提升为 `Verified`（见 fail-closed 判定表 FC-1）。

### W2 出件状态（T-W2-02）

- **体例真相源**：`domains/filing/data/template/申请文件模板.docx`（2012 CNIPA 体例的去标识化副本，5 节、宋体/Times New Roman 12pt、1.5 倍行距、首行缩进 24pt）。桌面原件当前不存在，以随包副本为准；其 sha256 与 `TEMPLATE-IDENTITY.md` 登记值一致，由 `TestTemplateFingerprint` 守住。
- **两步流程**：`build.py` 负责按模板反解体例并成文，`verify.py` 负责节数/编号/禁用词/禁用色/字号断言；验收不通过时删除成品，不留半成品。
- **Go 编排**：`domains/filing.Compose` 解压随包资产、调用引擎、校验报告；Python 解释器按「能 import docx」探测（本机 3.15 未装 python-docx，实际落到 3.11）。
- **未做**：出件工具接线（`filing.Compose` 仍是库 API）、模板去重（T-W2-04）、Office→PDF（T-W2-05）。

### W2 交付模板状态（T-W2-01）

- **资产**：13 套交付模板随包落位 `domains/deliver/data/templates/patent/`（732KB，含 SKILL.md、template.html、example、checklist/citation-log/conventions、tokens.css、manifest.json）。
- **渲染**：`deliver.Render` 用 x/net/html 解析模板，按 id 注入 HTML 片段、CSS 变量注入品牌、按 `data-paragraph-numbering` 写段落编号。编号语义与 dsh 对齐：标题/表格/figure/预格式不编号，段落不嵌套，图片与空白段不占号，已有编号先剥再写（幂等），编号后带一个空格。
- **合规**：`deliver.CheckCompliance` 四条规则——章节编号递增不重复、对外件内部章节、对外件内部正文、证据清单与正文双向核对。证据匹配带左侧 ASCII 边界（END2 不会被当作 D2），正文判定按节点切除证据表。内部记录模板（invalidation-analysis）走 `ScopeInternalRecord`，不查内部记录规则。
- **交付件核对**：`deliver.VerifyDeliverable` 检查主路径与交付副本字节一致、渲染件不早于其输入、要求-证据齐备，并产出 hash/mtime 清单；只接受普通文件且单文件 ≤64MB。
- **独立审查**：code-review 发现 4 项 blocking（figure 排除、可见文本、嵌套编号、证据前缀边界）与 4 项 suggestion，已全部修复或记录；幂等剥号一并补齐。
- **未做**：模板去重（T-W2-04）、Office→PDF（T-W2-05）；模板渲染仍是库 API——登记工具只接收渲染后的文件路径，不接收正文（见 T-W2-03 状态节遗留项）。

### W2 交付登记状态（T-W2-03）

- **入口**：`deliver_register` 工具（`deliver.NewExtension` 注入，与其它扩展一致）。一次登记给 `template_id`、`artifacts`（主路径 + 交付副本 + 角色）、`figures`、`rendered`、`requirements`、`self_check`；`artifacts` 与 `rendered` 至少给一项。`RenderAndRegister` 另提供「渲染 → 落盘 → 登记」一条调用。
- **核验**：模板存在性、交付件一致性（复用 `VerifyDeliverable`）、HTML 渲染件四条合规规则。清单含路径、sha256、大小、修改时间（统一 UTC）。
- **三值结论**：`pass` / `fail` / `inconclusive`。`inconclusive` 表示没有未通过项但有核验未执行；`Passed` 收紧为仅 `pass` 时为真。空交付（无 artifacts 且无 rendered）直接判 `fail`，不因「没有文件可查」而放行。
- **事件**：登记完成后发 `deliver/registered`（`agentcore.DeliverRegisteredEvent`，只带通用字段），已加入 `event_logger` 持久化白名单，会话日志可回放。
- **未执行项的边界**：PDF 渲染件无法做 HTML 合规检查，登记结论为 `inconclusive` 并在 `unverified` 中说明。T-W2-05 落地后应在 HTML→PDF 转换前检查源件，而不是回过头查 PDF。
- **独立审查**：code-review 报出 1 项 blocking（`Unverified` 存在时 `Passed` 仍为 true，把「未核验」呈现成「通过」）与 5 项 suggestion；blocking 以三值结论修复，suggestion 采纳 4 项、拒绝 1 项（显式逐字段构造与 staticcheck S1016 冲突，且类型转换本就要求字段名/类型/顺序完全一致）。
- **遗留**：`StructuralGate` 对交付正文的接线缺少正文入口——登记工具只收路径，需在渲染/出件链路上把正文送入 `Engine`；出件工具（`filing.Compose`）也仍是库 API。

### W2 模板去重状态（T-W2-04）

- **真相源**：`domains/doctmpl/templates/`（`go:embed`）。仓库根 `doc-templates/` 是同内容镜像，供桌面端浏览与用户参考；运行时覆盖走 `$MADY_HOME/doc-templates/`。
- **实测冲突**：dsh 17 份与 Mady 22 份中，12 份同名模板**正文逐字节相同**，差异只在 frontmatter——dsh 多 `language` / `formats` / `vars`。合并方式为「取 dsh 的 frontmatter + 保留 Mady 正文」，没有内容取舍；`vars` 全部 `required: false`，不改变现有渲染行为。
- **新增 5 份**：dsh `document/` 的 PRD、发布说明、分析报告、FAQ、会议纪要（`domain: document`），Mady 此前没有通用文档模板。
- **Mady 独有 10 份**：`legal/` 5 份与 `patent/` 5 份（Sati 对齐阶段落位）保留不动。
- **合并清单**：`domains/doctmpl/TEMPLATE-MANIFEST.md` 逐条记录 27 份模板的 ID、类别、目录、域、来源与处置；`manifest_test.go` 校验清单与目录一一对应、ID 唯一、category/目录与实际一致、变量声明不重复。
- **镜像**：`doc-templates/` 补齐此前缺失的 `patent/` 5 份，测试逐字节比对镜像与内置，任一侧单独改动即失败。
- **索引修复**：`DocIndex` 的类别顺序此前硬编码 5 类，`patent-report` 的 5 份模板从未出现在模板索引里；现补上 `patent-report` / `document`，并把未登记类别按字母序追加，避免以后新增目录再次从索引消失。
- **文档修正**：README 与 `docs/workflows.md` 此前称根目录 `doc-templates/` 被 `go:embed` 编进二进制，实际 embed 的是 `domains/doctmpl/templates/`，已更正。
