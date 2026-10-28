---
name: mechanical-spec
title: 机械领域说明书
category: specification
description: 机械领域专利申请说明书模板
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: tech_field
    type: string
    required: false
    description: "本发明所属技术领域"
  - name: invention_name
    type: string
    required: false
    description: "发明名称"
  - name: background_desc
    type: multiline
    required: false
    description: "背景技术现状"
  - name: problem_1
    type: multiline
    required: false
    description: "现有技术存在的问题1"
  - name: problem_2
    type: multiline
    required: false
    description: "现有技术存在的问题2"
  - name: core_problem
    type: multiline
    required: false
    description: "本发明要解决的核心技术问题"
  - name: tech_solution
    type: multiline
    required: false
    description: "完整技术方案，含结构组成与连接关系"
  - name: effect_1
    type: multiline
    required: false
    description: "有益效果1"
  - name: effect_2
    type: multiline
    required: false
    description: "有益效果2"
  - name: fig_2_desc
    type: string
    required: false
    description: "图2的内容说明"
  - name: embodiment_detail
    type: multiline
    required: false
    description: "实施例的结构、连接关系与工作原理"
---
# 说明书

## 技术领域
本发明涉及{{tech_field}}技术领域，具体涉及一种{{invention_name}}。

## 背景技术
{{background_desc}}

然而，现有技术存在以下问题：
1. {{problem_1}}
2. {{problem_2}}

## 发明内容
### 要解决的技术问题
本发明要解决的技术问题是{{core_problem}}。

### 技术方案
一种{{invention_name}}，包括：
{{tech_solution}}

### 有益效果
与现有技术相比，本发明具有以下有益效果：
1. {{effect_1}}
2. {{effect_2}}

## 附图说明
图1为本发明实施例的结构示意图。
图2为{{fig_2_desc}}。

## 具体实施方式
下面结合附图对本发明的具体实施方式进行详细说明。

### 实施例1
如图1所示，本实施例提供一种{{invention_name}}，包括：
{{embodiment_detail}}

以上所述仅为本发明的优选实施例。

---

> **填写指引：**
> - `tech_field`：所属技术领域
> - `background_desc`：现有技术现状描述
> - `tech_solution`：完整技术方案描述
