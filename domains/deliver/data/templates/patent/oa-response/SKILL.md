---
name: patent-oa-response
description: |
  审查意见答复 / 意见陈述书模板（专利律师场景交付物）。针对专利审查意见通知书，
  按「事实 → 规则 → 论证」三层组织论证，将修改后的权利要求、争辩意见、证据引用
  渲染为正式意见陈述书。
triggers:
  - "审查意见答复"
  - "OA 答复"
  - "意见陈述书"
  - "patent office action response"
template:
  kind: patent-document
  mode: response
  scenario: patent-office-action
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 审查意见答复 / 意见陈述书模板

将审查意见答复过程与结论渲染为**正式意见陈述书**：以「事实 → 规则 → 论证」三层推进，论证按争点组织而非按审查意见项号逐条铺陈，并强调修改对照清晰、法条与证据引用规范。

## 输入要求

渲染前必须已具备：

1. 审查意见通知书原文（通知书编号、发文日、审查结论、引用的对比文件）。
2. 原始权利要求书与修改后的权利要求书。
3. 权利要求修改对照表（修改前 / 修改后 / 修改依据 / 解决的技术问题）。
4. **事实抽取**：申请文件记载的事实（F1…）与对比文件公开的事实（P1…），每项带出处、不超过两句。
5. **规则抽取**：本案据以论证的法条条款号与《专利审查指南》节号（R1…），条号须已核验。
6. 针对各争点的论证（回指 F/P 与 R 编号）。
7. 形式缺陷清单：审查意见中不进入论证主线的笔误、附图标记、摘要撰写等，逐项写处理方式与依据。
8. 证据清单（D1/D2、公知常识证据等）、免责声明与密级（模板已内置）。

## 工作流

1. 读 `references/conventions.md` 与 `references/citation-log.md`；`conventions.md` §2 的三层骨架与 §3 的组织单元规则是硬要求。
2. 用 `render_patent_document` 渲染：template 传 `oa-response`，draft 传 id 键控草案。
   - `fields`（单行文本槽）：`meta-appno`、`meta-title`、`meta-oa-no`、`meta-oa-date`、`meta-response-date`、`meta-agent`、`position-summary`、`conclusion-text`、`footer-case`、`footer-date`。
   - `sections`（blocks 槽）：`doc-number`、`position-points`、`amended-claim-1`、`facts-application`、`facts-prior-art`、`rules-body`、`argument-novelty`、`argument-inventiveness`、`argument-other`（可选）、`formalities-body`（可选）。
   - `sections`（rows 槽）：`amendment-table`（4 列）、`evidence-table`（6 列）、`citation-table`（6 列）。
   - 槽位清单与必填项以草案校验报错为准。
3. 按结构填充：案件信息 → 总体立场 → 本次修改 → 本案事实 → 适用规则 → 意见陈述 → 其他缺陷的处理 → 证据与引用 → 结论与请求。
4. 事实部分只陈述、不评价；评价一律放进意见陈述部分，并回指 F/P 与 R 编号。
5. 论证按争点分节；审查意见项号只作定位括注。形式缺陷收入「其他缺陷的处理」逐项列明。
6. 修改后的权利要求用下划线或加粗标注修改处；删除线仅用于说明，不写入正式文件。
7. 删除占位文案，按 `references/checklist.md` 自查后定稿。

## 输出契约

```
文件：oa-response.html（单文件、内联 CSS）
可选：oa-response.pdf（A4 打印）
     oa-response.md（源稿）
```

本产出为**所内复核稿**；向国家知识产权局提交时，正文转入官方 `100014 附页`，表格使用 `100012 意见陈述书`（详见 `references/conventions.md` §8）。

品牌变量由 `assets/templates/patent/tokens.css` 注入；agent 不修改样式。
