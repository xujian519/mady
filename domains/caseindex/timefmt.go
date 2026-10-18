package caseindex

// timefmt.go 统一案件索引库时间列的文本格式约定（与 domains/sqlite 保持同一约定）。
//
// 写入侧一律使用 UTC RFC3339（"2026-10-18T15:04:05Z"）；读取侧额外兼容
// 历史遗留的 SQLite datetime('now') 文本（"2026-10-18 15:04:05"，UTC 无时区）。
// 历史数据里 case_paths.created_at / case_events.event_date 均由建表默认值
// datetime('now') 生成，按 RFC3339 解析会失败并退化为零值时间。

import "time"

// legacyTimeLayout 是 SQLite datetime('now') 的输出格式（UTC，无时区标识）。
const legacyTimeLayout = "2006-01-02 15:04:05"

// nowText 返回当前时间的 UTC RFC3339 文本，用于写入时间列。
func nowText() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// parseTimeText 解析时间列文本，兼容 RFC3339 与遗留 datetime 格式。
// 空串或无法识别时返回零值，调用方按"未设置"处理。
func parseTimeText(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(legacyTimeLayout, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
