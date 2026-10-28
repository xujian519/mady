// Package literature 实现学术文献的多源检索与全文下载。
//
// 四个免费公开源：OpenAlex、Semantic Scholar、arXiv、Crossref。共享一个 HTTP
// 客户端：按主机礼貌限速（不同主机互不排队）、进程内 GET 短 TTL 缓存（LRU 有界，
// 空响应与非预期格式不入缓存）、超时与错误分类。连接器只做「请求 + 解析」，
// 领域判定（PDF 魔数、大小、Content-Type）集中在下载层。
package literature
