package util

// TruncateRunes 按 rune 安全截断字符串到 maxLen 个字符：超出时截取前缀并
// 追加省略号"…"（省略号不计入 maxLen）。不做 Unicode 空白修剪——调用方
// 自行决定是否 TrimSpace。maxLen ≤ 0 时返回省略号（防御负值）。
//
// 这是全仓库统一的"单端 rune 截断"实现（claimchart 引文展示、slop 命中句
// 展示等消费方共用）；头尾双端语义请用 agentcore.SnipMessageContent。
func TruncateRunes(s string, maxLen int) string {
	r := []rune(s)
	if maxLen <= 0 {
		return "…"
	}
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
