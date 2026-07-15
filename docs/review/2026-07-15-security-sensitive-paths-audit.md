# 安全敏感路径审计报告

> 审计日期：2026-07-15
> 对应规范：`docs/GO-DEVELOPMENT-STANDARDS.md` 第 12 章
> 审查等级：L4（红色安全红线）

---

## 审计结果总览

| # | 路径 | 状态 | 风险等级 |
|---|------|------|---------|
| 1 | `agentcore/handoff.go` | ✅ 合格 | — |
| 2 | `guardrails/levels.go` | ✅ 合格 | — |
| 3 | `domains/router.go` | ✅ 合格 | — |
| 4 | `domains/approval.go` | ✅ 合格 | — |
| 5 | `tools/path.go` | ✅ 合格 | — |
| 6 | `tools/tools.go` | ✅ 合格 | — |
| 7 | `agentcore/manifest.go` | ⚠️ 建议改进 | 低 |
| 8 | `domains/project.go` | ✅ 合格 | — |
| 9 | `tools/bash.go` | ✅ 合格 | — |

---

## 1. `agentcore/handoff.go` — isHandoffAllowed 白名单校验

**状态：✅ 合格**

**发现**：
- `isHandoffAllowed()` 实现 default-deny：`AllowedSources` 为空 → 拒绝（第 310 行）
- `"*"` 通配符支持，但仅在 Chat Agent 的 `AllowedSources = ["*"]` 中使用（合理，Chat 是最低风险领域）
- Transfer 模式在 `handleTransfer()`（第 177 行）有 belt-and-suspenders 二次校验
- 所有专业领域（Patent/Legal/Assistant）的 `AllowedSources` 均为 `["mady-router", "chat-agent"]`，不使用通配符
- FallbackMsg 对用户友好，不暴露内部错误

**建议**：无。

---

## 2. `guardrails/levels.go` — 护栏等级枚举

**状态：✅ 合格**

**发现**：
- 三级枚举定义清晰（`LevelLight` / `LevelStandard` / `LevelStrict`），含 iota 注释
- `RegisterLevel()` / `RegisteredLevel()` 支持扩展，线程安全（`sync.RWMutex`）
- 注册时自动同步到 `agentcore.RegisterValidGuardrailLevel()`，保证 manifest 联动
- 内置三挡配置：BlockedPhrases（全等级）、Disclaimer（Standard+）、SuppressPersist（Strict+）
- 零值安全：`New()` 默认 LevelLight + 基础 BlockedPhrases

**建议**：无。

---

## 3. `domains/router.go` — AllowedSources 白名单

**状态：✅ 合格**

**发现**：
- `ProfessionalHandoffConfigs()` 明确注释声明白名单扩展需要安全审阅
- 所有专业领域 `AllowedSources` 不使用 `"*"` 通配符
- Chat 领域使用 `"*"` 合理（任何 Agent 都可交回聊天）
- `RouterConfigFromManifests()` 与 `ProfessionalHandoffConfigs` 白名单对齐
- `RouterConfigWithRegistry()` 中案件 Handoff 白名单为 `["mady-router", "chat-agent"]`，与专业领域白名单保持一致，不使用通配符

**建议**：无。

---

## 4. `domains/approval.go` — ApprovalGate 生命周期

**状态：✅ 合格**

**发现**：
- `ApprovalGate` 实现 `LifecycleHook.AfterModelCall`，在 LLM 输出后拦截
- 关键词触发制，不可 bypass（除非 `SkipIfNoTools = true`）
- `RequireApproval()` 返回 `InterruptError` 暂停执行，不自动通过
- `ApprovalRecord` 记录完整审批链路（ID/Session/关键词/决策/修改/反馈）
- `ApprovalStore` 接口支持持久化（`MemoryApprovalStore` + SQLite 后端）
- 内置状态机：`adopted` / `modified` / `rejected`

**建议**：无。

---

## 5. `tools/path.go` — 沙箱路径隔离

**状态：✅ 合格**

**发现**：
- `resolvePathSandboxed()` 实现完整沙箱：用户路径 → 解析 → 绝对路径 → NFD 标准化 → 符号链接解析 → 子树校验
- TOCTOU 防护：`pinPath()` → `verifyOpenedInode()` 对比 inode 防止 symlink swap 攻击
- `OpenSandboxed()` / `readFileSandboxed()` 通过文件描述符 pin inode
- Sandbox 关闭时打印 WARNING 日志（`sync.Once` 确保只打印一次）
- macOS NFD 兼容处理

**建议**：无。

---

## 6. `tools/tools.go` — 工具能力门控

**状态：✅ 合格**

**发现**：
- 双层门控：`EnabledTools`（正向允许列表）+ `DisableTools`（负向排除列表）
- Sandbox 配置自动传播到所有文件工具（Read/Edit/Write/Patch/Delete/Move/Ls/Grep/Find/Glob/View/Bash）
- 编译期接口检查：`var _ agentcore.Extension = (*Extension)(nil)`
- 工具名使用常量（`ToolBash`/`ToolGitStatus` 等），非裸字符串，工具重命名时编译期安全
- `ReadOnly` 标记声明式副作用分类

**建议**：无。

---

## 7. `agentcore/manifest.go` — Manifest 校验

**状态：⚠️ 建议改进（低风险）**

**发现**：
- `ValidateManifest()` 检查 name 格式（正则 `^[a-z0-9]+(-[a-z0-9]+)*$`，最长 64 字符）
- Domain 校验：`validDomains` 映射（chat/assistant/patent/legal）
- GuardrailLevel 校验：`validGuardrailLevels` 映射（light/standard/strict）
- `RegisterValidDomain()` / `RegisterValidGuardrailLevel()` 支持扩展，线程安全

**建议改进**：
- Manifest 的 `GuardrailLevel` 字段没有 fallback 到领域默认值：空字符串通过校验，但可能导致护栏不生效。建议在 `ValidateManifest` 中为空时注入领域默认等级。
- `HandoffTargets` 和 `Tools` 字段目前只声明不校验（manifest 第 32-34 行）。虽不是问题但可记录为待增强项（需要运行时信息）。

---

## 8. `domains/project.go` — ValidateProjectPath

**状态：✅ 合格**

**发现**：
- `ValidateProjectPath()` 执行三重校验：`Abs()` → `EvalSymlinks()` → `Stat()` + `IsDir()`
- 符号链接解析防止 `link_to_dir -> /etc` 绕过
- `Register()` 在加锁状态下调用 `ValidateProjectPath` 前置校验
- `RefreshStatus()` 解锁后 I/O 再更新锁，避免长时间持锁
- 路径清理（`filepath.Clean`）防止重复注册变体路径

**建议**：无。

---

## 9. `tools/bash.go` — 非沙箱模式安全警告

**状态：✅ 合格**

**发现**：
- `DangerousPatterns` 防御性深度过滤：危险命令模式匹配（如 `rm -rf /`）
- Sandbox 传播：`BashToolConfig.Sandbox` 从 `ExtensionConfig` 自动注入
- 超长输出自动溢出到临时文件（`os.CreateTemp`，使用系统临时目录）
- 进程组清理在 `bash_kill.go` 中实现（`killProcessTree`，校验 PID）

**建议**：无。

---

## 综合结论

**所有 9 条敏感路径均通过审计，未发现严重安全问题。**

- ✅ 8 条路径状态为"合格"
- ⚠️ 1 条路径（`manifest.go`）有低风险改进建议
- 无 🔴 严重问题

安全设计亮点：
1. **Default-deny 安全模型**：Handoff 白名单为空时默认拒绝
2. **TOCTOU 防护**：path.go 使用 inode 绑定防止路径检查后文件被替换
3. **Belt-and-suspenders**：Transfer 模式在 createHandoffTool 和 handleTransfer 两处独立校验
4. **Layered defense**：bash 工具同时有 Sandbox + DangerousPatterns + DisableTools 三层防护
5. **白名单一致性**：所有专业领域与案件 Agent 的 AllowedSources 统一为 `["mady-router", "chat-agent"]`（无通配符）；仅 Chat Agent 使用 `"*"` 允许任意来源交回聊天
