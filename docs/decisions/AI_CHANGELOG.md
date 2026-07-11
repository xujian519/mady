# AI 决策变更日志

> 本文件记录 AI 协助开发过程中的关键设计决策。
> 每条功能改动完成后必须追加记录，不是可选项。

## 格式

```
## YYYY-MM-DD feature-slug
- Decision: [做了什么设计决策]
- Reason: [为什么这么选择，而非其他方案]
- Risk: [已知风险或局限性]
- Human Owner: [负责人姓名]
- Spec: docs/specs/[feature]/ (如适用)
```

## 记录

### 2026-07-11 dev-standard-init

- Decision: 建立人机协助开发规范体系（AGENTS.md + AI_CHANGELOG.md + CI 质量门 + 敏感路径检测）
- Reason: 随着 AI 编码助手在项目中参与度提高，需要标准化协作流程，确保安全红线可控。
  选择了文档约定 + 自动化 CI 的组合方式，而非纯人工流程，兼顾效率与安全
- Risk: 新增文档和 CI 步骤增加了提交前的操作性负担，但可以通过 pre-commit hook 和 CI 自动化缓解。
  AGENTS.md 与 CLAUDE.md 双文件策略需要维护两份内容，但两者受众不同（跨工具 vs Claude Code 专用），
  重叠部分较少
- Human Owner: 待指定
- Spec: 无（纯流程/文档变更）
