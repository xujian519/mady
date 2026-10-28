---
name: report
title: 分析报告
category: document
description: 中文多章节分析报告模板：结论摘要、方法、发现、风险与建议
domain: document
version: "1.0"
language: zh-CN
use_when: 需要交付调研报告、分析报告、方案评估等多章节文档时
formats:
  - markdown
  - html
  - docx
vars:
  - name: report_title
    type: string
    required: false
    description: "报告标题"
  - name: report_type
    type: string
    required: false
    description: "报告类型（调研/分析/评估/复盘等）"
  - name: author
    type: string
    required: false
    description: "作者或负责人"
  - name: date
    type: string
    required: false
    description: "报告日期"
  - name: summary
    type: multiline
    required: false
    description: "结论摘要，先给结论再给依据"
  - name: background
    type: multiline
    required: false
    description: "背景、问题与本次分析的范围"
  - name: method
    type: multiline
    required: false
    description: "分析方法、数据来源与口径"
  - name: findings
    type: multiline
    required: false
    description: "主要发现，逐条给出证据"
  - name: data
    type: multiline
    required: false
    description: "关键数据与图表说明"
  - name: risks
    type: multiline
    required: false
    description: "风险、局限与未证实项"
  - name: conclusion
    type: multiline
    required: false
    description: "结论"
  - name: recommendations
    type: multiline
    required: false
    description: "建议动作与前置条件"
  - name: appendix
    type: multiline
    required: false
    default: "无"
    description: "附录（术语、原始数据、参考资料）"
---
# {{report_title}}

**类型：** {{report_type}}　**作者：** {{author}}　**日期：** {{date}}

## 结论摘要

{{summary}}

## 一、背景与范围

{{background}}

## 二、方法与数据来源

{{method}}

## 三、主要发现

{{findings}}

## 四、关键数据

{{data}}

## 五、风险与局限

{{risks}}

## 六、结论

{{conclusion}}

## 七、建议

{{recommendations}}

## 附录

{{appendix}}

---

> **使用说明：** 每条发现都要能追溯到证据；无法核验的内容写进"风险与局限"，不要写成结论。
