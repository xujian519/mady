---
name: apparatus-claim
title: 装置权利要求
category: claims
description: 装置/设备类专利权利要求模板
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: apparatus_name
    type: string
    required: false
    description: "装置名称"
  - name: comp_1
    type: string
    required: false
    description: "第一部件名称"
  - name: func_1
    type: string
    required: false
    description: "第一部件的功能"
  - name: comp_2
    type: string
    required: false
    description: "第二部件名称"
  - name: func_2
    type: string
    required: false
    description: "第二部件的功能"
  - name: comp_3
    type: string
    required: false
    description: "第三部件名称"
  - name: func_3
    type: string
    required: false
    description: "第三部件的功能"
  - name: comp_1_sub_1
    type: multiline
    required: false
    description: "第一部件包括的第一子部件或子结构"
  - name: comp_1_sub_2
    type: multiline
    required: false
    description: "第一部件包括的第二子部件或子结构"
  - name: comp_2_type
    type: string
    required: false
    description: "第二部件的具体类型或结构"
  - name: additional_comp
    type: string
    required: false
    description: "附加部件名称"
  - name: additional_func
    type: string
    required: false
    description: "附加部件的功能"
---
# 权利要求书

## 1. 一种{{apparatus_name}}，其特征在于，包括：

{{comp_1}}，用于{{func_1}}；

{{comp_2}}，与所述{{comp_1}}连接，用于{{func_2}}；

{{comp_3}}，与所述{{comp_2}}连接，用于{{func_3}}。

## 2. 根据权利要求1所述的{{apparatus_name}}，其特征在于，所述{{comp_1}}包括：

{{comp_1_sub_1}}；

{{comp_1_sub_2}}。

## 3. 根据权利要求1所述的{{apparatus_name}}，其特征在于，所述{{comp_2}}为{{comp_2_type}}。

## 4. 根据权利要求1所述的{{apparatus_name}}，其特征在于，所述装置还包括：

{{additional_comp}}，用于{{additional_func}}。

---

> **填写指引：**
> - `apparatus_name`：装置名称
> - `comp_N`：部件名称
> - `func_N`：部件功能
> - 从属权利要求按"包含部件"或"限定类型"逐级递进
