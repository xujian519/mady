# Phase 3 审阅：R15 AI_CHANGELOG 5 字段合规检查 — 2026-07-25

> Phase 3 子审阅｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 审阅对象：`docs/decisions/AI_CHANGELOG.md`（135 条记录）对照 `CONTRIBUTING.md` 的 5 字段强制要求

## 摘要

**核心发现：CONTRIBUTING.md 第 217-231 行规定的 5 字段强制格式在 AI_CHANGELOG 中几乎从未被遵守**。135 条历史记录中：
- **Human Owner 字段**：仅 **1 条**（0.7%）出现，其余 134 条无人工负责人署名
- **Risk 字段**：**0 条**（0.0%）显式标注已知风险
- **Spec 字段**：极少数散落出现（如 2026-07-22 复审请求条目），但未按规范作为独立字段

实际采用的格式是 **背景 / 改动清单 / 设计决策 / 影响**（中文小节标题），与文档化要求的 **Decision / Reason / Risk / Human Owner / Spec**（英文破折号列表）**完全不重合**。

这是**文档规范与实践的系统性偏离**，而非个别遗漏。两条路径择一修复：
1. **修订 CONTRIBUTING.md** 承认现实格式（背景/改动清单/设计决策/影响）为标准，将 Human Owner/Risk 作为可选补字段
2. **强制回归文档格式** 5 字段，并补齐历史 134 条（成本极高，不推荐）

**建议路径 1**（符合"去繁就简"——让规范服从实践而非反之），但在新格式中**强制保留"Human Owner 待指派"占位**与**"Risk/无已知风险"占位**，让审计可追溯。

## 1. 规范条款（CONTRIBUTING.md:217-231）

```
## YYYY-MM-DD feature-slug
- Decision: [做了什么设计决策]
- Reason: [为什么这么选择，而非其他方案]
- Risk: [已知风险或局限性]
- Human Owner: [负责人姓名]
- Spec: docs/specs/[feature]/ (如适用)
```

> CONTRIBUTING.md:231 明示："此文件不是可选项 —— 每次 AI 参与的功能变更都必须更新。"

## 2. 实践现状（统计证据）

| 字段 | 出现条数 | 占比 | 备注 |
|------|---------|------|------|
| Human Owner | 1 / 135 | 0.7% | 唯一一条是 2026-07-22 复审请求，写"Human Owner 待指派"（占位未落实） |
| Risk | 0 / 135 | 0.0% | 完全缺失 |
| Spec | 极少 | <5% | 散落在正文，非独立字段 |
| Decision / Reason | 部分 | ~30% | 大多以"设计决策"小节合并呈现 |
| **实际格式** | 135 / 135 | 100% | 统一使用 背景/改动清单/设计决策/影响 四段式 |

### 最近 15 条逐条核查

```
## 2026-07-20: 代码审查修复 — 6 项改进                          | HO=0 Risk=0 Spec=0
## 2026-07-20: Phase 4 — 系统态浮层                              | HO=0 Risk=0 Spec=0
## 2026-07-21: Sprint 1 文件规模治理 — R1/R2/R6                 | HO=0 Risk=0 Spec=0
## 2026-07-21: Sprint 2 文件规模治理 — R3/R4/R5                 | HO=0 Risk=0 Spec=0
## 2026-07-21: 全量质量审阅修复                                  | HO=0 Risk=0 Spec=0
## 2026-07-21: TUI 四轮优化 Sprint                              | HO=0 Risk=0 Spec=0
## 2026-07-21: TUI 欢迎页面                                     | HO=0 Risk=0 Spec=0
## 2026-07-21: 新增护航评估 GuardrailsMetric                    | HO=0 Risk=0 Spec=0
## 2026-07-21: Batch 6 — 联邦式 Agent 网络 + i18n              | HO=0 Risk=0 Spec=0
## 2026-07-21: P1-1 物理移动 + P0-1 Phase 3 iface 迁移          | HO=0 Risk=0 Spec=0
## 2026-07-22: 修复 CI 检查失败                                  | HO=0 Risk=0 Spec=0
## 2026-07-23: workflows/autoresearch 全量审阅修复              | HO=0 Risk=0 Spec=0
## 2026-07-23: workflows/autoresearch 第二轮修复                | HO=0 Risk=0 Spec=0
## 2026-07-24: IPC 审查标准卡片知识集成                         | HO=0 Risk=0 Spec=0
## 2026-07-24: 斜杠命令系统交互改进                              | HO=0 Risk=0 Spec=0
## 2026-07-25: TUI 方向键冲突修复 + Ctrl+C 快捷键语义修正        | HO=0 Risk=0 Spec=0
```

**最近 15 条全部 0/3 合规。** 这是一个长期、稳定、系统性的偏离。

## 3. 审阅维度执行情况（5 Lens）

| 维度 | 结论 |
|------|------|
| Lens-1 Go 编码 | N/A（文档审查） |
| Lens-2 架构分层 | N/A |
| Lens-3 安全红线 | ⚠️ 隐性风险：无 Human Owner = 无人工问责锚点，违反 AGENTS.md"AI 参与开发时人类最终负责"原则；无 Risk = 历史决策的风险账本缺失，Phase 5 无法做风险×成本排序 |
| Lens-4 测试门禁 | N/A |
| Lens-5 核心理念 | ⚠️ **规范与实践脱节本身违反"克制"**——文档写了不执行的规则是噪声；建议让规范服从实践（修订文档），而非让实践补齐 134 条历史（成本极高） |

## 4. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F-R15-1** | **M** | Lens-3 问责缺失 | `AI_CHANGELOG.md` 全文 135 条仅 1 条含 Human Owner；`CONTRIBUTING.md:227` 强制要求该字段 | AGENTS.md"人类最终负责"原则；CONTRIBUTING.md:217-231 | 修订 CONTRIBUTING.md 承认现实四段式 + 强制追加"- Human Owner: 待指派"占位行；新条目从下次开始强制执行 |
| F-R15-2 | M | Lens-3 风险账本缺失 | 135 条 0 条含 Risk；`CONTRIBUTING.md:226` 强制要求 | 同上 | 新格式追加"- Risk: 无已知风险 / [描述]"占位行；Phase 5 backlog 无法依赖 CHANGELOG 做风险追溯，需另立 docs/decisions/phase5-backlog.md |
| F-R15-3 | L | Lens-5 文档一致性 | `CONTRIBUTING.md:222-229` 示例与实际 100% 偏离；`CLAUDE.md`/`AGENTS.md` 引用该规范 | 文档同步 | 一次性修订 CONTRIBUTING.md 模板，使其与 AI_CHANGELOG 实际格式对齐 |

## 5. 建议处置方案（路径选择）

### 路径 A：修订文档服从实践（推荐 ✅）

**改动范围**：仅 `CONTRIBUTING.md` 一处 + 可选 `CLAUDE.md`/`AGENTS.md` 引用更新。

**新模板**：
```
## YYYY-MM-DD feature-slug

### 背景
[问题/需求描述]

### 改动清单
| 文件 | 操作 | 说明 |

### 设计决策
[关键决策 + 备选方案排除理由]

### 影响
[用户/系统影响]

- Human Owner: [姓名 或 待指派]
- Risk: [风险描述 或 无已知风险]
- Spec: docs/specs/[feature]/ (如适用)
```

**优点**：
- 符合"去繁就简"——不要求补齐 134 条历史
- 保留 Human Owner/Risk 作为审计关键字段
- 现有四段式自然延续，零迁移成本

**缺点**：
- 旧条目仍无 HO/Risk，历史审计受限（不可逆）

### 路径 B：强制回归 5 字段（不推荐 ❌）

需补齐 134 条历史记录的 Human Owner 和 Risk——多数已无法准确回溯（负责人可能已变动、风险可能已演化）。工时估算 ≥ 5 人日，且产出质量存疑。违反"克制"。

## 6. 与本次审阅计划的关系

- 本审阅计划本身（v1.0）的决策点 **D5** 要求"5 条 AI_CHANGELOG 记录"。鉴于 F-R15-1/2 揭示的系统性偏离，**D5 执行时必须采用路径 A 的新模板**，否则本次审阅的 5 条记录也会落入"0 合规"陷阱。
- 本次审阅产出的所有 `docs/review/2026-07-25-phase*.md` 报告均标注 `Human Owner：[NEEDS CLARIFICATION]`——这正是 F-R15-1 症状的自我示范，需用户在 Phase 5 收尾时指派。

## 7. 已验证合规项

| 项 | 证据 |
|----|------|
| ✅ CHANGELOG 持续更新 | 135 条记录，最新至 2026-07-25（本审阅窗口内） |
| ✅ Conventional Commits 风格标题 | 所有条目 `## YYYY-MM-DD: 描述` 格式一致 |
| ✅ 改动清单表格化 | 多数条目用 Markdown 表格呈现 文件/操作/说明 |
| ⚠️ 但 5 字段合规率 ≈ 0% | 见 §2 |

---

> 本报告统计基于 `docs/decisions/AI_CHANGELOG.md` 全文 grep + Python 正则审计，证据可复现。
