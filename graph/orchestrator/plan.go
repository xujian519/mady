package orchestrator

// ExecutionPlan describes a sequence of nodes to execute with optional
// retry and concurrency configuration.
type ExecutionPlan struct {
	// ID uniquely identifies this plan for checkpoint and resume.
	ID string `json:"id"`

	// Input is the initial data passed to the entry node.
	Input string `json:"input"`

	// Nodes lists all nodes in the execution plan.
	Nodes []PlanNode `json:"nodes"`

	// Edges defines the connections between nodes.
	Edges []PlanEdge `json:"edges,omitempty"`
}

// PlanNode describes a single step in the execution plan.
type PlanNode struct {
	// ID uniquely identifies this node within the plan.
	ID string `json:"id"`

	// Tool is the tool name to invoke (e.g., "read", "analyze").
	Tool string `json:"tool"`

	// Config provides tool-specific parameters.
	Config map[string]any `json:"config,omitempty"`

	// Retry specifies the maximum number of retries on failure.
	// NOTE: This field is not yet enforced by the Orchestrator.
	// Retry behavior should be configured via graph.NodePolicy during
	// graph compilation until this field is wired in.
	Retry int `json:"retry,omitempty"`
}

// PlanEdge defines a directed edge between two nodes.
type PlanEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// PlanResult summarizes the outcome of an orchestrated execution.
type PlanResult struct {
	PlanID  string `json:"plan_id"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Steps   int    `json:"steps"`
}
