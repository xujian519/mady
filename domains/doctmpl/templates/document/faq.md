---
name: faq
title: 常见问题（FAQ）
category: document
description: 中文常见问题文档模板：问题与回答成对出现，附适用范围与反馈渠道
domain: document
version: "1.0"
language: zh-CN
use_when: 需要交付帮助文档、FAQ、答疑清单或自助支持材料时
formats:
  - markdown
  - html
  - docx
vars:
  - name: topic
    type: string
    required: false
    description: "主题或产品名称"
  - name: version
    type: string
    required: false
    description: "文档版本号"
  - name: updated_at
    type: string
    required: false
    description: "最近更新日期"
  - name: audience
    type: multiline
    required: false
    description: "适用读者与使用前提"
  - name: question_1
    type: multiline
    required: false
    description: "问题1"
  - name: answer_1
    type: multiline
    required: false
    description: "问题1的回答"
  - name: question_2
    type: multiline
    required: false
    description: "问题2"
  - name: answer_2
    type: multiline
    required: false
    description: "问题2的回答"
  - name: question_3
    type: multiline
    required: false
    description: "问题3"
  - name: answer_3
    type: multiline
    required: false
    description: "问题3的回答"
  - name: question_4
    type: multiline
    required: false
    description: "问题4"
  - name: answer_4
    type: multiline
    required: false
    description: "问题4的回答"
  - name: contact
    type: multiline
    required: false
    description: "反馈渠道与联系方式"
---
# {{topic}} 常见问题

**版本：** {{version}}　**更新日期：** {{updated_at}}

## 适用读者

{{audience}}

## 问题1

{{question_1}}

**回答：** {{answer_1}}

## 问题2

{{question_2}}

**回答：** {{answer_2}}

## 问题3

{{question_3}}

**回答：** {{answer_3}}

## 问题4

{{question_4}}

**回答：** {{answer_4}}

## 反馈与联系

{{contact}}

---

> **使用说明：** 一个问题只回答一件事；回答里不要写"请咨询客服"以外的空泛结论。
