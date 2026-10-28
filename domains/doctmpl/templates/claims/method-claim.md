---
name: method-claim
title: 方法权利要求
category: claims
description: 方法类发明专利权利要求模板
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: method_name
    type: string
    required: false
    description: "方法名称"
  - name: step_1
    type: multiline
    required: false
    description: "步骤1的具体操作"
  - name: step_2
    type: multiline
    required: false
    description: "步骤2的具体操作"
  - name: step_3
    type: multiline
    required: false
    description: "步骤3的具体操作"
  - name: step_1_detail
    type: multiline
    required: false
    description: "对步骤1的进一步限定"
  - name: step_4
    type: multiline
    required: false
    description: "附加步骤4的具体操作"
  - name: key_param
    type: string
    required: false
    description: "关键技术参数名称"
  - name: range_value
    type: string
    required: false
    description: "关键技术参数的取值范围"
---
# 权利要求书

## 1. 一种{{method_name}}方法，其特征在于，包括以下步骤：

步骤1：{{step_1}}；

步骤2：{{step_2}}；

步骤3：{{step_3}}。

## 2. 根据权利要求1所述的方法，其特征在于，所述{{step_1}}具体包括：

{{step_1_detail}}。

## 3. 根据权利要求1所述的方法，其特征在于，所述方法还包括：

步骤4：{{step_4}}。

## 4. 根据权利要求1所述的方法，其特征在于，所述{{key_param}}的取值范围为{{range_value}}。

---

> **填写指引：**
> - `method_name`：方法名称，如"一种图像处理方法"
> - `step_1/2/3`：核心步骤描述
> - `step_1_detail`：对步骤1的进一步限定
> - `key_param`：关键技术参数
> - `range_value`：参数范围或取值
