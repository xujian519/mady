## 变更摘要

<!-- 请简要描述此 PR 做了什么 -->

## 变更类型

- [ ] 🐛 Bug 修复
- [ ] ✨ 新功能
- [ ] 📝 文档更新
- [ ] ♻️ 代码重构
- [ ] 🧪 测试
- [ ] 🔧 基础设施 / CI
- [ ] 其他：___

## 关联 Issue

<!-- 使用 "Closes #123" 或 "Fixes #123" 关联 issue -->

## AI 参与程度

- [ ] 纯人类撰写（无 AI 协助）
- [ ] AI 辅助（AI 提供思路/代码片段，人类主导）
- [ ] AI 主导（AI 编写主要代码，人类审查后提交）
- [ ] AI 独立撰写（AI 产出，人类仅形式审查）

## 涉及安全红线的改动

> 安全红线详见 [CONTRIBUTING.md](CONTRIBUTING.md)「代码审查分级」章节和 [AGENTS.md](AGENTS.md)「安全敏感路径」章节

- [ ] 无
- [ ] 有（说明：__________，已请 __________ 复核）

## 检查清单

- [ ] 代码通过 `make verify`（lint + build + test-race，覆盖根模块 + tools/ 子模块）
- [ ] 新功能包含测试（含竞态检测 `-race`）
- [ ] 文档已更新（README、代码注释等）
- [ ] CHANGELOG.md 已更新（记录在 `[Unreleased]` 下）
- [ ] 如涉及 AI 参与的功能变更，已更新 `docs/decisions/AI_CHANGELOG.md`
- [ ] 无新的编译警告

## 测试计划

<!-- 描述如何验证此变更 -->
1. 运行 `make verify`（覆盖 lint、build、test-race、root + tools）
2. 运行示例验证：`go run ./example/cli-chat/`

## 截图 / 录屏（如适用）

<!-- 对 UI 相关变更，提供截图或录屏 -->

## 额外说明

<!-- 任何审查者需要了解的其他信息 -->
