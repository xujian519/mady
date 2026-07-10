package agentcore

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// SystemPromptConfig — System Prompt 分段设计
// ---------------------------------------------------------------------------

// SystemPromptConfig 实现了 System Prompt 的 Static/Dynamic 边界分离。
//
// 借鉴 Claude Code 的 prefix caching 设计：
//   - StaticPrefix：角色定义、安全规则、通用指南（可缓存，每会话不变）
//   - ToolIndex：工具描述索引（工具注册后生成，变化时重算）
//   - DynamicSuffix：会话级上下文（环境、日期、项目信息、CLAUDE.md）
//
// CacheControl 标记可直接运用在 StaticPrefix 对应的 Message 上，
// 利用已有的 Message.CacheControl 字段实现 prompt caching。
type SystemPromptConfig struct {
	// StaticPrefix 是系统提示的静态部分。
	// 包含角色定义、行为规范、安全约束等不随会话变化的内容。
	// 生产环境中应在此 Message 上设置 CacheControl 标记。
	StaticPrefix string `json:"static_prefix,omitempty"`

	// ToolIndex 是工具索引描述。
	// 当工具注册/卸载时更新，包含可用工具的简要说明。
	ToolIndex string `json:"tool_index,omitempty"`

	// DynamicSuffix 是会话级的动态上下文。
	// 包含：当前日期、工作目录、项目上下文、CLAUDE.md 内容等。
	DynamicSuffix string `json:"dynamic_suffix,omitempty"`

	// Segments 是额外分段的有序列表。
	// 每个 Segment 可以携带独立的 CacheControl 标记。
	Segments []SystemPromptSegment `json:"segments,omitempty"`

	// Separator 是分段之间的分隔符。默认 "\n\n---\n\n"。
	Separator string `json:"separator,omitempty"`
}

// SystemPromptSegment 是一个独立的 system prompt 分段。
type SystemPromptSegment struct {
	// Name 是分段名称（用于诊断和日志）。
	Name string `json:"name,omitempty"`
	// Content 是分段内容。
	Content string `json:"content"`
	// Cacheable 标记此段是否可以缓存（用于 CacheControl）。
	Cacheable bool `json:"cacheable,omitempty"`
	// Priority 控制段的排序（数字越小越靠前）。
	Priority int `json:"priority,omitempty"`
}

// DefaultSystemPromptConfig 返回默认配置。
func DefaultSystemPromptConfig() SystemPromptConfig {
	return SystemPromptConfig{
		Separator: "\n\n---\n\n",
	}
}

// SystemPromptBuilder 构建分段式系统提示。
type SystemPromptBuilder struct {
	cfg SystemPromptConfig
}

// NewSystemPromptBuilder 创建构建器。
func NewSystemPromptBuilder(cfg SystemPromptConfig) *SystemPromptBuilder {
	return &SystemPromptBuilder{cfg: cfg}
}

// Build 构建完整的系统提示词字符串。
func (b *SystemPromptBuilder) Build() string {
	var parts []string

	sep := b.cfg.Separator
	if sep == "" {
		sep = "\n\n---\n\n"
	}

	if b.cfg.StaticPrefix != "" {
		parts = append(parts, b.cfg.StaticPrefix)
	}

	// 自定义段（按 Priority 排序后插入）
	if len(b.cfg.Segments) > 0 {
		sorted := make([]SystemPromptSegment, len(b.cfg.Segments))
		copy(sorted, b.cfg.Segments)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority < sorted[j].Priority
		})
		for _, seg := range sorted {
			if seg.Content != "" {
				parts = append(parts, seg.Content)
			}
		}
	}

	// ToolIndex
	if b.cfg.ToolIndex != "" {
		parts = append(parts, b.cfg.ToolIndex)
	}

	// DynamicSuffix（最后，最靠近对话）
	if b.cfg.DynamicSuffix != "" {
		parts = append(parts, b.cfg.DynamicSuffix)
	}

	return strings.Join(parts, sep)
}

// BuildSegments 构建分段的消息列表，每段可独立设置 CacheControl。
//
// 输出格式：
//
//	[0] Message{Role: system, Content: static, CacheControl: {Type: "ephemeral"}}
//	[1] Message{Role: system, Content: dynamic, CacheControl: nil}
func (b *SystemPromptBuilder) BuildSegments() []Message {
	var msgs []Message

	sep := b.cfg.Separator
	if sep == "" {
		sep = "\n\n---\n\n"
	}

	if b.cfg.StaticPrefix != "" {
		msg := Message{
			Role:    RoleSystem,
			Content: b.cfg.StaticPrefix,
		}
		if hasCacheableContent(b.cfg.StaticPrefix) {
			msg.CacheControl = &CacheControlMarker{Type: "ephemeral"}
		}
		msgs = append(msgs, msg)
	}

	// 自定义段
	if len(b.cfg.Segments) > 0 {
		sorted := make([]SystemPromptSegment, len(b.cfg.Segments))
		copy(sorted, b.cfg.Segments)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority < sorted[j].Priority
		})
		for _, seg := range sorted {
			if seg.Content == "" {
				continue
			}
			msg := Message{Role: RoleSystem, Content: seg.Content}
			if seg.Cacheable {
				msg.CacheControl = &CacheControlMarker{Type: "ephemeral"}
			}
			msgs = append(msgs, msg)
		}
	}

	// ToolIndex
	if b.cfg.ToolIndex != "" {
		msgs = append(msgs, Message{
			Role:    RoleSystem,
			Content: b.cfg.ToolIndex,
		})
	}

	// DynamicSuffix
	if b.cfg.DynamicSuffix != "" {
		msgs = append(msgs, Message{
			Role:    RoleSystem,
			Content: sep + b.cfg.DynamicSuffix,
		})
	}

	return msgs
}

// hasCacheableContent 判断内容是否可标记为缓存。
// 当内容长度超过阈值时认为可缓存（避免过短的段浪费缓存槽位）。
func hasCacheableContent(content string) bool {
	return len([]rune(content)) > 100
}

// ---------------------------------------------------------------------------
// 便捷构建函数
// ---------------------------------------------------------------------------

// BuildSystemPrompt 是单步构建完整 system prompt 的便捷函数。
func BuildSystemPrompt(cfg SystemPromptConfig) string {
	return NewSystemPromptBuilder(cfg).Build()
}

// InjectDynamicContext 将动态上下文附加到 system prompt 后缀。
func (cfg *SystemPromptConfig) InjectDynamicContext(cwd string, envVars map[string]string) {
	var b strings.Builder

	now := time.Now()
	fmt.Fprintf(&b, "当前时间: %s\n", now.Format("2006-01-02 15:04:05"))

	if cwd != "" {
		fmt.Fprintf(&b, "工作目录: %s\n", cwd)
	}

	if envVars != nil {
		// 按 key 排序
		keys := make([]string, 0, len(envVars))
		for k := range envVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if envVars[k] != "" {
				fmt.Fprintf(&b, "%s=%s\n", k, envVars[k])
			}
		}
	}

	cfg.DynamicSuffix = b.String()
}
