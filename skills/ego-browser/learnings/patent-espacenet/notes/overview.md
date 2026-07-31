# Espacenet Overview

## 搜索页（`https://worldwide.espacenet.com/patent/search?q=...`）

- 结果异步 XHR 加载，需滚动触发渲染
- 结果容器: `article[data-qa="result_resultList"]`（类名带 webpack contenthash 后缀，**不可**依赖）
- 标题: `span[lang="en"]`（class 含 hash，`lang` 属性稳定）
- 副标题行: `[class*="subtitle"]`，格式 `CN107891199A (B) • 2018-04-10 • {Applicant}`
- 申请人: `[title="Applicant"] span`
- 摘要: `[class*="abstract"]`
- 结果项内**无链接**（点击行为由事件驱动），详情页 URL 需构造：
  `https://worldwide.espacenet.com/publicationDetails/biblio?CC=CN&NR=107891199A&KC=A`

## 详情页（publicationDetails/biblio）

- 标题: `[data-qa="biblio_title"]` 或 `h1`
- 公开号: `[data-qa="publicationNumber"]`
- 申请人: `[data-qa="applicant"]`
- 发明人: `[data-qa="inventor"]`
- 摘要: `[data-qa="abstract"]`

## 搜索语法

- 支持 smart search：`ta("华为")` 申请人、`ti("图像识别")` 标题、`ab("深度学习")` 摘要、
  `pd within "2018-2020"` 公开日期、`in("张三")` 发明人
- 网页搜索框默认 smart search 语法
