// Package loader 提供 Wiki（百科）知识加载器，支持从 Obsidian wiki 目录
// 加载结构化知识到知识库中。
//
// 加载器：
//   - WikiLoader: 百科知识加载，含卡片索引和过滤
//
// 主要类型：
//   - WikiLoader: 百科加载器，支持元数据提取和重索引
//
// 使用示例：
//
//	loader := loader.NewWikiLoader(store, wikiPath)
//	docs, _ := loader.Load(ctx, "专利权")
package loader
