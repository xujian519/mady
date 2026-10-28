package legal

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// fixtureSchema 复刻 laws-full.db 的关键结构：law/category + trigram FTS5 索引。
var fixtureSchema = []string{
	`CREATE TABLE category (
		id INTEGER NOT NULL PRIMARY KEY, name TEXT NOT NULL, folder TEXT NOT NULL DEFAULT '',
		"isSubFolder" INTEGER NOT NULL DEFAULT 0, "group" TEXT, "order" integer)`,
	`CREATE TABLE law (
		id TEXT NOT NULL PRIMARY KEY, level TEXT NOT NULL, name TEXT NOT NULL, filename TEXT,
		publish DATE, expired INTEGER NOT NULL, category_id INTEGER NOT NULL, "order" integer,
		subtitle TEXT, valid_from DATE, ver INTEGER, content TEXT)`,
	`CREATE VIRTUAL TABLE law_fts USING fts5(name, subtitle, content, tokenize='trigram')`,
	`CREATE TABLE guiding_cases (
		case_id TEXT NOT NULL PRIMARY KEY, guiding_case_number TEXT NOT NULL, case_name TEXT NOT NULL,
		court TEXT, decision_date TEXT, case_type TEXT, key_points TEXT, related_laws TEXT, full_text TEXT)`,
}

var fixtureRows = []string{
	`INSERT INTO category (id, name, folder) VALUES (1, '法律', 'law'), (2, '司法解释', 'ji'), (3, '案例', 'case')`,
	`INSERT INTO law (id, level, name, publish, expired, category_id, "order", valid_from, content) VALUES
		('law-patent', '法律', '专利法', '2020-10-17', 0, 1, 1, '2021-06-01', '第一条 为了保护专利权人的合法权益，鼓励发明创造。'),
		('law-contract', '法律', '合同法', '1999-03-15', 1, 1, 2, '1999-10-01', '合同的订立与履行。'),
		('law-civil', '法律', '民法典', '2020-05-28', 0, 1, 3, '2021-01-01', '民事主体的民事权利能力。'),
		('law-unknown', '法律', '未核验效力法', NULL, 0, 1, 4, NULL, '施行日期尚未回填。'),
		('reg-patent', '行政法规', '专利法实施细则', '2023-12-11', 0, 1, 5, '2024-01-20', '专利申请文件的撰写要求。'),
		('ji-patent', '司法解释', '最高人民法院关于审理专利纠纷案件适用法律问题的若干规定', '2020-12-29', 0, 2, 1, '2021-01-01', '专利侵权赔偿数额的确定。'),
		('ji-evidence', '司法解释', '最高人民法院关于民事诉讼证据的若干规定', '2019-12-25', 1, 2, 2, '2020-05-01', '电子数据的审查判断。'),
		('case-1', '案例', '某公司诉某公司侵害专利权纠纷案', NULL, 0, 3, 1, NULL, '依据专利法第五十九条进行等同侵权判定。'),
		('case-guiding24', '案例', '指导案例24号：荣某某诉王某机动车交通事故责任纠纷案', NULL, 0, 3, 2, NULL, '机动车交通事故责任认定。')`,
	`INSERT INTO law_fts (rowid, name, subtitle, content)
		SELECT rowid, name, COALESCE(subtitle, ''), COALESCE(content, '') FROM law`,
	`INSERT INTO guiding_cases VALUES
		('gc-12', '指导案例12号', '某某公司诉某某公司专利权纠纷案', '最高人民法院', '2013-11-08', '民事',
		 '等同侵权的认定应当以基本相同的手段实现基本相同的功能。', '["专利法第五十九条"]', '全文：等同侵权判定规则。'),
		('gc-24', '指导案例第24号', '荣某某诉王某机动车交通事故责任纠纷案', '江苏省南京市中级人民法院', '2014-01-26', '民事',
		 '机动车交通事故责任划分。', '["侵权责任法第四十九条"]', '全文：交通事故责任划分。')`,
}

// newFixtureDB 构造一个与 laws-full.db 结构一致的小型法律知识库。
func newFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legal.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开 fixture: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range append(append([]string{}, fixtureSchema...), fixtureRows...) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture 初始化失败: %v\n%s", err, stmt)
		}
	}
	return path
}

func openFixture(t *testing.T) *Searcher {
	t.Helper()
	s, err := Open(Config{DBPath: newFixtureDB(t)})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func hitNames(hits []Hit) []string {
	names := make([]string, len(hits))
	for i, h := range hits {
		names[i] = h.Name
	}
	return names
}

func containsName(hits []Hit, name string) bool {
	for _, h := range hits {
		if h.Name == name {
			return true
		}
	}
	return false
}

// 法规检索 5 例：名称命中、正文命中、层级过滤、已废止过滤、未核验可见性。
func TestSearchRegulations(t *testing.T) {
	s := openFixture(t)

	t.Run("名称命中", func(t *testing.T) {
		hits, err := s.SearchRegulations("专利法", SearchOptions{Limit: 5})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 2 {
			t.Fatalf("expected 专利法与实施细则, got %v", hitNames(hits))
		}
		if hits[0].Scope != ScopeRegulation || hits[0].Validity != ValidityInForce {
			t.Errorf("首条应为在效法规，got %+v", hits[0])
		}
	})

	t.Run("正文命中", func(t *testing.T) {
		hits, err := s.SearchRegulations("鼓励发明创造", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !containsName(hits, "专利法") {
			t.Fatalf("expected 专利法 by content, got %v", hitNames(hits))
		}
		if hits[0].Via != viaFTS {
			t.Errorf("3 字以上查询应走 fts，got %q", hits[0].Via)
		}
	})

	t.Run("层级过滤", func(t *testing.T) {
		hits, err := s.SearchRegulations("专利", SearchOptions{Level: LevelAdministrativeRegulation})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].Name != "专利法实施细则" {
			t.Fatalf("expected 仅实施细则, got %v", hitNames(hits))
		}
	})

	t.Run("已废止不返回", func(t *testing.T) {
		hits, err := s.SearchRegulations("合同法", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("已废止的合同法不应返回, got %v", hitNames(hits))
		}
	})

	t.Run("未核验可见性", func(t *testing.T) {
		hits, err := s.SearchRegulations("未核验效力", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].Validity != ValidityUnverified {
			t.Fatalf("默认应返回未核验记录并标注, got %+v", hits)
		}
		include := false
		hits, err = s.SearchRegulations("未核验效力", SearchOptions{IncludeUnverified: &include})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("收紧后不应返回未核验记录, got %v", hitNames(hits))
		}
	})
}

// 司法解释检索 5 例。
func TestSearchJudicialInterpretations(t *testing.T) {
	s := openFixture(t)

	t.Run("名称命中", func(t *testing.T) {
		hits, err := s.SearchJudicialInterpretations("专利纠纷", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || !strings.Contains(hits[0].Name, "专利纠纷") {
			t.Fatalf("unexpected hits %v", hitNames(hits))
		}
	})

	t.Run("正文命中", func(t *testing.T) {
		hits, err := s.SearchJudicialInterpretations("赔偿数额", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || !strings.Contains(hits[0].Name, "专利纠纷") {
			t.Fatalf("unexpected hits %v", hitNames(hits))
		}
	})

	t.Run("不混入法规", func(t *testing.T) {
		hits, err := s.SearchJudicialInterpretations("专利法", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			if h.Level != LevelJudicialInterpretation {
				t.Errorf("司法解释检索混入 %s: %s", h.Level, h.Name)
			}
		}
	})

	t.Run("已废止不返回", func(t *testing.T) {
		hits, err := s.SearchJudicialInterpretations("民事诉讼证据", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("已废止的司法解释不应返回, got %v", hitNames(hits))
		}
	})

	t.Run("limit生效", func(t *testing.T) {
		hits, err := s.SearchJudicialInterpretations("专利", SearchOptions{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) > 1 {
			t.Fatalf("limit=1 生效失败, got %d", len(hits))
		}
	})
}

// 裁判案例检索 5 例。
func TestSearchCases(t *testing.T) {
	s := openFixture(t)

	t.Run("名称命中", func(t *testing.T) {
		hits, err := s.SearchCases("侵害专利权", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !containsName(hits, "某公司诉某公司侵害专利权纠纷案") {
			t.Fatalf("unexpected hits %v", hitNames(hits))
		}
	})

	t.Run("正文命中", func(t *testing.T) {
		hits, err := s.SearchCases("等同侵权", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !containsName(hits, "某公司诉某公司侵害专利权纠纷案") {
			t.Fatalf("unexpected hits %v", hitNames(hits))
		}
	})

	t.Run("只返回案例层级", func(t *testing.T) {
		hits, err := s.SearchCases("专利法", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected at least one case hit")
		}
		for _, h := range hits {
			if h.Level != LevelCase {
				t.Errorf("案例检索混入 %s: %s", h.Level, h.Name)
			}
		}
	})

	t.Run("短查询走like", func(t *testing.T) {
		hits, err := s.SearchCases("侵权", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits for 2-char query")
		}
		if hits[0].Via != viaLike {
			t.Errorf("2 字查询应走 like，got %q", hits[0].Via)
		}
	})

	t.Run("空查询返回空", func(t *testing.T) {
		hits, err := s.SearchCases("   ", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("空查询应返回空，got %v", hitNames(hits))
		}
	})
}

// 指导性案例检索 5 例：关键词、编号直达（阿拉伯/中文）、未命中、无数据标记。
func TestSearchGuidingCases(t *testing.T) {
	s := openFixture(t)
	if !s.GuidingCaseDataAvailable() {
		t.Fatal("fixture 应含指导性案例数据")
	}

	t.Run("关键词命中", func(t *testing.T) {
		hits, err := s.SearchGuidingCases("等同侵权", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].CaseNumber != "指导案例12号" {
			t.Fatalf("unexpected hits %+v", hits)
		}
		if len(hits[0].RelatedLaws) != 1 || hits[0].RelatedLaws[0] != "专利法第五十九条" {
			t.Errorf("related_laws 解析失败: %v", hits[0].RelatedLaws)
		}
	})

	t.Run("阿拉伯编号直达", func(t *testing.T) {
		for _, query := range []string{"指导案例第12号", "指导案例12号"} {
			hits, err := s.SearchGuidingCases(query, SearchOptions{})
			if err != nil {
				t.Fatalf("%s: %v", query, err)
			}
			if len(hits) != 1 || hits[0].CaseNumber != "指导案例12号" {
				t.Fatalf("%s: unexpected hits %+v", query, hits)
			}
		}
	})

	t.Run("中文编号直达", func(t *testing.T) {
		hits, err := s.SearchGuidingCases("第十二号", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].CaseNumber != "指导案例12号" {
			t.Fatalf("unexpected hits %+v", hits)
		}
	})

	t.Run("不存在编号回退关键词", func(t *testing.T) {
		hits, err := s.SearchGuidingCases("指导案例第99号", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("不存在的编号应无结果, got %+v", hits)
		}
	})

	t.Run("无指导案例数据", func(t *testing.T) {
		path := newFixtureDB(t)
		mustExec(t, path, `DROP TABLE guiding_cases`,
			`DELETE FROM law WHERE id = 'case-guiding24'`)
		noData, err := Open(Config{DBPath: path})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = noData.Close() }()
		if noData.GuidingCaseDataAvailable() {
			t.Fatal("删除数据后不应报告有指导案例")
		}
		hits, err := noData.SearchGuidingCases("指导案例第12号", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 0 {
			t.Fatalf("无数据时应返回空, got %+v", hits)
		}
	})
}

// 无 guiding_cases 表时回退到案例表中带「指导案例」标记的记录。
func TestSearchGuidingCasesFallbackToLaw(t *testing.T) {
	path := newFixtureDB(t)
	mustExec(t, path, `DROP TABLE guiding_cases`)
	s, err := Open(Config{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if !s.GuidingCaseDataAvailable() {
		t.Fatal("案例表中的指导案例记录应被识别")
	}
	hits, err := s.SearchGuidingCases("指导案例24号", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || !strings.Contains(hits[0].Name, "指导案例24号") {
		t.Fatalf("回退检索失败: %v", hitNames(hits))
	}
}

// 无库时在内存记录上降级检索四类。
func TestSearchMemoryFallback(t *testing.T) {
	records := []Record{
		{ID: "m-law", Level: LevelLaw, Name: "专利法", Content: "专利权保护", EffectiveDate: "2021-06-01"},
		{ID: "m-ji", Level: LevelJudicialInterpretation, Name: "专利纠纷若干规定", Content: "赔偿数额"},
		{ID: "m-case", Level: LevelCase, Name: "侵害专利权纠纷案", Content: "等同侵权"},
		{ID: "m-guide", Name: "某某诉某某专利权案", GuidingCaseNumber: "指导案例12号",
			KeyPoints: "等同侵权"},
	}
	s, err := Open(Config{Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if s.DataSource() != "memory" {
		t.Fatalf("expected memory data source, got %s", s.DataSource())
	}

	cases := []struct {
		scope Scope
		query string
		want  string
	}{
		{ScopeRegulation, "专利权", "专利法"},
		{ScopeJudicialInterpretation, "赔偿", "专利纠纷若干规定"},
		{ScopeCaseLaw, "侵害专利权", "侵害专利权纠纷案"},
		{ScopeGuidingCase, "等同侵权", "某某诉某某专利权案"},
	}
	for _, tc := range cases {
		hits, err := s.Search(tc.scope, tc.query, SearchOptions{})
		if err != nil {
			t.Fatalf("%s: %v", tc.scope, err)
		}
		if len(hits) != 1 || hits[0].Name != tc.want || hits[0].Via != viaMemory {
			t.Errorf("%s: got %+v, want %s via memory", tc.scope, hits, tc.want)
		}
	}
}

// Reload 与并发查询不能出现「使用已关闭连接」：读锁须贯穿查询全程。
func TestConcurrentSearchAndReload(t *testing.T) {
	path := newFixtureDB(t)
	s, err := Open(Config{DBPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := s.SearchRegulations("专利权", SearchOptions{}); err != nil {
					t.Errorf("concurrent search: %v", err)
					return
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		if err := s.Reload(path); err != nil {
			t.Fatalf("reload: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}

// 内存降级同样按效力过滤：已废止记录不返回，未核验记录带标注返回。
func TestMemoryFallbackFiltersByValidity(t *testing.T) {
	records := []Record{
		{ID: "old", Level: LevelLaw, Name: "旧法", Content: "专利权", Expired: true, EffectiveDate: "1999-10-01"},
		{ID: "new", Level: LevelLaw, Name: "新法", Content: "专利权", EffectiveDate: "2021-06-01"},
		{ID: "unknown", Level: LevelLaw, Name: "未核验法", Content: "专利权"},
	}
	s := newMemorySearcher(records)
	hits, err := s.SearchRegulations("专利权", SearchOptions{AsOf: "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if containsName(hits, "旧法") {
		t.Error("已废止记录不应出现在内存降级结果中")
	}
	if !containsName(hits, "新法") || !containsName(hits, "未核验法") {
		t.Fatalf("unexpected hits %v", hitNames(hits))
	}
	for _, h := range hits {
		want := ValidityUnverified
		if h.Name == "新法" {
			want = ValidityInForce
		}
		if h.Validity != want {
			t.Errorf("%s validity = %s, want %s", h.Name, h.Validity, want)
		}
	}
}

// 打开非法律知识库必须报错，不能当成空库。
func TestOpenRejectsNonLegalDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	if _, err := Open(Config{DBPath: path}); err == nil || !strings.Contains(err.Error(), "缺少 law 表") {
		t.Fatalf("expected non-legal db error, got %v", err)
	}
}

func TestParseGuidingCaseNumber(t *testing.T) {
	cases := []struct {
		query string
		want  int
		ok    bool
	}{
		{"指导案例第12号", 12, true},
		{"指导案例12号", 12, true},
		{"指导性案例 第 3 号", 3, true},
		{"第十二号", 12, true},
		{"指导案例第一百零五号", 105, true},
		{"等同侵权", 0, false},
		{"指导案例第0号", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseGuidingCaseNumber(tc.query)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%q = (%d,%v), want (%d,%v)", tc.query, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSearchRejectsUnknownLevel(t *testing.T) {
	s := openFixture(t)
	if _, err := s.SearchRegulations("专利", SearchOptions{Level: Level("国际条约")}); err == nil {
		t.Fatal("expected unknown level error")
	}
	if _, err := s.Search(Scope("unknown"), "x", SearchOptions{}); err == nil {
		t.Fatal("expected unknown scope error")
	}
}

func TestSearchRejectsInvalidAsOf(t *testing.T) {
	s := openFixture(t)
	if _, err := s.SearchRegulations("专利", SearchOptions{AsOf: "2026/01/01"}); err == nil {
		t.Fatal("expected invalid as_of error")
	}
}

func TestResolveValidity(t *testing.T) {
	cases := []struct {
		name      string
		expired   bool
		effective string
		asOf      string
		want      Validity
	}{
		{"已废止", true, "2021-01-01", "2026-01-01", ValidityExpired},
		{"尚未生效", false, "2027-01-01", "2026-01-01", ValidityNotYetEffective},
		{"在效", false, "2021-01-01", "2026-01-01", ValidityInForce},
		{"无施行日期", false, "", "2026-01-01", ValidityUnverified},
		{"施行日期非法", false, "2021/01/01", "2026-01-01", ValidityUnverified},
	}
	for _, tc := range cases {
		if got := resolveValidity(tc.expired, tc.effective, tc.asOf); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestResolvePaths(t *testing.T) {
	home := t.TempDir()
	queryPath, sourcePath := ResolvePaths(home)
	if queryPath != DefaultInstallPath(home) || sourcePath != "" {
		t.Fatalf("空目录: query=%q source=%q", queryPath, sourcePath)
	}

	source := filepath.Join(home, "knowledge", "laws-full.db")
	mustMkdirAndWrite(t, source)
	queryPath, sourcePath = ResolvePaths(home)
	if queryPath != source || sourcePath != source {
		t.Fatalf("直读源库: query=%q source=%q", queryPath, sourcePath)
	}

	installed := DefaultInstallPath(home)
	mustMkdirAndWrite(t, installed)
	queryPath, sourcePath = ResolvePaths(home)
	if queryPath != installed || sourcePath != source {
		t.Fatalf("安装库优先: query=%q source=%q", queryPath, sourcePath)
	}
}

func TestTruncateContent(t *testing.T) {
	if got := truncateContent("短"); got != "短" {
		t.Errorf("short text changed: %q", got)
	}
	long := strings.Repeat("专", maxContentRunes+10)
	got := truncateContent(long)
	if len([]rune(got)) >= len([]rune(long)) {
		t.Errorf("long text not truncated: %d runes", len([]rune(got)))
	}
	if !strings.Contains(got, "已截断") {
		t.Error("missing truncation marker")
	}
}

// TestSearchRealDatabase 在真实法律知识库上做冒烟检索（默认跳过）。
//
// 用法：
//
//	MADY_LEGAL_SMOKE_DB=~/.mady/knowledge/laws-full-local.db go test ./knowledge/legal -run TestSearchRealDatabase -v
func TestSearchRealDatabase(t *testing.T) {
	path := os.Getenv("MADY_LEGAL_SMOKE_DB")
	if path == "" {
		t.Skip("设置 MADY_LEGAL_SMOKE_DB 后运行真实库冒烟检索")
	}
	s, err := Open(Config{DBPath: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	cases := []struct {
		scope Scope
		query string
	}{
		{ScopeRegulation, "专利权"},
		{ScopeJudicialInterpretation, "专利侵权"},
		{ScopeCaseLaw, "劳动"},
	}
	for _, tc := range cases {
		hits, err := s.Search(tc.scope, tc.query, SearchOptions{Limit: 5})
		if err != nil {
			t.Fatalf("%s: %v", tc.scope, err)
		}
		if len(hits) == 0 {
			t.Errorf("%s %q: no hits in real database", tc.scope, tc.query)
			continue
		}
		t.Logf("%s %q: %d hits, first=%s via=%s validity=%s",
			tc.scope, tc.query, len(hits), hits[0].Name, hits[0].Via, hits[0].Validity)
	}
	t.Logf("guiding case data available: %v", s.GuidingCaseDataAvailable())
}
