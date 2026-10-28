---
name: electrical-spec
title: 电学领域说明书
category: specification
description: 电学/电子领域专利申请说明书模板
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
  - name: industry_trend
    type: string
    required: false
    description: "相关行业的发展趋势"
  - name: background_context
    type: multiline
    required: false
    description: "该趋势下的技术背景与由此产生的问题"
  - name: prior_art
    type: multiline
    required: false
    description: "现有技术通常采用的方案"
  - name: shortcoming_1
    type: multiline
    required: false
    description: "现有技术的不足1"
  - name: shortcoming_2
    type: multiline
    required: false
    description: "现有技术的不足2"
  - name: core_problem
    type: multiline
    required: false
    description: "本发明要解决的核心技术问题"
  - name: module_1
    type: string
    required: false
    description: "第一电路模块名称"
  - name: func_1
    type: string
    required: false
    description: "第一电路模块的功能"
  - name: module_2
    type: string
    required: false
    description: "第二电路模块名称"
  - name: func_2
    type: string
    required: false
    description: "第二电路模块的功能"
  - name: ctrl_func
    type: multiline
    required: false
    description: "控制模块的功能"
  - name: effect_1
    type: multiline
    required: false
    description: "有益效果1"
  - name: power_effect
    type: multiline
    required: false
    description: "功耗方面的改进效果"
  - name: embodiment_detail
    type: multiline
    required: false
    description: "具体实施例的电路结构与工作过程"
---
# 说明书

## 技术领域
本发明涉及{{tech_field}}技术领域，尤其涉及一种{{invention_name}}。

## 背景技术
随着{{industry_trend}}的发展，{{background_context}}。

现有技术方案通常采用{{prior_art}}，存在以下不足：
1. {{shortcoming_1}}
2. {{shortcoming_2}}

## 发明内容
### 要解决的技术问题
本发明提供一种{{invention_name}}，以解决{{core_problem}}。

### 技术方案
一种{{invention_name}}，包括：
- {{module_1}}，用于{{func_1}}；
- {{module_2}}，与{{module_1}}电连接，用于{{func_2}}；
- 控制模块，用于{{ctrl_func}}。

### 有益效果
1. {{effect_1}}
2. 功耗方面：{{power_effect}}

## 附图说明
图1为本发明的电路框图。

## 具体实施方式
### 实施例
{{embodiment_detail}}

以上为示例性说明。

---

> **填写指引：**
> - `module_N`：电路模块名称
> - `power_effect`：功耗方面的改进效果
