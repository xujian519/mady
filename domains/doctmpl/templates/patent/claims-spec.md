---
name: claims-spec
title: 专利权利要求书与说明书（撰写稿）
category: patent-report
description: 专利撰写初稿，含权利要求书与说明书正文（技术领域/背景/发明内容/附图说明/实施例）
domain: patent
version: "1.0.0"
language: zh-CN
style: sati
formats:
  - markdown
  - html
vars:
  - name: firm_name
    type: string
    required: true
    description: 机构名称
  - name: doc_no
    type: string
    required: false
    description: 文档编号
  - name: application_no
    type: string
    required: false
    description: 申请号
  - name: invention_title
    type: string
    required: true
    description: 发明名称
  - name: technical_field
    type: multiline
    required: true
    description: 技术领域
  - name: background
    type: multiline
    required: true
    description: 背景技术
  - name: summary
    type: multiline
    required: true
    description: 发明内容
  - name: claims
    type: multiline
    required: true
    description: 权利要求书正文
  - name: embodiments
    type: multiline
    required: true
    description: 具体实施方式/实施例
  - name: disclaimer
    type: string
    required: false
    default: "本撰写稿由 AI 辅助生成，供代理人整理，不构成正式申请文件。"
    description: 免责声明
---

# {{invention_title}}

<div class="doc-meta">
**机构：** {{firm_name}}　**文档编号：** {{doc_no}}　**申请号：** {{application_no}}
</div>

## 权利要求书

{{claims}}

## 说明书

### 技术领域

{{technical_field}}

### 背景技术

{{background}}

### 发明内容

{{summary}}

### 附图说明

*（此处列出附图，如 图1 本发明的整体结构示意图；图2 本发明的方法流程图。）*

### 具体实施方式

{{embodiments}}

<div class="callout warning">
> 撰写提示：权利要求须以说明书为依据（A26.4），并满足清楚/简要（A26.4）、充分公开（A26.3）；修改应避免超范围（A33）。
</div>

## 假设与局限

- 本稿为初稿，权利要求布局与说明书支持性表述需代理人结合技术内容完善。
- 附图编号与实施方式描述需与实际技术方案核对一致。

---

> ⚠️ {{disclaimer}}
