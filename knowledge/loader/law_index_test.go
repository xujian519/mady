package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSplitTitleTopics 验证 H3 标题 → 主题关键词的切分规则：
// 整串保留 + 按「与/及/和/、」切子短语 + ≥2 字过滤 + 去重。
func TestSplitTitleTopics(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		want    []string // 必须包含
		notWant []string // 不得包含
		wantLen int      // 期望长度（0 = 不检查）
	}{
		{
			name:    "连词切分：禁止重复授权与先申请原则",
			title:   "禁止重复授权与先申请原则",
			want:    []string{"禁止重复授权与先申请原则", "禁止重复授权", "先申请原则"},
			wantLen: 3,
		},
		{
			name:    "无连词整串保留：发明创造的定义",
			title:   "发明创造的定义",
			want:    []string{"发明创造的定义"},
			wantLen: 1,
		},
		{
			name:    "顿号与连字混合：新颖性、创造性和实用性",
			title:   "新颖性、创造性和实用性",
			want:    []string{"新颖性、创造性和实用性", "新颖性", "创造性", "实用性"},
			wantLen: 4,
		},
		{
			name:    "单字子短语被过滤",
			title:   "奖与酬",
			want:    []string{"奖与酬"},
			notWant: []string{"奖", "酬"},
			wantLen: 1,
		},
		{
			name:    "切分结果与整串相同则去重",
			title:   "撤回",
			want:    []string{"撤回"},
			wantLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTitleTopics(tt.title)
			for _, w := range tt.want {
				if !containsString(got, w) {
					t.Errorf("splitTitleTopics(%q) 缺少关键词 %q，得到 %v", tt.title, w, got)
				}
			}
			for _, nw := range tt.notWant {
				if containsString(got, nw) {
					t.Errorf("splitTitleTopics(%q) 不应包含 %q，得到 %v", tt.title, nw, got)
				}
			}
			if tt.wantLen > 0 && len(got) != tt.wantLen {
				t.Errorf("splitTitleTopics(%q) 长度 = %d，期望 %d：%v", tt.title, len(got), tt.wantLen, got)
			}
		})
	}
}

// TestBuildLawArticleIndex 在临时目录构造拆分法条文件，
// 验证文件过滤（目录/实施细则/part 文件）、H3 解析、合并与上限。
func TestBuildLawArticleIndex(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("写测试文件 %s 失败：%v", name, err)
		}
	}

	write("专利法-2020-拆分-01-第一章-总-则.md",
		"# 第一章\n\n### 第一条 立法宗旨\n正文。\n### 第九条 禁止重复授权与先申请原则\n正文。\n")
	write("专利法-2020-拆分-02-第三章-专利的申请.md",
		"### 第二十二条 新颖性、创造性和实用性\n正文。\n")
	// 以下文件必须被排除/忽略：
	write("专利法-2020-拆分-01-分拆目录.md", "### 第九十九条 不应被索引\n")
	write("专利法实施细则-2023-总则与申请.md", "### 第一条 细则不应被索引\n")
	write("专利法-2020-拆分-01-part.md", "# 分卷说明（无 H3 法条标题）\n")

	idx, err := BuildLawArticleIndex(dir)
	if err != nil {
		t.Fatalf("BuildLawArticleIndex 失败：%v", err)
	}

	if got := idx.ArticleCount(); got != 3 {
		t.Errorf("ArticleCount = %d，期望 3（目录/细则文件应被排除）", got)
	}
	if got := idx.MaxArticle(); got != 22 {
		t.Errorf("MaxArticle = %d，期望 22", got)
	}

	kw9, ok := idx.Topics(9)
	if !ok {
		t.Fatal("Topics(9) 未覆盖")
	}
	for _, w := range []string{"禁止重复授权", "先申请原则"} {
		if !containsString(kw9, w) {
			t.Errorf("Topics(9) 缺少 %q：%v", w, kw9)
		}
	}

	if _, ok := idx.Topics(99); ok {
		t.Error("Topics(99) 不应存在（分拆目录文件应被排除）")
	}
	if kw1, ok := idx.Topics(1); !ok || len(kw1) != 1 || kw1[0] != "立法宗旨" {
		t.Errorf("Topics(1) = %v，期望 [立法宗旨]", kw1)
	}
}

// TestBuildLawArticleIndex_Empty 目录无法索引到法条时必须报错
// （装配侧据此降级为仅 S1 源，而非带病启动）。
func TestBuildLawArticleIndex_Empty(t *testing.T) {
	if _, err := BuildLawArticleIndex(t.TempDir()); err == nil {
		t.Error("空目录应返回错误")
	}
	if _, err := BuildLawArticleIndex(filepath.Join(t.TempDir(), "nonexistent")); err == nil {
		t.Error("不存在目录应返回错误")
	}
}

// TestBuildLawArticleIndex_RealWiki 条件测试：真实 wiki 法条目录存在时，
// 断言《专利法（2020）》82 条全覆盖、第 9 条含「先申请」词。
func TestBuildLawArticleIndex_RealWiki(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法定位用户目录")
	}
	dir := filepath.Join(home, ".mady", "knowledge", "wiki", "法律法规", "法律")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("真实 wiki 法条目录不存在，跳过：%s", dir)
	}

	idx, err := BuildLawArticleIndex(dir)
	if err != nil {
		t.Fatalf("BuildLawArticleIndex 失败：%v", err)
	}
	if got := idx.ArticleCount(); got != 82 {
		t.Errorf("ArticleCount = %d，期望 82（专利法 2020 全条数）", got)
	}
	if got := idx.MaxArticle(); got != 82 {
		t.Errorf("MaxArticle = %d，期望 82", got)
	}
	kw9, ok := idx.Topics(9)
	if !ok || !containsString(kw9, "先申请原则") {
		t.Errorf("Topics(9) = %v，期望含「先申请原则」", kw9)
	}
	kw22, ok := idx.Topics(22)
	if !ok || !containsString(kw22, "创造性") {
		t.Errorf("Topics(22) = %v，期望含「创造性」", kw22)
	}
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
