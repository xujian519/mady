---
name: legal-opinion
title: 法律意见书
category: legal
description: 综合性法律分析意见书模板，适用于法条适用分析、案件评估等场景
domain: legal
version: "1.0.0"
language: zh-CN
style: legal-standard
formats:
  - markdown
  - docx
  - pdf
vars:
  - name: client_name
    type: string
    required: true
    description: 委托人名称
  - name: case_subject
    type: string
    required: true
    description: 案由
  - name: case_summary
    type: multiline
    required: true
    description: 案情摘要
  - name: legal_basis
    type: multiline
    required: true
    description: 适用法律法规及条款
  - name: legal_analysis
    type: multiline
    required: true
    description: 法律分析（三段论推理）
  - name: conclusion
    type: multiline
    required: true
    description: 结论与建议
  - name: disclaimer
    type: string
    required: false
    default: "本分析由 AI 辅助生成，不构成正式法律意见。法律判断和决策应由具备执业资格的律师确认。"
    description: 免责声明
---
# 法律意见书

## 一、案件概述

委托人：{{client_name}}
案由：{{case_subject}}

### 案件事实
{{case_summary}}

## 二、法律依据

{{legal_basis}}

## 三、法律分析

{{legal_analysis}}

## 四、结论与建议

{{conclusion}}

---

> ⚠️ {{disclaimer}}

---

> **填写指引：**
> - `legal_basis`：列出所有适用法条，注明发布机关、生效日期、具体条款
> - `legal_analysis`：采用三段论（大前提→小前提→结论）组织分析
> - `conclusion`：明确区分确定性法律结论和参考性分析
