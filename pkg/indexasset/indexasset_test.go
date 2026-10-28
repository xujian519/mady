package indexasset

import (
	"errors"
	"strings"
	"testing"
)

func mustFail(t *testing.T, origin string) Fail {
	t.Helper()
	return FailFor(origin)
}

func TestParseYAMLMapping(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		wantErr string
	}{
		{name: "正常映射", source: "law: 专利法\n"},
		{name: "语法错误", source: "law: [", wantErr: "YAML 解析失败"},
		{name: "根是数组", source: "- a\n- b\n", wantErr: "根节点必须是映射"},
		{name: "根是字符串", source: "hello\n", wantErr: "根节点必须是映射"},
		{name: "空文档", source: "", wantErr: "根节点必须是映射"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseYAMLMapping([]byte(tc.source), "测试索引", mustFail(t, "a.yaml"))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRequireKnownFields(t *testing.T) {
	fail := mustFail(t, "law.yaml")
	root := map[string]any{"law": "专利法", "revision": "2020 年修正"}
	if err := RequireKnownFields(root, []string{"law", "revision"}, "根节点", fail); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root["typo"] = 1
	root["another"] = 2
	err := RequireKnownFields(root, []string{"law", "revision"}, "根节点", fail)
	if err == nil {
		t.Fatal("want error for unknown fields")
	}
	if !strings.Contains(err.Error(), "another, typo") {
		t.Fatalf("unknown fields must be sorted in message, got %q", err.Error())
	}
}

func TestReadString(t *testing.T) {
	fail := mustFail(t, "")
	cases := []struct {
		name    string
		root    map[string]any
		want    string
		wantErr string
	}{
		{name: "正常", root: map[string]any{"k": "v"}, want: "v"},
		{name: "缺失", root: map[string]any{}, wantErr: "必须是非空字符串"},
		{name: "空串", root: map[string]any{"k": "  "}, wantErr: "必须是非空字符串"},
		{name: "类型错", root: map[string]any{"k": 1}, wantErr: "必须是非空字符串"},
		{name: "null", root: map[string]any{"k": nil}, wantErr: "必须是非空字符串"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadString(tc.root, "k", fail)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestReadOptionalString(t *testing.T) {
	fail := mustFail(t, "")
	t.Run("缺失返回 nil", func(t *testing.T) {
		got, err := ReadOptionalString(map[string]any{}, "k", fail)
		if err != nil || got != nil {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("null 返回 nil", func(t *testing.T) {
		got, err := ReadOptionalString(map[string]any{"k": nil}, "k", fail)
		if err != nil || got != nil {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("空串报错", func(t *testing.T) {
		_, err := ReadOptionalString(map[string]any{"k": ""}, "k", fail)
		if err == nil {
			t.Fatal("want error")
		}
	})
}

func TestReadOptionalDate(t *testing.T) {
	fail := mustFail(t, "")
	cases := []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "正常", value: "2026-09-26"},
		{name: "格式错", value: "2026/09/26", wantErr: "YYYY-MM-DD"},
		{name: "不存在的日期", value: "2026-13-45", wantErr: "不是有效日期"},
		{name: "类型错", value: 20260926, wantErr: "YYYY-MM-DD"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadOptionalDate(map[string]any{"d": tc.value}, "d", fail)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestReadOptionalDateAcceptsYAMLTimestamp(t *testing.T) {
	// yaml.v3 把未加引号的日期解析成 time.Time；随包资产两种写法都有。
	fail := mustFail(t, "")
	root, err := ParseYAMLMapping([]byte("verifiedOn: 2026-09-26\n"), "测试索引", fail)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := ReadOptionalDate(root, "verifiedOn", fail)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got == nil || *got != "2026-09-26" {
		t.Fatalf("got %v", got)
	}
}

func TestReadCount(t *testing.T) {
	fail := mustFail(t, "")
	cases := []struct {
		name    string
		value   any
		want    int
		wantErr bool
	}{
		{name: "正常", value: 3, want: 3},
		{name: "零", value: 0, wantErr: true},
		{name: "负数", value: -1, wantErr: true},
		{name: "小数", value: 1.5, wantErr: true},
		{name: "字符串", value: "3", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadCount(tc.value, "字段 n", fail)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %d", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v; want %d", got, err, tc.want)
			}
		})
	}
}

func TestReadOptionalBoolean(t *testing.T) {
	fail := mustFail(t, "")
	if got, err := ReadOptionalBoolean(map[string]any{"b": true}, "b", fail); err != nil || got == nil || !*got {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := ReadOptionalBoolean(map[string]any{}, "b", fail); err != nil || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ReadOptionalBoolean(map[string]any{"b": "true"}, "b", fail); err == nil {
		t.Fatal("want error for string boolean")
	}
}

func TestReadEnum(t *testing.T) {
	fail := mustFail(t, "")
	if got, err := ReadEnum("warn", []string{"block", "warn"}, "level", fail); err != nil || got != "warn" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ReadEnum("log", []string{"block", "warn"}, "level", fail); err == nil {
		t.Fatal("want error for value outside set")
	}
	if _, err := ReadEnum(1, []string{"block"}, "level", fail); err == nil {
		t.Fatal("want error for non-string")
	}
}

func TestCheckRecordedSource(t *testing.T) {
	fail := mustFail(t, "")
	text := "条文"
	doc := "官方全文"
	day := "2026-09-26"

	full := RecordedSource{SourceDoc: &doc, VerifiedOn: &day}
	half := RecordedSource{SourceDoc: &doc}
	none := RecordedSource{}

	cases := []struct {
		name    string
		text    *string
		source  RecordedSource
		wantErr bool
	}{
		{name: "有条文且有完整来源", text: &text, source: full},
		{name: "无条文且无来源", text: nil, source: none},
		{name: "有条文但缺来源", text: &text, source: none, wantErr: true},
		{name: "有条文但来源不全", text: &text, source: half, wantErr: true},
		{name: "无条文却有来源", text: nil, source: full, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckRecordedSource(tc.text, tc.source, "第2条", fail)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestReadEntryTopics(t *testing.T) {
	fail := mustFail(t, "")
	got, err := ReadEntryTopics([]any{"新颖性", "现有技术"}, "第22条", fail)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range []any{[]any{}, nil, "新颖性", []any{"ok", 1}, []any{"ok", "  "}} {
		if _, err := ReadEntryTopics(bad, "第22条", fail); err == nil {
			t.Fatalf("want error for %v", bad)
		}
	}
}

func TestReadArticleEntries(t *testing.T) {
	fail := mustFail(t, "law.yaml")
	source := `
articles:
  - article: 2
    topics: [发明创造, 定义]
    paragraphs: [1, 2]
    text: "本法所称的发明创造是指发明、实用新型和外观设计。"
    sourceDoc: 官方全文
    verifiedOn: 2026-09-26
  - article: 5
    topics: [违反法律]
`
	root, err := ParseYAMLMapping([]byte(source), "测试索引", fail)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	articles, err := ReadArticleEntries(root["articles"], fail)
	if err != nil {
		t.Fatalf("read entries: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("got %d articles", len(articles))
	}
	if articles[0].Article != 2 || articles[0].Text == nil || articles[0].RecordedSource.VerifiedOn == nil {
		t.Fatalf("first article = %+v", articles[0])
	}
	if articles[1].Article != 5 || articles[1].Text != nil || articles[1].RecordedSource.SourceDoc != nil {
		t.Fatalf("second article = %+v", articles[1])
	}
}

func TestReadArticleEntriesRejects(t *testing.T) {
	fail := mustFail(t, "law.yaml")
	cases := []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "不是数组", value: "x", wantErr: "必须是数组"},
		{name: "缺 article", value: []any{map[string]any{"topics": []any{"a"}}}, wantErr: "article 必须是正整数"},
		{name: "条号重复", value: []any{
			map[string]any{"article": 2, "topics": []any{"a"}},
			map[string]any{"article": 2, "topics": []any{"b"}},
		}, wantErr: "article 重复"},
		{name: "条号为零", value: []any{map[string]any{"article": 0, "topics": []any{"a"}}}, wantErr: "必须是正整数"},
		{name: "topics 为空", value: []any{map[string]any{"article": 2, "topics": []any{}}}, wantErr: "topics 必须是非空数组"},
		{name: "paragraphs 为空数组", value: []any{map[string]any{
			"article": 2, "topics": []any{"a"}, "paragraphs": []any{},
		}}, wantErr: "paragraphs 必须是 null 或非空数组"},
		{name: "paragraphs 非正整数", value: []any{map[string]any{
			"article": 2, "topics": []any{"a"}, "paragraphs": []any{0},
		}}, wantErr: "只能是正整数"},
		{name: "有条文无来源", value: []any{map[string]any{
			"article": 2, "topics": []any{"a"}, "text": "条文",
		}}, wantErr: "必须同时记录"},
		{name: "无条文有来源", value: []any{map[string]any{
			"article": 2, "topics": []any{"a"}, "sourceDoc": "官方全文", "verifiedOn": "2026-09-26",
		}}, wantErr: "不能单独记录"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadArticleEntries(tc.value, fail)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestErrorCarriesOrigin(t *testing.T) {
	_, err := ReadString(map[string]any{}, "k", FailFor("/abs/law.yaml"))
	if err == nil {
		t.Fatal("want error")
	}
	var assetErr *Error
	if !errors.As(err, &assetErr) {
		t.Fatalf("error type = %T", err)
	}
	if assetErr.Origin != "/abs/law.yaml" {
		t.Fatalf("origin = %q", assetErr.Origin)
	}
	if !strings.Contains(err.Error(), "/abs/law.yaml") {
		t.Fatalf("error text = %q", err.Error())
	}
}
