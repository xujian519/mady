---
name: chemical-spec
title: 化学领域说明书
category: specification
description: 化学/材料领域专利申请说明书模板
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
    description: "发明名称（化合物或材料名称）"
  - name: background_desc
    type: multiline
    required: false
    description: "背景技术现状"
  - name: prior_art_1
    type: multiline
    required: false
    description: "现有技术方案1"
  - name: drawback_1
    type: multiline
    required: false
    description: "现有技术方案1的缺陷"
  - name: prior_art_2
    type: multiline
    required: false
    description: "现有技术方案2"
  - name: drawback_2
    type: multiline
    required: false
    description: "现有技术方案2的缺陷"
  - name: chemical_formula
    type: multiline
    required: false
    description: "化学结构式（文本、ASCII 或 SMILES 描述）"
  - name: raw_1
    type: string
    required: false
    description: "制备原料1的名称"
  - name: raw_2
    type: string
    required: false
    description: "制备原料2的名称"
  - name: cond_1
    type: string
    required: false
    description: "第一步反应的反应条件（温度、压力、催化剂）"
  - name: time_1
    type: string
    required: false
    description: "第一步反应的反应时间"
  - name: cond_2
    type: string
    required: false
    description: "第二步反应的反应条件"
  - name: reaction_type
    type: string
    required: false
    description: "第二步反应的反应类型（如还原、取代）"
  - name: effect_1
    type: multiline
    required: false
    description: "有益效果1"
  - name: effect_2
    type: multiline
    required: false
    description: "有益效果2"
  - name: embodiment_1
    type: multiline
    required: false
    description: "实施例1的具体内容"
  - name: embodiment_2
    type: multiline
    required: false
    description: "实施例2的具体内容"
---
# 说明书

## 技术领域
本发明属于{{tech_field}}技术领域，具体涉及一种{{invention_name}}及其制备方法和应用。

## 背景技术
{{background_desc}}

现有技术方案包括：
1. {{prior_art_1}}，但其存在{{drawback_1}}。
2. {{prior_art_2}}，但其{{drawback_2}}。

## 发明内容
### 技术方案
一种{{invention_name}}，其化学结构式如下：
```
{{chemical_formula}}
```

所述{{invention_name}}的制备方法，包括以下步骤：
1. 将{{raw_1}}与{{raw_2}}在{{cond_1}}条件下反应{{time_1}}，得到中间体；
2. 将所述中间体在{{cond_2}}条件下{{reaction_type}}，得到所述{{invention_name}}。

### 有益效果
1. {{effect_1}}
2. {{effect_2}}

## 具体实施方式
### 实施例1
{{embodiment_1}}

### 实施例2
{{embodiment_2}}

---

> **填写指引：**
> - `chemical_formula`：用ASCII或文本描述化学结构
> - `raw_N`：原料名称
> - `cond_N`：反应条件（温度/压力/催化剂）
