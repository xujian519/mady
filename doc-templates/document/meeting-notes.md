---
name: meeting-notes
title: 会议纪要
category: document
description: 中文会议纪要模板：议程、结论、行动项与责任人
domain: document
version: "1.0"
language: zh-CN
use_when: 需要整理并交付会议纪要、评审记录或决策记录时
formats:
  - markdown
  - html
  - docx
vars:
  - name: meeting_title
    type: string
    required: false
    description: "会议名称"
  - name: date
    type: string
    required: false
    description: "会议日期"
  - name: time_range
    type: string
    required: false
    description: "会议时间段"
  - name: location
    type: string
    required: false
    description: "会议地点或线上会议链接"
  - name: host
    type: string
    required: false
    description: "主持人"
  - name: attendees
    type: string
    required: false
    description: "参会人员"
  - name: absentees
    type: string
    required: false
    default: "无"
    description: "缺席人员"
  - name: agenda
    type: multiline
    required: false
    description: "会议议程"
  - name: decisions
    type: multiline
    required: false
    description: "达成的结论与决策，注明依据"
  - name: action_items
    type: multiline
    required: false
    description: "行动项：事项、责任人、截止时间"
  - name: next_meeting
    type: multiline
    required: false
    default: "待定"
    description: "下次会议安排"
  - name: notes
    type: multiline
    required: false
    default: "无"
    description: "其他说明或未决事项"
---
# {{meeting_title}} 会议纪要

**日期：** {{date}}　**时间：** {{time_range}}　**地点：** {{location}}
**主持人：** {{host}}　**参会人员：** {{attendees}}　**缺席人员：** {{absentees}}

## 一、会议议程

{{agenda}}

## 二、结论与决策

{{decisions}}

## 三、行动项

{{action_items}}

## 四、下次会议

{{next_meeting}}

## 五、其他说明

{{notes}}

---

> **使用说明：** 结论与行动项分开写；行动项必须带责任人与截止时间，否则记为未决事项。
