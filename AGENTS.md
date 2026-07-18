# AGENTS.md

> 供所有 AI 代码助手（Claude Code / Cursor / Copilot / Codex 等）在启动时读取。
> Claude Code 用户：详见 `CLAUDE.md` 获取完整技术参考。

## 项目概览

Mady（中观智能体）：Go 1.25 编写的 Agent 运行时框架，服务于专利/法律专业领域智能体。
核心分层：agentcore（内核，含 doomloop/reasoning_strategy/atom/plugin/evaluate）
→ 领域扩展层（psychological/guardrails/knowledge/retrieval/domains/rules/doctmpl）
→ 基础设施层（graph/workflow/session/store/memory/disclosure/fuzzy/filequeue/prompt）
→ 协议与接口层（A2A/A2UI/AGUI/ACP/Server/MCP/TUI）
→ 应用入口（cmd/mady, example/）。
841 个 Go 源文件（558 非测试 + 283 测试），~185K 行代码。

## 构建与测试

- 构建：`go build ./...`
- 测试：`go test -race ./...`（并发相关代码必须带 -race）
- Lint：`golangci-lint run`
- 提交前必须三者全过，不接受"先跑通再说"
- 常用快捷命令见 `Makefile`：`make all`（vet+build+test）、`make test-race`、
  `make lint`、`make build-mady`、`make run-mady`（TUI 入口 `cmd/mady/`）

### ⚠️ 多模块工作区（重要 gotcha）

本仓库是 `go.work` 多模块结构：根模块 `.` + 独立子模块 `./tools`（各有自己的 `go.mod`）。
- 根目录执行 `go build/test/vet ./...` **不会**覆盖 `tools/` 模块
- 根模块通过 `replace github.com/xujian519/mady => ../` 引用 tools，反之亦然
- 对 `tools/` 的改动须单独 `cd tools && go build ./... && go test ./...`，
  或用 `make lint` / `make fmt`（Makefile 已封装 `cd tools && ...` 分支）

### 提交规范

- pre-commit hooks 已配置（`.pre-commit-config.yaml`）：trailing-whitespace、
  end-of-file-fixer、gofmt、goimports、go vet
- commit-msg hook 强制 **Conventional Commits**（commitlint + config-conventional），
  提交信息须形如 `feat:` / `fix:` / `docs:` 等，否则会被拒
- 注意：`go-imports` hook 配置了绝对路径 `/Users/xujian/go/bin/goimports`，
  换机器需重装 goimports 并调整该路径

### ⚠️ 资源定位：任意目录运行（重要）

`mady` 支持在**任意 cwd** 下启动，所有资源定位不依赖工作目录：

- **manifest**：4 个内置领域定义通过 `go:embed` 编进二进制（`agentcore/manifests/`），
  开箱即用。`$MADY_HOME/manifests/` 放同名 JSON 可覆盖、放新文件可新增领域（无需重编译）。
  合并逻辑见 `agentcore.LoadManifests`。
- **应用数据根目录**：`pkg/util.MadyHome()` 统一解析，优先级 `$MADY_HOME` > `~/.mady`。
  workspace（案件/AgentStore）、sessions 均落在其下。
- **改造前**：manifest/workspace/assistant WorkingDir 都硬编码 `./` 相对路径，
  非项目根目录启动会静默降级为裸 LLM 对话（`defaultSystemPrompt` 单 Agent 模式）。
- **改造后**：入口 `cmd/mady/main.go` 的 `setupFrameworkContext()` 统一解析路径，
  涉及路径的改动请复用 `util.MadyHome()` / `util.ResolveDataDir()`，禁止新增 `./` 相对路径默认值。

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

编辑以下路径时需额外谨慎，CI 将自动检测并标记。本表与 `scripts/check-sensitive-paths.sh`
的 `SENSITIVE_PATHS` 数组保持同步，后者为权威源。

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
| `agentcore/hooks.go` | LifecycleHook 运行时注册与优先级 |
| `disclosure/report.go` | review_gate 主动中断（Pregel 内中断信号） |
| `guardrails/citation_gate.go` | 引用核验门（双级核验判定矩阵） |
| `guardrails/citation_table.go` | 静态主题收录口径与漂移控制 |
| `mcp/config_trust.go` | MCP 配置信任存储（.mcp.json 命令执行） |
| `acp/auth.go` | ACP 认证（TokenAuthProvider 常量时间比较） |
| `server/server.go` | Agent 池引用计数（use-after-free 防护） |
| `tools/vision.go` | 视觉工具沙箱字段传播（历史沙箱绕过修复点） |
| `agentcore/permission/` | 权限决策（Allow/Ask/Deny） |
| `guardrails/guardian/` | Guardian AI 熔断器 |

## 现有参考文件

- `CLAUDE.md` — Claude Code 完整技术参考
- `CONTRIBUTING.md` — 人类贡献者指南
- `SECURITY.md` — 安全策略与漏洞报告
- `docs/tone-style-guide.md` — 面向用户文案风格规范
- `docs/chat-assistant-architecture.md` — Chat/Assistant 架构决策
- `docs/decisions/AI_CHANGELOG.md` — AI 决策变更日志
