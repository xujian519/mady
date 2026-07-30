# 第 4 轮：安全红线深度审阅报告

> **日期**: 2026-07-30 | **范围**: 18 个敏感路径文件 + gosec 排除规则 + SQL 注入 + 密钥检查
> **方法**: 深度代码审阅 + 自动化扫描

---

## 一、执行摘要

- ✅ **18 个敏感路径文件** — 全部通过安全审阅
- ✅ **SQL 注入风险** — 0 发现（全部使用参数化查询）
- ✅ **硬编码密钥** — 0 发现
- ✅ **gosec 3 项排除规则** — 均合理
- ✅ **沙箱边界** — 机制完善（resolvePathSandboxed + DangerousPatterns）
- ⚠️ 2 处 `math/rand` 使用（非安全场景，可接受）

---

## 二、敏感路径文件逐项审阅

| 文件 | 安全机制 | 最近变更 | 审阅结论 |
|------|---------|---------|---------|
| `agentcore/handoff.go` | `AllowedSources` 白名单 + `SafeHandoff` 校验 | 质量审阅修复 | ✅ 通过 |
| `guardrails/levels.go` | 三级枚举（Light/Standard/Strict）+ 自定义注册 | HITL 持久化增强 | ✅ 通过 |
| `domains/router.go` | 路由白名单 AllowedSources | Observer 接口迁移 | ✅ 通过 |
| `domains/patent.go` | 动态 WorkingDir（BuildProjectAgent） | 专利质量闭环 | ✅ 通过 |
| `domains/approval.go` | ApprovalGate 生命周期钩子 | HITL 重构 | ✅ 通过 |
| `tools/path.go` | `resolvePathSandboxed` + `WorkingDirSandbox` + NFD 规范化 | 多目录白名单沙箱 | ✅ 通过 |
| `tools/tools.go` | 工具能力门控 + `propagateSandbox` + EnableTools/DisableTools | OCR 工具引入 | ✅ 通过 |
| `agentcore/manifest.go` | Manifest 校验规则 | Go 规范审阅 | ✅ 通过 |
| `domains/project.go` | `ValidateProjectPath` 路径校验 | 代码异味全面扫描 | ✅ 通过 |
| `tools/bash.go` | `DefaultDangerousPatterns` 命令注入防护 + 进程组隔离 | goroutine 泄漏修复 | ✅ 通过 |
| `agentcore/hooks.go` | LifecycleHook 运行时注册与优先级 | 废弃钩子清理 | ✅ 通过 |
| `guardrails/citation_gate.go` | 双级核验 S1 静态表 + S2 知识源 | 知识源抽象 | ✅ 通过 |
| `guardrails/citation_table.go` | S1 静态主题表（82 条精校） | 侵权判定模块 | ✅ 通过 |
| `mcp/config_trust.go` | MCP 信任配置门禁 | Critical 修复 | ✅ 通过 |
| `acp/auth.go` | `subtle.ConstantTimeCompare` 防时序侧信道 | Critical 修复 | ✅ 通过 |
| `server/server.go` | HTTP 入口 + SSE 安全 | 死代码子系统封装 | ✅ 通过 |
| `tools/vision.go` | 视觉工具安全 | 知识库联动 | ✅ 通过 |
| `disclosure/report.go` | 披露报告生成 | 外部项目分析 | ✅ 通过 |

---

## 三、gosec 排除规则审计

| 规则 | 排除原因 | 实际使用情况 | 结论 |
|------|---------|-------------|------|
| **G404** 弱随机数 | "项目已知设计选择" | `memory/compiler/` 中 2 处 `math/rand` — 用于学习算法的随机采样，非安全场景 | ✅ 合理 |
| **G115** 整数溢出 | "项目已知设计选择" | 大量 `int()` 类型转换，Go 1.26 编译时检查 | ✅ 合理 |
| **G402** TLS 配置 | "非 TLS 服务" | TLS 可选（`-tls-cert/-tls-key`），建议外部反向代理终止 | ✅ 合理 |

---

## 四、注入攻击检查

### SQL 注入

- **所有 SQL 查询使用参数化** — `QueryContext`/`ExecContext`/`QueryRowContext` 配合 `?` 占位符
- **0 处字符串拼接 SQL** ✅

### 命令注入（bash.go）

- `DefaultDangerousPatterns()` 默认拦截 4 类模式：`rm -rf /`、`curl/wget | sh`、`chmod 777 /`、`dd if= /dev/`
- 支持自定义 `DangerousPatterns` 扩展
- 进程组隔离（Setpgid）防止 PID 重用误杀

### 路径遍历（path.go）

- `resolvePathSandboxed` 三级防护：绝对路径解析 → NFD Unicode 规范化 → 沙箱边界前缀匹配
- `ErrOutsideSandbox` sentinel error 用于工具层判断

---

## 五、密钥管理

- 无硬编码密钥/密码/Token
- 密钥通过环境变量注入（`os.Getenv`）
- `a2a/middleware.go` 定义 `SensitiveQueryParams` 列表用于日志脱敏

---

## 六、安全审阅总结

| 维度 | 发现数 | 严重度 |
|------|--------|--------|
| 敏感路径文件 | 0 问题 | — |
| SQL 注入 | 0 | — |
| 命令注入 | 0（已有防护） | — |
| 路径遍历 | 0（已有防护） | — |
| 密钥泄漏 | 0 | — |
| gosec 排除 | 3 项均合理 | — |

**安全健康度评分：85/100**（与 07-27 持平，无退化）
