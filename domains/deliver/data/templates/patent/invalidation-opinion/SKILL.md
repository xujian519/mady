---
name: patent-invalidity-request
description: |
  专利无效宣告请求书模板（专利律师场景交付物）。将专利无效分析、对比文件、
  无效理由与逐权利要求结论渲染为对外提交的无效宣告请求书。
triggers:
  - "无效宣告"
  - "专利无效"
  - "无效请求书"
  - "invalidity request"
  - "patent invalidation"
template:
  kind: patent-document
  mode: request
  scenario: patent-invalidation
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 专利无效宣告请求书模板

将无效分析结论渲染为**对外提交的无效宣告请求书**：只写实际主张的理由、逐权利要求请求结论与证据清单。内部评估（引用日志、结论分档、缺口披露、风险与最坏情形）**不进本件**，用 `invalidation-analysis` 模板或案卷内部文件承载。

## 输入要求

渲染前必须已具备：

1. 涉案专利全文（权利要求书、说明书、授权公告文本）。
2. 涉案专利法律状态（授权公告号、授权日、专利权人、请求人）。
3. 证据清单（每份含标识与名称、公开或出版日、与涉案专利的时间关系；标识与登记口径见 `references/conventions.md` §4.1）。正文引用与清单登记共用同一编号，两侧由渲染器机械核对。
4. 涉案权利要求分解表（特征编号、技术特征、技术效果）。
5. **本次实际主张**的无效理由与逐项四段式正文（法条、理由名、被主张的权项号、事实认定、证据分析、结论）。
6. 免责声明与密级（模板已内置）。

## 工作流

1. 读 `references/conventions.md`（§2 法条体系、§2.1 从属权项路径、§3 四段式）与 `references/checklist.md`。
2. 确认实际主张的理由清单：理由识别走 `patent_workflow_run`（manifestId=`patent_invalidation_v1`）的 grounds 阶段，并按各条命中的原文片段人工核对；未被主张的理由不写入本件。
3. 用 `render_patent_document` 渲染：template 传 `invalidation-opinion`，draft 传 id 键控草案：`fields` 填著录项与单段文本槽（meta-*、position-summary、conclusion-text、footer-case、footer-date），`sections` 传 blocks 槽（doc-number、position-points）、rows 槽（claim-feature-table/evidence-table/claim-conclusion-table）与 `grounds` 槽（grounds-body：每项 { article, ground, claimNos, facts, evidence, conclusion }，节标题 `4.N 权利要求 …不具备…（法条）` 与四段式小标题由渲染器生成）。槽位清单与必填项以草案校验报错为准。
4. 按结构填充：案件信息 → 总体立场 → 权利要求特征分解 → 证据清单 → 无效理由（逐条四段式）→ 逐权利要求请求结论 → 结论与请求。
5. 对比文件公开日必须早于涉案专利优先权日/申请日（有优先权的以优先权日为准）；抵触申请单独标注。
6. 删除占位文案，按 `references/checklist.md` 自查后定稿。

## 输出契约

```
文件：invalidation-opinion.html（单文件、内联 CSS）
可选：invalidation-opinion.pdf（A4 打印）
     invalidation-opinion.md（源稿）
```

品牌变量由 `assets/templates/patent/tokens.css` 注入；agent 不修改样式。

本产出为**所内复核稿**；向国家知识产权局提交时，正文转入官方 `100014 附页`，请求书表格使用 `101001 专利权无效宣告请求书`，补充意见与答复转送文件通知书使用 `100902 复审、无效宣告程序意见陈述书` 加 `100014 附页`；附件栏（101001 ⑨栏／100902 ⑦栏）逐条写明名称、份数、页数、原件或复印件，份数与页数在本件不登记、按附件实物补填（详见 `references/conventions.md` §7）。
