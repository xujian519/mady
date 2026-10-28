---
name: patent-infringement-opinion
description: |
  侵权比对意见书模板（专利律师场景交付物）。权利要求解释、被控物特征分解、全面覆盖逐特征比对、
  等同分析（手段-功能-效果三要素）与结论风险，用于专利侵权分析报告或诉讼主张佐证。
triggers:
  - "侵权比对"
  - "全面覆盖"
  - "等同侵权"
  - "infringement opinion"
template:
  kind: patent-document
  mode: opinion
  scenario: patent-infringement
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 侵权比对意见书模板

将权利要求解释、被控产品特征分解与逐特征比对结果渲染为**正式侵权比对意见书**：全面覆盖比对表、等同分析表、结论与风险，证据来源逐项可追溯。

## 输入要求

渲染前必须已具备：

1. 涉案专利授权文本（权利要求书、说明书、附图、批准文本）。
2. 被控产品/方法的技术资料（说明书、宣传材料、拆解图、来源链接）。
3. 权利要求解释结论（含禁反言、捐献原则等限制）。
4. 逐特征比对记录（特征号 → 权利要求文字 → 被控特征描述 → 结论：相同/等同/未落入）。
5. 等同分析的三要素判断（手段/功能/效果 + 显而易见性）。

## 工作流

1. 读 `references/conventions.md`。
2. 用 `render_patent_document` 渲染：template 传 `infringement-opinion`，draft 传 id 键控草案：`fields` 填著录项与结论文本槽（meta-*、claim-text、accused-summary、coverage-conclusion、conclusion-text、footer-case、footer-date），`sections` 传 blocks 槽（doc-number、interpretation-notes、equivalence-limits、risk-items、evidence-log）与 rows 槽（accused-table-body/comparison-table-body/equivalence-table-body）。槽位清单与必填项以草案校验报错为准。
3. 按结构填充：案件信息 → 权利要求解释 → 被控物特征分解 → 全面覆盖比对表 → 等同分析表 → 结论与风险 → 落款。
4. 每个特征结论注明来源类型徽标（证据/客户/模型推断/假设）。
5. 渲染后核对：特征编号连续、结论与表格一致、来源路径完整、无占位符。

## 输出契约（必含字段）

- 交付场景（侵权比对）
- 矫正清单（特征编号/术语/法条引用/数值核对结果）
- 渲染产物（html/pdf 路径）
