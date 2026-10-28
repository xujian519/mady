---
name: novelty-defense
title: 新颖性争辩模板
category: oa-response
description: 审查意见答复——新颖性争辩
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: oa_date
    type: string
    required: false
    description: "审查意见通知书的发出日期"
  - name: oa_number
    type: number
    required: false
    description: "审查意见通知书的次数"
  - name: d1_number
    type: string
    required: false
    description: "对比文件1的公开号"
  - name: feat_a
    type: string
    required: false
    description: "技术特征A的名称"
  - name: claim_feat_a
    type: string
    required: false
    description: "权利要求1中特征A的表述"
  - name: d1_feat_a
    type: string
    required: false
    description: "对比文件1中对应特征的表述"
  - name: feat_b
    type: string
    required: false
    description: "技术特征B的名称"
  - name: claim_feat_b
    type: string
    required: false
    description: "权利要求1中特征B的表述"
  - name: d1_feat_b
    type: string
    required: false
    description: "对比文件1中对应特征的表述"
  - name: key_diff
    type: string
    required: false
    description: "对比文件1未公开的关键区别特征"
  - name: orig_num
    type: number
    required: false
    description: "被修改的原始权利要求编号"
  - name: amended_content
    type: multiline
    required: false
    description: "对原权利要求所做的修改内容"
  - name: amended_claim
    type: multiline
    required: false
    description: "修改后的权利要求全文"
  - name: agent_name
    type: string
    required: false
    description: "代理人姓名"
  - name: date
    type: string
    required: false
    description: "意见陈述书日期"
---
# 意见陈述书

尊敬的审查员：

针对贵局于{{oa_date}}发出的第{{oa_number}}次审查意见通知书，申请人现陈述意见如下：

## 一、关于权利要求1的新颖性

审查员认为权利要求1相对于对比文件1（{{d1_number}}）不具备新颖性（专利法第22条第2款）。

申请人认为，该审查意见不能成立。理由如下：

### 区别特征分析

| 技术特征 | 权利要求1 | 对比文件1 | 判断 |
|----------|-----------|-----------|------|
| {{feat_a}} | {{claim_feat_a}} | {{d1_feat_a}} | 不同 |
| {{feat_b}} | {{claim_feat_b}} | {{d1_feat_b}} | 不同 |

如上表所示，对比文件1至少未公开本发明的{{key_diff}}。

### 单独对比原则

根据审查指南的规定，新颖性判断应遵循单独对比原则。对比文件1未公开权利要求1的全部技术特征，因此权利要求1具备新颖性。

## 二、修改说明

为更清楚地界定保护范围，申请人将原权利要求{{orig_num}}的{{amended_content}}。

修改后的权利要求1如下：
```
{{amended_claim}}
```

综上，请求审查员在修改后的权利要求基础上继续审查。

代理人：{{agent_name}}
日期：{{date}}

---

> **使用说明：**
> - 仅适用于新颖性争辩
> - 如涉及创造性争辩，请使用"创造性争辩模板"
