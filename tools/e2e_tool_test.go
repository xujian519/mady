package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// TestE2EPatentWebSearchTool 真实调用 patent_web_search 工具（需要 ego lite）。
func TestE2EPatentWebSearchTool(t *testing.T) {
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
