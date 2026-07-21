# Mady 开发者门禁速查表

> 一页掌握从克隆到提交通关的全部门禁要点。

---

## 首次克隆

```bash
# 安装钩子（必须，否则本地门禁不完整）
pre-commit install
pre-commit install --hook-type commit-msg
```

## 日常开发循环

```bash
# 快速验证（日常）
make all          # vet + build + test（不含 race）

# 提交前标准（必须）
make verify       # lint + build + test-race → 覆盖根模块 + tools/ 子模块
```

## 提交信息

```
<type>: <简短描述>

类型: feat / fix / docs / test / refactor / chore / style / perf
长度: 首行 ≤120 字符（已为中文放宽）
```

## PR 提交

1. PR 模板中选择 AI 参与级别
2. 勾选涉红线变更（如有）
3. 确保 `make verify` 通过
4. 更新 `CHANGELOG.md` + `docs/decisions/AI_CHANGELOG.md`（如涉及 AI 决策）

## 安全红线

| 类别 | 要求 |
|------|------|
| API 密钥 | 环境变量注入，禁止硬编码 |
| 敏感路径 | 编辑 `agentcore/handoff.go` 等路径需 L4 审查 |
| 生产数据 | 禁止提交真实案件/当事人数据 |
| 分级审查 | L1 无 review → L4 需两人 sign-off |

完整敏感路径清单：`scripts/check-sensitive-paths.sh`

## 相关文档速查

| 目的 | 文档 |
|------|------|
| Go 代码规范 | `docs/GO-DEVELOPMENT-STANDARDS.md` |
| 贡献流程 | `CONTRIBUTING.md` |
| 安全策略 | `SECURITY.md` |
| 措辞风格 | `docs/tone-style-guide.md` |
| 数据隐私 | `docs/data-privacy-standards.md` |
| AI 决策日志 | `docs/decisions/AI_CHANGELOG.md` |
