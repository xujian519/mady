package legal

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustExec 在指定数据库上执行若干语句。
func mustExec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("打开 %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("执行失败: %v\n%s", err, stmt)
		}
	}
}

// mustMkdirAndWrite 创建文件（内容无关紧要，仅用于路径解析）。
func mustMkdirAndWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录: %v", err)
	}
	if err := os.WriteFile(path, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("写入 %s: %v", path, err)
	}
}

func TestInstall(t *testing.T) {
	source := newFixtureDB(t)
	output := filepath.Join(t.TempDir(), "nested", "legal.db")

	result, err := Install(context.Background(), InstallOptions{SourcePath: source, OutputPath: output})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if result.InputBytes <= 0 || result.OutputBytes <= 0 {
		t.Errorf("size 摘要异常: %+v", result)
	}
	if result.RowCounts["law"] != 9 || result.RowCounts["guiding_cases"] != 2 {
		t.Errorf("row counts = %v, want law=9 guiding_cases=2", result.RowCounts)
	}
	if len(result.DroppedFTS) != 0 {
		t.Errorf("默认应保留 FTS，got %v", result.DroppedFTS)
	}

	s, err := Open(Config{DBPath: output})
	if err != nil {
		t.Fatalf("打开安装产物: %v", err)
	}
	defer func() { _ = s.Close() }()
	hits, err := s.SearchRegulations("专利权", SearchOptions{})
	if err != nil || len(hits) == 0 {
		t.Fatalf("安装产物检索失败: hits=%v err=%v", hits, err)
	}
	if hits[0].Via != viaFTS {
		t.Errorf("保留 FTS 时应走 fts，got %q", hits[0].Via)
	}
}

func TestInstallNoFTS(t *testing.T) {
	source := newFixtureDB(t)
	output := filepath.Join(t.TempDir(), "legal.db")

	result, err := Install(context.Background(), InstallOptions{SourcePath: source, OutputPath: output, NoFTS: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(result.DroppedFTS) != 1 || result.DroppedFTS[0] != "law_fts" {
		t.Fatalf("dropped = %v, want [law_fts]", result.DroppedFTS)
	}

	s, err := Open(Config{DBPath: output})
	if err != nil {
		t.Fatalf("打开安装产物: %v", err)
	}
	defer func() { _ = s.Close() }()
	if s.hasLawFTS {
		t.Error("no_fts 后不应再有 law_fts")
	}
	hits, err := s.SearchRegulations("专利权", SearchOptions{})
	if err != nil || len(hits) == 0 {
		t.Fatalf("压缩后检索应降级为 like: hits=%v err=%v", hits, err)
	}
	if hits[0].Via != viaLike {
		t.Errorf("expected like fallback, got %q", hits[0].Via)
	}
}

func TestInstallRejectsSameFile(t *testing.T) {
	source := newFixtureDB(t)
	if _, err := Install(context.Background(), InstallOptions{SourcePath: source, OutputPath: source}); err == nil ||
		!strings.Contains(err.Error(), "同一文件") {
		t.Fatalf("expected same-file error, got %v", err)
	}
}

func TestInstallRejectsNonLegalSource(t *testing.T) {
	other := filepath.Join(t.TempDir(), "other.db")
	db, err := sql.Open("sqlite", other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	if _, err := Install(context.Background(), InstallOptions{SourcePath: other, OutputPath: filepath.Join(t.TempDir(), "out.db")}); err == nil ||
		!strings.Contains(err.Error(), "缺少 law 表") {
		t.Fatalf("expected non-legal source error, got %v", err)
	}
}

func TestInstallMissingSource(t *testing.T) {
	if _, err := Install(context.Background(), InstallOptions{
		SourcePath: filepath.Join(t.TempDir(), "missing.db"),
		OutputPath: filepath.Join(t.TempDir(), "out.db"),
	}); err == nil {
		t.Fatal("expected missing source error")
	}
}

func newTestExtension(t *testing.T, cfg ExtensionConfig) *Extension {
	t.Helper()
	ext := NewExtension(cfg)
	if err := ext.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return ext
}

func TestSearchToolRun(t *testing.T) {
	ext := newTestExtension(t, ExtensionConfig{DBPath: newFixtureDB(t)})
	defer func() { _ = ext.Dispose() }()
	tool := newSearchTool(ext)

	result, err := tool.Func(context.Background(), json.RawMessage(
		`{"query":"专利权","scope":"regulation","limit":3}`))
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	out, ok := result.(searchOutput)
	if !ok {
		t.Fatalf("expected searchOutput, got %T", result)
	}
	if out.Count == 0 || len(out.Hits) != out.Count {
		t.Fatalf("count/hits 不一致: %+v", out)
	}
	if out.DataSource != "db" {
		t.Errorf("data_source = %q, want db", out.DataSource)
	}
	if !strings.Contains(out.Summary, "命中") {
		t.Errorf("summary 缺少命中说明: %s", out.Summary)
	}
}

func TestSearchToolRejectsBadInput(t *testing.T) {
	ext := newTestExtension(t, ExtensionConfig{DBPath: newFixtureDB(t)})
	defer func() { _ = ext.Dispose() }()
	tool := newSearchTool(ext)

	if _, err := tool.Func(context.Background(), json.RawMessage(`{"query":"x","scope":"unknown"}`)); err == nil {
		t.Fatal("expected unknown scope error")
	}
	if _, err := tool.Func(context.Background(), json.RawMessage(`{"query":"  "}`)); err == nil {
		t.Fatal("expected missing query error")
	}
}

// 无库时检索工具应说明数据来源，指导案例另给出「本部署无数据」提示。
func TestSearchToolWithoutDatabase(t *testing.T) {
	ext := newTestExtension(t, ExtensionConfig{HomeDir: t.TempDir()})
	defer func() { _ = ext.Dispose() }()
	tool := newSearchTool(ext)

	result, err := tool.Func(context.Background(), json.RawMessage(`{"query":"专利权"}`))
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	out := result.(searchOutput)
	if out.Count != 0 || out.DataSource != "memory" {
		t.Fatalf("无库时应为空内存结果: %+v", out)
	}
	if !strings.Contains(out.Summary, "未安装数据库") {
		t.Errorf("summary 应说明数据来源: %s", out.Summary)
	}

	result, err = tool.Func(context.Background(), json.RawMessage(`{"query":"指导案例第1号","scope":"guiding_case"}`))
	if err != nil {
		t.Fatalf("guiding search: %v", err)
	}
	out = result.(searchOutput)
	if !strings.Contains(out.Summary, "未安装指导性案例数据") {
		t.Errorf("无指导案例数据时应明确提示: %s", out.Summary)
	}
}

// 打开损坏的知识库时检索应报错，而不是返回空结果。
func TestSearchToolSurfacesOpenError(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken.db")
	db, err := sql.Open("sqlite", broken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	ext := newTestExtension(t, ExtensionConfig{DBPath: broken})
	defer func() { _ = ext.Dispose() }()
	tool := newSearchTool(ext)
	if _, err := tool.Func(context.Background(), json.RawMessage(`{"query":"专利"}`)); err == nil ||
		!strings.Contains(err.Error(), "法律知识库不可用") {
		t.Fatalf("expected open error surfaced, got %v", err)
	}
}

func TestInstallToolRun(t *testing.T) {
	home := t.TempDir()
	ext := newTestExtension(t, ExtensionConfig{HomeDir: home})
	defer func() { _ = ext.Dispose() }()
	tool := newInstallTool(ext)

	args, _ := json.Marshal(map[string]any{"source": newFixtureDB(t), "no_fts": true})
	result, err := tool.Func(context.Background(), args)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	out, ok := result.(installOutput)
	if !ok {
		t.Fatalf("expected installOutput, got %T", result)
	}
	want := DefaultInstallPath(home)
	if out.OutputPath != want {
		t.Errorf("output = %q, want %q", out.OutputPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("安装产物不存在: %v", err)
	}
	if ext.searcher.DataSource() != "db" {
		t.Errorf("安装后应切换到新库，got %s", ext.searcher.DataSource())
	}
	if !strings.Contains(out.Note, "已切换") {
		t.Errorf("note 应说明已生效: %s", out.Note)
	}
}

func TestInstallToolRejectsOutputOutsideHome(t *testing.T) {
	home := t.TempDir()
	ext := newTestExtension(t, ExtensionConfig{HomeDir: home})
	defer func() { _ = ext.Dispose() }()
	tool := newInstallTool(ext)

	args, _ := json.Marshal(map[string]any{
		"source": newFixtureDB(t),
		"output": filepath.Join(t.TempDir(), "outside.db"),
	})
	if _, err := tool.Func(context.Background(), args); err == nil ||
		!strings.Contains(err.Error(), "必须位于数据目录") {
		t.Fatalf("expected path guard error, got %v", err)
	}
}

func TestInstallToolRequiresSource(t *testing.T) {
	ext := newTestExtension(t, ExtensionConfig{HomeDir: t.TempDir()})
	defer func() { _ = ext.Dispose() }()
	tool := newInstallTool(ext)
	if _, err := tool.Func(context.Background(), json.RawMessage(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "未找到源库") {
		t.Fatalf("expected missing source error, got %v", err)
	}
}

func TestExtensionTools(t *testing.T) {
	ext := NewExtension(ExtensionConfig{})
	if ext.Name() != ExtensionName {
		t.Errorf("name = %q", ext.Name())
	}
	tools := ext.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names[SearchToolName] || !names[InstallToolName] {
		t.Errorf("missing tools: %v", names)
	}
	if !tools[0].ReadOnly {
		t.Error("search tool 应为只读")
	}
}
