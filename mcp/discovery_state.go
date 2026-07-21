package mcp

// This file contains discoveryState cache/state management methods, extracted
// from discovery.go to separate data access from discovery logic.

import (
	"context"
	"encoding/json"
	"fmt"
)

func newDiscoveryState(cfg DiscoveryConfig) *discoveryState {
	return &discoveryState{
		cfg:                 cfg,
		resourceContents:    make(map[string]*ReadResourceResult),
		promptResults:       make(map[string]*PromptResult),
		subscribedResources: make(map[string]struct{}),
	}
}

func (s *discoveryState) cachedResources() ([]Resource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.resourcesLoaded {
		return nil, false
	}
	return append([]Resource(nil), s.resources...), true
}

func (s *discoveryState) storeResources(resources []Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = append([]Resource(nil), resources...)
	s.resourcesLoaded = true
}

func (s *discoveryState) cachedResource(uri string) (*ReadResourceResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, ok := s.resourceContents[uri]
	if !ok {
		return nil, false
	}
	clone := *res
	clone.Contents = append([]EmbeddedResource(nil), res.Contents...)
	return &clone, true
}

func (s *discoveryState) storeResource(uri string, result *ReadResourceResult) {
	if result == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *result
	clone.Contents = append([]EmbeddedResource(nil), result.Contents...)
	s.resourceContents[uri] = &clone
}

func (s *discoveryState) cachedResourceTemplates() ([]ResourceTemplate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.resourceTemplatesLoaded {
		return nil, false
	}
	return append([]ResourceTemplate(nil), s.resourceTemplates...), true
}

func (s *discoveryState) storeResourceTemplates(templates []ResourceTemplate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resourceTemplates = append([]ResourceTemplate(nil), templates...)
	s.resourceTemplatesLoaded = true
}

func (s *discoveryState) cachedPrompts() ([]Prompt, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.promptsLoaded {
		return nil, false
	}
	return append([]Prompt(nil), s.prompts...), true
}

func (s *discoveryState) storePrompts(prompts []Prompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append([]Prompt(nil), prompts...)
	s.promptsLoaded = true
}

func (s *discoveryState) cachedPrompt(key string) (*PromptResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, ok := s.promptResults[key]
	if !ok {
		return nil, false
	}
	clone := *res
	clone.Messages = append([]PromptMessage(nil), res.Messages...)
	return &clone, true
}

func (s *discoveryState) storePrompt(key string, result *PromptResult) {
	if result == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *result
	clone.Messages = append([]PromptMessage(nil), result.Messages...)
	s.promptResults[key] = &clone
}

func (s *discoveryState) markSubscribed(uri string, subscribed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if subscribed {
		s.subscribedResources[uri] = struct{}{}
		return
	}
	delete(s.subscribedResources, uri)
}

func (s *discoveryState) invalidateResource(uri string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resourceContents, uri)
	_, subscribed := s.subscribedResources[uri]
	return subscribed
}

func (s *discoveryState) invalidateResourcesList() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = nil
	s.resourcesLoaded = false
	s.resourceTemplates = nil
	s.resourceTemplatesLoaded = false
}

func (s *discoveryState) invalidatePromptsList() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = nil
	s.promptsLoaded = false
	s.promptResults = make(map[string]*PromptResult)
}

func (s *discoveryState) onResourceUpdated(ctx context.Context, uri string, result *ReadResourceResult) {
	if s.cfg.ResourceUpdatedHandler != nil {
		s.cfg.ResourceUpdatedHandler(ctx, uri, result)
	}
}

func (s *discoveryState) onResourcesListChanged(ctx context.Context) {
	if s.cfg.ResourcesListChangedHandler != nil {
		s.cfg.ResourcesListChangedHandler(ctx)
	}
}

func (s *discoveryState) onPromptsListChanged(ctx context.Context) {
	if s.cfg.PromptsListChangedHandler != nil {
		s.cfg.PromptsListChangedHandler(ctx)
	}
}

func (s *discoveryState) onAsyncRefreshError(ctx context.Context, err error) {
	if err != nil && s.cfg.AsyncRefreshErrorHandler != nil {
		s.cfg.AsyncRefreshErrorHandler(ctx, err)
	}
}

func promptCacheKey(name string, arguments map[string]any) (string, error) {
	if arguments == nil {
		return name, nil
	}
	data, err := json.Marshal(arguments)
	if err != nil {
		return "", fmt.Errorf("mcp encode prompt cache key: %w", err)
	}
	return name + "\x00" + string(data), nil
}
