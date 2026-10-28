// Command rulescan 扫描 deepseek-harness 的规则资产，筛出可接入 Mady 确定性门禁的规则。
//
// 用法：
//
//	go run ./scripts/rulescan --src <dsh 规则目录> [--json]
//
// 判定口径：
//   - 可接入：check.type 属于 Mady 的确定性检查类型，且 check.method 不是 llm_judge；
//   - 跳过：需要 LLM 判断、或使用了 Mady 未实现的检查类型（witness / chain_of_custody 等）。
//
// 扫描结果用于逐条核对转换；转换后的规则放入 domains/rules/data/rules/gate-batch-*.yaml。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// supportedTypes 与 domains/rules/gate.go 的 ValidateGateRules 保持一致。
var supportedTypes = map[string]bool{
	"presence": true, "exist": true, "must_include": true,
	"absence": true, "must_not": true, "forbidden": true,
	"numeric": true, "range": true, "numerical": true,
	"composition": true, "compound": true,
	"keyword_blocklist": true, "structural_analysis": true,
}

// entry 是一条规则的扫描结果。
type entry struct {
	File        string `json:"file"`
	Key         string `json:"key"`
	ID          string `json:"id"`
	Action      string `json:"action"`
	Type        string `json:"type"`
	Domain      string `json:"domain"`
	Severity    string `json:"severity"`
	Convertible bool   `json:"convertible"`
	Reason      string `json:"reason"`
	Keywords    int    `json:"keywords"`
	RequiresAll int    `json:"requiresAll"`
}

func main() {
	src := flag.String("src", "", "dsh 规则资产目录")
	asJSON := flag.Bool("json", false, "输出 JSON")
	emitDir := flag.String("emit", "", "生成 Mady 格式门禁规则到该目录（批 A：block + 结构性）")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "rulescan: 需要 --src")
		os.Exit(2)
	}
	if *emitDir != "" {
		if err := emitBatchA(*src, *emitDir); err != nil {
			fmt.Fprintf(os.Stderr, "rulescan: %v\n", err)
			os.Exit(1)
		}
		return
	}

	entries, err := scan(*src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rulescan: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		encoded, err := json.MarshalIndent(entries, "", " ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rulescan: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(encoded))
		return
	}

	convertible, skipped := 0, 0
	for _, item := range entries {
		if item.Convertible {
			convertible++
		} else {
			skipped++
		}
	}
	fmt.Printf("规则总数 %d，可接入确定性门禁 %d，跳过 %d\n\n", len(entries), convertible, skipped)
	fmt.Println("== 可接入 ==")
	for _, item := range entries {
		if item.Convertible {
			fmt.Printf("%-24s %-8s %-22s %s\n", item.ID, item.Action, item.Type, item.File)
		}
	}
	fmt.Println("\n== 跳过 ==")
	for _, item := range entries {
		if !item.Convertible {
			fmt.Printf("%-24s %-30s %s\n", item.ID, item.Reason, item.File)
		}
	}
}

// emitBatchA 生成批 A 门禁规则：action=block 或 check.type=structural_analysis，
// 且通过确定性检查筛选；按 block / structural 拆成两个文件。
//
// 转换是机械的：id/name/description/legalBasis/domain/severity/action 与 check 全字段
// 原样保留（Mady 的 Check 会把未识别字段收进 Extra）。dsh 的 log 级规则不纳入——
// Mady 没有 log 动作，降级成 warn 会改变语义。
type record struct {
	rule map[string]any
	id   string
}

// manualGateRuleIDs 是 Mady 手工维护的门禁规则，生成时跳过以免重复。
var manualGateRuleIDs = map[string]bool{
	"PAT-RISK-001":     true,
	"PAT-APPROVAL-001": true,
	"PAT-ABS-001":      true,
}

func emitBatchA(src, outDir string) error {
	var blocks, structural, batchB []record
	seen := map[string]bool{}

	err := filepath.WalkDir(src, func(path string, dir fs.DirEntry, err error) error {
		if err != nil || dir.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		//nolint:gosec // path 来自 --src 目录的 WalkDir 结果，只读取其中的规则 YAML
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil
		}
		for _, rule := range ruleMaps(document["rules"]) {
			item := classify("", rule)
			if !item.Convertible || seen[item.ID] || item.Action == "log" || manualGateRuleIDs[item.ID] {
				continue
			}
			seen[item.ID] = true
			switch {
			case item.Action == "block":
				blocks = append(blocks, record{rule, item.ID})
			case item.Type == "structural_analysis":
				structural = append(structural, record{rule, item.ID})
			case item.Action == "warn" || item.Action == "review":
				// 批 B：warn/review 级非结构性规则，只进输出门禁做提示。
				batchB = append(batchB, record{rule, item.ID})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := writeRuleFile(outDir, "gate-batch-a-block.yaml", "批 A-1：block 级确定性门禁规则", blocks); err != nil {
		return err
	}
	if err := writeRuleFile(outDir, "gate-batch-a-structural.yaml", "批 A-2：结构性门禁规则（供 StructuralGate 判定交付制品）", structural); err != nil {
		return err
	}
	return writeRuleFile(outDir, "gate-batch-b-warn.yaml", "批 B：warn/review 级非结构性确定性门禁规则（只提示，不阻断）", batchB)
}

// writeRuleFile 写出一份 Mady 格式规则文件。
func writeRuleFile(outDir, name, title string, records []record) error {
	sort.SliceStable(records, func(i, j int) bool { return records[i].id < records[j].id })
	rules := make([]map[string]any, 0, len(records))
	for _, item := range records {
		rules = append(rules, normalizeRule(item.rule, item.id))
	}
	payload := map[string]any{"rules": rules}
	encoded, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("# %s\n#\n# 由 scripts/rulescan 从 deepseek-harness 规则资产机械转换生成，请勿手改；\n# 重新生成：go run ./scripts/rulescan --src <dsh 规则目录> --emit domains/rules/data/rules/gate\n#\n# 转换口径：保留 id/name/description/legalBasis/domain/severity/action 与 check 全字段；\n# 仅收录可确定性判定的规则，log 级规则不纳入。\n\n", title)
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, append([]byte(header), encoded...), 0o600); err != nil {
		return err
	}
	fmt.Printf("已生成 %s（%d 条）\n", path, len(records))
	return nil
}

// normalizeRule 把 dsh 规则映射转为 Mady 字段名。
func normalizeRule(rule map[string]any, id string) map[string]any {
	out := map[string]any{"ruleId": id}
	for _, field := range []string{"name", "description", "legalBasis", "domain", "severity", "action"} {
		if value, ok := rule[field]; ok && value != nil {
			out[field] = value
		}
	}
	if check, ok := rule["check"].(map[string]any); ok {
		copied := make(map[string]any, len(check))
		for key, value := range check {
			if key != "__key" {
				copied[key] = value
			}
		}
		out["check"] = copied
	}
	return out
}

// scan 遍历目录并返回规则清单。
func scan(src string) ([]entry, error) {
	var entries []entry
	err := filepath.WalkDir(src, func(path string, dir fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dir.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		//nolint:gosec // path 来自 --src 目录的 WalkDir 结果，只读取其中的规则 YAML
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := yaml.Unmarshal(data, &document); err != nil {
			fmt.Fprintf(os.Stderr, "rulescan: %s 解析失败（已跳过）：%v\n", path, err)
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		for _, rule := range ruleMaps(document["rules"]) {
			entries = append(entries, classify(rel, rule))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

// ruleMaps 把 rules 字段规范化为规则映射列表（支持映射与数组两种写法）。
func ruleMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			if rule, ok := typed[key].(map[string]any); ok {
				if _, exists := rule["__key"]; !exists {
					rule["__key"] = key
				}
				out = append(out, rule)
			}
		}
		return out
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if rule, ok := item.(map[string]any); ok {
				out = append(out, rule)
			}
		}
		return out
	default:
		return nil
	}
}

// classify 判定一条规则能否接入确定性门禁。
func classify(file string, rule map[string]any) entry {
	check, _ := rule["check"].(map[string]any)
	checkType := stringField(check, "type")
	method := stringField(check, "method")

	item := entry{
		File:        file,
		Key:         stringField(rule, "__key"),
		ID:          stringField(rule, "id"),
		Action:      stringField(rule, "action"),
		Type:        checkType,
		Domain:      stringField(rule, "domain"),
		Severity:    stringField(rule, "severity"),
		Keywords:    len(stringSlice(check["keywords"])),
		RequiresAll: len(anySlice(check["requiresAll"])),
	}
	if item.ID == "" {
		item.ID = item.Key
	}

	switch {
	case check == nil:
		item.Reason = "check 不是映射"
	case method == "llm_judge":
		item.Reason = "需要 LLM 判断"
	case !supportedTypes[checkType]:
		item.Reason = fmt.Sprintf("类型 %s 未实现确定性检查", checkType)
	default:
		item.Convertible = true
	}
	return item
}

// stringField 读取字符串字段。
func stringField(mapping map[string]any, key string) string {
	if mapping == nil {
		return ""
	}
	text, _ := mapping[key].(string)
	return text
}

// stringSlice 读取字符串数组长度。
func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// anySlice 读取任意数组。
func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}
