---
name: release-notes
title: 发布说明
category: document
description: 中文发布说明（Release Notes）模板：新增、改进、修复、破坏性变更与升级说明
domain: document
version: "1.0"
language: zh-CN
use_when: 需要向用户交付版本发布说明或升级公告时
formats:
  - markdown
  - html
  - docx
vars:
  - name: product_name
    type: string
    required: false
    description: "产品名称"
  - name: version
    type: string
    required: false
    description: "本次发布的版本号"
  - name: release_date
    type: string
    required: false
    description: "发布日期"
  - name: summary
    type: multiline
    required: false
    description: "本次发布概述：最重要的变化与影响面"
  - name: new_features
    type: multiline
    required: false
    description: "新增功能"
  - name: improvements
    type: multiline
    required: false
    description: "体验或性能改进"
  - name: fixes
    type: multiline
    required: false
    description: "问题修复"
  - name: breaking_changes
    type: multiline
    required: false
    default: "无"
    description: "破坏性变更与受影响范围"
  - name: upgrade_notes
    type: multiline
    required: false
    default: "无"
    description: "升级步骤与回滚方式"
  - name: known_issues
    type: multiline
    required: false
    default: "无"
    description: "已知问题与临时规避方式"
---
# {{product_name}} {{version}} 发布说明

**发布日期：** {{release_date}}

## 发布概述

{{summary}}

## 新增功能

{{new_features}}

## 改进

{{improvements}}

## 修复

{{fixes}}

## 破坏性变更

{{breaking_changes}}

## 升级说明

{{upgrade_notes}}

## 已知问题

{{known_issues}}

---

> **使用说明：** 破坏性变更必须写清受影响的接口或行为；没有变更时保留"无"而不是删掉小节。
