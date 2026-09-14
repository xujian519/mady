package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleSource() *Source {
	return &Source{
		Version: 1,
		Repo:    "xujian519/mady",
		Entries: []Entry{
			{ID: "DEBT-02", Priority: "P0", Title: "架构根因", Issue: new(59), Status: "open"},
			{ID: "DEBT-01", Priority: "P0", Title: "隐私 | 脱敏", Issue: nil, Status: "pending"},
			{ID: "DEBT-56a", Priority: "P3", Title: "日卡", Issue: nil, Status: "pending"},
			{ID: "DEBT-56", Priority: "P3", Title: "日卡汇总", Issue: new(90), Status: "open"},
		},
	}
}

func TestRenderIndexSortsByNumberThenSuffix(t *testing.T) {
	got := renderIndex(sampleSource())

	// DEBT-01 必须在 DEBT-02 之前；DEBT-56 必须在 DEBT-56a 之前（字母后缀不倒挂）。
	order := []string{"`DEBT-01`", "`DEBT-02`", "`DEBT-56`", "`DEBT-56a`"}
	last := -1
	for _, id := range order {
		i := strings.Index(got, id)
		if i < 0 {
			t.Fatalf("索引中缺少 %s\n%s", id, got)
		}
		if i < last {
			t.Fatalf("%s 排序位置错误\n%s", id, got)
		}
		last = i
	}
}

func TestRenderIndexEscapesPipeAndDropsBackticks(t *testing.T) {
	got := renderIndex(sampleSource())

	if !strings.Contains(got, `隐私 \| 脱敏`) {
		t.Errorf("标题中的 | 未转义，会破坏表格结构：\n%s", got)
	}
	// 反引号必须被剥掉，否则表格单元格里会出现嵌套代码标记。
	for line := range strings.SplitSeq(got, "\n") {
		if strings.HasPrefix(line, "| `DEBT-") && strings.Count(line, "`") != 2 {
			t.Errorf("标题保留了反引号，单元格标记数量异常：%s", line)
		}
	}
}

// TestEscapeCell 覆盖表格单元格转义，重点是转义顺序。
func TestEscapeCell(t *testing.T) {
	cases := map[string]string{
		"隐私 | 脱敏":  `隐私 \| 脱敏`,
		`路径 \| 管道`: `路径 \\\| 管道`, // 反斜杠须先转义，否则 \| 会被二次转义成裸管道
		"含`反引号`的":  "含反引号的",
		"跨\n行":     "跨 行",
	}
	for in, want := range cases {
		if got := escapeCell(in); got != want {
			t.Errorf("escapeCell(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestRenderIndexLinksIssuesAndMarksPending(t *testing.T) {
	got := renderIndex(sampleSource())

	if !strings.Contains(got, "https://github.com/xujian519/mady/issues/59") {
		t.Errorf("已建 issue 未生成链接：\n%s", got)
	}
	if !strings.Contains(got, "| 待建 |") || !strings.Contains(got, "| 已建 |") {
		t.Errorf("状态列未区分已建/待建：\n%s", got)
	}
}

func TestReplaceBlockRoundTrip(t *testing.T) {
	doc := "前言\n" + markerStart + "\n旧内容\n" + markerEnd + "\n后记\n"

	out, err := replaceBlock(doc, "\n新内容\n")
	if err != nil {
		t.Fatalf("replaceBlock 报错: %v", err)
	}
	if strings.Contains(out, "旧内容") {
		t.Errorf("旧内容未被替换：\n%s", out)
	}
	if !strings.Contains(out, markerStart) || !strings.Contains(out, markerEnd) {
		t.Errorf("标记被破坏：\n%s", out)
	}
	if !strings.HasPrefix(out, "前言\n") || !strings.HasSuffix(out, "后记\n") {
		t.Errorf("标记之外的内容被改动：\n%s", out)
	}

	// 幂等：同一份内容重复替换结果不变。
	again, err := replaceBlock(out, "\n新内容\n")
	if err != nil {
		t.Fatalf("二次 replaceBlock 报错: %v", err)
	}
	if again != out {
		t.Errorf("replaceBlock 不幂等")
	}
}

func TestReplaceBlockRejectsMissingMarkers(t *testing.T) {
	cases := map[string]string{
		"缺 start": "只有 " + markerEnd,
		"缺 end":   "只有 " + markerStart,
		"顺序颠倒":    markerEnd + " ... " + markerStart,
	}
	for name, doc := range cases {
		if _, err := replaceBlock(doc, "x"); err == nil {
			t.Errorf("%s：应当报错但通过了", name)
		}
	}
}

func TestLoadSourceRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	src.Entries = append(src.Entries, src.Entries[0]) // 人为制造重复
	writeSource(t, dir, src)

	if _, err := loadSource(dir); err == nil {
		t.Fatal("编号重复未被拒绝（编号一经分配不得复用）")
	}
}

func TestLoadSourceRejectsMissingFields(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	src.Entries[0].Title = ""
	writeSource(t, dir, src)

	if _, err := loadSource(dir); err == nil {
		t.Fatal("字段缺失的条目未被拒绝")
	}
}

// TestLoadSourceRejectsInvalidMeta 覆盖元数据与字段取值的非法情形。
//
// 这些取值不会被 check 1–4 发现：status 非闭集时 `issue: 5, status: ""` 会渲染成
// 「#5 + 待建」的自相矛盾行，repo 写错会静默产出坏链接，issue 为 0/负数会产出
// 指向不存在 issue 的链接——只有不带 -gh 的常规门禁根本看不见。
func TestLoadSourceRejectsInvalidMeta(t *testing.T) {
	cases := map[string]func(*Source){
		"version 不符": func(s *Source) { s.Version = 99 },
		"repo 缺斜杠":   func(s *Source) { s.Repo = "mady" },
		"repo 带空格":   func(s *Source) { s.Repo = "xujian519 /mady" },
		"status 为空":  func(s *Source) { s.Entries[0].Status = "" },
		"status 未知值": func(s *Source) { s.Entries[0].Status = "OPEN" },
		"issue 为零":   func(s *Source) { s.Entries[0].Issue = new(0) },
		"issue 为负":   func(s *Source) { s.Entries[0].Issue = new(-7) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := sampleSource()
			mutate(src)
			writeSource(t, dir, src)

			if _, err := loadSource(dir); err == nil {
				t.Errorf("%s：应当被拒绝但通过了", name)
			}
		})
	}
}

func TestLoadSourceRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, &Source{})

	if _, err := loadSource(dir); err == nil {
		t.Fatal("空事实源未被拒绝（空文件会让 check 变成永远通过）")
	}
}

func TestCheckAllFailsWhenReportHasUnregisteredID(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	writeSource(t, dir, src)

	// 正文登记齐全且含一个幽灵编号 DEBT-99；索引区块本身与事实源同步，
	// 以确保失败只能来自「幽灵编号」而非区块过期。
	body := "DEBT-01 DEBT-02 DEBT-56 DEBT-56a DEBT-99\n" + markerStart + renderIndex(src) + markerEnd + "\n"
	writeDoc(t, dir, body)

	err := checkAll(src, dir, false)
	if err == nil {
		t.Fatal("报告中出现未登记编号时应当失败")
	}
	if !strings.Contains(err.Error(), "DEBT-99") {
		t.Errorf("错误信息未点出违规编号: %v", err)
	}
}

// TestCheckAllRejectsReportWithoutIndexBlock 覆盖索引区块缺失时的 fail-closed 行为。
//
// 区块结构是其余检查的前提；缺失时必须直接失败，且不应把全部条目误报成
// 「正文未提及」——那会把真正的原因淹没在数十条噪音里。
func TestCheckAllRejectsReportWithoutIndexBlock(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, sampleSource())
	writeDoc(t, dir, "DEBT-01 DEBT-02 DEBT-56 DEBT-56a\n")

	err := checkAll(sampleSource(), dir, false)
	if err == nil {
		t.Fatal("报告缺少索引区块标记时应当失败")
	}
	if !strings.Contains(err.Error(), markerStart) {
		t.Errorf("错误信息未点出缺失的标记: %v", err)
	}
	if strings.Contains(err.Error(), "正文未提及") {
		t.Errorf("结构错误时不应连带误报「正文未提及」: %v", err)
	}
}

// TestCheckAllIgnoresGeneratedBlockWhenScanningProse 是一条回归测试。
//
// 索引区块由本工具生成、必然列出全部编号。若规则 1 连区块一起扫描，则只要跑过一次
// -apply，未在正文登记的债务也会被判为「已提及」——该项校验将恒不触发。
func TestCheckAllIgnoresGeneratedBlockWhenScanningProse(t *testing.T) {
	dir := t.TempDir()

	// DEBT-56a 只存在于索引区块，正文只字未提。
	src := sampleSource()
	writeSource(t, dir, src)
	body := "DEBT-01 DEBT-02 DEBT-56\n" + markerStart + renderIndex(src) + markerEnd + "\n"
	writeDoc(t, dir, body)

	err := checkAll(src, dir, false)
	if err == nil {
		t.Fatal("编号仅出现在自动生成的索引区块中时应当失败")
	}
	if !strings.Contains(err.Error(), "DEBT-56a") {
		t.Errorf("错误信息未点出仅存在于区块中的编号: %v", err)
	}
}

func TestStripBlockRemovesOnlyGeneratedRegion(t *testing.T) {
	doc := "前 DEBT-01 " + markerStart + " 区块内 DEBT-99 " + markerEnd + " 后 DEBT-02"

	got, err := stripBlock(doc)
	if err != nil {
		t.Fatalf("stripBlock 报错: %v", err)
	}
	if strings.Contains(got, "DEBT-99") {
		t.Errorf("区块内内容未被剔除: %q", got)
	}
	for _, want := range []string{"DEBT-01", "DEBT-02"} {
		if !strings.Contains(got, want) {
			t.Errorf("区块外内容 %s 被误删: %q", want, got)
		}
	}
}

func TestCheckAllPassesOnConsistentPair(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	writeSource(t, dir, src)

	// 报告正文提及全部编号 + 一份与事实源同步的索引区块。
	body := "DEBT-01 DEBT-02 DEBT-56 DEBT-56a\n" + markerStart + renderIndex(src) + markerEnd + "\n"
	writeDoc(t, dir, body)

	if err := checkAll(src, dir, false); err != nil {
		t.Fatalf("一致的事实源/报告不应报错: %v", err)
	}
}

func TestCheckAllDetectsStaleIndexBlock(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	writeSource(t, dir, src)

	// 索引区块故意留空 —— 正文提及齐全，只有区块过期。
	body := "DEBT-01 DEBT-02 DEBT-56 DEBT-56a\n" + markerStart + "\n_（尚未生成）_\n" + markerEnd + "\n"
	writeDoc(t, dir, body)

	err := checkAll(src, dir, false)
	if err == nil {
		t.Fatal("过期的索引区块应当失败")
	}
	if !strings.Contains(err.Error(), "-apply") {
		t.Errorf("错误信息未给出修复命令: %v", err)
	}
}

func TestCheckAllDetectsStatusContradiction(t *testing.T) {
	dir := t.TempDir()

	src := sampleSource()
	src.Entries[0].Status = "pending" // 有 issue 编号却标为待建
	writeSource(t, dir, src)
	body := "DEBT-01 DEBT-02 DEBT-56 DEBT-56a\n" + markerStart + renderIndex(src) + markerEnd + "\n"
	writeDoc(t, dir, body)

	err := checkAll(src, dir, false)
	if err == nil {
		t.Fatal("状态自相矛盾应当失败")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("错误信息未说明状态问题: %v", err)
	}
}

// TestCheckAllDetectsOpenWithoutIssue 覆盖 check 4 的另一个分支：
// 标为 open 却没有 issue 编号。
func TestCheckAllDetectsOpenWithoutIssue(t *testing.T) {
	dir := t.TempDir()
	src := sampleSource()
	src.Entries[1].Status = "open" // DEBT-01：Issue 仍为 nil
	writeSource(t, dir, src)
	body := "DEBT-01 DEBT-02 DEBT-56 DEBT-56a\n" + markerStart + renderIndex(src) + markerEnd + "\n"
	writeDoc(t, dir, body)

	err := checkAll(src, dir, false)
	if err == nil {
		t.Fatal("open 却无 issue 编号应当失败")
	}
	if !strings.Contains(err.Error(), "缺少 issue 编号") {
		t.Errorf("错误信息未说明问题: %v", err)
	}
}

// TestAuditIssueView 覆盖 -gh 校验的三条判定分支。
//
// 期望状态须由条目自身的 status 推导：写死 OPEN 会让「正常关闭 issue 并同步
// 事实源」变成一种失败状态，逼人把事实源改回去。
func TestAuditIssueView(t *testing.T) {
	open := Entry{ID: "DEBT-02", Status: "open", Issue: new(59)}
	closed := Entry{ID: "DEBT-02", Status: "closed", Issue: new(59)}

	cases := []struct {
		name  string
		entry Entry
		raw   string
		want  string // 期望错误信息包含的子串；空串表示应当无问题
	}{
		{"一致", open, `{"state":"OPEN","title":"DEBT-02 架构根因"}`, ""},
		{"closed 条目遇 CLOSED", closed, `{"state":"CLOSED","title":"DEBT-02 x"}`, ""},
		{"响应非法 JSON", open, `not json`, "响应解析失败"},
		{"标题不含编号", open, `{"state":"OPEN","title":"别的标题"}`, "标题未含编号"},
		{"open 条目被关闭", open, `{"state":"CLOSED","title":"DEBT-02 x"}`, "请同步更新"},
		{"closed 条目又开启", closed, `{"state":"OPEN","title":"DEBT-02 x"}`, "请同步更新"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := auditIssueView(c.entry, []byte(c.raw))
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("期望无问题，实际: %v", got)
				}
				return
			}
			if !strings.Contains(strings.Join(got, "\n"), c.want) {
				t.Errorf("错误信息未含 %q: %v", c.want, got)
			}
		})
	}
}

func TestGhState(t *testing.T) {
	if got := ghState("open"); got != "OPEN" {
		t.Errorf("ghState(open) = %q，期望 OPEN", got)
	}
	if got := ghState("closed"); got != "CLOSED" {
		t.Errorf("ghState(closed) = %q，期望 CLOSED", got)
	}
}

func TestLessIDOrdersSuffixAfterBase(t *testing.T) {
	if !lessID("DEBT-56", "DEBT-56a") {
		t.Error("DEBT-56 应排在 DEBT-56a 之前")
	}
	if !lessID("DEBT-56e", "DEBT-57") {
		t.Error("DEBT-56e 应排在 DEBT-57 之前")
	}
	if lessID("DEBT-57", "DEBT-56a") {
		t.Error("DEBT-57 不应排在 DEBT-56a 之前")
	}
}

// --- 测试辅助 ---

func writeSource(t *testing.T, dir string, s *Source) {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("序列化事实源失败: %v", err)
	}
	writeFile(t, dir, sourceJSON, raw)
}

func writeDoc(t *testing.T, dir, body string) {
	t.Helper()
	writeFile(t, dir, targetDoc, []byte(body))
}

// writeFile 按 sourceJSON/targetDoc 的相对路径建好父目录后写入。
func writeFile(t *testing.T, dir, rel string, raw []byte) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", rel, err)
	}
}
