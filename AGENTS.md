# AGENTS.md

> 供所有 AI 代码助手（Claude Code / Cursor / Copilot / Codex 等）在启动时读取。
> Claude Code 用户：详见 `CLAUDE.md` 获取完整技术参考。

## 项目概览

Mady（中观智能体）：Go 1.25 编写的 Agent 运行时框架，服务于专利/法律专业领域智能体。
核心分层：agentcore（内核）→ 领域扩展层（psychological/guardrails/knowledge/retrieval/domains）
→ 基础设施层（graph/workflow/session/store）→ 应用入口（TUI/Server/A2A）。

## 构建与测试

- 构建：`go build ./...`
- 测试：`go test -race ./...`（并发相关代码必须带 -race）
- Lint：`golangci-lint run`
- 提交前必须三者全过，不接受"先跑通再说"

## 编码规范

- 遵循 Effective Go；导出符号必须有注释
- Domain 层不得 import Infrastructure 层的具体实现，只能依赖接口
  （对应 `docs/chat-assistant-architecture.md` 里反复强调的依赖倒置原则）
- 涉及 `guardrails/`、`psychological/`、Handoff 校验逻辑的改动，
  必须先读 `docs/chat-assistant-architecture.md` 了解已有契约设计

## 任务粒度

- 单次改动限定在 3-5 个文件内（"小炸弹不是大炸弹"），跨度更大的任务先拆解
- 不确定的地方标记 `[NEEDS CLARIFICATION]` 并暂停，不自行假设、不编造不存在的类型/接口

## 安全红线（AI 与人类共同遵守，详见 SECURITY.md）

- 禁止硬编码密钥、API Key、任何凭证
- 禁止在测试数据中使用真实案件文件、真实当事人信息
- 涉及 Checkpoint、护栏等级（guardrails.Level）、Handoff 白名单（AllowedSources）、
  WorkingDir 沙箱边界（tools/path.go）的改动，禁止未经人工审阅直接合入
- 涉及护栏文案、报告结论措辞的改动，对照 `docs/tone-style-guide.md` 的禁用词表

## 变更即记录

任何完成的功能改动，必须同步在 `docs/decisions/AI_CHANGELOG.md` 追加一条记录
（格式见该文件头部），不允许"写完代码就走人"

## 安全敏感路径

编辑以下路径时需额外谨慎，CI 将自动检测并标记：

| 路径 | 涉及的安全边界 |
|------|---------------|
| `agentcore/handoff.go` | 交接白名单校验（isHandoffAllowed） |
| `guardrails/levels.go` | 护栏等级枚举（Light/Standard/Strict） |
| `domains/router.go` | 路由白名单 AllowedSources |
| `domains/patent.go` | BuildProjectAgent 动态 WorkingDir |
| `domains/approval.go` | ApprovalGate 生命周期钩子 |
| `tools/path.go` | 文件系统沙箱隔离（resolvePathSandboxed） |
| `tools/tools.go` | 工具能力门控（ExtensionConfig） |
| `agentcore/manifest.go` | Manifest 校验规则 |
| `domains/project.go` | ValidateProjectPath 路径校验 |
| `tools/bash.go` | Bash 工具（非沙箱模式） |

## 现有参考文件

- `CLAUDE.md` — Claude Code 完整技术参考
- `CONTRIBUTING.md` — 人类贡献者指南
- `SECURITY.md` — 安全策略与漏洞报告
- `docs/tone-style-guide.md` — 面向用户文案风格规范
- `docs/chat-assistant-architecture.md` — Chat/Assistant 架构决策
- `docs/decisions/AI_CHANGELOG.md` — AI 决策变更日志
