package evidence

import "github.com/xujian519/mady/agentcore"

func newTripleTool(engine *DefaultEngine) *agentcore.Tool {
	return &agentcore.Tool{Name: "judge_triple"}
}
func newBurdenTool() *agentcore.Tool   { return &agentcore.Tool{Name: "check_burden"} }
func newStandardTool() *agentcore.Tool { return &agentcore.Tool{Name: "assess_standard"} }
func newConflictTool(engine *DefaultEngine) *agentcore.Tool {
	return &agentcore.Tool{Name: "detect_conflict"}
}
func newTypeSpecificTool(engine *DefaultEngine) *agentcore.Tool {
	return &agentcore.Tool{Name: "judge_type_specific"}
}
