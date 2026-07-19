---
name: statute-interpretation
title: 法条适用分析
category: legal
description: 法律法规条款解释与适用性分析模板
domain: legal
version: "1.0.0"
language: zh-CN
style: legal-standard
formats:
  - markdown
vars:
  - name: statute
    type: string
    required: true
    description: 法律法规名称及条款
  - name: question
    type: multiline
    required: true
    description: 待分析的法律问题
  - name: interpretation
    type: multiline
    required: true
    description: 法条解释（文义/体系/目的/历史解释）
  - name: applicability
    type: multiline
    required: true
    description: 适用性分析（本案是否适用及理由）
  - name: related_cases
    type: multiline
    required: false
    description: 相关判例/司法解释
  - name: conclusion
    type: multiline
    required: true
    description: 适用结论
  - name: disclaimer
    type: string
    required: false
    default: "本分析由 AI 辅助生成，不构成正式法律意见。法条解释与适用应由具备执业资格的律师确认。"
    description: 免责声明
---
# 法条适用分析

## 一、法条原文

{{statute}}

## 二、待分析问题

{{question}}

## 三、法条解释

{{interpretation}}

## 四、适用性分析

{{applicability}}

## 五、相关判例与解释

{{related_cases}}

## 六、结论

{{conclusion}}

---

> ⚠️ {{disclaimer}}
