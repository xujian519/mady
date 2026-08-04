// commitlint 配置 — 强制 Conventional Commits，并放宽 subject 长度以兼容中文描述。
// 与 .pre-commit-config.yaml 的 commitlint-pre-commit-hook 共用本配置。
// 注意：CI 使用 wagoid/commitlint-github-action@v6，要求 .mjs 扩展名。
export default {
	extends: ['@commitlint/config-conventional'],
	rules: {
		// 中文 subject 按字符计数容易偏长，放宽到 120（默认 100）。
		'header-max-length': [2, 'always', 120],
		// 允许中文 scope 与 subject；主体可含中日韩文字及标点。
		'subject-case': [0],
		// dependabot gomod 生成的 commit 使用 deps(deps): 前缀，补充到允许的类型枚举。
		// （与 .github/dependabot.yml 的 commit-message.prefix: "deps" 保持一致）
		'type-enum': [2, 'always', [
			'build', 'chore', 'ci', 'docs', 'feat', 'fix', 'perf',
			'refactor', 'revert', 'style', 'test', 'deps',
		]],
	},
};
