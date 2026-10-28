---
name: standard-9-section
title: 标准九段式技术交底书
category: disclosure
description: 标准九段式技术交底书模板
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
  - name: tech_field
    type: string
    required: false
    description: "本发明所属技术领域"
  - name: background_problems
    type: multiline
    required: false
    description: "背景技术现状与其中存在的问题"
  - name: tech_problems
    type: multiline
    required: false
    description: "本发明要解决的技术问题"
  - name: tech_solution
    type: multiline
    required: false
    description: "完整技术方案，详至本领域技术人员可以实施"
  - name: benefits
    type: multiline
    required: false
    description: "有益效果"
  - name: figures
    type: multiline
    required: false
    default: "无"
    description: "附图清单与每幅附图的说明"
  - name: embodiments
    type: multiline
    required: false
    description: "具体实施方式"
  - name: industrial
    type: multiline
    required: false
    description: "工业实用性说明"
  - name: differences
    type: multiline
    required: false
    description: "与最接近现有技术的区别"
  - name: notes
    type: multiline
    required: false
    default: "无"
    description: "其他需要说明的事项"
---
# 技术交底书

## 1. 发明名称
{{invention_title}}

## 2. 技术领域
本发明属于{{tech_field}}技术领域。

## 3. 背景技术
{{background_problems}}

## 4. 发明内容
### 4.1 要解决的技术问题
{{tech_problems}}

### 4.2 技术方案
{{tech_solution}}

### 4.3 有益效果
{{benefits}}

## 5. 附图说明
{{figures}}

## 6. 具体实施方式
{{embodiments}}

## 7. 工业实用性
{{industrial}}

## 8. 与最接近现有技术的区别
{{differences}}

## 9. 其他说明
{{notes}}

---

> **填写指引：**
> - 第1-4节是最核心内容
> - 技术方案应详细到"本领域技术人员可以实施"
> - 区分"技术问题"和"有益效果"
