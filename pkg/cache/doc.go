// Package cache 提供有界 LRU 与「LRU + TTL + 并发合并」的异步结果缓存。
//
// 用于外部检索这类「命中即热、键空间无自然上界、失败不能污染缓存」的场景：
// LRU 负责容量，TTL 负责时效，in-flight 合并让同一键的并发请求只执行一次加载。
// 只有加载成功且通过 cacheable 判定的结果才写入缓存；错误与空结果原样透传。
package cache
