---
name: case-analysis
title: 案例分析报告
category: legal
description: 判例/案例比对分析报告模板，适用于类案检索与判决预测
domain: legal
version: "1.0.0"
language: zh-CN
style: legal-standard
formats:
  - markdown
  - docx
vars:
  - name: case_name
    type: string
    required: true
    description: 本案名称/编号
  - name: case_type
    type: string
    required: true
    description: 案件类型
  - name: key_issues
    type: multiline
    required: true
    description: 争议焦点
  - name: comparable_cases
    type: multiline
    required: true
    description: 类案列表（案号+裁判要点）
  - name: comparison_analysis
    type: multiline
    required: true
    description: 比对分析（相同点/不同点/裁判倾向）
  - name: prediction
    type: multiline
    required: true
    description: 判决预测与建议
  - name: disclaimer
    type: string
    required: false
    default: "以上判例分析仅供参考，个案情况存在差异，具体法律意见请咨询执业律师。"
    description: 免责声明
---
# 案例分析报告

## 一、本案基本情况

案件名称/编号：{{case_name}}
案件类型：{{case_type}}

### 争议焦点
{{key_issues}}

## 二、类案检索

{{comparable_cases}}

## 三、比对分析

{{comparison_analysis}}

## 四、判决预测与建议

{{prediction}}

---

> ⚠️ {{disclaimer}}
