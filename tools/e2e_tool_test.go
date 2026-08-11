package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// TestE2EPatentWebSearchTool 真实调用 patent_web_search 工具（需要 ego lite）。
// e2e 测试依赖外部浏览器运行时，默认跳过；设置 MADY_E2E=1 显式开启。
func TestE2EPatentWebSearchTool(t *testing.T) {
	if os.Getenv("MADY_E2E") != "1" {
		t.Skip("e2e tests disabled: set MADY_E2E=1 to run real ego-browser tests")
	}
	tool := NewPatentWebSearchTool(nil)
	if tool == nil {
		t.Skip("ego-browser not available")
	}
	args, _ := json.Marshal(map[string]any{"query": "深度学习图像识别", "max_results": 3})
	raw, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("Func: %v", err)
	}
	data, _ := json.Marshal(raw)
	fmt.Printf("result: %.600s\n", data)
}
