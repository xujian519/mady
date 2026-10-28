---
name: patent-rectification-response
description: |
  补正书模板（专利律师场景交付物）。对补正通知书的答复：逐项列出通知缺陷、修改前后对照、
  替换页清单与未超范围声明，用于形式缺陷补正或主动补正。
triggers:
  - "补正书"
  - "补正通知书"
  - "主动补正"
  - "rectification response"
template:
  kind: patent-document
  mode: opinion
  scenario: patent-rectification
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 补正书模板

将补正通知书解析结果与修改内容渲染为**正式补正书**：逐项对应通知缺陷，附修改前后对照与替换页清单，并声明修改未超出原申请文件记载的范围。

## 输入要求

渲染前必须已具备：

1. 补正通知书全文（缺陷项、引用页码/段落）。
2. 申请号、发明名称、申请人等著录项目（用于元数据表）。
3. 修改内容逐项列表：缺陷项 → 修改前 → 修改后。
4. 替换页清单：页号、版本号、对应修改内容。
5. 补正期限（通知书落款日 + 指定期限，用工具计算，禁止心算）。

## 工作流

1. 读 `references/conventions.md`。
2. 用 `render_patent_document` 渲染：template 传 `rectification-response`，draft 传 id 键控草案：`fields` 填著录项与落款文本槽（meta-*、rect-findings、replacement-note、rect-statement、footer-case、footer-date），`sections` 传 blocks 槽（doc-number、rect-defects）与 rows 槽（rect-table-body/replacement-table-body）。槽位清单与必填项以草案校验报错为准。
3. 按结构填充：申请信息 → 补正通知要点 → 补正内容对照表 → 替换页清单 → 未超范围声明 → 落款/页脚。
4. 补正内容表逐缺陷项一行；通知未涉及但主动补正的项单独标注「主动补正」。
5. 渲染后核对：无占位符、页码引用与替换页一致、期限数值正确。

## 输出契约（必含字段）

- 交付场景（补正）
- 矫正清单（术语/法条引用/编号/日期核对结果）
- 渲染产物（html/pdf 路径）
