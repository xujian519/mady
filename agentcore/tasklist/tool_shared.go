package tasklist

// tool_shared.go — shared helpers used by tool_create/get/update/list.

import "strconv"

// isValidTaskID 检查 ID 是否为纯数字字符串。
// 任务 ID 由 Store.NextID 分配，始终为正整数字符串（"1"、"2"、…）。
func isValidTaskID(id string) bool {
	if id == "" {
		return false
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}
