package mcp

import (
	"context"
	"fmt"
)

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
