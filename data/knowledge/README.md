# 知识库数据目录

此目录用于存放 Mady 专利事务的知识库数据。将 Markdown 文件放入此目录后，
`knowledgeinit.InitPatentKnowledge()` 会在启动时自动加载并索引。

## 文件清单

| 文件名 | 域 | 说明 | 推荐来源 |
|--------|-----|------|---------|
| `patent-law-full.md` | `patent-law` | 专利法全文（现行版本） | 国家知识产权局官网 |
| `guidelines.md` | `guidelines` | 专利审查指南全文 | 国家知识产权局官网 |
| `ipc-classes.md` | `ipc` | IPC 分类表（核心部分） | WIPO 官网 |
| `invalidation-top100.md` | `invalidation` | 精选无效决定典型案例 | CNIPA 无效决定数据库 |

## 使用方法

1. 将上述 Markdown 文件放入此目录
2. 重启 Mady，知识库自动加载
3. Agent 启动后可通过 `patent_rag` 或 `rule_check` 工具检索

> 注意：文件不存在时 InitPatentKnowledge() 会静默跳过，不影响正常启动。
> 当前目录仅包含 README 占位文件，实际数据需要用户自行准备。
