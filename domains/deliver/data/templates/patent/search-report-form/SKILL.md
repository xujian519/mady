---
name: patent-search-report-form
description: |
  表格式专利检索报告模板。将检索策略、对比文件清单与检索结论渲染为
  与国家知识产权局《表格 220701》检索记录部分对齐的表格式报告。
triggers:
  - "专利检索报告"
  - "检索报告"
  - "检索记录"
template:
  kind: patent-document
  mode: report
  scenario: patent-search-record
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 表格式专利检索报告模板

将专利检索过程与结果渲染为**表格式检索报告**：A–E 分区逐项对齐国家知识产权局《表格 220701（2011.4 版）》的检索记录部分，并附 E.1 相关度分布、检索式执行记录、F 检索范围与局限与声明。

## 与品牌风 `search-report` 的差别

- `search-report` 是品牌风的检索报告，套用 `tokens.css` 品牌变量。
- 本模板是**表格式检索报告**，复刻官方检索记录版式，使用黑白表格线、不套用品牌色；其 A–E 分区可直接并入 `right-evaluation-report`。
- 两者版式与用途不同，按交付场合选用；本模板不改动 `search-report`。

## 输入要求

渲染前必须已具备（缺任一项先补齐）：

1. 著录项：报告编号、检索日期、检索截止日、申请号/专利号、申请日、名称、专利类型、专利权人/申请人、委托方。
2. A–E 分区内容：主题分类、检索领域、数据库与检索式、相关文件清单、检索结论。
3. 每条相关文件的类型、文献号、公开日、分类号、相关部分、相关权利要求编号。
4. 检索式执行记录：轮次、数据库、检索式、命中数、执行日期。
5. 检索范围与局限；声明段落（模板已内置，固定内容，不可删除）。

## 工作流

1. 读 `references/conventions.md`、`references/checklist.md` 与 `references/citation-log.md`；槽位命名以 `references/slots.md` 为准。
2. 用 `render_patent_document` 渲染：template 传 `search-report-form`，draft 传表单草案——`fields` 填文本槽（日期按年/月/日三个槽位），`sections` 传 blocks 槽位（searchField/databases/conclusion，paragraph 每段一行、list 每项一行）与 rows 槽位（relatedDocuments 六列、searchRounds 五列，按条数给等宽字符串数组）；勾选类只有「相关文件，参见附页」（`moreDocuments: ["continued"]`）。槽位清单与必填项以草案校验报错为准。
3. 「F. 检索范围与局限」三条声明与页脚页码不由草案驱动：声明是否勾选、页码在定稿时人工处理。
4. 按 `references/checklist.md` 自查后定稿。

`assets/example-draft.json` 是完整示例草案，`example.html` 由它渲染生成，可作为结构参照。

## 输出契约

```
文件：search-report-form.html（单文件、内联 CSS）
可选：search-report-form.pdf（A4 打印）
```

本模板复刻官方检索记录版式，刻意使用黑白表格线，**不套用 `assets/templates/patent/tokens.css` 品牌色**；agent 不改样式。
