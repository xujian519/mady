# 第五阶段：代码规范合规性对照复审报告

**日期**: 2026-07-30
**基准**: `docs/GO-DEVELOPMENT-STANDARDS.md`

---

## 5.1 10 条硬约束逐项复审（第 0 章）

| # | 规则 | 状态 | 说明 |
|---|------|------|------|
| 1 | 所有 error 必须检查 | ⚠️ 部分 | tools 模块 30+ 处 G104（Close/Kill/Wait 等清理操作未检查错误），其中多数为 `make lint` 新暴露 |
| 2 | 禁止 dot import | ✅ 0 violations | 维持 |
| 3 | 禁止 init() 中 panic | ✅ 0 violations | 维持 |
| 4 | 禁止 common/utils/base 包 | ✅ 0 violations | 维持 |
| 5 | import 三组排序 | ✅ 正常 | goimports -l 无输出 |
| 6 | 错误信息小写开头 | ⚠️ ~92 候选 | 绝大多数为合法错误（BLOCKED、CDP、OCD、DOCX 等缩写 + 中文错误），非违规 |
| 7 | goroutine 生命周期管理 | ✅ 无新问题 | 07-27 P0 均已修复，本次无新增 |
| 8 | 导出符号必须注释 | ✅ revive 无 exported 违规 | 维持 |
| 9 | Config Validate 方法 | ⚠️ 94% 缺口 | 116 个 Config structs，仅 7 个有 Validate() |
| 10 | Mutex 使用 RWMutex | ✅ 无违规 | 8 Mutex + 6 RWMutex + 20 atomic 分布合理 |

---

## 5.2 6 类 AI 违规专项复审

| # | 违规模式 | 状态 | 说明 |
|---|---------|------|------|
| 1 | 幻觉 API | ✅ 0 发现 | 无编造标准库或第三方函数 |
| 2 | 重复造轮子 | ⚠️ 已知 4 类 | fuzzy 算法完整复制（fuzzy.go ↔ tui/internal/fuzzy/fuzzy.go）、工具注册 4 次重复、三性分析框架相似 |
| 3 | 风格漂移 | ⚠️ 已知 4 类 | tools 错误处理风格与 agentcore 不一致（裸 fmt.Errorf vs 分层错误类型） |
| 4 | 过度工程化 | ✅ 无新增 | 07-27 识别的 2 项已知 |
| 5 | Context 传播 | ⚠️ 19 项待修复 | tools 模块 noctx 发现 19 处 HTTP/exec 请求未使用 WithContext |
| 6 | 测试覆盖 | ⚠️ 37 文件无测试 | 无改善 |

---

## 5.3 tools 错误处理深度审查

**背景**: 07-30 审计指出 tools 错误处理风格与 agentcore 不一致。

| 对比维度 | agentcore | tools | 差距 |
|---------|-----------|-------|------|
| 错误类型体系 | RetryableError/FatalError/HandoffError/GuardrailError/NodeError | 裸 fmt.Errorf / errors.New | ❌ 未使用分层类型 |
| error 包装 | 使用 `%w` + 结构化类型 | 部分使用 `%w`，部分 `: %v` | ⚠️ 需统一 |
| 自定义错误检查 | errors.Is/As 配对定义 | 有限使用 | ⚠️ |

**结论**: 本次新增的 gosec G104（30+ 处 Close/Kill/Wait 未检查错误）和 noctx（19 处）应优先修复。引入分层错误类型对 tools 模块收益有限（tools 是工具层，错误处理以"传递和报告"为主而非"分类恢复"），建议仅在新增工具接口时对齐。

---

## 5.4 Config Validate 覆盖率追踪

| 指标 | 07-30 审计 | 本次 | 趋势 |
|------|-----------|------|------|
| Config struct 总数 | 116 | 116 | 持平 |
| 有 Validate() | 7 | 7 | 持平 |
| 覆盖率 | ~6% | ~6% | 无改善 |

**未覆盖的主要 Config**（按模块）：
- `agentcore/` — AgentConfig, RunConfig, CompactionConfig 等核心配置
- `tools/` — 各工具的 ToolConfig
- `mcp/` — MCPClientConfig
- `provider/` — ProviderConfig
- 共 109 个 Config struct 缺少 Validate()

---

## 5.5 关键发现

| ID | 严重度 | 描述 |
|----|--------|------|
| SC01 | P1 | noctx 19 处 — Context 传播是 AI 违规模式 #5 的直接体现 |
| SC02 | P1 | G104 30+ 处 — tools 模块错误检查硬约束未完全落实 |
| SC03 | P2 | Config Validate 覆盖率 6% — 规范 #9 的系统性缺失 |
| SC04 | P3 | tools vs agentcore 错误处理风格差异（已知技术债务） |
