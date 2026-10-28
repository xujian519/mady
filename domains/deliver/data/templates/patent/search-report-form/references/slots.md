# 槽位清单（机械统计 + 命名）

本文档是 `assets/template.html` 全部填写点的机械清单，是 draft 注册表
（`dsh-patent-document` 的 `draftSchema/`）的事实源：注册表槽位与本文档一一对应，
一致性测试逐条锁定。行号为 `template.html` 中的行号。

统计口径（2026-10-09，页码移出后）：fill 类 33 种 36 个 + 勾选项 1 个 + 章节槽位 5 种 7 个 = 44 处 `data-slot`。

## 文本槽位（`.fill` → `text`）

| 槽位 id | 位置 | 说明 |
| --- | --- | --- |
| `reportNo` | L146 | 报告编号 |
| `searchDateYear/Month/Day` | L147 | 检索日期 |
| `cutoffDateYear/Month/Day` | L148 | 检索截止日 |
| `applicationNo` | L151 | 申请号/专利号 |
| `applicationDateYear/Month/Day` | L152 | 申请日 |
| `inventionTitle` | L155 | 发明名称 |
| `patentType` | L156 | 专利类型 |
| `patentee` | L159 | 专利权人/申请人 |
| `client` | L160 | 委托方 |
| `searcher` | L163/290 | 检索人（抬头与落款同一值） |
| `reviewer` | L164/290 | 审核人（抬头与落款同一值） |
| `ipcClass` | L171 | A. 主题分类（IPC/CPC） |
| `footerFirm` | L226/300 | 页脚机构名（两处同一值） |
| `distXCount/Ratio/Impact` | L257 | 相关度分布 X 行 |
| `distYCount/Ratio/Impact` | L258 | 相关度分布 Y 行 |
| `distACount/Ratio/Impact` | L259 | 相关度分布 A 行 |
| `distTotalCount/Ratio` | L260 | 合计行（影响列固定为「—」） |
| `reportDateYear/Month/Day` | L290 | 落款日期 |

## 选项槽位（`.cb` → `choice`，值为选项 id）

| 组 id | 选项 id（模板行） | 多选 |
| --- | --- | --- |
| `moreDocuments` | `continued`（L206） | 否 |

## 章节槽位

| 槽位 id | 类型 | 位置 | 说明 |
| --- | --- | --- | --- |
| `searchField` | blocks | L176 | B. 检索领域 |
| `databases` | blocks | L181/182 | C. 数据库与检索式（两个空行占位，多余自动移除） |
| `relatedDocuments` | rows（6 列） | L203 | D. 相关文件数据行，按草案行数复制 |
| `conclusion` | blocks | L245/246 | E. 检索结论 |
| `searchRounds` | rows（5 列） | L274 | 附. 检索式执行记录数据行，按草案行数复制 |

## 纯展示元素（无槽位，逐条理由）

- 页脚页码 `第 N 页　共 M 页`（L227/301）：逐页页脚在静态 HTML 中各不相同，单一草案值无法表达；页码在打印/定稿时由人工或分页流程填写。

- F. 检索范围与局限的三个声明勾选项（L281–283）：固定免责语句，是否勾选由人工定稿时决定，不由草案驱动；示例件中保持未勾选。
- 相关度分布表的类型列（X/Y/A/合计）与合计行影响列「—」（L257–260）：固定行标签。
- 表头/分区标题/图例（L170、L175、L180、L186、L210–217、L250、L255、L270、L280、L295）：文书固定文字。
- 抬头 `firm-head`（L128–131、L234–237）：机构抬头与密级声明，属品牌/版式固有元素（品牌注入另行处理）。
- 落款声明块 `.statement`（L294–296）：固定免责声明文本。

## 渲染对比证据

`data-slot` 属性注入前后（2026-10-09）经 headless Chrome 截图比对，两份模板
PNG 的 MD5 完全一致（`31526a17…`），版式零变化。
