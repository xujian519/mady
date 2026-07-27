package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// WorkflowDescriptor describes a registered workflow in the central registry.
// It captures metadata and execution strategy regardless of the underlying
// definition format (Pregel graph, YAML orchestration, or plugin.json).
type WorkflowDescriptor struct {
	// Name is the unique workflow identifier (e.g., "novelty-analysis").
	Name string

	// Domain is the functional domain (patent, legal, design, general).
	Domain string

	// Description explains what this workflow does.
	Description string

	// ToolName is the name of the agent tool that triggers this workflow.
	// Empty means this is a data-only descriptor (no direct tool binding).
	ToolName string

	// Manifest is the workflow definition (can be nil for Pregel-only tools).
	Manifest WorkflowManifest

	// Executor is the execution engine for this workflow. When nil, the
	// workflow is registered for discovery only (no remote execution).
	Executor WorkflowExecutor
}

// WorkflowRegistry is a central catalog of all available workflows.
//
// It serves two purposes:
//  1. Discovery: list workflows by domain, name, or tool name for CLI/Server APIs.
//  2. Execution: provide Executor for manifest-based workflows (Plugin/Orchestration).
//
// Pregel-based tools (registered via ExtraTools) are registered as descriptors
// for discovery without executor binding — their execution goes through the
// standard agent.InvokeTool path.
type WorkflowRegistry struct {
	mu     sync.RWMutex
	byName map[string]WorkflowDescriptor // keyed by workflow Name
	byTool map[string]WorkflowDescriptor // keyed by ToolName
}

// NewWorkflowRegistry creates an empty workflow registry.
func NewWorkflowRegistry() *WorkflowRegistry {
	return &WorkflowRegistry{
		byName: make(map[string]WorkflowDescriptor),
		byTool: make(map[string]WorkflowDescriptor),
	}
}

// Register adds a workflow descriptor to the registry.
// If a descriptor with the same Name already exists, it is overwritten.
func (r *WorkflowRegistry) Register(d WorkflowDescriptor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName[d.Name] = d
	if d.ToolName != "" {
		r.byTool[d.ToolName] = d
	}
}

// Lookup returns the workflow descriptor by name, or nil if not found.
func (r *WorkflowRegistry) Lookup(name string) *WorkflowDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byName[name]
	if !ok {
		return nil
	}
	return &d
}

// LookupByTool returns the workflow descriptor associated with the given
// tool name, or nil if not found.
func (r *WorkflowRegistry) LookupByTool(toolName string) *WorkflowDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byTool[toolName]
	if !ok {
		return nil
	}
	return &d
}

// List returns all registered workflows, sorted by name.
func (r *WorkflowRegistry) List() []WorkflowDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]WorkflowDescriptor, 0, len(r.byName))
	for _, d := range r.byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ListByDomain returns workflows matching the given domain, sorted by name.
func (r *WorkflowRegistry) ListByDomain(domain string) []WorkflowDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []WorkflowDescriptor
	for _, d := range r.byName {
		if d.Domain == domain {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Export returns a JSON-serializable summary of all registered workflows.
// Each entry includes Name, Domain, Description, and ToolName. Manifest
// contents are excluded to keep the export lightweight.
func (r *WorkflowRegistry) Export() []map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]map[string]string, 0, len(r.byName))
	for _, d := range r.byName {
		out = append(out, map[string]string{
			"name":        d.Name,
			"domain":      d.Domain,
			"description": d.Description,
			"tool":        d.ToolName,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"] < out[j]["name"] })
	return out
}

// =============================================================================
// Registry-based Extension builder
// =============================================================================

// registryExtension provides tools for all workflows in the registry.
type registryExtension struct {
	registry *WorkflowRegistry
	domain   string // empty = all domains
	tools    []*Tool
}

// BuildExtension creates an Extension that provides tools for all registered
// workflows across all domains. Each workflow with a non-empty ToolName
// gets a synthetic tool that delegates to the Executor.
//
// For workflows without an Executor (e.g., Pregel tools), the caller must
// register the actual tools separately (via ExtraTools or other extensions).
func (r *WorkflowRegistry) BuildExtension() Extension {
	return r.buildExtension("")
}

// BuildDomainExtension creates an Extension scoped to a specific domain.
// Only workflows in the given domain are included.
func (r *WorkflowRegistry) BuildDomainExtension(domain string) Extension {
	return r.buildExtension(domain)
}

func (r *WorkflowRegistry) buildExtension(domain string) Extension {
	descs := r.List()
	if domain != "" {
		descs = r.ListByDomain(domain)
	}
	var tools []*Tool
	for _, d := range descs {
		if d.ToolName == "" {
			continue
		}
		if d.Executor == nil {
			// No executor — let the caller register the tool separately.
			continue
		}
		desc := d // capture
		tools = append(tools, &Tool{
			Name:        desc.ToolName,
			Description: desc.Description,
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
			Func: func(ctx context.Context, _ json.RawMessage) (any, error) {
				result, err := desc.Executor.Execute(ctx, desc.Manifest, nil)
				if err != nil {
					return nil, fmt.Errorf("workflow %q: %w", desc.Name, err)
				}
				return result.Output, nil
			},
		})
	}
	return &registryExtension{
		registry: r,
		domain:   domain,
		tools:    tools,
	}
}

func (e *registryExtension) Name() string {
	if e.domain != "" {
		return fmt.Sprintf("workflow-registry-%s", e.domain)
	}
	return "workflow-registry"
}

func (e *registryExtension) Init(_ context.Context, _ *Agent) error { return nil }
func (e *registryExtension) Dispose() error                         { return nil }
func (e *registryExtension) Tools() []*Tool                         { return e.tools }
