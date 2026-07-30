# YunXi → Mady 核心模块升级 — 任务清单与验收检查清单

> 基于实施方案 plan.md（2026-07-30）
> 使用方式：每完成一项勾选 `[x]`，遇到问题标注 `[!]`

---

## Phase 1: P0 优先（第 1-3 周）

### P0-2: 技能 Include 展开（Week 1，4-5 天）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 1.1 | 创建 `skill/include.go`，实现 `ExpandIncludes()` 核心逻辑（正则解析 `<include>` 标签） | `skill/include.go` | 1d | [ ] |
| 1.2 | 实现递归展开（最大 3 层深度限制 + 循环检测） | `skill/include.go` | 0.5d | [ ] |
| 1.3 | 实现路径沙箱校验（禁止 `../` 越出 skill 根目录） | `skill/include.go` | 0.5d | [ ] |
| 1.4 | 在 `skill/skill.go` 的 `readSkillBody()` 中集成 `ExpandIncludes()` 调用 | `skill/skill.go` | 0.5d | [ ] |
| 1.5 | 编写 `skill/include_test.go`（单层/多层/循环引用/路径越界/空格变体/不存在的引用） | `skill/include_test.go` | 1d | [ ] |
| 1.6 | 更新 `skills/patent/SKILL.md` 和 `skills/enablement/SKILL.md`，用 `<include>` 替换自然语言引用 | `skills/patent/SKILL.md` 等 | 0.5d | [ ] |
| 1.7 | 更新 `AGENTS.md` / `CLAUDE.md` 文档说明 include 用法 | `AGENTS.md`, `CLAUDE.md` | 0.5d | [ ] |

**P0-2 检查清单**：

- [ ] `ExpandIncludes()` 正确解析单个 `<include>` 标签
- [ ] 多层嵌套 include（A→B→C）展开后内容与手动内联一致（MD5 比对）
- [ ] 循环引用（A→B→A）检测并报错，不进入死循环
- [ ] 路径越界（`../../../etc/passwd`）被拦截
- [ ] 不存在的引用文件返回明确错误信息
- [ ] 空格变体 `<include  ref="x.md" />` 正常解析
- [ ] 已有自然语言引用（如"详见 references/xxx.md"）正常工作不受影响
- [ ] `go test -race ./skill/...` 全部通过
- [ ] `skills/patent/SKILL.md` 加载后 body 内容包含被引用的 checklist 内容

---

### P0-1: 统一意图分类器（Week 2-3，1.5-2 周）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 2.1 | 创建 `intent/doc.go` 和 `intent/intent.go`，定义 `IntentResult` 结构体和 `UnifiedIntentRouter` 接口 | `intent/doc.go`, `intent/intent.go` | 0.5d | [ ] |
| 2.2 | 提取 `domains/router.go` 关键词匹配逻辑到 `intent/keyword.go`（`KeywordClassifier`）| `intent/keyword.go` | 1d | [ ] |
| 2.3 | 提取 `domains/classifier.go` LLM 分类逻辑到 `intent/llm.go`（`LLMClassifier`）| `intent/llm.go` | 1d | [ ] |
| 2.4 | 实现 `intent/semantic.go` 语义向量匹配（复用 `retrieval/` 基础设施）| `intent/semantic.go` | 1d | [ ] |
| 2.5 | 实现 `intent/preference.go` 偏好存储（SQLite）+ 衰减机制 | `intent/preference.go` | 1d | [ ] |
| 2.6 | 实现 `UnifiedIntentRouter` 多路融合逻辑（偏好→关键词→LLM→语义→回退） | `intent/intent.go` | 1d | [ ] |
| 2.7 | 在 `domains/unified.go` 中注入 `UnifiedIntentRouter`，替换现有 `ClassifyIntent` 调用 | `domains/unified.go` | 1d | [ ] |
| 2.8 | 编写 `intent/intent_test.go`（覆盖 5 路分类 + 回退链 + 边界情况） | `intent/intent_test.go` | 2d | [ ] |
| 2.9 | 准备 50 条标注样本，运行准确率测试（目标 ≥85%） | 测试脚本 | 0.5d | [ ] |

**P0-1 检查清单**：

- [ ] `KeywordClassifier` 分类结果与现有 `domains/router.go` 完全一致（回归测试）
- [ ] `LLMClassifier` 正确处理低置信度回退（confidence < 0.7 → keyword 兜底）
- [ ] `SemanticClassifier` 对语义相似但关键词不同的输入正确分类
- [ ] 偏好学习：用户连续两次输入同类罕见术语后，第三次自动识别
- [ ] 偏好衰减：连续 3 次输入未触发某偏好，该偏好置信度下降 ≥20%
- [ ] 综合路由优先级顺序正确（偏好 0.9 → 关键词 0.8 → LLM → ...）
- [ ] 50 条标注样本分类准确率 ≥85%
- [ ] 回退链最终一定返回有效意图（永不返回空/panic）
- [ ] `go test -race ./intent/...` 全部通过
- [ ] `go test -race ./domains/...` 无新增失败（向后兼容）
- [ ] 现有 `mady tui` 对话的路由行为无退化

---

## Phase 2: P1 扩展（第 4-7 周）

### P1-1: 宪法规则引擎（Week 4-5，2 周）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 3.1 | 定义 `Rule` 接口和 `RulePipeline`（`guardrails/pipeline.go`） | `guardrails/pipeline.go` | 1d | [ ] |
| 3.2 | 实现 `guardrails/rule_config.go` YAML 配置加载（复用 `domains/domainconfig/` 模式） | `guardrails/rule_config.go` | 1d | [ ] |
| 3.3 | 实现 `guardrails/fact_check.go` LLM 驱动事实校核 | `guardrails/fact_check.go` | 2d | [ ] |
| 3.4 | 实现 `guardrails/consistency.go` 启发式前后矛盾检测 | `guardrails/consistency.go` | 1.5d | [ ] |
| 3.5 | 实现 `guardrails/format_check.go` 领域格式合规检查 | `guardrails/format_check.go` | 1d | [ ] |
| 3.6 | 实现 `guardrails/confidence_check.go` 置信度声明检查 | `guardrails/confidence_check.go` | 1d | [ ] |
| 3.7 | 将现有 `levels.go` 关键词规则适配到 `Rule` 接口 | `guardrails/levels.go` | 1d | [ ] |
| 3.8 | 在 `domains/unified.go` 中装配 `RulePipeline` 到 LifecycleHook 链 | `domains/unified.go` | 1d | [ ] |
| 3.9 | 编写测试（每条规则独立 + 管道集成 + 虚假注入/真实拒绝） | `guardrails/*_test.go` | 2.5d | [ ] |
| 3.10 | 创建默认 `guardrails/rules.yaml` 配置文件 | `guardrails/rules.yaml` | 0.5d | [ ] |

**P1-1 检查清单**：

- [ ] `Rule` 接口的 4 种 Action（block/inject/alert/log）行为符合预期
- [ ] `fact_check.go`：注入虚构法条号"专利法第 99 条"→ 被检测为虚构引用
- [ ] `consistency.go`："技术方案为 A" 和 "技术方案不含 A" 同时出现 → 标记为矛盾
- [ ] `format_check.go`：专利分析输出缺少"技术领域"章节 → 被标记但放行（alert 模式）
- [ ] `confidence_check.go`：结论性陈述无置信度 → 自动追加免责声明
- [ ] YAML 配置加载正常（含注释的 YAML、缺失字段的默认值）
- [ ] YAML 配置加载异常（语法错误、不存在的规则名）→ 明确错误信息
- [ ] 管道中某规则报错不影响后续规则执行
- [ ] 所有 LLM 驱动的规则默认关闭，按需启用（不影响默认性能）
- [ ] `go test -race ./guardrails/...` 全部通过
- [ ] 与现有 `citation_gate` 和 `guardian` 不冲突（串联执行）

---

### P1-2: 子 Agent 角色 XML 定义（Week 6，1 周）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 4.1 | 创建 `agentcore/handoff_loader.go`，实现 `ParseHandoffRole(xmlBytes)` XML 解析 | `agentcore/handoff_loader.go` | 1d | [ ] |
| 4.2 | 创建 `agentcore/handoff_store.go`，实现角色加载/缓存/热重载 | `agentcore/handoff_store.go` | 1d | [ ] |
| 4.3 | 创建 `roles/` 内置目录 + `go:embed`，为 patent/legal/chat 创建 XML 定义 | `roles/*.xml` | 1d | [ ] |
| 4.4 | 修改 `domains/unified.go`，从 `handoff_store` 加载角色 | `domains/unified.go` | 1d | [ ] |
| 4.5 | 编写测试（XML 解析/加载/热重载/代码覆盖/缺失文件/无效 XML） | `agentcore/handoff_*_test.go` | 1d | [ ] |

**P1-2 检查清单**：

- [ ] XML 解析成功生成等价的 `HandoffConfig`（与代码硬编码对比）
- [ ] `<allowed_tools>` 白名单正确限制子 Agent 可用工具
- [ ] 代码配置可以覆盖 XML 配置的部分字段（优先级正确）
- [ ] XML 文件不存在时回退到代码默认配置
- [ ] XML 语法错误时返回明确错误信息，不静默失败
- [ ] 热重载：修改 XML 后子 Agent 配置自动更新
- [ ] `go test -race ./agentcore/...` 全部通过
- [ ] 现有 Handoff 流程（delegate/transfer）不受影响

---

## Phase 3: P2 + P3 基础设施（第 8-14 周）

### P2-2: 工具结构化输出（Week 8，3-5 天）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 5.1 | 修改 `ToolResult` 结构体添加 `DurationMs`/`Success`/`Metadata`/`ToolName` 字段 | `agentcore/tool.go` | 0.5d | [ ] |
| 5.2 | 在 `executor.go` 的 `coreExecute` 中添加自动计时和字段填充 | `agentcore/executor.go` | 0.5d | [ ] |
| 5.3 | 审计 `tools/` 高频工具（read/write/bash/search/browser），添加 Metadata 填充 | `tools/*.go` | 1d | [ ] |
| 5.4 | 编写测试验证 DurationMs 和 Success 的正确性 | `agentcore/executor_test.go` | 0.5d | [ ] |

**P2-2 检查清单**：

- [ ] `ToolResult.DurationMs` 在每次工具调用后非零（单位毫秒）
- [ ] `ToolResult.Success` 与 `Error` 字段一致（Error=="" → Success=true）
- [ ] `ToolResult.ToolName` 与调用的工具名称一致
- [ ] `ToolResult.Metadata` 对文件类工具包含 `bytes_read` 和 `file_path`
- [ ] 向后兼容：所有现有 `go test -race ./...` 测试仍通过
- [ ] JSON 序列化不包含 `StartedAt` 字段（`json:"-"`）

---

### P3-1: 自动对话压缩（Week 9，3-5 天）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 6.1 | 创建 `session/auto_compactor.go`，实现 `AutoCompactor` + `Compact()` 方法 | `session/auto_compactor.go` | 1d | [ ] |
| 6.2 | 实现 `session/token_counter.go` 简单 Token 计数（字符数估算或 tiktoken-go） | `session/token_counter.go` | 0.5d | [ ] |
| 6.3 | 在 `agentcore/agent.go` 消息追加处注入压缩检查 | `agentcore/agent.go` | 0.5d | [ ] |
| 6.4 | 通过 `agentcore/config.go` 暴露 `AutoCompactionConfig` 配置项 | `agentcore/config.go` | 0.5d | [ ] |
| 6.5 | 编写测试（阈值触发/保留段正确/增量合并/Token 优先/禁用模式） | `session/auto_compactor_test.go` | 1d | [ ] |

**P3-1 检查清单**：

- [ ] 注入 200 条消息后自动触发压缩
- [ ] 压缩后消息数 < `KeepRecent + 1`（1 为摘要消息）—— 默认 < 11 条
- [ ] 保留最近 `KeepRecent` 条（默认 10 条）消息不被压缩
- [ ] 增量合并：连续两次压缩不丢失第一次压缩的摘要
- [ ] Token 阈值优先：消息数超 `MaxMessages` 但 Token 未超 `MaxTokens` → 不压缩
- [ ] `Enabled=false` 时完全跳过压缩
- [ ] 压缩失败不中断 Agent 运行（仅 warn 日志）
- [ ] `go test -race ./session/...` 全部通过

---

### P2-1: 四层分级记忆（Week 10-11，1.5-2 周）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| 7.1 | 在 `memory/types.go` 中新增 `MemoryTier` 枚举和 `MemoryEntry.Tier` 字段 | `memory/types.go` | 0.5d | [ ] |
| 7.2 | 修改 `memory/sqlite_store.go` schema（ALTER TABLE ADD COLUMN tier/marked_eternal/last_migrated_at） | `memory/sqlite_store.go` | 1d | [ ] |
| 7.3 | 更新 CRUD 操作支持 Tier 过滤（`ListByTier`/`SearchByTier`） | `memory/sqlite_store.go` | 1d | [ ] |
| 7.4 | 创建 `memory/tier.go`，实现 `TierMigrator`（HOT→WARM→COLD 迁移逻辑 + 回升） | `memory/tier.go` | 1.5d | [ ] |
| 7.5 | 创建 `memory/eternal.go`，实现 `MarkEternal`/`UnmarkEternal` | `memory/eternal.go` | 0.5d | [ ] |
| 7.6 | 修改 `memory/extension.go`，Agent 启动时注册 `TierMigrator` 为后台任务 | `memory/extension.go` | 1d | [ ] |
| 7.7 | 编写测试（迁移逻辑/回升/边界/并发/ETERNAL 不迁移） | `memory/tier_test.go` | 2d | [ ] |

**P2-1 检查清单**：

- [ ] 新创建的 MemoryEntry 默认 `Tier="hot"`
- [ ] HOT 层记录 7 天未访问 → 自动降级为 WARM
- [ ] WARM 层记录 30 天未访问 → 自动降级为 COLD
- [ ] COLD 层记录 90 天未访问 → 标记可清理
- [ ] 被重新访问的 WARM/COLD 记录 → 回升为 HOT
- [ ] `MarkEternal` 标注的记录永不参与迁移
- [ ] `UnmarkEternal` 后恢复正常迁移逻辑
- [ ] 1000 条记忆迁移后无数据丢失（总数不变）
- [ ] 记忆查询结果在迁移前后一致（同查询同结果）
- [ ] 并发安全：多个 goroutine 同时访问迁移逻辑无 race
- [ ] `go test -race ./memory/...` 全部通过
- [ ] SQLite schema 迁移兼容旧数据库（ALTER TABLE 默认值）

---

### P3-2: DAG 编排器（Week 12-14，3-4 周）

| # | 任务 | 文件 | 预估 | 状态 |
|---|------|------|------|------|
| | **子阶段 A：增强 Pregel Checkpoint 恢复** | | | |
| 8.1 | 在 `graph/pregel.go` 的 `PregelCheckpointer` 中添加 `Resume()` 方法 | `graph/pregel.go` | 2d | [ ] |
| 8.2 | 对齐 DAG 层 `InterruptableGraph.Resume` 语义 | `graph/pregel.go` | 1d | [ ] |
| 8.3 | 编写 Pregel 层 Resume 测试 | `graph/pregel_test.go` | 2d | [ ] |
| | **子阶段 B：CheckpointStore SQLite 持久化** | | | |
| 8.4 | 在 `graph/checkpoint.go` 中新增 `SQLiteCheckpointStore` | `graph/checkpoint.go` | 1d | [ ] |
| 8.5 | 实现 WAL 模式 crash-safe 保证 | `graph/checkpoint.go` | 0.5d | [ ] |
| 8.6 | 编写 SQLite 持久化测试 | `graph/checkpoint_test.go` | 0.5d | [ ] |
| | **子阶段 C：实现 Orchestrator** | | | |
| 8.7 | 创建 `graph/orchestrator/` 子包和 `doc.go` | `graph/orchestrator/doc.go` | 0.5d | [ ] |
| 8.8 | 实现 `orchestrator.go`（Execute/Resume 主循环） | `graph/orchestrator/orchestrator.go` | 2d | [ ] |
| 8.9 | 实现 `plan_generator.go`（从意图生成执行计划） | `graph/orchestrator/plan_generator.go` | 1.5d | [ ] |
| 8.10 | 实现 `graph_executor.go`（节点重试/并发控制） | `graph/orchestrator/graph_executor.go` | 2d | [ ] |
| 8.11 | 编写 Orchestrator 测试 | `graph/orchestrator/orchestrator_test.go` | 2d | [ ] |
| | **子阶段 D：集成到 disclosure/ 管线** | | | |
| 8.12 | 将 `disclosure/` 的 Pregel 图通过 Orchestrator 执行 | `disclosure/report.go` | 1.5d | [ ] |
| 8.13 | 验证 review_gate 中断→恢复正确性 | 集成测试 | 1d | [ ] |
| 8.14 | 编写端到端测试（正常执行 + 中断恢复 + 故障重试） | `disclosure/orchestrator_test.go` | 1.5d | [ ] |

**P3-2 检查清单**：

**子阶段 A：**
- [ ] `PregelCheckpointer.Resume()` 从 checkpoint 恢复后继续执行剩余的 supersteps
- [ ] Resume 后的 PregelState 与一次性执行完成后的 State 一致
- [ ] 不存在的 checkpoint ID → 明确错误
- [ ] 多节点 Pregel 图在中间节点中断后 Resume 正确

**子阶段 B：**
- [ ] `SQLiteCheckpointStore.Save()` → 进程崩溃 → 重启 → `Load()` 返回完整数据
- [ ] 并发保存不丢失数据（WAL 保证）
- [ ] 旧 checkpoint 自动清理（TTL 过期）

**子阶段 C：**
- [ ] Orchestrator 检测到未完成 checkpoint 自动 Resume 而非重新执行
- [ ] 节点失败后按 `Retry` 配置重试（非无限制重试）
- [ ] 重试耗尽 → 保存 checkpoint 退出，等待人工干预
- [ ] 执行成功 → 清理 checkpoint
- [ ] 并发执行多个 Plan 不互相干扰

**子阶段 D：**
- [ ] disclosure/ 管线在 review_gate 中断 → 人工确认 → Resume → 输出与一次性执行一致
- [ ] 中间任意节点失败 → 重试 → 最终完成
- [ ] 端到端测试覆盖正常/中断恢复/故障重试三种路径

---

## 全局回归验证

每个 Phase 完成后运行以下命令，全部通过才算该 Phase 完成：

```bash
# 构建
go build ./...
cd tools && go build ./... && cd ..
cd tui && go build ./... && cd ..

# 测试（含竞态检测）
go test -race ./...
cd tools && go test -race ./... && cd ..
cd tui && go test -race ./... && cd ..

# Lint
make lint

# 完整验证（推荐）
make verify
```

**全局检查清单**：

- [ ] `make verify` 全绿（lint + build + test-race）
- [ ] `mady tui` 启动后基本对话可用
- [ ] 无新增编译警告
- [ ] `docs/decisions/AI_CHANGELOG.md` 已追加变更记录
- [ ] 敏感路径改动已标注并准备人工审阅

---

*清单生成时间：2026-07-30 | 基于 plan.md 第 1 版*
