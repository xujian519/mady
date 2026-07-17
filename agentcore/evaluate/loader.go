package evaluate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadTestCases reads one or more JSON fixture files from the specified path
// (a single file or a directory) and returns parsed TestCase values.
//
// Each JSON fixture file contains an object with a "cases" array:
//
//	{
//	  "suite": "tool_accuracy",
//	  "description": "Tool selection accuracy tests",
//	  "cases": [
//	    {
//	      "id": "tool_search_correct",
//	      "domain": "patent",
//	      "input": "Search for AI patents",
//	      "expected": "expected tool call output",
//	      "required_citations": ["A22.2", "A22.3"],
//	      "metadata": {
//	        "expected_tools": ["search_patents"],
//	        "difficulty": "easy"
//	      }
//	    }
//	  ]
//	}
//
// The metadata field is opaque and may hold any JSON value; consumers access
// it via type assertion after loading.
type Loader struct {
	// BaseDir is the base directory for resolving relative fixture paths.
	// Defaults to the current working directory if empty.
	BaseDir string
}

// FixtureFile represents the top-level structure of a fixture JSON file.
type FixtureFile struct {
	Suite       string     `json:"suite"`
	Description string     `json:"description,omitempty"`
	Cases       []TestCase `json:"cases"`
}

// TestCaseExtended extends TestCase with an optional metadata field.
type TestCaseExtended struct {
	ID                string                 `json:"id"`
	Domain            string                 `json:"domain"`
	Input             string                 `json:"input"`
	Expected          string                 `json:"expected"`
	RequiredCitations []string               `json:"required_citations,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// LoadDir loads all JSON fixture files from a directory. It skips non-JSON
// files and files starting with ".".
func (l *Loader) LoadDir(dirPath string) (map[string][]TestCase, error) {
	resolved := l.resolve(dirPath)
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("loader: 读取目录 %s 失败: %w", dirPath, err)
	}

	result := make(map[string][]TestCase)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".jsonc") {
			continue
		}

		fixtures, err := l.LoadFile(filepath.Join(resolved, name))
		if err != nil {
			return nil, fmt.Errorf("loader: 加载 %s 失败: %w", name, err)
		}
		for suiteName, cases := range fixtures {
			result[suiteName] = append(result[suiteName], cases...)
		}
	}
	return result, nil
}

// LoadFile loads test cases from a single JSON fixture file. The file may
// contain a single FixtureFile object or a JSON array of FixtureFile objects.
func (l *Loader) LoadFile(filePath string) (map[string][]TestCase, error) {
	resolved := l.resolve(filePath)
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("loader: 读取 %s 失败: %w", filePath, err)
	}

	result := make(map[string][]TestCase)

	// Try single fixture file.
	var fixture FixtureFile
	if err := json.Unmarshal(data, &fixture); err == nil && len(fixture.Cases) > 0 {
		converted := convertCases(fixture.Cases, fixture.Suite)
		result[fixture.Suite] = converted
		return result, nil
	}

	// Try array of fixture files.
	var fixtures []FixtureFile
	if err := json.Unmarshal(data, &fixtures); err == nil {
		hasCases := false
		for _, f := range fixtures {
			if len(f.Cases) > 0 {
				hasCases = true
				converted := convertCases(f.Cases, f.Suite)
				result[f.Suite] = append(result[f.Suite], converted...)
			}
		}
		if hasCases {
			return result, nil
		}
	}

	// Try raw array of TestCaseExtended.
	var extended []TestCaseExtended
	if err := json.Unmarshal(data, &extended); err == nil {
		for _, tc := range extended {
			result["default"] = append(result["default"], TestCase{
				ID:                tc.ID,
				Domain:            tc.Domain,
				Input:             tc.Input,
				Expected:          tc.Expected,
				RequiredCitations: tc.RequiredCitations,
			})
		}
		return result, nil
	}

	return nil, fmt.Errorf("loader: 无法解析 %s: 不是有效的夹具文件格式", filePath)
}

// convertCases transforms raw TestCase entries, setting default domain and
// ensuring IDs are non-empty.
func convertCases(cases []TestCase, suiteHint string) []TestCase {
	out := make([]TestCase, 0, len(cases))
	for _, c := range cases {
		if c.ID == "" {
			continue
		}
		if c.Domain == "" {
			c.Domain = "general"
			if suiteHint != "" && suiteHint != "default" {
				c.Domain = suiteHint
			}
		}
		out = append(out, c)
	}
	return out
}

func (l *Loader) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	base := l.BaseDir
	if base == "" {
		base = "."
	}
	return filepath.Join(base, path)
}

// MustLoad is a convenience helper that loads test cases and panics on error.
// Useful in test helpers and benchmark initialization.
func MustLoad(loader *Loader, path string) []TestCase {
	var cases []TestCase
	// Try as directory first.
	if result, err := loader.LoadDir(path); err == nil {
		for _, cc := range result {
			cases = append(cases, cc...)
		}
		return cases
	}
	// Try as file.
	if result, err := loader.LoadFile(path); err == nil {
		for _, cc := range result {
			cases = append(cases, cc...)
		}
		return cases
	}
	panic(fmt.Sprintf("loader: 无法加载 %s", path))
}
