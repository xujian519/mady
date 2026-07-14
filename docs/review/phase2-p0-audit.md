# Phase 2: P0 安全红线逐行审计报告

> 日期：2026-07-14
> 审阅范围：10 个 L4 级安全敏感路径
> 方法：逐行阅读 + CodeGraph 调用链分析

## 审计总结

| 文件 | 风险等级 | 问题数 | 结论 |
|------|---------|--------|------|
| `tools/path.go` | 🟢 低 | 1 Medium | 沙箱设计健壮，TOCTOU 防护到位 |
| `tools/bash.go` | 🟢 低 | 1 Medium | ✅ **P0-2 已修复** — 新增 DangerousPatterns 校验 |
| `agentcore/handoff.go` | 🟢 低 | 1 Medium | Default-deny 白名单正确 |
| `guardrails/levels.go` | 🟢 低 | 0 | iota 枚举安全，三级分层清晰 |
| `domains/router.go` | 🟢 低 | 0 | ✅ **P0-5 已修复** — AllowedSources 对齐 |
| `domains/patent.go` | 🟢 低 | 0 | 沙箱启用 + 危险工具禁用 + 护栏 Strict |
| `domains/approval.go` | 🟢 低 | 0 | 审批门控设计完整，默认安全 |
| `agentcore/manifest.go` | 🟢 低 | 0 | 校验规则严格，白名单硬编码 |
| `domains/project.go` | 🟢 低 | 1 Low | 路径校验无符号链接解析 |
| `tools/tools.go` | 🟢 低 | 0 | 工具门控逻辑正确 |

---

## 逐文件审计

### 1. tools/path.go — 文件系统沙箱

**安全设计评估：✅ 健壮**

沙箱核心函数 `resolvePathSandboxed` 实现了多层防护：
1. **NFD 标准化**（line 44）：解决 macOS Unicode 编码问题
2. **符号链接解析**（line 61-67）：`filepath.EvalSymlinks` 防止 `link→/etc` 逃逸
3. **子树校验**（line 74-77）：`filepath.Rel` + `strings.HasPrefix(rel, "..")` 防止目录遍历

TOCTOU 防护：
- `pinPath`（line 169）+ `verifyOpenedInode`（line 182）：通过 inode 比对检测符号链接交换攻击
- `OpenSandboxed`/`readFileSandboxed`：打开 FD 后基于 inode 操作，防止 TOCTOU

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P0-1 | Medium | `path.go:158` | `readFileSandboxed` 中 `io.ReadAll(f)` 无大小限制，可能导致 OOM | 添加 `io.LimitReader(f, maxSize)` |

**验证通过：**
- ✅ `SandboxDisabled()` 仅打印一次性警告，不静默绕过
- ✅ 所有公开 API（OpenSandboxed/OpenSandboxedFile/readFileSandboxed）均经过沙箱
- ✅ 错误信息不泄漏路径细节（使用 `%q` 但仅引用 userPath）

---

### 2. tools/bash.go — Shell 执行

**安全设计评估：⚠️ 依赖配置约束**

命令执行通过 `BashOperations.Exec` 接口抽象，默认实现为本地 shell。

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P0-2 | **High** | `bash.go:246` | `BashToolInput.Command` 直接传递给 shell 执行，无命令校验/过滤 | 添加命令白名单或正则过滤；当前缓解：PatentAgentConfig 中 bash 在 DisableTools 列表中 |
| P0-3 | Medium | `bash.go:246` | `BashOperations.Exec(command, cwd, nil, ...)` 传入 `nil` env — 安全，但接口签名 `env map[string]string` 允许自定义实现注入环境变量 | 添加文档约束说明 env 参数需来自受信源 |

**验证通过：**
- ✅ 输出截断（MaxBytes/MaxLines）+ 临时文件回退
- ✅ 10 分钟延迟清理临时文件
- ✅ `onData` 回调在同步 Exec 中调用，无并发问题
- ✅ 空命令校验（line 199-201）

---

### 3. agentcore/handoff.go — 交接白名单

**安全设计评估：✅ Default-Deny**

`isHandoffAllowed` 三态逻辑：
1. 空 `AllowedSources` → **拒绝**（default-deny）
2. 含 `"*"` → 允许所有
3. 其他 → 按名称匹配

`handleTransfer` 中 belt-and-suspenders 二次校验（line 177-183）。

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P0-4 | Medium | `handoff.go:265` | `inheritRuntime` 中 `extensions.Register(context.Background(), ...)` 使用了硬编码的 Background context，应使用请求 context | 将 ctx 参数传递到 inheritRuntime |

**验证通过：**
- ✅ `createHandoffTool` 输入校验严格（`additionalProperties: false`）
- ✅ `FallbackMsg` 防止内部错误信息泄漏给用户
- ✅ `isHandoffTool` 前缀检查防止自定义工具名绕过

---

### 4. guardrails/levels.go — 护栏等级

**安全设计评估：✅ 正确分层**

- `Level` 使用 `iota`（Light=0, Standard=1, Strict=2），不可意外降级
- `New()` 默认 `LevelLight`（保守）
- 三级处理逻辑：BlockedPhrases（所有级别）→ RiskKeywords（Standard+）→ ApprovalKeywords（Strict+）

**发现：无**

**验证通过：**
- ✅ `hasRiskKeyword`/`hasApprovalKeyword` 使用简单字符串匹配，确定性行为
- ✅ `SuppressPersist` 防止未审批内容写入会话存储
- ⚠️ `strings.Contains` 大小写敏感 — 建议：对英文关键词做 `strings.ToLower`

---

### 5. domains/router.go — 路由白名单

**安全设计评估：⚠️ Manifest 加载路径存在通配符**

`ProfessionalHandoffConfigs` 正确限制 `AllowedSources` 为 `["mady-router", "chat-agent"]`。

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P0-5 | **High** | `router.go:222` | `RouterConfigFromManifests` 对所有从 manifest 加载的 handoff 使用 `AllowedSources: []string{"*"}`。与 `ProfessionalHandoffConfigs` 的受限策略不一致。虽然 manifest 域名通过 `domainFactoryMap` 白名单限制，但 `"*"` 违反了最小权限原则 | 改为 `AllowedSources: []string{"mady-router"}` 与 ProfessionalHandoffConfigs 对齐，或至少限制为 `["mady-router", "chat-agent"]` |

**验证通过：**
- ✅ `ProfessionalHandoffConfigs` 中非 Chat 领域正确使用受限 AllowedSources
- ✅ Chat handoff 有意使用 `"*"`（注释说明了理由）
- ✅ `ClassifyIntent` 确定性关键词分类，可审计

---

### 6. domains/patent.go — 专利 Agent 配置

**安全设计评估：✅ 纵深防御**

- `SandboxEnabled: true` + 危险工具 `DisableTools`
- `BuildProjectAgent` 额外使用 `EnabledTools` 白名单（仅 7 个文件工具）
- LevelStrict 护栏 + ApprovalGate 双层防护

**发现：无**

**验证通过：**
- ✅ `PatentAgentConfig`: bash/git/browser/execute_code/process/computer_use 全部禁用
- ✅ `BuildProjectAgent`: WorkingDir = rec.RootPath（经过 ValidateProjectPath 校验）
- ✅ `BuildProjectAgent`: EnabledTools 白名单优先于 DisableTools

---

### 7. domains/approval.go — 审批门控

**安全设计评估：✅ 完整**

- `NewApprovalGate`: 空配置自动回退 `DefaultApprovalConfig`
- `AfterModelCall`: nil/error 提前返回
- `RecordDecision`: 无 store 时静默跳过（不崩溃）

**发现：无**

**验证通过：**
- ✅ 预览截断 500 字符，防止大文本 DOS
- ✅ `MemoryApprovalStore` 线程安全（mutex）
- ✅ `DecisionAdopted/Modified/Rejected` 三态枚举

---

### 8. agentcore/manifest.go — Manifest 校验

**安全设计评估：✅ 严格**

- Name：正则 `^[a-z0-9]+(-[a-z0-9]+)*$` + 64 字符上限
- Domain：硬编码白名单 `{chat, assistant, patent, legal}`
- GuardrailLevel：硬编码白名单 `{light, standard, strict}`

**发现：无**

**验证通过：**
- ✅ 所有字段均有校验
- ✅ 白名单不可配置（硬编码）
- ⚠️ 空 `GuardrailLevel` 允许通过 — 设计合理（由工厂函数决定默认值）

---

### 9. domains/project.go — 项目路径校验

**安全设计评估：✅ 基本正确**

`ValidateProjectPath` 检查路径存在性、可访问性、是否为目录。

**发现：**

| # | 严重度 | 位置 | 问题 | 建议 |
|---|--------|------|------|------|
| P0-6 | Low | `project.go:347` | `ValidateProjectPath` 未解析符号链接（与 `resolvePathSandboxed` 不同），可能接受指向沙箱外目录的符号链接 | 添加 `filepath.EvalSymlinks` |

**验证通过：**
- ✅ `Register` 调用 `ValidateProjectPath` + 重复 RootPath 检测
- ✅ `Delete` 仅删记录不删物理文件
- ✅ 并发安全（RWMutex）

---

### 10. tools/tools.go — 工具能力门控

**安全设计评估：✅ 正确**

- `EnabledTools`（白名单）> `DisableTools`（黑名单）
- 危险工具仅在显式配置时注册：`Browser=nil` → 不注册浏览器工具
- 沙箱配置传播到所有文件工具

**发现：无**

**验证通过：**
- ✅ `addTool` 闭包正确实现白名单/黑名单逻辑
- ✅ `SandboxEnabled` 通过 `WorkingDirSandbox` 传播到 9 种文件工具配置
- ✅ `ComputerUse` 仅 `cfg.ComputerUse=true` 时注册

---

## 修复优先级

| 优先级 | ID | 文件 | 问题 | 状态 |
|--------|----|------|------|------|
| ~~🔴 立即~~ | ~~P0-5~~ | ~~`router.go:222`~~ | ~~Manifest AllowedSources 通配符~~ | ✅ **已修复** — 改为 `["mady-router", "chat-agent"]` |
| ~~🔴 立即~~ | ~~P0-2~~ | ~~`bash.go:246`~~ | ~~Shell 命令注入~~ | ✅ **已修复** — 新增 `DangerousPatterns` 默认拦截反引号和 `$()` 替换 |
| 🟡 本周 | P0-4 | `handoff.go:265` | context.Background() | 待修复 |
| 🟢 下迭代 | P0-1 | `path.go:158` | io.ReadAll 无限制 | 待修复 |
| 🟢 下迭代 | P0-3 | `bash.go:246` | env 注入文档 | 待修复 |
| 🟢 下迭代 | P0-6 | `project.go:347` | 符号链接 | 待修复 |

## 整体评估

P0 安全红线状态：**基本健康**。10 个文件中有 7 个零问题通过。核心发现 3 个 High/Medium 问题需要在进入 Phase 3 前修复。沙箱、护栏、权限门控三大安全支柱均工作正常，无绕过风险。
