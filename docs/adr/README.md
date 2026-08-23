# ADR 索引（架构决策记录）

> 本目录存放 Mady 的架构决策记录（Architecture Decision Records）。
> 决策有据可查：为什么选这个方案、备选方案是什么、后果是什么。

## 命名与编号规则

- 文件名格式：`NNN-<短横线主题>.md`（三位数字前缀 + 主题短横线）。
  命名格式由 `scripts/check-doc-consistency.py` 断言（doc-check 断言 12），
  新增 ADR 请勿偏离
- `0000` 保留给模板（`0000-adr-template.md`），实际决策从 `0001` 起
- 编号按序递增；跳号不违规（历史遗留 004/005 未建，无需补号），
  但新建决策必须大于现存最大编号

## 索引

| 编号 | 主题 | 文件 |
|------|------|------|
| 0000 | 模板 | [0000-adr-template.md](0000-adr-template.md) |
| 0001 | 采用分层架构 | [0001-use-layered-architecture.md](0001-use-layered-architecture.md) |
| 0002 | 图引擎采用 DAG + Pregel 双模式设计 | [0002-graph-engine-design.md](0002-graph-engine-design.md) |
| 0003 | 统一运行时（Unified Runtime）设计 | [0003-unified-runtime-design.md](0003-unified-runtime-design.md) |
| 0006 | Memory 模块设计 | [006-memory-module.md](006-memory-module.md) |
| 0007 | ContextBuilder — 统一上下文组装 | [007-context-builder.md](007-context-builder.md) |

## 何时写 ADR

- 跨模块的技术选型（图引擎、运行时、协议）
- 有多个备选方案的架构决策，且选择不可轻易反转
- 单一模块内的常规实现细节不需要 ADR——用 `docs/specs/` 四阶段流程或代码注释即可

相关：功能/架构变更的日常记录走 `docs/decisions/ai-changelog/`（见
[docs/decisions/README.md](../decisions/README.md)）；新功能的设计链见
[docs/specs/README.md](../specs/README.md)。
