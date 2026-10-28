---
name: clarity-amendment
title: 不清楚条款修改模板
category: oa-response
description: 审查意见答复——不清楚/不支持条款修改
domain: patent
version: "1.0"
language: zh-CN
formats:
  - markdown
  - html
  - docx
vars:
  - name: defect_1
    type: string
    required: false
    description: "第一项缺陷的类型（如不清楚、不支持）"
  - name: num_1
    type: number
    required: false
    description: "第一项缺陷涉及的权利要求编号"
  - name: issue_1
    type: multiline
    required: false
    description: "审查员针对第一项缺陷指出的具体问题"
  - name: defect_2
    type: string
    required: false
    description: "第二项缺陷的类型"
  - name: num_2
    type: number
    required: false
    description: "第二项缺陷涉及的权利要求编号"
  - name: issue_2
    type: multiline
    required: false
    description: "审查员针对第二项缺陷指出的具体问题"
  - name: orig_claim_1
    type: multiline
    required: false
    description: "修改前权利要求原文"
  - name: amended_claim_1
    type: multiline
    required: false
    description: "修改后权利要求原文"
  - name: para
    type: number
    required: false
    description: "修改依据所在的说明书段落号"
  - name: support
    type: multiline
    required: false
    description: "修改依据：原始说明书中支持该修改的记载"
  - name: orig_claim_2
    type: multiline
    required: false
    description: "第二条修改前权利要求原文"
  - name: amended_claim_2
    type: multiline
    required: false
    description: "第二条修改后权利要求原文"
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

针对审查意见通知书中指出的权利要求不清楚/不支持的缺陷，申请人做出以下修改和陈述：

## 一、审查意见摘要

| 缺陷类型 | 涉及权利要求 | 具体问题 |
|----------|-------------|----------|
| {{defect_1}} | 权利要求{{num_1}} | {{issue_1}} |
| {{defect_2}} | 权利要求{{num_2}} | {{issue_2}} |

## 二、修改说明

### 针对权利要求{{num_1}}的修改

原权利要求{{num_1}}为：
```
{{orig_claim_1}}
```

修改后：
```
{{amended_claim_1}}
```

修改依据：说明书第[{{para}}]段记载了{{support}}。

### 针对权利要求{{num_2}}的修改

原权利要求{{num_2}}为：
```
{{orig_claim_2}}
```

修改后：
```
{{amended_claim_2}}
```

## 三、关于修改未超范围的说明

上述修改均基于原始申请文件的明确记载，符合专利法第33条的规定。

## 四、结论

申请人认为，经上述修改后的权利要求已克服审查员指出的缺陷，请求继续审查。

代理人：{{agent_name}}
日期：{{date}}

---

> **使用说明：**
> - 每条修改需注明在原始说明书中的依据段落
> - 修改不得超出原始公开范围
