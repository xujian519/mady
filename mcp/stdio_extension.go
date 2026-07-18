package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/util"
)

// StdioExtension 是 MCP stdio 扩展的抽象层，包装 Client 实例。
type StdioExtension struct {
	name              string
	cfg               StdioConfig
	client            *Client
	tools             []*agentcore.Tool
	agent             *agentcore.Agent
	toolNames         []string
	refreshMu         sync.Mutex
	refreshScheduleMu sync.Mutex
	refreshInFlight   bool
	refreshPending    bool
}

func NewStdioExtension(ctx context.Context, cfg StdioConfig) (*StdioExtension, error) {
	client, err := NewStdioClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	tools, err := client.AgentTools(ctx)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	name := cfg.Name
	if name == "" {
		name = "mcp"
	}
	return &StdioExtension{
		name:   name,
		cfg:    cfg,
		client: client,
		tools:  tools,
	}, nil
}

func (e *StdioExtension) Name() string { return e.name }
func (e *StdioExtension) Init(ctx context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	e.toolNames = toolNames(e.tools)
	if e.client != nil {
		e.client.SetEventSink(agent.EmitEvent)
		e.client.AddNotificationHook(func(ctx context.Context, method string, params json.RawMessage) error {
			if method != "notifications/tools/list_changed" || !e.client.SupportsToolListChanged() {
				return nil
			}
			e.scheduleRefresh()
			return nil
		})
		e.client.AddCapabilityHook(func(ctx context.Context, caps ServerCapabilities) {
			e.emitCapabilitiesEvent(caps)
		})
		e.emitCapabilitiesEvent(e.client.Capabilities())
	}
	return nil
}
func (e *StdioExtension) Client() *Client { return e.client }
func (e *StdioExtension) Dispose() error {
	if e.client == nil {
		return nil
	}
	return e.client.Close()
}
func (e *StdioExtension) Tools() []*agentcore.Tool {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()
	return append([]*agentcore.Tool(nil), e.tools...)
}
func (e *StdioExtension) SnapshotEvents() []agentcore.Event {
	if e.client == nil {
		return nil
	}
	return []agentcore.Event{CapabilitiesUpdatedEvent{
		At:           time.Now(),
		Extension:    e.name,
		Transport:    "stdio",
		Capabilities: e.client.Capabilities(),
	}}
}

func (e *StdioExtension) emitCapabilitiesEvent(caps ServerCapabilities) {
	if e.agent == nil {
		return
	}
	e.agent.EmitEvent(CapabilitiesUpdatedEvent{
		At:           time.Now(),
		Extension:    e.name,
		Transport:    "stdio",
		Capabilities: caps,
	})
}

func decodeArguments(args json.RawMessage) (map[string]any, error) {
	if len(args) == 0 || string(args) == "null" {
		return map[string]any{}, nil
	}
	var decoded any
	if err := json.Unmarshal(args, &decoded); err != nil {
		return nil, fmt.Errorf("mcp decode arguments: %w", err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp tool arguments must be a JSON object")
	}
	return m, nil
}

func formatToolResult(result *ToolResult) string {
	if result == nil {
		return ""
	}
	text := strings.TrimSpace(formatToolContent(result.Content))
	if text == "" && result.StructuredContent != nil {
		data, marshalErr := json.Marshal(result.StructuredContent)
		if marshalErr == nil {
			text = string(data)
		}
	}
	if result.IsError {
		text = strings.TrimSpace(text)
		if text == "" {
			text = "MCP tool returned an error"
		}
		if !strings.HasPrefix(strings.ToLower(text), "error:") {
			text = "Error: " + text
		}
	}
	return text
}

func formatToolContent(items []ToolResultContent) string {
	var parts []string
	for _, item := range items {
		switch item.Type {
		case "text":
			if strings.TrimSpace(item.Text) != "" {
				parts = append(parts, item.Text)
			}
		case "image":
			parts = append(parts, fmt.Sprintf("[image %s]", item.MIMEType))
		case "audio":
			parts = append(parts, fmt.Sprintf("[audio %s]", item.MIMEType))
		case "resource_link":
			if item.Name != "" {
				parts = append(parts, fmt.Sprintf("[resource] %s %s", item.Name, item.URI))
			} else {
				parts = append(parts, fmt.Sprintf("[resource] %s", item.URI))
			}
		case "resource":
			if item.Resource != nil {
				if strings.TrimSpace(item.Resource.Text) != "" {
					parts = append(parts, item.Resource.Text)
				} else {
					parts = append(parts, fmt.Sprintf("[resource] %s", item.Resource.URI))
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

func agentToolsFor(ctx context.Context, prefix string, bridge toolBridge) ([]*agentcore.Tool, error) {
	tools, err := bridge.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*agentcore.Tool, 0, len(tools))
	for _, tool := range tools {
		toolName := qualifyToolName(prefix, tool.Name)
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			}
		}
		originalName := tool.Name
		out = append(out, &agentcore.Tool{
			Name:        toolName,
			Description: util.FirstNonEmpty(tool.Description, tool.Title, tool.Name),
			Parameters:  schema,
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				decoded, err := decodeArguments(args)
				if err != nil {
					return nil, err
				}
				result, err := bridge.CallTool(ctx, originalName, decoded)
				if err != nil {
					return nil, err
				}
				return formatToolResult(result), nil
			},
		})
	}
	return out, nil
}

func qualifyToolName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + name
}
