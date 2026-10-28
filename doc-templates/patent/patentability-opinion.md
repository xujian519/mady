---
name: patentability-opinion
title: 可专利性分析意见书
category: patent-report
description: 发明可专利性分析意见书，评估新颖性/创造性/实用性，输出授权前景与规避建议
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
    description: 机构/代理所名称（抬头）
  - name: doc_no
    type: string
    required: false
    description: 文档编号
  - name: case_no
    type: string
    required: false
    description: 申请号/案号
  - name: applicant
    type: string
    required: true
    description: 申请人
  - name: invention_title
    type: string
    required: true
    description: 发明名称
  - name: technical_field
    type: multiline
    required: false
    description: 技术领域
  - name: prior_art
    type: multiline
    required: true
    description: 检索到的现有技术
  - name: novelty_analysis
    type: multiline
    required: true
    description: 新颖性分析
  - name: inventiveness_analysis
    type: multiline
    required: true
    description: 创造性分析
  - name: conclusion
    type: multiline
    required: true
    description: 结论与建议
  - name: disclaimer
    type: string
    required: false
    default: "本分析由 AI 辅助生成，不构成正式法律意见，授权前景判断仅供参考。"
    description: 免责声明
---

# 可专利性分析意见书

<div class="doc-meta">
**机构：** {{firm_name}}　**文号：** {{doc_no}}　**申请号：** {{case_no}}
</div>

| 委托人 | 申请号 | 发明名称 |
| --- | --- | --- |
| {{applicant}} | {{case_no}} | {{invention_title}} |

## 结论摘要

| 审查维度 | 初步结论 | 置信度 |
| --- | --- | --- |
| 新颖性（A22.2） | <span class="verdict-success">具备</span> | 高 |
| 创造性（A22.3） | <span class="verdict-warning">存疑</span> | 中 |
| 实用性 | <span class="verdict-success">具备</span> | 高 |

<div class="callout warning">
> 综合判断：该技术方案具有授权可能，但创造性论证需结合技术启示进一步强化。
</div>

## 一、技术领域与背景

{{technical_field}}

## 二、现有技术检索

{{prior_art}}

## 三、新颖性分析

{{novelty_analysis}}

## 四、创造性分析

{{inventiveness_analysis}}

## 五、结论与建议

{{conclusion}}

## 假设与局限

- 检索范围限于本地知识库与公开数据库，未覆盖所有在先技术。
- 本分析基于当前权利要求文本，修改后结论可能变化。

---

> ⚠️ {{disclaimer}}
