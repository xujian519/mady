---
name: software-spec
title: 软件领域说明书
category: specification
description: 软件/算法领域专利申请说明书模板
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
  - name: app_scenario
    type: string
    required: false
    description: "本发明的应用场景"
  - name: problem_1
    type: multiline
    required: false
    description: "现有技术存在的问题1"
  - name: problem_2
    type: multiline
    required: false
    description: "现有技术存在的问题2"
  - name: input_data
    type: string
    required: false
    description: "方法步骤1获取的输入数据"
  - name: algo_core
    type: multiline
    required: false
    description: "处理输入数据的核心算法逻辑"
  - name: intermediate
    type: string
    required: false
    description: "处理后得到的中间结果"
  - name: decision_logic
    type: multiline
    required: false
    description: "由中间结果得到输出的判定逻辑"
  - name: output
    type: string
    required: false
    description: "方法最终输出的结果"
  - name: algo_name
    type: string
    required: false
    description: "装置中处理模块执行的算法名称"
  - name: effect_desc
    type: multiline
    required: false
    description: "有益效果"
  - name: embodiment
    type: multiline
    required: false
    description: "具体实施方式"
---
# 说明书

## 技术领域
本发明涉及{{tech_field}}技术领域，具体涉及一种{{invention_name}}方法及装置。

## 背景技术
在{{app_scenario}}场景中，现有技术存在以下问题：
1. {{problem_1}}
2. {{problem_2}}

## 发明内容
### 技术方案
第一方面，本发明提供一种{{invention_name}}方法，包括：
1. 获取{{input_data}}；
2. 基于{{algo_core}}对所述{{input_data}}进行处理，得到{{intermediate}}；
3. 根据{{decision_logic}}输出{{output}}。

第二方面，本发明提供一种{{invention_name}}装置，包括：
- 获取模块，用于获取{{input_data}}；
- 处理模块，用于执行{{algo_name}}算法；
- 输出模块，用于输出{{output}}。

### 有益效果
{{effect_desc}}

## 附图说明
图1为本发明方法的流程图。
图2为本发明装置的结构框图。

## 具体实施方式
{{embodiment}}

---

> **填写指引：**
> - 软件专利建议采用"方法+装置"双轨写法
> - `algo_core`：核心算法逻辑
> - 注意避免纯智力活动规则
