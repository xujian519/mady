---
name: patent-litigation-pleading
description: |
  专利侵权诉讼文书模板（专利律师场景交付物）。民事起诉状 / 民事答辩状通用骨架：当事人信息、
  诉讼请求（或答辩意见）、事实与理由、证据清单与落款，立场中立可双向使用。
triggers:
  - "民事起诉状"
  - "民事答辩状"
  - "诉状"
  - "答辩状"
  - "patent litigation pleading"
template:
  kind: patent-document
  mode: opinion
  scenario: patent-litigation
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 专利侵权诉讼文书模板

将当事人信息、案件事实与主张渲染为**正式民事起诉状或民事答辩状**。立场由标题与「文书类型」决定（起诉状 = 原告方；答辩状 = 被告方），模板骨架中立，内容按立场填充。

## 输入要求

渲染前必须已具备：

1. 当事人信息（原/被告名称、住所、法定代表人、联系人）。
2. 管辖法院、案号（答辩状必有；起诉状可留待立案）。
3. 诉讼请求（起诉状）/ 答辩意见（答辩状）：赔额、停止侵权、合理开支等，数值与计算依据。
4. 事实与理由：涉案专利、被控行为、证据清单（三性说明）。
5. 期限信息（答辩期 = 收到起诉状副本 + 15 日，用工具计算，禁止心算）。

## 工作流

1. 读 `references/conventions.md`。
2. 用 `render_patent_document` 渲染：template 传 `litigation-pleading`，draft 传 id 键控草案：`fields` 填著录项、当事人、请求与事实文本槽（meta-*、doc-type/doc-kind、party-*、relief-*、facts-*、footer-case、footer-date），`sections` 传 blocks 槽（doc-number、defense-items、facts-rebuttal、sign-party、sign-date）与 rows 槽（evidence-table-body）。签名与日期槽位非必填（人工签署）。槽位清单与必填项以草案校验报错为准。
3. 确定文书类型（起诉状/答辩状）→ 填充当事人信息 → 诉讼请求/答辩意见 → 事实与理由 → 证据清单 → 落款。
4. 答辩状需逐项回应起诉状主张（对事实、对证据、对法律适用），不得遗漏。
5. 渲染后核对：金额/日期数值、案由表述、证据编号连续、无占位符。

## 输出契约（必含字段）

- 交付场景（诉讼文书）
- 矫正清单（称谓/金额/日期/证据编号核对结果）
- 渲染产物（html/pdf 路径）
