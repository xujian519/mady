package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Icon describes an icon resource for MCP resources and prompts.
type Icon struct {
	Src      string   `json:"src"`
	MIMEType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// Annotations carries optional metadata annotations for MCP resources.
type Annotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     float64  `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
}

// Resource describes an MCP resource available from the server.
type Resource struct {
	URI         string       `json:"uri"`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	MIMEType    string       `json:"mimeType,omitempty"`
	Size        int64        `json:"size,omitempty"`
	Icons       []Icon       `json:"icons,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// ResourceTemplate describes a URI template pattern for MCP resources.
type ResourceTemplate struct {
	URITemplate string       `json:"uriTemplate"`
	Name        string       `json:"name"`
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	MIMEType    string       `json:"mimeType,omitempty"`
	Icons       []Icon       `json:"icons,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// ReadResourceResult contains the content returned from reading a resource.
type ReadResourceResult struct {
	Contents []EmbeddedResource `json:"contents"`
}

// Prompt describes an MCP prompt template available from the server.
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Icons       []Icon           `json:"icons,omitempty"`
}

// PromptArgument describes a parameter accepted by a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptResult contains the result of rendering a prompt template.
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage represents a single message within a prompt result.
type PromptMessage struct {
	Role    string        `json:"role"`
	Content PromptContent `json:"content"`
}

// PromptContent holds the content body of a prompt message.
type PromptContent struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	Data        string            `json:"data,omitempty"`
	MIMEType    string            `json:"mimeType,omitempty"`
	Resource    *EmbeddedResource `json:"resource,omitempty"`
	Annotations *Annotations      `json:"annotations,omitempty"`
}

type resourceListResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type resourceTemplateListResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	NextCursor        string             `json:"nextCursor,omitempty"`
}

type promptListResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

type discoveryRPC func(ctx context.Context, method string, params any, out any) error

// listWithCursor is a generic helper for cursor-paginated resource listing.
// It eliminates dupl across listResources, listResourceTemplates, and listPrompts.
func listWithCursor[T any](
	fetchPage func(cursor string) (items []T, nextCursor string, err error)) ([]T, error) {
	var out []T
	cursor := ""
	for {
		items, nextCursor, err := fetchPage(cursor)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if nextCursor == "" {
			return out, nil
		}
		cursor = nextCursor
	}
}

// DiscoveryClient is the interface for MCP resource and prompt discovery operations.
type DiscoveryClient interface {
	ListResources(ctx context.Context) ([]Resource, error)
	ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error)
	ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error)
	SubscribeResource(ctx context.Context, uri string) error
	UnsubscribeResource(ctx context.Context, uri string) error
	ListPrompts(ctx context.Context) ([]Prompt, error)
	GetPrompt(ctx context.Context, name string, arguments map[string]any) (*PromptResult, error)
}

// DiscoveryToolConfig configures how discovery tools are exposed to the agent.
type DiscoveryToolConfig struct {
	Name             string
	ToolPrefix       string
	IncludeResources bool
	IncludePrompts   bool
}

// DiscoveryExtension is an agentcore.Extension that exposes MCP resource
// and prompt discovery tools to the agent.
type DiscoveryExtension struct {
	name  string
	tools []*agentcore.Tool
}

// DiscoveryConfig configures async discovery event handlers.
type DiscoveryConfig struct {
	ResourceUpdatedHandler      func(context.Context, string, *ReadResourceResult)
	ResourcesListChangedHandler func(context.Context)
	PromptsListChangedHandler   func(context.Context)
	AsyncRefreshErrorHandler    func(context.Context, error)
}

type discoveryState struct {
	mu sync.RWMutex

	cfg DiscoveryConfig

	resources               []Resource
	resourcesLoaded         bool
	resourceTemplates       []ResourceTemplate
	resourceTemplatesLoaded bool
	prompts                 []Prompt
	promptsLoaded           bool
	resourceContents        map[string]*ReadResourceResult
	promptResults           map[string]*PromptResult
	subscribedResources     map[string]struct{}
}

// NewDiscoveryExtension creates a new DiscoveryExtension from a DiscoveryClient.
func NewDiscoveryExtension(client DiscoveryClient, cfg DiscoveryToolConfig) (*DiscoveryExtension, error) {
	if client == nil {
		return nil, fmt.Errorf("mcp: discovery client is required")
	}
	var tools []*agentcore.Tool
	if cfg.IncludeResources {
		tools = append(tools, discoveryResourceTools(client, cfg.ToolPrefix)...)
	}
	if cfg.IncludePrompts {
		tools = append(tools, discoveryPromptTools(client, cfg.ToolPrefix)...)
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("mcp: no discovery tools enabled")
	}
	name := cfg.Name
	if name == "" {
		name = "mcp-discovery"
	}
	return &DiscoveryExtension{name: name, tools: tools}, nil
}

// Name returns the extension name.
func (e *DiscoveryExtension) Name() string { return e.name }

// Init initializes the discovery extension with the agent.
func (e *DiscoveryExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose cleans up the discovery extension.
func (e *DiscoveryExtension) Dispose() error { return nil }

// Tools returns the agent tools exposed by this extension.
func (e *DiscoveryExtension) Tools() []*agentcore.Tool { return e.tools }

// ListResources returns all resources from the MCP server.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.discovery, c.invokeDiscovery)
}

// ReadResource reads a resource by URI from the MCP server.
func (c *Client) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// ListResourceTemplates returns all resource templates from the MCP server.
func (c *Client) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.discovery, c.invokeDiscovery)
}

// SubscribeResource subscribes to updates for a resource URI.
func (c *Client) SubscribeResource(ctx context.Context, uri string) error {
	if !c.SupportsResourceSubscribe() {
		return fmt.Errorf("mcp: server does not advertise resources.subscribe")
	}
	return subscribeResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// UnsubscribeResource unsubscribes from updates for a resource URI.
func (c *Client) UnsubscribeResource(ctx context.Context, uri string) error {
	if !c.SupportsResourceSubscribe() {
		return fmt.Errorf("mcp: server does not advertise resources.subscribe")
	}
	return unsubscribeResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// ListPrompts returns all prompts from the MCP server.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.discovery, c.invokeDiscovery)
}

// GetPrompt retrieves a prompt by name with optional arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]any) (*PromptResult, error) {
	return getPrompt(ctx, c.discovery, c.invokeDiscovery, name, arguments)
}

// ListResources returns all resources from the MCP server.
func (c *HTTPClient) ListResources(ctx context.Context) ([]Resource, error) {
	return listResources(ctx, c.discovery, c.invokeDiscovery)
}

// ReadResource reads a resource by URI from the MCP server.
func (c *HTTPClient) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return readResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// ListResourceTemplates returns all resource templates from the MCP server.
func (c *HTTPClient) ListResourceTemplates(ctx context.Context) ([]ResourceTemplate, error) {
	return listResourceTemplates(ctx, c.discovery, c.invokeDiscovery)
}

// SubscribeResource subscribes to updates for a resource URI.
func (c *HTTPClient) SubscribeResource(ctx context.Context, uri string) error {
	if !c.SupportsResourceSubscribe() {
		return fmt.Errorf("mcp: server does not advertise resources.subscribe")
	}
	return subscribeResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// UnsubscribeResource unsubscribes from updates for a resource URI.
func (c *HTTPClient) UnsubscribeResource(ctx context.Context, uri string) error {
	if !c.SupportsResourceSubscribe() {
		return fmt.Errorf("mcp: server does not advertise resources.subscribe")
	}
	return unsubscribeResource(ctx, c.discovery, c.invokeDiscovery, uri)
}

// ListPrompts returns all prompts from the MCP server.
func (c *HTTPClient) ListPrompts(ctx context.Context) ([]Prompt, error) {
	return listPrompts(ctx, c.discovery, c.invokeDiscovery)
}

// GetPrompt retrieves a prompt by name with optional arguments.
func (c *HTTPClient) GetPrompt(ctx context.Context, name string, arguments map[string]any) (*PromptResult, error) {
	return getPrompt(ctx, c.discovery, c.invokeDiscovery, name, arguments)
}

func (c *Client) invokeDiscovery(ctx context.Context, method string, params any, out any) error {
	return c.call(ctx, method, params, out)
}

func (c *HTTPClient) invokeDiscovery(ctx context.Context, method string, params any, out any) error {
	return c.call(ctx, method, params, out)
}

// listWithCache combines cache-check with listWithCursor, eliminating dupl
// across listResources, listResourceTemplates, and listPrompts.
func listWithCache[T any](
	cacheCheck func() ([]T, bool), store func([]T),
	fetchPage func(cursor string) (items []T, nextCursor string, err error)) ([]T, error) {
	if cached, ok := cacheCheck(); ok {
		return cached, nil
	}
	out, err := listWithCursor(fetchPage)
	if err != nil {
		return nil, err
	}
	store(out)
	return out, nil
}

func listResources(ctx context.Context, state *discoveryState, rpc discoveryRPC) ([]Resource, error) {
	return listWithCache(
		state.cachedResources, state.storeResources,
		func(cursor string) ([]Resource, string, error) {
			params := map[string]any{}
			if cursor != "" {
				params["cursor"] = cursor
			}
			var result resourceListResult
			if err := rpc(ctx, "resources/list", params, &result); err != nil {
				return nil, "", err
			}
			return result.Resources, result.NextCursor, nil
		})
}

func readResource(ctx context.Context, state *discoveryState, rpc discoveryRPC, uri string) (*ReadResourceResult, error) {
	if result, ok := state.cachedResource(uri); ok {
		return result, nil
	}
	var result ReadResourceResult
	if err := rpc(ctx, "resources/read", map[string]any{"uri": uri}, &result); err != nil {
		return nil, err
	}
	state.storeResource(uri, &result)
	return &result, nil
}

func listResourceTemplates(ctx context.Context, state *discoveryState, rpc discoveryRPC) ([]ResourceTemplate, error) {
	return listWithCache(
		state.cachedResourceTemplates, state.storeResourceTemplates,
		func(cursor string) ([]ResourceTemplate, string, error) {
			params := map[string]any{}
			if cursor != "" {
				params["cursor"] = cursor
			}
			var result resourceTemplateListResult
			if err := rpc(ctx, "resources/templates/list", params, &result); err != nil {
				return nil, "", err
			}
			return result.ResourceTemplates, result.NextCursor, nil
		})
}

func listPrompts(ctx context.Context, state *discoveryState, rpc discoveryRPC) ([]Prompt, error) {
	return listWithCache(
		state.cachedPrompts, state.storePrompts,
		func(cursor string) ([]Prompt, string, error) {
			params := map[string]any{}
			if cursor != "" {
				params["cursor"] = cursor
			}
			var result promptListResult
			if err := rpc(ctx, "prompts/list", params, &result); err != nil {
				return nil, "", err
			}
			return result.Prompts, result.NextCursor, nil
		})
}

func getPrompt(ctx context.Context, state *discoveryState, rpc discoveryRPC, name string, arguments map[string]any) (*PromptResult, error) {
	key, err := promptCacheKey(name, arguments)
	if err != nil {
		return nil, err
	}
	if result, ok := state.cachedPrompt(key); ok {
		return result, nil
	}
	params := map[string]any{"name": name}
	if arguments != nil {
		params["arguments"] = arguments
	}
	var result PromptResult
	if err := rpc(ctx, "prompts/get", params, &result); err != nil {
		return nil, err
	}
	state.storePrompt(key, &result)
	return &result, nil
}

func subscribeResource(ctx context.Context, state *discoveryState, rpc discoveryRPC, uri string) error {
	if state == nil {
		return fmt.Errorf("mcp: discovery state is required")
	}
	state.markSubscribed(uri, true)
	var result map[string]any
	if err := rpc(ctx, "resources/subscribe", map[string]any{"uri": uri}, &result); err != nil {
		state.markSubscribed(uri, false)
		return err
	}
	return nil
}

func unsubscribeResource(ctx context.Context, state *discoveryState, rpc discoveryRPC, uri string) error {
	var result map[string]any
	if err := rpc(ctx, "resources/unsubscribe", map[string]any{"uri": uri}, &result); err != nil {
		return err
	}
	state.markSubscribed(uri, false)
	return nil
}

// AgentMessages converts prompt messages to agentcore messages.
func (r PromptResult) AgentMessages() []agentcore.Message {
	out := make([]agentcore.Message, 0, len(r.Messages))
	for _, msg := range r.Messages {
		out = append(out, msg.AgentMessage())
	}
	return out
}

// AgentMessage converts a prompt message to an agentcore message.
func (m PromptMessage) AgentMessage() agentcore.Message {
	out := agentcore.Message{}
	switch m.Role {
	case "assistant":
		out.Role = agentcore.RoleAssistant
	default:
		out.Role = agentcore.RoleUser
	}
	switch m.Content.Type {
	case "text":
		out.Content = m.Content.Text
	case "image":
		out = out.AppendImageURLBlock(dataURL(m.Content.MIMEType, m.Content.Data))
	case "resource":
		if m.Content.Resource != nil {
			if m.Content.Resource.Text != "" {
				out.Content = m.Content.Resource.Text
			} else {
				out.Content = m.Content.Resource.URI
			}
		}
	default:
		if m.Content.Text != "" {
			out.Content = m.Content.Text
		} else if m.Content.Resource != nil && m.Content.Resource.Text != "" {
			out.Content = m.Content.Resource.Text
		}
	}
	return out
}

// subUnsubResourceTool creates subscribe/unsubscribe tool definitions,
// consolidating their nearly identical structure.
func subUnsubResourceTool(prefix, action string, label string, subscribed bool, fn func(context.Context, string) error) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        qualifyToolName(prefix, "resources."+action),
		Description: label + " MCP resource updates by URI.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"uri": map[string]any{"type": "string"},
			},
			"required":             []string{"uri"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				URI string `json:"uri"`
			}
			if err := decodeObjectArgs(args, &in, "mcp decode "+action+" resource args"); err != nil {
				return nil, err
			}
			if err := fn(ctx, in.URI); err != nil {
				return nil, err
			}
			return map[string]any{"subscribed": subscribed, "uri": in.URI}, nil
		},
	}
}

func discoveryResourceTools(client DiscoveryClient, prefix string) []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        qualifyToolName(prefix, "resources.list"),
			Description: "List available MCP resources.",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
			Func: func(ctx context.Context, _ json.RawMessage) (any, error) {
				resources, err := client.ListResources(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"resources": resources}, nil
			},
		},
		{
			Name:        qualifyToolName(prefix, "resources.templates.list"),
			Description: "List available MCP resource templates.",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
			Func: func(ctx context.Context, _ json.RawMessage) (any, error) {
				templates, err := client.ListResourceTemplates(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"resource_templates": templates}, nil
			},
		},
		subUnsubResourceTool(prefix, "subscribe", "Subscribe to", true, client.SubscribeResource),
		subUnsubResourceTool(prefix, "unsubscribe", "Unsubscribe from", false, client.UnsubscribeResource),
		{
			Name:        qualifyToolName(prefix, "resources.read"),
			Description: "Read one MCP resource by URI.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"uri": map[string]any{"type": "string"},
				},
				"required":             []string{"uri"},
				"additionalProperties": false,
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var in struct {
					URI string `json:"uri"`
				}
				if err := decodeObjectArgs(args, &in, "mcp decode read resource args"); err != nil {
					return nil, err
				}
				return client.ReadResource(ctx, in.URI)
			},
		},
	}
}

func discoveryPromptTools(client DiscoveryClient, prefix string) []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        qualifyToolName(prefix, "prompts.list"),
			Description: "List available MCP prompts.",
			Parameters: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
			Func: func(ctx context.Context, _ json.RawMessage) (any, error) {
				prompts, err := client.ListPrompts(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"prompts": prompts}, nil
			},
		},
		{
			Name:        qualifyToolName(prefix, "prompts.get"),
			Description: "Get one MCP prompt with optional arguments.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"arguments": map[string]any{
						"type": "object",
					},
				},
				"required":             []string{"name"},
				"additionalProperties": false,
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var in struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments"`
				}
				if err := decodeObjectArgs(args, &in, "mcp decode get prompt args"); err != nil {
					return nil, err
				}
				return client.GetPrompt(ctx, in.Name, in.Arguments)
			},
		},
	}
}

func dataURL(mimeType, data string) string {
	if data == "" {
		return ""
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + data
}

func (c *Client) handleDiscoveryNotification(ctx context.Context, method string, params json.RawMessage) error {
	return handleDiscoveryNotification(ctx, c.discovery, method, params, c.refreshResourceAsync, c.refreshResourcesListAsync, c.refreshPromptsListAsync)
}

func (c *HTTPClient) handleDiscoveryNotification(ctx context.Context, method string, params json.RawMessage) error {
	return handleDiscoveryNotification(ctx, c.discovery, method, params, c.refreshResourceAsync, c.refreshResourcesListAsync, c.refreshPromptsListAsync)
}

func handleDiscoveryNotification(
	ctx context.Context,
	state *discoveryState,
	method string,
	params json.RawMessage,
	refreshResource func(string),
	refreshResourcesList func(),
	refreshPromptsList func(),
) error {
	switch method {
	case "notifications/resources/updated":
		var in struct {
			URI string `json:"uri"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err := json.Unmarshal(params, &in); err != nil {
				return fmt.Errorf("mcp: decode resources updated params: %w", err)
			}
		}
		if in.URI == "" {
			return nil
		}
		if subscribed := state.invalidateResource(in.URI); subscribed {
			refreshResource(in.URI)
		} else if state.cfg.ResourceUpdatedHandler != nil {
			state.cfg.ResourceUpdatedHandler(ctx, in.URI, nil)
		}
	case "notifications/resources/list_changed":
		state.invalidateResourcesList()
		if state.cfg.ResourcesListChangedHandler != nil {
			state.cfg.ResourcesListChangedHandler(ctx)
		}
		refreshResourcesList()
	case "notifications/prompts/list_changed":
		state.invalidatePromptsList()
		if state.cfg.PromptsListChangedHandler != nil {
			state.cfg.PromptsListChangedHandler(ctx)
		}
		refreshPromptsList()
	}
	return nil
}

// goSafe runs fn in a goroutine with panic recovery.
func goSafe(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("mcp: discovery goroutine panicked", "err", r, "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

// refreshResource refreshes a single resource asynchronously.
// Shared by Client and HTTPClient to eliminate mirror duplication.
func refreshResource(bgCtx context.Context, d *discoveryState, rpc discoveryRPC, uri string) {
	goSafe(func() {
		result, err := readResource(bgCtx, d, rpc, uri)
		if err != nil {
			if d.cfg.AsyncRefreshErrorHandler != nil {
				d.cfg.AsyncRefreshErrorHandler(bgCtx, err)
			} else {
				slog.Warn("mcp: async resource refresh failed", "uri", uri, "err", err)
			}
			return
		}
		if d.cfg.ResourceUpdatedHandler != nil {
			d.cfg.ResourceUpdatedHandler(bgCtx, uri, result)
		}
	})
}

// refreshResourcesList refreshes resource lists asynchronously.
// Shared by Client and HTTPClient to eliminate mirror duplication.
func refreshResourcesList(bgCtx context.Context, d *discoveryState, rpc discoveryRPC) {
	goSafe(func() {
		if _, err := listResources(bgCtx, d, rpc); err != nil {
			if d.cfg.AsyncRefreshErrorHandler != nil {
				d.cfg.AsyncRefreshErrorHandler(bgCtx, err)
			} else {
				slog.Warn("mcp: async resources list refresh failed", "err", err)
			}
			return
		}
		if _, err := listResourceTemplates(bgCtx, d, rpc); err != nil {
			if d.cfg.AsyncRefreshErrorHandler != nil {
				d.cfg.AsyncRefreshErrorHandler(bgCtx, err)
			} else {
				slog.Warn("mcp: async resource templates list refresh failed", "err", err)
			}
		}
	})
}

// refreshPromptsList refreshes the prompts list asynchronously.
// Shared by Client and HTTPClient to eliminate mirror duplication.
func refreshPromptsList(bgCtx context.Context, d *discoveryState, rpc discoveryRPC) {
	goSafe(func() {
		if _, err := listPrompts(bgCtx, d, rpc); err != nil {
			if d.cfg.AsyncRefreshErrorHandler != nil {
				d.cfg.AsyncRefreshErrorHandler(bgCtx, err)
			} else {
				slog.Warn("mcp: async prompts list refresh failed", "err", err)
			}
		}
	})
}

func (c *Client) refreshResourceAsync(uri string) {
	refreshResource(c.bgCtx, c.discovery, c.invokeDiscovery, uri)
}

func (c *Client) refreshResourcesListAsync() {
	refreshResourcesList(c.bgCtx, c.discovery, c.invokeDiscovery)
}

func (c *Client) refreshPromptsListAsync() {
	refreshPromptsList(c.bgCtx, c.discovery, c.invokeDiscovery)
}

func (c *HTTPClient) refreshResourceAsync(uri string) {
	refreshResource(c.bgCtx, c.discovery, c.invokeDiscovery, uri)
}

func (c *HTTPClient) refreshResourcesListAsync() {
	refreshResourcesList(c.bgCtx, c.discovery, c.invokeDiscovery)
}

func (c *HTTPClient) refreshPromptsListAsync() {
	refreshPromptsList(c.bgCtx, c.discovery, c.invokeDiscovery)
}

// Interface assertions.
var _ agentcore.Extension = (*DiscoveryExtension)(nil)
var _ agentcore.ToolProvider = (*DiscoveryExtension)(nil)
