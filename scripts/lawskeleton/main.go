// Command lawskeleton 把逐部法规的条号索引 YAML 合并成单个紧凑索引文件。
//
// 输入目录里每个 YAML 是一部法规的条级索引（字段见 pkg/indexasset）。输出只保留
// 存在性判定需要的字段：法律简称、全称、修订版本、条数上限、是否已转录条文。
// 骨架的条目本身多为 text: null，合并后体积从数十 MB 降到百 KB 级，也避免把
// 七千多个文件放进版本库。
//
// 用法：
//
//	go run ./scripts/lawskeleton --src <法规目录> --out data/law/skeleton/index.json
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xujian519/mady/pkg/indexasset"
)

// lawEntry 是合并后的一部法规。
type lawEntry struct {
	Name        string `json:"name"`
	Document    string `json:"document"`
	Revision    string `json:"revision"`
	MaxArticle  int    `json:"maxArticle"`
	Transcribed bool   `json:"transcribed"`
}

// indexFile 是合并索引的文件结构。
type indexFile struct {
	GeneratedFrom string     `json:"generatedFrom"`
	Count         int        `json:"count"`
	Laws          []lawEntry `json:"laws"`
}

func main() {
	src := flag.String("src", "", "法规条号索引 YAML 所在目录")
	out := flag.String("out", "data/law/skeleton/index.json", "输出文件")
	label := flag.String("label", "legal-law", "写入索引的来源说明（不写机器路径）")
	flag.Parse()

	if *src == "" {
		fmt.Fprintln(os.Stderr, "lawskeleton: 需要 --src")
		os.Exit(2)
	}
	if err := run(*src, *out, *label); err != nil {
		fmt.Fprintf(os.Stderr, "lawskeleton: %v\n", err)
		os.Exit(1)
	}
}

// run 读取目录并写出合并索引。
func run(src, out, label string) error {
	paths, err := filepath.Glob(filepath.Join(src, "*.yaml"))
	if err != nil {
		return fmt.Errorf("扫描 %s: %w", src, err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("%s 下没有 YAML 文件", src)
	}

	laws := make([]lawEntry, 0, len(paths))
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		//nolint:gosec // path 来自 --src 目录的 glob 结果，只读取其中的法规索引 YAML
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", path, err)
		}
		entry, err := parseSkeleton(data, path)
		if err != nil {
			return err
		}
		if previous, dup := seen[entry.Name]; dup {
			return fmt.Errorf("法律简称重复：%s（%s 与 %s）", entry.Name, previous, path)
		}
		seen[entry.Name] = path
		laws = append(laws, entry)
	}

	sort.Slice(laws, func(i, j int) bool { return laws[i].Name < laws[j].Name })
	payload := indexFile{
		GeneratedFrom: label,
		Count:         len(laws),
		Laws:          laws,
	}
	encoded, err := json.MarshalIndent(payload, "", " ")
	if err != nil {
		return fmt.Errorf("序列化: %w", err)
	}
	encoded = append(encoded, '\n')
	if strings.HasSuffix(out, ".gz") {
		compressed, err := gzipBytes(encoded)
		if err != nil {
			return fmt.Errorf("压缩: %w", err)
		}
		encoded = compressed
	}

	if dir := filepath.Dir(out); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("创建目录 %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(out, encoded, 0o600); err != nil {
		return fmt.Errorf("写入 %s: %w", out, err)
	}

	transcribed := 0
	for _, law := range laws {
		if law.Transcribed {
			transcribed++
		}
	}
	fmt.Printf("已生成 %s：%d 部法规，其中 %d 部带条文转录，%d 字节\n",
		out, len(laws), transcribed, len(encoded))
	return nil
}

// gzipBytes 把内容压缩为 gzip。
//
// 骨架索引未压缩约 1.8 MB，超过 pre-commit 的 500 KB 大文件阈值；压缩后约 1/6，
// 读取方（lawcite.LoadSkeletonFile）按魔数自动识别，无需额外参数。
func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// parseSkeleton 解析一部法规的条号索引。
//
// 与法条精校索引不同，骨架允许缺 document/revision——它们由自动生成流程产出，
// 缺失不影响「条号是否存在」这一唯一用途；但 law 与 articles 仍然必填，
// 否则这份文件无法承担任何判定。
func parseSkeleton(data []byte, path string) (lawEntry, error) {
	fail := indexasset.FailFor(path)
	root, err := indexasset.ParseYAMLMapping(data, "法规条号索引", fail)
	if err != nil {
		return lawEntry{}, err
	}
	name, err := indexasset.ReadString(root, "law", fail)
	if err != nil {
		return lawEntry{}, err
	}
	document, err := indexasset.ReadOptionalString(root, "document", fail)
	if err != nil {
		return lawEntry{}, err
	}
	revision, err := indexasset.ReadOptionalString(root, "revision", fail)
	if err != nil {
		return lawEntry{}, err
	}
	maxArticle, err := indexasset.ReadOptionalCount(root, "maxArticle", fail)
	if err != nil {
		return lawEntry{}, err
	}
	articles, err := indexasset.ReadArticleEntries(root["articles"], fail)
	if err != nil {
		return lawEntry{}, err
	}

	entry := lawEntry{Name: name}
	if document != nil {
		entry.Document = *document
	}
	if revision != nil {
		entry.Revision = *revision
	}
	if maxArticle != nil {
		entry.MaxArticle = *maxArticle
	}
	for _, article := range articles {
		if article.Text != nil {
			entry.Transcribed = true
			break
		}
	}
	return entry, nil
}
