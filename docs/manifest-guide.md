# Agent Manifest 编写指南

Agent Manifest 是 Mady 的声明式 Agent 注册机制。通过编写 JSON 格式的 Manifest 文件，
可以将新的 Agent 注册到 Mady Router，无需修改 Go 代码。

## 文件位置

4 个内置领域 manifest（chat/assistant/patent/legal）通过 `go:embed` 编进二进制，**无需额外资源文件**即可在任意目录启动。

要覆盖内置 manifest 或新增领域，将 `.json` 文件放入 `$MADY_HOME/manifests/` 目录（默认 `~/.mady/manifests/`，可通过 `MANIFEST_DIR` 环境变量自定义）。加载顺序：内置 → 外部覆盖（同名外部文件优先）。

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
# 内置 4 个领域 manifest 始终可用，无需任何外部文件
mady serve
mady tui

# 自定义 manifest 覆盖/扩展目录（默认 $MADY_HOME/manifests/）
MANIFEST_DIR=/path/to/manifests mady serve
# 或通过 MADY_HOME 统一管理
MADY_HOME=/path/to/madyhome mady serve

# TUI 模式（自动启用多域路由）
mady tui

# 强制单 Agent 模式绕过 Manifest
MADY_SINGLE_AGENT=1 mady tui
```

## 注意事项

- Manifest 是声明式契约，仅描述 Agent 身份和能力，运行时行为由 `domains/` 包中的工厂函数决定
- 未知的 `domain` 值会自动跳过（不报错）
- 多个文件声明相同 `name` 时，Router 会使用最后一个
