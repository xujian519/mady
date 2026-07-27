package evidence

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionNameDomain 是领域证据判断扩展的注册名称。
const ExtensionNameDomain = "evidence_judge"

// EvidenceDomainExtension 将专利证据判断工具集注入 Agent。
type EvidenceDomainExtension struct {
	engine *DefaultEngine
}

var (
	_ agentcore.Extension    = (*EvidenceDomainExtension)(nil)
	_ agentcore.ToolProvider = (*EvidenceDomainExtension)(nil)
)

// NewDomainExtension 创建领域证据判断扩展。
func NewDomainExtension(index *RuleIndex) *EvidenceDomainExtension {
	return &EvidenceDomainExtension{engine: NewEngine(index)}
}

// Name returns the extension identifier.
func (e *EvidenceDomainExtension) Name() string { return ExtensionNameDomain }

// Init initializes the evidence extension — currently a no-op.
func (e *EvidenceDomainExtension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose cleans up the evidence extension — currently a no-op.
func (e *EvidenceDomainExtension) Dispose() error { return nil }

// Tools returns the evidence judgment tool set for agent use.
func (e *EvidenceDomainExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newTripleTool(e.engine),
		newBurdenTool(),
		newStandardTool(),
		newDetermineStandardTool(),
		newConflictTool(e.engine),
		newTypeSpecificTool(e.engine),
		newCredibilityTool(),
	}
}
