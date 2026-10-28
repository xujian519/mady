---
name: oa-response-sati
title: 审查意见答复书（Sati 版）
category: patent-report
description: 针对审查意见通知书的答复书，逐条处理审查意见并论证修改或争辩
domain: patent
version: "1.0.0"
language: zh-CN
style: sati
formats:
  - markdown
  - html
vars:
  - name: firm_name
    type: string
    required: true
    description: 机构名称
  - name: doc_no
    type: string
    required: false
    description: 文档编号
  - name: application_no
    type: string
    required: true
    description: 申请号
  - name: applicant
    type: string
    required: true
    description: 申请人
  - name: oa_type
    type: string
    required: true
    description: 审查意见类型（新颖性/创造性/26.3/26.4 等）
  - name: examiner_opinion
    type: multiline
    required: true
    description: 审查员意见要点
  - name: amendment
    type: multiline
    required: true
    description: 修改方案（修改权利要求/说明书要点）
  - name: arguments
    type: multiline
    required: true
    description: 争辩理由
  - name: conclusion
    type: multiline
    required: true
    description: 答复结论
  - name: disclaimer
    type: string
    required: false
    default: "本答复书由 AI 辅助生成，不构成正式法律意见，最终应以代理人与申请人确认内容为准。"
    description: 免责声明
---

# 审查意见答复书

<div class="doc-meta">
**机构：** {{firm_name}}　**文档编号：** {{doc_no}}　**申请号：** {{application_no}}
</div>

| 申请人 | 申请号 | 审查意见类型 |
| --- | --- | --- |
| {{applicant}} | {{application_no}} | {{oa_type}} |

## 结论摘要

| 处理事项 | 结论 | 置信度 |
| --- | --- | --- |
| 修改认可度 | <span class="verdict-success">建议修改</span> | 高 |
| 争辩可行性 | <span class="verdict-warning">需补强</span> | 中 |

<div class="callout">
> 本答复书优先采用修改方式克服审查意见，争辩部分需结合技术启示进一步论证。
</div>

## 一、审查意见要点

{{examiner_opinion}}

## 二、修改方案

{{amendment}}

## 三、争辩理由

{{arguments}}

## 四、答复结论

{{conclusion}}

## 假设与局限

- 答复策略基于当前审查意见文本，实际审查进程（补充检索/会晤）可能影响走向。
- 修改后权利要求应满足 A33（修改超范围）限制。

---

> ⚠️ {{disclaimer}}
