# AI_CHANGELOG 拆分设计

## 背景

`docs/decisions/AI_CHANGELOG.md` 当前 9,567 行 / 639KB / 260 条记录，单文件平铺，
AI 智能体每次需加载全文才能定位目标条目，上下文浪费严重。

## 设计目标

面向 AI 智能体优化：索引优先、按需加载、结构化可过滤。

## 目录结构

```
docs/decisions/ai-changelog/       ← 新目录
  INDEX.json                        ← 结构化索引（AI 主入口）
  2026-07-31.md                     ← 每日详细记录
  2026-07-30.md
  ...

docs/decisions/AI_CHANGELOG.md      ← 保留为重定向文件
```

## INDEX.json Schema

```json
{
  "version": 1,
  "updated": "2026-07-31T20:51:00+08:00",
  "total": 260,
  "entries": [
    {
      "date": "2026-07-31",
      "type": "feat",
      "scope": "retrieval",
      "title": "ego-browser 集成",
      "file": "2026-07-31.md",
      "line": 450
    }
  ]
}
```

- `type`: conventional commit 类型（feat/fix/refactor/docs/test/chore/style/perf）
- `scope`: 影响模块（tui/agentcore/domains/retrieval/desktop/*）
- `file` + `line`: 精准定位到日期文件中的行号

## 日期文件格式

```markdown
# 2026-07-31

## type(scope): title

**背景**：...
**改动清单**：...
**验证**：...
**影响**：...

---

## type(scope): title
...
```

- 文件级标题 `#` 表示日期
- 条目间 `---` 分隔
- 标题统一 `type(scope): description` 格式

## 迁移

一次性 Go 脚本 `scripts/migrate-changelog/main.go`：
1. 解析旧文件按 `## YYYY-MM-DD:` 边界拆分
2. 提取 type/scope（正则 + 启发式）
3. 生成 INDEX.json
4. 按日期分组写入 `.md` 文件
5. 验证条目总数一致

## 维护

脚本追加 `scripts/changelog/main.go`：
```bash
go run scripts/changelog/main.go \
  --type=feat --scope=tui \
  --title="标题" --body="详细内容"
```
自动更新 INDEX.json + 对应日期文件。

## 约束

- 新条目必须通过脚本追加，不允许手写
- CI 可选校验步骤确保 INDEX.json 与日期文件一致
- 原 AI_CHANGELOG.md 保留为重定向文件
