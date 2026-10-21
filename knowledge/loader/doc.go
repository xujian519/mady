// Package loader 提供 Obsidian Wiki 知识加载：遍历 Wiki/、cards/、patent-cards/，
// 提取标题、来源、法条、标签等元数据后写入 knowledge.Store。
//
// 主要类型：
//   - WikiLoader: Wiki 导入器（过滤索引页/片段页，支持 card-index.json 增强）
//   - WikiFilter: 导入过滤规则
//   - LawArticleIndex: 从法条文本构建条款主题索引
//
// 使用示例：
//
//	l := loader.NewWikiLoader(store, wikiPath)
//	stats, err := l.ImportWiki()
package loader
