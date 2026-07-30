# Mady 安全与数据隐私审查报告

> 审查日期：2026-07-31
> 审查范围：Go 源码 + Desktop 前端 (React/TypeScript)
> 审查依据：SECURITY.md · GO-DEVELOPMENT-STANDARDS.md 第12章 · data-privacy-standards.md · check-sensitive-paths.sh

---

## 总体评级

| 维度 | 评级 | 说明 |
|------|------|------|
| **P0 — 数据隐私** | ⚠️ 告警 | 无 PII 脱敏机制，LLM 出站可能泄漏用户信息 |
| **P0 — 敏感路径** | ✅ 可控 | 频繁变更但均为重构/功能添加，trust store 和 sandbox 持续增强 |
| **P1 — 基础设施** | ✅ 通过 | Sandbox 实现扎实，前端 CSP 到位，ACP 认证使用常量时间比较 |
| **P2 — 代码质量** | ⚠️ 建议 | gosec 103 处标记大多合理，memory 包有遗留风险 |
| **综合评级** | **⚠️ 中等风险** | 核心框架设计安全意识强，但 LLM 出站数据无脱敏是最严重的缺口 |

---

## P0-1: LLM 出站 PII 路径

### ❌ 不通过 — 无 PII 脱敏机制

#### 出站路径清单（已确认 25 条）

所有出站调用均通过 `agentcore.Provider` 接口的 `Complete()` / `Stream()` 方法，底层统一由 `provider/chatcompat/chat.go` 的 HTTP 调用实现。

| # | 文件 | 行号 | 调用方 | 数据来源 | 是否脱敏 |
|---|------|------|--------|---------|---------|
| 1 | `agentcore/agent_run_phase.go` | 59-66 | `runModelTurn` | `a.state.messagesReadOnly()` — 全量对话历史 | ❌ |
| 2 | `agentcore/orchestrate.go` | - | `callModel` | Agent state messages | ❌ |
| 3 | `provider/chatcompat/chat.go` | 317 | `Provider.Complete` | `ProviderRequest.Messages` — 透传上游 | ❌ |
| 4 | `provider/chatcompat/chat.go` | 379 | `Provider.Stream` | 同上 | ❌ |
| 5 | `evaluate/cli/cli.go` | 291 | Eval runner | 测试 prompt + 黄金答案 | ❌ |
| 6 | `evaluate/benchmark/live_agent_test.go` | 492 | Benchmark | 测试数据 | ❌ |
| 7 | `guardrails/guardian/guardian.go` | 109 | AI 审查 Agent | 对话内容 | ❌ |
| 8 | `domains/claimdrafting/provider_adapter.go` | 34 | CLA 撰写 | 用户 prompt | ❌ |
| 9 | `domains/claimdrafting/drafter.go` | 65 | LLM Drafter | 专利分析 prompt | ❌ |
| 10 | `tools/vision.go` | 338 | Vision Provider | 图片 base64 + prompt | ❌ |
| 11 | `tools/vision.go` | 205 | `DefaultVisionOperations.Analyze` | HTTP API call to LLM | ❌ |
| 12 | `memory/extractor_llm.go` | 62 | LLM 记忆提取 | 对话内容 | ❌ |
| 13 | `memory/extractor_llm.go` | 90 | LLM 情绪提取 | 对话内容 | ❌ |
| 14 | `memory/session_summarizer.go` | 56 | 会话摘要 | Session 层记忆 | ❌ |
| 15 | `memory/dedup_llm.go` | 54 | 记忆去重 | 记忆内容 | ❌ |
| 16 | `domains/reasoning/provider_adapter.go` | 35 | 三段论检查 | 推理事实 | ❌ |
| 17-25 | `provider/chatcompat/*_test.go` | 多行 | 单元测试 | 合成数据（测试可豁免） | ✅ 测试 |

#### 关键问题

1. **零脱敏基础设施** — 整个代码库仅有 1 处与脱敏相关的引用（`pkg/agentconfig/load.go:16` 的注释 `Sanitize file path`），无任何 PII 检测/脱敏/替换逻辑。

2. **消息透传** — `agentcore/agent_run_tool.go:226` 的 `buildRequestMessages()` 直接从 `a.state.messagesReadOnly()` 读取用户原始输入，经由 `DefaultConvertToLLM()` 过滤非标准消息类型，但**不修改内容本身**。

3. **记忆提取也含对话原文** — `memory/extractor_llm.go:62` 将 `"用户说: {userInput}\n助手回答: {assistantOutput}"` 直接作为 LLM 提取 prompt，其中 `userInput` 可能包含当事人姓名、案件号等 PII。

4. **legacy resolveModelName 硬编码默认模型** — `provider/chatcompat/chat.go:537` 的 `resolveModelName` 在 `PROVIDER` 为空时默认返回 `"deepseek-v4-flash"`，所有与模型相关的敏感信息（包括 API key）可能因此发送到第三方。

#### 修复建议（P0 优先级）

1. 在 `agentcore/agent_run_tool.go` 的 `buildRequestMessages()` 中添加 PII 脱敏步骤：
   - 实现 `MessageSanitizer` 接口（检测身份证号、手机号、邮箱、银行账号、企业税号等）
   - 替换为 `[已脱敏]` 占位符
   - 或在 `ConvertToLLM` 链中增加脱敏步骤

2. 为 `memory/extractor_llm.go` 和 `memory/session_summarizer.go` 添加输入脱敏

3. 在 `provider/chatcompat/chat.go` 的 `buildRequest` 方法中添加可选的 `SanitizeMessages` 钩子

---

## P0-2: Memory 跨项目泄漏风险

### ⚠️ 告警 — 作用域隔离模型正确但 SearchAllLayers 绕过了隔离

#### 作用域模型（设计正确 ✅）

```
MemoryScope = { UserID, AgentID, SessionID, ProjectID }
```

- `SQLiteMemoryStore` 在 SQL WHERE 子句中始终应用 scope 过滤（`buildWhereClause`）
- `InMemoryStore` 在 `collectCandidates` + `matchFilter` 中过滤
- `List` 支持 `UserID` 下推到 SQL/内存过滤（代码注释明确标注"安全边界"）
- `ForgetAll` 同样应用 scope 过滤

#### 关键风险点：SearchAllLayers（❌）

**文件**：`memory/manager.go:222-250`

```go
func (m *Manager) SearchAllLayers(ctx context.Context, query string, topK int) ([]ScoredMemory, error) {
    for _, layer := range ValidLayers() {
        results, err := m.retriever.Search(ctx, m.store, query, MemoryFilter{
            UserID:    "",    // ← 空字符串 = 不过滤
            SessionID: "",    // ← 空字符串 = 不过滤
            Layer:     layer,
            TopK:      perLayer,
        })
```

**影响**：此方法检索**所有用户、所有项目**的记忆，完全无视 scope 隔离。如果被用于多用户/多项目场景，会导致严重的跨用户数据泄漏。

**当前缓解因素**：此方法目前仅被用于：
- 对话汇总场景（`OnSessionClose` 中的 `List` 按 scope 过滤）
- 测试代码

但它是公开导出的方法，任何调用方都可能误用。

#### 其他风险

1. **`buildMemoryContextBlock` 使用 UserID** — `memory/extension.go:400` 将 `scope.UserID` 以 `"与 {userLabel} 相关的历史记录"` 注入到 system prompt，如果 UserID 是真实用户名则会泄漏。

2. **无审计日志** — Memory 操作（Remember/Recall/ForgetAll）没有审计日志记录，数据泄漏后难以追溯。

#### 修复建议

1. **紧急修复**：将 `SearchAllLayers` 标记为 `// Deprecated` 并限制为仅测试使用，或强制要求调用方传入非空的 `UserID`
2. **中期**：添加 `MustScopeFilter()` 编译时断言，防止新方法无意中遗落 scope 过滤
3. **UserID 混淆**：将 `buildMemoryContextBlock` 中的 `scope.UserID` 在输出前模糊化

---

## P0-3: 敏感路径变更审计

### ✅ 通过 — 无异常变更

#### 审计结果

自最近一次审查以来，敏感路径共有 10 次变更提交：

| 提交 | 类型 | 说明 |
|------|------|------|
| `adaa6e5` | lint 修复 | golangci-lint 告警（gocognit/staticcheck/unused/unparam） |
| `01775ed` | 深度重构 | 大文件拆分/God包拆分/架构解耦 |
| `77e7e2e` | 中频重构 | 函数拆分/重复消除/未用参数清理 |
| `f3e704b` | 修复 | CI check-arch 和 lint |
| `e242d48` | 功能 | 核心模块升级 8 项能力 |
| `2121d9e` | lint | 全量 lint 审查 + 全量修复 |
| `e10c5eb` | 功能 | 条款智能体体系 |
| `cfe8c7b` | 修复 | Desktop/ACP 前端断链 + 引用核验 Gate |
| `fd2ff2b` | 修复 | TUI 全链路修复 |
| `560b484` | 性能 | 消除两阶段启动门闩缺陷 |

**评估**：所有变更均为重构、功能添加或修复，未发现针对安全机制的绕过或削弱。但大型重构 commit（如 `01775ed`）可能将敏感变更混入数百个文件中难以独立审阅。

**建议**：未来对敏感路径的变更建议使用 **独立 commit + commit 正文注明安全审阅要求**。

---

## P1-4: MCP 信任 TOCTOU 时序窗口

### ⚠️ 告警 — 存在理论 TOCTOU 窗口

#### 流程分析

```
DiscoverMCPExtensions()
  1. 扫描 $PWD/.mcp.json
  2. isConfigTrusted(path) → 读取文件内容 → SHA-256 比对
  3. LoadMCPConfig(path) → 再次读取文件 → 解析 JSON
  4. createExtension(name, cfg) → 构造 MCP 客户端 → 可能启动子进程
```

**风险**：步骤 2 信任校验与步骤 4 命令执行之间，文件内容可能被篡改。

**场景**：攻击者拥有本地文件系统写入权限且在步骤 2 完成后、子进程启动前修改 `.mcp.json` 中的 `command` 字段。

#### 缓解因素

1. **信任校验已 fail-closed**：文件内容变化后哈希失配，不会被执行
2. **文件所有者检查**：`isOwnedByCurrentUser()` 防止共享目录投毒
3. **窗口极其狭窄**：steps 2→3→4 在同一 goroutine 中顺序执行（ms 级）
4. **`MADY_MCP_TRUST_CWD` 显式逃生门**：仅开发环境使用

#### 修复建议（P2 优先级）

1. **内存校验**：在 `isConfigTrusted` 时计算并缓存文件内容的 SHA-256；在 `LoadMCPConfig` 后、`createExtension` 前重新校验哈希是否一致
2. 或使用 `mmap` / 文件锁在信任校验后锁定文件句柄

---

## P1-5: 视觉工具沙箱字段传播

### ✅ 通过 — 沙箱传播完整

#### 验证结果

**文件**：`tools/vision.go`

| 路径 | 安全措施 | 状态 |
|------|---------|------|
| 本地文件读取（L438） | `resolvePathSandboxed(input.Image, cwd, cfg.Sandbox)` | ✅ |
| URL 下载（L61-93） | `newSSRFSafeHTTPClient(60s)` — 防 SSRF | ✅ |
| URL 前缀验证（L60） | 必须 `http://` 或 `https://` | ✅ |
| 下载大小限制（L80） | `io.LimitReader(resp.Body, 50MB)` | ✅ |
| Provider API 调用（L338） | `o.provider.Complete()` 含 API key | ✅ (但无 PII 脱敏) |
| Base64 输入（L424） | 直接解码，无路径解析 | ✅ |
| 图片 MIME 验证（L456） | 必须为 `image/*` | ✅ |
| 尺寸限制（L291-296） | MaxBytes=20MB, MaxPixels=~16MP | ✅ |

**发现**：VisionToolConfig.Sandbox 字段被 `resolvePathSandboxed` 正确使用，沙箱边界完整传播。唯一关联的 P0-1 问题（LLM API 调用无脱敏）已在 P0-1 中单独跟踪。

---

## P1-6: 文件系统沙箱隔离完整性

### ✅ 通过 — 沙箱实现扎实

#### 安全机制清单

| 机制 | 位置 | 说明 |
|------|------|------|
| 符号链接解引用 | `path.go:93-100` | `evalSymlinksExist` 防止 `link_to_etc → /etc` |
| 不存文件回退 | `path.go:155-175` | 写操作时走最近的已存在父目录 |
| 白名单读写分级 | `path.go:108-120` | `AllowRead` vs `AllowWrite` 分离 |
| 边界检测 | `path.go:145-149` | `isWithin` 使用 `filepath.Rel` + `..` 前缀检查 |
| Inode 钉住 | `path.go:228-237` | `OpenSandboxed` → 返回已打开的文件描述符 |
| Inode 验证 | `path.go:258-283` | `pinPath` + `verifyOpenedInode` 防 symlink swap |
| NFD 标准化 | `path.go:183-192` | macOS NFD 兼容 |
| 默认 AccessRead | `path.go:63` | 向后兼容模式 |
| 写操作 AccessWrite | `path.go:68` | `resolvePathSandboxedMode` 模式感知 |

#### 特定攻击面覆盖

| 攻击类型 | 是否覆盖 | 说明 |
|---------|---------|------|
| 路径遍历 (`../../../etc/passwd`) | ✅ | `isWithin` 拦截 |
| 符号链接逃逸 | ✅ | `evalSymlinksExist` 预解析 |
| TOCTOU symlink swap | ✅ | `OpenSandboxed` inode pinning |
| macOS NFD vs NFC | ✅ | `resolveNFD` fallback |
| 写目标不存在 | ✅ | `evalSymlinksExist` 父目录回退 |
| 白名单不存在目录 | ✅ | `resolveAllowList` 静默跳过 |

**唯一的告警**：`SandboxDisabled` 仅输出警告而不拒绝，这是为向后兼容的设计决策，生产环境应始终启用沙箱。

---

## P1-7: 前端安全审查

### ✅ 通过 — 前端安全态势良好

#### CSP 策略

**文件**：`desktop/frontend/index.html`

```html
<meta http-equiv="Content-Security-Policy"
      content="default-src 'self'; script-src 'self';
               style-src 'self' 'unsafe-inline'; img-src 'self' data:" />
```

**评估**：合理的 CSP 基线。`unsafe-inline` 对 style-src 是合理的（CSS-in-JS）。无 `unsafe-eval`。

#### XSS 向量

- `dangerouslySetInnerHTML` — **未使用**（仅在测试中对 container.innerHTML 做空值断言）
- `innerHTML` — **未用于渲染**
- `eval()` — **未使用**
- `location.href`/`window.location` — 仅在 `ProjectTree.tsx:790` 用于刷新页面 (reload)，安全

#### API 安全

- 所有 API 调用通过 Wails Go Binding → Go 后端处理
- 用户输入不经前端直接传给 LLM（通过 `backendChat` → `App.Chat` Go method）
- 请求体大小限制已在 server 端实现（10 MiB default）

#### 测试 API 门控

- `__mady` 全局测试接口仅通过 `VITE_ENABLE_TEST_API=true` 启用
- 生产构建默认禁用

#### 告警项

1. **`// @vite-ignore`** — `backend.ts:44` 在动态导入中使用 `@vite-ignore` 绕过 Vite 静态分析，但导入路径是硬编码的 `../../wailsjs/go/${module}`，非用户可控，风险低。

2. **Token 泄露风险** — `chat.ts` 中 `ToolCall.args` 存储工具调用参数，如果工具调用中包含敏感数据（如文件路径、API 响应），会在 UI 中渲染。但 Go 后端已做沙箱校验，前端展示的是已处理后的结果。

---

## P2-8: gosec 告警审查

### ⚠️ 建议 — 103 处标记，大部分合理

#### 告警分类统计

| 规则 | 计数 | 评估 |
|------|------|------|
| G304 (文件路径) | ~35 | ✅ 多数来自 sandbox-checked source 或 trusted dir |
| G104 (未检查错误) | ~30 | ✅ cleanup-only (close/kill/remove) |
| G204 (子进程) | ~10 | ✅ binary 已通过 LookPath 或硬编码 |
| G118 (后台 goroutine) | 2 (memory/extension.go) | ⚠️ 需关注 goroutine 泄漏 |
| G301 (目录权限) | 1 | ✅ 4700 → 需要 +x for shared lib |
| G101 (凭证) | 1 | ✅ 环境变量名常量，非凭证 |
| G703 (err check) | 2 | ⚠️ 次优但安全（os.Stat → 路径验证） |
| G602 (假阳性) | 1 | ✅ 明确标注 |
| G601 (unsafe) | 1 | ⚠️ 次优但安全（0-copy vector） |

#### 值得注意的告警

**1. G118 — 后台 goroutine** — `memory/extension.go:151,331`

```go
go func() { //nolint:gosec // G118: background goroutine with no request context
```

这两个 goroutine 在会话关闭和后模型调用时异步执行记忆提取/汇总，没有父 context 链。如果 LLM 调用挂起，goroutine 可能泄漏。当前设置了 30s 超时 context，但 panic 恢复是否充分需要验证。

**2. `//nolint:gosec` 注释质量** — 大部分注释提供了明确的理由（"path from sandbox-checked source"、"cleanup-only"），表明开发者对安全告警有意识审查。少数中文注释（如 `pkg/agentconfig/role.go`）在英文代码中略显不一致，但不影响安全。

---

## P2-9: 密钥管理审查

### ✅ 通过 — 无硬编码凭证

#### 检查结果

| 检查项目 | 结果 |
|---------|------|
| `gitleaks` 扫描 | 不可用（未安装），替代搜索无命中 |
| `sk-*` API key 模式搜索 | 未命中 |
| `ghp_*` / `gho_*` GitHub token 搜索 | 未命中 |
| 环境变量名搜索 (API_KEY, TOKEN, SECRET) | ✅ 仅环境变量名常量，无值 |
| `.env` 文件 | 已在 `.gitignore` 中 |
| 硬编码密码 | 未发现 |

#### 凭证管理方式

```
Provider API Key:  MADY_API_KEY / API_KEY 环境变量
Vision API Key:    MADY_VISION_API_KEY（缺省回退 API_KEY）
ACP Token:         MADY_ACP_TOKEN 环境变量
Model Selection:   MADY_VISION_MODEL / PROVIDER / MODEL 环境变量
```

#### 告警

1. **`VISION_API_KEY` 环境变量命名** — `tools/vision.go:31` 中 `EnvVisionAPIKey = "MADY_VISION_API_KEY"` 使用了 `#nosec G101` 抑制，这不是安全问题（只是环境变量名），但值得注意未来不要改为包含真实密钥值的变量。

---

## 服务器安全态势

### ACP 认证（acp/auth.go）✅

```go
subtle.ConstantTimeCompare([]byte(params.Token), []byte(p.token))
```

使用常量时间比较，防止时序侧信道攻击。Fail-closed：空 token 始终认证失败。

### HTTP 服务器安全（server/server.go）✅

- **CORS**：默认 fail-closed（未配置时无 CORS 头）
- **请求体大小限制**：10 MiB 默认上限，可通过 `SetMaxRequestBodyBytes` 调整
- **`ReadHeaderTimeout`**：10 秒，防 Slow Loris 攻击
- **TLS 支持**：`ListenAndServeTLS` 已实现

### Handoff 白名单（agentcore/handoff.go:89）✅

```go
if !a.isHandoffAllowed(h) { ... return NewFailureResult("交接被拦截", fallback), nil }
```

Default-deny 模式：`AllowedSources` 为空时拒绝所有来源。

---

## 修复优先级建议

| 优先级 | 问题 | 影响面 | 建议修复窗口 |
|--------|------|--------|------------|
| **P0** | P0-1: LLM 出站无 PII 脱敏 | 数据合规/用户隐私 | **立即** — 1-2 周 |
| **P0** | P0-2: `SearchAllLayers` 绕过隔离 | 多用户数据泄漏 | **立即** — 标记废弃或限制调用方 |
| **P1** | P1-4: MCP TOCTOU | 命令注入（低概率） | 下一迭代 |
| **P2** | P2-8: G118 goroutine 泄漏风险 | 稳定性 | 下一迭代 |
| **P2** | P0-3: 敏感路径混入大型重构 commit | 审计可追溯性 | 持续改进 |

---

## 项目安全亮点

1. **Sandbox 实现扎实** — 符号链接解引用 + inode pinning + 白名单读写分级是行业最佳实践
2. **Fail-closed 设计** — CORS、MCP 信任、ACP 认证、Handoff 白名单均采用 default-deny
3. **SSRF 防护** — 视觉工具的 HTTP client 使用了 `newSSRFSafeHTTPClient`
4. **CSP 策略** — Desktop 前端已设置 CSP
5. **TOCTOU 防护意识** — `pinPath` + `verifyOpenedInode` 在文件工具层面实现了层内防护
6. **测试覆盖** — 关键安全模块（token 认证、MCP 信任、沙箱路径）有单元测试

---

*报告生成工具: `scripts/check-sensitive-paths.sh` + 手动代码审计*
