// Package loader 提供外部知识加载器，支持从 Wiki（百科）、Patent（专利）
// 和 Legal（法律法规）三种数据源加载结构化知识到知识库中。
//
// 加载器：
//   - WikiLoader: 百科知识加载（Wikipedia/Mooc 等），含卡片索引和过滤
//   - PatentLoader: 专利文献加载（标题/摘要/权利要求）
//   - LegalLoader: 法律法规加载（法条/司法解释/案例）
//
// 主要类型：
//   - WikiLoader: 百科加载器，支持元数据提取和重索引
//   - PatentLoader: 专利加载器
//   - LegalLoader: 法律数据加载器
//
// 使用示例：
//
//	loader := loader.NewWikiLoader(store)
//	docs, _ := loader.Load(ctx, "专利权")
package loader
