---
name: simplified-disclosure
title: 简化版技术交底书
category: disclosure
description: 简化版技术交底书模板
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: invention_title
    type: string
    required: false
    description: "发明名称"
  - name: existing_problems
    type: multiline
    required: false
    description: "现有技术存在的问题"
  - name: tech_solution
    type: multiline
    required: false
    description: "本发明解决上述问题的技术方案"
  - name: innovation_1
    type: multiline
    required: false
    description: "关键创新点1"
  - name: innovation_2
    type: multiline
    required: false
    description: "关键创新点2"
  - name: innovation_3
    type: multiline
    required: false
    description: "关键创新点3"
  - name: tech_effects
    type: multiline
    required: false
    description: "本发明相对现有技术的技术效果"
  - name: figures
    type: multiline
    required: false
    default: "无"
    description: "附图清单与每幅附图的内容说明"
  - name: notes
    type: multiline
    required: false
    default: "无"
    description: "补充说明、已知缺陷或需要发明人确认的事项"
---
# 技术交底书（简化版）

## 发明名称
{{invention_title}}

## 现有技术存在的问题
{{existing_problems}}

## 本发明的技术方案
{{tech_solution}}

## 本发明的关键创新点
1. {{innovation_1}}
2. {{innovation_2}}
3. {{innovation_3}}

## 技术效果
{{tech_effects}}

## 附图
{{figures}}

## 备注
{{notes}}

---

> **使用说明：**
> - 适用于研发人员快速记录发明构思
> - 简化版主要关注"解决了什么问题"和"怎么解决的"
