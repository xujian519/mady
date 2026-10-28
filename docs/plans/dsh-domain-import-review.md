# DeepSeek Harness 三域引入方案：评审报告

> 评审对象：`docs/plans/dsh-domain-import-plan.md`、`docs/plans/dsh-domain-import-tasks.md`
> 评审方式：对方案中的技术假设逐项实测（本机环境探测 + 两仓源码核验），不只做文档自洽检查
> 评审日期：2026-10-28

---

## 一、结论

**有条件批准。** 方案的方向、波次划分与 fail-closed 原则成立，但有 **5 项阻断级问题**必须在开工前修订，
其中 3 项是事实错误（会导致返工），2 项是环境前提不成立（会导致任务无法交付）。
建议先执行 **W-1 前置 spike（3 人日）**，再按修订后的 W0 开工。

估算修正：由 164–172 人日调整为 **160–168 人日（不含 FreeCAD 结构图与 W6）**，
并新增 FreeCAD 能力研究作为独立可选任务。

---

## 二、已核验成立的假设

| 假设 | 实测证据 | 结论 |
|---|---|---|
| 出件引擎可在本机运行 | Python 3.11.16 + `python-docx` 1.2.0 已安装 | ✅ 成立 |
| Graphviz 渲染可用 | `/opt/homebrew/bin/dot` 存在 | ✅ 成立 |
| Inkscape 渲染可用 | `/opt/homebrew/bin/inkscape` 存在 | ✅ 成立 |
| Mady `graph/` 具备 Pregel 生态 | `node_policy.go`、`degradation.go`、`state_schema.go` 齐备 | ✅ 跳过 `atoms` 的判断成立 |
| 交付前置闸门可接线 | `ToolExecutionContext{ToolCalls, Results}` 存在；`BeforeToolExecution` 可取当前工具名 | ✅ 成立（跨轮状态需 hook 自维护） |
| 目标目录无冲突 | `pkg/indexasset`、`domains/{fees,filing,deliver,teams,triz}`、`knowledge/legal` 均不存在 | ✅ 成立 |
| 附图默认路径零系统依赖 | `render-selector.ts`：wasm 为主，CLI 仅 png/pdf 与超限图回退 | ✅ 成立（Go 侧仍建议用 dot CLI） |
| 撰写语料/样式同源 | 10 份语料、4 套样式、`mady-gap-rules.yaml` 均标注来自 Mady | ✅ 跳过正确 |

---

## 三、必须修正的问题

### P0-1 `verify_deliverable` 的语义写错了（事实错误）

方案 §W2-01 把它描述为「模板结构断言（章节、必备段落）、占位符残留、引用日志与正文引用一致性」。
**实际实现**（`patent-document/src/tool/verify-deliverable.ts`）判定的是：

1. 案件主路径文件与交付副本是否**字节一致**（防止读者打开主路径看到旧稿）；
2. 渲染产物是否**晚于其输入**（按 mtime 排序，防止先出件后改稿）；
3. 产出交付清单（输入 hash + mtime），供交付报告引用。

「模板结构/合规检查」是另一个函数 `checkDocumentCompliance(html, scope)`，四条规则：
`section-numbering`（中文数码章节编号递增不重复）、`internal-section`（对外件不得含内部工作章节）、
`internal-body`（不得含待办/核验记录/成功率等内部用语）、`evidence-cross-reference`（证据编号交叉核对），
并按 `external-filing` / `internal-record` 两档分流。

**修订**：W2-01 拆成三项能力——`Render`（模板渲染）、`Compliance`（四条 HTML 合规规则）、
`VerifyDeliverable`（路径一致性 + 时序 + 清单）。后者并入 T-W2-03 的登记模型。

### P0-2 民诉法 66 条八类与 Mady 15 类是正交维度，不能「做映射表」

方案 §W3-01 写「与既有 15 类专利取证场景做映射表」。但 66 条是**法定证据种类**
（书证/物证/视听资料/电子数据/证人证言/当事人陈述/鉴定意见/勘验笔录），
Mady 的 15 类是**取证场景/证据形态**（互联网公开/使用公开/域外证据/公证…）。
两者不是同一分类轴，强行一一映射会同时损失两边语义（例如「互联网公开」可对应电子数据，
但其证明力判断依赖公开时间与可及性，与证据种类无关）。

**修订**：证据模型改为**二维**——`EvidenceKind`（66 条八类，法定）∥ `AcquisitionContext`（Mady 15 类，取证场景）。
三性评分分别从两维取规则，不做互相覆盖的映射表。

### P0-3 FreeCAD 缺失，且方案违背「先研究能力」的既有先例

本机 `freecad` / `FreeCADCmd` 均未安装。方案 W4-F3（12–20 人日）直接安排「通过 subprocess 调 FreeCAD CLI」，
但 2026-09-30 已确认的方向是**先研究充分利用 FreeCAD 的能力，而非直接推进生成侧改造**。

**修订**：W4-F3 从实施波次中移出，改为独立任务 `T-R-01 FreeCAD 能力研究（spike，2 人日）`，
产出一页能力边界报告（可做什么图、依赖版本、脚本化入口、失败模式），再决定是否立项。
F1/F2/F4 不受影响。

### P0-4 LibreOffice 缺失，Office→PDF 无原生引擎

本机 `soffice` / `libreoffice` 均未安装。方案 T-W2-05 写「探测 LibreOffice，缺失时返回结构化降级提示」——
结论正确但等于该任务在当前环境**不可验收**。本机可用的是 `pandoc` + `typst`。

**修订**：T-W2-05 改为双通道：LibreOffice 原生优先 → `pandoc + typst` 兜底（docx→typst→pdf；
需实测版式保真度，尤其是中文字体与表格）→ 两者皆缺时结构化拒绝。验收增加「pandoc 通道版式抽查」。

### P0-5 W1 门禁与 W2 交付工具存在循环依赖

DeliveryGate 要在交付工具执行前检查「本会话是否已成功执行 rule_check」，
但交付工具（render/verify/compose）在 W2 才存在；W1 又排在 W2 之前。

**修订**：门禁分两期接线——
- W1 只接 `OutputGate`（AfterToolExecution，全局生效）与 `EvidenceGuard`（单调 deny），验收用已有工具；
- W2 完成交付工具后再接 `DeliveryGate`，归入 T-W2-03 的验收项。

---

## 四、重要问题（影响工作量与验收）

### P1-1 模板模型不是同一种，工作量需上调

Mady `doctmpl` 是 **Markdown 模板 + `{{var}}` 替换 → Markdown/HTML/DOCX/PDF 渲染**；
dsh `patent-document` 是 **HTML 模板 + 受控草案转换（draftConverter/draftSchema）+ 品牌注入 + 段落编号 `[NNNN]` + 合规检查**。
实测 Mady 无段落编号能力（grep 无命中），HTML 渲染器只做 Markdown→HTML 转换，不能直接吃 HTML 模板。

**修订**：T-W2-01 增加「HTML 模板渲染通道」（原生 HTML 模板 + 槽位填充，不复用 goldmark）
与「段路编号器」两个子项；工作量 12 → **15 人日**。

### P1-2 规则包适配比预想重

dsh `patent-rule` 实测：**44 条规则、200+ 个 check 块**，类型分布以 `structural_analysis`（106）与
`keyword_blocklist`（24）为主，另有大量 Mady 引擎不认识的类型（`witness`、`chain_of_custody`、
`overseas`、`proof_standard` 等）；action 分布为 warn 99 / log 51 / block 32 / apply 12。
Mady 规则引擎是另一套自定义 check 类型体系。

**修订**：T-W1-05 拆为两批——
- 批 A（block + 结构性规则，约 30 条）先落地并接入单调 deny；
- 批 B（warn/log 提示级，其余）批量转换，未识别类型一律**报错而非忽略**。
工作量 10 → **15 人日**。

### P1-3 法条索引只覆盖专利法族，勿过度承诺

实测：专利法转录 37 条（82 条范围内）、实施条例 45 条、审查指南 68 节点；
Mady `citation_table` 现有 33 条专利法主题词 + 商标法 25 + 著作权法 23 + 反不正当竞争法 9。
移植后**商标/著作权/反法仍无条文原文**。

**修订**：W1 明确范围为「专利法族」，报告与文档不得暗示其他法域已核验；
商标/著作权/反法补录列为 W5 可选任务。

### P1-4 `patent-teams` 缺 ADR 依据就估 15 人日

Mady 已有 `agentcore/worker` + `tasklist` + `provisions`，与 dsh `patent-teams`
（队长-成员、邮箱、依赖 DAG、调度器、锁）语义重叠但形态不同。方案已列 ADR，但仍给了确定估算。

**修订**：把 `T-W5-01 团队模型 ADR` 提升为 **spike（1–2 人日）**，ADR 产出后再估实施工作量；
W5 总计先按 49 人日保留，但标注「含 ±40% 不确定性」。

### P1-5 工作量估算缺方法说明

当前数字没有给出估算依据。**修订**：在方案 §4 末尾注明方法（按任务复杂度 +
同类模块历史实现速度类比），并标注置信区间 ±30%；W5 单独标 ±40%。

---

## 五、建议（P2）

1. **任务清单需补文件级 bite-sized 分解**。当前是任务级表格，符合排期评审，但不符合执行规范。
   建议每波开工前再产出一份该波的文件级分解（精确路径、单步可验证、含测试文件），
   避免过早细化全部波次。
2. **文档归类错误**：`document-deliver` 被放在 §4.1 patent 表内，应归入 §4.2 document 表。
3. **补充遗漏能力项**：`htmlScan`（HTML 扫描）、`paragraphNumbering`（段落编号）、
   `brandInjector`（品牌注入）在方案中只被笼统提及，应在 W2 任务中显式列出。
4. **检查清单增加环境前置项**：`python3`/`python-docx`/`dot`/`inkscape`/`freecad`/`soffice`/`pandoc`/`typst`，
   每个任务开工前跑一次 `scripts/check-domain-deps`。
5. **Mady 的 tone gate 是 CI 层脚本，不是运行时门禁**，W1-05 不要与之混淆；两者互补。

---

## 六、修订后的波次与估算

| 波次 | 主题 | 原估算 | 修订 | 说明 |
|---|---|---:|---:|---|
| W-1 | 前置 spike | — | 3 | FreeCAD 能力研究 2 + 团队模型 ADR 1 |
| W0 | 基座与资产落位 | 3 | 3 | 增加环境探测脚本 |
| W1 | 可核验性 | 31 | 36 | 门禁 10→15（分两批） |
| W2 | 交付闭环 | 32 | 35 | 模板 12→15（HTML 模板 + 段落编号）；Office→PDF 改双通道 |
| W3 | 法律域 | 11 | 11 | 证据模型改二维，不新增工作量 |
| W4 | 附图（F1/F2/F4） | 38–46 | 26 | F3 移出，改为可选研究后立项 |
| W5 | 协作与外围 | 49 | 49 ±40% | 团队模型待 ADR 后重估 |
| **合计** | | 164–172 | **163（不含 W6 与 FreeCAD 结构图）** | 含不确定性区间：143–183 |

> W4 的 26 人日 = F1 8 + F2 10 + F4 8；FreeCAD 结构图另行立项。

---

## 七、开工前置条件

在 W0 开工前完成：

- [ ] 方案按 P0-1 ～ P0-5 修订完毕（本轮评审后可直接改）
- [ ] `scripts/check-domain-deps` 落地，输出环境矩阵
- [ ] T-R-01（FreeCAD 能力研究）产出结论
- [ ] T-W5-01（团队模型 ADR）产出结论
- [ ] 确认 W1 范围为「专利法族」，商标/著作权/反法不在本期
- [ ] 用户批准本评审报告与修订后方案

---

## 八、评审意见汇总

| 编号 | 级别 | 问题 | 处置 |
|---|---|---|---|
| P0-1 | 阻断 | `verify_deliverable` 语义写错 | 拆为 Render / Compliance / VerifyDeliverable 三项 |
| P0-2 | 阻断 | 66 条与 15 类是正交维度 | 证据模型改二维 |
| P0-3 | 阻断 | FreeCAD 缺失且违背先研究先例 | F3 移出，改 spike |
| P0-4 | 阻断 | LibreOffice 缺失 | 改 pandoc+typst 双通道 |
| P0-5 | 阻断 | 门禁与交付工具循环依赖 | 门禁分两期接线 |
| P1-1 | 重要 | 模板模型不同 | 增 HTML 模板通道 + 段落编号，12→15 |
| P1-2 | 重要 | 规则包 44 条/200+ check 块 | 分两批，10→15 |
| P1-3 | 重要 | 法条只覆盖专利法族 | 明确范围，勿过度承诺 |
| P1-4 | 重要 | 团队模型缺 ADR | 提升为 spike |
| P1-5 | 重要 | 估算缺方法 | 补方法说明与置信区间 |
| P2-1～5 | 建议 | 任务粒度/归类/遗漏项/环境检查/tone gate 分层 | 按建议修订 |
