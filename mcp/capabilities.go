package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ServerCapabilities describes the capabilities advertised by an MCP server.
type ServerCapabilities struct {
	Tools     ToolCapabilities     `json:"tools,omitempty"`
	Resources ResourceCapabilities `json:"resources,omitempty"`
	Prompts   PromptCapabilities   `json:"prompts,omitempty"`
}

// ToolCapabilities describes tool-related server capabilities.
type ToolCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourceCapabilities describes resource-related server capabilities.
type ResourceCapabilities struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// PromptCapabilities describes prompt-related server capabilities.
type PromptCapabilities struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type capabilityState struct {
	mu           sync.RWMutex
	capabilities ServerCapabilities
	hooks        []func(context.Context, ServerCapabilities)
}

func newCapabilityState() *capabilityState {
	return &capabilityState{}
}

func (s *capabilityState) get() ServerCapabilities {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capabilities
}

func (s *capabilityState) set(ctx context.Context, capabilities ServerCapabilities) {
	s.mu.Lock()
	s.capabilities = capabilities
	hooks := append([]func(context.Context, ServerCapabilities){}, s.hooks...)
	s.mu.Unlock()
	for _, hook := range hooks {
		hook(ctx, capabilities)
	}
}

func (s *capabilityState) addHook(h func(context.Context, ServerCapabilities)) {
	if h == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks = append(s.hooks, h)
}

// Capabilities returns the last known set of server capabilities.
func (c *Client) Capabilities() ServerCapabilities {
	if c.capState == nil {
		return ServerCapabilities{}
	}
	return c.capState.get()
}

// Capabilities returns the last known set of server capabilities.
func (c *HTTPClient) Capabilities() ServerCapabilities {
	if c.capState == nil {
		return ServerCapabilities{}
	}
	return c.capState.get()
}

// AddCapabilityHook registers a hook that fires when capabilities change.
func (c *Client) AddCapabilityHook(h func(context.Context, ServerCapabilities)) {
	if c.capState != nil {
		c.capState.addHook(h)
	}
}

// AddCapabilityHook registers a hook that fires when capabilities change.
func (c *HTTPClient) AddCapabilityHook(h func(context.Context, ServerCapabilities)) {
	if c.capState != nil {
		c.capState.addHook(h)
	}
}

// SupportsToolListChanged reports whether the server supports tools/list/changed notifications.
func (c *Client) SupportsToolListChanged() bool {
	return c.Capabilities().Tools.ListChanged
}

// SupportsToolListChanged reports whether the server supports tools/list/changed notifications.
func (c *HTTPClient) SupportsToolListChanged() bool {
	return c.Capabilities().Tools.ListChanged
}

// SupportsResourceSubscribe reports whether the server supports resource subscriptions.
func (c *Client) SupportsResourceSubscribe() bool {
	return c.Capabilities().Resources.Subscribe
}

// SupportsResourceSubscribe reports whether the server supports resource subscriptions.
func (c *HTTPClient) SupportsResourceSubscribe() bool {
	return c.Capabilities().Resources.Subscribe
}

// SupportsResourceListChanged reports whether the server supports resources/list/changed.
func (c *Client) SupportsResourceListChanged() bool {
	return c.Capabilities().Resources.ListChanged
}

// SupportsResourceListChanged reports whether the server supports resources/list/changed.
func (c *HTTPClient) SupportsResourceListChanged() bool {
	return c.Capabilities().Resources.ListChanged
}

// SupportsPromptListChanged reports whether the server supports prompts/list/changed.
func (c *Client) SupportsPromptListChanged() bool {
	return c.Capabilities().Prompts.ListChanged
}

// SupportsPromptListChanged reports whether the server supports prompts/list/changed.
func (c *HTTPClient) SupportsPromptListChanged() bool {
	return c.Capabilities().Prompts.ListChanged
}

func decodeCapabilities(raw json.RawMessage) (ServerCapabilities, error) {
	if len(raw) == 0 {
		return ServerCapabilities{}, nil
	}
	var caps ServerCapabilities
	if err := json.Unmarshal(raw, &caps); err != nil {
		return ServerCapabilities{}, fmt.Errorf("mcp: decode capabilities: %w", err)
	}
	return caps, nil
}
