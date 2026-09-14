// debtlink 维护 DEBT 编号 ↔ GitHub issue 的双向索引，防止文档二次漂移。
//
// 背景：本项目的技术债务长期只存在于 docs/ 中，文档自身会过期（`docs/` 里已有
// 多份失效报告）。故把映射关系抽成机器可读的事实源 `docs/plans/debt-issues.json`，
// 由本工具生成索引区块，并由 `-check` 在 CI 中守卫时效性。
//
// Usage:
//
//	go run scripts/debtlink/main.go -apply          # 依据事实源重新生成报告中的索引区块
//	go run scripts/debtlink/main.go -check          # 校验事实源 ↔ 报告双向一致（fail-closed）
//	go run scripts/debtlink/main.go -check -gh      # 额外校验 issue 在 GitHub 上仍存活
//
// 退出码：0 通过；1 校验失败或执行错误。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sourceJSON = "docs/plans/debt-issues.json"
	targetDoc  = "docs/plans/debt-issue-map.md"

	markerStart = "<!-- debt-index:start -->"
	markerEnd   = "<!-- debt-index:end -->"

	// ghTimeout 限制单次 gh 调用耗时，避免 CI 上网络挂起无限等待。
	ghTimeout = 30 * time.Second

	// sourceVersion 是事实源格式版本。文件声明的版本与之不符即拒绝，
	// 避免旧工具误读新格式后静默产出错误索引。
	sourceVersion = 1

	// unparsableIDNum 是编号无法解析时的排序权重，保证其排在所有合法编号之后。
	unparsableIDNum = 1 << 30
)

// Entry 是事实源中的一个 DEBT 条目。
type Entry struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
	Title    string `json:"title"`
	Issue    *int   `json:"issue"`
	Status   string `json:"status"`
}

// Source 是事实源文件的结构。
type Source struct {
	Version   int     `json:"version"`
	Updated   string  `json:"updated"`
	Repo      string  `json:"repo"`
	SourceDoc string  `json:"sourceDoc"`
	Note      string  `json:"note"`
	Entries   []Entry `json:"entries"`
}

// debtIDRe 匹配报告正文中出现的 DEBT 编号（含 56a–56e 这类带字母后缀的）。
var debtIDRe = regexp.MustCompile(`DEBT-\d+[a-z]?`)

// debtIDPartsRe 拆分编号为数字部分与字母后缀，供排序使用。
var debtIDPartsRe = regexp.MustCompile(`^DEBT-(\d+)([a-z]?)$`)

func main() {
	var (
		apply = flag.Bool("apply", false, "依据事实源重新生成报告中的索引区块")
		check = flag.Bool("check", false, "校验事实源与报告双向一致（fail-closed）")
		ghChk = flag.Bool("gh", false, "配合 -check：额外向 GitHub 校验 issue 存活与状态")
		root  = flag.String("root", ".", "仓库根目录")
	)
	flag.Parse()

	if !*apply && !*check {
		fmt.Fprintln(os.Stderr, "用法：debtlink -apply | -check [-gh]")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *apply && *check {
		fail("-apply 与 -check 互斥：apply 会先改写文件，check 随即对着自己的产物判定，必然通过")
	}

	src, err := loadSource(*root)
	if err != nil {
		fail("加载事实源失败: %v", err)
	}

	if *apply {
		if err := applyIndex(src, *root); err != nil {
			fail("生成索引失败: %v", err)
		}
		fmt.Printf("✓ 已生成索引区块：%d 条（%s）\n", len(src.Entries), targetDoc)
	}

	if *check {
		if err := checkAll(src, *root, *ghChk); err != nil {
			fail("%v", err)
		}
		// 文案须与实际覆盖面一致：未加 -gh 时并没有发起任何 GitHub 请求，
		// 若笼统宣称「与 GitHub 一致」，会把读者引向过高的信任。
		if *ghChk {
			fmt.Printf("✓ 校验通过：事实源 %d 条 ↔ 报告 ↔ GitHub 一致（含 issue 存活核验）\n", len(src.Entries))
		} else {
			fmt.Printf("✓ 校验通过：事实源 %d 条 ↔ 报告一致（未核验 GitHub，需要时加 -gh）\n", len(src.Entries))
		}
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "❌ "+format+"\n", args...)
	os.Exit(1)
}

func loadSource(root string) (*Source, error) {
	//nolint:gosec // 路径由 -root 参数 + 固定常量拼接，无外部输入
	raw, err := os.ReadFile(filepath.Join(root, sourceJSON))
	if err != nil {
		return nil, err
	}
	var s Source
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s 解析失败: %w", sourceJSON, err)
	}
	if len(s.Entries) == 0 {
		return nil, fmt.Errorf("%s 中没有任何条目", sourceJSON)
	}

	if s.Version != sourceVersion {
		return nil, fmt.Errorf("%s 声明 version=%d，本工具支持 %d（格式变更须同步升级工具）",
			sourceJSON, s.Version, sourceVersion)
	}
	// repo 用于拼接 issue 链接，缺失或格式不对会静默产出坏链接——不带 -gh 时无从发现。
	if strings.Count(s.Repo, "/") != 1 || strings.ContainsAny(s.Repo, " \t") {
		return nil, fmt.Errorf("%s 的 repo 非法: %q（应形如 owner/name）", sourceJSON, s.Repo)
	}

	// 自检：编号唯一、字段非空、status 属闭集、issue 编号为正。
	// status 与 issue 不校验时，`issue: 5, status: ""` 会渲染成「#5 + 待建」这种
	// 自相矛盾的行，而 check 4 只看 open/pending 两个分支，判断不出来。
	seen := make(map[string]bool, len(s.Entries))
	for _, e := range s.Entries {
		if e.ID == "" || e.Priority == "" || e.Title == "" {
			return nil, fmt.Errorf("%s 存在字段缺失的条目: %+v", sourceJSON, e)
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("%s 中编号重复: %s（编号一经分配不得复用）", sourceJSON, e.ID)
		}
		seen[e.ID] = true
		switch e.Status {
		case "open", "pending", "closed":
		default:
			return nil, fmt.Errorf("%s 的条目 %s status 非法: %q（仅允许 open/pending/closed）",
				sourceJSON, e.ID, e.Status)
		}
		if e.Issue != nil && *e.Issue <= 0 {
			return nil, fmt.Errorf("%s 的条目 %s issue 编号非法: %d", sourceJSON, e.ID, *e.Issue)
		}
	}
	return &s, nil
}

// renderIndex 生成索引区块正文（不含 marker 行）。
func renderIndex(s *Source) string {
	entries := make([]Entry, len(s.Entries))
	copy(entries, s.Entries)
	sort.Slice(entries, func(i, j int) bool { return lessID(entries[i].ID, entries[j].ID) })

	var b strings.Builder
	b.WriteString("\n| 编号 | 优先级 | 标题 | Issue | 状态 |\n")
	b.WriteString("| --- | :---: | --- | :---: | :---: |\n")
	for _, e := range entries {
		issue := "—"
		if e.Issue != nil {
			issue = fmt.Sprintf("[#%d](https://github.com/%s/issues/%d)", *e.Issue, s.Repo, *e.Issue)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n",
			e.ID, e.Priority, escapeCell(e.Title), issue, statusText(e.Status))
	}
	return b.String()
}

// escapeCell 把标题转义成可安全放入表格单元格的形式。
//
// 顺序不能颠倒：反斜杠必须先于竖线转义，否则 `|` 先变成 `\|`、再被二次转义成
// `\\|`——其中 `\\` 是转义后的反斜杠，竖线仍是裸管道，表格列会错位。
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	// 反引号在单元格里会造成嵌套代码标记；换行会把表格断成两行。
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// statusText 把事实源状态译成表格文案。取值由 loadSource 保证属闭集，
// default 分支仅作兜底。
func statusText(s string) string {
	switch s {
	case "open":
		return "已建"
	case "closed":
		return "已关闭"
	default:
		return "待建"
	}
}

// lessID 按数字部分 + 字母后缀排序，使 DEBT-56a 排在 DEBT-56 之后、DEBT-57 之前。
func lessID(a, b string) bool {
	na, sa := splitID(a)
	nb, sb := splitID(b)
	if na != nb {
		return na < nb
	}
	return sa < sb
}

func splitID(id string) (int, string) {
	m := debtIDPartsRe.FindStringSubmatch(id)
	if m == nil {
		return unparsableIDNum, id
	}
	n, _ := strconv.Atoi(m[1])
	return n, m[2]
}

// blockBounds 返回索引区块内容的 [bodyStart, bodyEnd) 边界（不含 marker 行本身）。
func blockBounds(text string) (bodyStart, bodyEnd int, err error) {
	i := strings.Index(text, markerStart)
	if i < 0 {
		return 0, 0, fmt.Errorf("报告中缺少标记 %s", markerStart)
	}
	j := strings.Index(text, markerEnd)
	if j < 0 {
		return 0, 0, fmt.Errorf("报告中缺少标记 %s", markerEnd)
	}
	if j < i {
		return 0, 0, fmt.Errorf("报告中标记顺序颠倒：%s 出现在 %s 之前", markerEnd, markerStart)
	}
	return i + len(markerStart), j, nil
}

// replaceBlock 用 body 替换 text 中 markerStart..markerEnd 之间的内容。
func replaceBlock(text, body string) (string, error) {
	start, end, err := blockBounds(text)
	if err != nil {
		return "", err
	}
	return text[:start] + body + text[end:], nil
}

// stripBlock 返回剔除索引区块后的正文。
//
// 校验「正文是否真的提及了某个编号」必须先剔除该区块：区块由本工具生成、必然列出
// 全部编号，若一并扫描，则只要跑过一次 -apply，未在正文登记的债务也会被判为已提及，
// 该项校验将恒不触发。
func stripBlock(text string) (string, error) {
	return replaceBlock(text, "")
}

func applyIndex(s *Source, root string) error {
	path := filepath.Join(root, targetDoc)
	//nolint:gosec // path 由 -root 参数 + 固定常量拼接，无外部输入
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out, err := replaceBlock(string(raw), renderIndex(s))
	if err != nil {
		return err
	}
	// 0644 与仓库内其他文档一致；该文件是受版本控制的文档，非敏感数据。
	//nolint:gosec // G306: 文档文件按仓库惯例使用 0644
	return os.WriteFile(path, []byte(out), 0o644)
}

func checkAll(s *Source, root string, useGh bool) error {
	path := filepath.Join(root, targetDoc)
	//nolint:gosec // path 由 -root 参数 + 固定常量拼接，无外部输入
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc := string(raw)

	var problems []string

	// 索引区块结构异常时无法可靠扫描正文，故只报这一条并跳过 1)–3)。
	// 若继续执行，inDoc 会因扫描失败而为空，把全部条目误报成「正文未提及」，
	// 真正的失败原因反而被噪音淹没。
	prose, stripErr := stripBlock(doc)
	if stripErr != nil {
		problems = append(problems, fmt.Sprintf("索引区块结构错误: %v", stripErr))
	} else {
		// 1) 事实源 → 报告：每个编号都须能在报告正文中找到（区块不算数，见 stripBlock）。
		inDoc := make(map[string]bool)
		for _, id := range debtIDRe.FindAllString(prose, -1) {
			inDoc[id] = true
		}
		for _, e := range s.Entries {
			if !inDoc[e.ID] {
				problems = append(problems, fmt.Sprintf(
					"事实源有 %s，但报告正文未提及（新增债务须同时登记 %s 与报告）", e.ID, sourceJSON))
			}
		}

		// 2) 报告 → 事实源：报告正文里出现的每个编号都必须已登记。
		known := make(map[string]bool, len(s.Entries))
		for _, e := range s.Entries {
			known[e.ID] = true
		}
		var orphan []string
		for id := range inDoc {
			if !known[id] {
				orphan = append(orphan, id)
			}
		}
		sort.Strings(orphan)
		if len(orphan) > 0 {
			problems = append(problems, fmt.Sprintf(
				"报告提及但未登记到 %s 的编号: %s", sourceJSON, strings.Join(orphan, ", ")))
		}

		// 3) 索引区块时效性：重新生成后应与现有内容完全一致。
		if want, err := replaceBlock(doc, renderIndex(s)); err != nil {
			problems = append(problems, fmt.Sprintf("索引区块结构错误: %v", err))
		} else if want != doc {
			problems = append(problems, "索引区块已过期，请运行 `go run scripts/debtlink/main.go -apply`")
		}
	}

	// 4) 状态自洽：已建 issue 的条目必须有编号；待建的必须没有。
	for _, e := range s.Entries {
		switch {
		case e.Status == "open" && e.Issue == nil:
			problems = append(problems, fmt.Sprintf("%s 状态为 open 但缺少 issue 编号", e.ID))
		case e.Status == "pending" && e.Issue != nil:
			problems = append(problems, fmt.Sprintf("%s 已有 issue #%d 但状态仍为 pending", e.ID, *e.Issue))
		}
	}

	// 5) 可选：向 GitHub 校验 issue 存活。
	if useGh {
		problems = append(problems, checkGitHub(s, root)...)
	}

	if len(problems) > 0 {
		// 明细随 error 回传而非直接打 stderr：调用方（含测试）才能据错误内容判定失败原因。
		return fmt.Errorf("债务映射校验失败，共 %d 项：\n  · %s",
			len(problems), strings.Join(problems, "\n  · "))
	}
	return nil
}

// checkGitHub 逐一确认 issue 存在且未关闭。任一网络/权限异常即视为失败（fail-closed），
// 避免「取不到数据」被误读成「没问题」。
func checkGitHub(s *Source, root string) []string {
	var problems []string
	for _, e := range s.Entries {
		if e.Issue == nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
		// 参数为事实源中的整数编号与固定字符串常量，无外部输入。
		//nolint:gosec // G204: 参数非外部输入
		cmd := exec.CommandContext(ctx, "gh", "issue", "view", strconv.Itoa(*e.Issue),
			"-R", s.Repo, "--json", "state,title")
		cmd.Dir = root
		out, err := cmd.Output()
		cancel()
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s（#%d）查询失败: %v", e.ID, *e.Issue, err))
			continue
		}
		problems = append(problems, auditIssueView(e, out)...)
	}
	return problems
}

// issueView 是 `gh issue view --json state,title` 的响应。
type issueView struct {
	State string `json:"state"`
	Title string `json:"title"`
}

// auditIssueView 把一次 gh 查询结果与事实源条目比对，返回不一致之处。
//
// 期望状态由条目自身的 status 推导而非写死 OPEN：工具支持 `closed`，
// 若一律要求 OPEN，则正常关闭 issue 并同步事实源后，-gh 反而会报错逼人改回去。
func auditIssueView(e Entry, raw []byte) []string {
	var v issueView
	if err := json.Unmarshal(raw, &v); err != nil {
		return []string{fmt.Sprintf("%s（#%d）响应解析失败: %v", e.ID, *e.Issue, err)}
	}
	var problems []string
	if !strings.Contains(v.Title, e.ID) {
		problems = append(problems, fmt.Sprintf(
			"#%d 标题未含编号 %s（实际: %q）——issue 与事实源已失联", *e.Issue, e.ID, v.Title))
	}
	if want := ghState(e.Status); v.State != want {
		problems = append(problems, fmt.Sprintf(
			"#%d 在 GitHub 上为 %s，但事实源标为 %s（期望 %s）；请同步更新 %s",
			*e.Issue, v.State, e.Status, want, sourceJSON))
	}
	return problems
}

// ghState 把事实源状态映射为 gh 报告的 issue 状态值。
func ghState(status string) string {
	if status == "closed" {
		return "CLOSED"
	}
	return "OPEN"
}
