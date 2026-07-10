# Agent Manifest 编写指南

Agent Manifest 是 Mady 的声明式 Agent 注册机制。通过编写 JSON 格式的 Manifest 文件，
可以将新的 Agent 注册到 Mady Router，无需修改 Go 代码。

## 文件位置

将 Manifest 文件放置在 `manifests/` 目录下（可通过 `MANIFEST_DIR` 环境变量自定义路径），
每个文件必须以 `.json` 为后缀名。

## 文件格式

```json
{
  "name": "agent-name",
  "domain": "chat",
  "description": "Agent 职责描述",
  "guardrail_level": "standard",
  "tools": ["web_search", "web_fetch"],
  "handoff_targets": ["chat-agent", "assistant-agent"],
  "knowledge_domain": "patent"
}
```

## 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | ✅ | Agent 唯一标识，匹配 `[a-z0-9-]+` 模式，最长 64 字符 |
| `domain` | ✅ | 功能领域：`chat` / `assistant` / `patent` / `legal` |
| `description` | ❌ | 职责描述，注入 Router SystemPrompt 和 HandoffConfig |
| `guardrail_level` | ❌ | 护栏等级：`light` / `standard` / `strict`（默认使用领域默认值） |
| `tools` | ❌ | 可用工具列表（仅记录，运行时行为由工厂函数决定） |
| `handoff_targets` | ❌ | 可委托的目标 Agent 名称列表 |
| `knowledge_domain` | ❌ | 知识检索领域 |

## 示例

### 聊天 Agent

```json
{
  "name": "chat-agent",
  "domain": "chat",
  "description": "日常聊天与情感陪伴。处理问候、闲聊、情绪支持等纯对话场景。",
  "guardrail_level": "light"
}
```

### 专利 Agent

```json
{
  "name": "patent-agent",
  "domain": "patent",
  "description": "专利代理与知识产权分析。处理专利检索、权利要求分析、新颖性比对。",
  "guardrail_level": "strict",
  "handoff_targets": ["chat-agent", "assistant-agent"],
  "knowledge_domain": "patent"
}
```

## 启动方式

```bash
# 默认使用 manifests/ 目录
mady serve

# 自定义 Manifest 目录
MANIFEST_DIR=/path/to/manifests mady serve

# TUI 模式（自动启用多域路由）
mady tui

# 强制单 Agent 模式绕过 Manifest
MADY_SINGLE_AGENT=1 mady tui
```

## 注意事项

- Manifest 是声明式契约，仅描述 Agent 身份和能力，运行时行为由 `domains/` 包中的工厂函数决定
- 未知的 `domain` 值会自动跳过（不报错）
- 多个文件声明相同 `name` 时，Router 会使用最后一个
