# 槽位清单（机械统计 + 命名）

本文档是 `assets/template.html` 全部填写点的机械清单，是 draft 注册表
（`dsh-patent-document` 的 `draftSchema/`）的事实源：注册表槽位与本文档一一对应，
一致性测试逐条锁定。行号为 `template.html` 中的行号。

统计口径（2026-10-09，页码移出后）：fill 类 42 种 47 个 + 勾选项 27 个 + 章节槽位 5 种 9 个 = 83 处 `data-slot`。

## 文本槽位（`.fill` → `text`）

| 槽位 id | 位置 | 说明 |
| --- | --- | --- |
| `patentNo` | L153 | 专利号 |
| `applicationDateYear/Month/Day` | L154 | 申请日（年/月/日三分栏） |
| `priorityDateYear/Month/Day` | L155 | 优先权日（无优先权时留空） |
| `grantDateYear/Month/Day` | L158 | 授权公告日 |
| `inventionTitle` | L159 | 实用新型名称 |
| `patentee` | L162 | 专利权人 |
| `requester` | L163 | 请求人 |
| `requestDateYear/Month/Day` | L166 | 请求日 |
| `totalPages` | L169 | 评价报告总计页数 |
| `attachedCopiesCount` | L170 | 引用文件副本份数（随 `attachedCopies` 勾选） |
| `invalidDecisionNo` | L178 | 无效宣告请求审查决定号（随 `evalTarget=maintained`） |
| `allClaimsRange` | L183 | 「全部权利要求 N」的项号范围 |
| `art2NotSearchedClaims` | L184 | 未被检索：专利法第 2 条第 3 款 |
| `art5NotSearchedClaims` | L185 | 未被检索：第 5 条或第 25 条 |
| `notSearchedUtilityClaims` | L186 | 未被检索：实用性 |
| `otherNotSearchedClaims` / `otherNotSearchedReason` | L188 | 其他未被检索的项号与理由 |
| `ipcClass` | L204 | A. 主题分类（IPC） |
| `soundClaimsRange` | L274 | 初步结论：未发现缺陷的权利要求 |
| `defectiveClaimsRange` | L275 | 初步结论：不符合授权条件的权利要求 |
| `partialDefectiveClaims` / `partialSoundClaims` | L276 | 初步结论：部分不符合/部分未发现 |
| `art5Claims` / `art25Claims` / `art2SubjectClaims` / `utilityDefectClaims` | L280–283 | 具体结论：不授权范围/实用性缺陷的项号 |
| `noveltyYesClaims` / `noveltyNoClaims` | L290/291 | 新颖性具备/不具备的项号 |
| `inventiveYesClaims` / `inventiveNoClaims` | L292/293 | 创造性具备/不具备的项号 |
| `art264Claims` / `rules202Claims` / `art33Claims` / `art9Claims` | L299–302 | 清楚性/支持/修改超范围/重复授权的项号 |
| `examiner` / `reviewer` | L323/324 | 审查员/审核员（签名人工补，非必填） |
| `completionDateYear/Month/Day` | L325 | 完成日期（非必填） |

## 选项槽位（`.cb` → `choice`，值为选项 id）

| 组 id | 选项 id（模板行） | 多选 |
| --- | --- | --- |
| `attachedCopies` | `attached`（L170） | 否 |
| `evalTarget` | `granted`（L177）/ `maintained`（L178） | 否 |
| `searchScope` | `all`（L183）/ `art2`（L184）/ `art5_25`（L185）/ `utility`（L186）/ `enablement`（L187）/ `other`（L188） | 是 |
| `moreDocuments` | `continued`（L239） | 否 |
| `prelimConclusion` | `allSound`（L274）/ `allDefective`（L275）/ `partial`（L276） | 否 |
| `scopeConclusion` | `art5`（L280）/ `art25`（L281）/ `art2`（L282）/ `utility224`（L283）/ `spec263`（L284，无项号 fill） | 是 |
| `noveltyConclusion` | `novel`（L290）/ `no`（L291） | 是 |
| `inventiveConclusion` | `yes`（L292）/ `no`（L293） | 是 |
| `clarityConclusion` | `art264`（L299）/ `rules202`（L300）/ `art33`（L301）/ `art9`（L302） | 是 |
| `moreOpinion` | `continued`（L312） | 否 |

## 章节槽位

| 槽位 id | 类型 | 位置 | 说明 |
| --- | --- | --- | --- |
| `searchField` | blocks | L209 | B. 检索领域；模板 `.part-value` 为行包装，多余空行自动移除 |
| `databases` | blocks | L214/215 | C. 数据库与检索式（两个空行占位，多余自动移除） |
| `relatedDocuments` | rows（6 列） | L236 | D. 相关文件数据行，按草案行数复制 |
| `opinion` | blocks | L309/310 | 专利权评价意见（第 3 页） |
| `opinionContinued` | blocks | L344–346 | 评价意见续页 II；草案不给该槽时保留空骨架页 |

## 纯展示元素（无槽位，逐条理由）

- 页脚页码 `第 N 页　共 M 页`（L195/264/333/353）：逐页页脚在静态 HTML 中各不相同，单一草案值无法表达；页码在打印/定稿时由人工或分页流程填写，与签名块「人工补」约定一致。

- 表头/分区标题/图例（L176、L208、L213、L219、L243–251、L273、L278、L307、L341）：文书固定文字，不由模型填写。
- `spec263` 选项（L284）无配套 fill：结论针对说明书整体，不涉及权利要求项号。
- 落款专用章（L322）与表格代码 `220701/2011.4`：版式固有元素。
- 续页勾选行 `moreDocuments`/`moreOpinion` 的「参见续页」文字：固定引导语。

## 渲染对比证据

`data-slot` 属性注入前后（2026-10-09）经 headless Chrome 截图比对，两份模板
PNG 的 MD5 完全一致（`0f40adcf…` / `31526a17…`），版式零变化。
