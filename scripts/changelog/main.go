// changelog 向 AI_CHANGELOG 追加一条新记录，自动更新 INDEX.json 和日期文件。
//
// Usage:
//
//	go run scripts/changelog/main.go \
//	  --type=feat --scope=tui \
//	  --title="修复 Markdown 结构塌陷" \
//	  --body="**背景**：...
//
//	**改动清单**：...
//
//	**验证**：...
//
//	**影响**：..."
//
// 若省略 --body，则从 stdin 读取。
// 若省略 --date，默认使用今天日期。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Index 是 INDEX.json 的结构（与迁移脚本一致）。
type Index struct {
	Version int     `json:"version"`
	Updated string  `json:"updated"`
	Total   int     `json:"total"`
	Entries []Entry `json:"entries"`
}

// Entry 表示一条 changelog 条目。
type Entry struct {
	Date  string `json:"date"`
	Type  string `json:"type"`
	Scope string `json:"scope"`
	Title string `json:"title"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

func main() {
	date := flag.String("date", "", "日期 (YYYY-MM-DD)，默认今天")
	typ := flag.String("type", "", "变更类型 (feat/fix/refactor/docs/test/chore/style/perf)")
	scope := flag.String("scope", "", "影响范围 (tui/agentcore/domains/...)")
	title := flag.String("title", "", "变更标题（一行）")
	body := flag.String("body", "", "变更详细内容（Markdown）")
	flag.Parse()

	if *typ == "" || *title == "" {
		printUsage()
		os.Exit(1)
	}

	validTypes := map[string]bool{
		"feat": true, "fix": true, "refactor": true, "docs": true,
		"test": true, "chore": true, "style": true, "perf": true,
	}
	if !validTypes[*typ] {
		fmt.Fprintf(os.Stderr, "无效的 type: %s，有效值: feat/fix/refactor/docs/test/chore/style/perf\n", *typ)
		os.Exit(1)
	}

	today := *date
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}

	bodyText := *body
	if bodyText == "" {
		bodyText = readStdin()
	}
	if bodyText == "" {
		fmt.Fprintf(os.Stderr, "body 不能为空，请通过 --body 或 stdin 提供\n")
		os.Exit(1)
	}

	rootDir := findRootDir()
	outDir := filepath.Join(rootDir, "docs", "decisions", "ai-changelog")
	indexPath := filepath.Join(outDir, "INDEX.json")

	index := loadIndex(indexPath)
	dateFile := today + ".md"
	dateFilePath := filepath.Join(outDir, dateFile)
	newLine := determineLine(dateFilePath)

	newEntry := Entry{
		Date:  today,
		Type:  *typ,
		Scope: *scope,
		Title: *title,
		File:  dateFile,
		Line:  newLine,
	}

	index.Entries = append(index.Entries, newEntry)
	sortEntries(index.Entries)
	index.Total = len(index.Entries)
	index.Updated = time.Now().Format(time.RFC3339)

	writeJSON(indexPath, index)
	appendToDateFile(dateFilePath, newEntry, bodyText)

	fmt.Printf("✅ 已追加: [%s][%s] %s → %s (INDEX.json total=%d)\n",
		*typ, *scope, *title, dateFile, index.Total)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "用法: go run scripts/changelog/main.go --type=<type> --scope=<scope> --title=\"...\" [--body=\"...\"]\n")
	fmt.Fprintf(os.Stderr, "  --type   必填: feat/fix/refactor/docs/test/chore/style/perf\n")
	fmt.Fprintf(os.Stderr, "  --scope  可选: tui/agentcore/domains/...\n")
	fmt.Fprintf(os.Stderr, "  --title  必填: 变更标题（一行）\n")
	fmt.Fprintf(os.Stderr, "  --body   可选: 详细内容，省略则从 stdin 读取\n")
	fmt.Fprintf(os.Stderr, "  --date   可选: YYYY-MM-DD，默认今天\n")
}

func readStdin() string {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func loadIndex(path string) Index {
	//nolint:gosec // path comes from findRootDir which locates go.work
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法读取 INDEX.json: %v\n", err)
		os.Exit(1)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		fmt.Fprintf(os.Stderr, "INDEX.json 格式错误: %v\n", err)
		os.Exit(1)
	}
	return index
}

func determineLine(dateFilePath string) int {
	//nolint:gosec // path is constructed from findRootDir + known subdirectory
	f, err := os.Open(dateFilePath)
	if err != nil {
		return 3 // 文件不存在，新文件从第 3 行开始
	}
	defer func() { _ = f.Close() }()

	lines := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines++
	}
	return lines + 2
}

func appendToDateFile(path string, entry Entry, body string) {
	_, err := os.Stat(path)
	isNew := os.IsNotExist(err)

	//nolint:gosec // path is constructed from findRootDir + known subdirectory
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法打开日期文件: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	if isNew {
		_, _ = fmt.Fprintf(f, "# %s\n\n", entry.Date)
	} else {
		_, _ = fmt.Fprintf(f, "\n---\n\n")
	}

	if entry.Scope != "" {
		_, _ = fmt.Fprintf(f, "## %s(%s): %s\n\n", entry.Type, entry.Scope, entry.Title)
	} else {
		_, _ = fmt.Fprintf(f, "## %s: %s\n\n", entry.Type, entry.Title)
	}

	_, _ = fmt.Fprint(f, body)
	_, _ = fmt.Fprint(f, "\n")
}

func writeJSON(path string, index Index) {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON 序列化失败: %v\n", err)
		os.Exit(1)
	}
	//nolint:gosec // path is constructed from findRootDir + known subdirectory
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "写入 INDEX.json 失败: %v\n", err)
		os.Exit(1)
	}
}

func sortEntries(entries []Entry) {
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].Date < entries[j].Date {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
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
