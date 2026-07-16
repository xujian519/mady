# Spec-Driven 开发流程

> 新功能和架构调整按四阶段文档链推进，确保 AI 参与时需求清晰、决策可追溯。

## 变更分档

### 小改动（Bug 修复、单函数调整）

口头描述或 Issue 描述 + Diff 审查即可，不走完整四步。

### 新功能 / 架构调整

在 `docs/specs/{feature-name}/` 下创建四个独立文档，纳入版本控制，每步都可人工审阅：

```
docs/specs/{feature-name}/
    01-proposal.md    背景、目标、成功标准（人写或人机共写，人必须过一遍）
    02-spec.md        输入输出、数据模型、接口定义、验证规则（AI 可初稿，人审）
    03-design.md      技术选型、架构图、关键算法、安全考量
    04-tasks.md       拆解为可执行的具体步骤，每步标注涉及文件范围
```

## 关键约定

- 遇到需求不明确的地方，AI 必须在 `02-spec.md` 里标记 `[NEEDS CLARIFICATION: ...]`，
  不能自己猜一个"看起来合理"的答案就往下走
- 每份 Spec 必须有一个 **Human Owner** 字段，写清楚这次改动谁是最终负责人，
  AI 参与撰写不改变这一点
- 四步文档全部走完、人工 Sign-off 后，再进入代码实现阶段

## Spec 索引

| 功能 | 目录 | 日期 | 阶段 | Human Owner |
|------|------|------|------|-------------|
| 心理引擎设计 | `docs/superpowers/specs/2026-07-10-nuochat-psychological-engine-design.md` | 2026-07-10 | 已完成 | — |
| 向量检索落地 | `docs/specs/vector-retrieval/` | 2026-07-13 | Spec 待 Sign-off | [待指派] |
| 现有技术检索阶段（disclosure retrieve_prior_art 节点） | `docs/specs/design-prior-art-retrieval-stage.md` | 2026-07-16 | 设计草案（已核对代码） | [待指派] |
| 获取规则阶段（知识资产接入与人工确认闭环） | `docs/specs/design-rule-acquisition-stage.md` | 2026-07-16 | 设计草案（已核对代码） | [待指派] |

> 阶段含义：Spec 待 Sign-off（四阶段文档已就绪，等人工审阅）→ 实现中 → 已完成。
