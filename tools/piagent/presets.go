package piagent

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
)

// Preset 定义子会话角色（对齐 Sati builtinSubagentTypes 语义）。
type Preset struct {
	// Name 是预设标识，对应 spawn_agent.subagent_type。
	Name string
	// Description 单行摘要，用于 spawn_agent 工具描述。
	Description string
	// AllowedTools 是预设默认工具白名单（Mady 工具名）。空 = 继承父注册表全量。
	AllowedTools []string
	// IsReadOnly 为 true 时，桥接层在执行前拒绝写类工具（AC-2）。
	IsReadOnly bool
	// SystemPromptSuffix 追加到子会话系统提示词末尾。
	SystemPromptSuffix string
}

// Preset 常量。
const (
	PresetExplore        = "explore"
	PresetVerify         = "verify"
	PresetPlan           = "plan"
	PresetGeneralPurpose = "general-purpose"
)

// 只读预设可见的 Mady 工具白名单。
//
// 注意：verify 预设刻意不含 bash —— Mady bash 工具无只读命令判定能力，
// 无法在桥接层可靠区分「只读命令」与写命令，纳入 bash 会破坏 AC-2 不变量。
// 需要 shell 的只读任务可通过 spawn_agent.tools 显式追加并承担相应风险。
var readOnlyAllowedTools = []string{"read", "grep", "glob", "ls", "find", "view"}

// 写类工具集合：只读预设与 general-purpose 的子会话桥接层均拒绝这些工具。
// 与 tool_domains 的 domain 判定互补（此处为名称级硬边界）。
var writeToolNames = map[string]bool{
	"bash":         true,
	"edit":         true,
	"write_file":   true,
	"delete":       true,
	"move":         true,
	"patch":        true,
	"pandoc":       true,
	"execute_code": true,
	"computer_use": true,
}

// Presets 是全部内置预设。
var Presets = []Preset{
	{
		Name:         PresetExplore,
		Description:  "只读探查子会话：检视文件、搜索内容，不修改任何状态",
		AllowedTools: readOnlyAllowedTools,
		IsReadOnly:   true,
		SystemPromptSuffix: "" +
			"你是探查子会话（explore）：只读模式。允许的工具：" + strings.Join(readOnlyAllowedTools, "/") + "。\n" +
			"禁止写/改/删文件与执行 shell；这些调用会被直接拒绝。\n" +
			"优先使用 grep/glob/find 定位内容，不要用 bash 绕道。\n" +
			"完成探查后按强制输出格式返回报告。",
	},
	{
		Name:         PresetVerify,
		Description:  "只读核验子会话：核验已有产物并报告问题，不修改文件",
		AllowedTools: readOnlyAllowedTools,
		IsReadOnly:   true,
		SystemPromptSuffix: "" +
			"你是核验子会话（verify）：只读模式。允许的工具：" + strings.Join(readOnlyAllowedTools, "/") + "。\n" +
			"核验任务给出的产物/结论是否成立，逐项报告问题与证据，不修改任何文件。\n" +
			"完成核验后按强制输出格式返回报告。",
	},
	{
		Name:         PresetPlan,
		Description:  "只读规划子会话：检视代码并产出逐步执行计划，不执行",
		AllowedTools: readOnlyAllowedTools,
		IsReadOnly:   true,
		SystemPromptSuffix: "" +
			"你是规划子会话（plan）：只读模式。允许的工具：" + strings.Join(readOnlyAllowedTools, "/") + "。\n" +
			"基于检视结果产出可执行的逐步计划（步骤/涉及文件/验证方式），不要执行任何操作。\n" +
			"完成规划后按强制输出格式返回报告。",
	},
	{
		Name:         PresetGeneralPurpose,
		Description:  "通用子会话：继承父工具注册表全量工具（除嵌套 spawn_agent）",
		AllowedTools: nil,
		IsReadOnly:   false,
		SystemPromptSuffix: "" +
			"你是通用子会话（general-purpose）：继承父会话的工具权限，但只限于本次指令范围。\n" +
			"禁止再次派发子会话（spawn_agent 不可用）。\n" +
			"完成工作后按强制输出格式返回报告。",
	},
}

// FindPreset 按名称查找预设，未知名称返回 nil。
func FindPreset(name string) *Preset {
	for i := range Presets {
		if Presets[i].Name == name {
			return &Presets[i]
		}
	}
	return nil
}

// PresetNames 返回全部预设名（用于 spawn_agent 参数枚举）。
func PresetNames() []string {
	out := make([]string, 0, len(Presets))
	for _, p := range Presets {
		out = append(out, p.Name)
	}
	return out
}

// ResolveAllowed 计算子会话生效工具白名单（03-design.md §4.1）：
//
//	allowed = (presetTools ∪ extra) ∩ registered − exclude − {spawn_agent}
//
// general-purpose 预设仅在「未显式指定 tools/exclude_tools」时继承父注册表
// 全量；一旦调用方给出白名单参数（即使排除后为空），走与其余预设相同的
// 显式集合运算，排除不会被忽略。
//
// 返回过滤后的工具名列表与注册表内的对应工具。
func ResolveAllowed(registered []*agentcore.Tool, preset *Preset, extra, exclude []string) []*agentcore.Tool {
	want := map[string]bool{}
	if preset != nil {
		for _, n := range preset.AllowedTools {
			want[n] = true
		}
	}
	for _, n := range extra {
		want[n] = true
	}
	for _, n := range exclude {
		delete(want, n)
	}
	delete(want, "spawn_agent")

	// general-purpose：无显式白名单参数时继承全量注册表（嵌套 spawn_agent 除外）。
	inheritAll := preset != nil && preset.Name == PresetGeneralPurpose &&
		len(extra) == 0 && len(exclude) == 0
	if len(want) == 0 && inheritAll {
		var out []*agentcore.Tool
		for _, t := range registered {
			if t != nil && t.Name != "spawn_agent" {
				out = append(out, t)
			}
		}
		return out
	}

	var out []*agentcore.Tool
	for _, t := range registered {
		if t != nil && want[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

// IsWriteTool 判定工具名是否为写类工具（只读预设执行前调用）。
func IsWriteTool(name string) bool {
	return writeToolNames[name]
}
