package tools

// 本文件是内置工具 `inputSchema` 的 golden fixture 护栏（M74）。
//
// 工具 Parameters 会原样进入模型请求，是 LLM 侧最硬的一类契约：字段改名、类型变化、
// required 增删都会让模型调用失败或行为漂移，而且不会编译报错。这里把 BuildTools
// 产出的全部工具 schema 快照成 JSON，任何形状变化都会让本测试变红，必须在提交里
// 显式更新 fixture（`go test ./tools -run TestContractToolSchemas -args -update`）。

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

var updateSchemaGolden = flag.Bool("update", false, "重建 tools/testdata/tool_schemas.golden.json")

const schemaGoldenPath = "testdata/tool_schemas.golden.json"

// buildSchemaSnapshot 构造全部内置工具并抽取 name → Parameters 的映射。
// 使用固定的 WorkingDir，避免临时目录名进入 schema 造成快照不稳定。
func buildSchemaSnapshot(t *testing.T) map[string]any {
	t.Helper()

	built := BuildTools(ExtensionConfig{WorkingDir: "/workspace"})
	if len(built) == 0 {
		t.Fatal("BuildTools 未返回任何工具")
	}

	snapshot := make(map[string]any, len(built))
	for _, tool := range built {
		if tool == nil {
			t.Fatal("BuildTools 返回了 nil 工具")
		}
		if _, dup := snapshot[tool.Name]; dup {
			t.Fatalf("工具名重复：%q", tool.Name)
		}
		snapshot[tool.Name] = tool.Parameters
	}
	return snapshot
}

func TestContractToolSchemas(t *testing.T) {
	snapshot := buildSchemaSnapshot(t)

	got, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("序列化 schema 失败：%v", err)
	}
	got = append(got, '\n')

	if *updateSchemaGolden {
		if err := os.MkdirAll(filepath.Dir(schemaGoldenPath), 0o750); err != nil {
			t.Fatalf("创建 testdata 目录失败：%v", err)
		}
		if err := os.WriteFile(schemaGoldenPath, got, 0o600); err != nil {
			t.Fatalf("写入 golden 失败：%v", err)
		}
		t.Logf("已更新 golden：%s（%d 个工具）", schemaGoldenPath, len(snapshot))
		return
	}

	want, err := os.ReadFile(schemaGoldenPath)
	if err != nil {
		t.Fatalf("读取 golden 失败（首次运行请加 -args -update）：%v", err)
	}
	if string(got) == string(want) {
		return
	}

	// 不一致时列出新增/删除的工具，帮助判断是「漏更新 fixture」还是「误删工具」。
	var gotMap, wantMap map[string]json.RawMessage
	if err := json.Unmarshal(got, &gotMap); err != nil {
		t.Fatalf("解析当前快照失败：%v", err)
	}
	if err := json.Unmarshal(want, &wantMap); err != nil {
		t.Fatalf("解析 golden 失败：%v", err)
	}
	for _, name := range diffKeys(wantMap, gotMap) {
		t.Errorf("工具 %q 在 golden 中存在但当前未注册（疑似误删）", name)
	}
	for _, name := range diffKeys(gotMap, wantMap) {
		t.Errorf("工具 %q 当前已注册但 golden 中缺失（新增工具须更新 fixture）", name)
	}
	t.Fatal("工具 inputSchema 与 golden 不一致：确认改动符合预期后执行 " +
		"`go test ./tools -run TestContractToolSchemas -args -update` 更新 fixture")
}

// diffKeys 返回 a 中有而 b 中没有的键（升序）。
func diffKeys(a, b map[string]json.RawMessage) []string {
	var out []string
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
