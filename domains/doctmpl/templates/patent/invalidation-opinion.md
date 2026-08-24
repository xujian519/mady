---
name: invalidation-opinion
title: 专利无效宣告意见书
category: patent-report
description: 针对某一专利的无效宣告分析意见书，评估无效理由（新颖性/创造性/26.3等）与可行性
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
  - name: patent_no
    type: string
    required: true
    description: 被无效专利号
  - name: patentee
    type: string
    required: true
    description: 专利权人
  - name: invalidator
    type: string
    required: true
    description: 请求人
  - name: grounds
    type: multiline
    required: true
    description: 无效理由与依据
  - name: evidence
    type: multiline
    required: true
    description: 证据列表
  - name: feature_comparison
    type: multiline
    required: true
    description: 技术特征比对
  - name: feasibility
    type: multiline
    required: true
    description: 无效可行性评估
  - name: conclusion
    type: multiline
    required: true
    description: 结论与建议
  - name: disclaimer
    type: string
    required: false
    default: "本分析由 AI 辅助生成，不构成正式法律意见，无效可行性判断仅供参考。"
    description: 免责声明
---

# 专利无效宣告意见书

<div class="doc-meta">
**机构：** {{firm_name}}　**文档编号：** {{doc_no}}
</div>

| 被无效专利号 | 专利权人 | 请求人 |
| --- | --- | --- |
| {{patent_no}} | {{patentee}} | {{invalidator}} |

## 结论摘要

| 无效理由 | 初步评估 | 置信度 |
| --- | --- | --- |
| 新颖性（A22.2） | <span class="verdict-warning">尚需补强</span> | 中 |
| 创造性（A22.3） | <span class="verdict-success">有较强把握</span> | 高 |
| 26.3 充分公开 | <span class="verdict-danger">证据不足</span> | 低 |

<div class="callout danger">
> 综合判断：创造性理由具备较强无效可行性，建议以此为突破点并补充证据。
</div>

## 一、无效理由与依据

{{grounds}}

## 二、证据材料

{{evidence}}

## 三、技术特征比对

{{feature_comparison}}

## 四、无效可行性评估

{{feasibility}}

## 五、结论与建议

{{conclusion}}

## 假设与局限

- 无效可行性基于现有证据与检索结果，未经口审/复审委认定。
- 证据证明力与创造性显而易见性判断需专业代理人进一步核实。

---

> ⚠️ {{disclaimer}}
