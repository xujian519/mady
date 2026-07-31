# CNIPA (Chinese Patents) Overview

## 数据源说明

本 learning 通过 **Google Patents 的 `country:CN` 过滤**检索中国专利，原因：

- Google Patents 完整收录中国专利（CN 申请/授权全文，含说明书与权利要求）
- CNIPA 官方电子申请公布系统（epub.cnipa.gov.cn）对自动化访问不稳定（超时/防护），且无公开稳定的查询 API
- Google Patents 查询语法可覆盖 CNIPA 官网的核心检索维度：关键词、申请人、发明人、日期、IPC

## 搜索页（`https://patents.google.com/?q=QUERY+country:CN&num=N`）

与 `patent-google` learning 共用同一 DOM 结构（见其 notes/overview.md）：

- 结果容器: `search-result-item`，`innerText` 按行解析（标题 / `CN • 专利号 • 发明人 • 权利人` / 日期行 / 摘要）
- PDF: `a.pdfLink[href]`

## 搜索语法（CN 相关）

- `country:CN` 限定中国专利
- `assignee:华为` 限定申请人
- `inventor:任正非` 限定发明人
- `before:priority:20200101` / `after:priority:20200101` 限定优先权日
- `language:Chinese` 限定中文文献

## 注意

- 结果按 family 去重：个别结果可能显示同族最早的公开版本（非 CN 国家代码），属正常现象
- 专利号为 CN 格式（如 `CN110515732B`、`CN114526990A`），可直接用于 `patent_lookup` 等下游工具
