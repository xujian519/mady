---
name: system-claim
title: 系统权利要求
category: claims
description: 系统级专利权利要求模板
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: system_name
    type: string
    required: false
    description: "系统名称"
  - name: sub_1
    type: string
    required: false
    description: "第一子系统名称"
  - name: func_1
    type: string
    required: false
    description: "第一子系统的功能"
  - name: sub_2
    type: string
    required: false
    description: "第二子系统名称"
  - name: func_2
    type: string
    required: false
    description: "第二子系统的功能"
  - name: sub_1_unit
    type: string
    required: false
    description: "第一子系统包含的单元名称"
  - name: topology
    type: string
    required: false
    description: "单元之间的拓扑连接方式"
  - name: protocol
    type: string
    required: false
    description: "第二子系统与控制单元的通信协议"
  - name: additional_sub
    type: string
    required: false
    description: "附加子系统名称"
  - name: additional_func
    type: string
    required: false
    description: "附加子系统的功能"
---
# 权利要求书

## 1. 一种{{system_name}}，其特征在于，包括：

{{sub_1}}，用于{{func_1}}；

{{sub_2}}，与所述{{sub_1}}通信连接，用于{{func_2}}；

控制单元，分别与所述{{sub_1}}和所述{{sub_2}}连接，用于协调各子系统工作。

## 2. 根据权利要求1所述的系统，其特征在于，所述{{sub_1}}包括：

多个{{sub_1_unit}}，以{{topology}}方式连接。

## 3. 根据权利要求1所述的系统，其特征在于，所述{{sub_2}}通过{{protocol}}与所述控制单元通信。

## 4. 根据权利要求1所述的系统，其特征在于，所述系统还包括：

{{additional_sub}}，用于{{additional_func}}。

---

> **填写指引：**
> - `system_name`：系统名称
> - `sub_N`：子系统名称
> - `topology`：拓扑连接方式
> - `protocol`：通信协议
