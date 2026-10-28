---
name: patent-re-examination-request
description: |
  复审请求书模板（专利律师场景交付物）。针对驳回决定：解析驳回理由、提出复审请求与修改方案、
  附法律依据与请求事项，用于驳回后复审（3 个月期限）。
triggers:
  - "复审请求书"
  - "复审"
  - "驳回决定"
  - "re-examination request"
template:
  kind: patent-document
  mode: opinion
  scenario: patent-re-examination
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 复审请求书模板

将驳回决定解析、修改方案与争辩意见渲染为**正式复审请求书**：请求人信息、复审理由（逐项针对驳回理由）、修改对照、法律依据与请求事项。

## 输入要求

渲染前必须已具备：

1. 驳回决定全文（驳回理由、引用的对比文件、权利要求版本）。
2. 申请号、发明名称、申请人（请求人）等著录项目。
3. 修改后的权利要求文本（与复审理由对应，注明修改依据）。
4. 复审期限（驳回决定送达日 + 3 个月，用工具计算，禁止心算；说明恢复窗口）。

## 工作流

1. 读 `references/conventions.md`。
2. 用 `render_patent_document` 渲染：template 传 `re-examination-request`，draft 传 id 键控草案：`fields` 填著录项、理由与落款文本槽（meta-*、rejection-summary、ground-1/ground-2、procedure-note、request-date、footer-case、footer-date），`sections` 传 blocks 槽（doc-number、ground-citations、legal-basis）与 rows 槽（amendment-table-body）。槽位清单与必填项以草案校验报错为准。
3. 按结构填充：请求人信息 → 请求复审的理由（逐条：驳回理由 → 请求人观点 → 依据）→ 修改对照表 → 法律依据 → 请求事项 → 落款。
4. 复审理由必须针对驳回理由逐项回应；修改方案须与理由一一对应并给出修改依据（法条/原申请文件）。
5. 渲染后核对：期限数值、法条引用、对比文件公开号、无占位符。

## 输出契约（必含字段）

- 交付场景（复审）
- 矫正清单（术语/法条引用/编号/日期核对结果）
- 渲染产物（html/pdf 路径）
