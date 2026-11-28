//go:build darwin

package main

// 本文件是「后端 Wails 事件名 ↔ 前端订阅表 / 载荷表」的跨语言契约护栏（M73）。
//
// 背景：AGUI 事件经 mapAguiEventToWailsName 变成 `agui:<kebab>` 后由 Wails 推给
// 前端，前端在 client.ts 订阅、在 reducer.ts 注册处理器、在 payloads.ts 声明载荷。
// 这三处与后端映射表之间没有任何编译期约束——改名只会表现为「前端静默收不到事件」。
// 本测试从 Go 侧推导出后端可产出的事件名集合，再解析三个前端文件比对键集合，
// 任何一侧单边改名都会在此失败。
//
// 新增事件时的正确顺序：后端映射/转换 → 前端 client.ts + reducer.ts + payloads.ts
// → 本测试无需改动（期望集自动跟随后端）。

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

// backendEmittedNames 推导后端可能推送的全部 `agui:*` 事件名（去掉前缀）。
func backendEmittedNames(t *testing.T) map[string]bool {
	t.Helper()

	// 1) AGUI 事件 → Wails 名：显式映射的 6 类走类型分支，
	//    其余走 default 分支（kebab(EventType)），自定义事件走 kebab(CustomEvent.Name)。
	events := []any{
		agui.RunStartedEvent{},
		agui.TextMessageContentEvent{},
		agui.ThinkingTextMessageContentEvent{},
		agui.ToolCallStartEvent{},
		agui.ToolCallEndEvent{},
		agui.RunErrorEvent{},
		agui.StepStartedEvent{BaseEvent: agui.BaseEvent{Type: agui.EventStepStarted}},
		agui.StepFinishedEvent{BaseEvent: agui.BaseEvent{Type: agui.EventStepFinished}},
		agui.ToolCallArgsEvent{BaseEvent: agui.BaseEvent{Type: agui.EventToolCallArgs}},
		agui.ToolCallResultEvent{BaseEvent: agui.BaseEvent{Type: agui.EventToolCallResult}},
		agui.RunFinishedEvent{BaseEvent: agui.BaseEvent{Type: agui.EventRunFinished}},
		agui.ContextUsageEvent{BaseEvent: agui.BaseEvent{Type: agui.EventContextUsage}},
		agui.CustomEvent{Name: "compaction_start"},
		agui.CustomEvent{Name: "compaction_end"},
		agui.CustomEvent{Name: "auto_retry"},
		agui.CustomEvent{Name: "a2ui"},
	}

	names := make(map[string]bool, len(events)+4)
	for _, ev := range events {
		wails := mapAguiEventToWailsName(ev)
		if !strings.HasPrefix(wails, "agui:") {
			t.Fatalf("事件 %T 的 Wails 名 %q 缺少 agui: 前缀", ev, wails)
		}
		names[strings.TrimPrefix(wails, "agui:")] = true
	}

	// 2) 由 agentcore 事件类型直接 kebab 得到的三项（未经过 AGUI 转换）。
	for _, et := range []agentcore.EventType{
		agentcore.EventApprovalPrompt,
		agentcore.EventTaskCreated,
		agentcore.EventTaskUpdated,
	} {
		names[toKebabCase(string(et))] = true
	}

	// 3) 流结束信号由 app_chat.go 直接 emit，不经过映射函数。
	appChat, err := os.ReadFile("app_chat.go")
	if err != nil {
		t.Fatalf("读取 app_chat.go 失败：%v", err)
	}
	if !strings.Contains(string(appChat), `"agui:done"`) {
		t.Fatalf("app_chat.go 中未找到 agui:done 直发点，前端的 done 订阅失去依据")
	}
	names["done"] = true

	return names
}

var (
	reQuotedName  = regexp.MustCompile(`'([a-z0-9][a-z0-9-]*)'`)
	reBareKeyName = regexp.MustCompile(`^\s+'?([a-z0-9][a-z0-9-]*)'?:\s*handle`)
	reTypeKeyName = regexp.MustCompile(`^\s+'?([a-z0-9][a-z0-9-]*)'?:\s*\S+`)
)

// sliceBetween 截取 start 与 end 之间的文本，用于解析前端文件里的登记块。
func sliceBetween(t *testing.T, path, start, end string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", "src", "agui-bridge", path))
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	text := string(raw)
	begin := strings.Index(text, start)
	if begin < 0 {
		t.Fatalf("%s 中未找到 %q", path, start)
	}
	rest := text[begin+len(start):]
	stop := strings.Index(rest, end)
	if stop < 0 {
		t.Fatalf("%s 中未找到 %q 的结束标记 %q", path, start, end)
	}
	return rest[:stop]
}

func TestWailsEventNamesMatchFrontend(t *testing.T) {
	backends := backendEmittedNames(t)

	subscribed := collectNames(t, sliceBetween(t, "client.ts", "const SUBSCRIBED_EVENTS = [", "] as const"), reQuotedName)
	handlers := collectNames(t, sliceBetween(t, "reducer.ts", "const HANDLERS: AguiHandlers = {", "\n}"), reBareKeyName)
	payloads := collectNames(t, sliceBetween(t, "payloads.ts", "export interface AguiEventPayloads {", "\n}"), reTypeKeyName)

	tables := []struct {
		name string
		got  map[string]bool
	}{
		{"client.ts SUBSCRIBED_EVENTS", subscribed},
		{"reducer.ts HANDLERS", handlers},
		{"payloads.ts AguiEventPayloads", payloads},
	}
	for _, table := range tables {
		if diff := setDiff(backends, table.got); len(diff) > 0 {
			t.Errorf("%s 缺事件：%v（后端可产出但前端未登记）", table.name, diff)
		}
		if diff := setDiff(table.got, backends); len(diff) > 0 {
			t.Errorf("%s 多事件：%v（前端登记了后端不会产出的名字）", table.name, diff)
		}
	}
}

func collectNames(t *testing.T, block string, re *regexp.Regexp) map[string]bool {
	t.Helper()
	names := make(map[string]bool)
	for _, line := range strings.Split(block, "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			names[m[1]] = true
		}
	}
	if len(names) == 0 {
		t.Fatalf("解析结果为空，检查 %q 的格式是否变化", re.String())
	}
	return names
}

// setDiff 返回 a 中有而 b 中没有的元素（升序）。
func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
