---
name: patent-invalidation-defense
description: |
  无效宣告程序意见陈述书模板（专利律师场景交付物）。将专利权人（被请求人）对无效宣告请求的
  答辩——程序性意见、证据质证、权利要求书修改与逐项答辩——渲染为对外提交的意见陈述书。
triggers:
  - "无效答辩"
  - "无效宣告答复"
  - "意见陈述书"
  - "被请求人答辩"
  - "invalidation defense"
  - "patent invalidation response"
template:
  kind: patent-document
  mode: response
  scenario: patent-invalidation-defense
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 无效宣告程序意见陈述书模板

将专利权人（被请求人）的答辩意见渲染为**对外提交的意见陈述书**：程序性意见、对请求人证据的质证意见、权利要求书的修改、逐项答辩与逐权利要求答辩结论。请求人侧的无效宣告请求书用 `invalidation-opinion`；内部评估（引用日志、结论分档、风险与应对预案）**不进本件**，用 `invalidation-analysis` 模板或案卷内部文件承载。

## 输入要求

渲染前必须已具备：

1. 请求人提交的无效宣告请求书与转送文件通知书（转送日期、发文序号、指定的答复期限届满日）。
2. 涉案专利授权公告文本（权利要求书、说明书、附图）；主张修改时的原始申请文件，作为修改是否超范围的核对基线。
3. 请求人实际主张的**每一项**理由及其法条、被主张的权利要求号、所依据的证据编号。
4. 请求人提交的每份证据的形态（原件或复印件、外文证据及其译文、域外证据的证明手续、互联网证据的页面与发布时间）。
5. 专利权人提交的反证清单（每份含标识与名称、公开或出版日、与涉案专利的时间关系；登记口径见 `conventions.md` §6）。正文引用与清单登记共用同一编号，两侧由渲染器机械核对。
6. 本次答复是否修改权利要求书；修改时的修改方式、修改前与修改后的要点及其出处。
7. 免责声明与密级（模板已内置）。

## 工作流

1. 读 `references/conventions.md`（§2 程序性意见依据、§3 修改规则、§4 逐项答辩、§5 质证、§6 证据登记）与 `references/checklist.md`。
2. 从请求书摘出实际主张的理由清单与每项理由下的权利要求号；逐项与请求书原文核对，**不得增加请求书未主张的理由，也不得遗漏已主张的理由**。
3. 逐份证据走质证：真实性、合法性、关联性三项分别给出结论与依据（`conventions.md` §5）。
4. 权利要求书的修改先过 §3 的两道关：修改方式是否在允许的范围内（权利要求的删除、技术方案的删除、权利要求的进一步限定、明显错误的修正），以及本答复期限是否属于可以作删除以外修改的三种情形之一。修改后的权利要求全文随附页提交。
5. 用 `render_patent_document` 渲染：template 传 `invalidation-defense`，draft 传 id 键控草案。
   - `fields`：`meta-patent-no`、`meta-patent-title`、`meta-grant-date`、`meta-patentee`、`meta-requester`、`meta-request-date`、`meta-transfer-no`、`meta-deadline`、`meta-date`、`position-summary`、`conclusion-text`、`footer-case`、`footer-date`。
   - `sections`（blocks 槽）：`doc-number`、`position-points`、`procedural-body`、`evidence-objection-body`、`amendment-basis`。
   - `sections`（rows 槽）：`amendment-table`（5 列）、`evidence-table`（6 列）、`claim-conclusion-table`（4 列）。
   - `sections`（grounds 槽）：`defense-body`。每个理由只提交法条、理由名、被主张的权项号与三段正文；节标题（`5.1 权利要求 1、3 的创造性（…）`）与四段式小标题由渲染器按结构生成，**不得手写**——节序与关系词「的」由模板体例给出。理由名写成名词短语（「创造性」「新颖性」「说明书支持与清楚」）。
   - 槽位清单与必填项以草案校验报错为准。
6. 按结构填充：案件信息 → 答辩意见概述 → 程序性意见 → 对请求人证据的质证意见 → 权利要求书的修改 → 逐项答辩 → 证据清单 → 逐权利要求结论 → 结论与请求。
7. 删除占位文案，按 `references/checklist.md` 自查后定稿。

## 输出契约

```
文件：invalidation-defense.html（单文件、内联 CSS）
可选：invalidation-defense.pdf（A4 打印）
     invalidation-defense.md（源稿）
```

品牌变量由 `assets/templates/patent/tokens.css` 注入；agent 不修改样式。

本产出为**所内复核稿**；向国家知识产权局提交时，载体系官方 `100902 复审、无效宣告程序意见陈述书`：陈述意见写 ⑥栏、附件列 ⑦栏，⑥栏填写不下时用 `100014 附页` 续写，意见陈述书及其附件一式两份（详见 `references/conventions.md` §10）。
