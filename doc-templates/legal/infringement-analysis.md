---
name: infringement-analysis
title: 侵权分析报告
category: legal
description: 专利/商标/著作权侵权判定分析报告模板
domain: legal
version: "1.0.0"
language: zh-CN
style: legal-standard
formats:
  - markdown
  - docx
vars:
  - name: right_type
    type: string
    required: true
    description: 权利类型（专利权/商标权/著作权）
  - name: right_holder
    type: string
    required: true
    description: 权利人
  - name: right_scope
    type: multiline
    required: true
    description: 权利范围（专利权利要求/商标核定商品/作品内容）
  - name: alleged_infringement
    type: multiline
    required: true
    description: 涉嫌侵权行为描述
  - name: comparison
    type: multiline
    required: true
    description: 特征/要素逐一比对
  - name: defense_analysis
    type: multiline
    required: false
    description: 可能的抗辩事由分析
  - name: conclusion
    type: string
    required: true
    description: 侵权判定结论
  - name: disclaimer
    type: string
    required: false
    default: "本分析由 AI 辅助生成，不构成正式法律意见。侵权判定应由具备执业资格的律师或专利代理人确认。"
    description: 免责声明
---
# 侵权分析报告

## 一、权利基础

权利类型：{{right_type}}
权利人：{{right_holder}}

### 权利范围
{{right_scope}}

## 二、涉嫌侵权行为

{{alleged_infringement}}

## 三、侵权比对分析

{{comparison}}

## 四、抗辩事由分析

{{defense_analysis}}

## 五、结论

**侵权判定结论：{{conclusion}}**

---

> ⚠️ {{disclaimer}}
