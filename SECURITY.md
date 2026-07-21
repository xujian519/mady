# 安全策略

## 报告漏洞

Mady 项目非常重视安全问题。如果你发现了安全漏洞，请**不要**通过公开的 Issue 报告。

### 报告方式

请通过以下方式之一私下报告：

1. **GitHub Security Advisory**：使用 GitHub 的 [私有漏洞报告](https://github.com/xujian519/mady/security/advisories/new) 功能（推荐）
2. **邮件**：发送邮件至项目维护者

### 报告内容

请尽可能详细地描述漏洞，包括：

- 漏洞类型和影响范围
- 复现步骤
- 受影响的版本
- 可能的修复建议

### 处理流程

1. 收到报告后，维护者将在 **48 小时内** 确认收到
2. 维护者将在 **7 天内** 提供初步评估
3. 修复方案确定后，将通过安全公告发布
4. 修复发布后，将在 CHANGELOG 中记录（不包含利用细节）

## 安全最佳实践

### 环境变量

- **绝不**将 API 密钥（`API_KEY`、`DEEPSEEK_API_KEY` 等）提交到代码仓库
- 使用 `.env` 文件管理敏感配置，该文件已被 `.gitignore` 忽略
- 参考 `.env.example` 了解所需的环境变量

### 工具执行

- `bash` 和 `process` 工具应仅在受信任的沙箱环境中启用
- `computer_use`（macOS 桌面控制）需要明确的用户授权
- 在生产环境中关闭不必要的工具

### 护栏配置

Mady 提供三级护栏系统（`guardrails/`），每级包含关键词屏蔽、免责声明和审批门控：

| 级别 | 关键词屏蔽 | 免责声明 | 审批门 |
|------|-----------|---------|--------|
| **Light** | 通用风险关键词 | — | — |
| **Standard** | 专业风险关键词 | 领域免责声明 | — |
| **Strict** | 法律/专利关键词 | 法律免责声明 | 敏感结论需审批 |

此外，`guardrails/guardian/`（opt-in）提供 AI 安全审查子 Agent，内置熔断器（连续 3 次拒绝或 50 次窗口内 10 次拒绝时熔断）。

建议在生产环境中至少使用 `Standard` 级别。

### 权限门控（opt-in）

`agentcore/permission/` 提供细粒度权限系统，决策优先级为 deny > ask > allow > fallback。在 Strict 模式下与护栏配合使用。

### 安全敏感路径

以下文件涉及安全边界，修改时需额外审阅。**完整且唯一权威的敏感路径清单**
见 `scripts/check-sensitive-paths.sh` 的 `SENSITIVE_PATHS` 数组。
此处仅列代表性路径，具体条目以脚本为准。

- `agentcore/handoff.go` / `guardrails/levels.go` — 交接白名单与护栏等级
- `domains/router.go` / `domains/patent.go` / `domains/approval.go` / `domains/project.go` — 领域路由与安全边界
- `tools/path.go` / `tools/tools.go` / `tools/bash.go` / `tools/vision.go` — 沙箱、门控与执行
- `agentcore/manifest.go` / `agentcore/hooks.go` / `agentcore/permission/` — 内核校验、钩子与权限
- `disclosure/report.go` — review_gate 主动中断
- `guardrails/citation_gate.go` / `guardrails/citation_table.go` / `guardrails/guardian/` — 引用核验与 AI 熔断器
- `mcp/config_trust.go` — MCP 配置信任
- `acp/auth.go` — ACP 认证
- `server/server.go` — Agent 池引用计数

## 支持的版本

| 版本 | 支持状态 |
|------|---------|
| 0.x.x | ✅ 积极维护（当前 v0.3.0） |

## 致谢

我们会在安全公告中感谢负责任地报告漏洞的研究人员（除非你要求匿名）。

---

## 相关文档

- `docs/data-privacy-standards.md` — 数据处理与隐私规范
- `docs/GO-DEVELOPMENT-STANDARDS.md` — Go 开发安全规范（第 12 章）
- `docs/tone-style-guide.md` — 面向用户文案措辞规范
