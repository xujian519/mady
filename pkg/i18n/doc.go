// Package i18n 提供轻量级国际化（i18n）支持。
//
// 本包使用 YAML 格式的翻译文件，基于 gopkg.in/yaml.v3，不依赖第三方 i18n 库。
// 翻译数据以线程安全的方式存储在 Catalog 中。
//
// 基本用法：
//
//	// 创建翻译目录并加载翻译文件
//	c := i18n.New(i18n.LocaleZhCN)
//	c.LoadDir("translations/")
//
//	// 获取翻译文本
//	msg := c.T("guardrail.disclaimer.standard")
//
//	// 使用全局快捷方式
//	i18n.SetGlobal(c)
//	msg := i18n.T("guardrail.disclaimer.standard")
//
// 翻译文件格式（YAML）：
//
//	# 简单键值对（默认语言为 zh-CN）
//	common.yes: "是"
//
//	# 按 locale 分组
//	greeting:
//	  zh-CN: "你好"
//	  en-US: "Hello"
//
// 目录结构：
//
//	translations/
//	├── zh-CN/
//	│   ├── guardrails.yaml
//	│   └── common.yaml
//	└── en-US/
//	    ├── guardrails.yaml
//	    └── common.yaml
//
// Catalog 行为：
//   - 如果 key 不存在，T() 返回 key 本身（开发期可见 fallback）
//   - 如果当前语言无翻译，自动回退到 zh-CN
//   - 支持 %s/%d 等 fmt.Sprintf 格式化参数
package i18n
