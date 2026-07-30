# 文档一致性审查报告

> 审查编号：55 | 审查范围：全部文档文件 | 审查日期：2026-07-31
> 审查方法：文件计数对比 + 格式扫描 + ADR 对照 + 代码抽样

---

## 总体健康度评分：C（待修复）

| 评分维度 | 等级 | 说明 |
|----------|------|------|
| **数据准确度** | D | 文件计数、目录引用等硬数据多处过期 |
| **格式合规** | D | AI_CHANGELOG 214/223 条不合规 |
| **架构一致性** | C | ADR 与现行文档有分歧，ADR 列如 004/005 缺失 |
| **文档完整性** | C | Spec-Driven 流程未严格执行 |
| **文风合规** | A | 面向用户文档基本遵守 tone-style-guide |

---

## P1 重要项

### 1. AI_CHANGELOG 格式合规 — ❌ 不通过

#### 要求
`CONTRIBUTING.md` 第 228-237 行规定：

```
## YYYY-MM-DD feature-slug
- Decision: [做了什么设计决策]
- Reason: [为什么这么选择，而非其他方案]
- Risk: [已知风险或局限性]
- Human Owner: [负责人姓名]
- Spec: docs/specs/[feature]/ (如适用)
```

#### 实际统计

| 指标 | 数值 | 合规率 |
|------|------|--------|
| 总条目数（`## YYYY-MM-DD` 标题） | 223 | — |
| 含 `- Decision:` 字段 | 9 | 4.0% |
| 含 `- Reason:` 字段 | 9 | 4.0% |
| 含 `- Risk:` 字段 | 9 | 4.0% |
| 含 `- Human Owner:`（含空格） | 11 | 4.9%（均值为 `[NEEDS CLARIFICATION]`） |
| 含 `- Spec:` 字段 | 3 | 1.3% |
| **完全合规（5 字段全部填写）** | **0** | **0%** |

#### 格式漂移趋势

- **2026-07-25 及之前**：使用要求的 5 字段格式（9 条，分布在 L7898-L8124）。
- **2026-07-27 之后**：全面切换为自由叙述格式，使用 `### 根因 / ### 修复 / ### 验证` 三段式结构。
- **最近条目（2026-07-30/31）**：使用 `### 变更内容 / ### 验证` + 表格分类，完全抛弃了 5 字段格式。

#### 格式修正方案

**方案 A（推荐——渐进收敛，不破坏现有记录）**：

修订 `docs/decisions/AI_CHANGELOG.md` 头部格式说明为：

```markdown
# AI 变更记录

## 格式规范

每条变更记录**至少**包含以下 5 个字段（可在正文末尾追加）：

## YYYY-MM-DD: type(scope) 简短标题

（正文：自由叙述，支持 ### 根因 / ### 变更内容 / ### 验证 等小节）

### 元数据
- Decision: 一句话总结关键决策
- Reason: 为何如此选择（而非备选）
- Risk: 已知副作用或限制
- Human Owner: 负责人姓名（AI 标记 / 补填）
- Spec: docs/specs/xxx/（如适用）
```

同时：
1. **不改历史**：214 条历史不全记录不追溯回填（成本过高）。
2. **加门禁**：在 `pre-commit` 或 `scripts/check-sensitive-paths.sh` 中增加对 `AI_CHANGELOG.md` 新条目的 5 字段检查（正则：`- (Decision|Reason|Risk|Human Owner|Spec):\s+.+`）。
3. **模板补丁**：在 `CONTRIBUTING.md` 第 228 行明确标注「字段可在末尾追加，不要求取代自然语言描述」。

**方案 B（激进——自动化回填）**：用 LLM 对 214 条历史逐条提取 Decision/Reason/Risk/HumanOwner/Spec。缺点：成本高，HumanOwner 无法确定。

---

### 2. 文件计数与目录引用漂移 — ❌ 不通过

#### 2a. 总文件数不匹配

| 来源 | 总 Go 文件 | 非测试 | 测试 | 行数 |
|------|-----------|--------|------|------|
| AGENTS.md 声明 (2026-07-27) | 1,229 | 820 | 409 | ~281K |
| **实际**（不含 vendor） | **1,405** | **969** | **436** | — |
| 偏差 | +176 (+14%) | +149 (+18%) | +27 (+7%) | — |

**根因**：2026-07-28 以来进行了大规模重构（God 包拆分、大文件拆分、Worker 引擎等），新增了大量文件。

**问题**：AGENTS.md 顶部的计数注释写着"文件计数更新时间：2026-07-27"，但仅 4 天后偏差已达 14%。内置的 `find . -name '*.go'` 命令因 2026-07-28 启用 `go work vendor` 现会额外计入 2,987 个 vendor 文件，运行结果为 4,392 — 与声明值相差 3,163。

#### 2b. 各文档工具计数三叉矛盾

| 文档 | tools 计数 | 位置 |
|------|-----------|------|
| CLAUDE.md | "69 源 + 23 测试" | 目录结构 |
| CLAUDE.md | "工具层(85源)" | 架构图 |
| CONTRIBUTING.md | "60 工具" | 目录结构 |
| README.md | "35 内置工具" | 扩展表 |
| **实际** | **74 非测试 + 23 测试（共 97 文件）** | — |

#### 2c. agentcore 文件计数不匹配

| 来源 | 非测试 | 测试 |
|------|--------|------|
| CLAUDE.md 声明 | 105 | 66 |
| **实际** | **112** | **69** |

#### 2d. CLAUDE.md 目录结构错误

| 错误类型 | 位置 | 详情 |
|----------|------|------|
| 🐛 目录名错误 | `domains/domainconfig/` | CLAUDE.md 第56行写 `domainconfig/`，实际目录为 `domains/config/` |
| 🐛 重复条目 | `agentcore/concurrency/` | 两次出现在 CLAUDE.md 第35-36行 |
| 🐛 重复条目 | `workflows/` | 两次出现在 CLAUDE.md 第114-115行，描述不同 |
| 🐛 缺失目录 | `domains/config/` | 实际存在但未列在 CLAUDE.md 中 |

#### 修复建议

1. **更新 AGENTS.md 文件计数**：改为使用 `--exclude-dir=vendor` 或自动计算。
2. **统一工具计数**：检查 `tools/` 下实际导出的 `*_test.go` 文件数量，在各文档中统一数字。
3. **修复 CLAUDE.md 目录结构**：`domainconfig/` → `config/`，去重 concurrency 和 workflows 条目。
4. **在 Makefile 中添加 `make count-files`** 命令，生成最新计数供文档引用。

---

### 3. ADR 与当前架构一致性 — ⚠️ 告警

#### 抽查结果

| ADR | 状态 | 当前一致性 | 备注 |
|-----|------|-----------|------|
| 0001: 分层架构 | Accepted | ⚠️ 部分过时 | 描述 6 层架构，但 CLAUDE.md 现在描述为 8 层分层；领域→心理层的层次变化未在 ADR 中更新 |
| 0002: DAG+Pregel | Accepted | ✅ 一致 | Pregel 和 DAG 仍在 `graph/` 中使用 |
| 0003: 统一运行时 | **Proposed** | ❌ 未实现 | 仍是 Proposed 状态，ExecutionPlan/UnifiedRuntime 未出现在当前代码中（`grep -r 'UnifiedRuntime\|ExecutionPlan' --include='*.go' .` 无结果），ADR 定义的 Phase 2-5 未执行 |
| 0006: Memory 模块 | Accepted | ⚠️ 部分实现 | Phase 2（LLM 自动提取）标注为"待启用"，ContextBuilder 默认关闭；ADR 描述 8 文件约 1500 行，实际 `memory/` 约 35 文件 |
| 0007: ContextBuilder | Accepted | ⚠️ 未启用 | `Enabled = false` 默认关闭，Agent 仍通过旧路径运行 |

#### ADR 完整性问题

| 问题 | 详情 |
|------|------|
| 编号断裂 | 0000→0001→0002→0003→**006**→007（缺少 004、005） |
| 状态不一致 | ADR-0003 为 Proposed 而非 Accepted，但被其他 ADR 引用为前置 |
| 架构图分歧 | ADR-0001 的 6 层架构与 CLAUDE.md 的 8 层架构无法对应 |

#### 修复建议

1. **ADR-0001 需要修订**：更新架构层数从 6 层到与 CLAUDE.md 一致的描述，或标记为 Superseded 并创建 ADR-0004。
2. **ADR-0003 要么 Accepted（补实现代码）要么 Rejected（标记不再执行）**：当前 Proposed 状态 + 零实现代码 = 误导。
3. **补充 ADR-004 和 ADR-005**：即使只是占位（标记为 Deprecated/Not used），也需要保持编号连续性。

---

## P2 建议项

### 4. Spec-Driven 文档完整性 — ⚠️ 告警

#### Spec 索引（来自 `docs/specs/README.md`）

| 功能 | 目录 | 阶段 | 四步完整？ | Human Owner |
|------|------|------|-----------|-------------|
| 心理引擎 | `docs/superpowers/specs/` | 已完成 | ❌ 单文件 | — |
| 向量检索 | `docs/specs/vector-retrieval/` | 已上线 | ✅ 四步完整 | — |
| 现有技术检索 | `docs/specs/design-prior-art-retrieval-stage.md` | 已上线 | ❌ 单文件 | — |
| 获取规则阶段 | `docs/specs/design-rule-acquisition-stage.md` | 已上线 | ❌ 单文件 | — |
| 复审请求 | `docs/specs/reexamination-request/` | 设计草案 | ❌ 仅 01-proposal | 待指派 |
| 桌面端 | `docs/specs/desktop/` | 设计草案 | ✅ 四步完整 | 待指派 |
| enablement-a26.3 | `docs/specs/enablement-a26.3/` | — | ❌ 仅 01-task-breakdown | — |
| plantask-introduction | `docs/specs/plantask-introduction/` | — | ❌ 仅 plan.md | — |
| prompt-templates-wiring | `docs/specs/prompt-templates-wiring/` | — | ❌ 缺少 proposal 和 tasks | — |

#### 关键问题

1. **代码先于 Spec**：向量检索、现有技术检索、获取规则阶段标注为"已上线"但 Spec 文档链不完整，违反了「四步文档全部完成、人工 Sign-off 后再进入代码实现」的规则。
2. **Human Owner 栏为空**：所有 Spec 条目的 Human Owner 字段均为空白或「待指派」，无法追溯决策负责人。
3. **超能力 Spec 未整合**：`docs/superpowers/specs/` 目录的 5 个文档独立于 `docs/specs/` 之外，索引未覆盖。

#### 修复建议

1. 为所有"已上线"的 Spec 补充缺失的文档文件，或在 `specs/README.md` 中标注"代码先行，文档待同步"。
2. 完善 `docs/superpowers/specs/` 的索引引用。

---

### 5. 代码注释与实现同步 — ✅ 通过

抽查了 `agentcore`、`graph`、`guardrails`、`knowledge`、`retrieval`、`domains/reasoning` 共 6 个包的首批导出符号注释：

- 注释格式符合 Effective Go 规范（被导出的符号均有注释）
- 注释内容与实际代码逻辑一致
- God 包拆分后的新文件(`agentcore/worker/executor.go` 等)也有完整注释

**结论**：Go 源码的文档注释质量良好，主要问题集中在 Markdown 文档层，而非代码注释层。

---

### 6. README/文档中的错误引用 — ❌ 不通过

#### 发现 6 处错误/过时引用

| # | 文件 | 问题 | 严重度 |
|---|------|------|--------|
| 1 | **README.md:215** | Manifest 示例 JSON 中的 `handoff_targets` 含 `"chat-agent"`——该目标已在 2026-07-30 的 AllowedSources 清理中删除，是死引用 | 中 |
| 2 | **README.md:219** | 说"3 个内置领域 manifest（assistant/patent/legal）"，但 manifest-guide.md:8 说"4 个内置领域 manifest（chat/assistant/patent/legal）"——两处矛盾 | 高 |
| 3 | **manifest-guide.md:8** | 说"4 个内置领域 manifest（chat/assistant/patent/legal）"，但 `agentcore/manifests/` 只有 3 个文件（assistant.json, patent.json, legal.json），`chat.json` 不存在 | 高 |
| 4 | **CLAUDE.md:56** | `domains/domainconfig/` 不存在，实际为 `domains/config/` | 中 |
| 5 | **CLAUDE.md:35-36** | `agentcore/concurrency/` 重复列出两次 | 低 |
| 6 | **CLAUDE.md:114-115** | `workflows/` 重复列出两次 | 低 |

#### 跨文档不一致汇总

| 概念 | AGENTS.md | CLAUDE.md | README.md | CONTRIBUTING.md | 实际值 |
|------|-----------|-----------|-----------|----------------|--------|
| Go 文件数 | 1,229 | ~1,200+ | — | — | **1,405** |
| 内置 manifests | 3 | — | 3 | — | **3**（无 chat） |
| tools 工具数 | — | 69 源 + 23 测试 | 35 内置工具 | 60 工具 | **74+23 文件** |
| 架构层数 | — | 8 层 | 6 层（图） | 6 层（图） | **ADR-0001:6层vs CLAUDE.md:8层** |

---

### 7. 措辞规范合规 — ✅ 通过

对照 `docs/tone-style-guide.md` 检查记录：

| 检查项 | 结果 | 备注 |
|--------|------|------|
| 禁用词表遵守 | ✅ | 文档本身文档中未发现违规使用"绝对/一定/百分百/保证"等禁用词（技术性文档的"确保"为合理语境） |
| 结论性表述附置信度 | ✅ | README 产品功能列表均带 ✅ 标记，无夸大描述 |
| 拒绝类文案提供替代帮助 | ✅ | tone-style-guide 本身提供正确示范，README 中无生硬拒绝类文案 |
| 非 About/Banner 场景提中观 | ⚠️ | README 顶部 tagline "离二边，行中道"在约 80 行长的 README 首页中出现，tone-style-guide §1 的曝光矩阵允许 About 和首次 Banner 场景提及。可视为"品牌标语"豁免，建议保持。 |
| 面向用户文案 | ✅ | 护栏系统、报告输出均遵守规范 |

**结论**：文档层遵守良好，但建议抽查 `guardrails/` 和 `disclosure/` 的实际运行时文案以确保运行时也合规。

---

## 总结

### 优先级修复清单

| 优先级 | 问题 | 文件 | 难度 |
|--------|------|------|------|
| **P0** | AI_CHANGELOG 格式规范 + 门禁 | `CONTRIBUTING.md` + `AI_CHANGELOG.md` | 低 |
| **P0** | 目录名错误 `domainconfig/` → `config/` | `CLAUDE.md:56` | 低 |
| **P1** | manifest 数量矛盾（3 vs 4） | `manifest-guide.md:8` 或 `README.md:219` | 低 |
| **P1** | AGENTS.md 文件计数更新 | `AGENTS.md:18` | 低 |
| **P1** | CLAUDE.md 重复目录条目 | `CLAUDE.md:35-36,114-115` | 低 |
| **P1** | README manifest 示例中 `chat-agent` 死引用 | `README.md:215` | 低 |
| **P2** | ADR-0001 架构层数更新 | `docs/adr/0001-use-layered-architecture.md` | 中 |
| **P2** | ADR-0003 状态明确（Accepted/Rejected） | `docs/adr/0003-unified-runtime-design.md` | 中 |
| **P2** | Spec 索引完整性补全 | `docs/specs/README.md` | 中 |
| **P3** | 工具数三叉矛盾统一 | `CLAUDE.md` + `CONTRIBUTING.md` + `README.md` | 低 |
| **P3** | ADR 编号断裂补 004/005 | `docs/adr/` | 低 |

### 文档健康度评分分解

```
总体健康度:   C   (55/100)
  ├── 数据准确度:  D   (35/100) — 4/7 硬数据过期
  ├── 格式合规:    D   (30/100) — 0% AI_CHANGELOG 合规
  ├── 架构一致性:  C   (55/100) — ADR 半数需更新
  ├── 文档完整性:  C   (50/100) — Spec 半数不完整
  └── 文风合规:    A   (95/100) — 基本无违规
```

### 结论

文档系统存在 **P0 级阻塞问题 2 项**（AI_CHANGELOG 格式违规 + 目录名错误）和 **P1 级问题 4 项**。健康度评分为 **C 级（待修复）**。核心矛盾在于：代码库在 2026-07-28 至 2026-07-31 经历了大规模重构（God 包拆分、大文件拆分、Worker 引擎等），但文档更新滞后，导致多份关键文档与实际代码状态不一致。

建议在下次 AI 接入时，优先完成本报告标记的 P0 和 P1 修复项（共 8 项），预计工作量为 30-60 分钟文档修正。
