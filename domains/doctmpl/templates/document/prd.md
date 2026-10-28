---
name: prd
title: 产品需求文档
category: document
description: 中文产品需求文档（PRD）模板：背景、目标用户、功能需求、非功能需求与里程碑
domain: document
version: "1.0"
language: zh-CN
use_when: 需要定义或评审一个产品/功能的范围、需求与验收标准时
formats:
  - markdown
  - html
  - docx
vars:
  - name: product_name
    type: string
    required: false
    description: "产品、功能或模块名称"
  - name: doc_version
    type: string
    required: false
    description: "文档版本号"
  - name: author
    type: string
    required: false
    description: "文档负责人"
  - name: date
    type: string
    required: false
    description: "文档日期"
  - name: background
    type: multiline
    required: false
    description: "业务背景与要解决的问题"
  - name: target_users
    type: multiline
    required: false
    description: "目标用户、使用场景与核心痛点"
  - name: goals
    type: multiline
    required: false
    description: "产品目标与可度量的成功指标"
  - name: scope
    type: multiline
    required: false
    description: "本期范围（做什么、不做什么）与范围外事项"
  - name: requirements
    type: multiline
    required: false
    description: "功能需求清单，每条含优先级与验收口径"
  - name: user_stories
    type: multiline
    required: false
    description: "关键用户故事"
  - name: non_functional
    type: multiline
    required: false
    description: "非功能需求（性能、安全、兼容性、可维护性）"
  - name: milestones
    type: multiline
    required: false
    description: "里程碑与排期"
  - name: open_questions
    type: multiline
    required: false
    default: "无"
    description: "待决问题与负责人"
---
# {{product_name}} 产品需求文档

**版本：** {{doc_version}}　**负责人：** {{author}}　**日期：** {{date}}

## 一、背景与问题

{{background}}

## 二、目标用户与场景

{{target_users}}

## 三、产品目标与成功指标

{{goals}}

## 四、范围

{{scope}}

## 五、功能需求

{{requirements}}

## 六、关键用户故事

{{user_stories}}

## 七、非功能需求

{{non_functional}}

## 八、里程碑与排期

{{milestones}}

## 九、待决问题

{{open_questions}}

---

> **使用说明：** 每条功能需求都应能被验证；范围外的想法放进"待决问题"而不是正文。
