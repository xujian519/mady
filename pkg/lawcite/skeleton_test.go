package lawcite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSkeleton = `{
 "generatedFrom": "test",
 "count": 3,
 "laws": [
  {"name":"会计法","document":"中华人民共和国会计法","revision":"2024-06-28 通过","maxArticle":51,"transcribed":false},
  {"name":"中华人民共和国民法典","document":"中华人民共和国民法典","revision":"2020 通过","maxArticle":1260,"transcribed":true},
  {"name":"上海市专利保护条例","document":"","revision":"","maxArticle":40,"transcribed":false}
 ]
}`

func loadSample(t *testing.T) *Skeleton {
	t.Helper()
	skeleton, err := LoadSkeleton([]byte(sampleSkeleton), "test.json")
	if err != nil {
		t.Fatalf("LoadSkeleton: %v", err)
	}
	return skeleton
}

func TestSkeletonLookup(t *testing.T) {
	skeleton := loadSample(t)
	if skeleton.Len() != 3 {
		t.Fatalf("len = %d", skeleton.Len())
	}
	if law, ok := skeleton.Lookup("会计法"); !ok || law.MaxArticle != 51 {
		t.Fatalf("lookup 会计法 = %+v, %v", law, ok)
	}
	// 全称通过归一化命中简称。
	if law, ok := skeleton.Lookup("中华人民共和国会计法"); !ok || law.Name != "会计法" {
		t.Fatalf("lookup 全称 = %+v, %v", law, ok)
	}
	if _, ok := skeleton.Lookup("《会计法》"); !ok {
		t.Fatal("书名号应被归一化")
	}
	if _, ok := skeleton.Lookup("不存在的法"); ok {
		t.Fatal("不应命中的法律被命中")
	}
}

// 骨架只产生 ExistOnly，绝不产生 Verified。
func TestSkeletonNeverVerified(t *testing.T) {
	skeleton := loadSample(t)
	for _, name := range []string{"会计法", "中华人民共和国民法典", "上海市专利保护条例"} {
		res := skeleton.Resolve(name, 1)
		if res.Status == StatusVerified {
			t.Fatalf("%s 第1条 被判 Verified", name)
		}
		if res.Status != StatusExistOnly {
			t.Fatalf("%s 第1条 status=%v", name, res.Status)
		}
		if !strings.Contains(res.Reason, "条号骨架") {
			t.Fatalf("%s reason=%q", name, res.Reason)
		}
	}
}

func TestSkeletonOutOfRangeIsUnverified(t *testing.T) {
	skeleton := loadSample(t)
	// 超出骨架声明上限，但上限本身未核验，只能判未核验。
	res := skeleton.Resolve("会计法", 52)
	if res.Status != StatusUnverified {
		t.Fatalf("status=%v, want Unverified", res.Status)
	}
	if !strings.Contains(res.Reason, "上限未核验") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestSkeletonUnknownLaw(t *testing.T) {
	skeleton := loadSample(t)
	res := skeleton.Resolve("证券法", 1)
	if res.Status != StatusUnverified {
		t.Fatalf("status=%v", res.Status)
	}
	if !strings.Contains(res.Reason, "不在条号骨架中") {
		t.Fatalf("reason=%q", res.Reason)
	}
}

func TestSkeletonInvalidArticle(t *testing.T) {
	skeleton := loadSample(t)
	if res := skeleton.Resolve("会计法", 0); res.Status != StatusUnverified {
		t.Fatalf("status=%v", res.Status)
	}
	var nilSkeleton *Skeleton
	if res := nilSkeleton.Resolve("会计法", 1); res.Status != StatusUnverified {
		t.Fatalf("nil skeleton status=%v", res.Status)
	}
}

func TestSkeletonRejectsBadFile(t *testing.T) {
	if _, err := LoadSkeleton([]byte("not json"), "bad.json"); err == nil {
		t.Fatal("want error for invalid json")
	}
	badCount := `{"count":5,"laws":[{"name":"甲法","maxArticle":1}]}`
	if _, err := LoadSkeleton([]byte(badCount), "bad-count.json"); err == nil {
		t.Fatal("want error for count mismatch")
	}
	emptyName := `{"count":1,"laws":[{"name":" ","maxArticle":1}]}`
	if _, err := LoadSkeleton([]byte(emptyName), "empty-name.json"); err == nil {
		t.Fatal("want error for empty name")
	}
}

// 真实随包骨架：抽样 50 部法规，条号 1 必须命中且不产生 Verified。
func TestBundledSkeletonSample(t *testing.T) {
	path := filepath.Join("..", "..", "data", "law", "skeleton", "index.json.gz")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("随包骨架不存在：%v", err)
	}
	skeleton, err := LoadSkeletonFile(path)
	if err != nil {
		t.Fatalf("LoadSkeletonFile: %v", err)
	}
	if skeleton.Len() < 7900 {
		t.Fatalf("len = %d, want >= 7900", skeleton.Len())
	}

	// 取真实条目抽样（骨架按名称排序，每 150 部取一部）。
	names := make([]string, 0, 50)
	for raw := range skeleton.byName {
		names = append(names, raw)
	}
	if len(names) < 50 {
		t.Fatalf("names = %d", len(names))
	}
	step := len(names) / 50
	for i := range 50 {
		name := names[i*step]
		res := skeleton.Resolve(name, 1)
		if res.Status != StatusExistOnly {
			t.Fatalf("%s 第1条 status=%v reason=%s", name, res.Status, res.Reason)
		}
	}

	// 民法典在骨架里标记为已转录，但骨架仍不能判 Verified。
	if law, ok := skeleton.Lookup("中华人民共和国民法典"); !ok || !law.Transcribed {
		t.Fatalf("民法典 = %+v, %v", law, ok)
	}
	if res := skeleton.Resolve("中华人民共和国民法典", 1); res.Status == StatusVerified {
		t.Fatal("骨架不得产生 Verified")
	}
}
