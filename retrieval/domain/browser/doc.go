// Package browser 提供基于 ego-browser CLI（ego lite 浏览器）的
// domain.DomainRetriever 实现，支持从在线专利数据库（Google Patents /
// CNIPA 中国专利 / Espacenet）实时检索，弥补本地静态语料无法覆盖的
// 最新专利数据。
//
// 与 sqlite 子包（本地 FTS5 语料）互补：sqlite 快而静态，browser 慢而实时。
// browser 检索器仅在 ego lite 可用时构造（工厂返回 nil 静默降级），
// 不引入新依赖，通过 exec 调用 `ego-browser nodejs` heredoc 脚本驱动
// 真实浏览器。
//
// 使用方式：
//
//	cfg := browser.DefaultConfig()
//	cfg.EgoBrowserPath = "/path/to/ego-browser"
//	r := browser.NewGooglePatentsRetriever(cfg) // nil 当 ego-browser 不可用
//	results, err := r.Search(ctx, query)
//
// 组合检索器（本地 + 在线源）见 CompositeRetriever；在线检索的启用/关闭
// 由装配层控制（bootstrap，MADY_BROWSER_RETRIEVERS=off 关闭）。
package browser
