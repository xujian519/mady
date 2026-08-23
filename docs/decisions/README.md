# docs/decisions 目录说明

> 本目录是 Mady 的决策与评审记录仓库。按内容类型分三块，新增文档时先判断属于哪块：

## 1. `ai-changelog/` — AI 参与的功能变更（日期流）

- **唯一权威入口**：`ai-changelog/INDEX.json`（结构化索引：日期/类型/范围/标题）
- 文件格式：`YYYY-MM-DD.md`，一个文件内可含多条 `## type(scope): title` 条目
- **新增条目必须用脚本追加，禁止手写 Markdown**：
  ```bash
  go run scripts/changelog/main.go \
    --type=feat|fix|refactor|docs|test|chore|style|perf \
    --scope=<模块名> --title="变更标题" \
    --body="详细内容（背景/改动清单/验证/影响）"
  ```
- 条目格式门禁：`scripts/check-aichangelog-format.sh`（pre-commit 接线，
  格式断言见 doc-check 断言 10；门禁自测见 `scripts/gatechecks/`）
- 历史说明：v0.4.0 前此流为单文件 `AI_CHANGELOG.md`，2026-07 迁移为目录日期流；
  归档件在 `archive/`

## 2. `archive/` — 归档

- 已被日期流取代的历史记录（如旧 `AI_CHANGELOG.md` 迁移产物）
- 只读归档，不再追加新内容

## 3. 根级散文件 — 评审报告与单次决策记录

- 会议/审查产物（如 `a2ui-agui-review-report.md`、`REVIEW_REPORT_*.md`）与
  一次性决策记录（如 `p3-features-decision.md`）
- 不按日期流组织、不需要脚本追加；保持文件名自解释即可

## 判断标准（新文档放哪）

| 内容 | 去处 |
|------|------|
| AI 参与的功能变更（每次提交对应） | `ai-changelog/`（脚本追加） |
| 架构选型决策（多备选、不可轻易反转） | `docs/adr/`（ADR-00XX） |
| 新功能四阶段设计链 | `docs/specs/{feature-name}/` |
| 评审报告 / 一次性决策 | 本目录根级散文件 |
