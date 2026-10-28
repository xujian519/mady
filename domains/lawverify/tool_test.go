package lawverify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/pkg/lawcite"
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
	if !strings.Contains(got[0].Reason, "尚无随包索引或条号骨架") {
		t.Fatalf("reason = %q", got[0].Reason)
	}
}

const sampleSkeletonJSON = `{
 "generatedFrom": "test",
 "count": 3,
 "laws": [
  {"name":"会计法","document":"中华人民共和国会计法","revision":"","maxArticle":51,"transcribed":false},
  {"name":"商标法","document":"中华人民共和国商标法","revision":"","maxArticle":73,"transcribed":true},
  {"name":"上海市专利保护条例","document":"","revision":"","maxArticle":40,"transcribed":false}
 ]
}`

func callWithSkeleton(t *testing.T, args string) (map[string]any, error) {
	t.Helper()
	skeleton, err := lawcite.LoadSkeleton([]byte(sampleSkeletonJSON), "test.json")
	if err != nil {
		t.Fatalf("LoadSkeleton: %v", err)
	}
	tool := NewTool(Config{Skeleton: skeleton})
	out, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func TestSkeletonFallback(t *testing.T) {
	result, err := callWithSkeleton(t, `{"law":"会计法","article":1}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "条号存在但未转录" {
		t.Fatalf("status = %q, reason = %q", got[0].Status, got[0].Reason)
	}
	if got[0].Source != "skeleton" {
		t.Fatalf("source = %q", got[0].Source)
	}
	if got[0].Citation != "《会计法》第1条" {
		t.Fatalf("citation = %q", got[0].Citation)
	}
	if got[0].Text != "" {
		t.Fatalf("骨架不应带条文：%q", got[0].Text)
	}
}

func TestSkeletonNeverVerifiedEvenWhenTranscribed(t *testing.T) {
	// 商标法在骨架里标记为带条文转录，但骨架不携带条文，仍只能判 ExistOnly。
	result, err := callWithSkeleton(t, `{"law":"商标法","article":57}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "条号存在但未转录" {
		t.Fatalf("status = %q", got[0].Status)
	}
	if got[0].Source != "skeleton" {
		t.Fatalf("source = %q", got[0].Source)
	}
}

func TestSkeletonOutOfRange(t *testing.T) {
	result, err := callWithSkeleton(t, `{"law":"会计法","article":52}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "未核验" || !strings.Contains(got[0].Reason, "上限未核验") {
		t.Fatalf("status=%q reason=%q", got[0].Status, got[0].Reason)
	}
}

func TestPreciseIndexWinsOverSkeleton(t *testing.T) {
	// 专利法同时存在于精校索引与骨架时，必须以精校索引为准（判 Verified）。
	skeleton, err := lawcite.LoadSkeleton([]byte(`{"count":1,"laws":[{"name":"专利法","maxArticle":82}]}`), "t.json")
	if err != nil {
		t.Fatalf("LoadSkeleton: %v", err)
	}
	tool := NewTool(Config{Skeleton: skeleton})
	out, err := tool.Func(context.Background(), json.RawMessage(`{"law":"专利法","article":22}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := out.(map[string]any)["findings"].([]finding)
	if got[0].Status != "已核验" || got[0].Source != "index" {
		t.Fatalf("status=%q source=%q", got[0].Status, got[0].Source)
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
	// 无法识别的法律名不再直接报错：交给骨架判定，骨架也没有时返回未核验。
	result, err := call(t, `{"law":"不存在的法","article":1}`)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	got := findings(t, result)
	if got[0].Status != "未核验" {
		t.Fatalf("status = %q", got[0].Status)
	}
	if got[0].Source != "none" {
		t.Fatalf("source = %q", got[0].Source)
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
