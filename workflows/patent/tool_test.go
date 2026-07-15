package patent_test

import (
	"testing"

	"github.com/xujian519/mady/workflows/patent"
)

func TestNewPatentNoveltyTool(t *testing.T) {
	tool := patent.NewPatentNoveltyTool()
	if tool == nil {
		t.Fatal("NewPatentNoveltyTool() returned nil")
	}
	if tool.Name == "" {
		t.Fatal("tool has empty Name")
	}
	if tool.Description == "" {
		t.Fatal("tool has empty Description")
	}
	if tool.Parameters == nil {
		t.Fatal("tool has nil Parameters")
	}
}

func TestNewPatentNoveltyTool_HasFunc(t *testing.T) {
	tool := patent.NewPatentNoveltyTool()
	if tool.Func == nil {
		t.Fatal("tool Func is nil")
	}
}
