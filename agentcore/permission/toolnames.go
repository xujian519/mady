package permission

// Tool name constants used in permission policies.
//
// These are duplicated from the tools package to avoid a cross-module
// dependency from agentcore (kernel layer) to tools (outer layer).
const (
	ToolBash        = "bash"
	ToolExecuteCode = "execute_code"
	ToolComputerUse = "computer_use"
)
