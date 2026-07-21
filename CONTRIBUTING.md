# 贡献指南

感谢你对 Mady 的关注！本文档将帮助你快速上手开发。

## 开发环境

### 前置要求

- **Go 1.26+**：Mady 使用 Go 1.26 的特性，请确保已安装正确版本。
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

推荐使用 Makefile 封装（覆盖根模块和 `tools/` 子模块）：

```bash
# 构建所有包（根模块 + tools/ 子模块）
make build

# 或使用原始的 go build（注意不会覆盖 tools/）
go build ./...
```

> **注意**：Mady 是 `go.work` 多模块结构。根目录执行 `go build ./...` 不会覆盖 `tools/` 子模块。
> 除非使用 Makefile，否则需要单独 `cd tools && go build ./...`。

### 运行测试

```bash
# 提交前标准（推荐）：lint + build + race 测试，覆盖根模块 + tools/
make verify

# 快速验证（日常开发）
make all       # vet + build + test（不含 race）

# 仅带竞态检测
make test-race

# 生成覆盖率报告（仅根模块，tools/ 需单独执行）
make coverage
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
│   ├── reasoning/    #   事实黑板、三段论、多跳遍历
│   └── rules/        #   YAML 规则引擎 + OA 解析 + 反套话引擎
├── graph/            # 图引擎（DAG + Pregel）
├── guardrails/       # 安全护栏
├── knowledge/        # 知识库（文档加载器、图谱、SQLite 读取层）
├── mcp/              # MCP 客户端
├── prompt/           # 提示词管理
├── protocol/         # JSON-RPC 协议原语
├── provider/         # LLM 提供者实现
├── psychological/    # 心理引擎（VAD/OCC/EMA/SDT/CBT）
├── retrieval/        # 检索基础设施（关键词/BM25/向量/RRF）
├── server/           # HTTP 服务器
├── session/          # 会话管理
├── skill/            # 技能加载器
├── skills/           # 内置技能定义
├── store/            # 快照存储
├── tools/            # 内置工具扩展（独立子模块，60 工具）
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
├── disclosure/       # 技术交底书分析管线（10 节点 Pregel）
├── memory/           # 长期记忆系统 + 策略学习型记忆编译器
├── filequeue/        # 文件队列基础设施
├── fuzzy/            # 模糊搜索
├── benchmark/        # 性能基准测试
├── integration/      # 端到端集成测试（5 条核心链路）
├── pkg/
│   ├── agentconfig/  #   统一 Provider/Model 配置层
│   └── util/         #   路径解析等通用工具
├── example/          # 示例应用
└── docs/             # 文档
```

### 分层架构

```
外部接口层：  A2A | A2UI | Server | AGUI | MCP | ACP
                        |
                   核心引擎层：agentcore
                 /      |       \         \
         提供者层(2)   工具层(35)    扩展层    领域扩展层
         chatcompat    tools/      Extension  psychological/
         smartrouter               接口       domains/
                                             guardrails/
                                             knowledge/
                                             retrieval/
                                             workflows/
                                             disclosure/
                                             memory/
                  \      |       /         /
          基础设施层：graph/ session/ skill/ prompt/ store/ mcp/ knowledge/graph
                          disclosure/ memory/ filequeue/ fuzzy/ benchmark/ integration/
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

## 规约驱动开发 (Spec-Driven Development)

本项目遵循两阶变更流程：

### 小型变更（Bug 修复、单函数调整）

口头描述或 Issue 描述 + Diff 审查即可，不走完整四步。

### 新功能 / 架构调整

在 `docs/specs/{feature-name}/` 下创建四个独立文档：

1. **01-proposal.md** — 背景、目标、成功标准（人写或人机共写，人必须过一遍）
2. **02-spec.md** — 输入输出、数据模型、接口定义、验证规则（AI 可初稿，人审）
3. **03-design.md** — 技术选型、架构图、关键算法、安全考量
4. **04-tasks.md** — 拆解为可执行的具体步骤，每步标注涉及文件范围

关键约定：
- 需求不明确时，AI 必须在 spec 中标记 `[NEEDS CLARIFICATION: ...]`，不自行假设
- 每份 Spec 必须有 **Human Owner** 字段，AI 参与撰写不改变人类最终负责
- 四步文档全部完成、人工 Sign-off 后再进入代码实现

## AI 变更日志

`docs/decisions/AI_CHANGELOG.md` 记录 AI 协助开发过程中的关键决策。
每个 AI 参与的功能变更必须在对应版本下追加记录，格式如下：

```
## YYYY-MM-DD feature-slug
- Decision: [做了什么设计决策]
- Reason: [为什么这么选择，而非其他方案]
- Risk: [已知风险或局限性]
- Human Owner: [负责人姓名]
- Spec: docs/specs/[feature]/ (如适用)
```

此文件不是可选项 —— 每次 AI 参与的功能变更都必须更新。

## AI 提交规范

AI 参与的提交除遵循 Conventional Commits 外，还需附加 Co-authored-by 标记：

```
feat: 添加专利新颖性分析引擎

Co-authored-by: Claude <noreply@anthropic.com>
```

## 代码审查分级

不是所有改动都需要同等审查强度：

| 层级 | 适用场景 | 审查要求 |
|------|----------|----------|
| L1 | 纯格式/文档/测试补充 | 常规 review，可 AI 自审 + 一人 approve |
| L2 | Bug 修复、单函数调整 | 至少一位非提交者的人工 review |
| L3 | 新功能、架构变更 | 强制人工 review，对照关联 Spec 做规范审查 |
| L4 | 触碰安全红线 | 强制人工 review，至少两人 approve，AI 不能作为唯一 approver |

安全红线包括：
- 密钥与凭证：禁止硬编码；禁止把真实 API Key 写进测试/示例代码
- 案件数据：禁止使用未脱敏的真实案件文件、当事人信息
- 沙箱边界：任何改动到 `tools/path.go`（resolvePathSandboxed）或 `domains/patent.go`（BuildProjectAgent）的代码，必须人工复核
- 护栏等级：任何降低 `guardrails.Level` 的改动必须在 PR 描述中说明理由
- Handoff 白名单：`AllowedSources` 的变更必须对照 Manifest 设计文档核实
- 免责声明与措辞：涉及护栏文案、报告结论措辞的改动，对照 `docs/tone-style-guide.md` 禁用词表

## 本地检查

### 首次克隆后安装钩子

```bash
# 注册 pre-commit 钩子（pre-commit 阶段：格式、vet 等）
pre-commit install

# 注册 commit-msg 钩子（commit-msg 阶段：敏感路径门禁 + Conventional Commits）
# 这一步容易遗漏——不装则本地提交不会触发 sensitive-paths gate 和 commitlint
pre-commit install --hook-type commit-msg
```

> `.git/hooks/` 不在版本控制内，所以每个新克隆的仓库都需要手动执行上述命令。
> 如未安装 commit-msg 钩子，本地 `git commit` 不会拦截"AI 参与 + 敏感路径"的违规组合，
> 只能等到 GitHub Actions CI 阶段才暴露。

### 提交前自检

```bash
# 运行所有 pre-commit 钩子（不区分阶段）
pre-commit run --all-files

# 模拟 commit-msg 阶段（用临时消息文件验证敏感路径门禁 + commitlint）
TMP=$(mktemp); printf 'feat: xxx\n' > "$TMP"
pre-commit run --hook-stage commit-msg --commit-msg-filename "$TMP"
rm -f "$TMP"

# 手动检查是否涉及安全敏感路径（需先 git add 暂存变更）
git add .
./scripts/check-sensitive-paths.sh
```

## 获取帮助

- 在 GitHub Issues 中提问
- 查阅 [README.md](README.md) 了解项目概览
- 查阅 `docs/` 目录下的设计文档
