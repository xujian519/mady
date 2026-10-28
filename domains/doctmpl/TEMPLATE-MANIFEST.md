# 文档模板合并清单（T-W2-04）

真相源是 `templates/`（`go:embed` 编进二进制）。仓库根的 `doc-templates/` 是同内容镜像，
供桌面端浏览与用户参考；`$MADY_HOME/doc-templates/` 是运行时覆盖目录，同名模板以用户版本为准。

合并来源：

- **dsh**：deepseek-harness `packages/document/doc-template/assets/templates`（v1.0 的 17 份）。
- **Mady**：本仓库既有模板（Sati 对齐阶段落位的 `patent-report` 5 份与 `legal` 5 份为 Mady 独有）。

冲突处置只有两类：正文相同的 12 份按 dsh 的 frontmatter 合并（补 `language` / `formats` / `vars`，
Mady 正文保持不变），Mady 没有的 5 份通用文档直接引入。`doc-templates/*.example.md` 是渲染示例，
不是模板，不参与本清单。

| ID | 类别 | 目录 | 域 | 来源 | 处置 |
|---|---|---|---|---|---|
| apparatus-claim | claims | claims | patent | dsh | 元数据合并，正文一致 |
| method-claim | claims | claims | patent | dsh | 元数据合并，正文一致 |
| system-claim | claims | claims | patent | dsh | 元数据合并，正文一致 |
| simplified-disclosure | disclosure | disclosure | patent | dsh | 元数据合并，正文一致 |
| standard-9-section | disclosure | disclosure | patent | dsh | 元数据合并，正文一致 |
| clarity-amendment | oa-response | oa-response | patent | dsh | 元数据合并，正文一致 |
| inventiveness-defense | oa-response | oa-response | patent | dsh | 元数据合并，正文一致 |
| novelty-defense | oa-response | oa-response | patent | dsh | 元数据合并，正文一致 |
| chemical-spec | specification | specification | patent | dsh | 元数据合并，正文一致 |
| electrical-spec | specification | specification | patent | dsh | 元数据合并，正文一致 |
| mechanical-spec | specification | specification | patent | dsh | 元数据合并，正文一致 |
| software-spec | specification | specification | patent | dsh | 元数据合并，正文一致 |
| faq | document | document | document | dsh | 新增 |
| meeting-notes | document | document | document | dsh | 新增 |
| prd | document | document | document | dsh | 新增 |
| release-notes | document | document | document | dsh | 新增 |
| report | document | document | document | dsh | 新增 |
| case-analysis | legal | legal | legal | Mady | 独有 |
| contract-review | legal | legal | legal | Mady | 独有 |
| infringement-analysis | legal | legal | legal | Mady | 独有 |
| legal-opinion | legal | legal | legal | Mady | 独有 |
| statute-interpretation | legal | legal | legal | Mady | 独有 |
| claims-spec | patent-report | patent | patent | Mady | 独有 |
| invalidation-opinion | patent-report | patent | patent | Mady | 独有 |
| oa-response-sati | patent-report | patent | patent | Mady | 独有 |
| patentability-opinion | patent-report | patent | patent | Mady | 独有 |
| search-report | patent-report | patent | patent | Mady | 独有 |
