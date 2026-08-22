// Package caseindex 提供基于 SQLite 的案件索引库：案件主记录、文档、
// 路径与生命周期事件的持久化，以及 FTS 全文检索。它是案件管理域的
// 持久化 adapter 子包——SQL 与驱动依赖收敛于此，领域根包通过
// caseindex_alias.go 的类型别名面向本包编程。
package caseindex
