---
name: contract-review
title: 合同审查报告
category: legal
description: 合同条款逐条审查报告模板，适用于合同风险分析与修改建议
domain: legal
version: "1.0.0"
language: zh-CN
style: legal-standard
formats:
  - markdown
  - docx
vars:
  - name: contract_name
    type: string
    required: true
    description: 合同名称
  - name: parties
    type: string
    required: true
    description: 合同各方
  - name: review_scope
    type: string
    required: true
    description: 审查范围说明
  - name: key_findings
    type: multiline
    required: true
    description: 关键发现（风险条款列表）
  - name: clause_analysis
    type: multiline
    required: true
    description: 逐条分析（条款原文→风险→建议）
  - name: overall_risk
    type: string
    required: true
    description: 总体风险评估（低/中/高）
  - name: recommendations
    type: multiline
    required: true
    description: 修改建议汇总
  - name: disclaimer
    type: string
    required: false
    default: "本审查由 AI 辅助完成，不替代专业律师的合同审查。签署前请交由律师最终确认。"
    description: 免责声明
---
# 合同审查报告

## 一、合同基本信息

合同名称：{{contract_name}}
合同各方：{{parties}}
审查范围：{{review_scope}}

## 二、审查摘要

### 关键发现
{{key_findings}}

### 总体风险评估
**风险等级：{{overall_risk}}**

## 三、条款逐条分析

{{clause_analysis}}

## 四、修改建议

{{recommendations}}

---

> ⚠️ {{disclaimer}}

---

> **填写指引：**
> - `key_findings`：列出最关键的2-5个风险点
> - `clause_analysis`：每条按「原条款→风险分析→修改建议」三段式
> - `overall_risk`：综合评估为「低风险」「中等风险」或「高风险」
