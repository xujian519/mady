package lawverify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func call(t *testing.T, args string) (map[string]any, error) {
	t.Helper()
	tool := NewTool(Config{})
	out, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	result, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("output type = %T", out)
	}
	return result, nil
}

func findings(t *testing.T, result map[string]any) []finding {
	t.Helper()
	items, ok := result["findings"].([]finding)
	if !ok {
		t.Fatalf("findings type = %T", result["findings"])
	}
	return items
}

func TestStructuredVerified(t *testing.T) {
	result, err := call(t, `{"law":"专利法","article":22,"paragraph":3}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if len(got) != 1 || got[0].Status != "已核验" {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].SourceDoc == "" || got[0].VerifiedOn == "" || got[0].Text == "" {
		t.Fatalf("已核验条目缺少条文或来源：%+v", got[0])
	}
	if !strings.Contains(result["summary"].(string), "1 条已核验") {
		t.Fatalf("summary = %q", result["summary"])
	}
}

func TestTextExtraction(t *testing.T) {
	result, err := call(t, `{"text":"依据《专利法》第22条第3款，该方案具备创造性。"}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if len(got) != 1 {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Citation != "专利法第22条第3款" || got[0].Status != "已核验" {
		t.Fatalf("finding = %+v", got[0])
	}
}

func TestUntranscribedArticle(t *testing.T) {
	result, err := call(t, `{"law":"专利法","article":1}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "条号存在但未转录" {
		t.Fatalf("status = %q, reason = %q", got[0].Status, got[0].Reason)
	}
	if got[0].Text != "" {
		t.Fatalf("未转录条目不应带条文：%q", got[0].Text)
	}
}

func TestWithoutBundledIndex(t *testing.T) {
	result, err := call(t, `{"law":"商标法","article":57}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "未核验" {
		t.Fatalf("status = %q", got[0].Status)
	}
	if !strings.Contains(got[0].Reason, "尚无随包条文索引") {
		t.Fatalf("reason = %q", got[0].Reason)
	}
}

func TestOutOfRange(t *testing.T) {
	result, err := call(t, `{"law":"专利法","article":83}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := findings(t, result); got[0].Status != "条号越界" {
		t.Fatalf("status = %q", got[0].Status)
	}
}

func TestNoCitation(t *testing.T) {
	if _, err := call(t, `{"text":"本方案具备新颖性。"}`); err == nil {
		t.Fatal("want error for text without citation")
	}
}

func TestUnknownLawName(t *testing.T) {
	if _, err := call(t, `{"law":"民法典","article":1}`); err == nil {
		t.Fatal("want error for unknown law")
	}
}

func TestToolMetadata(t *testing.T) {
	tool := NewTool(Config{})
	if tool.Name != ToolName || !tool.ReadOnly {
		t.Fatalf("name=%q readonly=%v", tool.Name, tool.ReadOnly)
	}
	if tool.Description == "" {
		t.Fatal("description is empty")
	}
}
