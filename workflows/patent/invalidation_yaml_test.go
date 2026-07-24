package patent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/xujian519/mady/graph"
)

func TestBuildInvalidationGraphFromYAML(t *testing.T) {
	tmpFile := "/tmp/test-inv-orch.yaml"
	yamlContent := `
discoveryStages:
  - name: "测试阶段"
    goal: "测试目标"
    suggestions:
      - "建议1"
      - "建议2"
  - name: "测试阶段2"
    goal: "测试目标2"
    suggestions:
      - "建议3"
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	compiled, err := BuildInvalidationGraphFromYAML(tmpFile)
	if err != nil {
		t.Fatalf("BuildInvalidationGraphFromYAML: %v", err)
	}
	if compiled == nil {
		t.Fatal("compiled graph is nil")
	}

	input := `1. 一种测试方法。
	请求人主张该专利不符合专利法第22条第2款新颖性。`

	out, err := compiled.Run(context.Background(), graph.PregelState{
		InvStateInput: input,
	})
	if err != nil {
		t.Fatalf("graph run: %v", err)
	}

	output := out.GetString(InvStateOutput)
	if !strings.Contains(output, "建议1") {
		t.Error("YAML suggestions should be injected, missing '建议1'")
	}
	if !strings.Contains(output, "编排建议") {
		t.Error("should contain '编排建议' header")
	}
}

func TestBuildInvalidationGraphFromYAML_InvalidPath(t *testing.T) {
	_, err := BuildInvalidationGraphFromYAML("/nonexistent/file.yaml")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}
