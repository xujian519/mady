# TOOL_CONTRACT.md — Mady 内置工具 Schema 契约

> 本文档记录所有内置工具的稳定 Schema 契约。工具 Schema 变更必须同步更新本文档，
> 否则 CI 调和测试（`agentcore/contract_test.go`）将失败。
>
> 每个工具记录：名称、描述、分类（read/write/command/network/other）、关键参数。

## 文件操作工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| read | read | `path`, `offset`, `limit` | 读取文件内容 |
| write_file | write | `path`, `content` | 写入文件 |
| edit | write | `path`, `old_string`, `new_string` | 精确字符串替换编辑 |
| delete | write | `path` | 删除文件 |
| move | write | `source`, `destination` | 移动/重命名文件 |
| ls | read | `path` | 列出目录内容 |
| glob | read | `pattern`, `path` | glob 模式匹配文件 |
| grep | read | `pattern`, `path` | 内容搜索 |

## 命令执行

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| bash | command | `command`, `timeout` | 执行 shell 命令 |
| execute_code | command | `code`, `language` | 执行代码片段 |
| process | command | `pid` | 进程管理 |

## 网络工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| web_search | network | `query` | 网页搜索 |
| web_fetch | network | `url` | 获取网页内容 |
| browser | network | `action`, `url` | 浏览器控制 |

## 知识工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| search_knowledge | read | `query`, `domain` | 知识库检索 |
| add_document | write | `content`, `domain` | 添加知识文档 |
| add_file_to_knowledge | write | `path`, `domain` | 添加文件到知识库 |

## Git 工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| git_status | read | — | Git 状态查看 |
| git_diff | read | — | Git 差异查看 |
| git_log | read | `count` | Git 日志查看 |

## Agent 控制

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| ask | read | `question` | 向用户提问 |
| todo | write | `todos` | 任务列表管理 |
| task | read | — | 后台任务管理 |

## 视觉工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| computer_use | write | `action`, `coordinate` | 桌面控制 |
| vision | read | `image_path`, `prompt` | 图片分析 |

## 领域工具

| 工具 | 分类 | 关键参数 | 说明 |
|------|------|----------|------|
| search_rules | read | `query`, `domain` | 规则库检索 |
| get_article_framework | read | `statute` | 获取法条框架 |
| get_orchestration | read | `case_type` | 获取审查意见编排 |

---

> 最后更新: 2026-07-18 | 审计来源: docs/decisions/reasonix-analysis.md §9 P2
