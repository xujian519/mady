---
name: inventiveness-defense
title: 创造性争辩模板
category: oa-response
description: 审查意见答复——创造性三步法争辩
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: d1_number
    type: string
    required: false
    description: "对比文件1的公开号"
  - name: d2_number
    type: string
    required: false
    description: "对比文件2的公开号"
  - name: d1_disclosure
    type: multiline
    required: false
    description: "对比文件1公开的技术内容"
  - name: distinguishing_feature
    type: multiline
    required: false
    description: "权利要求1与对比文件1的区别特征"
  - name: tech_problem
    type: multiline
    required: false
    description: "基于区别特征确定的实际解决的技术问题"
  - name: d2_content
    type: multiline
    required: false
    description: "对比文件2涉及的技术内容"
  - name: reason_1
    type: multiline
    required: false
    description: "对比文件2未给出结合启示的理由1"
  - name: reason_2
    type: multiline
    required: false
    description: "对比文件2未给出结合启示的理由2"
  - name: aux_factor_1
    type: string
    required: false
    default: "无"
    description: "辅助判断因素1（如商业成功、长期需求）"
  - name: aux_factor_2
    type: string
    required: false
    default: "无"
    description: "辅助判断因素2"
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

针对审查意见通知书中关于创造性的驳回理由，申请人现陈述意见如下：

## 一、审查员的驳回理由

审查员认为权利要求1相对于对比文件1（{{d1_number}}）和对比文件2（{{d2_number}}）的结合不具备创造性（专利法第22条第3款）。

## 二、三步法分析

### 第一步：最接近的现有技术

申请人认可对比文件1作为最接近的现有技术。对比文件1公开了：{{d1_disclosure}}。

### 第二步：区别特征及实际解决的技术问题

权利要求1与对比文件1的区别在于：{{distinguishing_feature}}。

基于该区别特征，本发明实际解决的技术问题是：{{tech_problem}}。

### 第三步：非显而易见性

对比文件2虽然涉及{{d2_content}}，但其并未给出将其与对比文件1结合以解决上述技术问题的技术启示。理由如下：

1. {{reason_1}}
2. {{reason_2}}

因此，权利要求1的技术方案对于本领域技术人员而言并非显而易见。

## 三、辅助判断因素

- {{aux_factor_1}}
- {{aux_factor_2}}

## 四、结论

综上所述，修改后的权利要求1具备创造性，请求审查员予以认可。

代理人：{{agent_name}}
日期：{{date}}

---

> **使用说明：**
> - 必须按三步法框架论证
> - "技术启示"论证是核心
> - 辅助因素可增强说服力
