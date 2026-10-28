---
name: patent-invalidation-analysis
description: |
  内部无效宣告分析意见模板（专利律师场景内部交付物）。将无效可行性评估、对比文件、
  逐权项结论分档、引用日志与缺口披露渲染为事务所内部使用的无效分析意见；对外提交件
  用 `invalidation-opinion` 模板。
triggers:
  - "无效分析意见"
  - "内部无效意见"
  - "无效风险评估"
  - "invalidity analysis"
  - "patent invalidation analysis"
template:
  kind: patent-document
  mode: opinion
  scenario: patent-invalidation
  preview:
    type: html
    entry: assets/template.html
  exports: [html, pdf]
---

# 内部无效宣告分析意见模板

将无效可行性评估渲染为**所内使用的内部无效分析意见**：逐权项给出结论档与法条路径，披露证据缺口与待补事项，并把关键断言记入引用日志。本件不对外提交；**对外提交件用 `invalidation-opinion`（专利无效宣告请求书）**。两个产物章节集合不同，不做模式切换——内部件承载的内部工作记录不得出现在对外件里。

## 输入要求

渲染前必须已具备：

1. 涉案专利授权公告文本（权利要求书、说明书、附图）。
2. 涉案专利法律状态（专利号、授权公告日、专利权人）与委托方信息。
3. 原始申请文件：修改超范围理由的核对基线。
4. 证据清单（每份含标识与名称、公开或出版日、与涉案专利的时间关系；登记口径见 `invalidation-opinion/references/conventions.md` §4.1）。正文引用与清单登记共用同一编号，两侧由渲染器机械核对。
5. 权利要求特征分解（特征编号、技术特征、技术效果）。
6. 逐权项证据覆盖情况，含未获得覆盖、因而难以主张的权项。
7. 未完成事项清单：待核外文原文、待补检索、待取材料、待确认的优先权与程序史。
8. 免责声明与密级（模板已内置）。

## 工作流

1. 读 `references/conventions.md` 与 `references/citation-log.md`；§2.1 的从属权项法条路径与对外件同口径。
2. 用 `render_patent_document` 渲染：template 传 `invalidation-analysis`，draft 传 id 键控草案。
   - `fields`：`meta-patent-no`、`meta-patent-title`、`meta-grant-date`、`meta-patentee`、`meta-client`、`meta-date`、`scope-summary`、`conclusion-text`、`footer-case`、`footer-date`。
   - `sections`（blocks 槽）：`doc-number`、`scope-points`、`gaps-body`。
   - `sections`（rows 槽）：`claim-feature-table`（4 列）、`evidence-table`（6 列）、`claim-outlook-table`（4 列）、`citation-table`（6 列）。
   - `sections`（grounds 槽）：`grounds-body`。每个理由只提交法条、理由名、被主张的权项号与三段正文；节标题（`4.1 权利要求 1 不具备创造性（…）`）与四段式小标题由渲染器按结构生成，**不得手写**。
   - 槽位清单与必填项以草案校验报错为准。
3. 按结构填充：案件信息 → 分析范围与依据 → 涉案专利权利要求分析 → 证据清单 → 无效理由逐条分析 → 逐权项结论与分档 → 引用日志 → 缺口与待补事项 → 分析结论。
4. 无效理由按法条组织，每个理由下列出：事实认定 → 证据分析 → 法条适用与结论。
5. 逐权项结论覆盖全部目标权项；从属权项与其引用权项使用同一法条路径，附加技术特征无覆盖时记入结论档与第七节，不得改写法条路径来凑理由。
6. 对比文件公开日必须早于涉案专利优先权日 / 申请日；未主张优先权的以申请日为界，抵触申请单独标注。
7. 删除占位文案，按 `references/checklist.md` 自查后定稿。

## 输出契约

```
文件：invalidation-analysis.html（单文件、内联 CSS）
可选：invalidation-analysis.pdf（A4 打印）
     invalidation-analysis.md（源稿）
```

**本件产出内部件**：可载引用日志、逐权项结论分档、缺口与待补事项、风险与最坏情形。这些内部工作记录不得进入对外件；对外件只写实际主张的理由与请求结论，并在该模板的 `references/conventions.md` §6.1 列出禁止项。本件同样不写「请求宣告……无效」一类对外请求句。

品牌变量由 `assets/templates/patent/tokens.css` 注入；agent 不修改样式。
