// migrate-changelog 将旧 AI_CHANGELOG.md 拆分为 INDEX.json + 按日期分组的 .md 文件。
//
//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry 表示一条 changelog 条目。
type Entry struct {
	Date  string `json:"date"`
	Type  string `json:"type"`
	Scope string `json:"scope"`
	Title string `json:"title"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Body  string `json:"-"`
}

// Index 是 INDEX.json 的结构。
type Index struct {
	Version int     `json:"version"`
	Updated string  `json:"updated"`
	Total   int     `json:"total"`
	Entries []Entry `json:"entries"`
}

var dateHeaderRe = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2})(?:（[^）]*）)?:?\s*(.*)$`)
var convCommitRe = regexp.MustCompile(`^(feat|fix|refactor|docs|test|chore|style|perf|techdebt)(\(([^)]*)\))?:?\s*(.*)$`)

var knownScopes = []string{
	"tui", "agentcore", "domains", "retrieval", "desktop",
	"mcp", "a2ui", "agui", "a2a", "acp", "server",
	"disclosure", "workflows", "knowledge", "memory",
	"guardrails", "psychological", "tools", "plantask",
	"cmd", "bootstrap", "evaluate", "integration", "search",
}

func main() {
	rootDir := findRootDir()
	archivePath := filepath.Join(rootDir, "docs", "decisions", "ai-changelog", "archive-AI_CHANGELOG.md")
	outDir := filepath.Join(rootDir, "docs", "decisions", "ai-changelog")

	entries := parseArchive(archivePath)
	fmt.Printf("解析到 %d 条记录\n", len(entries))

	byDate := groupByDate(entries)

	for date, dayEntries := range byDate {
		fname := date + ".md"
		writeDateFile(filepath.Join(outDir, fname), date, dayEntries)
		fmt.Printf("写入 %s (%d 条)\n", fname, len(dayEntries))
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Date != entries[j].Date {
			return entries[i].Date > entries[j].Date
		}
		return i < j
	})

	index := Index{
		Version: 1,
		Updated: time.Now().Format(time.RFC3339),
		Total:   len(entries),
		Entries: entries,
	}
	writeJSON(filepath.Join(outDir, "INDEX.json"), index)
	fmt.Printf("写入 INDEX.json (%d 条)\n", len(entries))

	verify(outDir, len(entries))
}

func parseArchive(path string) []Entry {
	//nolint:gosec
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法打开归档文件: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	var entries []Entry
	var current *Entry
	var bodyLines []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if line == "# AI 变更记录" || line == "# AI 变更记录（按日期分组）" {
			continue
		}

		if matches := dateHeaderRe.FindStringSubmatch(line); matches != nil {
			if current != nil {
				current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
				entries = append(entries, *current)
			}

			date := matches[1]
			rawTitle := strings.TrimSpace(matches[2])

			entry := Entry{
				Date:  date,
				Title: rawTitle,
				File:  date + ".md",
			}

			entry.Type, entry.Scope, entry.Title = extractTypeScope(rawTitle)

			current = &entry
			bodyLines = nil
		} else if current != nil {
			bodyLines = append(bodyLines, line)
		}
	}

	if current != nil {
		current.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		entries = append(entries, *current)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取错误: %v\n", err)
		os.Exit(1)
	}

	return entries
}

func extractTypeScope(rawTitle string) (typ, scope, title string) {
	if m := convCommitRe.FindStringSubmatch(rawTitle); m != nil {
		typ = m[1]
		scope = m[3]
		title = strings.TrimSpace(m[4])
		if scope == "" {
			scope = inferScope(title)
		}
		return
	}

	typ = inferType(rawTitle)
	scope = inferScope(rawTitle)
	title = rawTitle
	return
}

func inferType(title string) string {
	lower := strings.ToLower(title)

	switch {
	case strings.Contains(lower, "修复") || strings.Contains(lower, "fix"):
		return "fix"
	case strings.Contains(lower, "新增") || strings.Contains(lower, "feat") || strings.Contains(lower, "引入") || strings.Contains(lower, "接入") || strings.Contains(lower, "添加"):
		return "feat"
	case strings.Contains(lower, "重构") || strings.Contains(lower, "refactor") || strings.Contains(lower, "拆分") || strings.Contains(lower, "迁移"):
		return "refactor"
	case strings.Contains(lower, "文档") || strings.Contains(lower, "docs") || strings.Contains(lower, "审阅") || strings.Contains(lower, "审查"):
		return "docs"
	case strings.Contains(lower, "测试") || strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "chore") || strings.Contains(lower, "ci") || strings.Contains(lower, "lint") || strings.Contains(lower, "技术债务") || strings.Contains(lower, "techdebt"):
		return "chore"
	case strings.Contains(lower, "style") || strings.Contains(lower, "视觉") || strings.Contains(lower, "配色"):
		return "style"
	case strings.Contains(lower, "perf") || strings.Contains(lower, "性能") || strings.Contains(lower, "优化"):
		return "perf"
	default:
		return "chore"
	}
}

func inferScope(text string) string {
	lower := strings.ToLower(text)

	if strings.Contains(lower, "tui") || strings.Contains(lower, "markdown") && strings.Contains(lower, "渲染") {
		return "tui"
	}

	for _, s := range knownScopes {
		if strings.Contains(lower, s) {
			return s
		}
	}

	switch {
	case strings.Contains(lower, "claimdrafting") || strings.Contains(lower, "权利要求"):
		return "domains"
	case strings.Contains(lower, "specdrafting") || strings.Contains(lower, "说明书"):
		return "domains"
	case strings.Contains(lower, "novelty") || strings.Contains(lower, "新颖性"):
		return "domains"
	case strings.Contains(lower, "inventiveness") || strings.Contains(lower, "创造性"):
		return "domains"
	case strings.Contains(lower, "infringement") || strings.Contains(lower, "侵权"):
		return "domains"
	case strings.Contains(lower, "enablement") || strings.Contains(lower, "充分公开"):
		return "domains"
	case strings.Contains(lower, "drafting") || strings.Contains(lower, "撰写"):
		return "domains"
	case strings.Contains(lower, "handoff") || strings.Contains(lower, "router") || strings.Contains(lower, "unified"):
		return "agentcore"
	case strings.Contains(lower, "全仓") || strings.Contains(lower, "全量") || strings.Contains(lower, "全局"):
		return "*"
	case strings.Contains(lower, "代码质量") || strings.Contains(lower, "code") && strings.Contains(lower, "review"):
		return "*"
	default:
		return "*"
	}
}

func groupByDate(entries []Entry) map[string][]*Entry {
	result := make(map[string][]*Entry)
	for i := range entries {
		e := &entries[i]
		result[e.Date] = append(result[e.Date], e)
	}
	return result
}

func writeDateFile(path, date string, entries []*Entry) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", date)

	for i, e := range entries {
		e.Line = sb.Len() + 1

		if e.Scope != "" && e.Scope != "*" {
			fmt.Fprintf(&sb, "## %s(%s): %s\n\n", e.Type, e.Scope, e.Title)
		} else if e.Type != "chore" || !strings.HasPrefix(e.Title, "全量") {
			fmt.Fprintf(&sb, "## %s: %s\n\n", e.Type, e.Title)
		} else {
			fmt.Fprintf(&sb, "## %s\n\n", e.Title)
		}

		sb.WriteString(e.Body)
		sb.WriteString("\n\n")

		if i < len(entries)-1 {
			sb.WriteString("---\n\n")
		}
	}

	//nolint:gosec
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "写入文件失败 %s: %v\n", path, err)
		os.Exit(1)
	}

	recalcLines(path, entries)
}

func recalcLines(path string, entries []*Entry) {
	//nolint:gosec
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	lineNo := 0
	entryIdx := 0
	dateHeaderReLocal := regexp.MustCompile(`^## .+`)

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		if entryIdx < len(entries) && dateHeaderReLocal.MatchString(line) && !strings.HasPrefix(line, "# ") {
			entries[entryIdx].Line = lineNo
			entryIdx++
		}
	}
}

func writeJSON(path string, index Index) {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 序列化失败: %v\n", err)
		os.Exit(1)
	}
	//nolint:gosec
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "写入 INDEX.json 失败: %v\n", err)
		os.Exit(1)
	}
}

func findRootDir() string {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func verify(outDir string, expectedCount int) {
	indexPath := filepath.Join(outDir, "INDEX.json")
	//nolint:gosec
	data, err := os.ReadFile(indexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "验证失败: 无法读取 INDEX.json: %v\n", err)
		os.Exit(1)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		fmt.Fprintf(os.Stderr, "验证失败: INDEX.json 格式错误: %v\n", err)
		os.Exit(1)
	}
	if index.Total != expectedCount {
		fmt.Fprintf(os.Stderr, "验证失败: INDEX.json 条目数 %d != 预期 %d\n", index.Total, expectedCount)
		os.Exit(1)
	}

	for _, e := range index.Entries {
		fpath := filepath.Join(outDir, e.File)
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "验证失败: 日期文件不存在 %s\n", e.File)
			os.Exit(1)
		}
	}

	fmt.Printf("\n✅ 验证通过: %d 条记录，%s\n", index.Total, indexPath)
}
