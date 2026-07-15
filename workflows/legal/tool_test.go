package legal_test

import (
	"testing"

	"github.com/xujian519/mady/workflows/legal"
)

func TestNewLegalComparisonTool(t *testing.T) {
	tool := legal.NewLegalComparisonTool()
	if tool == nil {
		t.Fatal("NewLegalComparisonTool() returned nil")
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

func TestNewLegalComparisonTool_HasFunc(t *testing.T) {
	tool := legal.NewLegalComparisonTool()
	if tool.Func == nil {
		t.Fatal("tool Func is nil")
	}
}
