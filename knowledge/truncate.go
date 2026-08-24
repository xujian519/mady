package knowledge

import "fmt"

// TruncateRunes 按 rune 截断长文本，超限时在末尾附加中文省略提示。
// 作为知识输出（wiki 卡片/判例/图谱详情）的统一截断入口，避免各工具
// 重复实现截断逻辑。
func TruncateRunes(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + fmt.Sprintf("\n…（截断，共 %d 字符）", len(r))
}
