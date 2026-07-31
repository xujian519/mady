# Google Patents Overview

## 搜索页（`https://patents.google.com/?q=...&num=N`）

- 查询框: `input[name="q"]`
- 结果容器: `search-result-item`（自定义元素，`id` 形如 `patent/CN110515732B`）
- 标题: `state-modifier.result-title` 内的 `a#link`（innerText，含 `<b>` 高亮标签）
- 专利号: `a.pdfLink` 内的 `span` 文本
- PDF 链接: `a.pdfLink[href]`（`https://patentimages.storage.googleapis.com/...`）
- 国别: `span.active`（CN/US/EP 等）
- 发明人/权利人: `h4.metadata` 内 `span.bullet-before raw-html` 序列（专利号后第一个为发明人，其余为权利人）
- 日期行: metadata 后独立 heading，格式 `Priority YYYY-MM-DD • Filed YYYY-MM-DD • Granted YYYY-MM-DD • Published YYYY-MM-DD`
- 摘要: `div.abstract`（innerText 中 metadata 行之后的部分）

**稳妥提取方式**：取 `search-result-item` 的 `innerText` 按行解析——
第 1 行标题，第 2 行 `{country} • {number} • {inventors} • {assignee}`，第 3 行日期，其余为摘要。

## 详情页（`https://patents.google.com/patent/{NUMBER}/zh`）

- 标题: `meta[name="DC.title"]`
- 专利号: `meta[name="citation_patent_number"]`
- PDF: `meta[name="citation_pdf_url"]`（或 `a.pdfLink, a[href*=".pdf"]`）
- 摘要: `section#abstract`（注意：使用 **id** 而非 class）
- 说明书: `section#description`（懒加载，需滚动到底部触发）
- 权利要求: `section#claims`（懒加载）

## 搜索语法

- 支持操作符：`country:CN`、`assignee:`、`inventor:`、`before:priority:20200101`、`after:priority:20200101`、`language:Chinese`、`type:PATENT`
- 结果按 family 去重，个别结果可能显示同族最早的公开版本（国别与 CN 不同属正常现象）
