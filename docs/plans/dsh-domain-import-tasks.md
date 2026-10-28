# DeepSeek Harness 三域引入：任务清单与检查清单

> 配套方案：`docs/plans/dsh-domain-import-plan.md`
> 用法：按波次顺序执行；每个任务完成即勾选并跑通用 DoD；每波结束跑波次门禁；
> 合并前跑全局门禁。任务 ID 一经使用不复用，废弃任务标 ~~删除线~~ 并注明原因。
> 人日为 AI 协作口径估算，含测试与文档。

---

## 一、波次总览

| 波次 | 主题 | 任务数 | 人日 | 前置 | 可并行 |
|---|---|---:|---:|---|---|
| W-1 | 前置 spike（FreeCAD 研究/团队 ADR/环境探测） | 3 | 3.5 | — | — |
| W0 | 基座与资产落位 | 4 | 3 | W-1 | — |
| W1 | 可核验性（法条/费用/期限/门禁） | 5 | 36 | W0 | W3、W4 |
| W2 | 交付闭环（模板/出件/登记） | 5 | 35 | W0、W1 | W4 |
| W3 | 法律域（66 条证据/四类检索） | 2 | 11 | W0 | W1、W4 |
| W4 | 附图生成（F1/F2/F4） | 3 | 26 | W0 | W1、W2、W3 |
| W5 | 协作与外围 | 5 | 48 | W-1、W1、W2 | — |
| W6 | 桌面呈现（可选） | 1 | 8 | W2 | — |

**推荐执行顺序**：W-1 → W0 → W1 → W3（可与 W1 后段并行）→ W2 → W4（可与 W2 并行）→ W5 → W6（按需）。
FreeCAD 结构图（原 W4-F3）不在本表，由 T-R-01 研究结论决定是否单独立项。

---

## 二、任务清单

### W-1 前置 spike（3.5 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-R-01 | FreeCAD 能力研究 | — | 2 | `docs/plans/freecad-capability-study.md` | 覆盖安装路径/版本约束/非交互入口/中文 locale/失败模式；给出「立项/不立项」结论 |
| T-R-02 | 团队模型 ADR | — | 1 | `docs/adr/000X-team-model.md` | 对比 worker/tasklist/provisions 与 patent-teams；给出扩展或新增的选择 |
| T-R-03 | 环境探测脚本 | — | 0.5 | `scripts/check-domain-deps` | 输出依赖矩阵；缺失项退出码非零 |

### W0 基座与资产落位（3 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W0-01 | 实现 `pkg/indexasset` | — | 1 | `pkg/indexasset/indexasset.go` + `indexasset_test.go`：`ParseYAMLMapping`、字段读取器、`RequireKnownFields`、`RecordedSource`、`ReadArticleEntries`、`*indexasset.Error` | 单测 95.5% 覆盖；字段缺失/类型错/未知字段/来源不完整均报错 |
| T-W0-02 | 制定资产目录与格式约定 | — | 0.5 | `docs/design/index-asset-format.md` | 随包/外置判据、目录布局、字段规则、来源三态齐全 |
| T-W0-03 | 资产指纹脚本 | T-W0-02 | 1 | `scripts/asset-fingerprint` + `data/ASSETS.sha256` | 生成/校验/篡改检测自测通过；重复生成幂等 |
| T-W0-04 | fail-closed 判定表文档 | T-W0-01 | 0.5 | `docs/design/fail-closed-matrix.md`（FC-1…FC-11） | 每条含触发条件/行为/措辞/测试；降级清单单列 |

### W1 可核验性（31 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W1-01 | 法条核验升级 | T-W0-01 | 5 | `lawcite.Resolve` 四值状态（Verified/ExistOnly/OutOfRange/Unverified）+ Gate 到既有五值的映射；`law_verify` 工具 | 已转录 37 条逐条用例；专利法其余 45 条必须非 Verified；条号漂移回归 |
| T-W1-02 | 法规骨架并入 | T-W1-01 | 4 | `data/law/skeleton/`（7,910 份，17 份带条文）；精校子集优先 | 随机 50 部法规条号命中；骨架不产生 `Verified` |
| T-W1-03 | 费用测算 | T-W0-01 | 6 | `domains/fees` + `cn-fees.yaml` + `patent_fees` 工具 | 15 个金标用例；`null` 金额触发 `Refused`；减缴/分档/滞纳金各 3 例 |
| T-W1-04 | 期限增强 | T-W0-01 | 6 | `AddServiceTerm`、`ExtendToWorkday`、`cn-holidays.yaml` | 12 个日期用例（春节/国庆/跨年/调休）；缺年份报错 |
| T-W1-05 | 门禁接线（分两批） | T-W1-01、T-W1-03、T-W1-04 | 15 | `domains/rules/gate`（Output/Structural/Evidence）；44 条规则/200+ check 块适配：批 A 为 block+结构性，批 B 为 warn/log | 5 个翻转攻击测试；未知 `check.type` 报错；批 A 全部生效；DeliveryGate 不在本波（归 W2） |

### W2 交付闭环（32 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W2-01 | 13 套交付模板 + 渲染 | T-W1-01 | 15 | `skills/patent/templates/*`；HTML 模板渲染通道；段落编号 `[NNNN]`；`brandInjector`；`htmlScan`；`Compliance`（四条规则）；`VerifyDeliverable`（路径/时序/清单） | 13 套各渲染一次；四条合规规则各 1 反例被拦；段落编号连续；`VerifyDeliverable` 对主路径/副本不一致判失败 |
| T-W2-02 | CNIPA 出件 | T-W0-01 | 10 | `domains/filing` + Python 引擎编排 + `申请文件.docx` 模板 | 样例出件通过 5 节/编号/禁用词/禁用色/字号断言；故意注入禁用词失败 |
| T-W2-03 | 交付件登记 | T-W2-01 | 4 | `deliver.Register`；`deliver/registered` 事件 | 登记项含 hash/格式/两列结论；会话日志可回放 |
| T-W2-04 | 文档模板并入去重 | T-W0-02 | 3 | dsh 17 份模板与 Mady 22 份合并清单 | 无重复 ID；冲突项按来源字段合并；目录清单测试 |
| T-W2-05 | Office→PDF（双通道） | — | 3 | `tools/office_pdf.go`：LibreOffice 原生 → pandoc+typst → 拒绝 | 6 种格式各 1 例；本机无 LibreOffice 时走 pandoc 通道并抽查中文版式 |

### W3 法律域（11 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W3-01 | 民诉法 66 条证据模型（二维） | — | 5 | `domains/evidence/statutory.go`：`EvidenceKind`（法定种类）∥ `AcquisitionContext`（取证场景） | 八类各 1 个评分用例；两维组合至少 5 例；不建立互相覆盖的映射表 |
| T-W3-02 | 四类司法检索 | T-W0-01 | 6 | `knowledge/legal`；`legal_knowledge_install` | 四类各 5 例；无库降级；「指导案例第 N 号」检索 |

### W4 附图生成（26 人日，F3 移出）

> 源基线：只移植 dsh master 已合入且可验证的部分（断线、跨图一致性、渲染门禁、页码前缀等）。
> FreeCAD 结构图（原 F3）已移出，由 T-R-01 研究结论决定是否立项。

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W4-01 | F1 矢量图 + Graphviz | T-W0-02 | 8 | `domains/figure/generate`（dot 构建、渲染、SVG 视口/后处理） | 3 个案例渲染；SVG 结构校验 |
| T-W4-02 | F2 图型 + 渲染核验 | T-W4-01 | 10 | 流程图/框图/时序图/剖面图；引线穿字与字框检测；断线 | 净距分档、引线穿字 2 个门禁用例；3 个案例 |
| T-W4-03 | F4 一致性与交付 | T-W4-02 | 8 | 图号/标记/摘要附图/跨图一致性/提交页 | 与 `domains/figure` 现有核验对齐；3 个案例 |

### W5 协作与外围（48 人日）

> 团队模型形态取决于 W-1 的 T-R-02 ADR；T-W5-01 估算 ±40%，ADR 后重估。

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W5-01 | patent-teams | T-R-02 | 15 ±40% | 队长-成员、邮箱、依赖 DAG、调度器、锁、状态视图 | 3 智能体协作用例；依赖乱序调度正确；锁竞争用例 |
| T-W5-02 | patent-core 增量 | — | 15 | TRIZ、adversary、analysis-report、四检验、coverage/unity、response/plan | 每项 3 个用例；与既有模块不重复实现 |
| T-W5-03 | patent-tools 补齐 | T-W1、T-W4 | 10 | 见附录 A 标注「缺」「部分」的工具 | 每个工具「规范值 + render」双测 |
| T-W5-04 | patent-data/knowledge 增强 | — | 5 | LRU 缓存、CNIPR 通道、wiki 卡片查询、install 引导 | 缓存命中率用例；CNIPR 通道失败降级 |
| T-W5-05 | tool-literature 增强 | — | 3 | OpenAlex、`paper_download`、按主机限速、LRU | 限速不串行化；OpenAlex 5 例 |

### W6 桌面呈现（可选，8 人日）

| ID | 任务 | 依赖 | 人日 | 产出 | 验收 |
|---|---|---|---:|---|---|
| T-W6-01 | 交付工作室 | T-W2-03 | 8 | `desktop/` 交付物列表 + 两列质量门 | 与登记事件一致；空状态/失败态截图 |

---

## 附录 A：`patent-tools` 55 个工具文件处置

图例：**有**＝Mady 已等价；**部分**＝需补齐；**缺**＝需新增；**W4**＝归附图波次。

| # | 工具文件 | 状态 | 落点 / 说明 |
|---:|---|---|---|
| 1 | add-patent-figure-references | W4 | F4（dsh 已合入） |
| 2 | analyze-patent-figure | W4 | F2 |
| 3 | check-patent-figure-consistency | 有 | `domains/figure` |
| 4 | claim-chart-build | 有 | `domains/claimchart` |
| 5 | compare-patent-figure-analysis | W4 | F4 |
| 6 | draft-claims | 有 | `domains/claimdrafting` |
| 7 | draft-specification | 有 | `domains/specdrafting` |
| 8 | evaluate-evidence | 有 | `domains/evidence` |
| 9–16 | figure-delivery / input / output / panels / render-plan / run / submission / tool-schemas | W4 | F1–F4 |
| 17 | generate-patent-figure | W4 | F1 |
| 18 | generate-structure-figure | 待研究 | 依赖 FreeCAD，T-R-01 结论前不立项 |
| 19 | knowledge-note-file-writer | 有 | `knowledge/patenttools` 写入 user.db（可被 search_knowledge 召回），不落文件 |
| 20 | knowledge-note-save | 有 | `knowledge_note_save`（id 幂等，写 user.db） |
| 21 | law-search | 有 | `legal_knowledge_search`（四类检索 + 效力四态 + 来源路径）与 `search_laws` |
| 22 | matter-log-append | 有 | `domains/matterlog`（W5-03）：只追加日志 + 取代链校验 + 五类拒绝 |
| 23 | parse-office-action | 有 | `domains/rules/oa_parser` |
| 24 | patent-analysis-report | 有 | `domains/analysisreport`（W5-02） |
| 25 | patent-case-search | 有 | `knowledge/sqlite` |
| 26 | patent-eval | 有 | `tools/patent_eval.go` |
| 27 | patent-flexible-plan | 有 | `domains/plantask` |
| 28 | patent-kg-query | 有 | `knowledge/graph` |
| 29 | patent-legal-status | 有 | `tools/patent_search.go` |
| 30 | patent-metadata | 有 | 同上 |
| 31–33 | patent-pdf-download / -channel / -ego | 有 | 同上 |
| 34 | patent-plan-task | 有 | `domains/plantask` |
| 35 | patent-search | 有 | `tools/patent_search.go` |
| 36 | patent-wiki-search | 有 | `knowledge/patenttools` |
| 37 | patent-worker-validate | 有 | `domains/workercontract` |
| 38 | patent-workflow-run | 有 | `domains/workflows/patent` |
| 39 | patent-workflow | 有 | 同上 |
| 40 | recognize-chemical-structure | 部分 | 需视觉模型与结构识别引擎，W5-03 未做（见未覆盖） |
| 41 | rule-check | 有 | `domains/rules`；门禁化见 T-W1-05 |
| 42 | search-patent-figure | W4 | F4 |
| 43 | spec-claim-coverage | 有 | `domains/claimdrafting` coverage + unity（W5-02 修正口径） |
| 44 | spec-figure-marks | W4 | F4 |
| 45 | spec-numeric | 有 | `domains/novelty/numeric_range.go` |
| 46 | spec-quantification | 有 | `domains/specdrafting/quantification.go`（W5-03） |
| 47 | spec-style | 有 | `domains/specdrafting/style.go`（W5-03）：五部分标题/顺序、附图标记、图号、段号、摘要标题 |
| 48 | spec-types | 有 | 类型词汇并入 `domains/specdrafting` 的 SpecInput/ScoreReport 契约 |
| 49–51 | structure-model / -render / -types | 待研究 | 依赖 FreeCAD，随 T-R-01 结论 |
| 52 | triz-contradiction-analysis | 有 | `domains/triz`（W5-02） |
| 53 | validate-specification | 有 | `domains/specdrafting` |
| 54 | verify-patent-figure | W4 | F4 |
| 55 | workbench-link-patent-case | 不适用 | 依赖 dsh 个人工作台 loopback 服务；Mady 案件管理由 `domains/caseindex` 承担 |

---

## 三、检查清单

### 3.1 每个任务的通用 DoD（Definition of Done）

- [ ] 开工前跑 `scripts/check-domain-deps`，确认本任务所需外部依赖可用；缺失则先记录并调整方案
- [ ] 先读后改：实现前已读相关既有模块，确认不重复造轮子
- [ ] 接口对等：dsh 契约 → Go 接口的映射已记录在代码注释或设计小节
- [ ] 正常路径测试通过
- [ ] **负控制测试**：故意错误输入必须被拒绝/拦截（fail-closed 验证）
- [ ] **金标用例**：确定值（金额/日期/条号/编号）有硬编码期望
- [ ] `go build ./...` 与 `go test ./<module>/...` 通过
- [ ] 如涉及新工具：`docs/TOOL_CONTRACT.md` 契约（规范值 + render）满足
- [ ] 如涉及资产：来源注释（`sourceDoc`/`verifiedOn`/文号）保留
- [ ] 如涉及用户文案：符合 `docs/tone-style-guide.md`
- [ ] 无 TODO/FIXME 残留，无「80% 完成」
- [ ] AI_CHANGELOG 条目已追加（非文档/测试类变更）

### 3.1a W-1 波次门禁

- [ ] FreeCAD 能力研究覆盖：安装路径、版本约束（1.1 API）、非交互入口、中文 locale、失败模式
- [ ] 研究报告给出明确结论：立项 / 不立项 / 延后，并说明依据
- [ ] 团队模型 ADR 已批准，明确「扩展 worker」或「新增 domains/teams」
- [ ] `scripts/check-domain-deps` 可重复运行，输出矩阵完整

### 3.2 W0 波次门禁

- [x] `pkg/indexasset` 测试通过，未知字段/缺失字段均报错
- [x] 资产目录与格式文档定稿，方案中四处引用一致
- [x] 指纹脚本可重复生成相同清单（幂等）
- [x] fail-closed 判定表 W0 范围内的 FC-1（未转录）、FC-4（来源不全）有测试；FC-2/3/5/6/7/8/9/10/11 在对应波次补测试

### 3.3 W1 波次门禁

- [ ] 已转录 37 条专利法逐条核验测试通过；其余 45 条必须返回非 `Verified`
- [ ] 未转录条文 100% 不返回 `Verified`（抽样 + 全量自动断言）
- [ ] `ExistOnly` 与 `Verified` 在工具输出与报告中措辞不同
- [ ] 费用 15 个金标用例通过；金额 `null` 一律 `Refused`
- [ ] 期限 12 个日期用例通过；缺年份报错而非静默
- [ ] 门禁：deny 不可翻转（5 个攻击测试）；未知 `check.type` 报错
- [ ] 交付前闸门：未跑 `rule_check` 的交付调用被拒绝
- [ ] 敏感路径检查：`scripts/check-sensitive-paths.sh` 通过并人工复核引用了 `guardrails/citation_*`

### 3.4 W2 波次门禁

- [ ] 13 套模板全部渲染成功
- [ ] `Compliance` 四条规则（章节编号/内部章节/内部正文/证据交叉引用）各有 1 个反例被拦截
- [ ] `VerifyDeliverable`：主路径与交付副本不一致时判失败；渲染产物早于输入时判失败
- [ ] 段落编号 `[NNNN]` 连续，成文与验收共用同一匹配式
- [ ] 模板资产与 dsh 原件的差异清单为空（除品牌参数化项）
- [ ] 出件样例通过全部断言；3 个反例（禁用词/彩色/编号断号）均失败
- [ ] 体例从模板反解，未硬编码字号行距
- [ ] 交付登记两列（确定性核验 / 模型自述）在事件中并存
- [ ] Office→PDF 在 LibreOffice 与 pandoc 两条通道下行为明确；两者皆缺时结构化拒绝

### 3.5 W3 波次门禁

- [ ] 民诉法 66 条八类各有用例；与 15 类专利取证映射表无遗漏
- [ ] 四类检索各 5 例；无库时降级而非报错
- [ ] 指导案例编号检索可用
- [ ] 安装命令幂等，不修改源库

### 3.6 W4 波次门禁（每期单独验收）

- [ ] F1：3 个案例渲染成功，SVG 结构校验通过
- [ ] F2：净距分档与引线穿字门禁生效（2 个反例被拦）
- [ ] F4：图号/标记/跨图一致性与既有 `domains/figure` 结论一致
- [ ] 所有生成物可追溯到输入模型（同一输入两次运行结果一致或差异可解释）
- [ ] 未引入 FreeCAD 依赖（F3 未立项前不得出现 `freecadcmd` 调用）

### 3.7 W5 波次门禁

- [ ] 团队 ADR 已批准
- [ ] 3 智能体协作用例通过；依赖乱序调度正确；锁竞争无死锁
- [ ] 新增工具全部满足契约双测
- [ ] 工具总数增长后 preset 装配未全量注入（按域分组）
- [ ] 文献源限速生效且不串行化跨源请求

### 3.8 全局门禁（每个 PR / 每波合并前）

- [ ] `make verify` 通过（lint + build + race，覆盖全部模块）
- [ ] `scripts/check-domain-deps` 矩阵与任务声明一致（不在缺失依赖上宣称完成）
- [ ] `cd tui && go test ./...` 通过（若触及共享类型）
- [ ] 覆盖率不低于该模块既有基线
- [ ] 规范元规则六条自查（三件套/例外显式/事实清单可验证/本地窄 CI 全/覆盖率定位/fail-closed）
- [ ] 敏感路径清单核对（`scripts/check-sensitive-paths.sh`）
- [ ] AI_CHANGELOG 已追加，`INDEX.json` 已更新
- [ ] 新增文档已加入对应索引（`docs/plans/README.md` 或目录 README）

### 3.9 风险复检（每波结束时）

- [ ] 是否引入了不可移植的外部依赖？是否已提供降级路径？
- [ ] 是否把「骨架/未转录」误报为「已核验」？（抽查 20 条）
- [ ] 是否有两套并行实现（例如证据分类、团队模型）未收敛？
- [ ] 资产是否被手改而与来源漂移？（指纹校验）
- [ ] 工具数量增长是否导致提示词/上下文膨胀？（测量 preset 注入量）

---

## 四、进度追踪

| 波次 | 状态 | 起止 | 提交 | 备注 |
|---|---|---|---|---|
| W-1 | ✅ 已完成 | 2026-10-28 | 未提交 | ADR 0008 已批准；FreeCAD 结论为「暂不立项」 |
| W0 | ✅ 已完成 | 2026-10-28 | 98e1828 | 4 项完成；indexasset 覆盖 95.5%；门禁 4/4 |
| W1 | ✅ 完成 | 2026-10-28 | 6a2e7ed…ec93acd | T-W1-01~05 完成；门禁共接入 107 条（批 A 96 + 批 B 11） |
| W2 | ✅ 完成 | 2026-10-28 | 27b0085 · 21eaec1 · 47cd499 · fa41576 · eb3517b · d5936e8 · c1fdc6d | T-W2-01～T-W2-05 全部完成 |
| W3 | ✅ 完成 | 2026-10-28 | b6bda17 · b399490 | T-W3-01、T-W3-02 全部完成 |
| W4-F1 | ✅ 完成 | 2026-10-28 | e32b069 | T-W4-01 完成：DOT 构建 + Graphviz 渲染 + SVG 视口/标号 |
| W4-F2 | ✅ 完成 | 2026-10-28 | 83e6dea | T-W4-02 完成：字框/渲染核验 + 时序图 + 剖视图 |
| W4-F3 | ✅ 完成 | 2026-10-28 | 9d7fb05 | T-W4-03 完成：法域档案 / 提交页落版 / 跨图一致性 + 两个工具 |
| W5-02 | ✅ 完成 | 2026-10-28 | e307ef3 | T-W5-02 完成：TRIZ、四检验增量、覆盖/单一性修正、OA 答复计划、分析报告、对立审查 |
| W5-03 | ✅ 完成（化学结构识别除外） | 2026-10-28 | 6c7fd42 | 说明书形式/量化检查 + 案卷事件日志；其余工具经核验已有等价实现或不适配 |
| W5-04 | ✅ 完成 | 2026-10-28 | 本提交 | LRU/TTL/并发合并缓存 + CNIPR 通道 + 知识库状态与安装引导 |
| W5（其余） | ☐ 未开始 | | | T-W5-01 团队、T-W5-03 工具补齐、T-W5-04 数据/知识、T-W5-05 文献 |
| W6（可选） | ☐ 未开始 | | | |

> 状态取值：未开始 / 进行中 / 待验收 / 已完成 / 已搁置（附原因）。
