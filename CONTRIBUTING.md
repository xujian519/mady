# 贡献指南

感谢你对 Mady 的关注！本文档将帮助你快速上手开发。

## 开发环境

### 前置要求

- **Go 1.25+**：Mady 使用 Go 1.25 的特性，请确保已安装正确版本。
- **Git**：用于版本控制。

### 克隆与设置

```bash
git clone https://github.com/xujian519/mady.git
cd mady
```

Mady 是一个 Go 多模块项目，使用 `go.work` 链接根模块和 `tools/` 子模块：

```bash
# go.work 已包含在仓库中，直接使用即可
go work sync
```

### 构建

```bash
# 构建所有包
go build ./...

# 构建 tools 子模块
cd tools && go build ./...
```

### 运行测试

```bash
# 运行所有测试
go test ./...

# 带竞态检测
go test -race ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

## 代码库结构

```
mady/
├── agentcore/        # 核心 Agent 运行时（LLM-工具循环、事件、钩子、压缩）
├── a2a/              # Agent-to-Agent 协议（Google A2A）
├── a2ui/             # Agent-to-UI 声明式协议
├── acp/              # Agent 通信协议（JSON-RPC）
├── agui/             # Agent GUI 事件协议（SSE）
├── domains/          # 领域 Agent 配置（Router/Chat/Patent/Legal）
├── graph/            # 图引擎（DAG + Pregel）
├── guardrails/       # 安全护栏
├── knowledge/        # 知识库（文档加载器、分块器）
├── mcp/              # MCP 客户端
├── prompt/           # 提示词管理
├── protocol/         # JSON-RPC 协议原语
├── provider/         # LLM 提供者实现
├── psychological/    # 心理引擎（VAD/OCC/EMA/SDT/CBT）
├── retrieval/        # 检索基础设施
├── server/           # HTTP 服务器
├── session/          # 会话管理
├── skill/            # 技能加载器
├── skills/           # 内置技能定义
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块）
├── tui/              # 终端 UI（8 层 Elm 架构）
│   ├── core/         #   基础层 (Layer 0)
│   ├── terminal/     #   终端 I/O (Layer 1)
│   ├── theme/        #   主题系统 (Layer 2)
│   ├── tui.go        #   引擎层 (Layer 3)
│   ├── component/    #   UI 组件 (Layer 4)
│   ├── chat/         #   聊天应用 (Layer 5)
│   ├── stdio/        #   过程式 I/O (Layer 6)
│   └── agentadapter/ #   Agent 适配器 (Layer 7)
├── workflow/         # 工作流原语
├── workflows/        # 领域工作流
├── example/          # 示例应用
└── docs/             # 文档
```

### 分层架构

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
        提供者层(2)   工具层(10+)   扩展层    领域扩展层
        chatcompat    tools/      Extension  psychological/
        smartrouter               接口       domains/
                                            guardrails/
                                            knowledge/
                                            retrieval/
                                            workflows/
                 \      |       /         /
         基础设施层：graph/ session/ skill/ prompt/ store/ mcp/ knowledge/graph
                                   |
                    TUI 层：8-layer Elm 架构
                                   |
                    应用入口：cmd/mady  server/  example/
```

## 编码规范

### Go 代码风格

- 使用 `gofmt` 格式化代码：`go fmt ./...`
- 通过 `go vet` 静态检查：`go vet ./...`
- 遵循 Go 标准命名约定（驼峰命名、首字母大小写控制可见性）
- 错误处理：始终检查并传播错误，使用 `fmt.Errorf("context: %w", err)` 包装

### 提交信息

遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/) 规范：

```
feat: 添加心理引擎 EMA 认知评价模块
fix: 修复上下文压缩时消息丢失问题
docs: 更新 README 安装说明
test: 添加 A2A 握手超时测试
refactor: 重构事件总线为泛型实现
chore: 更新 Go 依赖版本
```

### 添加新工具

1. 在 `tools/` 目录下创建工具实现文件
2. 在 `tools/tools.go` 的 `BuildTools()` 中注册
3. 编写 `*_test.go` 测试文件
4. 更新相关文档

### 添加新领域

1. 在 `domains/` 下创建领域配置文件
2. 实现领域 Agent 的 System Prompt
3. 在 `domains/router.go` 中注册
4. 在 `skills/` 下添加对应的 SKILL.md
5. 如果需要，在 `workflows/` 下创建工作流步骤

### 添加新技能

1. 在 `skills/<domain>/` 下创建 `SKILL.md` 文件
2. 包含 YAML 前置元数据（name, description, allowed-tools）
3. 编写清晰的使用说明和示例
4. 服务器会自动热重载技能

## PR 流程

1. **Fork 仓库**并创建功能分支：`git checkout -b feat/my-feature`
2. **编写代码**，确保通过 `go vet` 和 `go test`
3. **更新文档**：如有 API 变更，更新 README 或相关文档
4. **更新 CHANGELOG.md**：在 `[Unreleased]` 下记录变更
5. **提交 PR**：填写 PR 模板，描述变更内容
6. **代码审查**：维护者会审查代码，请耐心等待

## 代码审查关注点

- 是否遵循 Go 惯用写法
- 错误处理是否完整
- 是否添加了足够的测试
- 是否引入了不必要的依赖
- 文档是否同步更新
- 是否破坏了现有 API

## 获取帮助

- 在 GitHub Issues 中提问
- 查阅 [README.md](README.md) 了解项目概览
- 查阅 `docs/` 目录下的设计文档
